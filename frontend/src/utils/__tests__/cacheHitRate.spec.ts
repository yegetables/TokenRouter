import { describe, expect, it } from 'vitest'

import {
  cacheHitRateDenominator,
  computeCacheHitRate,
  hasAnyCacheData,
  hasCacheCreationData,
} from '../cacheHitRate'

describe('cacheHitRate', () => {
  it('分母 = 输入 + 缓存创建 + 缓存读取', () => {
    expect(cacheHitRateDenominator(100, 200, 700)).toBe(1000)
  })

  it('命中率按缓存读取占输入侧总量计算', () => {
    // 真实部署样本：deepseek-flash，creation 恒为 0
    expect(computeCacheHitRate(28_679_273, 0, 1_826_891_006)).toBeCloseTo(98.45, 1)
  })

  it('分母为 0 返回 null 而不是 0（用于显示"不适用"）', () => {
    expect(computeCacheHitRate(0, 0, 0)).toBeNull()
    expect(computeCacheHitRate(-1, 0, 0)).toBeNull()
  })

  it('缓存写入未被上游回报时 hasCacheCreationData 为 false', () => {
    // OpenAI 兼容中转不返回缓存写入字段，但读取有值。
    expect(hasCacheCreationData(0)).toBe(false)
    expect(hasCacheCreationData(120)).toBe(true)
  })

  it('完全没有缓存活动时 hasAnyCacheData 为 false', () => {
    expect(hasAnyCacheData(0, 0)).toBe(false)
    expect(hasAnyCacheData(0, 500)).toBe(true)
    expect(hasAnyCacheData(500, 0)).toBe(true)
  })
})
