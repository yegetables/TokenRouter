<template>
  <SettingsSection data-testid="upstream-usage-config">
    <SettingToggleRow
      :id="`${uid}-enabled`"
      v-model="enabledModel"
      :label="t('admin.providers.upstreamUsage.title')"
      :hint="t('admin.providers.upstreamUsage.hint')"
      testid="upstream-usage-enabled"
    />

    <Collapse :open="enabledModel && !automaticAdapter" unmount-on-hide>
      <SettingsSubpanel>
        <div>
          <label class="input-label">{{ t('admin.providers.upstreamUsage.adapter') }}</label>
          <Select
            v-model="adapterModel"
            :options="adapterOptions"
            data-testid="upstream-usage-adapter"
          />
          <p v-if="adapterModel === 'zcode'" class="input-hint">
            <i18n-t keypath="admin.providers.upstreamUsage.zcodeNotice" tag="span" scope="global">
              <template #link>
                <a
                  href="https://github.com/TriDefender/zcode-api"
                  target="_blank"
                  rel="noopener noreferrer"
                  class="text-primary-600 hover:underline dark:text-primary-400"
                >zcode-api</a>
              </template>
            </i18n-t>
          </p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.providers.upstreamUsage.baseUrl') }}</label>
          <input
            v-model="baseUrlModel"
            type="text"
            class="input"
            :placeholder="t('admin.providers.upstreamUsage.baseUrlPlaceholder')"
            data-testid="upstream-usage-base-url"
          />
          <p class="input-hint">{{ t('admin.providers.upstreamUsage.baseUrlHint') }}</p>
        </div>
        <template v-if="adapterModel === 'new_api'">
          <div>
            <label class="input-label">{{ t('admin.providers.upstreamUsage.walletAccessToken') }}</label>
            <input
              v-model="walletAccessTokenModel"
              type="password"
              class="input font-mono"
              autocomplete="new-password"
              :placeholder="t('admin.providers.upstreamUsage.walletAccessTokenPlaceholder')"
              data-testid="upstream-usage-wallet-access-token"
            />
            <p class="input-hint">{{ t('admin.providers.upstreamUsage.walletAccessTokenHint') }}</p>
          </div>
          <div>
            <label class="input-label">{{ t('admin.providers.upstreamUsage.walletUserId') }}</label>
            <input
              v-model="walletUserIdModel"
              type="text"
              inputmode="numeric"
              class="input"
              :placeholder="t('admin.providers.upstreamUsage.walletUserIdPlaceholder')"
              data-testid="upstream-usage-wallet-user-id"
            />
            <p class="input-hint">{{ t('admin.providers.upstreamUsage.walletUserIdHint') }}</p>
          </div>
        </template>
      </SettingsSubpanel>
    </Collapse>
  </SettingsSection>
</template>

<script setup lang="ts">
import Collapse from '@/components/common/Collapse.vue'
import SettingsSection from '@/components/common/settings/SettingsSection.vue'
import SettingsSubpanel from '@/components/common/settings/SettingsSubpanel.vue'
import SettingToggleRow from '@/components/common/settings/SettingToggleRow.vue'
import { isUpstreamUsageAdapter } from '@/utils/upstreamUsage'

import { computed, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import type { UpstreamUsageAdapter } from '@/types'
const props = withDefaults(defineProps<{
  enabled: boolean
  adapter: UpstreamUsageAdapter
  baseUrl: string
  walletAccessToken?: string
  walletUserId?: string
  automaticAdapter?: boolean
}>(), {
  enabled: true,
  adapter: 'sub2api',
  baseUrl: '',
  walletAccessToken: '',
  walletUserId: '',
  automaticAdapter: false
})

const emit = defineEmits<{
  'update:enabled': [value: boolean]
  'update:adapter': [value: UpstreamUsageAdapter]
  'update:baseUrl': [value: string]
  'update:walletAccessToken': [value: string]
  'update:walletUserId': [value: string]
}>()

const { t } = useI18n()
const uid = useId()

// 前端提供后端已注册适配器的固定选项。
const adapterOptions = computed<SelectOption[]>(() => [
  { value: 'sub2api', label: t('admin.providers.upstreamUsage.adapters.sub2api') },
  { value: 'new_api', label: t('admin.providers.upstreamUsage.adapters.newApi') },
  { value: 'zivv', label: t('admin.providers.upstreamUsage.adapters.zivv') },
  { value: 'zcode', label: t('admin.providers.upstreamUsage.adapters.zcode') },
  { value: 'cline', label: t('admin.providers.upstreamUsage.adapters.cline') },
  { value: 'cline_pass', label: t('admin.providers.upstreamUsage.adapters.clinePass') }
])

const enabledModel = computed({
  get: () => props.enabled,
  set: (value: boolean) => emit('update:enabled', value)
})

const adapterModel = computed({
  get: () => props.adapter,
  set: (value: string | number | boolean | null) => {
    // 先收窄成适配器名，再走 emit 的字符串重载，避免联合类型干扰重载解析。
    if (isUpstreamUsageAdapter(value)) {
      const adapter: UpstreamUsageAdapter = value
      emit('update:adapter', adapter)
    }
  }
})

const baseUrlModel = computed({
  get: () => props.baseUrl,
  set: (value: string) => emit('update:baseUrl', value)
})

const walletAccessTokenModel = computed({
  get: () => props.walletAccessToken,
  set: (value: string) => emit('update:walletAccessToken', value)
})

const walletUserIdModel = computed({
  get: () => props.walletUserId,
  set: (value: string) => emit('update:walletUserId', value)
})
</script>
