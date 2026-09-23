import { describe, expect, it } from 'vitest'

import {
  applyAccountModelsToPricing,
  createEmptyPricingEntry,
  diffAccountModels,
  looksLikeImageModelName,
  normalizeModelNames
} from '../accountModelSync'
import type { PricingFormEntry } from '../types'

function tokenEntry(models: string[], inputPrice: number | null = 8): PricingFormEntry {
  return {
    ...createEmptyPricingEntry(),
    models,
    input_price: inputPrice,
    output_price: inputPrice === null ? null : inputPrice * 3
  }
}

describe('normalizeModelNames', () => {
  it('去空白、去空值、去重并保持首次出现顺序', () => {
    expect(normalizeModelNames([' b ', 'a', '', '  ', 'b', 'a'])).toEqual(['b', 'a'])
  })
})

describe('looksLikeImageModelName', () => {
  it('按名称粗判生图模型', () => {
    expect(looksLikeImageModelName('gpt-image-2')).toBe(true)
    expect(looksLikeImageModelName('gemini-3.1-flash-lite-image')).toBe(true)
    expect(looksLikeImageModelName('qwen-image-2.0')).toBe(true)
    expect(looksLikeImageModelName('wan2.7-image')).toBe(true)
    expect(looksLikeImageModelName('deepseek-flash')).toBe(false)
  })
})

describe('diffAccountModels', () => {
  it('区分新增、复用与将移除三组', () => {
    const diff = diffAccountModels(['a', 'e', 'g'], ['a', 'b', 'c'])
    expect(diff.added).toEqual(['e', 'g'])
    expect(diff.reused).toEqual(['a'])
    expect(diff.removed).toEqual(['b', 'c'])
  })

  it('忽略空白与重复项', () => {
    const diff = diffAccountModels([' a ', 'a', 'b'], ['b', 'b', ' '])
    expect(diff.added).toEqual(['a'])
    expect(diff.reused).toEqual(['b'])
    expect(diff.removed).toEqual([])
  })
})

describe('applyAccountModelsToPricing', () => {
  it('覆盖：保留已有价格、新增填 0、并集外移除', () => {
    const pricing = [tokenEntry(['a', 'b'], 8), tokenEntry(['c'], 5)]
    const result = applyAccountModelsToPricing(pricing, ['a', 'e'])

    // a 保留原价 8；b、c 不在并集 → 移除；e 新增 → 填 0。
    const kept = result.find(entry => entry.models.includes('a'))
    expect(kept?.input_price).toBe(8)

    expect(result.some(entry => entry.models.includes('b'))).toBe(false)
    expect(result.some(entry => entry.models.includes('c'))).toBe(false)

    const added = result.find(entry => entry.models.includes('e'))
    expect(added).toBeTruthy()
    expect(added?.billing_mode).toBe('token')
    expect(added?.input_price).toBe(0)
    expect(added?.output_price).toBe(0)
    expect(added?.cache_write_price).toBe(0)
    expect(added?.cache_read_price).toBe(0)
  })

  it('混合条目按模型拆分并保留并集内部分的价格', () => {
    const pricing = [tokenEntry(['a', 'stale'], 8)]
    const result = applyAccountModelsToPricing(pricing, ['a'])

    expect(result).toHaveLength(1)
    expect(result[0].models).toEqual(['a'])
    expect(result[0].input_price).toBe(8)
  })

  it('新增生图模型用 image 模式 + 每张 0', () => {
    const result = applyAccountModelsToPricing([], ['gpt-image-2', 'deepseek-flash'])

    const image = result.find(entry => entry.models.includes('gpt-image-2'))
    expect(image?.billing_mode).toBe('image')
    expect(image?.per_request_price).toBe(0)
    expect(image?.input_price).toBeNull()

    const token = result.find(entry => entry.models.includes('deepseek-flash'))
    expect(token?.billing_mode).toBe('token')
    expect(token?.input_price).toBe(0)
  })

  it('并集为空时清空全部价卡', () => {
    expect(applyAccountModelsToPricing([tokenEntry(['a'], 8)], [])).toEqual([])
  })
})
