package service

// 站点展示口径 → DeepSeek 官方价取值方式
//
// 站点币种（settings.balance_unit_name）只是展示标签，但网关的账本数字必须与站点
// 口径一致，否则等于少收/多收一个汇率。DeepSeek 官方价的取值方式：
//
//   - 站点 CNY → 直接用官方中文页的人民币价原样（不换算）
//   - 站点 USD → 官方中文页人民币价 ÷ usd_exchange_rate（设置语义 1 USD = N CNY），
//     与同步脚本折算其它分组（组7/8、渠道价）的方式完全一致，站内口径统一；
//     汇率缺失或非正数时不覆盖价卡，并打一条告警日志
//   - 其它币种 → 不覆盖价卡（保持上游/内置价），同样打告警日志便于排查
//
// 这里用「包级 resolver + 短 TTL 缓存」提供站点口径：
//   - 由服务装配（ProvideSettingService）注入真实读取器；
//   - 计费热路径只读 atomic 缓存，不产生设置表读取；
//   - 设置写入后调用 InvalidateDeepSeekPricingCache 立即失效，
//     未调用时也最多在 TTL 内自动收敛。

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/pkg/logger"
)

const (
	// deepSeekDefaultCurrency 站点未配置币种时的默认口径（与 settings 默认值一致）。
	deepSeekDefaultCurrency = "USD"
	// deepSeekBaseCurrency 官方价快照的基准币种：人民币（中文页原样价）。
	deepSeekBaseCurrency = "CNY"
	// deepSeekFallbackConstantsCurrency 内置兜底常量（billing_service.go）的币种口径。
	// 常量值为人民币口径，站点口径不是该币种时不得套用。
	deepSeekFallbackConstantsCurrency = deepSeekBaseCurrency
	// deepSeekCurrencyCacheTTL 站点口径缓存时长（切换币种/汇率最多在该时长内生效）。
	deepSeekCurrencyCacheTTL = 60 * time.Second

	deepSeekPricingCurrencyLogScope = "service.deepseek_pricing"
)

// DeepSeekPricingDisplaySettings 站点展示口径：展示币种 + 美元兑人民币汇率。
type DeepSeekPricingDisplaySettings struct {
	// Currency 归一化后的展示币种（大写）；空串表示未配置。
	Currency string
	// USDExchangeRate 站点配置的汇率，语义为 1 USD = N CNY；<=0 表示未配置或非法。
	USDExchangeRate float64
}

type deepSeekDisplayCacheEntry struct {
	settings  DeepSeekPricingDisplaySettings
	expiresAt time.Time
}

var (
	deepSeekDisplayResolver atomic.Value // func() DeepSeekPricingDisplaySettings
	deepSeekDisplayCache    atomic.Pointer[deepSeekDisplayCacheEntry]
	// deepSeekWarnedDisplayKey 记录上一次告警的口径，避免每个缓存周期重复刷同一条日志。
	deepSeekWarnedDisplayKey atomic.Value // string
)

// SetDeepSeekPricingDisplayResolver 注入站点展示口径读取器（由服务装配调用）。
func SetDeepSeekPricingDisplayResolver(resolver func() DeepSeekPricingDisplaySettings) {
	if resolver == nil {
		return
	}
	deepSeekDisplayResolver.Store(resolver)
	InvalidateDeepSeekPricingCache()
}

// InvalidateDeepSeekPricingCache 立即失效站点口径缓存（币种或汇率设置变更后调用）。
func InvalidateDeepSeekPricingCache() {
	deepSeekDisplayCache.Store(nil)
}

// InvalidateDeepSeekPricingCurrencyCache 兼容旧名，语义同 InvalidateDeepSeekPricingCache。
func InvalidateDeepSeekPricingCurrencyCache() {
	InvalidateDeepSeekPricingCache()
}

// normalizeDeepSeekCurrency 归一化币种标识（大写、去空白）。
func normalizeDeepSeekCurrency(currency string) string {
	return strings.ToUpper(strings.TrimSpace(currency))
}

// deepSeekSiteDisplay 返回站点展示口径（按 TTL 缓存）。
// 读取器缺失或币种为空时按设置默认值 USD 处理。
func deepSeekSiteDisplay() DeepSeekPricingDisplaySettings {
	if entry := deepSeekDisplayCache.Load(); entry != nil && time.Now().Before(entry.expiresAt) {
		return entry.settings
	}
	settings := DeepSeekPricingDisplaySettings{Currency: deepSeekDefaultCurrency}
	if resolver, ok := deepSeekDisplayResolver.Load().(func() DeepSeekPricingDisplaySettings); ok && resolver != nil {
		resolved := resolver()
		if currency := normalizeDeepSeekCurrency(resolved.Currency); currency != "" {
			settings.Currency = currency
		}
		if resolved.USDExchangeRate > 0 {
			settings.USDExchangeRate = resolved.USDExchangeRate
		}
	}
	deepSeekDisplayCache.Store(&deepSeekDisplayCacheEntry{
		settings:  settings,
		expiresAt: time.Now().Add(deepSeekCurrencyCacheTTL),
	})
	warnDeepSeekDisplayInert(settings)
	return settings
}

// deepSeekSiteCurrency 返回站点展示币种（归一化大写）。
func deepSeekSiteCurrency() string {
	return deepSeekSiteDisplay().Currency
}

// deepSeekDisplayUsable 报告该口径下能否取到官方价（CNY 直用 / USD 按汇率折算）。
func deepSeekDisplayUsable(settings DeepSeekPricingDisplaySettings) bool {
	switch settings.Currency {
	case deepSeekBaseCurrency:
		return true
	case deepSeekDefaultCurrency:
		return settings.USDExchangeRate > 0
	default:
		return false
	}
}

// warnDeepSeekDisplayInert 当站点口径无法取用官方价时打一条告警（同一口径只打一次）。
// 站点币种写成非 CNY/USD、或 USD 站点未配置汇率时，DeepSeek 官方价会静默退回
// 上游/内置价卡；这条日志用于排查"官方价怎么没生效"。
func warnDeepSeekDisplayInert(settings DeepSeekPricingDisplaySettings) {
	if deepSeekDisplayUsable(settings) {
		deepSeekWarnedDisplayKey.Store("")
		return
	}
	key := settings.Currency
	if key == deepSeekDefaultCurrency {
		key += ":no-rate"
	}
	if last, ok := deepSeekWarnedDisplayKey.Load().(string); ok && last == key {
		return
	}
	deepSeekWarnedDisplayKey.Store(key)

	switch settings.Currency {
	case deepSeekDefaultCurrency:
		logger.LegacyPrintf(deepSeekPricingCurrencyLogScope,
			"[DeepSeekPricing] 站点币种为 %s 但未配置 usd_exchange_rate（1 USD = N CNY），"+
				"官方价不覆盖价卡，改用上游/内置价卡；请在管理后台设置汇率后自动生效",
			settings.Currency)
	default:
		logger.LegacyPrintf(deepSeekPricingCurrencyLogScope,
			"[DeepSeekPricing] 站点币种 %s 无法从官方价折算（仅支持 CNY 直用 / USD 按汇率折算），"+
				"官方价不覆盖价卡，改用上游/内置价卡",
			settings.Currency)
	}
}
