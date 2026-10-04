<template>
  <BaseDialog :show="show" :title="t('usage.detail.title')" width="full" :close-on-click-outside="true" @close="close">
    <ContentSkeleton v-if="loading" variant="detail" :rows="8" class="p-6" />

    <div v-else-if="row" class="space-y-4 text-sm">
      <!-- 基本上下文 -->
      <section class="grid grid-cols-2 gap-x-6 gap-y-3 md:grid-cols-4">
        <div>
          <span class="detail-label">{{ t('usage.errors.time') }}</span>
          <p class="detail-value">{{ formatDateTime(row.created_at) }}</p>
        </div>
        <div>
          <span class="detail-label">{{ t('usage.latencyDuration') }}</span>
          <p class="detail-value">{{ row.duration_ms != null ? formatDuration(row.duration_ms) : '-' }}</p>
        </div>
        <div>
          <span class="detail-label">{{ t('usage.errors.platform') }}</span>
          <p class="detail-value">{{ row.platform || '-' }}</p>
        </div>
        <div>
          <span class="detail-label">{{ t('usage.errors.model') }}</span>
          <p class="detail-value break-all">{{ row.model || '-' }}</p>
        </div>
        <div>
          <span class="detail-label">{{ t('usage.inbound') }}</span>
          <p class="detail-value break-all">{{ row.inbound_endpoint?.trim() || '-' }}</p>
        </div>
        <div v-if="row.upstream_endpoint?.trim()">
          <span class="detail-label">{{ t('usage.upstream') }}</span>
          <p class="detail-value break-all">{{ row.upstream_endpoint }}</p>
        </div>
        <div>
          <span class="detail-label">{{ t('usage.errors.keyName') }}</span>
          <p class="detail-value">{{ row.api_key?.name || '-' }}</p>
        </div>
        <div>
          <span class="detail-label">IP</span>
          <p class="detail-value">{{ row.ip_address || '-' }}</p>
        </div>
        <div class="col-span-2 md:col-span-4">
          <span class="detail-label">Request ID</span>
          <p class="break-all font-mono text-xs text-gray-700 dark:text-gray-300">{{ row.request_id || '-' }}</p>
        </div>
        <div v-if="row.upstream_request_id" class="col-span-2 md:col-span-4">
          <span class="detail-label">{{ t('admin.usage.upstreamRequestId') }}</span>
          <p class="break-all font-mono text-xs text-gray-700 dark:text-gray-300">{{ row.upstream_request_id }}</p>
        </div>
        <div v-if="row.user_agent" class="col-span-2 md:col-span-4">
          <span class="detail-label">{{ t('usage.userAgent') }}</span>
          <p class="break-all text-xs text-gray-700 dark:text-gray-300">{{ row.user_agent }}</p>
        </div>
      </section>

      <!-- Token 与费用 -->
      <section class="grid grid-cols-2 gap-x-6 gap-y-3 md:grid-cols-4">
        <div>
          <span class="detail-label">{{ t('admin.usage.inputTokens') }}</span>
          <p class="detail-value tabular-nums">{{ row.input_tokens?.toLocaleString() || 0 }}</p>
        </div>
        <div>
          <span class="detail-label">{{ t('admin.usage.outputTokens') }}</span>
          <p class="detail-value tabular-nums">{{ row.output_tokens?.toLocaleString() || 0 }}</p>
        </div>
        <div v-if="row.cache_read_tokens > 0">
          <span class="detail-label">{{ t('admin.usage.cacheReadTokens') }}</span>
          <p class="detail-value tabular-nums">{{ row.cache_read_tokens.toLocaleString() }}</p>
        </div>
        <div v-if="row.cache_creation_tokens > 0">
          <span class="detail-label">{{ t('admin.usage.cacheCreationTokens') }}</span>
          <p class="detail-value tabular-nums">{{ row.cache_creation_tokens.toLocaleString() }}</p>
        </div>
        <div>
          <span class="detail-label">{{ t('usage.totalCost') }}</span>
          <p class="detail-value tabular-nums">{{ formatBalanceAmount(row.actual_cost) }}</p>
        </div>
        <div v-if="row.total_cost != null">
          <span class="detail-label">{{ t('usage.standardCost') }}</span>
          <p class="detail-value tabular-nums">{{ formatUsdAmount(row.total_cost) }}</p>
        </div>
      </section>

      <!-- 阶段耗时 -->
      <section v-if="row.detailed_timing">
        <h4 class="section-title">{{ t('usage.detailedTiming') }}</h4>
        <div class="grid grid-cols-2 gap-x-6 gap-y-2 md:grid-cols-3">
          <div v-for="stage in timingStages" :key="stage.key" class="flex items-center justify-between gap-4">
            <span class="text-xs text-gray-500 dark:text-gray-400">{{ stage.label }}</span>
            <span class="tabular-nums text-xs font-medium text-gray-900 dark:text-gray-100">{{ formatTimingMs(row.detailed_timing?.[stage.key]) }}</span>
          </div>
        </div>
      </section>

      <!-- 请求/响应载荷快照（fork 专属捕获） -->
      <section v-if="payloadSections.length > 0">
        <h4 class="section-title">{{ t('usage.detail.payloadTitle') }}</h4>
        <div class="space-y-3">
          <div v-for="section in payloadSections" :key="section.key">
            <div class="mb-1 flex items-center justify-between gap-2">
              <span class="text-xs font-semibold text-gray-600 dark:text-gray-300">{{ section.label }}</span>
              <span v-if="section.truncated" class="text-xs text-amber-600 dark:text-amber-400">{{ t('usage.detail.truncated') }}</span>
            </div>
            <pre class="payload-pre mt-1 max-h-[40vh] overflow-auto whitespace-pre-wrap break-all rounded-surface bg-gray-50 p-3 text-xs text-gray-800 dark:bg-dark-900 dark:text-gray-200">{{ section.body }}</pre>
          </div>
        </div>
      </section>
      <p v-else-if="payloadAvailable" class="text-xs text-gray-400 dark:text-gray-500">{{ t('usage.detail.payloadEmpty') }}</p>
      <p v-else class="text-xs text-gray-400 dark:text-gray-500">{{ t('usage.detail.payloadUnavailable') }}</p>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ContentSkeleton from '@/components/common/ContentSkeleton.vue'
import { selfAPI, type SelfRequestPayload } from '@/api/self'
import { useBalanceDisplay } from '@/composables/useBalanceDisplay'
import { formatDateTime } from '@/utils/format'
import type { AdminUsageLog } from '@/types'

const props = defineProps<{
  show: boolean
  row: AdminUsageLog | null
}>()

const emit = defineEmits<{
  (e: 'update:show', v: boolean): void
}>()

const { t } = useI18n()
const { formatBalanceAmount, formatUsdAmount } = useBalanceDisplay()

const loading = ref(false)
const payload = ref<SelfRequestPayload | null>(null)

const payloadAvailable = computed(() => !!props.row?.request_id)

// 用量行 request_id 形如 "client:<id>"，剥离前缀即捕获表关联键。
const clientRequestID = computed(() => {
  const id = props.row?.request_id?.trim() || ''
  return id.startsWith('client:') ? id.slice('client:'.length) : ''
})

watch(
  () => [props.show, clientRequestID.value] as const,
  ([show, id]) => {
    if (!show) {
      payload.value = null
      return
    }
    if (!id) {
      payload.value = null
      return
    }
    fetchPayload(id)
  }
)

async function fetchPayload(id: string) {
  loading.value = true
  payload.value = null
  try {
    payload.value = await selfAPI.getRequestPayload(id)
  } catch {
    // 捕获未开启或数据已过期时静默降级，只展示行内字段。
    payload.value = null
  } finally {
    loading.value = false
  }
}

function prettyText(raw?: string | null): string {
  const value = (raw || '').trim()
  if (!value) return ''
  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    return value
  }
}

const payloadSections = computed(() => {
  const p = payload.value
  if (!p) return []
  const sections: Array<{ key: string; label: string; body: string; truncated?: boolean }> = []
  const requestHeaders = prettyText(p.request_headers)
  const responseHeaders = prettyText(p.response_headers)
  const requestBody = prettyText(p.request_body)
  const responseBody = prettyText(p.response_body)
  if (requestHeaders) sections.push({ key: 'request_headers', label: t('usage.detail.requestHeaders'), body: requestHeaders })
  if (requestBody) sections.push({ key: 'request_body', label: t('usage.detail.requestBody'), body: requestBody, truncated: p.request_body_truncated })
  if (responseHeaders) sections.push({ key: 'response_headers', label: t('usage.detail.responseHeaders'), body: responseHeaders })
  if (responseBody) sections.push({ key: 'response_body', label: t('usage.detail.responseBody'), body: responseBody, truncated: p.response_body_truncated })
  return sections
})

const timingStages = computed(() => [
  { key: 'provider_slot_acquired_ms', label: t('usage.timingSlot') },
  { key: 'upstream_get_conn_ms', label: t('usage.timingGetConn') },
  { key: 'upstream_wrote_request_ms', label: t('usage.timingWriteRequest') },
  { key: 'upstream_first_response_byte_ms', label: t('usage.timingFirstByte') },
  { key: 'upstream_first_sse_data_ms', label: t('usage.timingFirstSSE') },
  { key: 'first_visible_output_ms', label: t('usage.timingVisible') },
] as Array<{ key: TimingStageKey; label: string }>)

type TimingStageKey = 'provider_slot_acquired_ms' | 'upstream_get_conn_ms' | 'upstream_wrote_request_ms' | 'upstream_first_response_byte_ms' | 'upstream_first_sse_data_ms' | 'first_visible_output_ms'

function formatTimingMs(value?: number | null): string {
  if (value == null) return '-'
  return `${value}ms`
}

// 超过 1 分钟简化为 "Xm Ys"，与表格延迟列保持一致。
function formatDuration(ms: number | null | undefined): string {
  if (ms == null) return '-'
  if (ms < 1000) return `${ms}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(2)}s`
  const totalSec = Math.round(ms / 1000)
  if (totalSec < 3600) return `${Math.floor(totalSec / 60)}m ${totalSec % 60}s`
  return `${Math.floor(totalSec / 3600)}h ${Math.floor((totalSec % 3600) / 60)}m`
}

function close() {
  emit('update:show', false)
}
</script>

<style scoped>
.detail-label {
  display: block;
  font-weight: 500;
  color: var(--color-gray-500, #6b7280);
  font-size: 0.75rem;
}
.detail-value {
  margin-top: 0.125rem;
  color: var(--color-gray-900, #111827);
}
.dark .detail-value {
  color: var(--color-gray-100, #f3f4f6);
}
.section-title {
  margin-bottom: 0.5rem;
  font-size: 0.875rem;
  font-weight: 700;
  color: var(--color-gray-900, #111827);
}
.dark .section-title {
  color: #ffffff;
}
.payload-pre {
  border: 1px solid var(--color-gray-200, #e5e7eb);
}
.dark .payload-pre {
  border-color: var(--color-gray-700, #374151);
}
</style>
