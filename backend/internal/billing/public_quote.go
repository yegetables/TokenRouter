package billing

import (
	"context"

	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
)

// PublicQuoteInput 是公开展示所需的价卡与倍率投影，FreeFastApplicable 表示该报价场景允许展示免费 Fast。
type PublicQuoteInput struct {
	PricingInput
	RateMultiplier     float64
	FreeFastApplicable bool
}

// PublicQuote 保留共享查价顺序；free Fast 只调整已支持该档位的展示副本；
// 展示价叠加当前分时与高峰因子，与结算同源。
func (r *PriceResolver) PublicQuote(ctx context.Context, input PublicQuoteInput) ModelDisplayPricing {
	resolved := r.Resolve(ctx, input.PricingInput)
	settings := r.BillingSettings(ctx, input.GroupID)
	if settings.FreeOpenAIFast && input.FreeFastApplicable && pricing.ResolvedHasFastModeDisplayPricing(resolved) {
		cloned := *resolved
		standardMultiplier := 1.0
		pricing.ApplyPricingModifiers(&cloned, &ModelPricingEntry{FastMultiplier: &standardMultiplier})
		resolved = &cloned
	}
	return r.calculator.DisplayPricingForQuote(input.Model, input.RateMultiplier, resolved, settings)
}
