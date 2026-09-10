// 价格展示格式化：模型广场卡片与价格面板共用，避免两处口径漂移。
//
// 规则（不四舍五入，只去尾零）：
//   - 有效数字按值原样保留，只去掉尾部多余的 0；小数位不足两位时补足两位
//     （0.025 → 0.025，0.05054 → 0.05054，0.0205015 → 0.0205015，0.1 → 0.10，2 → 2.00）
//   - 绝不按“两位小数”直接舍入：0.05054 舍成 0.05、0.003 舍成 0.00 都属于价格错误
//   - 精度上限只用于两点：消除浮点噪声（¥0.02/M 存为 per-token 2e-8，乘回 1e6
//     得到 0.020000000000000004）与防止极小值产生超长输出；真实有效数字不丢
//
// 注意：不能按“小数位”设上限。按小数位截断会让小单价严重失真
// （4.1666e-8 只留 3 位有效数字、8.33e-10 只剩 1 位），因此这里按“有效数字”计算。

/** 展示下限：不足两位小数补零。 */
const PRICE_MIN_FRACTION_DIGITS = 2

/** 保留的有效数字位数（约等于 double 精度上限；用于消除浮点噪声而不丢真实数字）。 */
const PRICE_SIGNIFICANT_DIGITS = 15

/** 小数位兜底上限，避免极小值输出过长（约 1e-24 起才触及）。 */
const PRICE_MAX_FRACTION_DIGITS = 24

/**
 * 格式化价格数字（不含货币单位）。
 *
 * 示例：0.01 → 0.01 ｜ 0.1 → 0.10 ｜ 0.025 → 0.025 ｜ 0.05054 → 0.05054
 *       0.0205015 → 0.0205015 ｜ 2 → 2.00 ｜ 0 → 0.00
 *       2 ÷ 7.2 → 0.277777777777778（保留真实有效数字，不截断到 2 位小数）
 */
export function formatPriceNumber(value: number): string {
  const abs = Math.abs(value)
  let fractionDigits = PRICE_MIN_FRACTION_DIGITS
  if (Number.isFinite(abs) && abs > 0) {
    const exponent = Math.floor(Math.log10(abs))
    fractionDigits = Math.min(
      PRICE_MAX_FRACTION_DIGITS,
      Math.max(PRICE_MIN_FRACTION_DIGITS, PRICE_SIGNIFICANT_DIGITS - 1 - exponent)
    )
  }

  return new Intl.NumberFormat(undefined, {
    minimumFractionDigits: PRICE_MIN_FRACTION_DIGITS,
    maximumFractionDigits: fractionDigits
  }).format(value)
}
