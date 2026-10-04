package selfcapture

import (
	"bytes"
	"io"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/infra/telemetry"

	"github.com/gin-gonic/gin"
)

// Middleware 返回捕获中间件。store 为 nil 或 enabled=false 时返回 inner 本身，零开销。
// 包装目标通常是 OpsErrorLogger：捕获层先还原响应写出，再由外层错误记录逻辑接管。
func Middleware(store *Store, inner gin.HandlerFunc, enabled bool) gin.HandlerFunc {
	if store == nil || !enabled {
		return inner
	}
	if inner == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		if !shouldCapture(c) {
			inner(c)
			return
		}
		capture(store, c)
		inner(c)
	}
}

// shouldCapture 只捕获网关转发请求；WebSocket 升级连接无法回读响应，跳过。
func shouldCapture(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	if strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
		return false
	}
	// 环境开关由装配方写入 context，方便测试覆盖；未设置时默认开启。
	if v, ok := c.Get("self_capture_disabled"); ok {
		if b, ok := v.(bool); ok && b {
			return false
		}
	}
	return true
}

// capture 读取请求快照、包装响应写入器，待响应结束后异步入库。
func capture(store *Store, c *gin.Context) {
	var requestBody []byte
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		// ponytail: 全量读入内存（受全局 BodyLimit 限制在上游已兜底），1MB 截断在落库侧。
		raw, err := io.ReadAll(c.Request.Body)
		if err == nil {
			requestBody = raw
			c.Request.Body = io.NopCloser(bytes.NewReader(raw))
		}
	}

	reqHeaders := CaptureRequestHeaders(map[string][]string(c.Request.Header))
	reqBodyText, reqTruncated := CaptureRequestBody(requestBody)

	writer := &captureWriter{ResponseWriter: c.Writer, buf: &bytes.Buffer{}}
	c.Writer = writer

	c.Next()

	respHeaders := CaptureResponseHeaders(map[string][]string(writer.Header()))
	respBodyText, respTruncated := CaptureResponseBody(writer.buf.Bytes())
	clientRequestID, _ := c.Request.Context().Value(telemetry.ClientRequestID).(string)

	store.Enqueue(Entry{
		ClientRequestID:       clientRequestID,
		RequestHeadersJSON:    reqHeaders,
		ResponseHeadersJSON:   respHeaders,
		RequestBody:           reqBodyText,
		ResponseBody:          respBodyText,
		RequestBodyTruncated:  reqTruncated,
		ResponseBodyTruncated: respTruncated,
		StatusCode:            writer.Status(),
		RequestPath:           c.Request.Method + " " + c.Request.URL.Path,
	})
}

// captureWriter 包装 gin.ResponseWriter，把写出内容复制进缓冲。
type captureWriter struct {
	gin.ResponseWriter
	buf *bytes.Buffer
}

func (w *captureWriter) Write(b []byte) (int, error) {
	if w.buf != nil {
		w.buf.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

func (w *captureWriter) WriteString(s string) (int, error) {
	if w.buf != nil {
		w.buf.WriteString(s)
	}
	return w.ResponseWriter.WriteString(s)
}

// WriteHeader 记录真实状态码，供详情弹窗展示。
func (w *captureWriter) WriteHeader(code int) {
	w.ResponseWriter.WriteHeader(code)
}
