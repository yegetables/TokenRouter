package clinepass

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/upstream/internal/usageclient"
	"github.com/TokenFlux/TokenRouter/internal/upstream/usagecontract"
	"github.com/TokenFlux/TokenRouter/internal/upstream/usageview"
	"github.com/tidwall/gjson"
)

// ClinePassUsageAdapter 对接 ClinePass 订阅的用量窗口端点。
// 上游只报告已用百分比（percentUsed，数字或字符串都有出现）与可选的重置时间，
// 没有绝对 token/金额数；三个滚动窗口 five_hour/weekly/monthly 各映射为一条
// PERCENT 限额（limit 恒为 100）。缺席的窗口不会出现在结果中。
type ClinePassUsageAdapter struct{}

func (*ClinePassUsageAdapter) Name() string { return usageview.UpstreamUsageAdapterClinePass }

// windowNames 把上游窗口类型映射为面板显示名；未知类型跳过而不是拒绝，
// 上游新增窗口类型不应导致已支持窗口的查询整体失败。
var windowNames = map[string]string{
	"five_hour": "5h",
	"weekly":    "weekly",
	"monthly":   "monthly",
}

// Query 请求 Cline API 站点根的 plan usage-limits 端点。提供商 Base URL 沿用
// OpenAI 兼容约定可能带 /v1 版本段，复用 RootEndpoint 先剥版本段再拼接。
func (*ClinePassUsageAdapter) Query(ctx context.Context, input *usagecontract.Request) (*usageview.UpstreamUsageInfo, error) {
	client := usageclient.New(input)
	endpoint, err := usageclient.UpstreamUsageRootEndpoint(client.BaseURL, "/api/v1/users/me/plan/usage-limits")
	if err != nil {
		return nil, usageview.ErrUpstreamUsageConfigInvalid.WithCause(err)
	}
	body, status, err := client.GetURL(ctx, endpoint, true)
	if err != nil {
		return nil, err
	}
	if err := usageclient.UpstreamUsageHTTPError(status, true); err != nil {
		return nil, err
	}
	return ParseClinePassUsage(body)
}

func ParseClinePassUsage(body []byte) (*usageview.UpstreamUsageInfo, error) {
	limits := gjson.GetBytes(body, "data.limits")
	if !limits.Exists() || !limits.IsArray() || len(limits.Array()) == 0 {
		return nil, usageview.ErrUpstreamUsageInvalidResponse
	}
	seen := make(map[string]struct{}, len(limits.Array()))
	usageLimits := make([]usageview.UpstreamUsageLimit, 0, len(limits.Array()))
	for _, item := range limits.Array() {
		window := strings.TrimSpace(item.Get("type").String())
		name, known := windowNames[window]
		if !known {
			continue
		}
		if _, duplicate := seen[window]; duplicate {
			return nil, usageview.ErrUpstreamUsageInvalidResponse
		}
		seen[window] = struct{}{}
		used, err := parsePercentUsed(item)
		if err != nil {
			return nil, err
		}
		remaining := 100 - used
		limit := usageview.UpstreamUsageLimit{
			Name:      name,
			Used:      &used,
			Limit:     ptr(100.0),
			Remaining: &remaining,
			ResetAt:   parseResetAt(item.Get("resetsAt")),
		}
		usageLimits = append(usageLimits, limit)
	}
	if len(usageLimits) == 0 {
		return nil, usageview.ErrUpstreamUsageInvalidResponse
	}
	return &usageview.UpstreamUsageInfo{
		Provider: usageview.UpstreamUsageAdapterClinePass,
		Mode:     "limits",
		Unit:     "PERCENT",
		Limits:   usageLimits,
	}, nil
}

func parsePercentUsed(item gjson.Result) (float64, error) {
	raw := item.Get("percentUsed")
	// percentUsed 在野外观测到字符串与数字两种类型，都接受。
	if raw.Exists() && raw.Type == gjson.String {
		value, err := strconv.ParseFloat(strings.TrimSpace(raw.String()), 64)
		if err != nil {
			return 0, usageview.ErrUpstreamUsageInvalidResponse
		}
		raw = gjson.Result{Num: value, Type: gjson.Number}
	}
	if !raw.Exists() || raw.Type != gjson.Number {
		return 0, usageview.ErrUpstreamUsageInvalidResponse
	}
	value := raw.Float()
	if !usageview.ValidNonNegativeNumber(value) || value > 100 {
		return 0, usageview.ErrUpstreamUsageInvalidResponse
	}
	return value, nil
}

// parseResetAt 兼容上游观测到的三种格式：epoch 秒、epoch 毫秒与 RFC3339 字符串；
// 解析不出时返回 nil（重置时间缺失可容忍），不因此拒绝整份响应。
func parseResetAt(raw gjson.Result) *time.Time {
	if !raw.Exists() {
		return nil
	}
	if raw.Type == gjson.Number {
		seconds := raw.Int()
		if seconds <= 0 {
			return nil
		}
		if seconds > 1_000_000_000_000 {
			seconds /= 1000
		}
		value := time.Unix(seconds, 0).UTC()
		return &value
	}
	if text := strings.TrimSpace(raw.String()); text != "" {
		if value, err := time.Parse(time.RFC3339, text); err == nil {
			utc := value.UTC()
			return &utc
		}
	}
	return nil
}

func ptr(value float64) *float64 { return &value }
