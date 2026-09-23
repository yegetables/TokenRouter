import type { PricingFormEntry } from './types'
import { createDefaultTimePricingForm } from './types'

export interface AccountModelSyncDiff {
  /** 并集中渠道价卡没有的模型（新增，显式填 0） */
  added: string[]
  /** 并集中渠道已有配置的模型（复用原价卡） */
  reused: string[]
  /** 渠道价卡有、但并集中没有的模型（将被移除） */
  removed: string[]
}

// createEmptyPricingEntry 生成一条空的定价条目（价格留空，由调用方按需填充）。
export function createEmptyPricingEntry(): PricingFormEntry {
  return {
    models: [],
    billing_mode: 'token',
    price_multiplier: null,
    fast_mode_multiplier: null,
    fast_multiplier: null,
    flex_multiplier: null,
    max_reasoning_effort_multiplier: null,
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_write_1h_price: null,
    cache_read_price: null,
    image_input_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: [],
    time_pricing: createDefaultTimePricingForm()
  }
}

// looksLikeImageModelName 按名称粗判生图模型，仅用于给新增条目选默认计费模式。
export function looksLikeImageModelName(model: string): boolean {
  return /image/i.test(model)
}

// normalizeModelNames 去空白、去空值并去重，保持首次出现顺序。
export function normalizeModelNames(models: Iterable<string>): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const raw of models) {
    const model = String(raw).trim()
    if (!model || seen.has(model)) continue
    seen.add(model)
    out.push(model)
  }
  return out
}

// diffAccountModels 对比账号模型并集与渠道当前模型，得出新增/复用/将移除三组。
export function diffAccountModels(union: Iterable<string>, currentModels: Iterable<string>): AccountModelSyncDiff {
  const unionList = normalizeModelNames(union)
  const currentList = normalizeModelNames(currentModels)
  const unionSet = new Set(unionList)
  const currentSet = new Set(currentList)
  return {
    added: unionList.filter(model => !currentSet.has(model)),
    reused: unionList.filter(model => currentSet.has(model)),
    removed: currentList.filter(model => !unionSet.has(model))
  }
}

// applyAccountModelsToPricing 用并集覆盖渠道价卡：
// - 并集内已有条目保留原价格；同一价卡里混合了并集外模型时按模型拆分，保留并集内部分及其价格。
// - 新增模型显式填 0（生图模型用 image 模式 + 每张 0），避免留空回退内置目录价。
// - 并集外的模型连同其价卡一并移除。
export function applyAccountModelsToPricing(pricing: PricingFormEntry[], union: Iterable<string>): PricingFormEntry[] {
  const unionSet = new Set(normalizeModelNames(union))

  const kept: PricingFormEntry[] = []
  for (const entry of pricing) {
    const inside = entry.models.filter(model => unionSet.has(model.trim()))
    const outside = entry.models.filter(model => !unionSet.has(model.trim()))
    if (inside.length === 0) continue
    kept.push(outside.length === 0 ? entry : { ...entry, models: inside })
  }

  const covered = new Set(kept.flatMap(entry => entry.models.map(model => model.trim())))
  const newModels = [...unionSet].filter(model => !covered.has(model))
  const newTokenModels = newModels.filter(model => !looksLikeImageModelName(model))
  const newImageModels = newModels.filter(model => looksLikeImageModelName(model))

  if (newTokenModels.length > 0) {
    kept.push({
      ...createEmptyPricingEntry(),
      models: newTokenModels,
      input_price: 0,
      output_price: 0,
      cache_write_price: 0,
      cache_read_price: 0
    })
  }
  if (newImageModels.length > 0) {
    kept.push({
      ...createEmptyPricingEntry(),
      models: newImageModels,
      billing_mode: 'image',
      per_request_price: 0
    })
  }
  return kept
}

export interface UpstreamModelRefreshOutcome {
  /** 成功账号的模型并集 */
  union: string[]
  /** 获取失败的账号名（已忽略，不参与并集） */
  failedNames: string[]
}

// refreshAccountModelsConcurrently 并发刷新各账号的上游模型：
// 失效账号忽略、不参与并集；只有全部失败时并集才为空。
export async function refreshAccountModelsConcurrently(
  accounts: Array<{ id: number; name: string }>,
  refresh: (id: number) => Promise<{ models?: string[] | null }>
): Promise<UpstreamModelRefreshOutcome> {
  const results = await Promise.allSettled(accounts.map(account => refresh(account.id)))
  const union = new Set<string>()
  const failedNames: string[] = []
  results.forEach((result, index) => {
    if (result.status === 'fulfilled') {
      for (const model of normalizeModelNames(result.value?.models ?? [])) union.add(model)
    } else {
      failedNames.push(accounts[index].name)
    }
  })
  return { union: [...union], failedNames }
}
