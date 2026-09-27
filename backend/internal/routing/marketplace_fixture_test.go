package routing_test

import (
	"log/slog"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
	billingadapter "github.com/TokenFlux/TokenRouter/internal/billing/provider"
	billingtestkit "github.com/TokenFlux/TokenRouter/internal/billing/testkit"
	"github.com/TokenFlux/TokenRouter/internal/gateway/provider/modelidentity"
	catalogprovider "github.com/TokenFlux/TokenRouter/internal/modelcatalog/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	routingprovider "github.com/TokenFlux/TokenRouter/internal/routing/provider"
)

// newMarketplaceFixture 只绑定原价卡及目录输入，规则由 routing 和 billing 唯一实现。
func newMarketplaceFixture(groups routing.MarketplaceGroups, settings routing.MarketplaceSettings, calculator *billing.Calculator, resolver *billing.PriceResolver) *routing.Marketplace {
	var prices routing.MarketplacePrices
	if calculator != nil {
		prices = marketplaceQuoteFixture{calculator: calculator, resolver: resolver}
	}
	return routing.NewMarketplace(groups, settings, nil, routing.RequestableResolver{}, prices, nil, nil, routing.MarketplaceOptions{Now: time.Now, Warn: slog.Warn, DefaultModels: routingprovider.MarketplaceModelDefs, DisplayNames: routingprovider.MarketplaceDisplayNames})
}

func newMarketplaceCalculator(catalog *catalogprovider.Service, prices map[string]*pricing.ModelPricing) *billing.Calculator {
	return billingtestkit.Calculator(0, catalog, prices)
}

// newMarketplaceCalculatorAt 用固定取时点构造计算器，供分时与高峰展示断言使用。
func newMarketplaceCalculatorAt(at time.Time) *billing.Calculator {
	return billing.NewCalculator(nil, billing.CalculatorOptions{
		Now:          func() time.Time { return at },
		LoadLocation: billingadapter.LoadPricingLocation,
	})
}

func NewModelPricingResolver(pricingConfigs *routing.PricingConfigService, calculator *billing.Calculator) *billing.PriceResolver {
	var source billing.ConfigPrices
	if pricingConfigs != nil {
		source = pricingConfigs
	}
	return billing.NewPriceResolver(source, calculator, modelidentity.Identity, func(model string, err error) {
		slog.Debug("failed to get model pricing from model catalog, using fallback", "model", model, "error", err)
	})
}
