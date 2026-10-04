// Package selfcapture 实现 fork 专属的请求载荷捕获：网关中间件抓取请求与响应快照，
// 异步写入 self_ 前缀表，并对外提供管理端查询端点。
// 设计约束：不改动上游表结构与既有链路，只通过装配处注入，便于随上游更新。
package selfcapture

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/TokenFlux/TokenRouter/internal/pkg/logredact"
)

const (
	// 请求体/响应体的存储上限。超过即截断并标记，避免大请求拖垮存储。
	bodyLimitBytes = 1 << 20
	// 请求头/响应头快照序列化后的上限。
	headerLimitBytes = 16 << 10
	// 单行捕获的清理概率分母：约 1/50 的写入会触发一次过期清理。
	cleanupSample = 50
	retentionDays = 7
	// 队列上限；满后丢弃新条目，捕获永远不阻塞主请求。
	queueSize = 512
)

// headerRedactedKeys 是写入前强制脱敏的请求头名单；值替换为固定占位。
var headerRedactedKeys = map[string]struct{}{
	"authorization":       {},
	"proxy-authorization": {},
	"cookie":              {},
	"set-cookie":          {},
	"x-api-key":           {},
	"api-key":             {},
	"x-goog-api-key":      {},
	"x-auth-token":        {},
}

// Entry 是一次请求的载荷快照。
type Entry struct {
	ClientRequestID       string
	RequestHeadersJSON    string
	ResponseHeadersJSON   string
	RequestBody           string
	ResponseBody          string
	RequestBodyTruncated  bool
	ResponseBodyTruncated bool
	StatusCode            int
	RequestPath           string
}

// Store 把快照写入 self_request_payloads 表。
type Store struct {
	db      *sql.DB
	queue   chan Entry
	dropped atomic.Int64
	// disabled 在首次写库失败后置位，避免数据库不可用时反复打日志和占用连接。
	disabled   atomic.Bool
	disabledMu sync.Mutex
}

// NewStore 创建存储并启动后台写入协程。
func NewStore(db *sql.DB) *Store {
	if db == nil {
		return nil
	}
	s := &Store{db: db, queue: make(chan Entry, queueSize)}
	go s.worker()
	return s
}

// Disabled 报告存储是否已因写库失败自动停用。
func (s *Store) Disabled() bool { return s != nil && s.disabled.Load() }

// DroppedTotal 返回因队列满或停用而丢弃的快照数。
func (s *Store) DroppedTotal() int64 { return s.dropped.Load() }

// Enqueue 异步入队一条快照；队列满或已停用时静默丢弃。
func (s *Store) Enqueue(entry Entry) {
	if s == nil || s.disabled.Load() {
		return
	}
	if entry.ClientRequestID == "" {
		return
	}
	select {
	case s.queue <- entry:
	default:
		s.dropped.Add(1)
	}
}

// Close 停止接收新条目并等待队列排空。
func (s *Store) Close() {
	if s == nil {
		return
	}
	s.disabledMu.Lock()
	if s.disabled.Load() {
		s.disabledMu.Unlock()
		return
	}
	s.disabled.Store(true)
	s.disabledMu.Unlock()
	close(s.queue)
}

func (s *Store) worker() {
	for entry := range s.queue {
		if err := s.insert(entry); err != nil {
			// 写库失败说明数据库不可用或表缺失：停用捕获，保护主链路。
			s.disabled.Store(true)
			return
		}
		// 插入成功时按固定概率清理过期数据，避免独立定时任务。
		if entry.ClientRequestID != "" && hashMod(entry.ClientRequestID, cleanupSample) == 0 {
			cutoff := time.Now().AddDate(0, 0, -retentionDays)
			_, _ = s.db.Exec(`DELETE FROM self_request_payloads WHERE created_at < $1`, cutoff)
		}
	}
}

func (s *Store) insert(entry Entry) error {
	_, err := s.db.Exec(`
		INSERT INTO self_request_payloads
			(client_request_id, request_headers, response_headers, request_body, response_body,
			 request_body_truncated, response_body_truncated, status_code, request_path)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (client_request_id) DO NOTHING`,
		entry.ClientRequestID,
		nullText(entry.RequestHeadersJSON),
		nullText(entry.ResponseHeadersJSON),
		nullText(entry.RequestBody),
		nullText(entry.ResponseBody),
		entry.RequestBodyTruncated,
		entry.ResponseBodyTruncated,
		entry.StatusCode,
		nullText(entry.RequestPath),
	)
	return err
}

// GetByClientRequestID 查询单条快照。
func (s *Store) GetByClientRequestID(ctx context.Context, clientRequestID string) (*Entry, error) {
	if s == nil {
		return nil, sql.ErrNoRows
	}
	var (
		e             Entry
		reqHeaders    sql.NullString
		respHeaders   sql.NullString
		reqBody       sql.NullString
		respBody      sql.NullString
		reqTruncated  bool
		respTruncated bool
		statusCode    int
		requestPath   sql.NullString
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT client_request_id, request_headers, response_headers, request_body, response_body,
		       request_body_truncated, response_body_truncated, status_code, request_path
		FROM self_request_payloads
		WHERE client_request_id = $1`, clientRequestID,
	).Scan(&e.ClientRequestID, &reqHeaders, &respHeaders, &reqBody, &respBody,
		&reqTruncated, &respTruncated, &statusCode, &requestPath)
	if err != nil {
		return nil, err
	}
	e.RequestHeadersJSON = reqHeaders.String
	e.ResponseHeadersJSON = respHeaders.String
	e.RequestBody = reqBody.String
	e.ResponseBody = respBody.String
	e.RequestBodyTruncated = reqTruncated
	e.ResponseBodyTruncated = respTruncated
	e.StatusCode = statusCode
	e.RequestPath = requestPath.String
	return &e, nil
}

// CaptureRequestBody 复制并截断请求体，返回存储文本与截断标记。
func CaptureRequestBody(body []byte) (string, bool) {
	return captureBody(body, bodyLimitBytes)
}

// CaptureResponseBody 复制并截断响应体，返回存储文本与截断标记。
func CaptureResponseBody(body []byte) (string, bool) {
	return captureBody(body, bodyLimitBytes)
}

func captureBody(body []byte, limit int) (string, bool) {
	if len(body) == 0 {
		return "", false
	}
	truncated := false
	if len(body) > limit {
		body = body[:limit]
		// 回退到最近的 UTF-8 边界，避免存入残缺字符。
		for len(body) > 0 && !validUTF8Suffix(body) {
			body = body[:len(body)-1]
		}
		truncated = true
	}
	return string(body), truncated
}

func validUTF8Suffix(b []byte) bool {
	// 交给标准库判定整体合法性；截断场景最多回退 3 字节即可对齐边界。
	if utf8.Valid(b) {
		return true
	}
	for i := len(b) - 1; i >= 0 && i >= len(b)-3; i-- {
		if utf8.Valid(b[:i]) {
			return false
		}
	}
	return false
}

// CaptureRequestHeaders 把请求头脱敏并序列化为 JSON。
func CaptureRequestHeaders(h map[string][]string) string {
	return captureHeaders(h)
}

// CaptureResponseHeaders 把响应头脱敏并序列化为 JSON。
func CaptureResponseHeaders(h map[string][]string) string {
	return captureHeaders(h)
}

func captureHeaders(h map[string][]string) string {
	if len(h) == 0 {
		return ""
	}
	out := make(map[string][]string, len(h))
	for key, values := range h {
		lower := strings.ToLower(key)
		if _, sensitive := headerRedactedKeys[lower]; sensitive {
			out[key] = []string{"[REDACTED]"}
			continue
		}
		out[key] = values
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	return logredact.TruncateUTF8(string(encoded), headerLimitBytes)
}

func nullText(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

// hashMod 对字符串做稳定取模，用于抽样清理。
func hashMod(s string, mod int) int {
	if mod <= 0 {
		return 0
	}
	var h uint64
	for i := 0; i < len(s); i++ {
		h = h*31 + uint64(s[i])
	}
	return int(h % uint64(mod))
}
