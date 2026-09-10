package service

// /v1/models 上下文元数据透传
//
// 上游 /v1/models 若声明模型上下文窗口（context_length / max_completion_tokens 等），
// 则在网关 /v1/models 响应中一并透传；上游未提供时保持历史响应结构不变
// （不猜测、不从本地价格目录兜底）。
//
// 快照按账号缓存在 account.extra，TTL 内不重复抓取；刷新在后台异步执行，
// 不阻塞 /v1/models 请求路径。仅 OpenAI 兼容的 API Key 账号（含国产供应商）参与，
// 其它平台账号视为上游不提供该信息。

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	upstreamModelContextExtraKey = "upstream_models_meta"
	upstreamModelContextTTL      = 6 * time.Hour
	upstreamModelContextTimeout  = 15 * time.Second
)

// UpstreamModelMetadata 上游 /v1/models 声明的可透传元数据
// （上下文窗口、最大输出与能力标签）。
//
// 零值（0 / nil / 空切片）表示上游未提供该项，调用方必须省略该字段：
// 不得按模型名、内置目录、价格阶梯或客户端能力猜测补齐。
type UpstreamModelMetadata struct {
	ContextLength       int `json:"context_length,omitempty"`
	MaxCompletionTokens int `json:"max_completion_tokens,omitempty"`

	// 能力标签用指针区分「上游未声明」与「上游显式声明 false」。
	SupportsVision    *bool    `json:"supports_vision,omitempty"`
	SupportsTools     *bool    `json:"supports_tools,omitempty"`
	SupportsReasoning *bool    `json:"supports_reasoning,omitempty"`
	SupportsResponses *bool    `json:"supports_responses,omitempty"`
	SupportsAnthropic *bool    `json:"supports_anthropic,omitempty"`
	ResponsesModes    []string `json:"responses_modes,omitempty"`
	// ResponsesCapabilities 原样透传上游子树：结构随上游演进，网关不做字段映射。
	ResponsesCapabilities json.RawMessage `json:"responses_capabilities,omitempty"`
}

func (m UpstreamModelMetadata) isZero() bool {
	return m.ContextLength <= 0 &&
		m.MaxCompletionTokens <= 0 &&
		m.SupportsVision == nil &&
		m.SupportsTools == nil &&
		m.SupportsReasoning == nil &&
		m.SupportsResponses == nil &&
		m.SupportsAnthropic == nil &&
		len(m.ResponsesModes) == 0 &&
		len(m.ResponsesCapabilities) == 0
}

// mergeFrom 用 other 补齐本值缺失的字段（多账号服务同一模型时合并信息）。
// 已声明项（含显式 false）优先于 other，不被覆盖。
func (m UpstreamModelMetadata) mergeFrom(other UpstreamModelMetadata) UpstreamModelMetadata {
	if m.ContextLength <= 0 {
		m.ContextLength = other.ContextLength
	}
	if m.MaxCompletionTokens <= 0 {
		m.MaxCompletionTokens = other.MaxCompletionTokens
	}
	if m.SupportsVision == nil {
		m.SupportsVision = other.SupportsVision
	}
	if m.SupportsTools == nil {
		m.SupportsTools = other.SupportsTools
	}
	if m.SupportsReasoning == nil {
		m.SupportsReasoning = other.SupportsReasoning
	}
	if m.SupportsResponses == nil {
		m.SupportsResponses = other.SupportsResponses
	}
	if m.SupportsAnthropic == nil {
		m.SupportsAnthropic = other.SupportsAnthropic
	}
	if len(m.ResponsesModes) == 0 {
		m.ResponsesModes = other.ResponsesModes
	}
	if len(m.ResponsesCapabilities) == 0 {
		m.ResponsesCapabilities = other.ResponsesCapabilities
	}
	return m
}

type upstreamModelContextSnapshot struct {
	FetchedAt string                           `json:"fetched_at"`
	Source    string                           `json:"source,omitempty"`
	Models    map[string]UpstreamModelMetadata `json:"models"`
}

var (
	upstreamModelContextFlight      sync.Map // accountID -> struct{}（单飞，避免并发重复抓取）
	upstreamModelContextLastAttempt sync.Map // accountID -> time.Time（失败退避，避免上游异常时请求风暴）
)

// upstreamModelContextRetryInterval 上次抓取失败后的最小重试间隔。
const upstreamModelContextRetryInterval = 5 * time.Minute

// UpstreamModelMetadataForAccounts 合并多个账号的上游上下文快照。
// 仅返回上游确实提供过信息的模型；无数据时返回 nil，调用方保持既有响应结构。
func (s *GatewayService) UpstreamModelMetadataForAccounts(accounts []Account) map[string]UpstreamModelMetadata {
	if len(accounts) == 0 {
		return nil
	}
	merged := make(map[string]UpstreamModelMetadata)
	for i := range accounts {
		snap := parseUpstreamModelContextSnapshot(accounts[i].Extra)
		if snap == nil {
			continue
		}
		for modelID, meta := range snap.Models {
			if meta.isZero() {
				continue
			}
			if existing, ok := merged[modelID]; ok {
				merged[modelID] = existing.mergeFrom(meta)
				continue
			}
			merged[modelID] = meta
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

// scheduleUpstreamModelContextRefresh 异步刷新过期快照（TTL + 单飞），不阻塞请求路径。
func (s *GatewayService) scheduleUpstreamModelContextRefresh(accounts []Account) {
	if s == nil || s.accountRepo == nil || s.httpUpstream == nil {
		return
	}
	for i := range accounts {
		account := accounts[i]
		if !supportsUpstreamModelContextSync(&account) {
			continue
		}
		if snap := parseUpstreamModelContextSnapshot(account.Extra); snap != nil &&
			upstreamModelContextSnapshotFresh(snap) {
			continue
		}
		// 上次抓取失败后的短退避：上游异常时不因每个请求都重试而形成请求风暴。
		if last, ok := upstreamModelContextLastAttempt.Load(account.ID); ok {
			if attemptedAt, valid := last.(time.Time); valid &&
				time.Since(attemptedAt) < upstreamModelContextRetryInterval {
				continue
			}
		}
		if _, loaded := upstreamModelContextFlight.LoadOrStore(account.ID, struct{}{}); loaded {
			continue
		}
		acc := account // 复制快照供后台任务使用
		go func() {
			defer upstreamModelContextFlight.Delete(acc.ID)
			upstreamModelContextLastAttempt.Store(acc.ID, time.Now())
			ctx, cancel := context.WithTimeout(context.Background(), upstreamModelContextTimeout)
			defer cancel()
			if err := s.refreshUpstreamModelContext(ctx, &acc); err != nil {
				slog.Debug("upstream_model_context_sync_failed", "account_id", acc.ID, "error", err)
			}
		}()
	}
}

// supportsUpstreamModelContextSync 报告账号能否通过 OpenAI 兼容 /v1/models 提供上下文元数据。
func supportsUpstreamModelContextSync(account *Account) bool {
	if account == nil || account.Type != AccountTypeAPIKey {
		return false
	}
	return account.IsOpenAI() || account.IsCNProvider()
}

// refreshUpstreamModelContext 抓取上游 /v1/models 并把上下文元数据写入账号 Extra。
func (s *GatewayService) refreshUpstreamModelContext(ctx context.Context, account *Account) error {
	if s == nil || account == nil || s.accountRepo == nil || s.httpUpstream == nil {
		return nil
	}
	req, err := s.buildUpstreamModelContextRequest(ctx, account)
	if err != nil {
		return err
	}

	resp, err := s.httpUpstream.Do(req, upstreamModelsProxyURL(account), account.ID, account.Concurrency)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("upstream model list returned HTTP %d", resp.StatusCode)
	}

	bodyLimit := resolveModelsListReadLimit(s.cfg)
	body, err := io.ReadAll(io.LimitReader(resp.Body, bodyLimit+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > bodyLimit {
		return fmt.Errorf("upstream model list response exceeds %d bytes", bodyLimit)
	}

	infos, err := extractUpstreamModelInfos(body)
	if err != nil {
		return err
	}

	// 上游未声明的模型不入快照；若整个上游都没有可透传元数据，仍写入空快照，
	// 以便 TTL 生效、避免每次请求都重复抓取。
	models := make(map[string]UpstreamModelMetadata, len(infos))
	for _, info := range infos {
		meta := UpstreamModelMetadata{
			ContextLength:         info.ContextLength,
			MaxCompletionTokens:   info.MaxCompletionTokens,
			SupportsVision:        info.SupportsVision,
			SupportsTools:         info.SupportsTools,
			SupportsReasoning:     info.SupportsReasoning,
			SupportsResponses:     info.SupportsResponses,
			SupportsAnthropic:     info.SupportsAnthropic,
			ResponsesModes:        info.ResponsesModes,
			ResponsesCapabilities: info.ResponsesCapabilities,
		}
		if meta.isZero() {
			continue
		}
		models[info.ID] = meta
	}

	snap := upstreamModelContextSnapshot{
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		Source:    "account_upstream_models",
		Models:    models,
	}
	payload, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
		upstreamModelContextExtraKey: json.RawMessage(payload),
	})
}

// buildUpstreamModelContextRequest 构造 OpenAI 兼容的 /v1/models 请求。
// 与账号真实转发使用同一协议基准地址与鉴权信息，避免 anthropic 协议账号取错端点。
func (s *GatewayService) buildUpstreamModelContextRequest(ctx context.Context, account *Account) (*http.Request, error) {
	apiKey := strings.TrimSpace(account.GetOpenAIProtocolAPIKey())
	if apiKey == "" {
		return nil, fmt.Errorf("no upstream API key available")
	}
	baseURL := strings.TrimSpace(account.GetOpenAIFormatBaseURL())
	if baseURL == "" {
		return nil, fmt.Errorf("no upstream base url available")
	}
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildOpenAIModelsURL(normalizedBaseURL), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	// 账号级请求头覆写：与真实转发保持一致。
	account.ApplyHeaderOverrides(req.Header)
	return req, nil
}

// parseUpstreamModelContextSnapshot 读取账号 Extra 中的上游上下文快照。
func parseUpstreamModelContextSnapshot(extra map[string]any) *upstreamModelContextSnapshot {
	if len(extra) == 0 {
		return nil
	}
	raw, ok := extra[upstreamModelContextExtraKey]
	if !ok || raw == nil {
		return nil
	}
	body, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var snap upstreamModelContextSnapshot
	if err := json.Unmarshal(body, &snap); err != nil {
		return nil
	}
	if snap.Models == nil {
		return nil
	}
	return &snap
}

func upstreamModelContextSnapshotFresh(snap *upstreamModelContextSnapshot) bool {
	if snap == nil {
		return false
	}
	fetchedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(snap.FetchedAt))
	if err != nil {
		return false
	}
	return time.Since(fetchedAt) < upstreamModelContextTTL
}
