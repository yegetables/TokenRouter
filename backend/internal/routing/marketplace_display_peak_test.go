package routing_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
	"github.com/TokenFlux/TokenRouter/internal/routing"
)

// 模型广场展示价必须与结算同源：分组倍率 × 分时倍率 × 高峰因子，并下发时段快照。
func TestMarketplaceDisplayPricingAppliesTimeAndPeak(t *testing.T) {
	input, output := 2e-6, 8e-6
	timePricing := &pricing.TimePricingConfig{
		Timezone:     "UTC",
		WeekdaysOnly: true,
		Periods:      []pricing.TimePricingPeriod{{StartTime: "14:00", EndTime: "18:00", Multiplier: 3}},
	}
	settings := pricing.BillingSettings{
		PeakRateEnabled:    true,
		PeakStart:          "14:00",
		PeakEnd:            "18:00",
		PeakRateMultiplier: 2,
	}
	cases := []struct {
		name           string
		at             time.Time
		wantFactor     float64
		wantTimeFactor float64
		wantPeakActive bool
	}{
		// 2026-06-29 是周一：15:00 同时落在分时 3x 与高峰 2x 内，两者独立叠加。
		{"分时与高峰叠加", time.Date(2026, 6, 29, 15, 0, 0, 0, time.UTC), 6, 3, true},
		{"两者都不生效", time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC), 1, 1, false},
		// 2026-06-27 是周六：限工作日分时跳过，高峰窗口是每日区间仍然生效。
		{"周末跳过限工作日分时", time.Date(2026, 6, 27, 15, 0, 0, 0, time.UTC), 2, 1, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := marketplaceWithConfig(newMarketplaceCalculatorAt(c.at), 908, settings, []routing.ModelPricingEntry{{
				Models:      []string{"deepseek-v4"},
				BillingMode: routing.BillingModeToken,
				InputPrice:  &input,
				OutputPrice: &output,
				TimePricing: timePricing,
			}})
			group := &routing.Group{ID: 908, RateMultiplier: 1}

			display := svc.PublicModelPricing(context.Background(), group, "deepseek-v4")

			if !closeEnough(display.InputPricePerToken, input*c.wantFactor) || !closeEnough(display.OutputPricePerToken, output*c.wantFactor) {
				t.Fatalf("展示价 = (%g, %g), 期望 (%g, %g)", display.InputPricePerToken, display.OutputPricePerToken, input*c.wantFactor, output*c.wantFactor)
			}
			if !closeEnough(display.ActiveMultiplier, c.wantFactor) {
				t.Fatalf("总倍率 = %g, 期望 %g", display.ActiveMultiplier, c.wantFactor)
			}
			if display.TimePricing == nil || display.TimePricing.ActiveMultiplier != c.wantTimeFactor {
				t.Fatalf("分时快照 = %#v, 期望 ActiveMultiplier=%v", display.TimePricing, c.wantTimeFactor)
			}
			if len(display.TimePricing.Periods) != 1 || display.TimePricing.Periods[0].Multiplier != 3 || !display.TimePricing.WeekdaysOnly {
				t.Fatalf("分时时段未下发: %#v", display.TimePricing)
			}
			if display.PeakRate == nil || display.PeakRate.Multiplier != 2 || display.PeakRate.Active != c.wantPeakActive {
				t.Fatalf("高峰快照 = %#v, 期望 Active=%v", display.PeakRate, c.wantPeakActive)
			}
		})
	}
}

// 未配置分时与高峰时不下发快照，展示价保持分组倍率。
func TestMarketplaceDisplayPricingWithoutTimeAndPeak(t *testing.T) {
	input := 2e-6
	svc := marketplaceWithConfig(newMarketplaceCalculatorAt(time.Date(2026, 6, 29, 10, 0, 0, 0, time.UTC)), 909, pricing.DefaultBillingSettings(), []routing.ModelPricingEntry{{
		Models:      []string{"deepseek-v4"},
		BillingMode: routing.BillingModeToken,
		InputPrice:  &input,
	}})
	group := &routing.Group{ID: 909, RateMultiplier: 2}

	display := svc.PublicModelPricing(context.Background(), group, "deepseek-v4")

	if !closeEnough(display.InputPricePerToken, input*2) {
		t.Fatalf("展示价 = %g, 期望 %g", display.InputPricePerToken, input*2)
	}
	if display.TimePricing != nil || display.PeakRate != nil {
		t.Fatalf("未配置时不应下发快照: time=%#v peak=%#v", display.TimePricing, display.PeakRate)
	}
	if !closeEnough(display.ActiveMultiplier, 1) {
		t.Fatalf("未配置时总倍率 = %g, 期望 1", display.ActiveMultiplier)
	}
}

func closeEnough(got, want float64) bool {
	return math.Abs(got-want) <= 1e-12
}
