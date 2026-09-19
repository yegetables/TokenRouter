import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import ModelUsageCacheTable from '../ModelUsageCacheTable.vue'
import type { ModelStat } from '@/types'

const messages: Record<string, string> = {
  'usage.modelUsageCache.title': 'Usage & Cache',
  'usage.modelUsageCache.model': 'Model',
  'usage.modelUsageCache.requests': 'Requests',
  'usage.modelUsageCache.input': 'Input',
  'usage.modelUsageCache.cache': 'Cache',
  'usage.modelUsageCache.cacheRate': 'Cache rate',
  'usage.modelUsageCache.output': 'Output',
  'usage.modelUsageCache.quota': 'Quota',
  'usage.modelUsageCache.notApplicable': 'N/A',
  'usage.modelUsageCache.creationNotReported': 'creation not reported',
  'usage.modelUsageCache.cacheRateHint': 'hit rate hint',
  'admin.dashboard.noDataAvailable': 'No data available',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

const modelStat = (overrides: Partial<ModelStat>): ModelStat => ({
  model: 'model-a',
  requests: 1,
  input_tokens: 0,
  output_tokens: 0,
  cache_creation_tokens: 0,
  cache_read_tokens: 0,
  total_tokens: 0,
  cost: 0,
  actual_cost: 0,
  ...overrides,
})

describe('ModelUsageCacheTable', () => {
  it('按缓存读取占输入侧总量的比例计算缓存率', () => {
    // 命中率 = 缓存读取 /（输入 + 缓存创建 + 缓存读取）
    const wrapper = mount(ModelUsageCacheTable, {
      props: {
        models: [
          modelStat({
            model: 'deepseek',
            requests: 100,
            input_tokens: 250,
            cache_creation_tokens: 250,
            cache_read_tokens: 9500,
            output_tokens: 40,
            actual_cost: 16.65,
          }),
        ],
      },
    })

    const text = wrapper.text()
    expect(text).toContain('deepseek')
    expect(text).toContain('100')
    expect(text).toContain('250') // 输入
    expect(text).toContain('9.75K') // 缓存 = 250 + 9500
    expect(text).toContain('95.0%') // 9500 / 10000
    expect(text).toContain('$16.65')
  })

  it('quota-field 指定账号成本时取 account_cost', () => {
    const wrapper = mount(ModelUsageCacheTable, {
      props: {
        models: [
          modelStat({
            actual_cost: 1,
            account_cost: 2.5,
          }),
        ],
        quotaField: 'account_cost',
        quotaUnit: 'usd',
      },
    })

    expect(wrapper.text()).toContain('$2.50')
    expect(wrapper.text()).not.toContain('$1.00')
  })

  it('输入侧为 0 时显示不适用而不是 0%', () => {
    const wrapper = mount(ModelUsageCacheTable, {
      props: { models: [modelStat({ model: 'empty' })] },
    })

    expect(wrapper.text()).toContain('N/A')
    expect(wrapper.text()).not.toContain('NaN')
    expect(wrapper.text()).not.toContain('0.0%')
  })

  it('上游不回报缓存写入时创建值显示不适用且命中率仍有效', () => {
    // 真实部署样本：OpenAI 兼容中转 creation=0、read 巨大
    const wrapper = mount(ModelUsageCacheTable, {
      props: {
        models: [
          modelStat({
            model: 'deepseek-flash',
            requests: 6899,
            input_tokens: 28_679_273,
            cache_creation_tokens: 0,
            cache_read_tokens: 1_826_891_006,
          }),
        ],
      },
    })

    const text = wrapper.text()
    expect(text).toContain('1.83B') // 仅缓存读取，带 * 标记
    expect(text).toContain('98.5%')
    expect(text).not.toContain('N/A')
  })
})
