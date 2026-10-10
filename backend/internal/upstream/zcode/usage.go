package zcode

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"

	"github.com/TokenFlux/TokenRouter/internal/upstream/internal/usageclient"
	"github.com/TokenFlux/TokenRouter/internal/upstream/usagecontract"
	"github.com/TokenFlux/TokenRouter/internal/upstream/usageview"
)

// ZCodeUsageAdapter 对接 ZCode 系反代（zcode2api / zcode-api）的 /quota 余额端点。
// 上游返回按模型分桶的 token 配额：大桶是套餐总额度（随新领取的套餐重置），
// 小桶是每日重置余额；不同套餐的桶可能同名（showName 相同），只能靠总量区分。
// 归一化为 limits 视图：每桶一条限额，expiresAt 作为 ResetAt。
type ZCodeUsageAdapter struct{}

func (*ZCodeUsageAdapter) Name() string { return usageview.UpstreamUsageAdapterZCode }

// Query 请求站点根路径的 /quota。提供商 Base URL 沿用 OpenAI 兼容约定可能带
// /v1 版本段，复用 RootEndpoint 先剥版本段再拼接，与转发端点的解释保持一致。
func (*ZCodeUsageAdapter) Query(ctx context.Context, input *usagecontract.Request) (*usageview.UpstreamUsageInfo, error) {
	client := usageclient.New(input)
	endpoint, err := usageclient.UpstreamUsageRootEndpoint(client.BaseURL, "/quota")
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
	return ParseZCodeUsage(body)
}

func ParseZCodeUsage(body []byte) (*usageview.UpstreamUsageInfo, error) {
	balances := gjson.GetBytes(body, "balances")
	if !balances.Exists() || !balances.IsArray() || len(balances.Array()) == 0 {
		return nil, usageview.ErrUpstreamUsageInvalidResponse
	}
	limits := make([]usageview.UpstreamUsageLimit, 0, len(balances.Array()))
	seen := make(map[string]int, len(balances.Array()))
	for _, item := range balances.Array() {
		limit, err := parseZCodeLimit(item, seen)
		if err != nil {
			return nil, err
		}
		limits = append(limits, *limit)
	}
	return &usageview.UpstreamUsageInfo{
		Provider: usageview.UpstreamUsageAdapterZCode,
		Mode:     "limits",
		Unit:     "TOKENS",
		Limits:   limits,
	}, nil
}

func parseZCodeLimit(item gjson.Result, seen map[string]int) (*usageview.UpstreamUsageLimit, error) {
	name := strings.TrimSpace(item.Get("showName").String())
	if name == "" {
		return nil, usageview.ErrUpstreamUsageInvalidResponse
	}
	// unitType 实测取值 "token"（且字段可选缺失）；未来出现新单位再显式接入。
	switch strings.ToUpper(strings.TrimSpace(item.Get("unitType").String())) {
	case "", "TOKEN":
	default:
		return nil, usageview.ErrUpstreamUsageInvalidResponse
	}
	used, usedOK := usageclient.CnParseF64(item.Get("usedUnits").Value())
	if !item.Get("usedUnits").Exists() || !usedOK || !usageview.ValidNonNegativeNumber(used) {
		return nil, usageview.ErrUpstreamUsageInvalidResponse
	}
	total, totalOK := usageclient.CnParseF64(item.Get("totalUnits").Value())
	if !item.Get("totalUnits").Exists() || !totalOK || !usageview.ValidNonNegativeNumber(total) || total <= 0 {
		return nil, usageview.ErrUpstreamUsageInvalidResponse
	}
	remaining, remainingOK := usageclient.CnParseF64(item.Get("remainingUnits").Value())
	if !item.Get("remainingUnits").Exists() || !remainingOK || !usageview.ValidNonNegativeNumber(remaining) {
		return nil, usageview.ErrUpstreamUsageInvalidResponse
	}
	// 上游合约保证 used = total - remaining；交叉验证失败说明响应被改写，
	// 宁可拒绝也不展示自相矛盾的余额。
	if !zcodeCloseEnough(remaining, math.Max(0, total-used)) {
		return nil, usageview.ErrUpstreamUsageInvalidResponse
	}
	// 同名桶（不同套餐的同一模型）追加序号，避免面板展示无法区分。
	seen[name]++
	display := name
	if seen[name] > 1 {
		display = name + " #" + strconv.Itoa(seen[name])
	}
	limit := &usageview.UpstreamUsageLimit{
		Name:      display,
		Used:      &used,
		Limit:     &total,
		Remaining: &remaining,
	}
	if raw := item.Get("expiresAt"); raw.Exists() {
		if seconds := raw.Int(); seconds > 0 {
			value := time.Unix(seconds, 0).UTC()
			limit.ResetAt = &value
		}
	}
	return limit, nil
}

// zcodeCloseEnough 与 usageprovider.CloseEnough 同语义；本包不依赖该包避免拉入
// 全量适配器，容差取相对 1e-5、绝对 1e-6（token 计数下等价于精确匹配）。
func zcodeCloseEnough(left, right float64) bool {
	return math.Abs(left-right) <= math.Max(0.000001, math.Max(math.Abs(left), math.Abs(right))*0.00001)
}
