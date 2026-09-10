// 价格展示格式化：模型广场卡片与价格面板共用，避免两处口径漂移。
//
// 历史上这两处的 formatPriceNumber 按数值大小切换精度
// （≥1 用 2 位、<1 用 4 位、<0.01 用 6 位），导致同一列出现
// “2.00 / 8.00” 与 “0.8000 / 0.0400 / 0.000000” 混排。

/** 价格展示统一保留的小数位。 */
export const PRICE_FRACTION_DIGITS = 2

/** 两位小数会把非零价格显示成 0.00 时使用的备用精度，避免极小单价被展示成免费。 */
const TINY_PRICE_FRACTION_DIGITS = 6

/**
 * 格式化价格数字（不含货币单位）：统一保留两位小数。
 *
 * 仅当两位小数会把**非零**价格显示成 0.00 时才保留更多精度；零值固定显示 0.00。
 */
export function formatPriceNumber(value: number): string {
  const abs = Math.abs(value)
  const digits = abs > 0 && abs < 0.005 ? TINY_PRICE_FRACTION_DIGITS : PRICE_FRACTION_DIGITS

  return new Intl.NumberFormat(undefined, {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits
  }).format(value)
}
