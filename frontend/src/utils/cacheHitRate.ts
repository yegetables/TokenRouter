/**
 * 缓存命中率口径工具。
 *
 * 命中率 = 缓存读取 / 输入侧总量，其中输入侧总量 = 输入 + 缓存创建 + 缓存读取。
 * 该口径与 TokenUsageTrend 的 Cached Input % 曲线一致；三处以上复用，
 * 集中在此避免多处实现漂移。
 */

/** 输入侧总量：命中率分母。 */
export function cacheHitRateDenominator(
  inputTokens: number,
  cacheCreationTokens: number,
  cacheReadTokens: number,
): number {
  return (inputTokens || 0) + (cacheCreationTokens || 0) + (cacheReadTokens || 0)
}

/**
 * 计算缓存命中率（百分比）。
 * 分母为 0 时返回 null，调用方应显示"不适用"而不是 0%。
 */
export function computeCacheHitRate(
  inputTokens: number,
  cacheCreationTokens: number,
  cacheReadTokens: number,
): number | null {
  const denominator = cacheHitRateDenominator(inputTokens, cacheCreationTokens, cacheReadTokens)
  if (denominator <= 0) return null
  return ((cacheReadTokens || 0) / denominator) * 100
}

/**
 * 上游是否回报了缓存创建（写入）token。
 *
 * 多数 OpenAI 兼容中转不返回缓存写入字段，`cache_creation_tokens` 恒为 0，
 * 但 `cache_read_tokens` 有值。此时创建值属于"不适用"，
 * 不能当作故障性的 0 展示（参见上游 issue #4126）。
 */
export function hasCacheCreationData(cacheCreationTokens: number): boolean {
  return (cacheCreationTokens || 0) > 0
}

/**
 * 该行是否完全没有任何缓存数据（创建与读取都为 0）。
 * 用于区分"上游不回报"与"确实没有缓存活动"。
 */
export function hasAnyCacheData(cacheCreationTokens: number, cacheReadTokens: number): boolean {
  return (cacheCreationTokens || 0) > 0 || (cacheReadTokens || 0) > 0
}
