<template>
  <div class="card p-4" data-testid="model-usage-cache-table">
    <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">
      {{ title || t('usage.modelUsageCache.title') }}
    </h3>

    <div v-if="loading" class="flex h-32 items-center justify-center">
      <LoadingSpinner />
    </div>
    <div
      v-else-if="rows.length === 0"
      class="flex h-32 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t('admin.dashboard.noDataAvailable') }}
    </div>
    <div v-else class="overflow-x-auto">
      <table class="w-full text-xs">
        <thead>
          <tr class="border-b border-gray-200 text-gray-500 dark:border-dark-700 dark:text-gray-400">
            <th class="pb-2 text-left font-medium">{{ t('usage.modelUsageCache.model') }}</th>
            <th class="pb-2 text-right font-medium">{{ t('usage.modelUsageCache.requests') }}</th>
            <th class="pb-2 text-right font-medium">{{ t('usage.modelUsageCache.input') }}</th>
            <th class="pb-2 text-right font-medium">{{ t('usage.modelUsageCache.cache') }}</th>
            <th class="pb-2 text-right font-medium">{{ t('usage.modelUsageCache.cacheRate') }}</th>
            <th class="pb-2 text-right font-medium">{{ t('usage.modelUsageCache.output') }}</th>
            <th class="pb-2 text-right font-medium">{{ t('usage.modelUsageCache.quota') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="row in rows"
            :key="row.model"
            class="border-b border-gray-100 last:border-b-0 dark:border-dark-800"
          >
            <td
              class="max-w-[220px] truncate py-2 pr-3 font-medium text-gray-900 dark:text-white"
              :title="row.model"
            >
              {{ row.model }}
            </td>
            <td class="py-2 text-right tabular-nums text-gray-600 dark:text-gray-400">
              {{ row.requests.toLocaleString() }}
            </td>
            <td class="py-2 text-right tabular-nums text-gray-600 dark:text-gray-400">
              {{ formatTokens(row.input) }}
            </td>
            <td class="py-2 text-right tabular-nums text-gray-600 dark:text-gray-400">
              {{ formatTokens(row.cache) }}
            </td>
            <td class="py-2 text-right tabular-nums text-gray-600 dark:text-gray-400">
              {{ row.cacheRate.toFixed(1) }}%
            </td>
            <td class="py-2 text-right tabular-nums text-gray-600 dark:text-gray-400">
              {{ formatTokens(row.output) }}
            </td>
            <td class="py-2 text-right tabular-nums font-medium text-gray-900 dark:text-white">
              {{ formatQuota(row.quota) }}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useBalanceDisplay } from '@/composables/useBalanceDisplay'
import type { ModelStat } from '@/types'

const props = withDefaults(defineProps<{
  /** 按模型的用量行，来自账号/用户统计接口的 models 字段。 */
  models: ModelStat[]
  loading?: boolean
  /** 额度列取哪个成本字段：用户实际扣费 / 账号口径成本 / 标准成本。 */
  quotaField?: 'actual_cost' | 'account_cost' | 'cost'
  /** 额度列金额单位：跟随站点余额单位或固定 USD。 */
  quotaUnit?: 'balance' | 'usd'
  title?: string
}>(), {
  loading: false,
  quotaField: 'actual_cost',
  quotaUnit: 'balance',
})

const { t } = useI18n()
const { formatBalanceAmount, formatUsdAmount } = useBalanceDisplay()

interface ModelUsageCacheRow {
  model: string
  requests: number
  input: number
  cache: number
  cacheRate: number
  output: number
  quota: number
}

// 缓存 = 缓存创建 + 缓存读取；缓存率与 TokenUsageTrend 保持一致，分母为输入侧总量。
const rows = computed<ModelUsageCacheRow[]>(() =>
  (props.models || []).map((stat) => {
    const input = stat.input_tokens || 0
    const cacheCreation = stat.cache_creation_tokens || 0
    const cacheRead = stat.cache_read_tokens || 0
    const cache = cacheCreation + cacheRead
    const inputSideTokens = input + cache
    const quota = Number(
      (stat as unknown as Record<string, number | undefined>)[props.quotaField] ?? 0
    )
    return {
      model: stat.model,
      requests: stat.requests || 0,
      input,
      cache,
      cacheRate: inputSideTokens > 0 ? (cacheRead / inputSideTokens) * 100 : 0,
      output: stat.output_tokens || 0,
      quota: Number.isFinite(quota) ? quota : 0,
    }
  })
)

const formatTokens = (value: number): string => {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(2)}B`
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(2)}K`
  return value.toLocaleString()
}

const formatQuota = (value: number): string =>
  props.quotaUnit === 'usd'
    ? formatUsdAmount(value, { fractionDigits: 2 })
    : formatBalanceAmount(value, { fractionDigits: 2 })
</script>
