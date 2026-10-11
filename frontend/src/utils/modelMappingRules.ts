/** ModelMappingRow 是映射编辑器的一行，来源与目标都保留用户输入的原文。 */
export interface ModelMappingRow {
  from: string
  to: string
}

/** ModelMappingIssue 与 keys.modelRedirect 下的文案键同名，调用方可直接拼接翻译键。 */
export type ModelMappingIssue =
  | 'sourceRequired'
  | 'targetRequired'
  | 'nameTooLong'
  | 'sourceWildcardInvalid'
  | 'targetWildcardInvalid'
  | 'selfMapping'
  | 'duplicateSource'

/** ModelMappingRowIssues 记录一行中来源和目标各自的第一个问题。 */
export interface ModelMappingRowIssues {
  from?: ModelMappingIssue
  to?: ModelMappingIssue
}

/** ModelMappingRuleOptions 描述站点需要开启的检查项，未开启的检查不会执行。 */
export interface ModelMappingRuleOptions {
  required?: boolean
  maxLength?: number
  sourceWildcard?: 'trailing'
  forbidTargetWildcard?: boolean
  forbidSelfMapping?: boolean
  uniqueSource?: boolean
}

/** KEY_REDIRECT_RULES 与后端 API Key 模型重定向的约束保持一致。 */
export const KEY_REDIRECT_RULES: ModelMappingRuleOptions = {
  required: true,
  maxLength: 100,
  sourceWildcard: 'trailing',
  forbidTargetWildcard: true,
  forbidSelfMapping: true,
  uniqueSource: true,
}

/** WILDCARD_ONLY_RULES 只提示通配符位置，用于原本只校验通配符的映射。 */
export const WILDCARD_ONLY_RULES: ModelMappingRuleOptions = {
  sourceWildcard: 'trailing',
  forbidTargetWildcard: true,
}

/** isValidWildcardPattern 校验通配符格式：* 最多一个且只能放在末尾。 */
export function isValidWildcardPattern(pattern: string): boolean {
  const starIndex = pattern.indexOf('*')
  if (starIndex === -1) return true
  return starIndex === pattern.length - 1 && pattern.lastIndexOf('*') === starIndex
}

// 模型名长度按码点计算，与后端的 rune 计数一致。
const exceedsLength = (value: string, maxLength?: number) =>
  maxLength !== undefined && [...value].length > maxLength

// 来源依次检查必填、长度、通配符和重复，只保留第一个问题。
function sourceIssue(
  source: string,
  options: ModelMappingRuleOptions,
  sourceCounts: Map<string, number>,
): ModelMappingIssue | undefined {
  if (!source) return options.required ? 'sourceRequired' : undefined
  if (exceedsLength(source, options.maxLength)) return 'nameTooLong'
  if (options.sourceWildcard === 'trailing' && !isValidWildcardPattern(source)) {
    return 'sourceWildcardInvalid'
  }
  if (options.uniqueSource && (sourceCounts.get(source) ?? 0) > 1) {
    return 'duplicateSource'
  }
  return undefined
}

// 目标依次检查必填、长度、通配符和自映射，只保留第一个问题。
function targetIssue(
  source: string,
  target: string,
  options: ModelMappingRuleOptions,
): ModelMappingIssue | undefined {
  if (!target) return options.required ? 'targetRequired' : undefined
  if (exceedsLength(target, options.maxLength)) return 'nameTooLong'
  if (options.forbidTargetWildcard && target.includes('*')) {
    return 'targetWildcardInvalid'
  }
  if (options.forbidSelfMapping && source && source === target) {
    return 'selfMapping'
  }
  return undefined
}

/**
 * validateModelMappingRows 按行校验映射，返回结果与 rows 按下标对齐。
 * 比较前会去掉首尾空白，大小写保持敏感。
 */
export function validateModelMappingRows(
  rows: ModelMappingRow[],
  options: ModelMappingRuleOptions = {},
): ModelMappingRowIssues[] {
  const sourceCounts = new Map<string, number>()
  for (const row of rows) {
    const source = row.from.trim()
    if (source) sourceCounts.set(source, (sourceCounts.get(source) ?? 0) + 1)
  }

  return rows.map((row) => {
    const source = row.from.trim()
    const target = row.to.trim()
    const issues: ModelMappingRowIssues = {}
    const fromIssue = sourceIssue(source, options, sourceCounts)
    const toIssue = targetIssue(source, target, options)
    if (fromIssue) issues.from = fromIssue
    if (toIssue) issues.to = toIssue
    return issues
  })
}

/** firstModelMappingIssue 按行序返回第一个问题，同一行先报来源。 */
export function firstModelMappingIssue(
  issues: ModelMappingRowIssues[],
): ModelMappingIssue | null {
  for (const issue of issues) {
    if (issue.from) return issue.from
    if (issue.to) return issue.to
  }
  return null
}

/** mappingRowsToRecord 把编辑行转换为提交用的映射对象，键和值都去掉首尾空白。 */
export function mappingRowsToRecord(rows: ModelMappingRow[]): Record<string, string> {
  return Object.fromEntries(rows.map((row) => [row.from.trim(), row.to.trim()]))
}

/** recordToMappingRows 把映射对象展开为编辑行，保持对象的键顺序。 */
export function recordToMappingRows(
  mapping: Record<string, string> | null | undefined,
): ModelMappingRow[] {
  return Object.entries(mapping ?? {}).map(([from, to]) => ({ from, to }))
}

/**
 * overwriteSelfMappings 用上游型号覆盖映射里的自映射，保留 from≠to 的自定义改写。
 * 上游型号与自定义改写的来源重名时保留自定义改写，空来源的行不进入结果。
 */
export function overwriteSelfMappings(
  rows: ModelMappingRow[],
  upstreamModels: string[],
): ModelMappingRow[] {
  const customMappings = rows.filter((row) => row.from.trim().length > 0 && row.from !== row.to)
  const customSources = new Set(customMappings.map((row) => row.from))
  const selfMappings = upstreamModels
    .filter((model) => !customSources.has(model))
    .map((model) => ({ from: model, to: model }))
  return [...selfMappings, ...customMappings]
}
