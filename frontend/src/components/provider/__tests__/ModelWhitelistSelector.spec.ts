import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ModelWhitelistSelector from '../ModelWhitelistSelector.vue'

const {
  catalogDefaults,
  syncUpstreamModels,
  syncUpstreamModelsPreview,
  showError,
  showInfo,
  showSuccess,
  copyToClipboard
} = vi.hoisted(() => ({
  catalogDefaults: vi.fn(),
  syncUpstreamModels: vi.fn(),
  syncUpstreamModelsPreview: vi.fn(),
  showError: vi.fn(),
  showInfo: vi.fn(),
  showSuccess: vi.fn(),
  copyToClipboard: vi.fn().mockResolvedValue(true)
}))

vi.mock('@/api/admin/providers', () => ({
  providersAPI: {
    syncUpstreamModels,
    syncUpstreamModelsPreview
  }
}))

vi.mock('@/api/admin/modelAttributes', () => ({ modelAttributesAPI: { defaults: catalogDefaults } }))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showInfo,
    showSuccess
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copyToClipboard })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key
    })
  }
})

function mountSelector(props: Record<string, unknown> = {}) {
  return mount(ModelWhitelistSelector, {
    props: {
      modelValue: [],
      platform: 'openai',
      models: ['gpt-5.6-sol'],
      ...props
    },
    global: {
      stubs: {
        ModelIcon: true,
        Icon: true
      }
    }
  })
}

describe('ModelWhitelistSelector', () => {
  let confirmSpy: ReturnType<typeof vi.spyOn>

  afterEach(() => {
    vi.useRealTimers()
    confirmSpy.mockRestore()
  })
  beforeEach(() => {
    confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    catalogDefaults.mockReset()
    syncUpstreamModels.mockReset()
    syncUpstreamModelsPreview.mockReset()
    showError.mockReset()
    showInfo.mockReset()
    showSuccess.mockReset()
    copyToClipboard.mockReset()
    copyToClipboard.mockResolvedValue(true)
  })

  it('复制模型 ID 时不会选中模型', async () => {
    const wrapper = mountSelector()
    await wrapper.get('div.cursor-pointer').trigger('click')

    const row = wrapper
      .findAll('[data-testid="model-option"]')
      .find(candidate => candidate.text().includes('gpt-5.6-sol'))
    expect(row).toBeTruthy()

    const copyButton = row!.get('[data-testid="copy-model-id"]')
    expect(copyButton.attributes('aria-label')).toBe('common.copy gpt-5.6-sol')
    await copyButton.trigger('click')
    await flushPromises()

    expect(copyToClipboard).toHaveBeenCalledWith('gpt-5.6-sol')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('模型选择行为保持不变', async () => {
    const wrapper = mountSelector()
    await wrapper.get('div.cursor-pointer').trigger('click')

    const row = wrapper
      .findAll('[data-testid="model-option"]')
      .find(candidate => candidate.text().includes('gpt-5.6-sol'))
    expect(row).toBeTruthy()
    await row!.get('[data-testid="select-model"]').trigger('click')

    expect(wrapper.emitted('update:modelValue')).toEqual([[['gpt-5.6-sol']]])
    expect(copyToClipboard).not.toHaveBeenCalled()
  })

  it('创建提供商时使用临时凭证同步上游模型', async () => {
    syncUpstreamModelsPreview.mockResolvedValue({ models: ['gpt-5.1', 'o3', 'gpt-5.1'] })
    const syncCredentials = {
      platform: 'openai',
      type: 'apikey',
      base_url: 'https://openai.example.com/v1',
      api_key: 'openai-key'
    }
    const wrapper = mountSelector({ syncCredentials })

    const button = wrapper.findAll('button').find((item) => item.text().includes('admin.providers.syncUpstreamModels'))
    expect(button).toBeTruthy()
    await button!.trigger('click')

    expect(syncUpstreamModelsPreview).toHaveBeenCalledWith(syncCredentials)
    expect(syncUpstreamModels).not.toHaveBeenCalled()
    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toEqual(['gpt-5.1', 'o3'])
    expect(showSuccess).toHaveBeenCalled()
  })

  it('编辑提供商时仍使用提供商 ID 同步上游模型', async () => {
    syncUpstreamModels.mockResolvedValue({ models: ['claude-sonnet-4-5'] })
    const wrapper = mountSelector({
      platform: 'anthropic',
      providerId: 7,
      syncCredentials: {
        platform: 'anthropic',
        type: 'apikey',
        api_key: 'should-not-use'
      }
    })

    const button = wrapper.findAll('button').find((item) => item.text().includes('admin.providers.syncUpstreamModels'))
    expect(button).toBeTruthy()
    await button!.trigger('click')

    expect(syncUpstreamModels).toHaveBeenCalledWith(7)
    expect(syncUpstreamModelsPreview).not.toHaveBeenCalled()
    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toEqual(['claude-sonnet-4-5'])
  })

  it('同步上游模型时用上游列表覆盖白名单', async () => {
    syncUpstreamModels.mockResolvedValue({ models: ['gpt-5.1', 'o3'] })
    const wrapper = mountSelector({ providerId: 7, modelValue: ['legacy-local', 'gpt-5.1'] })

    const button = wrapper.findAll('button').find((item) => item.text().includes('admin.providers.syncUpstreamModels'))
    await button!.trigger('click')

    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toEqual(['gpt-5.1', 'o3'])
    expect(showSuccess).toHaveBeenCalled()
  })

  it('取消确认时不修改白名单', async () => {
    confirmSpy.mockReturnValue(false)
    syncUpstreamModels.mockResolvedValue({ models: ['gpt-5.1'] })
    const wrapper = mountSelector({ providerId: 7, modelValue: ['legacy-local'] })

    const button = wrapper.findAll('button').find((item) => item.text().includes('admin.providers.syncUpstreamModels'))
    await button!.trigger('click')

    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('白名单与上游一致时提示无变化', async () => {
    syncUpstreamModels.mockResolvedValue({ models: ['gpt-5.1', 'o3'] })
    const wrapper = mountSelector({ providerId: 7, modelValue: ['gpt-5.1', 'o3'] })

    const button = wrapper.findAll('button').find((item) => item.text().includes('admin.providers.syncUpstreamModels'))
    await button!.trigger('click')

    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(showInfo).toHaveBeenCalled()
  })

  it('显式候选包含目录未知型号，空候选保持为空', async () => {
    const wrapper = mountSelector({ models: ['custom/unknown-model'] })
    await wrapper.get('div.cursor-pointer').trigger('click')
    expect(wrapper.get('[data-testid="model-option"]').text()).toContain('custom/unknown-model')
    expect(catalogDefaults).not.toHaveBeenCalled()
    await wrapper.setProps({ models: [] })
    expect(wrapper.findAll('[data-testid="model-option"]')).toHaveLength(0)
    wrapper.unmount()
  })

  it('目录分页只加载候选，搜索以最新响应为准', async () => {
    vi.useFakeTimers()
    let resolveOld!: (value: unknown) => void
    catalogDefaults.mockReturnValueOnce(new Promise(resolve => { resolveOld = resolve }))
    const wrapper = mountSelector({ models: undefined, modelValue: ['saved-custom'] })
    await wrapper.get('div.cursor-pointer').trigger('click')
    await vi.advanceTimersByTimeAsync(300)
    catalogDefaults.mockResolvedValueOnce({ items: [{ model: 'new-model' }], total: 2 })
    await wrapper.get('input[placeholder="admin.providers.searchModels"]').setValue('new')
    await vi.advanceTimersByTimeAsync(300)
    resolveOld({ items: [{ model: 'stale-model' }], total: 1 })
    await flushPromises()
    expect(wrapper.findAll('[data-testid="model-option"]').map(row => row.text())).toEqual(['new-model'])
    catalogDefaults.mockResolvedValueOnce({ items: [{ model: 'new-second' }], total: 2 })
    await wrapper.findAll('button').find(button => button.text() === 'admin.providers.loadMoreModels')!.trigger('click')
    await flushPromises()
    expect(catalogDefaults).toHaveBeenLastCalledWith({ search: 'new', page: 2, page_size: 50 })
    expect(wrapper.findAll('[data-testid="model-option"]')).toHaveLength(2)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(wrapper.text()).toContain('saved-custom')
    wrapper.unmount()
  })

  it('目录失败可以重试，仍可手动添加型号', async () => {
    vi.useFakeTimers()
    catalogDefaults.mockRejectedValueOnce(new Error('unavailable'))
    const wrapper = mountSelector({ models: undefined })
    await wrapper.get('div.cursor-pointer').trigger('click')
    await vi.advanceTimersByTimeAsync(300)
    const retry = wrapper.findAll('button').find(button => button.text() === 'common.retry')
    expect(retry).toBeTruthy()
    await wrapper.get('input[placeholder="admin.providers.enterCustomModelName"]').setValue('custom-model')
    await wrapper.findAll('button').find(button => button.text() === 'admin.providers.addModel')!.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toEqual(['custom-model'])
    catalogDefaults.mockResolvedValueOnce({ items: [{ model: 'recovered' }], total: 1 })
    await retry!.trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="model-option"]').text()).toContain('recovered')
    wrapper.unmount()
  })

})
