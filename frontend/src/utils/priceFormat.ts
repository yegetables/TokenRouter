// 价格展示格式化：模型广场卡片与价格面板共用，避免两处口径漂移。
//
// 规则（不四舍五入，只去尾零）：
//   - 小数位 ≥ 2 时保留有效数字，尾部多余的 0 去掉（0.025 → 0.025，0.0205015 → 0.0205015）
//   - 小数位 < 2 时补足两位（0.1 → 0.10，2 → 2.00）
//   - 若按“两位小数”直接舍入，会把非零位抹掉导致价格显示错误
//     （0.05054 舍成 0.05、0.003 舍成 0.00），因此这里以价格正确优先。

/** 展示下限：不足两位小数补零。 */
const PRICE_MIN_FRACTION_DIGITS = 2

/**
 * 展示上限，仅用于消除浮点误差（如 ¥0.02/M 存为 per-token 2e-8，
 * 乘回 1e6 得到 0.020000000000000004），不影响计价精度本身。
 */
const PRICE_MAX_FRACTION_DIGITS = 10

/**
 * 格式化价格数字（不含货币单位）。
 *
 * 示例：0.01 → 0.01 ｜ 0.1 → 0.10 ｜ 0.025 → 0.025 ｜ 0.05054 → 0.05054
 *       0.0205015 → 0.0205015 ｜ 2 → 2.00 ｜ 0 → 0.00
 */
export function formatPriceNumber(value: number): string {
  return new Intl.NumberFormat(undefined, {
    minimumFractionDigits: PRICE_MIN_FRACTION_DIGITS,
    maximumFractionDigits: PRICE_MAX_FRACTION_DIGITS
  }).format(value)
}
