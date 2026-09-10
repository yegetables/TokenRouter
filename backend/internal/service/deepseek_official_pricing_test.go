//go:build unit

package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// 夹具来自官方文档定价页真实抓取内容（2026-09-10）：
//   testdata/deepseek_pricing_zh.html → https://api-docs.deepseek.com/zh-cn/quick_start/pricing
//   testdata/deepseek_pricing_en.html → https://api-docs.deepseek.com/quick_start/pricing
func loadDeepSeekFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	require.NotEmpty(t, body)
	return body
}

func TestParseDeepSeekPricingHTML_ChinesePage(t *testing.T) {
	body := loadDeepSeekFixture(t, "deepseek_pricing_zh.html")

	snap, err := parseDeepSeekPricingHTML(body, defaultDeepSeekPricingURL)
	require.NoError(t, err)
	require.Equal(t, "CNY", snap.Currency)
	require.Equal(t, "Asia/Shanghai", snap.Timezone)
	require.True(t, snap.WeekdaysOnly)
	require.NoError(t, validateDeepSeekOfficialPricing(snap))

	// 中文页价格（元/百万 token）：flash 0.02/1/4，pro 0.15/4.5/13.5
	flash := snap.Models["flash"]
	require.InDelta(t, 2e-8, flash.InputCacheHitOffPeak, 1e-12) // ¥0.02/M
	require.InDelta(t, 1e-6, flash.InputOffPeak, 1e-12)         // ¥1/M
	require.InDelta(t, 4e-6, flash.OutputOffPeak, 1e-12)        // ¥4/M
	require.InDelta(t, 2.0, flash.PeakMultiplier, 1e-9)

	pro := snap.Models["pro"]
	require.InDelta(t, 1.5e-7, pro.InputCacheHitOffPeak, 1e-12) // ¥0.15/M
	require.InDelta(t, 4.5e-6, pro.InputOffPeak, 1e-12)         // ¥4.5/M
	require.InDelta(t, 1.35e-5, pro.OutputOffPeak, 1e-12)       // ¥13.5/M
	require.InDelta(t, 2.0, pro.PeakMultiplier, 1e-9)

	// 高峰时段：北京时间周一至周五 9:00-12:00、14:00-18:00
	require.Len(t, snap.PeakWindows, 2)
	require.Equal(t, DeepSeekPeakWindow{Start: "09:00", End: "12:00"}, snap.PeakWindows[0])
	require.Equal(t, DeepSeekPeakWindow{Start: "14:00", End: "18:00"}, snap.PeakWindows[1])
}

func TestParseDeepSeekPricingHTML_EnglishPage(t *testing.T) {
	body := loadDeepSeekFixture(t, "deepseek_pricing_en.html")

	snap, err := parseDeepSeekPricingHTML(body, fallbackDeepSeekPricingURL)
	require.NoError(t, err)
	require.Equal(t, "USD", snap.Currency)
	require.Equal(t, "UTC", snap.Timezone)
	require.True(t, snap.WeekdaysOnly)
	require.NoError(t, validateDeepSeekOfficialPricing(snap))

	// 英文页价格（美元/百万 token）：flash 0.003/0.15/0.6，pro 0.022/0.66/1.98
	flash := snap.Models["flash"]
	require.InDelta(t, 3e-9, flash.InputCacheHitOffPeak, 1e-12)
	require.InDelta(t, 1.5e-7, flash.InputOffPeak, 1e-12)
	require.InDelta(t, 6e-7, flash.OutputOffPeak, 1e-12)

	pro := snap.Models["pro"]
	require.InDelta(t, 2.2e-8, pro.InputCacheHitOffPeak, 1e-12)
	require.InDelta(t, 6.6e-7, pro.InputOffPeak, 1e-12)
	require.InDelta(t, 1.98e-6, pro.OutputOffPeak, 1e-12)

	// 高峰时段：UTC 01:00-04:00、06:00-10:00（工作日）
	require.Len(t, snap.PeakWindows, 2)
	require.Equal(t, DeepSeekPeakWindow{Start: "01:00", End: "04:00"}, snap.PeakWindows[0])
	require.Equal(t, DeepSeekPeakWindow{Start: "06:00", End: "10:00"}, snap.PeakWindows[1])
}

func TestValidateDeepSeekOfficialPricing_RejectsInvalid(t *testing.T) {
	valid := func() *DeepSeekOfficialPricing {
		return &DeepSeekOfficialPricing{
			FetchedAt:    time.Now(),
			SourceURL:    "test",
			Currency:     "CNY",
			Timezone:     "Asia/Shanghai",
			WeekdaysOnly: true,
			PeakWindows:  []DeepSeekPeakWindow{{Start: "09:00", End: "12:00"}},
			Models: map[string]DeepSeekModelRate{
				"flash": {InputOffPeak: 1e-6, InputCacheHitOffPeak: 2e-8, OutputOffPeak: 4e-6, PeakMultiplier: 2},
				"pro":   {InputOffPeak: 4.5e-6, InputCacheHitOffPeak: 1.5e-7, OutputOffPeak: 1.35e-5, PeakMultiplier: 2},
			},
		}
	}
	require.NoError(t, validateDeepSeekOfficialPricing(valid()))

	tests := []struct {
		name   string
		mutate func(*DeepSeekOfficialPricing)
	}{
		{"nil snapshot", func(*DeepSeekOfficialPricing) {}},
		{"missing pro", func(s *DeepSeekOfficialPricing) { delete(s.Models, "pro") }},
		{"zero input price", func(s *DeepSeekOfficialPricing) {
			r := s.Models["flash"]
			r.InputOffPeak = 0
			s.Models["flash"] = r
		}},
		{"cache hit above miss", func(s *DeepSeekOfficialPricing) {
			r := s.Models["flash"]
			r.InputCacheHitOffPeak = r.InputOffPeak * 2
			s.Models["flash"] = r
		}},
		{"no peak windows", func(s *DeepSeekOfficialPricing) { s.PeakWindows = nil }},
		{"bad window", func(s *DeepSeekOfficialPricing) {
			s.PeakWindows = []DeepSeekPeakWindow{{Start: "12:00", End: "09:00"}}
		}},
		{"empty timezone", func(s *DeepSeekOfficialPricing) { s.Timezone = "" }},
		{"empty currency", func(s *DeepSeekOfficialPricing) { s.Currency = "" }},
		{"price out of range", func(s *DeepSeekOfficialPricing) {
			r := s.Models["flash"]
			r.InputOffPeak = 1e-2 // ¥10000/M，远超合理区间
			s.Models["flash"] = r
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "nil snapshot" {
				require.Error(t, validateDeepSeekOfficialPricing(nil))
				return
			}
			snap := valid()
			tt.mutate(snap)
			require.Error(t, validateDeepSeekOfficialPricing(snap))
		})
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, deepSeekPricingFileName)

	body := loadDeepSeekFixture(t, "deepseek_pricing_zh.html")
	snap, err := parseDeepSeekPricingHTML(body, defaultDeepSeekPricingURL)
	require.NoError(t, err)

	require.NoError(t, saveDeepSeekPricingSnapshot(path, snap))

	loaded := loadDeepSeekPricingSnapshot(path)
	require.NotNil(t, loaded)
	require.Equal(t, snap.Currency, loaded.Currency)
	require.Equal(t, snap.Timezone, loaded.Timezone)
	require.Equal(t, snap.PeakWindows, loaded.PeakWindows)
	require.InDelta(t, snap.Models["flash"].InputOffPeak, loaded.Models["flash"].InputOffPeak, 1e-15)
	require.True(t, deepSeekPricingEqual(snap, loaded))
}

// 同步快照存在时，计费覆盖与峰谷倍率都应使用快照（人民币原生价）。
func TestApplyDeepSeekPricing_UsesSyncedSnapshot(t *testing.T) {
	restore := deepSeekOfficial.Load()
	t.Cleanup(func() { deepSeekOfficial.Store(restore) })

	body := loadDeepSeekFixture(t, "deepseek_pricing_zh.html")
	snap, err := parseDeepSeekPricingHTML(body, defaultDeepSeekPricingURL)
	require.NoError(t, err)
	deepSeekOfficial.Store(snap)

	// 谷价覆盖：flash 使用快照值 ¥1/M（而非兜底常量 $0.22/M）
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

// 无快照时回退内置常量与内置峰谷规则，保证既有行为不回归。
func TestApplyDeepSeekPricing_FallbackWhenNoSnapshot(t *testing.T) {
	restore := deepSeekOfficial.Load()
	t.Cleanup(func() { deepSeekOfficial.Store(restore) })
	deepSeekOfficial.Store(nil)

	pricing := &ModelPricing{InputPricePerToken: 1, OutputPricePerToken: 1, CacheReadPricePerToken: 1}
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
	require.Equal(t, "CNY", snap.Currency)
	require.NoError(t, validateDeepSeekOfficialPricing(snap))
	require.InDelta(t, 1e-6, snap.Models["flash"].InputOffPeak, 1e-15)
	require.InDelta(t, 4.5e-6, snap.Models["pro"].InputOffPeak, 1e-15)
}

func TestNormalizeDeepSeekFamily(t *testing.T) {
	for _, model := range []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-0731", "deepseek-chat"} {
		require.Equal(t, "flash", normalizeDeepSeekFamily(model), model)
	}
	for _, model := range []string{"deepseek-v4-pro", "deepseek-v4-pro-0813", "deepseek-pro"} {
		require.Equal(t, "pro", normalizeDeepSeekFamily(model), model)
	}
}
