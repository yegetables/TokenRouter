package pricing

// ModelPricing 保存用于结算的每 token 单价及模型计费规则。
type ModelPricing struct {
	// TimePricing 是目录声明的分时规则，仅用于默认目录价。
	TimePricing                        *TimePricingConfig
	CatalogSource                      string
	PriorityInputPresent               bool
	PriorityOutputPresent              bool
	PriorityCacheReadPresent           bool
	PriorityCacheWritePresent          bool
	ContextPrices                      []ContextModelPrice
	InputPricePerToken                 float64  // 每token输入价格 (USD)
	InputPricePerTokenPriority         float64  // priority service tier 下每token输入价格 (USD)
	ImageInputPricePerToken            float64  // 图片输入 token 价格 (USD)，为 0 时回退到普通输入价格
	OutputPricePerToken                float64  // 每token输出价格 (USD)
	OutputPricePerTokenPriority        float64  // priority service tier 下每token输出价格 (USD)
	CacheCreationPricePerToken         float64  // 缓存创建每token价格 (USD)
	CacheCreationPricePerTokenPriority float64  // priority service tier 下缓存创建每token价格 (USD)
	CacheCreationPriceExplicit         bool     // 价卡、区间或目录显式零价不触发缓存费用回退
	CacheReadPricePerToken             float64  // 缓存读取每token价格 (USD)
	CacheReadPricePerTokenPriority     float64  // priority service tier 下缓存读取每token价格 (USD)
	CacheCreation5mPrice               float64  // 5分钟缓存创建每token价格 (USD)
	CacheCreation1hPrice               float64  // 1小时缓存创建每token价格 (USD)
	SupportsCacheBreakdown             bool     // 是否支持详细的缓存分类
	SupportsServiceTier                bool     // 是否支持 service_tier（Fast/Flex）
	FastModeMultiplier                 *float64 // 价卡配置的 Fast 模式收费倍率；nil 表示沿用模型默认 Fast 定价
	FastMultiplier                     *float64 // 新版价卡 Fast/priority 倍率
	FlexMultiplier                     *float64 // 价卡配置的 Flex 倍率
	// MaxReasoningEffortMultiplier 仅在最终推理档位为 max 时应用。
	MaxReasoningEffortMultiplier  *float64
	LongContextInputThreshold     int     // 超过阈值后按整次会话提升输入价格
	LongContextThresholdInclusive bool    // 达到阈值即应用（xAI）；默认严格大于以兼容既有模型
	LongContextInputMultiplier    float64 // 长上下文整次会话输入倍率
	LongContextOutputMultiplier   float64 // 长上下文整次会话输出倍率
	ImageOutputPricePerToken      float64 // 图片输出 token 价格 (USD)
	ImageOutputPriceExplicit      bool    // 目录或价卡显式设定后不再回退，零价也有效
}

// ContextModelPrice 保存严格超过阈值时使用的整次绝对单价。
// 来源使用包含阈值的边界时，由目录转换层先减一，结算和展示共用 (min,max]。
type ContextModelPrice struct {
	Threshold int
	Pricing   *ModelPricing
}

// UsageTokens 使用的token数量
type UsageTokens struct {
	InputTokens           int
	ImageInputTokens      int
	OutputTokens          int
	CacheCreationTokens   int
	CacheReadTokens       int
	CacheCreation5mTokens int
	CacheCreation1hTokens int
	ImageOutputTokens     int
}

// CostBreakdown 费用明细
type CostBreakdown struct {
	InputCost                 float64 // 文本输入费用（不含图片输入，图片输入单独记入 ImageInputCost）
	ImageInputCost            float64 // 图片输入 token 费用（如 gpt-image-2 图片编辑）
	OutputCost                float64
	ImageOutputCost           float64
	CacheCreationCost         float64
	CacheReadCost             float64
	TotalCost                 float64
	ActualCost                float64 // 应用倍率后的实际费用
	BillingMode               string  // 计费模式（"token"/"per_request"/"image"），由 CalculateCostUnified 填充
	LongContextBillingApplied bool    // 长上下文规则是否实际增加费用
}

// ModelDisplayPricing 是面向前端展示的模型价格快照。
// 所有价格都已经应用了分组倍率，直接表示实际扣费单价。
type ModelDisplayPricing struct {
	PricingMode             string
	PriceStatus             string
	InputPricePerToken      float64
	ImageInputPricePerToken float64
	OutputPricePerToken     float64
	CacheWritePricePerToken float64
	// CacheWrite1hPricePerToken 是可选的 1 小时缓存写入展示单价。
	CacheWrite1hPricePerToken     float64
	CacheReadPricePerToken        float64
	ImageOutputPricePerToken      float64
	FastInputPricePerToken        float64
	FastImageInputPricePerToken   float64
	FastOutputPricePerToken       float64
	FastCacheWritePricePerToken   float64
	FastCacheWrite1hPricePerToken float64
	FastCacheReadPricePerToken    float64
	FastImageOutputPricePerToken  float64
	ContextIntervals              []ModelDisplayPricingInterval
	// ImagePriceSizes 标记确有报价的尺寸，区分显式零价与缺价。
	ImagePriceSizes []string
	ImagePrice1K    float64
	ImagePrice2K    float64
	ImagePrice4K    float64
	// TimePricing 是生效的分时倍率快照；展示价已按 ActiveMultiplier 计入，nil 表示未配置。
	TimePricing *ModelDisplayTimePricing
	// PeakRate 是价格配置高峰窗口快照；token 模式下展示价已计入 Multiplier，nil 表示未启用。
	PeakRate *ModelDisplayPeakRate
	// ActiveMultiplier 是本时刻实际计入展示价的总倍率（分时倍率 × 高峰因子；图片、按次与视频恒为 1）。
	// 它与 TimePricing、PeakRate 同源同刻生成，供公开 DTO 直接下发，避免前端再乘一次而与结算口径分叉。
	ActiveMultiplier float64
}

// ModelDisplayTimePricing 是模型广场展示用的分时倍率快照。
type ModelDisplayTimePricing struct {
	Timezone     string
	WeekdaysOnly bool
	// Periods 是价卡配置的原始时段，供前端标注时段。
	Periods []TimePricingPeriod
	// ActiveMultiplier 是 at 时刻生效的倍率；1 表示低谷。
	ActiveMultiplier float64
}

// ModelDisplayPeakRate 是模型广场展示用的高峰窗口快照。
type ModelDisplayPeakRate struct {
	StartTime  string
	EndTime    string
	Multiplier float64
	// Active 表示 at 时刻正处于该高峰窗口。
	Active bool
}

// ModelDisplayPricingInterval 是按上下文 token 区间展示的模型价格。
type ModelDisplayPricingInterval struct {
	MinTokens                     int
	MaxTokens                     *int
	InputPricePerToken            float64
	ImageInputPricePerToken       float64
	OutputPricePerToken           float64
	CacheWritePricePerToken       float64
	CacheWrite1hPricePerToken     float64
	CacheReadPricePerToken        float64
	ImageOutputPricePerToken      float64
	FastInputPricePerToken        float64
	FastImageInputPricePerToken   float64
	FastOutputPricePerToken       float64
	FastCacheWritePricePerToken   float64
	FastCacheWrite1hPricePerToken float64
	FastCacheReadPricePerToken    float64
	FastImageOutputPricePerToken  float64
}

type AudioPriceConfig struct {
	RealtimePerMin *float64
	TTSPerMChars   *float64
	STTPerHour     *float64
}
