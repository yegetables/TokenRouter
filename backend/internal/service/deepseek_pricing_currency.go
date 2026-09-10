package service

// 站点展示币种 → DeepSeek 官方价币种选择
//
// 站点币种（settings.balance_unit_name）本身只是展示标签，但 DeepSeek 官方价按
// 币种来自不同官方页面：中文页以人民币计价、英文页以美元计价。要避免"人民币数字
// 被当成美元记账"这类跨币种误用，计费热路径必须知道当前站点口径，才能取到同一
// 口径的官方数字。
//
// 这里用「包级 resolver + 短 TTL 缓存」提供站点币种：
//   - 由服务装配（ProvideSettingService）注入真实读取器；
//   - 计费热路径只读 atomic 缓存，不产生设置表读取；
//   - 币种设置变更后可调用 InvalidateDeepSeekPricingCurrencyCache 立即生效，
//     未调用时也最多在 TTL 内自动收敛。

import (
	"strings"
	"sync/atomic"
	"time"
)

const (
	// deepSeekDefaultCurrency 站点未配置币种时的默认口径（与 settings 默认值一致）。
	deepSeekDefaultCurrency = "USD"
	// deepSeekFallbackConstantsCurrency 内置兜底常量（billing_service.go）的币种口径。
	// 常量值为人民币口径，站点口径不是该币种时不得套用。
	deepSeekFallbackConstantsCurrency = "CNY"
	// deepSeekCurrencyCacheTTL 站点币种缓存时长（币种切换最多在该时长内生效）。
	deepSeekCurrencyCacheTTL = 60 * time.Second
)

type deepSeekCurrencyCacheEntry struct {
	currency  string
	expiresAt time.Time
}

var (
	deepSeekCurrencyResolver atomic.Value // func() string
	deepSeekCurrencyCache    atomic.Pointer[deepSeekCurrencyCacheEntry]
)

// SetDeepSeekPricingCurrencyResolver 注入站点展示币种读取器（由服务装配调用）。
func SetDeepSeekPricingCurrencyResolver(resolver func() string) {
	if resolver == nil {
		return
	}
	deepSeekCurrencyResolver.Store(resolver)
	InvalidateDeepSeekPricingCurrencyCache()
}

// InvalidateDeepSeekPricingCurrencyCache 立即失效币种缓存（币种设置变更后调用）。
func InvalidateDeepSeekPricingCurrencyCache() {
	deepSeekCurrencyCache.Store(nil)
}

// normalizeDeepSeekCurrency 归一化币种标识（大写、去空白）。
func normalizeDeepSeekCurrency(currency string) string {
	return strings.ToUpper(strings.TrimSpace(currency))
}

// deepSeekSiteCurrency 返回站点展示币种（归一化大写）。
// 读取器缺失或返回空值时按设置默认值 USD 处理；结果按 TTL 缓存。
func deepSeekSiteCurrency() string {
	if entry := deepSeekCurrencyCache.Load(); entry != nil && time.Now().Before(entry.expiresAt) {
		return entry.currency
	}
	currency := deepSeekDefaultCurrency
	if resolver, ok := deepSeekCurrencyResolver.Load().(func() string); ok && resolver != nil {
		if resolved := normalizeDeepSeekCurrency(resolver()); resolved != "" {
			currency = resolved
		}
	}
	deepSeekCurrencyCache.Store(&deepSeekCurrencyCacheEntry{
		currency:  currency,
		expiresAt: time.Now().Add(deepSeekCurrencyCacheTTL),
	})
	return currency
}
