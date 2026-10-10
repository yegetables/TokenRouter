package cline

import (
	"context"
	"strings"

	"github.com/tidwall/gjson"

	"github.com/TokenFlux/TokenRouter/internal/infra/httpclient"
	"github.com/TokenFlux/TokenRouter/internal/upstream/internal/usageclient"
	"github.com/TokenFlux/TokenRouter/internal/upstream/usagecontract"
	"github.com/TokenFlux/TokenRouter/internal/upstream/usageview"
)

// clineAPIPrefix 是 Cline 账户 API 的路径前缀，自带 /api/v1。
const clineAPIPrefix = "/api/v1"

// ClineUsageAdapter 对接 Cline 按量计费（usage-billing）账户的 credit 余额。
// 余额端点需要用户 id，先请求 /users/me 取 id，再请求 /users/{id}/balance；
// balance 字段以美分计，归一化时除以 100 换算成美元。
// ClinePass 订阅的百分比窗口由 cline_pass 适配器负责，两种账户形态各自配置。
type ClineUsageAdapter struct{}

func (*ClineUsageAdapter) Name() string { return usageview.UpstreamUsageAdapterCline }

// clineBaseURL 返回用量查询的站点根地址。提供商 base_url 按聊天端点的填写习惯
// 以 /api 或 /api/v1 结尾，直接拼接会得到 /api/api 重复段，先剥掉末尾的版本段
// 和 /api 段。
func clineBaseURL(base string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(base), "/")
	if httpclient.OpenAIBaseURLHasVersionSuffix(trimmed) {
		if index := strings.LastIndex(trimmed, "/"); index >= 0 {
			trimmed = trimmed[:index]
		}
	}
	return strings.TrimSuffix(trimmed, "/api")
}

// usageEndpoint 从提供商 base_url 拼出账户 API 的完整地址。
func usageEndpoint(base, path string) (string, error) {
	return usageclient.UpstreamUsageRootEndpoint(clineBaseURL(base), clineAPIPrefix+path)
}

// Query 链式请求 /users/me 与 /users/{id}/balance。
func (*ClineUsageAdapter) Query(ctx context.Context, input *usagecontract.Request) (*usageview.UpstreamUsageInfo, error) {
	client := usageclient.New(input)
	meEndpoint, err := usageEndpoint(client.BaseURL, "/users/me")
	if err != nil {
		return nil, usageview.ErrUpstreamUsageConfigInvalid.WithCause(err)
	}
	body, status, err := client.GetURL(ctx, meEndpoint, true)
	if err != nil {
		return nil, err
	}
	if err := usageclient.UpstreamUsageHTTPError(status, true); err != nil {
		return nil, err
	}
	userID := strings.TrimSpace(gjson.GetBytes(body, "data.id").String())
	if userID == "" {
		return nil, usageview.ErrUpstreamUsageInvalidResponse
	}

	balanceEndpoint, err := usageEndpoint(client.BaseURL, "/users/"+userID+"/balance")
	if err != nil {
		return nil, usageview.ErrUpstreamUsageConfigInvalid.WithCause(err)
	}
	body, status, err = client.GetURL(ctx, balanceEndpoint, true)
	if err != nil {
		return nil, err
	}
	if err := usageclient.UpstreamUsageHTTPError(status, true); err != nil {
		return nil, err
	}
	return ParseClineBalance(body)
}

// ParseClineBalance 解析 /users/{id}/balance 的响应，余额美分换算为美元。
func ParseClineBalance(body []byte) (*usageview.UpstreamUsageInfo, error) {
	if success := gjson.GetBytes(body, "success"); success.Exists() && !success.Bool() {
		return nil, usageview.ErrUpstreamUsageInvalidResponse
	}
	cents := gjson.GetBytes(body, "data.balance")
	if !cents.Exists() || cents.Type != gjson.Number {
		return nil, usageview.ErrUpstreamUsageInvalidResponse
	}
	remaining := cents.Float() / 100
	if !usageview.ValidFiniteNumber(remaining) {
		return nil, usageview.ErrUpstreamUsageInvalidResponse
	}
	return &usageview.UpstreamUsageInfo{
		Provider: usageview.UpstreamUsageAdapterCline,
		Mode:     "balance",
		Unit:     "USD",
		Balance:  &usageview.UpstreamUsageAmount{Remaining: &remaining},
	}, nil
}
