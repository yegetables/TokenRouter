package dto

import (
	"time"

	"github.com/TokenFlux/TokenRouter/internal/service"
)

type ModelMarketplaceStats struct {
	TodayTokens int64 `json:"today_tokens"`
	TotalTokens int64 `json:"total_tokens"`
	TotalUsers  int64 `json:"total_users"`
}

type ModelMarketplacePricing struct {
	PricingMode                   string                            `json:"pricing_mode"`
	PriceStatus                   string                            `json:"price_status"`
	InputPricePerToken            float64                           `json:"input_price_per_token,omitempty"`
	ImageInputPricePerToken       float64                           `json:"image_input_price_per_token,omitempty"`
	OutputPricePerToken           float64                           `json:"output_price_per_token,omitempty"`
	CacheWritePricePerToken       float64                           `json:"cache_write_price_per_token,omitempty"`
	CacheWrite1hPricePerToken     float64                           `json:"cache_write_1h_price_per_token,omitempty"`
	CacheReadPricePerToken        float64                           `json:"cache_read_price_per_token,omitempty"`
	ImageOutputPricePerToken      float64                           `json:"image_output_price_per_token,omitempty"`
	FastInputPricePerToken        float64                           `json:"fast_input_price_per_token,omitempty"`
	FastImageInputPricePerToken   float64                           `json:"fast_image_input_price_per_token,omitempty"`
	FastOutputPricePerToken       float64                           `json:"fast_output_price_per_token,omitempty"`
	FastCacheWritePricePerToken   float64                           `json:"fast_cache_write_price_per_token,omitempty"`
	FastCacheWrite1hPricePerToken float64                           `json:"fast_cache_write_1h_price_per_token,omitempty"`
	FastCacheReadPricePerToken    float64                           `json:"fast_cache_read_price_per_token,omitempty"`
	FastImageOutputPricePerToken  float64                           `json:"fast_image_output_price_per_token,omitempty"`
	ContextIntervals              []ModelMarketplacePricingInterval `json:"context_intervals,omitempty"`
	ImagePrice1K                  float64                           `json:"image_price_1k,omitempty"`
	ImagePrice2K                  float64                           `json:"image_price_2k,omitempty"`
	ImagePrice4K                  float64                           `json:"image_price_4k,omitempty"`
	TimePricing                   *ModelMarketplaceTimePricing      `json:"time_pricing,omitempty"`
	GroupPeak                     *ModelMarketplaceGroupPeak        `json:"group_peak,omitempty"`
}

// ModelMarketplaceGroupPeak 是分组高峰倍率展示信息（用户加价，窗口用全局系统时区）。
type ModelMarketplaceGroupPeak struct {
	StartTime  string  `json:"start_time"`
	EndTime    string  `json:"end_time"`
	Multiplier float64 `json:"multiplier"`
	Active     bool    `json:"active"`
}

// ModelMarketplaceTimePricing 是渠道分时倍率配置；展示价已按当前时刻生效。
type ModelMarketplaceTimePricing struct {
	Timezone         string                       `json:"timezone"`
	WeekdaysOnly     bool                         `json:"weekdays_only"`
	Periods          []ModelMarketplaceTimePeriod `json:"periods"`
	ActiveMultiplier float64                      `json:"active_multiplier"`
}

// ModelMarketplaceTimePeriod 是单个分时时段（本地时间）。
type ModelMarketplaceTimePeriod struct {
	StartTime  string  `json:"start_time"`
	EndTime    string  `json:"end_time"`
	Multiplier float64 `json:"multiplier"`
}

// ModelMarketplacePricingInterval 是前端模型广场展示用的上下文区间价格。
type ModelMarketplacePricingInterval struct {
	MinTokens                     int     `json:"min_tokens"`
	MaxTokens                     *int    `json:"max_tokens,omitempty"`
	InputPricePerToken            float64 `json:"input_price_per_token,omitempty"`
	ImageInputPricePerToken       float64 `json:"image_input_price_per_token,omitempty"`
	OutputPricePerToken           float64 `json:"output_price_per_token,omitempty"`
	CacheWritePricePerToken       float64 `json:"cache_write_price_per_token,omitempty"`
	CacheWrite1hPricePerToken     float64 `json:"cache_write_1h_price_per_token,omitempty"`
	CacheReadPricePerToken        float64 `json:"cache_read_price_per_token,omitempty"`
	ImageOutputPricePerToken      float64 `json:"image_output_price_per_token,omitempty"`
	FastInputPricePerToken        float64 `json:"fast_input_price_per_token,omitempty"`
	FastImageInputPricePerToken   float64 `json:"fast_image_input_price_per_token,omitempty"`
	FastOutputPricePerToken       float64 `json:"fast_output_price_per_token,omitempty"`
	FastCacheWritePricePerToken   float64 `json:"fast_cache_write_price_per_token,omitempty"`
	FastCacheWrite1hPricePerToken float64 `json:"fast_cache_write_1h_price_per_token,omitempty"`
	FastCacheReadPricePerToken    float64 `json:"fast_cache_read_price_per_token,omitempty"`
	FastImageOutputPricePerToken  float64 `json:"fast_image_output_price_per_token,omitempty"`
}

type ModelMarketplaceModel struct {
	ID               string                  `json:"id"`
	DisplayName      string                  `json:"display_name"`
	Pricing          ModelMarketplacePricing `json:"pricing"`
	InputModalities  []string                `json:"input_modalities,omitempty"`
	OutputModalities []string                `json:"output_modalities,omitempty"`
}

type ModelMarketplaceCapacity struct {
	ConcurrencyUsed int `json:"concurrency_used"`
	ConcurrencyMax  int `json:"concurrency_max"`
	SessionsUsed    int `json:"sessions_used"`
	SessionsMax     int `json:"sessions_max"`
	RPMUsed         int `json:"rpm_used"`
	RPMMax          int `json:"rpm_max"`
}

type ModelMarketplaceAvailabilityDay struct {
	Date             string   `json:"date"`
	SuccessCount     int64    `json:"success_count"`
	TotalCount       int64    `json:"total_count"`
	AvailabilityRate *float64 `json:"availability_rate,omitempty"`
}

type ModelMarketplaceAvailability struct {
	WindowDays       int                               `json:"window_days"`
	BucketMinutes    int                               `json:"bucket_minutes"`
	SuccessCount     int64                             `json:"success_count"`
	TotalCount       int64                             `json:"total_count"`
	AvailabilityRate *float64                          `json:"availability_rate,omitempty"`
	LastStatus       string                            `json:"last_status,omitempty"`
	LastCheckedAt    *time.Time                        `json:"last_checked_at,omitempty"`
	Days             []ModelMarketplaceAvailabilityDay `json:"days"`
}

type ModelMarketplaceGroup struct {
	ID                         int64                         `json:"id"`
	Name                       string                        `json:"name"`
	Description                string                        `json:"description"`
	Platform                   string                        `json:"platform"`
	DisplayBrand               string                        `json:"display_brand"`
	SortOrder                  int                           `json:"sort_order"`
	RateMultiplier             float64                       `json:"rate_multiplier"`
	ImageRateIndependent       bool                          `json:"image_rate_independent"`
	ImageRateMultiplier        float64                       `json:"image_rate_multiplier"`
	OfficialPriceRatio         *float64                      `json:"official_price_ratio,omitempty"`
	OfficialPriceRMBEquivalent *float64                      `json:"official_price_rmb_equivalent,omitempty"`
	Capacity                   *ModelMarketplaceCapacity     `json:"capacity,omitempty"`
	Availability               *ModelMarketplaceAvailability `json:"availability,omitempty"`
	ModelCount                 int                           `json:"model_count"`
	Models                     []ModelMarketplaceModel       `json:"models"`
}

func ModelMarketplaceGroupsFromService(groups []service.ModelMarketplaceGroup) []ModelMarketplaceGroup {
	out := make([]ModelMarketplaceGroup, 0, len(groups))
	for _, group := range groups {
		models := make([]ModelMarketplaceModel, 0, len(group.Models))
		for _, model := range group.Models {
			models = append(models, ModelMarketplaceModel{
				ID:               model.ID,
				DisplayName:      model.DisplayName,
				Pricing:          modelMarketplacePricingFromService(model.Pricing),
				InputModalities:  model.InputModalities,
				OutputModalities: model.OutputModalities,
			})
		}

		out = append(out, ModelMarketplaceGroup{
			ID:                         group.ID,
			Name:                       group.Name,
			Description:                group.Description,
			Platform:                   group.Platform,
			DisplayBrand:               group.DisplayBrand,
			SortOrder:                  group.SortOrder,
			RateMultiplier:             group.RateMultiplier,
			ImageRateIndependent:       group.ImageRateIndependent,
			ImageRateMultiplier:        group.ImageRateMultiplier,
			OfficialPriceRatio:         group.OfficialPriceRatio,
			OfficialPriceRMBEquivalent: group.OfficialPriceRMBEquivalent,
			Capacity:                   modelMarketplaceCapacityFromService(group.Capacity),
			Availability:               modelMarketplaceAvailabilityFromService(group.Availability),
			ModelCount:                 group.ModelCount,
			Models:                     models,
		})
	}

	return out
}

// modelMarketplaceAvailabilityFromService 将主动可用性摘要转换为公开 DTO。
func modelMarketplaceAvailabilityFromService(availability *service.GroupAvailabilitySummary) *ModelMarketplaceAvailability {
	if availability == nil {
		return nil
	}
	days := make([]ModelMarketplaceAvailabilityDay, 0, len(availability.Days))
	for _, day := range availability.Days {
		days = append(days, ModelMarketplaceAvailabilityDay{
			Date:             day.Date,
			SuccessCount:     day.SuccessCount,
			TotalCount:       day.TotalCount,
			AvailabilityRate: day.AvailabilityRate,
		})
	}
	return &ModelMarketplaceAvailability{
		WindowDays:       availability.WindowDays,
		BucketMinutes:    availability.BucketMinutes,
		SuccessCount:     availability.SuccessCount,
		TotalCount:       availability.TotalCount,
		AvailabilityRate: availability.AvailabilityRate,
		LastStatus:       availability.LastStatus,
		LastCheckedAt:    availability.LastCheckedAt,
		Days:             days,
	}
}

// modelMarketplaceCapacityFromService 将分组容量快照转换为公开 DTO。
func modelMarketplaceCapacityFromService(capacity *service.GroupCapacitySummary) *ModelMarketplaceCapacity {
	if capacity == nil {
		return nil
	}
	return &ModelMarketplaceCapacity{
		ConcurrencyUsed: capacity.ConcurrencyUsed,
		ConcurrencyMax:  capacity.ConcurrencyMax,
		SessionsUsed:    capacity.SessionsUsed,
		SessionsMax:     capacity.SessionsMax,
		RPMUsed:         capacity.RPMUsed,
		RPMMax:          capacity.RPMMax,
	}
}

// modelMarketplacePricingFromService 将服务层价格快照转换为接口 DTO。
func modelMarketplacePricingFromService(pricing service.ModelDisplayPricing) ModelMarketplacePricing {
	intervals := make([]ModelMarketplacePricingInterval, 0, len(pricing.ContextIntervals))
	for _, interval := range pricing.ContextIntervals {
		intervals = append(intervals, ModelMarketplacePricingInterval{
			MinTokens:                     interval.MinTokens,
			MaxTokens:                     interval.MaxTokens,
			InputPricePerToken:            interval.InputPricePerToken,
			ImageInputPricePerToken:       interval.ImageInputPricePerToken,
			OutputPricePerToken:           interval.OutputPricePerToken,
			CacheWritePricePerToken:       interval.CacheWritePricePerToken,
			CacheWrite1hPricePerToken:     interval.CacheWrite1hPricePerToken,
			CacheReadPricePerToken:        interval.CacheReadPricePerToken,
			ImageOutputPricePerToken:      interval.ImageOutputPricePerToken,
			FastInputPricePerToken:        interval.FastInputPricePerToken,
			FastImageInputPricePerToken:   interval.FastImageInputPricePerToken,
			FastOutputPricePerToken:       interval.FastOutputPricePerToken,
			FastCacheWritePricePerToken:   interval.FastCacheWritePricePerToken,
			FastCacheWrite1hPricePerToken: interval.FastCacheWrite1hPricePerToken,
			FastCacheReadPricePerToken:    interval.FastCacheReadPricePerToken,
			FastImageOutputPricePerToken:  interval.FastImageOutputPricePerToken,
		})
	}

	return ModelMarketplacePricing{
		PricingMode:                   pricing.PricingMode,
		PriceStatus:                   pricing.PriceStatus,
		InputPricePerToken:            pricing.InputPricePerToken,
		ImageInputPricePerToken:       pricing.ImageInputPricePerToken,
		OutputPricePerToken:           pricing.OutputPricePerToken,
		CacheWritePricePerToken:       pricing.CacheWritePricePerToken,
		CacheWrite1hPricePerToken:     pricing.CacheWrite1hPricePerToken,
		CacheReadPricePerToken:        pricing.CacheReadPricePerToken,
		ImageOutputPricePerToken:      pricing.ImageOutputPricePerToken,
		FastInputPricePerToken:        pricing.FastInputPricePerToken,
		FastImageInputPricePerToken:   pricing.FastImageInputPricePerToken,
		FastOutputPricePerToken:       pricing.FastOutputPricePerToken,
		FastCacheWritePricePerToken:   pricing.FastCacheWritePricePerToken,
		FastCacheWrite1hPricePerToken: pricing.FastCacheWrite1hPricePerToken,
		FastCacheReadPricePerToken:    pricing.FastCacheReadPricePerToken,
		FastImageOutputPricePerToken:  pricing.FastImageOutputPricePerToken,
		ContextIntervals:              intervals,
		ImagePrice1K:                  pricing.ImagePrice1K,
		ImagePrice2K:                  pricing.ImagePrice2K,
		ImagePrice4K:                  pricing.ImagePrice4K,
		TimePricing:                   modelMarketplaceTimePricingFromService(pricing.TimePricing),
		GroupPeak:                     modelMarketplaceGroupPeakFromService(pricing.GroupPeak),
	}
}

// modelMarketplaceGroupPeakFromService 将分组高峰倍率展示信息转换为公开 DTO。
func modelMarketplaceGroupPeakFromService(groupPeak *service.ModelDisplayGroupPeak) *ModelMarketplaceGroupPeak {
	if groupPeak == nil {
		return nil
	}
	return &ModelMarketplaceGroupPeak{
		StartTime:  groupPeak.StartTime,
		EndTime:    groupPeak.EndTime,
		Multiplier: groupPeak.Multiplier,
		Active:     groupPeak.Active,
	}
}

// modelMarketplaceTimePricingFromService 将渠道分时倍率配置转换为公开 DTO。
func modelMarketplaceTimePricingFromService(timePricing *service.ModelDisplayTimePricing) *ModelMarketplaceTimePricing {
	if timePricing == nil || len(timePricing.Periods) == 0 {
		return nil
	}
	periods := make([]ModelMarketplaceTimePeriod, 0, len(timePricing.Periods))
	for _, period := range timePricing.Periods {
		periods = append(periods, ModelMarketplaceTimePeriod{
			StartTime:  period.StartTime,
			EndTime:    period.EndTime,
			Multiplier: period.Multiplier,
		})
	}
	return &ModelMarketplaceTimePricing{
		Timezone:         timePricing.Timezone,
		WeekdaysOnly:     timePricing.WeekdaysOnly,
		Periods:          periods,
		ActiveMultiplier: timePricing.ActiveMultiplier,
	}
}

func ModelMarketplaceStatsFromService(stats *service.DashboardPublicStats) ModelMarketplaceStats {
	return ModelMarketplaceStats{
		TodayTokens: stats.TodayTokens,
		TotalTokens: stats.TotalTokens,
		TotalUsers:  stats.TotalUsers,
	}
}
