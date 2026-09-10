//go:build unit

package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// deepSeekSortedCurrencies 返回快照内币种的有序列表（断言用）。
func deepSeekSortedCurrencies(snap *DeepSeekOfficialPricing) []string {
	if snap == nil {
		return nil
	}
	out := make([]string, 0, len(snap.Currencies))
	for currency := range snap.Currencies {
		out = append(out, currency)
	}
	sort.Strings(out)
	return out
}

// 夹具来自官方文档定价页真实抓取内容（2026-09-10）：
//
//	testdata/deepseek_pricing_zh.html → https://api-docs.deepseek.com/zh-cn/quick_start/pricing
//	testdata/deepseek_pricing_en.html → https://api-docs.deepseek.com/quick_start/pricing
func loadDeepSeekFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	require.NotEmpty(t, body)
	return body
}

// useDeepSeekSiteCurrency 固定站点展示币种，并在用例结束后恢复默认（USD）。
func useDeepSeekSiteCurrency(t *testing.T, currency string) {
	t.Helper()
	SetDeepSeekPricingCurrencyResolver(func() string { return currency })
	t.Cleanup(func() {
		SetDeepSeekPricingCurrencyResolver(func() string { return "" })
	})
}

// parseDeepSeekFixtureSnapshot 解析夹具页并返回快照。
func parseDeepSeekFixtureSnapshot(t *testing.T, fixture, src string) *DeepSeekOfficialPricing {
	t.Helper()
	snap, err := parseDeepSeekPricingHTML(loadDeepSeekFixture(t, fixture), src)
	require.NoError(t, err)
	return snap
}

// dualCurrencySnapshot 合并中英文两页，得到 CNY + USD 双币种快照。
func dualCurrencySnapshot(t *testing.T) *DeepSeekOfficialPricing {
	t.Helper()
	snap := parseDeepSeekFixtureSnapshot(t, "deepseek_pricing_zh.html", defaultDeepSeekPricingURL)
	_, err := mergeDeepSeekSourceSnapshot(snap,
		parseDeepSeekFixtureSnapshot(t, "deepseek_pricing_en.html", fallbackDeepSeekPricingURL), false)
	require.NoError(t, err)
	require.NoError(t, validateDeepSeekOfficialPricing(snap))
	return snap
}

func TestParseDeepSeekPricingHTML_ChinesePage(t *testing.T) {
	snap := parseDeepSeekFixtureSnapshot(t, "deepseek_pricing_zh.html", defaultDeepSeekPricingURL)

	require.Equal(t, "CNY", snap.primaryCurrency())
	require.Equal(t, "Asia/Shanghai", snap.Timezone)
	require.True(t, snap.WeekdaysOnly)
	require.NoError(t, validateDeepSeekOfficialPricing(snap))

	rates, ok := snap.ratesFor("CNY")
	require.True(t, ok)
	require.Equal(t, "CNY", rates.Currency)
	require.Equal(t, defaultDeepSeekPricingURL, rates.SourceURL)

	// 中文页价格（元/百万 token）：flash 0.02/1/4，pro 0.15/4.5/13.5
	flash := rates.Models["flash"]
	require.InDelta(t, 2e-8, flash.InputCacheHitOffPeak, 1e-12) // ¥0.02/M
	require.InDelta(t, 1e-6, flash.InputOffPeak, 1e-12)         // ¥1/M
	require.InDelta(t, 4e-6, flash.OutputOffPeak, 1e-12)        // ¥4/M
	require.InDelta(t, 2.0, flash.PeakMultiplier, 1e-9)

	pro := rates.Models["pro"]
	require.InDelta(t, 1.5e-7, pro.InputCacheHitOffPeak, 1e-12) // ¥0.15/M
	require.InDelta(t, 4.5e-6, pro.InputOffPeak, 1e-12)         // ¥4.5/M
	require.InDelta(t, 1.35e-5, pro.OutputOffPeak, 1e-12)       // ¥13.5/M
	require.InDelta(t, 2.0, pro.PeakMultiplier, 1e-9)

	// 模型细节表：上下文长度 1M、输出长度 最大 384K、功能矩阵
	flashFacts, ok := snap.ModelFacts["flash"]
	require.True(t, ok)
	require.Equal(t, 1000000, flashFacts.ContextLength)
	require.Equal(t, 384000, flashFacts.MaxCompletionTokens)
	require.True(t, *flashFacts.SupportsTools)     // Tool Calls 支持
	require.True(t, *flashFacts.SupportsResponses) // Responses API 支持
	require.True(t, *flashFacts.SupportsAnthropic) // Anthropic API 支持
	require.True(t, *flashFacts.SupportsVision)    // 图像理解 支持

	proFacts, ok := snap.ModelFacts["pro"]
	require.True(t, ok)
	require.Equal(t, 1000000, proFacts.ContextLength)
	require.Equal(t, 384000, proFacts.MaxCompletionTokens)
	require.NotNil(t, proFacts.SupportsVision)
	require.False(t, *proFacts.SupportsVision, "pro 的图像理解不支持，必须保留显式 false")
	require.True(t, *proFacts.SupportsTools)

	// 高峰时段：北京时间周一至周五 9:00-12:00、14:00-18:00
	require.Len(t, snap.PeakWindows, 2)
	require.Equal(t, DeepSeekPeakWindow{Start: "09:00", End: "12:00"}, snap.PeakWindows[0])
	require.Equal(t, DeepSeekPeakWindow{Start: "14:00", End: "18:00"}, snap.PeakWindows[1])
}

func TestParseDeepSeekPricingHTML_EnglishPage(t *testing.T) {
	snap := parseDeepSeekFixtureSnapshot(t, "deepseek_pricing_en.html", fallbackDeepSeekPricingURL)

	require.Equal(t, "USD", snap.primaryCurrency())
	require.Equal(t, "UTC", snap.Timezone)
	require.True(t, snap.WeekdaysOnly)
	require.NoError(t, validateDeepSeekOfficialPricing(snap))

	// 英文页价格（美元/百万 token）：flash 0.003/0.15/0.6，pro 0.022/0.66/1.98
	rates, ok := snap.ratesFor("USD")
	require.True(t, ok)
	require.Equal(t, "USD", rates.Currency)

	flash := rates.Models["flash"]
	require.InDelta(t, 3e-9, flash.InputCacheHitOffPeak, 1e-12)
	require.InDelta(t, 1.5e-7, flash.InputOffPeak, 1e-12)
	require.InDelta(t, 6e-7, flash.OutputOffPeak, 1e-12)

	pro := rates.Models["pro"]
	require.InDelta(t, 2.2e-8, pro.InputCacheHitOffPeak, 1e-12)
	require.InDelta(t, 6.6e-7, pro.InputOffPeak, 1e-12)
	require.InDelta(t, 1.98e-6, pro.OutputOffPeak, 1e-12)

	// 英文页同样解析模型细节表（CONTEXT LENGTH / MAX OUTPUT / Vision）
	enFlash := snap.ModelFacts["flash"]
	require.Equal(t, 1000000, enFlash.ContextLength)
	require.Equal(t, 384000, enFlash.MaxCompletionTokens)
	require.True(t, *enFlash.SupportsTools)
	require.True(t, *enFlash.SupportsVision) // ✓
	enPro := snap.ModelFacts["pro"]
	require.NotNil(t, enPro.SupportsVision)
	require.False(t, *enPro.SupportsVision, "Not supported → 显式 false")

	// 高峰时段：UTC 01:00-04:00、06:00-10:00（工作日）
	require.Len(t, snap.PeakWindows, 2)
	require.Equal(t, DeepSeekPeakWindow{Start: "01:00", End: "04:00"}, snap.PeakWindows[0])
	require.Equal(t, DeepSeekPeakWindow{Start: "06:00", End: "10:00"}, snap.PeakWindows[1])
}

// 模型细节表解析的边界：措辞不明确按未声明处理；colspan 单值行两族共用。
func TestParseDeepSeekModelFacts_EdgeCases(t *testing.T) {
	cells := []string{
		"上下文长度", "128K", // 单值行（colspan）→ 两族共用
		"输出长度", "最大 8K",
		"Tool Calls", "支持", "仅非思考模式支持", // 第二族措辞不明确 → 未声明
		"Vision", "✓", "✗",
	}
	facts := parseDeepSeekModelFacts(cells)

	require.Equal(t, 128000, facts["flash"].ContextLength)
	require.Equal(t, 128000, facts["pro"].ContextLength, "colspan 单值行两族共用")
	require.Equal(t, 8000, facts["flash"].MaxCompletionTokens)
	require.Equal(t, 8000, facts["pro"].MaxCompletionTokens)

	require.NotNil(t, facts["flash"].SupportsTools)
	require.True(t, *facts["flash"].SupportsTools)
	require.Nil(t, facts["pro"].SupportsTools, "「仅非思考模式支持」不是明确的支持/不支持，按未声明处理")

	require.True(t, *facts["flash"].SupportsVision)
	require.False(t, *facts["pro"].SupportsVision)

	// 页面完全没有这些行时不产生任何事实
	require.Empty(t, parseDeepSeekModelFacts([]string{"模型", "deepseek-flash"}))
}

func TestParseDeepSeekTokenQuantity(t *testing.T) {
	cases := map[string]int{
		"1M": 1000000, "128K": 128000, "最大 384K": 384000,
		"MAXIMUM: 384K": 384000, "1,000,000": 1000000, "200000": 200000,
	}
	for raw, want := range cases {
		got, ok := parseDeepSeekTokenQuantity(raw)
		require.True(t, ok, raw)
		require.Equal(t, want, got, raw)
	}
	for _, raw := range []string{"", "-", "—", "支持", "Max", "不支持"} {
		_, ok := parseDeepSeekTokenQuantity(raw)
		require.False(t, ok, raw)
	}
}

func TestParseDeepSeekSupportFlag(t *testing.T) {
	for _, yes := range []string{"支持", "✓", "Yes", "Supported"} {
		flag, ok := parseDeepSeekSupportFlag(yes)
		require.True(t, ok, yes)
		require.True(t, *flag, yes)
	}
	for _, no := range []string{"不支持", "✗", "Not supported", "No"} {
		flag, ok := parseDeepSeekSupportFlag(no)
		require.True(t, ok, no)
		require.False(t, *flag, no)
	}
	for _, unknown := range []string{"", "-", "仅非思考模式支持", "Non-thinking mode only"} {
		_, ok := parseDeepSeekSupportFlag(unknown)
		require.False(t, ok, unknown)
	}
}
func TestValidateDeepSeekOfficialPricing_RejectsInvalid(t *testing.T) {
	valid := func() *DeepSeekOfficialPricing {
		return &DeepSeekOfficialPricing{
			FetchedAt:    time.Now(),
			Timezone:     "Asia/Shanghai",
			WeekdaysOnly: true,
			PeakWindows:  []DeepSeekPeakWindow{{Start: "09:00", End: "12:00"}},
			Currencies: map[string]DeepSeekCurrencyRates{
				"CNY": {
					Currency:  "CNY",
					SourceURL: "test",
					FetchedAt: time.Now(),
					Models: map[string]DeepSeekModelRate{
						"flash": {InputOffPeak: 1e-6, InputCacheHitOffPeak: 2e-8, OutputOffPeak: 4e-6, PeakMultiplier: 2},
						"pro":   {InputOffPeak: 4.5e-6, InputCacheHitOffPeak: 1.5e-7, OutputOffPeak: 1.35e-5, PeakMultiplier: 2},
					},
				},
			},
		}
	}
	require.NoError(t, validateDeepSeekOfficialPricing(valid()))

	tests := []struct {
		name   string
		mutate func(*DeepSeekOfficialPricing)
	}{
		{"missing pro", func(s *DeepSeekOfficialPricing) {
			rates := s.Currencies["CNY"]
			delete(rates.Models, "pro")
			s.Currencies["CNY"] = rates
		}},
		{"zero input price", func(s *DeepSeekOfficialPricing) {
			rates := s.Currencies["CNY"]
			r := rates.Models["flash"]
			r.InputOffPeak = 0
			rates.Models["flash"] = r
			s.Currencies["CNY"] = rates
		}},
		{"cache hit above miss", func(s *DeepSeekOfficialPricing) {
			rates := s.Currencies["CNY"]
			r := rates.Models["flash"]
			r.InputCacheHitOffPeak = r.InputOffPeak * 2
			rates.Models["flash"] = r
			s.Currencies["CNY"] = rates
		}},
		{"currency key mismatch", func(s *DeepSeekOfficialPricing) {
			rates := s.Currencies["CNY"]
			rates.Currency = "USD"
			s.Currencies["CNY"] = rates
		}},
		{"no peak windows", func(s *DeepSeekOfficialPricing) { s.PeakWindows = nil }},
		{"bad window", func(s *DeepSeekOfficialPricing) {
			s.PeakWindows = []DeepSeekPeakWindow{{Start: "12:00", End: "09:00"}}
		}},
		{"empty timezone", func(s *DeepSeekOfficialPricing) { s.Timezone = "" }},
		{"no currencies", func(s *DeepSeekOfficialPricing) { s.Currencies = nil }},
		{"price out of range", func(s *DeepSeekOfficialPricing) {
			rates := s.Currencies["CNY"]
			r := rates.Models["flash"]
			r.InputOffPeak = 1e-2 // ¥10000/M，远超合理区间
			rates.Models["flash"] = r
			s.Currencies["CNY"] = rates
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap := valid()
			tt.mutate(snap)
			require.Error(t, validateDeepSeekOfficialPricing(snap))
		})
	}
	require.Error(t, validateDeepSeekOfficialPricing(nil))
}

func TestSnapshotRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, deepSeekPricingFileName)

	snap := dualCurrencySnapshot(t)
	require.NoError(t, saveDeepSeekPricingSnapshot(path, snap))

	loaded := loadDeepSeekPricingSnapshot(path)
	require.NotNil(t, loaded)
	require.Equal(t, []string{"CNY", "USD"}, deepSeekSortedCurrencies(loaded))
	require.Equal(t, snap.Timezone, loaded.Timezone)
	require.Equal(t, snap.PeakWindows, loaded.PeakWindows)
	require.True(t, deepSeekPricingEqual(snap, loaded))

	// 双币种各自的数字都保留，互不污染
	cny, ok := loaded.ratesFor("CNY")
	require.True(t, ok)
	require.InDelta(t, 1e-6, cny.Models["flash"].InputOffPeak, 1e-15)
	usd, ok := loaded.ratesFor("USD")
	require.True(t, ok)
	require.InDelta(t, 1.5e-7, usd.Models["flash"].InputOffPeak, 1e-15)
}

// v1 单币种快照文件（旧版本落盘）应能迁移为多币种结构后载入。
func TestLoadDeepSeekPricingSnapshot_MigratesLegacyV1(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, deepSeekPricingFileName)

	legacy := map[string]any{
		"fetched_at":    time.Now().Format(time.RFC3339Nano),
		"source_url":    defaultDeepSeekPricingURL,
		"currency":      "CNY",
		"timezone":      "Asia/Shanghai",
		"weekdays_only": true,
		"peak_windows":  []map[string]string{{"start": "09:00", "end": "12:00"}},
		"models": map[string]any{
			"flash": map[string]any{"input_off_peak": 1e-6, "input_cache_hit_off_peak": 2e-8, "output_off_peak": 4e-6, "peak_multiplier": 2},
			"pro":   map[string]any{"input_off_peak": 4.5e-6, "input_cache_hit_off_peak": 1.5e-7, "output_off_peak": 1.35e-5, "peak_multiplier": 2},
		},
	}
	body, err := json.MarshalIndent(legacy, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, body, 0o644))

	loaded := loadDeepSeekPricingSnapshot(path)
	require.NotNil(t, loaded)
	require.Equal(t, []string{"CNY"}, deepSeekSortedCurrencies(loaded))
	cny, ok := loaded.ratesFor("CNY")
	require.True(t, ok)
	require.InDelta(t, 1e-6, cny.Models["flash"].InputOffPeak, 1e-15)
	// 兼容字段已清空，避免与新结构重复
	require.Empty(t, loaded.Currency)
	require.Empty(t, loaded.Models)
}

// 同步快照存在时，计费覆盖与峰谷倍率都应使用「站点币种对应」的那一套数字。
func TestApplyDeepSeekPricing_UsesSyncedSnapshot(t *testing.T) {
	restore := deepSeekOfficial.Load()
	t.Cleanup(func() { deepSeekOfficial.Store(restore) })
	useDeepSeekSiteCurrency(t, "CNY")

	snap := parseDeepSeekFixtureSnapshot(t, "deepseek_pricing_zh.html", defaultDeepSeekPricingURL)
	deepSeekOfficial.Store(snap)

	// 谷价覆盖：flash 使用快照值 ¥1/M（而非兜底常量 ¥0.22/M）
	pricing := &ModelPricing{InputPricePerToken: 1, OutputPricePerToken: 1, CacheReadPricePerToken: 1}
	overridden := applyDeepSeekOfficialPricing("deepseek-flash", pricing)
	require.InDelta(t, 1e-6, overridden.InputPricePerToken, 1e-15)
	require.InDelta(t, 4e-6, overridden.OutputPricePerToken, 1e-15)
	require.InDelta(t, 2e-8, overridden.CacheReadPricePerToken, 1e-15)

	// 版本化/别名命名同样命中快照
	aliased := applyDeepSeekOfficialPricing("deepseek-v4-flash-0731", pricing)
	require.InDelta(t, 1e-6, aliased.InputPricePerToken, 1e-15)
	proAliased := applyDeepSeekOfficialPricing("deepseek-v4-pro-0813", pricing)
	require.InDelta(t, 4.5e-6, proAliased.InputPricePerToken, 1e-15)

	// 峰谷：北京时间周一 10:00 → 高峰 ×2
	peakBase := &ModelPricing{InputPricePerToken: 1e-6, OutputPricePerToken: 4e-6, CacheReadPricePerToken: 2e-8}
	peak := applyDeepSeekPeakPricing("deepseek-flash", peakBase, time.Date(2026, 9, 14, 10, 0, 0, 0, time.FixedZone("CST", 8*3600)))
	require.InDelta(t, 2e-6, peak.InputPricePerToken, 1e-15) // 1e-6 × 2

	// 峰谷：北京时间周一 13:00（午休）→ 空闲，不加倍
	idle := applyDeepSeekPeakPricing("deepseek-flash", peakBase, time.Date(2026, 9, 14, 13, 0, 0, 0, time.FixedZone("CST", 8*3600)))
	require.InDelta(t, 1e-6, idle.InputPricePerToken, 1e-15)

	// 峰谷：周日 10:00 → 周末全天空闲
	weekend := applyDeepSeekPeakPricing("deepseek-flash", peakBase, time.Date(2026, 9, 13, 10, 0, 0, 0, time.FixedZone("CST", 8*3600)))
	require.InDelta(t, 1e-6, weekend.InputPricePerToken, 1e-15)
}

// 站点币种决定取哪一套官方数字：同一份双币种快照下，USD 站点用美元页、CNY 站点用人民币页。
func TestApplyDeepSeekPricing_SelectsCurrencyBySiteSetting(t *testing.T) {
	restore := deepSeekOfficial.Load()
	t.Cleanup(func() { deepSeekOfficial.Store(restore) })

	deepSeekOfficial.Store(dualCurrencySnapshot(t))

	pricing := &ModelPricing{InputPricePerToken: 1, OutputPricePerToken: 1, CacheReadPricePerToken: 1}

	useDeepSeekSiteCurrency(t, "CNY")
	cny := applyDeepSeekOfficialPricing("deepseek-flash", pricing)
	require.InDelta(t, 1e-6, cny.InputPricePerToken, 1e-15) // ¥1/M
	require.InDelta(t, 4e-6, cny.OutputPricePerToken, 1e-15)

	useDeepSeekSiteCurrency(t, "USD")
	usd := applyDeepSeekOfficialPricing("deepseek-flash", pricing)
	require.InDelta(t, 1.5e-7, usd.InputPricePerToken, 1e-15) // $0.15/M
	require.InDelta(t, 6e-7, usd.OutputPricePerToken, 1e-15)

	usdPro := applyDeepSeekOfficialPricing("deepseek-v4-pro", pricing)
	require.InDelta(t, 6.6e-7, usdPro.InputPricePerToken, 1e-15) // $0.66/M
}

// 站点为 USD 但只有人民币快照时：既不换算，也不套用人民币常量，保持上游价卡数字。
func TestApplyDeepSeekPricing_USDSiteWithoutUSDRatesKeepsUpstreamCard(t *testing.T) {
	restore := deepSeekOfficial.Load()
	t.Cleanup(func() { deepSeekOfficial.Store(restore) })

	deepSeekOfficial.Store(parseDeepSeekFixtureSnapshot(t, "deepseek_pricing_zh.html", defaultDeepSeekPricingURL))
	useDeepSeekSiteCurrency(t, "USD")

	pricing := &ModelPricing{InputPricePerToken: 0.15e-6, OutputPricePerToken: 0.6e-6, CacheReadPricePerToken: 0.003e-6}
	got := applyDeepSeekOfficialPricing("deepseek-flash", pricing)
	require.InDelta(t, 0.15e-6, got.InputPricePerToken, 1e-18) // 未被人民币数字覆盖
	require.InDelta(t, 0.6e-6, got.OutputPricePerToken, 1e-18)

	// 同一快照在 CNY 站点则正常接管
	useDeepSeekSiteCurrency(t, "CNY")
	gotCNY := applyDeepSeekOfficialPricing("deepseek-flash", pricing)
	require.InDelta(t, 1e-6, gotCNY.InputPricePerToken, 1e-15)
}

// 无快照时：CNY 站点回退内置常量与内置峰谷规则；USD 站点不动上游价卡。
func TestApplyDeepSeekPricing_FallbackWhenNoSnapshot(t *testing.T) {
	restore := deepSeekOfficial.Load()
	t.Cleanup(func() { deepSeekOfficial.Store(restore) })
	deepSeekOfficial.Store(nil)

	pricing := &ModelPricing{InputPricePerToken: 1, OutputPricePerToken: 1, CacheReadPricePerToken: 1}

	useDeepSeekSiteCurrency(t, "USD")
	require.InDelta(t, 1.0, applyDeepSeekOfficialPricing("deepseek-flash", pricing).InputPricePerToken, 1e-15)

	useDeepSeekSiteCurrency(t, "CNY")
	flash := applyDeepSeekOfficialPricing("deepseek-flash", pricing)
	require.InDelta(t, deepseekFlashOffPeakInputPrice, flash.InputPricePerToken, 1e-18)
	pro := applyDeepSeekOfficialPricing("deepseek-v4-pro", pricing)
	require.InDelta(t, deepseekProOffPeakInputPrice, pro.InputPricePerToken, 1e-18)

	// 内置规则：工作日 UTC 02:00 → 高峰 ×2
	peakBase := &ModelPricing{InputPricePerToken: 1e-6, OutputPricePerToken: 4e-6, CacheReadPricePerToken: 2e-8}
	peak := applyDeepSeekPeakPricing("deepseek-flash", peakBase,
		time.Date(2026, 9, 14, 2, 0, 0, 0, time.UTC))
	require.InDelta(t, 2e-6, peak.InputPricePerToken, 1e-15)
	// 内置规则：工作日 UTC 05:00（非峰段）→ 不加倍
	idle := applyDeepSeekPeakPricing("deepseek-flash", peakBase,
		time.Date(2026, 9, 14, 5, 0, 0, 0, time.UTC))
	require.InDelta(t, 1e-6, idle.InputPricePerToken, 1e-15)

	// 非 DeepSeek 模型不受影响
	other := applyDeepSeekOfficialPricing("gpt-5.5", pricing)
	require.InDelta(t, 1.0, other.InputPricePerToken, 1e-15)
}

func TestParseDeepSeekPricingJSONPayload(t *testing.T) {
	body := []byte(`{
	  "currency": "CNY",
	  "timezone": "Asia/Shanghai",
	  "weekdays_only": true,
	  "peak_windows": [{"start": "09:00", "end": "12:00"}, {"start": "14:00", "end": "18:00"}],
	  "models": {
	    "deepseek-flash": {"input_off_peak": 1e-6, "input_cache_hit_off_peak": 2e-8, "output_off_peak": 4e-6, "peak_multiplier": 2},
	    "deepseek-v4-pro": {"input_off_peak": 4.5e-6, "input_cache_hit_off_peak": 1.5e-7, "output_off_peak": 1.35e-5, "peak_multiplier": 2}
	  }
	}`)
	snap, err := parseDeepSeekPricingJSON(body, "https://example.com/pricing.json")
	require.NoError(t, err)
	require.Equal(t, "CNY", snap.primaryCurrency())
	require.NoError(t, validateDeepSeekOfficialPricing(snap))

	rates, ok := snap.ratesFor("CNY")
	require.True(t, ok)
	require.InDelta(t, 1e-6, rates.Models["flash"].InputOffPeak, 1e-15)
	require.InDelta(t, 4.5e-6, rates.Models["pro"].InputOffPeak, 1e-15)
}

// 单源失败或校验不通过时，只保留其它币种的上一份好数据，窗口不被覆盖。
func TestMergeDeepSeekSourceSnapshot_KeepsOtherCurrencyOnFailure(t *testing.T) {
	dst := newDeepSeekPricingSnapshot()

	cnySource := parseDeepSeekFixtureSnapshot(t, "deepseek_pricing_zh.html", defaultDeepSeekPricingURL)
	usdSource := parseDeepSeekFixtureSnapshot(t, "deepseek_pricing_en.html", fallbackDeepSeekPricingURL)

	// 第一轮：只抓到人民币页 → CNY + 北京时间峰谷窗口
	currency, err := mergeDeepSeekSourceSnapshot(dst, cnySource, true)
	require.NoError(t, err)
	require.Equal(t, "CNY", currency)
	require.Equal(t, "Asia/Shanghai", dst.Timezone)

	// 第二轮：只抓到美元页 → USD 并入，峰谷窗口保持不覆盖（时间表示等价，避免来回翻转）
	currency, err = mergeDeepSeekSourceSnapshot(dst, usdSource, false)
	require.NoError(t, err)
	require.Equal(t, "USD", currency)
	require.Equal(t, []string{"CNY", "USD"}, deepSeekSortedCurrencies(dst))
	require.Equal(t, "Asia/Shanghai", dst.Timezone)
	require.NoError(t, validateDeepSeekOfficialPricing(dst))

	// 坏源：币种键与字段不一致 → 报错且不修改目标
	bad := newDeepSeekPricingSnapshot()
	bad.Currencies["USD"] = DeepSeekCurrencyRates{
		Currency: "CNY",
		Models:   usdSource.Currencies["USD"].Models,
	}
	_, err = mergeDeepSeekSourceSnapshot(dst, bad, true)
	require.Error(t, err)
	require.Equal(t, []string{"CNY", "USD"}, deepSeekSortedCurrencies(dst))
	require.Equal(t, "Asia/Shanghai", dst.Timezone)

	// 空源：无币种 → 报错
	_, err = mergeDeepSeekSourceSnapshot(dst, newDeepSeekPricingSnapshot(), true)
	require.Error(t, err)
}

func TestDeepSeekSiteCurrency_ResolverAndCache(t *testing.T) {
	SetDeepSeekPricingCurrencyResolver(func() string { return "  cny " })
	t.Cleanup(func() { SetDeepSeekPricingCurrencyResolver(func() string { return "" }) })

	require.Equal(t, "CNY", deepSeekSiteCurrency())

	// 缓存命中：切换 resolver 但不失效缓存时仍返回旧值，失效后立即收敛
	deepSeekCurrencyResolver.Store(func() string { return "usd" })
	require.Equal(t, "CNY", deepSeekSiteCurrency())
	InvalidateDeepSeekPricingCurrencyCache()
	require.Equal(t, "USD", deepSeekSiteCurrency())

	// 读取器返回空 → 回退默认 USD
	SetDeepSeekPricingCurrencyResolver(func() string { return "" })
	require.Equal(t, "USD", deepSeekSiteCurrency())
}

func TestNormalizeDeepSeekFamily(t *testing.T) {
	for _, model := range []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-0731", "deepseek-chat"} {
		require.Equal(t, "flash", normalizeDeepSeekFamily(model), model)
	}
	for _, model := range []string{"deepseek-v4-pro", "deepseek-v4-pro-0813", "deepseek-pro"} {
		require.Equal(t, "pro", normalizeDeepSeekFamily(model), model)
	}
}
