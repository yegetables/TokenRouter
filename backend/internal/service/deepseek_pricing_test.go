//go:build unit

package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/stretchr/testify/require"
)

func TestDeepseekPeakMultiplierAt(t *testing.T) {
	weekday := func(hour, minute int) time.Time {
		return time.Date(2026, 8, 24, hour, minute, 0, 0, time.UTC)
	}
	tests := []struct {
		name string
		now  time.Time
		want float64
	}{
		{name: "weekday peak start", now: weekday(1, 0), want: 2},
		{name: "weekday peak end", now: weekday(4, 0), want: 1},
		{name: "weekday second peak", now: weekday(6, 30), want: 2},
		{name: "weekday second peak end", now: weekday(10, 0), want: 1},
		{name: "weekday off peak", now: weekday(12, 0), want: 1},
		{name: "beijing weekend", now: time.Date(2026, 8, 22, 2, 0, 0, 0, time.UTC), want: 1},
		{name: "utc weekend boundary", now: time.Date(2026, 8, 22, 16, 30, 0, 0, time.UTC), want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, deepseekPeakMultiplierAt(tt.now))
		})
	}
}

func TestGetModelPricing_DeepseekUsesOfficialRatesForStaleEntries(t *testing.T) {
	pricingService := &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"deepseek-v4-pro":      {InputCostPerToken: 1e-6, OutputCostPerToken: 2e-6, CacheReadInputTokenCost: 3e-8},
		"deepseek-v4-flash":    {InputCostPerToken: 1e-6, OutputCostPerToken: 2e-6, CacheReadInputTokenCost: 3e-8},
		"deepseek-v3-2-251201": {InputCostPerToken: 0, OutputCostPerToken: 0},
	}}
	bs := NewBillingService(&config.Config{}, pricingService)

	tests := []struct {
		model                 string
		input, output, cached float64
	}{
		{model: "deepseek-v4-pro", input: deepseekProOffPeakInputPrice, output: deepseekProOffPeakOutputPrice, cached: deepseekProOffPeakCacheRead},
		{model: "deepseek-v4-flash", input: deepseekFlashOffPeakInputPrice, output: deepseekFlashOffPeakOutputPrice, cached: deepseekFlashOffPeakCacheRead},
		{model: "deepseek-v4-pro-0813", input: deepseekProOffPeakInputPrice, output: deepseekProOffPeakOutputPrice, cached: deepseekProOffPeakCacheRead},
		{model: "deepseek-v3-2-251201", input: deepseekFlashOffPeakInputPrice, output: deepseekFlashOffPeakOutputPrice, cached: deepseekFlashOffPeakCacheRead},
		{model: "deepseek-unknown", input: deepseekFlashOffPeakInputPrice, output: deepseekFlashOffPeakOutputPrice, cached: deepseekFlashOffPeakCacheRead},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			pricing, err := bs.GetModelPricing(tt.model)
			require.NoError(t, err)
			require.InDelta(t, tt.input, pricing.InputPricePerToken, 1e-15)
			require.InDelta(t, tt.output, pricing.OutputPricePerToken, 1e-15)
			require.InDelta(t, tt.cached, pricing.CacheReadPricePerToken, 1e-15)
		})
	}
}

func TestCalculateCostUnified_DeepseekPeakDoesNotOverrideGroupPricing(t *testing.T) {
	bs := NewBillingService(&config.Config{}, nil)
	resolver := NewModelPricingResolver(nil, bs)
	inputPrice, outputPrice := 1e-6, 2e-6
	group := &Group{
		ID:       1,
		Platform: PlatformDeepseek,
		ModelPricing: []ChannelModelPricing{{
			Models:      []string{"deepseek-v4-flash"},
			BillingMode: BillingModeToken,
			InputPrice:  &inputPrice,
			OutputPrice: &outputPrice,
		}},
	}
	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 1000}
	for _, pricingAt := range []time.Time{
		time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 24, 2, 0, 0, 0, time.UTC),
	} {
		cost, err := bs.CalculateCostUnified(CostInput{
			Ctx: context.Background(), Model: "deepseek-v4-flash", Group: group,
			Tokens: tokens, RateMultiplier: 1, PricingAt: pricingAt, Resolver: resolver,
		})
		require.NoError(t, err)
		require.InDelta(t, 1000*inputPrice+500*outputPrice+1000*deepseekFlashOffPeakCacheRead, cost.TotalCost, 1e-12)
	}
}

func TestDeepseekPricingFileContainsOnlyCurrentCatalogEntries(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)
	pricingService := &PricingService{}
	pricingData, err := pricingService.parsePricingData(data)
	require.NoError(t, err)

	for _, removed := range []string{"deepseek-chat", "deepseek-reasoner", "deepseek-v3-2-251201"} {
		_, exists := pricingData[removed]
		require.False(t, exists, "%s 不应继续作为本地价格目录条目", removed)
	}
	for _, model := range []string{"deepseek-v4-flash", "deepseek-v4-flash-vision-exp", "deepseek-v4-pro"} {
		entry, exists := pricingData[model]
		require.True(t, exists, "%s 必须存在于本地价格目录", model)
		require.NotNil(t, entry)
	}
}

// TestDeepseekChannelDualPeriodTimePricingAppliesToCost 验证「运营者在渠道模型定价里
// 配置的双段高峰」在计费中真正生效：北京时间工作日 09:00-12:00、14:00-18:00 为高峰
// （×2），其余工作日时段与周末为空闲（×1）。全程使用固定时刻，不依赖系统当前时间。
func TestDeepseekChannelDualPeriodTimePricingAppliesToCost(t *testing.T) {
	billing := NewBillingService(&config.Config{}, nil)
	resolver := NewModelPricingResolver(nil, billing)

	inputPrice := deepseekFlashOffPeakInputPrice
	outputPrice := deepseekFlashOffPeakOutputPrice
	cacheReadPrice := deepseekFlashOffPeakCacheRead
	channelPricing := &ChannelModelPricing{
		Models:         []string{"deepseek-v4-flash"},
		BillingMode:    BillingModeToken,
		InputPrice:     &inputPrice,
		OutputPrice:    &outputPrice,
		CacheReadPrice: &cacheReadPrice,
		// 与线上渠道配置一致的双段高峰：北京工作日 09:00-12:00、14:00-18:00 ×2。
		TimePricing: &ChannelTimePricing{
			Timezone:     "Asia/Shanghai",
			WeekdaysOnly: true,
			Periods: []ChannelTimePricingPeriod{
				{StartTime: "09:00:00", EndTime: "12:00:00", Multiplier: 2},
				{StartTime: "14:00:00", EndTime: "18:00:00", Multiplier: 2},
			},
		},
	}
	base, err := billing.GetModelPricingWithChannel("deepseek-v4-flash", channelPricing)
	require.NoError(t, err)
	resolved := &ResolvedPricing{
		Mode:           BillingModeToken,
		Source:         PricingSourceChannel,
		BasePricing:    base,
		channelPricing: channelPricing,
	}

	// 北京时间转 UTC；2026-08-24 为周一，2026-08-23 为周日。
	beijingTime := func(day, hour int) time.Time {
		return time.Date(2026, 8, day, hour-8, 0, 0, 0, time.UTC)
	}
	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 1000}
	costAt := func(at time.Time) *CostBreakdown {
		cost, err := billing.CalculateCostUnified(CostInput{
			Ctx:            context.Background(),
			Model:          "deepseek-v4-flash",
			Tokens:         tokens,
			RateMultiplier: 1,
			PricingAt:      at,
			Resolver:       resolver,
			Resolved:       resolved,
		})
		require.NoError(t, err)
		return cost
	}

	offPeak := costAt(beijingTime(24, 20))    // 周一 20:00 空闲
	peakFirst := costAt(beijingTime(24, 10))  // 周一 10:00 第一段高峰
	peakSecond := costAt(beijingTime(24, 15)) // 周一 15:00 第二段高峰
	weekend := costAt(beijingTime(23, 10))    // 周日 10:00 周末 → 空闲

	// 高峰为空闲的 2 倍：两段都生效。
	require.InDelta(t, 2*offPeak.TotalCost, peakFirst.TotalCost, 1e-12)
	require.InDelta(t, 2*offPeak.TotalCost, peakSecond.TotalCost, 1e-12)
	// 仅工作日：周末同样时段不乘峰。
	require.InDelta(t, offPeak.TotalCost, weekend.TotalCost, 1e-12)
}
