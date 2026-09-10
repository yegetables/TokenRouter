package service

// ============================================================================
// DeepSeek 官方定价与峰谷时段自动同步
//
// 背景
//   DeepSeek 官方未提供价格查询 API（已验证：/v1/models 仅返回模型名、
//   /user/balance 仅返回余额、其余价格类端点均 404）。价格与峰谷时段的
//   唯一权威来源是官方文档定价页，因此这里采用「抓取官方文档页 → 结构化解析
//   → 自检校验 → 落盘 → 热加载」的方式自动同步，使官方调价或峰谷时段调整
//   无需修改代码或升级镜像即可生效。
//
// 数据源（多币种，两页同时抓取、各自独立更新）
//   1) 配置 pricing.deepseek_pricing_url（默认官方中文定价页，人民币原生价格）
//      + pricing.deepseek_pricing_url_usd（默认官方英文定价页，美元原生价格）
//      · 返回 JSON（若未来官方提供 API）→ 按 deepSeekOfficialJSONPayload 解析，
//        币种取 payload.currency
//      · 返回 HTML（当前）→ 按文档页表格解析，币种由价格单元格的货币标记判定
//   2) 落盘文件 {pricing.data_dir}/deepseek_official_pricing.json（上次成功结果）
//   3) 内置常量（仅人民币口径，见 billing_service.go）
//
// 币种选择：计费时按站点展示币种（settings.balance_unit_name）取同一口径的那一套
// 数字；缺失该币种快照时不换算、也不借用其它币种的数字，详见
// deepseek_pricing_currency.go。峰谷倍率与时段是比值与时刻，与币种无关。
//
// 计费热路径仅读取内存 atomic 快照，不产生网络或磁盘 IO。
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/TokenFlux/TokenRouter/internal/pkg/logger"
	"github.com/TokenFlux/TokenRouter/internal/util/urlvalidator"
)

const (
	// 默认数据源：官方中文定价页（价格以人民币元计价，与国内账号余额口径一致）
	defaultDeepSeekPricingURL = "https://api-docs.deepseek.com/zh-cn/quick_start/pricing"
	// 英文页作为解析兜底（美元计价）
	fallbackDeepSeekPricingURL = "https://api-docs.deepseek.com/quick_start/pricing"

	deepSeekPricingFileName = "deepseek_official_pricing.json"
	deepSeekPricingLogScope = "service.deepseek_pricing"

	defaultDeepSeekSyncIntervalHours = 6
	deepSeekPricingHTTPTimeout       = 25 * time.Second
)

// DeepSeekPeakWindow 单个高峰时段（使用 DeepSeekOfficialPricing.Timezone 指定时区）
type DeepSeekPeakWindow struct {
	Start string `json:"start"` // "09:00"
	End   string `json:"end"`   // "12:00"
}

// DeepSeekModelRate 单模型官方空闲时段单价（单位：货币/token）与高峰倍率
type DeepSeekModelRate struct {
	InputOffPeak         float64 `json:"input_off_peak"`             // 输入（缓存未命中）
	InputCacheHitOffPeak float64 `json:"input_cache_hit_off_peak"`   // 输入（缓存命中）
	OutputOffPeak        float64 `json:"output_off_peak"`            // 输出
	PeakMultiplier       float64 `json:"peak_multiplier"`            // 高峰倍率（官方为 2）
}

// DeepSeekCurrencyRates 单一币种的官方价格与来源（一个币种对应一个官方定价页）。
type DeepSeekCurrencyRates struct {
	Currency  string                       `json:"currency"`   // "CNY" / "USD"
	SourceURL string                       `json:"source_url"` // 该币种价格的来源页面
	FetchedAt time.Time                    `json:"fetched_at"`
	Models    map[string]DeepSeekModelRate `json:"models"` // key: "flash" / "pro"
}

// DeepSeekOfficialPricing 官方定价与峰谷快照（按币种保存多套价格）。
//
// 站点展示币种可以切换（settings.balance_unit_name），而官方中文页以人民币计价、
// 英文页以美元计价，因此快照按币种分别保存：计费时只取与站点口径相同的那一套
// 数字，不做汇率换算，也不跨币种复用数字。
type DeepSeekOfficialPricing struct {
	FetchedAt    time.Time                        `json:"fetched_at"`
	Timezone     string                           `json:"timezone"`      // 峰谷时段所属时区
	WeekdaysOnly bool                             `json:"weekdays_only"` // 高峰仅工作日
	PeakWindows  []DeepSeekPeakWindow             `json:"peak_windows"`
	Currencies   map[string]DeepSeekCurrencyRates `json:"currencies"` // key: "CNY" / "USD"

	// 以下为 v1 单币种快照的兼容字段：读盘时迁移进 Currencies，之后不再写入。
	Currency  string                       `json:"currency,omitempty"`
	SourceURL string                       `json:"source_url,omitempty"`
	Models    map[string]DeepSeekModelRate `json:"models,omitempty"`
}

// newDeepSeekPricingSnapshot 构造空快照。
func newDeepSeekPricingSnapshot() *DeepSeekOfficialPricing {
	return &DeepSeekOfficialPricing{Currencies: map[string]DeepSeekCurrencyRates{}}
}

// ratesFor 返回指定币种的官方价（币种按大写比对）。
func (s *DeepSeekOfficialPricing) ratesFor(currency string) (DeepSeekCurrencyRates, bool) {
	if s == nil {
		return DeepSeekCurrencyRates{}, false
	}
	rates, ok := s.Currencies[normalizeDeepSeekCurrency(currency)]
	return rates, ok && len(rates.Models) > 0
}

// primaryCurrency 返回快照内已解析出的第一个币种（单源解析结果使用）。
func (s *DeepSeekOfficialPricing) primaryCurrency() string {
	if s == nil {
		return ""
	}
	if len(s.Currencies) == 1 {
		for k := range s.Currencies {
			return k
		}
	}
	for _, preferred := range []string{deepSeekFallbackConstantsCurrency, deepSeekDefaultCurrency} {
		if _, ok := s.Currencies[preferred]; ok {
			return preferred
		}
	}
	for k := range s.Currencies {
		return k
	}
	return ""
}

// migrateLegacyCurrencies 将 v1 单币种快照字段迁移进 Currencies，并清空兼容字段。
func (s *DeepSeekOfficialPricing) migrateLegacyCurrencies() {
	if s == nil {
		return
	}
	if s.Currencies == nil {
		s.Currencies = map[string]DeepSeekCurrencyRates{}
	}
	legacyCurrency := normalizeDeepSeekCurrency(s.Currency)
	if legacyCurrency != "" && len(s.Models) > 0 {
		if _, exists := s.Currencies[legacyCurrency]; !exists {
			s.Currencies[legacyCurrency] = DeepSeekCurrencyRates{
				Currency:  legacyCurrency,
				SourceURL: s.SourceURL,
				FetchedAt: s.FetchedAt,
				Models:    s.Models,
			}
		}
	}
	s.Currency = ""
	s.SourceURL = ""
	s.Models = nil
}

// cloneDeepSeekPricing 深拷贝快照（刷新时以上一份为基础，保留未被本轮更新的币种）。
func cloneDeepSeekPricing(src *DeepSeekOfficialPricing) *DeepSeekOfficialPricing {
	if src == nil {
		return nil
	}
	body, err := json.Marshal(src)
	if err != nil {
		return nil
	}
	var cloned DeepSeekOfficialPricing
	if err := json.Unmarshal(body, &cloned); err != nil {
		return nil
	}
	cloned.migrateLegacyCurrencies()
	return &cloned
}

// deepSeekOfficialJSONPayload 未来若官方提供 JSON API 时的期望结构
type deepSeekOfficialJSONPayload struct {
	Currency     string          `json:"currency"`
	Timezone     string          `json:"timezone"`
	WeekdaysOnly *bool           `json:"weekdays_only"`
	PeakWindows  []DeepSeekPeakWindow `json:"peak_windows"`
	Models       map[string]struct {
		InputOffPeak         float64 `json:"input_off_peak"`
		InputCacheHitOffPeak float64 `json:"input_cache_hit_off_peak"`
		OutputOffPeak        float64 `json:"output_off_peak"`
		PeakMultiplier       float64 `json:"peak_multiplier"`
	} `json:"models"`
}

var (
	deepSeekOfficial atomic.Pointer[DeepSeekOfficialPricing]
	deepSeekSyncOnce sync.Once
	deepSeekDataPath string
)

// StartDeepSeekOfficialPricingSync 载入磁盘快照并启动周期同步（幂等）。
// 仅在配置了 pricing.data_dir 时启用：生产环境该值始终存在（默认 ./data），
// 而单元测试构造的空配置不会触发后台网络抓取。
func StartDeepSeekOfficialPricingSync(cfg *config.Config) {
	if cfg == nil || strings.TrimSpace(cfg.Pricing.DataDir) == "" {
		return
	}
	deepSeekSyncOnce.Do(func() {
		dataDir := strings.TrimSpace(cfg.Pricing.DataDir)
		deepSeekDataPath = filepath.Join(dataDir, deepSeekPricingFileName)

		if snap := loadDeepSeekPricingSnapshot(deepSeekDataPath); snap != nil {
			deepSeekOfficial.Store(snap)
			logger.LegacyPrintf(deepSeekPricingLogScope,
				"[DeepSeekPricing] loaded snapshot: currencies=%s fetched_at=%s rates=%s",
				describeDeepSeekCurrencies(snap), snap.FetchedAt.Format(time.RFC3339), describeDeepSeekRates(snap))
		}

		if cfg != nil && !deepSeekAutoSyncEnabled(cfg) {
			logger.LegacyPrintf(deepSeekPricingLogScope, "[DeepSeekPricing] auto sync disabled")
			return
		}

		interval := defaultDeepSeekSyncIntervalHours
		if cfg != nil && cfg.Pricing.DeepSeekSyncIntervalHours > 0 {
			interval = cfg.Pricing.DeepSeekSyncIntervalHours
		}

		go deepSeekPricingSyncLoop(cfg, time.Duration(interval)*time.Hour)
	})
}

func deepSeekAutoSyncEnabled(cfg *config.Config) bool {
	if cfg == nil {
		return true
	}
	return deepSeekAutoSyncSetting(cfg)
}

// deepSeekAutoSyncSetting 返回显式配置值；未配置时视为启用。
func deepSeekAutoSyncSetting(cfg *config.Config) bool {
	if cfg == nil || cfg.Pricing.DeepSeekAutoSync == nil {
		return true
	}
	return *cfg.Pricing.DeepSeekAutoSync
}

// deepSeekPricingSyncLoop 周期性刷新官方定价；失败时保留上一份快照。
func deepSeekPricingSyncLoop(cfg *config.Config, interval time.Duration) {
	// 启动后稍作延迟再拉取，避免与其它启动任务抢占网络
	time.Sleep(30 * time.Second)
	refreshDeepSeekOfficialPricing(cfg)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		refreshDeepSeekOfficialPricing(cfg)
	}
}

// refreshDeepSeekOfficialPricing 依次抓取各币种的官方定价页，合并进快照并落盘。
//
// 每个币种独立更新：某个币种抓取或校验失败时保留上一份好数据（含磁盘载入的），
// 不用另一币种的数字补齐，也不做汇率换算。
func refreshDeepSeekOfficialPricing(cfg *config.Config) {
	prev := deepSeekOfficial.Load()
	next := cloneDeepSeekPricing(prev)
	if next == nil {
		next = newDeepSeekPricingSnapshot()
	}

	updated := make(map[string]struct{})
	windowsFromSource := false
	var lastErr error
	for _, src := range deepSeekPricingSourceURLs(cfg) {
		parsed, err := fetchDeepSeekOfficialPricing(cfg, src)
		if err != nil {
			lastErr = err
			logger.LegacyPrintf(deepSeekPricingLogScope,
				"[DeepSeekPricing] fetch failed: url=%s err=%v", src, err)
			continue
		}
		// 峰谷时段只采纳本轮第一个成功来源：来源顺序固定，避免中文页（北京时间）
		// 与英文页（UTC）的等价表示互相覆盖。
		currency, err := mergeDeepSeekSourceSnapshot(next, parsed, !windowsFromSource)
		if err != nil {
			lastErr = err
			logger.LegacyPrintf(deepSeekPricingLogScope,
				"[DeepSeekPricing] rejected source: url=%s err=%v", src, err)
			continue
		}
		updated[currency] = struct{}{}
		if len(parsed.PeakWindows) > 0 {
			windowsFromSource = true
		}
	}

	if len(updated) == 0 {
		if lastErr != nil {
			logger.LegacyPrintf(deepSeekPricingLogScope,
				"[DeepSeekPricing] all sources failed, keeping previous snapshot: %v", lastErr)
		}
		return
	}

	if err := validateDeepSeekOfficialPricing(next); err != nil {
		logger.LegacyPrintf(deepSeekPricingLogScope,
			"[DeepSeekPricing] merged snapshot rejected, keeping previous: %v", err)
		return
	}

	next.FetchedAt = time.Now()
	deepSeekOfficial.Store(next)
	if err := saveDeepSeekPricingSnapshot(deepSeekDataPath, next); err != nil {
		logger.LegacyPrintf(deepSeekPricingLogScope, "[DeepSeekPricing] persist failed: %v", err)
	}
	if prev == nil || !deepSeekPricingEqual(prev, next) {
		logger.LegacyPrintf(deepSeekPricingLogScope,
			"[DeepSeekPricing] updated: currencies=%s peak=%v rates=%s",
			describeDeepSeekCurrencies(next), next.PeakWindows, describeDeepSeekRates(next))
	}
}

// mergeDeepSeekSourceSnapshot 把单源解析结果合并进目标快照，返回该来源的币种。
// applyWindows 为 true 时同时采纳该来源的峰谷时段（快照级字段）。
// 单币种校验不通过时返回错误，且不修改目标快照。
func mergeDeepSeekSourceSnapshot(dst, src *DeepSeekOfficialPricing, applyWindows bool) (string, error) {
	if dst == nil || src == nil {
		return "", fmt.Errorf("nil snapshot")
	}
	currency := src.primaryCurrency()
	if currency == "" {
		return "", fmt.Errorf("no currency parsed")
	}
	rates, ok := src.Currencies[currency]
	if !ok {
		return "", fmt.Errorf("currency %q has no rates", currency)
	}
	if err := validateDeepSeekCurrencyRates(currency, rates); err != nil {
		return "", err
	}
	if applyWindows && len(src.PeakWindows) > 0 {
		if err := validateDeepSeekPeakWindows(src.Timezone, src.PeakWindows); err != nil {
			return "", err
		}
		dst.Timezone = src.Timezone
		dst.WeekdaysOnly = src.WeekdaysOnly
		dst.PeakWindows = src.PeakWindows
	}
	if dst.Currencies == nil {
		dst.Currencies = map[string]DeepSeekCurrencyRates{}
	}
	dst.Currencies[currency] = rates
	return currency, nil
}

func deepSeekPricingSourceURLs(cfg *config.Config) []string {
	var out []string
	if cfg != nil {
		if u := strings.TrimSpace(cfg.Pricing.DeepSeekPricingURL); u != "" {
			out = append(out, u)
		}
		if u := strings.TrimSpace(cfg.Pricing.DeepSeekPricingURLUSD); u != "" {
			out = append(out, u)
		}
	}
	out = append(out, defaultDeepSeekPricingURL, fallbackDeepSeekPricingURL)
	return dedupeStrings(out)
}

// fetchDeepSeekOfficialPricing 拉取单个数据源并解析（自动识别 JSON / HTML）。
// 数据源 URL 需通过安全白名单校验（security.url_allowlist.pricing_hosts）。
func fetchDeepSeekOfficialPricing(cfg *config.Config, src string) (*DeepSeekOfficialPricing, error) {
	target, err := validateDeepSeekPricingSourceURL(cfg, src)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), deepSeekPricingHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "TokenRouter/DeepSeekPricingSync")
	req.Header.Set("Accept", "text/html,application/json;q=0.9,*/*;q=0.8")

	client := deepSeekPricingHTTPClient(cfg)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}

	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "{") ||
		strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "application/json") {
		return parseDeepSeekPricingJSON([]byte(trimmed), src)
	}
	return parseDeepSeekPricingHTML(body, src)
}

// deepSeekPricingHTTPClient 构造抓取客户端（支持通过 update.proxy_url 走代理）。
func deepSeekPricingHTTPClient(cfg *config.Config) *http.Client {
	client := &http.Client{Timeout: deepSeekPricingHTTPTimeout}
	if cfg == nil {
		return client
	}
	proxyURL := strings.TrimSpace(cfg.Update.ProxyURL)
	if proxyURL == "" {
		return client
	}
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return client
	}
	client.Transport = &http.Transport{Proxy: http.ProxyURL(parsed)}
	return client
}

// validateDeepSeekPricingSourceURL 按安全白名单校验数据源 URL。
// 未启用白名单时仅做格式校验；启用时必须命中 security.url_allowlist.pricing_hosts。
func validateDeepSeekPricingSourceURL(cfg *config.Config, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty pricing source url")
	}
	if cfg == nil || !cfg.Security.URLAllowlist.Enabled {
		normalized, err := urlvalidator.ValidateURLFormat(raw, true)
		if err != nil {
			return "", fmt.Errorf("invalid deepseek pricing url: %w", err)
		}
		return normalized, nil
	}
	normalized, err := urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
		AllowedHosts:     cfg.Security.URLAllowlist.PricingHosts,
		RequireAllowlist: true,
		AllowPrivate:     cfg.Security.URLAllowlist.AllowPrivateHosts,
	})
	if err != nil {
		return "", fmt.Errorf("deepseek pricing url not allowed: %w", err)
	}
	return normalized, nil
}

// parseDeepSeekPricingJSON 解析未来的 JSON API 响应。
func parseDeepSeekPricingJSON(body []byte, src string) (*DeepSeekOfficialPricing, error) {
	var payload deepSeekOfficialJSONPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("json decode: %w", err)
	}
	currency := normalizeDeepSeekCurrency(payload.Currency)
	if currency == "" {
		currency = deepSeekFallbackConstantsCurrency
	}
	models := make(map[string]DeepSeekModelRate, len(payload.Models))
	for name, m := range payload.Models {
		models[normalizeDeepSeekFamily(name)] = DeepSeekModelRate{
			InputOffPeak:         m.InputOffPeak,
			InputCacheHitOffPeak: m.InputCacheHitOffPeak,
			OutputOffPeak:        m.OutputOffPeak,
			PeakMultiplier:       m.PeakMultiplier,
		}
	}
	now := time.Now()
	snap := &DeepSeekOfficialPricing{
		FetchedAt:    now,
		Timezone:     strings.TrimSpace(payload.Timezone),
		WeekdaysOnly: payload.WeekdaysOnly == nil || *payload.WeekdaysOnly,
		PeakWindows:  payload.PeakWindows,
		Currencies: map[string]DeepSeekCurrencyRates{
			currency: {Currency: currency, SourceURL: src, FetchedAt: now, Models: models},
		},
	}
	if snap.Timezone == "" {
		snap.Timezone = "Asia/Shanghai"
	}
	return snap, nil
}

var (
	deepSeekTDCellRe = regexp.MustCompile(`(?is)<td[^>]*>(.*?)</td>`)
	deepSeekTagRe    = regexp.MustCompile(`(?s)<[^>]+>`)
	deepSeekNumberRe = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)`)
	deepSeekWindowRe = regexp.MustCompile(`([0-9]{1,2}:[0-9]{2})\s*(?:-|–|—|~|至)\s*([0-9]{1,2}:[0-9]{2})`)
)

// parseDeepSeekPricingHTML 解析官方文档定价页（中文/英文页均支持）。
//
// 表格结构（td 单元格序列）：
//
//	1M INPUT TOKENS (CACHE HIT) | OFF-PEAK | v_flash v_pro | PEAK | v_flash v_pro
//	1M INPUT TOKENS (CACHE MISS)| OFF-PEAK | v_flash v_pro | PEAK | v_flash v_pro
//	1M OUTPUT TOKENS            | OFF-PEAK | v_flash v_pro | PEAK | v_flash v_pro
//
// 中文页对应：百万tokens输入（缓存命中/未命中）/ 百万tokens输出 + 空闲时段/高峰时段。
func parseDeepSeekPricingHTML(body []byte, src string) (*DeepSeekOfficialPricing, error) {
	raw := string(body)
	cells := make([]string, 0, 128)
	for _, m := range deepSeekTDCellRe.FindAllStringSubmatch(raw, -1) {
		cells = append(cells, cleanDeepSeekCell(m[1]))
	}
	if len(cells) == 0 {
		return nil, fmt.Errorf("no table cells found")
	}

	hitRow, err := deepSeekFindPriceRow(cells, deepSeekCacheHitLabels)
	if err != nil {
		return nil, fmt.Errorf("cache hit row: %w", err)
	}
	missRow, err := deepSeekFindPriceRow(cells, deepSeekCacheMissLabels)
	if err != nil {
		return nil, fmt.Errorf("cache miss row: %w", err)
	}
	outRow, err := deepSeekFindPriceRow(cells, deepSeekOutputLabels)
	if err != nil {
		return nil, fmt.Errorf("output row: %w", err)
	}

	currency := detectDeepSeekCurrency(cells)
	now := time.Now()
	models := map[string]DeepSeekModelRate{
		"flash": {
			InputCacheHitOffPeak: perMillionToPerToken(hitRow.offPeak[0]),
			InputOffPeak:         perMillionToPerToken(missRow.offPeak[0]),
			OutputOffPeak:        perMillionToPerToken(outRow.offPeak[0]),
			PeakMultiplier:       ratioOr(hitRow.peak[0], hitRow.offPeak[0], 2),
		},
		"pro": {
			InputCacheHitOffPeak: perMillionToPerToken(hitRow.offPeak[1]),
			InputOffPeak:         perMillionToPerToken(missRow.offPeak[1]),
			OutputOffPeak:        perMillionToPerToken(outRow.offPeak[1]),
			PeakMultiplier:       ratioOr(hitRow.peak[1], hitRow.offPeak[1], 2),
		},
	}
	snap := &DeepSeekOfficialPricing{
		FetchedAt: now,
		Currencies: map[string]DeepSeekCurrencyRates{
			currency: {Currency: currency, SourceURL: src, FetchedAt: now, Models: models},
		},
	}

	tz, weekdaysOnly, windows := parseDeepSeekPeakWindows(raw)
	snap.Timezone = tz
	snap.WeekdaysOnly = weekdaysOnly
	snap.PeakWindows = windows
	return snap, nil
}

var (
	deepSeekCacheHitLabels  = []string{"缓存命中", "CACHE HIT"}
	deepSeekCacheMissLabels = []string{"缓存未命中", "CACHE MISS"}
	deepSeekOutputLabels    = []string{"tokens输出", "OUTPUT TOKENS"}
	deepSeekOffPeakLabels   = []string{"空闲时段", "OFF-PEAK", "OFF PEAK"}
	deepSeekPeakLabels      = []string{"高峰时段", "PEAK"}
)

type deepSeekPriceRow struct {
	offPeak [2]float64
	peak    [2]float64
}

func (r deepSeekPriceRow) valid() bool {
	return r.offPeak[0] > 0 && r.offPeak[1] > 0 && r.peak[0] > 0 && r.peak[1] > 0
}

// deepSeekFindPriceRow 定位价格行，并读取「空闲时段/高峰时段」各两列数值。
func deepSeekFindPriceRow(cells []string, labels []string) (deepSeekPriceRow, error) {
	var row deepSeekPriceRow
	start := -1
	for i, c := range cells {
		if containsAnyFold(c, labels) {
			start = i
			break
		}
	}
	if start < 0 {
		return row, fmt.Errorf("row label not found")
	}
	mode := ""
	for i := start + 1; i < len(cells) && i < start+12; i++ {
		c := cells[i]
		if containsAnyFold(c, deepSeekOffPeakLabels) {
			mode = "off"
			continue
		}
		if containsAnyFold(c, deepSeekPeakLabels) {
			mode = "peak"
			continue
		}
		if containsAnyFold(c, deepSeekCacheHitLabels) || containsAnyFold(c, deepSeekCacheMissLabels) ||
			containsAnyFold(c, deepSeekOutputLabels) || strings.Contains(c, "并发") || strings.Contains(c, "CONCURRENCY") {
			break
		}
		if mode == "" {
			continue
		}
		value, ok := parseDeepSeekMoney(c)
		if !ok {
			continue
		}
		if mode == "off" {
			if row.offPeak[0] == 0 {
				row.offPeak[0] = value
			} else if row.offPeak[1] == 0 {
				row.offPeak[1] = value
			}
		} else {
			if row.peak[0] == 0 {
				row.peak[0] = value
			} else if row.peak[1] == 0 {
				row.peak[1] = value
			}
		}
	}
	if !row.valid() {
		return row, fmt.Errorf("incomplete prices: off=%v peak=%v", row.offPeak, row.peak)
	}
	return row, nil
}

// parseDeepSeekPeakWindows 解析峰谷时段说明（含时区与是否仅工作日）。
func parseDeepSeekPeakWindows(raw string) (string, bool, []DeepSeekPeakWindow) {
	text := deepSeekTagRe.ReplaceAllString(raw, " ")
	text = strings.Join(strings.Fields(text), " ")

	timezone := "Asia/Shanghai"
	weekdaysOnly := true
	idx := -1
	for _, marker := range []string{"高峰时段为", "Peak hours are"} {
		if i := strings.Index(text, marker); i >= 0 {
			idx = i
			break
		}
	}
	if idx < 0 {
		return timezone, weekdaysOnly, nil
	}
	segment := text[idx:]
	if cut := strings.IndexAny(segment, "。（(；;"); cut > 0 {
		segment = segment[:cut]
	}
	if strings.Contains(segment, "UTC") {
		timezone = "UTC"
	}
	if strings.Contains(segment, "周一至周五") || strings.Contains(strings.ToLower(segment), "monday through friday") ||
		strings.Contains(strings.ToLower(segment), "weekday") {
		weekdaysOnly = true
	}
	windows := make([]DeepSeekPeakWindow, 0, 2)
	for _, m := range deepSeekWindowRe.FindAllStringSubmatch(segment, -1) {
		windows = append(windows, DeepSeekPeakWindow{
			Start: normalizeClock(m[1]),
			End:   normalizeClock(m[2]),
		})
		if len(windows) == 2 {
			break
		}
	}
	return timezone, weekdaysOnly, windows
}

func normalizeClock(v string) string {
	parts := strings.SplitN(v, ":", 2)
	if len(parts) != 2 {
		return v
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return v
	}
	return fmt.Sprintf("%02d:%s", hour, parts[1])
}

func cleanDeepSeekCell(raw string) string {
	// <sup>(1)</sup> 等脚注标记换成空格，避免污染数值与标签匹配
	s := strings.ReplaceAll(raw, "</sup>", " ")
	s = deepSeekTagRe.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&yen;", "¥")
	s = strings.ReplaceAll(s, "&#165;", "¥")
	s = strings.ReplaceAll(s, "&amp;", "&")
	return strings.Join(strings.Fields(s), " ")
}

// detectDeepSeekCurrency 仅识别价格单元格的货币标记（前缀 ¥/￥/$ 或后缀「元」），
// 避免页面其它文案（语言切换、脚注等）造成误判。
func detectDeepSeekCurrency(cells []string) string {
	for _, c := range cells {
		s := strings.TrimSpace(c)
		if s == "" {
			continue
		}
		if strings.HasPrefix(s, "¥") || strings.HasPrefix(s, "￥") || strings.HasSuffix(s, "元") {
			return "CNY"
		}
		if strings.HasPrefix(s, "$") || strings.HasPrefix(s, "US$") {
			return "USD"
		}
	}
	return "CNY"
}

// parseDeepSeekMoney 解析 "1元" / "￥1.5" / "$0.15" 形式的金额。
func parseDeepSeekMoney(cell string) (float64, bool) {
	s := strings.TrimSpace(cell)
	if s == "" {
		return 0, false
	}
	if strings.Contains(s, "元") || strings.Contains(s, "¥") || strings.Contains(s, "￥") || strings.Contains(s, "$") {
		m := deepSeekNumberRe.FindStringSubmatch(strings.ReplaceAll(s, ",", ""))
		if len(m) < 2 {
			return 0, false
		}
		v, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			return 0, false
		}
		return v, true
	}
	return 0, false
}

func perMillionToPerToken(v float64) float64 {
	return v / 1e6
}

func ratioOr(peak, offPeak, fallback float64) float64 {
	if offPeak <= 0 || peak <= 0 {
		return fallback
	}
	ratio := peak / offPeak
	if ratio < 1.5 || ratio > 4 {
		return fallback
	}
	return ratio
}

func containsAnyFold(value string, needles []string) bool {
	upper := strings.ToUpper(value)
	for _, n := range needles {
		if strings.Contains(upper, strings.ToUpper(n)) {
			return true
		}
	}
	return false
}

// normalizeDeepSeekFamily 归一模型族：deepseek-v4-pro/…-pro-0813 → pro，其余 → flash。
func normalizeDeepSeekFamily(model string) string {
	lower := strings.ToLower(strings.TrimSpace(model))
	if strings.Contains(lower, "pro") {
		return "pro"
	}
	return "flash"
}

// validateDeepSeekOfficialPricing 自检：防止解析错误（页面改版/抓取到异常内容）污染计费。
func validateDeepSeekOfficialPricing(snap *DeepSeekOfficialPricing) error {
	if snap == nil {
		return fmt.Errorf("nil snapshot")
	}
	if len(snap.Currencies) == 0 {
		return fmt.Errorf("no currencies parsed")
	}
	for currency, rates := range snap.Currencies {
		if err := validateDeepSeekCurrencyRates(currency, rates); err != nil {
			return err
		}
	}
	return validateDeepSeekPeakWindows(snap.Timezone, snap.PeakWindows)
}

// validateDeepSeekCurrencyRates 单币种价格自检：模型族齐备、价格在合理区间、
// 缓存命中价不高于未命中价、峰谷倍率可信。
func validateDeepSeekCurrencyRates(currency string, rates DeepSeekCurrencyRates) error {
	normalized := normalizeDeepSeekCurrency(currency)
	if normalized == "" {
		return fmt.Errorf("empty currency")
	}
	if normalizeDeepSeekCurrency(rates.Currency) != normalized {
		return fmt.Errorf("currency mismatch: key=%q field=%q", currency, rates.Currency)
	}
	if len(rates.Models) == 0 {
		return fmt.Errorf("currency %q: no models parsed", currency)
	}
	for _, family := range []string{"flash", "pro"} {
		rate, ok := rates.Models[family]
		if !ok {
			return fmt.Errorf("currency %q: missing model family %q", currency, family)
		}
		if rate.InputOffPeak <= 0 || rate.OutputOffPeak <= 0 {
			return fmt.Errorf("currency %q family %q: non-positive price", currency, family)
		}
		// 合理性区间：单价 (货币/M) 应在 0.001 ~ 1000 之间
		if perTokenToMillion(rate.InputOffPeak) < 0.001 || perTokenToMillion(rate.InputOffPeak) > 1000 ||
			perTokenToMillion(rate.OutputOffPeak) < 0.001 || perTokenToMillion(rate.OutputOffPeak) > 1000 {
			return fmt.Errorf("currency %q family %q: price out of sane range", currency, family)
		}
		if rate.InputCacheHitOffPeak > rate.InputOffPeak {
			return fmt.Errorf("currency %q family %q: cache-hit price exceeds cache-miss price", currency, family)
		}
		if rate.PeakMultiplier < 1 || rate.PeakMultiplier > 8 {
			return fmt.Errorf("currency %q family %q: implausible peak multiplier %v", currency, family, rate.PeakMultiplier)
		}
	}
	return nil
}

// validateDeepSeekPeakWindows 校验峰谷时段与所属时区（时段与币种无关）。
func validateDeepSeekPeakWindows(timezone string, windows []DeepSeekPeakWindow) error {
	if len(windows) == 0 {
		return fmt.Errorf("no peak windows parsed")
	}
	for _, w := range windows {
		if _, _, err := parseClockRange(w.Start, w.End); err != nil {
			return fmt.Errorf("invalid peak window %s-%s: %w", w.Start, w.End, err)
		}
	}
	if strings.TrimSpace(timezone) == "" {
		return fmt.Errorf("empty timezone")
	}
	return nil
}

func perTokenToMillion(v float64) float64 { return v * 1e6 }

func parseClockRange(start, end string) (int, int, error) {
	parse := func(v string) (int, error) {
		parts := strings.SplitN(v, ":", 2)
		if len(parts) != 2 {
			return 0, fmt.Errorf("bad clock %q", v)
		}
		h, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, err
		}
		m, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, err
		}
		if h < 0 || h > 24 || m < 0 || m > 59 {
			return 0, fmt.Errorf("out of range clock %q", v)
		}
		return h*60 + m, nil
	}
	s, err := parse(start)
	if err != nil {
		return 0, 0, err
	}
	e, err := parse(end)
	if err != nil {
		return 0, 0, err
	}
	if e <= s {
		return 0, 0, fmt.Errorf("end not after start")
	}
	return s, e, nil
}

// ---------------------------------------------------------------------------
// 落盘 / 载入
// ---------------------------------------------------------------------------

func loadDeepSeekPricingSnapshot(path string) *DeepSeekOfficialPricing {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var snap DeepSeekOfficialPricing
	if err := json.Unmarshal(body, &snap); err != nil {
		logger.LegacyPrintf(deepSeekPricingLogScope,
			"[DeepSeekPricing] snapshot decode failed: %v", err)
		return nil
	}
	// v1 单币种快照在读取时迁移为多币种结构。
	snap.migrateLegacyCurrencies()
	if validateDeepSeekOfficialPricing(&snap) != nil {
		return nil
	}
	return &snap
}

func saveDeepSeekPricingSnapshot(path string, snap *DeepSeekOfficialPricing) error {
	if strings.TrimSpace(path) == "" || snap == nil {
		return nil
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	body, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func deepSeekPricingEqual(a, b *DeepSeekOfficialPricing) bool {
	if a == nil || b == nil {
		return a == b
	}
	aj, err1 := json.Marshal(a)
	bj, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && string(aj) == string(bj)
}

func describeDeepSeekRates(snap *DeepSeekOfficialPricing) string {
	if snap == nil {
		return ""
	}
	parts := make([]string, 0, len(snap.Currencies))
	for currency, rates := range snap.Currencies {
		for family, rate := range rates.Models {
			parts = append(parts, fmt.Sprintf("%s/%s=%.4f/%.4f/%.4f per-M",
				currency, family, perTokenToMillion(rate.InputOffPeak),
				perTokenToMillion(rate.InputCacheHitOffPeak), perTokenToMillion(rate.OutputOffPeak)))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

// describeDeepSeekCurrencies 列出快照内已同步的币种（日志用）。
func describeDeepSeekCurrencies(snap *DeepSeekOfficialPricing) string {
	if snap == nil {
		return ""
	}
	currencies := make([]string, 0, len(snap.Currencies))
	for currency := range snap.Currencies {
		currencies = append(currencies, currency)
	}
	sort.Strings(currencies)
	return strings.Join(currencies, ",")
}

// ---------------------------------------------------------------------------
// 计费热路径访问器
// ---------------------------------------------------------------------------

// lookupDeepSeekOfficialRate 返回与站点展示币种口径一致的官方单价快照。
//
// 站点币种决定取哪一套官方数字（中文页人民币 / 英文页美元）。没有对应币种的
// 快照时 ok=false：调用方不得改用其它币种的数字（避免跨币种误用），由
// applyDeepSeekOfficialPricing 决定是否回退内置常量或保持上游价卡。
func lookupDeepSeekOfficialRate(model string) (DeepSeekModelRate, DeepSeekOfficialPricing, bool) {
	snap := deepSeekOfficial.Load()
	if snap == nil {
		return DeepSeekModelRate{}, DeepSeekOfficialPricing{}, false
	}
	rates, ok := snap.ratesFor(deepSeekSiteCurrency())
	if !ok {
		return DeepSeekModelRate{}, *snap, false
	}
	rate, ok := rates.Models[normalizeDeepSeekFamily(model)]
	if !ok {
		return DeepSeekModelRate{}, *snap, false
	}
	return rate, *snap, true
}

// deepSeekOfficialPeakWindows 返回同步到的峰谷时段（未同步时 ok=false）。
func deepSeekOfficialPeakWindows() (string, bool, []DeepSeekPeakWindow, bool) {
	snap := deepSeekOfficial.Load()
	if snap == nil || len(snap.PeakWindows) == 0 {
		return "", true, nil, false
	}
	return snap.Timezone, snap.WeekdaysOnly, snap.PeakWindows, true
}
