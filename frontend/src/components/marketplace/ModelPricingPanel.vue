<template>
  <div v-if="hasDisplayPricing" class="min-w-0">
    <!-- 展开/收起触发条：右下角箭头指示面板状态，展开时向上、收起时向下。 -->
    <button
      type="button"
      class="mt-3 flex w-full items-center justify-between gap-2 border-t border-gray-100 pt-3 text-sm font-medium text-primary-600 transition hover:text-primary-700 dark:border-dark-700 dark:text-primary-300 dark:hover:text-primary-200"
      data-testid="model-pricing-toggle"
      :aria-expanded="expanded"
      @click="expanded = !expanded"
    >
      <span class="inline-flex items-center gap-1.5">
        <Icon name="eye" size="sm" />
        {{ expanded ? t('marketplace.collapsePricing') : t('marketplace.viewPricing') }}
      </span>
      <Icon name="chevronDown" class="transition-transform duration-normal" :class="{ 'rotate-180': expanded }" size="sm" :animate-on-hover="false" />
    </button>

    <!-- 收起后保留上下文区间和 fast mode，退出期间同步折叠高度。 -->
    <Collapse :open="expanded">
      <div class="pt-3">
        <!-- 右上角：上下文区间 / fast mode 切换，定价行随选择联动。 -->
        <div
          v-if="selectableIntervals.length > 0 || hasFastPricing"
          class="mb-3 flex flex-wrap items-center justify-end gap-2"
        >
          <div
            v-segmented
            v-if="selectableIntervals.length > 0"
            class="segmented max-w-full flex-wrap"
            data-testid="pricing-interval-switch"
          >
            <button
              v-for="(item, index) in selectableIntervals"
              :key="item.key"
              type="button"
              class="segmented-item px-2 py-0.5 text-xs font-semibold"
              :class="{ 'segmented-item-active': index === activeIntervalIndex }"
              @click="selectedIntervalIndex = index"
            >
              {{ formatCompactTokenRange(item.interval.min_tokens, item.interval.max_tokens) }}
            </button>
          </div>
          <div
            v-segmented
            v-if="hasFastPricing"
            class="segmented max-w-full flex-wrap"
            data-testid="pricing-fast-switch"
          >
            <button
              type="button"
              class="segmented-item px-2 py-0.5 text-xs font-semibold"
              :class="{ 'segmented-item-active': !fastMode }"
              @click="fastMode = false"
            >
              {{ t('marketplace.pricingStandard') }}
            </button>
            <button
              type="button"
              class="segmented-item px-2 py-0.5 text-xs font-semibold"
              :class="{ 'segmented-item-active': fastMode }"
              @click="fastMode = true"
            >
              {{ t('marketplace.pricingFast') }}
            </button>
          </div>
        </div>

        <!-- 高峰期角标：展示价已含当前时段与高峰倍率，角标只读服务端下发的生效值。 -->
        <div v-if="timePricing || peakRate" class="mb-3 flex flex-wrap items-center gap-2" data-testid="pricing-peak-row">
          <span
            class="inline-flex items-center rounded-compact px-2 py-0.5 text-xs font-semibold tabular-nums"
            :class="activeTimeMultiplier > 1 ? peakBadgeActiveClass : peakBadgeInactiveClass"
            data-testid="pricing-peak-badge"
          >
            {{ activeTimeMultiplier > 1 ? t('marketplace.pricingPeak') : t('marketplace.pricingOffPeak') }}
            ×{{ formatMultiplier(activeTimeMultiplier) }}
          </span>
          <span
            v-for="window in timeWindows"
            :key="window"
            class="rounded-compact bg-gray-100 px-2 py-0.5 text-xs text-gray-600 dark:bg-dark-800 dark:text-dark-300"
            data-testid="pricing-peak-window"
          >
            {{ window }}
          </span>
        </div>

        <!-- 完整定价允许在窄卡片内换行，避免隐藏的抽屉也撑大父网格。 -->
        <div v-if="activeRows.length > 0" class="space-y-2.5" data-testid="pricing-rows">
          <div
            v-for="row in activeRows"
            :key="row.key"
            class="flex items-baseline justify-between gap-3 border-b border-gray-100 pb-2 text-sm dark:border-dark-700"
          >
            <span class="min-w-0 max-w-[45%] shrink-0 break-words text-gray-500 dark:text-dark-400">{{ row.label }}</span>
            <span class="min-w-0 break-words text-right font-medium [overflow-wrap:anywhere] tabular-nums text-gray-900 dark:text-white">{{ row.value }}</span>
          </div>
        </div>
        <p v-else class="text-sm text-gray-400 dark:text-dark-500">
          {{ t('marketplace.pricingUnavailable') }}
        </p>
      </div>
    </Collapse>
  </div>
</template>

<script setup lang="ts">
import { vSegmented } from '@/directives/segmented'
import Collapse from '@/components/common/Collapse.vue'
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useBalanceDisplay } from '@/composables/useBalanceDisplay'
import { formatCompactTokenRange } from '@/utils/formatters'
import type { MarketplaceModel, MarketplaceModelPricing, MarketplacePricingInterval } from '@/types'

// 抽屉式完整定价面板：原地展开收起、上下文区间与 fast mode 切换都收敛在卡片内部。
const props = defineProps<{
  model: MarketplaceModel
}>()

const { t } = useI18n()
const { balanceUnitName } = useBalanceDisplay()

const expanded = ref(false)
const fastMode = ref(false)
const selectedIntervalIndex = ref(0)


interface PricingRow {
  key: string
  label: string
  value: string
}

function hasPositiveValue(value?: number | null): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0
}

// —— 价格格式化：与模型广场卡片预览保持同一口径 ——

function formatPriceNumber(value: number): string {
  const abs = Math.abs(value)
  const maximumFractionDigits = abs >= 1 ? 2 : abs >= 0.01 ? 4 : 6
  const minimumFractionDigits = abs >= 1 ? 2 : 4

  return new Intl.NumberFormat(undefined, {
    minimumFractionDigits,
    maximumFractionDigits,
  }).format(value)
}

function formatPrice(value: number): string {
  return `${formatPriceNumber(value)} ${balanceUnitName.value}`
}

function formatPerMillion(value: number): string {
  return `${formatPrice(value * 1_000_000)} ${t('usage.perMillionTokens')}`
}

function formatPerImage(value: number): string {
  return `${formatPrice(value)} ${t('marketplace.perImage')}`
}

// —— 定价行构建 ——

function tokenPricingRowsFromValues(pricing: MarketplaceModelPricing | MarketplacePricingInterval): PricingRow[] {
  const rows: PricingRow[] = []

  if (hasPositiveValue(pricing.input_price_per_token)) {
    rows.push({ key: 'input', label: t('marketplace.input'), value: formatPerMillion(pricing.input_price_per_token) })
  }
  if (hasPositiveValue(pricing.image_input_price_per_token)) {
    rows.push({ key: 'image_input', label: t('marketplace.imageInput'), value: formatPerMillion(pricing.image_input_price_per_token) })
  }
  if (hasPositiveValue(pricing.output_price_per_token)) {
    rows.push({ key: 'output', label: t('marketplace.output'), value: formatPerMillion(pricing.output_price_per_token) })
  }
  if (hasPositiveValue(pricing.cache_write_price_per_token)) {
    rows.push({ key: 'cache_write', label: t('marketplace.cacheWrite'), value: formatPerMillion(pricing.cache_write_price_per_token) })
  }
  if (hasPositiveValue(pricing.cache_write_1h_price_per_token)) {
    rows.push({ key: 'cache_write_1h', label: t('marketplace.cacheWrite1h'), value: formatPerMillion(pricing.cache_write_1h_price_per_token) })
  }
  if (hasPositiveValue(pricing.cache_read_price_per_token)) {
    rows.push({ key: 'cache_read', label: t('marketplace.cacheRead'), value: formatPerMillion(pricing.cache_read_price_per_token) })
  }
  if (hasPositiveValue(pricing.image_output_price_per_token)) {
    rows.push({ key: 'image_output', label: t('marketplace.imageOutput'), value: formatPerMillion(pricing.image_output_price_per_token) })
  }

  return rows
}

// fast mode 价格行：只取 fast_* 字段，没有 fast 定价时返回空列表。
function fastTokenPricingRows(pricing: MarketplaceModelPricing | MarketplacePricingInterval): PricingRow[] {
  const rows: PricingRow[] = []

  if (hasPositiveValue(pricing.fast_input_price_per_token)) {
    rows.push({ key: 'fast_input', label: t('marketplace.fastInput'), value: formatPerMillion(pricing.fast_input_price_per_token) })
  }
  if (hasPositiveValue(pricing.fast_image_input_price_per_token)) {
    rows.push({ key: 'fast_image_input', label: t('marketplace.fastImageInput'), value: formatPerMillion(pricing.fast_image_input_price_per_token) })
  }
  if (hasPositiveValue(pricing.fast_output_price_per_token)) {
    rows.push({ key: 'fast_output', label: t('marketplace.fastOutput'), value: formatPerMillion(pricing.fast_output_price_per_token) })
  }
  if (hasPositiveValue(pricing.fast_cache_write_price_per_token)) {
    rows.push({ key: 'fast_cache_write', label: t('marketplace.fastCacheWrite'), value: formatPerMillion(pricing.fast_cache_write_price_per_token) })
  }
  if (hasPositiveValue(pricing.fast_cache_write_1h_price_per_token)) {
    rows.push({ key: 'fast_cache_write_1h', label: t('marketplace.fastCacheWrite1h'), value: formatPerMillion(pricing.fast_cache_write_1h_price_per_token) })
  }
  if (hasPositiveValue(pricing.fast_cache_read_price_per_token)) {
    rows.push({ key: 'fast_cache_read', label: t('marketplace.fastCacheRead'), value: formatPerMillion(pricing.fast_cache_read_price_per_token) })
  }
  if (hasPositiveValue(pricing.fast_image_output_price_per_token)) {
    rows.push({ key: 'fast_image_output', label: t('marketplace.fastImageOutput'), value: formatPerMillion(pricing.fast_image_output_price_per_token) })
  }

  return rows
}

// 显式价格为 0 表示免费， priced 状态但无正价时展示 0 而不是空列表。
function zeroTokenPricingRows(): PricingRow[] {
  return [
    { key: 'input', label: t('marketplace.input'), value: formatPerMillion(0) },
    { key: 'output', label: t('marketplace.output'), value: formatPerMillion(0) },
  ]
}

function imagePricingRows(pricing: MarketplaceModelPricing): PricingRow[] {
  const values = [
    { key: '1k', label: '1K', price: pricing.image_price_1k },
    { key: '2k', label: '2K', price: pricing.image_price_2k },
    { key: '4k', label: '4K', price: pricing.image_price_4k },
  ]

  return values.flatMap((item) => {
    if (typeof item.price !== 'number' || !Number.isFinite(item.price) || item.price < 0) {
      return []
    }

    return [{
      key: item.key,
      label: item.label,
      value: formatPerImage(item.price),
    }]
  })
}

function hasImagePricing(pricing: MarketplaceModelPricing): boolean {
  return [
    pricing.image_price_1k,
    pricing.image_price_2k,
    pricing.image_price_4k,
  ].some((value) => typeof value === 'number' && Number.isFinite(value) && value >= 0)
}

function pricingKind(pricing: MarketplaceModelPricing): 'token' | 'image' | 'unpriced' {
  if (pricing.price_status !== 'priced') {
    return 'unpriced'
  }
  if (pricing.pricing_mode === 'image' && hasImagePricing(pricing)) {
    return 'image'
  }
  if (pricing.pricing_mode === 'token') {
    return 'token'
  }
  return 'unpriced'
}

const hasDisplayPricing = computed(() => pricingKind(props.model.pricing) !== 'unpriced')

// 后端仅返回有定价的区间；零价字段会被 JSON 省略，不能据此过滤免费区间。
const selectableIntervals = computed(() =>
  (props.model.pricing.context_intervals ?? [])
    .map((interval, index) => ({ interval, key: `${interval.min_tokens}-${interval.max_tokens ?? 'up'}-${index}` }))
)

const activeIntervalIndex = computed(() =>
  Math.min(selectedIntervalIndex.value, Math.max(0, selectableIntervals.value.length - 1))
)

// —— 分时与高峰：展示价已是当前生效价，这里只用角标标注时段与当前档位 ——
// 档位一律读服务端下发的生效值，不用浏览器时间推算，避免与结算口径不一致。

const peakBadgeActiveClass = 'bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300'
const peakBadgeInactiveClass = 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300'

function formatMultiplier(value: number): string {
  return Number.isInteger(value) ? String(value) : String(Number(value.toFixed(2)))
}

// 分时时段是左闭右开区间；结束时间为 00:00 表示当日 24:00。
function formatTimeRange(start: string, end: string): string {
  const normalizedEnd = /^00:00(:00)?$/.test(end) ? '24:00' : end.slice(0, 5)
  return `${start.slice(0, 5)}–${normalizedEnd}`
}

const timePricing = computed(() => {
  const config = props.model.pricing.time_pricing
  return config && config.periods.length > 0 ? config : null
})

const peakRate = computed(() => props.model.pricing.peak_rate ?? null)

// 展示价实际计入的总倍率（分时 × 高峰）由服务端下发，前端不按同一份配置再乘一次。
const activeTimeMultiplier = computed(() => props.model.pricing.active_multiplier ?? 1)

// 时段标签：分时各段、限工作日与高峰窗口。
const timeWindows = computed<string[]>(() => {
  const windows = (timePricing.value?.periods ?? []).map(
    (period) => `${formatTimeRange(period.start_time, period.end_time)} ×${formatMultiplier(period.multiplier)}`,
  )
  if (timePricing.value?.weekdays_only) {
    windows.push(t('marketplace.pricingWeekdaysOnly'))
  }
  if (peakRate.value) {
    windows.push(
      t('marketplace.pricingPeakWindow', {
        range: formatTimeRange(peakRate.value.start_time, peakRate.value.end_time),
        multiplier: formatMultiplier(peakRate.value.multiplier),
      }),
    )
  }
  return windows
})

// 定价数据来源：选中区间优先，否则用模型顶层价格。
const activeSource = computed<MarketplaceModelPricing | MarketplacePricingInterval>(() =>
  selectableIntervals.value[activeIntervalIndex.value]?.interval ?? props.model.pricing
)

const standardRows = computed<PricingRow[]>(() => {
  if (pricingKind(props.model.pricing) === 'image') {
    return imagePricingRows(props.model.pricing)
  }

  const rows = tokenPricingRowsFromValues(activeSource.value)
  if (rows.length > 0) {
    return rows
  }
  return props.model.pricing.price_status === 'priced' ? zeroTokenPricingRows() : []
})

const fastRows = computed(() => fastTokenPricingRows(activeSource.value))

// 当前定价来源存在 fast mode 加价时才展示切换。
const hasFastPricing = computed(() => pricingKind(props.model.pricing) === 'token' && fastRows.value.length > 0)

const activeRows = computed(() => {
  if (fastMode.value && fastRows.value.length > 0) {
    return fastRows.value
  }
  return standardRows.value
})
</script>
