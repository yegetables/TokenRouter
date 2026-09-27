package billing

import (
	"context"
	"fmt"
	"strings"
	"time"

	purepricing "github.com/TokenFlux/TokenRouter/internal/billing/pricing"
)

// ModelPricing 保留旧用量/定价类型入口。
type ModelPricing = purepricing.ModelPricing

// UsageTokens 保留旧用量/定价类型入口。
type UsageTokens = purepricing.UsageTokens

// CostBreakdown 保留旧用量/定价类型入口。
type CostBreakdown = purepricing.CostBreakdown

// applyCostBreakdownMultiplier 委托纯定价实现，旧查询与配置投影保留在适配层。
func applyCostBreakdownMultiplier(cost *CostBreakdown, multiplier float64) {
	purepricing.ApplyCostBreakdownMultiplier(cost, multiplier)
}

// maxReasoningEffortBillingMultiplier 委托纯定价实现，旧查询与配置投影保留在适配层。
func maxReasoningEffortBillingMultiplier(model, effort string, pricing *ModelPricing) float64 {
	return purepricing.MaxReasoningEffortBillingMultiplier(model, effort, pricing)
}

// GetModelPricing 获取模型价格配置
func (s *Calculator) GetModelPricing(model string) (*ModelPricing, error) {
	model = strings.ToLower(model)
	var raw *CatalogModelPricing
	if s.catalog != nil {
		raw = s.catalog.GetModelPricing(model)
	}
	return purepricing.ResolveModelPricing(model, raw)
}

// GetModelPricingWithConfig 保留目录查价失败语义，覆盖计算由纯包完成。
func (s *Calculator) GetModelPricingWithConfig(model string, configPricing *ModelPricingEntry) (*ModelPricing, error) {
	pricing, err := s.GetModelPricing(model)
	if err != nil {
		return nil, err
	}
	return purepricing.ApplyConfigPrice(pricing, configPricing), nil
}

// CostInput 统一计费输入
type CostInput struct {
	Ctx             context.Context
	Model           string
	GroupID         *int64 // 用于共享价格配置定价查找
	Tokens          UsageTokens
	RequestCount    int     // 按次计费时使用
	UsageUnits      float64 // 音频等连续计量单位（分钟/小时/百万字符）
	SizeTier        string  // 按次/图片模式的层级标签（"1K","2K","4K","HD" 等）
	RateMultiplier  float64
	PricingAt       time.Time        // 共享价格配置分时定价使用的计费时刻
	ServiceTier     string           // "priority","flex","" 等
	ReasoningEffort string           // 最终转发的推理档位；max 可触发模型/共享价格配置倍率
	Resolver        *PriceResolver   // 定价解析器
	Resolved        *ResolvedPricing // 可选：预解析的定价结果（避免重复 Resolve 调用）
}

// CalculateCostUnified 统一计费入口，支持三种计费模式。
// 使用 PriceResolver 解析定价，然后根据 BillingMode 分发计算。
func (s *Calculator) CalculateCostUnified(input CostInput) (*CostBreakdown, error) {
	if input.Resolver == nil {
		// 无 Resolver，回退到旧路径
		breakdown, err := s.CalculateCostInternal(input.Model, input.Tokens, input.RateMultiplier, input.ServiceTier, nil)
		if err == nil {
			price, _ := s.GetModelPricing(input.Model)
			applyCostBreakdownMultiplier(breakdown, maxReasoningEffortBillingMultiplier(input.Model, input.ReasoningEffort, price))
		}
		return breakdown, err
	}

	// 优先使用预解析结果，避免重复 Resolve 调用
	resolved := input.Resolved
	if resolved == nil {
		resolved = input.Resolver.Resolve(input.Ctx, PricingInput{
			Model:   input.Model,
			GroupID: input.GroupID,
		})
	}

	// 只有明确的图片计费模式使用尺寸查价；按次价卡保留标签、上下文区间及默认价规则。
	if resolved.Mode == purepricing.BillingModeImage {
		price, err := s.resolvedImageUnitPrice(input.Model, input.SizeTier, resolved)
		if err != nil {
			return nil, err
		}
		copy := *resolved
		copy.RequestTiers = nil
		copy.DefaultPerRequestPrice = price
		copy.DefaultPerRequestPricePresent = true
		resolved = &copy
	}
	return purepricing.CalculateCost(resolved, s.ProjectCostInput(input, resolved))
}

// ComputeTokenBreakdown 委托纯定价实现，旧查询与配置投影保留在适配层。
func (s *Calculator) ComputeTokenBreakdown(
	pricing *ModelPricing, tokens UsageTokens,
	rateMultiplier float64, serviceTier string,
	applyLongCtx bool,
) *CostBreakdown {
	return purepricing.ComputeTokenBreakdown(pricing, tokens, rateMultiplier, serviceTier, applyLongCtx)
}

// CalculateCost 计算使用费用
func (s *Calculator) CalculateCost(model string, tokens UsageTokens, rateMultiplier float64) (*CostBreakdown, error) {
	return s.CalculateCostInternal(model, tokens, rateMultiplier, "", nil)
}

func (s *Calculator) CalculateCostWithServiceTier(model string, tokens UsageTokens, rateMultiplier float64, serviceTier string) (*CostBreakdown, error) {
	return s.CalculateCostInternal(model, tokens, rateMultiplier, serviceTier, nil)
}

func (s *Calculator) CalculateCostInternal(model string, tokens UsageTokens, rateMultiplier float64, serviceTier string, ConfigPricing *ModelPricingEntry) (*CostBreakdown, error) {
	var pricing *ModelPricing
	var err error
	if ConfigPricing != nil {
		pricing, err = s.GetModelPricingWithConfig(model, ConfigPricing)
	} else {
		pricing, err = s.GetModelPricing(model)
	}
	if err != nil {
		return nil, err
	}
	if ConfigPricing == nil {
		pricing = s.applyCatalogTimePricing(pricing, s.options.Now())
	}

	return s.ComputeTokenBreakdown(pricing, tokens, rateMultiplier, serviceTier, true), nil
}

// CalculateCostWithConfig 使用配置中的默认倍率计算费用
func (s *Calculator) CalculateCostWithConfig(model string, tokens UsageTokens) (*CostBreakdown, error) {
	multiplier := s.options.DefaultRateMultiplier
	if multiplier <= 0 {
		multiplier = 1.0
	}
	return s.CalculateCost(model, tokens, multiplier)
}

// GetEstimatedCost 估算费用（用于前端展示）
func (s *Calculator) GetEstimatedCost(model string, estimatedInputTokens, estimatedOutputTokens int) (float64, error) {
	tokens := UsageTokens{
		InputTokens:  estimatedInputTokens,
		OutputTokens: estimatedOutputTokens,
	}

	breakdown, err := s.CalculateCostWithConfig(model, tokens)
	if err != nil {
		return 0, err
	}

	return breakdown.ActualCost, nil
}

// GetCatalogStatus 获取统一模型目录的加载和更新状态。
func (s *Calculator) GetCatalogStatus() map[string]any {
	if s.catalog != nil {
		return s.catalog.GetStatus()
	}
	return map[string]any{
		"model_count":  0,
		"last_updated": "unavailable",
		"local_hash":   "N/A",
	}
}

// ForceUpdatePricing 强制更新价格数据
func (s *Calculator) ForceUpdatePricing() error {
	if s.catalog != nil {
		return s.catalog.ForceUpdate()
	}
	return fmt.Errorf("pricing service not initialized")
}

// ModelDisplayPricing 保留旧用量/定价类型入口。
type ModelDisplayPricing = purepricing.ModelDisplayPricing

// DisplayPricing 使用分组倍率计算模型广场展示价格。
func (s *Calculator) DisplayPricing(model string, rateMultiplier float64) ModelDisplayPricing {
	if rateMultiplier < 0 {
		rateMultiplier = 0
	}

	if mediaPricing, found := s.imageDisplayPricing(model, rateMultiplier); found {
		return mediaPricing
	}

	pricing, err := s.GetModelPricing(model)
	if err != nil || pricing == nil || !hasAnyDisplayTokenPricing(pricing) {
		return unknownDisplayPricing()
	}

	return buildTokenDisplayPricing(s.applyCatalogTimePricing(pricing, s.options.Now()), rateMultiplier)
}

// DisplayPricingWithResolvedMultipliers 优先使用已解析的共享价格配置价格计算展示价格。
func (s *Calculator) DisplayPricingWithResolvedMultipliers(model string, rateMultiplier float64, resolved *ResolvedPricing) ModelDisplayPricing {
	if rateMultiplier < 0 {
		rateMultiplier = 0
	}
	if resolved != nil && resolved.Mode == purepricing.BillingModeImage {
		if quote, ok := s.imageDisplayPricingWithResolved(model, rateMultiplier, resolved); ok {
			return quote
		}
		return unknownDisplayPricing()
	}
	if resolved != nil && resolved.Mode == purepricing.BillingModePerRequest {
		if quote, ok := displayPricingFromResolved(model, rateMultiplier, resolved); ok {
			return quote
		}
		return unknownDisplayPricing()
	}
	if resolved.IsUnpriced() {
		// 未配置价卡时，token 缺价不代表完整型号的独立按张报价也缺失。
		if resolved.ConfigPricing == nil {
			if mediaPricing, found := s.imageDisplayPricing(model, rateMultiplier); found {
				return mediaPricing
			}
		}
		return unknownDisplayPricing()
	}
	if resolved != nil && resolved.Source == PricingSourceCatalog && resolved.BasePricing != nil {
		copy := *resolved
		copy.BasePricing = s.applyCatalogTimePricing(resolved.BasePricing, s.options.Now())
		resolved = &copy
	}
	if pricing, ok := displayPricingFromResolved(model, rateMultiplier, resolved); ok {
		return pricing
	}
	return s.DisplayPricing(model, rateMultiplier)
}

// imageDisplayPricing 只展示已知按张报价，不把聊天模型的图片元数据改成图片计费。
func (s *Calculator) imageDisplayPricing(model string, rateMultiplier float64) (ModelDisplayPricing, bool) {
	return s.imageDisplayPricingWithResolved(model, rateMultiplier, nil)
}

func (s *Calculator) imageDisplayPricingWithResolved(model string, rateMultiplier float64, resolved *ResolvedPricing) (ModelDisplayPricing, bool) {
	raw := s.RawModelPricing(model)
	knownImage := raw != nil && len(raw.ImagePrices) > 0
	if resolved == nil && !knownImage && !hasExplicitImageGenerationPricing(raw) && !looksLikeImageModel(model) && (raw == nil || !raw.TokenPricingAbsent) {
		return ModelDisplayPricing{}, false
	}
	prices := make([]float64, 3)
	found := false
	var sizes []string
	for i, size := range []string{"1K", "2K", "4K"} {
		price, err := purepricing.ResolveImageUnitPrice(resolved, raw, size)
		if err != nil {
			continue
		}
		found = true
		sizes = append(sizes, size)
		prices[i] = price * rateMultiplier
	}
	result := buildImageDisplayPricing(prices[0], prices[1], prices[2])
	result.ImagePriceSizes = sizes
	return result, found
}

// DisplayPricingForQuote 计算模型广场展示价：在分组倍率上叠加当前时刻的分时与高峰因子，
// 与结算使用同一份配置和同一取时口径，使展示价即当前实际扣费单价；并附带时段快照供前端标注。
func (s *Calculator) DisplayPricingForQuote(model string, rateMultiplier float64, resolved *ResolvedPricing, settings purepricing.BillingSettings) ModelDisplayPricing {
	at := s.options.Now()
	config := resolvedTimePricingConfig(resolved)
	var location *time.Location
	if config != nil {
		location, _ = s.options.LoadLocation(config.Timezone)
	}
	// 高峰因子与结算口径一致：只有 token 模式叠加，图片/按次/视频保持基础倍率。
	peak := 1.0
	if resolved.Mode == purepricing.BillingModeToken {
		peak = settings.PeakMultiplierAt(at)
	}
	// 总倍率只在这里算一次：既乘进展示价，也随快照下发，前端不按同一份配置重复相乘。
	factor := purepricing.ResolvedTimeMultiplier(resolved, at, location) * peak
	display := s.DisplayPricingWithResolvedMultipliers(model, rateMultiplier*factor, resolved)
	display.ActiveMultiplier = factor
	display.TimePricing = purepricing.ModelDisplayTimePricingAt(config, at, location)
	if resolved.Mode == purepricing.BillingModeToken {
		display.PeakRate = settings.ModelDisplayPeakRateAt(at)
	}
	return display
}

// resolvedTimePricingConfig 读取生效价卡的分时配置；未配置时返回 nil。
func resolvedTimePricingConfig(resolved *ResolvedPricing) *purepricing.TimePricingConfig {
	if resolved == nil || resolved.ConfigPricing == nil {
		return nil
	}
	return resolved.ConfigPricing.TimePricing
}

// displayPricingFromResolved 委托纯定价实现，旧查询与配置投影保留在适配层。
func displayPricingFromResolved(model string, rateMultiplier float64, resolved *ResolvedPricing) (ModelDisplayPricing, bool) {
	return purepricing.DisplayPricingFromResolved(model, rateMultiplier, resolved)
}

// buildTokenDisplayPricing 委托纯定价实现，旧查询与配置投影保留在适配层。
func buildTokenDisplayPricing(pricing *ModelPricing, rateMultiplier float64) ModelDisplayPricing {
	return purepricing.BuildTokenDisplayPricing(pricing, rateMultiplier)
}

// buildImageDisplayPricing 委托纯定价实现，旧查询与配置投影保留在适配层。
func buildImageDisplayPricing(price1K, price2K, price4K float64) ModelDisplayPricing {
	return purepricing.BuildImageDisplayPricing(price1K, price2K, price4K)
}

// unknownDisplayPricing 委托纯定价实现，旧查询与配置投影保留在适配层。
func unknownDisplayPricing() ModelDisplayPricing { return purepricing.UnknownDisplayPricing() }

// CalculateWebSearchCost 委托纯定价实现，旧查询与配置投影保留在适配层。
func (s *Calculator) CalculateWebSearchCost(callCount int, groupPrice *float64, rateMultiplier float64) *CostBreakdown {
	return purepricing.CalculateWebSearchCost(callCount, operationPrice("web_search", groupPrice, s.operationPrices().WebSearchPricePerCall), rateMultiplier)
}

// CalculateSearchCost 委托纯定价实现，旧查询与配置投影保留在适配层。
func (s *Calculator) CalculateSearchCost(numCalls int, groupPricePer1k *float64, rateMultiplier float64) *CostBreakdown {
	return purepricing.CalculateSearchCost(numCalls, operationPrice("search", groupPricePer1k, s.operationPrices().SearchPricePer1k), rateMultiplier)
}

// audioPriceConfig 保留旧用量/定价类型入口。
type audioPriceConfig = purepricing.AudioPriceConfig

// CalculateAudioCost 委托纯定价实现，旧查询与配置投影保留在适配层。
func (s *Calculator) CalculateAudioCost(mode string, durationOrUnits float64, groupConfig *audioPriceConfig, rateMultiplier float64) *CostBreakdown {
	return purepricing.CalculateAudioCost(mode, durationOrUnits, s.audioPrices(mode, groupConfig), rateMultiplier)
}

// CalculateImageCost 按精确的独立单张价计算费用，缺价时返回错误。
func (s *Calculator) CalculateImageCost(model, imageSize string, imageCount int, rateMultiplier float64) (*CostBreakdown, error) {
	if imageCount <= 0 {
		return purepricing.CalculateImageCost(0, imageCount, rateMultiplier), nil
	}
	imageSize = purepricing.NormalizeImageBillingTierOrDefault(imageSize)
	price, err := s.DefaultImagePrice(model, imageSize)
	if err != nil {
		return nil, err
	}
	return purepricing.CalculateImageCost(price, imageCount, rateMultiplier), nil
}

// CalculateVideoCost 按完整型号的每秒价计算费用，缺价时返回错误。
func (s *Calculator) CalculateVideoCost(model, resolution string, videoCount, durationSeconds int, rateMultiplier float64) (*CostBreakdown, error) {
	if videoCount <= 0 {
		return purepricing.CalculateVideoCost(0, videoCount, durationSeconds, rateMultiplier), nil
	}
	resolution = purepricing.NormalizeVideoBillingResolutionOrDefault(resolution)
	durationSeconds = purepricing.NormalizeVideoBillingDurationSecondsOrDefault(durationSeconds)
	price, err := s.DefaultVideoPrice(model, resolution)
	if err != nil {
		return nil, err
	}
	return purepricing.CalculateVideoCost(price, videoCount, durationSeconds, rateMultiplier), nil
}

// DefaultImagePrice 只读取完整型号的独立按张价格，显式零价保持有效。
func (s *Calculator) DefaultImagePrice(model, imageSize string) (float64, error) {
	var raw *CatalogModelPricing
	if s.catalog != nil {
		raw = s.catalog.GetModelPricing(model)
	}
	price, found := purepricing.DefaultImagePrice(raw, imageSize)
	if !found {
		return 0, fmt.Errorf("image pricing not found for model %s: %w", model, purepricing.ErrModelPricingUnavailable)
	}
	return purepricing.ValidateImageUnitPrice(price)
}

func (s *Calculator) RawModelPricing(model string) *CatalogModelPricing {
	if s == nil || s.catalog == nil {
		return nil
	}
	return s.catalog.GetModelPricing(model)
}

// hasExplicitImageGenerationPricing 委托纯定价实现，旧查询与配置投影保留在适配层。
func hasExplicitImageGenerationPricing(pricing *CatalogModelPricing) bool {
	return purepricing.HasExplicitImageGenerationPricing(pricing)
}

// hasAnyDisplayTokenPricing 委托纯定价实现，旧查询与配置投影保留在适配层。
func hasAnyDisplayTokenPricing(pricing *ModelPricing) bool {
	return purepricing.HasAnyDisplayTokenPricing(pricing)
}

// looksLikeImageModel 委托纯定价实现，旧查询与配置投影保留在适配层。
func looksLikeImageModel(model string) bool { return purepricing.LooksLikeImageModel(model) }

// DefaultVideoPrice 只读取已登记的每秒价，不借用图片单价。
func (s *Calculator) DefaultVideoPrice(model string, resolution string) (float64, error) {
	if price, ok := purepricing.DefaultVideoPrice(s.RawModelPricing(model), resolution); ok {
		return purepricing.ValidateImageUnitPrice(price)
	}
	return 0, fmt.Errorf("video pricing not found for model %s: %w", model, purepricing.ErrModelPricingUnavailable)
}

// PriceCatalog 暴露目录读取及既有维护操作，不向核心暴露文件或网络客户端。
type PriceCatalog interface {
	GetModelPricing(string) *purepricing.CatalogModelPricing
	GetStatus() map[string]any
	ForceUpdate() error
}

// CalculatorOptions 由 app 投影默认倍率、取时点和平台模型身份。
type CalculatorOptions struct {
	DefaultRateMultiplier float64
	Now                   func() time.Time
	LoadLocation          func(string) (*time.Location, error)
}

// Calculator 统一拥有查价及计费编排；价卡算法由 pricing 唯一实现。
type Calculator struct {
	catalog PriceCatalog
	options CalculatorOptions
}

func NewCalculator(catalog PriceCatalog, options CalculatorOptions) *Calculator {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.LoadLocation == nil {
		options.LoadLocation = time.LoadLocation
	}
	return &Calculator{catalog: catalog, options: options}
}

// ProjectCostInput 只投影已解析结果，保持查价懒加载与请求固定的取时点。
func (s *Calculator) ProjectCostInput(input CostInput, resolved *ResolvedPricing) purepricing.CostInput {
	var location *time.Location
	if resolved != nil && resolved.ConfigPricing != nil && resolved.ConfigPricing.TimePricing != nil {
		location, _ = s.options.LoadLocation(resolved.ConfigPricing.TimePricing.Timezone)
	}
	modelAt := input.PricingAt
	if modelAt.IsZero() {
		modelAt = s.options.Now()
	}
	var modelLocation *time.Location
	if resolved != nil && resolved.BasePricing != nil && resolved.BasePricing.TimePricing != nil {
		modelLocation, _ = s.options.LoadLocation(resolved.BasePricing.TimePricing.Timezone)
	}
	return purepricing.CostInput{ModelTimeLocation: modelLocation, Model: input.Model, Tokens: input.Tokens, RequestCount: input.RequestCount, UsageUnits: input.UsageUnits, SizeTier: input.SizeTier, RateMultiplier: input.RateMultiplier, PricingAt: input.PricingAt, ModelPricingAt: modelAt, ServiceTier: input.ServiceTier, ReasoningEffort: input.ReasoningEffort, TimePricingLocation: location}
}

type (
	CatalogModelPricing = purepricing.CatalogModelPricing
	ModelPricingEntry   = purepricing.ModelPricingEntry
)

// GetModelModalities 直接走目录的身份元数据查询，不能继承价格回退。
func (s *Calculator) GetModelModalities(model string) ([]string, []string) {
	if s == nil {
		return nil, nil
	}
	if source, ok := s.catalog.(interface {
		GetModelModalities(string) ([]string, []string)
	}); ok {
		return source.GetModelModalities(model)
	}
	return nil, nil
}
