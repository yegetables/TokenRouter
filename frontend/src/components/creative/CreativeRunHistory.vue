<template>
  <!-- 历史入口：画布右上角的浮层按钮，高度与顶部工具条一致 -->
  <div class="canvas-island absolute right-3 top-3 z-20 rounded-surface p-1" :class="bumping && 'history-bump'" @animationend="onBumpEnd">
    <button
      ref="historyButtonRef"
      type="button"
      class="canvas-tool-btn relative"
      :class="open && 'canvas-tool-btn-active'"
      :title="t('creative.history.toggle')"
      :aria-expanded="open"
      @click="open = !open"
    >
      <Icon name="history" size="md" />
      <!-- 活动任务数量：保持在图标右上角，不展开历史也能感知后台进度。 -->
      <span
        v-if="props.activeRunCount > 0"
        class="absolute -right-1.5 -top-1.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-primary-600 px-1 text-xs font-semibold leading-none text-white ring-2 ring-white tabular-nums dark:ring-dark-900"
      >
        {{ props.activeRunCount > 99 ? '99+' : props.activeRunCount }}
      </span>
    </button>
  </div>

  <!-- 悬浮历史列表：点击展开 / 收起，选择行后不自动收起 -->
  <MotionTransition name="pop-float">
    <div
      v-if="open"
      class="canvas-island history-pop-float absolute right-3 top-16 z-20 flex max-h-[70%] w-80 flex-col overflow-hidden rounded-surface"
    >
      <div class="flex items-center gap-1 border-b border-primary-900/8 py-1.5 pl-3 pr-1.5 dark:border-dark-600">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('creative.history.title') }}
        </h3>
        <button
          type="button"
          class="canvas-tool-btn ml-auto"
          :disabled="refreshing || studio.loadingHistory.value"
          :aria-busy="refreshing || studio.loadingHistory.value"
          :title="t('common.refresh')"
          @click="refresh"
        >
          <Icon
            name="refresh"
            size="sm"
            :class="(refreshing || studio.loadingHistory.value) && 'animate-spin'"
          />
        </button>
        <button
          type="button"
          class="canvas-tool-btn"
          :title="t('common.close')"
          @click="open = false"
        >
          <Icon name="x" size="sm" />
        </button>
      </div>

      <div class="min-h-0 flex-1 overflow-y-auto p-1.5">
        <div v-if="studio.runHistory.value.length" class="space-y-0.5">
          <div
            v-for="run in studio.runHistory.value"
            :key="run.id"
            class="rounded-control transition-colors"
            :class="studio.currentRun.value?.id === run.id || expandedRunId === run.id ? 'bg-primary-500/8' : 'hover:bg-gray-50 dark:hover:bg-dark-800'"
          >
            <!-- 行头：点击原地展开 / 收起 -->
            <button type="button" class="w-full px-2.5 py-2 text-left" @click="toggleRun(run.id)">
              <div class="flex items-center gap-2">
                <span class="run-status flex-shrink-0" :class="`run-status-${statusTone(run)}`">
                  <Icon
                    v-if="isActive(run)"
                    name="loader"
                    size="xs"
                    class="animate-spin"
                    :animate-on-hover="false"
                  />
                  <span v-else class="h-1.5 w-1.5 rounded-full bg-current" aria-hidden="true"></span>
                  {{ t(`creative.status.${run.status}`, run.status) }}
                </span>
                <span class="min-w-0 flex-1 truncate text-xs text-gray-500 dark:text-dark-300">{{ run.model }}</span>
                <Icon
                  name="chevronDown"
                  size="sm"
                  class="flex-shrink-0 text-gray-400 transition-transform dark:text-dark-400"
                  :class="expandedRunId === run.id && 'rotate-180'"
                  :animate-on-hover="false"
                />
              </div>
              <div class="mt-1 flex items-center gap-2 text-xs tabular-nums text-gray-400 dark:text-dark-400">
                <span :title="formatRunTime(run.created_at)">{{ formatRunRelative(run.created_at) }}</span>
                <span
                  v-if="formatElapsed(run)"
                  class="inline-flex shrink-0 items-center gap-1"
                  :aria-label="t('creative.history.elapsed', { time: formatElapsed(run) })"
                  :title="t('creative.history.elapsed', { time: formatElapsed(run) })"
                >
                  <Icon name="clock" size="xs" aria-hidden="true" />
                  <span>{{ formatElapsed(run) }}</span>
                </span>
                <span v-if="run.actual_cost != null" class="ml-auto">{{ t('creative.result.actualCost', { cost: formatBalanceAmount(run.actual_cost, { fractionDigits: 3 }) }) }}</span>
              </div>
            </button>

            <!-- 进行中的任务只显示加载状态，终态任务才显示素材与操作按钮。 -->
            <MotionTransition name="history-details">
              <div v-if="expandedRunId === run.id" class="history-details-grid">
                <div class="min-h-0 overflow-hidden">
                  <div class="space-y-2 px-2.5 pb-2.5">
                    <div v-if="isActive(run)" class="flex items-center gap-3 text-xs text-gray-500 dark:text-dark-300">
                      <div class="flex h-16 w-16 flex-shrink-0 items-center justify-center rounded-control bg-white dark:bg-dark-950">
                        <Icon
                          name="loader"
                          size="md"
                          class="animate-spin text-primary-500"
                          :animate-on-hover="false"
                        />
                      </div>
                      <div class="min-w-0">
                        <span class="block">{{ t(`creative.status.${run.status}`, run.status) }}</span>
                        <span
                          v-if="formatElapsed(run)"
                          data-testid="creative-run-elapsed"
                          class="mt-1 inline-flex items-center gap-1 tabular-nums text-xs text-gray-400 dark:text-dark-400"
                          :aria-label="t('creative.history.elapsed', { time: formatElapsed(run) })"
                          :title="t('creative.history.elapsed', { time: formatElapsed(run) })"
                        >
                          <Icon name="clock" size="xs" aria-hidden="true" />
                          <span>{{ formatElapsed(run) }}</span>
                        </span>
                      </div>
                    </div>
                    <template v-else-if="run.outputs?.length">
                      <!-- 输出纵向排列，图片撑满弹窗宽度；导入画布和下载悬浮在图片右上角，触屏设备常显 -->
                      <div
                        v-for="output in run.outputs"
                        :key="output.output_index"
                        class="history-output group relative flex w-full items-center justify-center overflow-hidden rounded-control bg-white dark:bg-dark-950"
                      >
                        <img
                          v-if="assetFor(run.id, output.output_index)"
                          :src="urlForAsset(outputAssetKey(run.id, output.output_index), assetFor(run.id, output.output_index)!.blob)"
                          :alt="`output-${output.output_index}`"
                          draggable="true"
                          class="block h-auto w-full cursor-grab select-none active:cursor-grabbing"
                          @dragstart.stop="onOutputDragStart($event, run.id, output.output_index)"
                        />
                        <div v-else class="flex h-24 w-full flex-col items-center justify-center gap-1 text-gray-400 dark:text-dark-500">
                          <Icon name="modalityImage" size="sm" />
                          <span class="text-xs">{{ t('creative.result.missing') }}</span>
                        </div>
                        <div v-if="assetFor(run.id, output.output_index)" class="history-output-actions">
                          <button
                            type="button"
                            class="history-output-btn"
                            :title="t('creative.history.importToCanvas')"
                            :aria-label="t('creative.history.importToCanvas')"
                            @click="importToCanvas(run.id, output.output_index)"
                          >
                            <Icon name="plus" size="sm" />
                          </button>
                          <button
                            type="button"
                            class="history-output-btn"
                            :title="t('creative.history.download')"
                            :aria-label="t('creative.history.download')"
                            @click="downloadOutput(run.id, output.output_index, output.mime_type)"
                          >
                            <Icon name="download" size="sm" />
                          </button>
                        </div>
                      </div>
                    </template>
                    <p v-else class="text-xs text-gray-400 dark:text-dark-400">{{ t('creative.history.noOutputs') }}</p>
                    <button
                      v-if="canRetry(run)"
                      type="button"
                      data-testid="creative-run-retry"
                      class="flex w-full items-center justify-center gap-1 rounded-control border border-primary-900/10 px-2 py-1 text-xs text-gray-600 transition-colors hover:border-primary-500 hover:text-primary-600 dark:border-dark-600 dark:text-gray-300 dark:hover:border-primary-500 dark:hover:text-primary-300"
                      @click="emit('retry', run.id)"
                    >
                      <Icon name="refresh" size="sm" />
                      {{ t('creative.history.retry') }}
                    </button>
                  </div>
                </div>
              </div>
            </MotionTransition>
          </div>
        </div>
        <p v-else-if="!studio.loadingHistory.value" class="py-8 text-center text-xs text-gray-400 dark:text-dark-400">
          {{ t('creative.history.empty') }}
        </p>
      </div>
    </div>
  </MotionTransition>
</template>

<script setup lang="ts">
import MotionTransition from '@/components/common/MotionTransition.vue'
/**
 * 创作 run 历史（悬浮层）：
 * - 画布右上角图标按钮展开 / 收起；列表每行 = 状态 + 模型名 + 相对时间（悬停显示完整时间）+ 耗时 + 实际费用
 * - 状态按四种色调显示：进行中品牌青加转圈、成功绿色、失败和结果丢失红色、取消灰色
 * - 点击行原地向下展开：终态任务显示本地保存的输出图片，图片按原始比例撑满弹窗宽度并可拖到画布；
 *   「导入到画布」和「下载」悬浮在图片右上角，本地素材缺失时显示缺失占位
 * - 进行中的任务只展示加载状态，界面上没有素材操作或取消入口
 */
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { saveAs } from 'file-saver'
import Icon from '@/components/icons/Icon.vue'
import { CREATIVE_RUN_TERMINAL_STATUSES, type CreativeRun } from '@/api/creative'
import { formatDateTime, formatRelativeTime } from '@/utils/format'
import { creativeTimestampToMs, formatCreativeRunElapsed } from '@/utils/creativeRunTime'
import { outputAssetKey, type LocalAsset } from '@/utils/creativeLocalStore'
import { CREATIVE_OUTPUT_DRAG_MIME, serializeCreativeOutputDrag } from '@/utils/creativeDrag'
import { useBalanceDisplay } from '@/composables/useBalanceDisplay'
import type { useCreativeStudio } from '@/composables/useCreativeStudio'

type Studio = ReturnType<typeof useCreativeStudio>

interface Props {
  studio: Studio
  activeRunCount?: number
}

const props = withDefaults(defineProps<Props>(), {
  activeRunCount: 0,
})
const emit = defineEmits<{ retry: [runId: string] }>()
// 本地别名：studio 为 props 传入的共享状态机，子组件经它读写
const studio = props.studio
const { t } = useI18n()
const { formatBalanceAmount } = useBalanceDisplay()

// 历史面板展开状态：默认折叠
const open = ref(false)
// 原地展开的历史任务 id（同时只展开一条）
const expandedRunId = ref<string | null>(null)
// 手动刷新状态独立维护，确保快速响应也能先渲染出旋转反馈
const refreshing = ref(false)
const historyButtonRef = ref<HTMLButtonElement | null>(null)
// 发送动画落到历史入口时播放一次弹跳，由 animationend 复位
const bumping = ref(false)

function bump(): void {
  bumping.value = false
  // 下一帧再加类名，连续触发时动画能重新开始
  requestAnimationFrame(() => {
    bumping.value = true
  })
}

function onBumpEnd(event: AnimationEvent): void {
  if (event.animationName === 'history-bump') bumping.value = false
}

defineExpose({ historyButtonRef, bump })
// 只有存在活动任务时才运行时钟，终态任务直接使用服务端完成时间。
const elapsedNow = ref(Date.now())
let elapsedTimer: ReturnType<typeof setInterval> | null = null

// 展开区的 objectURL 缓存：切换收起或卸载时统一回收
const expandedUrls = new Map<string, string>()

function toggleRun(runId: string): void {
  expandedRunId.value = expandedRunId.value === runId ? null : runId
}

function assetFor(runId: string, outputIndex: number): LocalAsset | null {
  return studio.outputAssetMap.value.get(outputAssetKey(runId, outputIndex)) ?? null
}

function urlForAsset(key: string, blob: Blob): string {
  const cached = expandedUrls.get(key)
  if (cached) return cached
  const url = URL.createObjectURL(blob)
  expandedUrls.set(key, url)
  return url
}

function revokeExpandedUrls(): void {
  expandedUrls.forEach((url) => URL.revokeObjectURL(url))
  expandedUrls.clear()
}

// 切换展开行 / 收起面板时回收上一批 objectURL；组件卸载兜底回收
watch(expandedRunId, revokeExpandedUrls)
watch(open, (value) => {
  if (!value) {
    expandedRunId.value = null
    revokeExpandedUrls()
  }
})
watch(
  () => studio.runHistory.value,
  (runs) => {
    const hasActiveRun = runs.some((run) => isActive(run))
    if (hasActiveRun && !elapsedTimer) {
      elapsedNow.value = Date.now()
      elapsedTimer = setInterval(() => {
        elapsedNow.value = Date.now()
      }, 1000)
    } else if (!hasActiveRun && elapsedTimer) {
      clearInterval(elapsedTimer)
      elapsedTimer = null
    }
  },
  { deep: true, immediate: true },
)
onBeforeUnmount(() => {
  if (elapsedTimer) {
    clearInterval(elapsedTimer)
    elapsedTimer = null
  }
  revokeExpandedUrls()
})

// 导入画布：把本地保存的输出素材放上画布（走画布桥接）
function importToCanvas(runId: string, outputIndex: number): void {
  studio.importOutputToCanvas(runId, outputIndex)
}

// 历史缩略图拖放只传运行记录索引，画布接收后从 IndexedDB 取回图片本体。
function onOutputDragStart(event: DragEvent, runId: string, outputIndex: number): void {
  const asset = assetFor(runId, outputIndex)
  if (!asset || !event.dataTransfer) return
  event.dataTransfer.effectAllowed = 'copy'
  event.dataTransfer.setData(
    CREATIVE_OUTPUT_DRAG_MIME,
    serializeCreativeOutputDrag({ runId, outputIndex }),
  )
}

// 下载本地保存的输出素材
function downloadOutput(runId: string, outputIndex: number, mimeType?: string): void {
  const asset = assetFor(runId, outputIndex)
  if (!asset) return
  const extension = (mimeType || asset.blob.type || 'image/png').split('/')[1] || 'png'
  saveAs(asset.blob, `creative-${runId.slice(0, 12)}-${outputIndex}.${extension}`)
}

function isActive(run: CreativeRun): boolean {
  return !CREATIVE_RUN_TERMINAL_STATUSES.includes(run.status)
}

// 失败、取消、结果丢失的任务可以按原参数重新发起；成功的任务直接再发新任务即可
function canRetry(run: CreativeRun): boolean {
  return CREATIVE_RUN_TERMINAL_STATUSES.includes(run.status) && run.status !== 'succeeded'
}

// 状态色调：结算、释放等中间阶段都归入进行中
function statusTone(run: CreativeRun): 'active' | 'success' | 'danger' | 'muted' {
  if (isActive(run)) return 'active'
  if (run.status === 'succeeded') return 'success'
  if (run.status === 'failed' || run.status === 'result_lost') return 'danger'
  return 'muted'
}

function formatRunTime(timestamp: number | undefined): string {
  const ms = creativeTimestampToMs(timestamp)
  if (ms == null) return ''
  return formatDateTime(new Date(ms))
}

function formatRunRelative(timestamp: number | undefined): string {
  const ms = creativeTimestampToMs(timestamp)
  if (ms == null) return ''
  return formatRelativeTime(new Date(ms))
}

// 活动任务实时计时，终态任务使用服务端完成时间固定显示最终耗时。
function formatElapsed(run: CreativeRun): string {
  return formatCreativeRunElapsed(run, elapsedNow.value)
}

async function refresh(): Promise<void> {
  if (refreshing.value || studio.loadingHistory.value) return
  refreshing.value = true
  // 先提交刷新状态，让图标在请求开始前完成一次绘制。
  await nextTick()
  try {
    await studio.refreshHistory()
  } finally {
    refreshing.value = false
  }
}
</script>

<style scoped>
/* 历史面板从右上入口展开；条目详情使用网格轨道实现真实高度折叠。 */
/* 历史面板动效用全局 pop-float,锚点方向(右上锚、向上收起)用局部变量表达;
   条目详情折叠(history-details)是网格轨道动画,保留本地。 */
.history-pop-float {
  --pop-shift: calc(-1 * var(--motion-shift));
}

.history-details-grid {
  display: grid;
  grid-template-rows: 1fr;
}

.history-details-enter-active,
.history-details-leave-active {
  overflow: hidden;
  transition:
    grid-template-rows var(--motion-layout) var(--motion-ease),
    opacity var(--motion-layout) var(--motion-ease);
}

.history-details-enter-from,
.history-details-leave-to {
  grid-template-rows: 0fr;
  opacity: 0;
}

.run-status {
  @apply inline-flex items-center gap-1.5 text-xs font-medium;
}

.run-status-active {
  @apply text-primary-700 dark:text-primary-500;
}

.run-status-success {
  @apply text-green-600 dark:text-green-400;
}

.run-status-danger {
  @apply text-red-600 dark:text-red-400;
}

.run-status-muted {
  @apply text-gray-500 dark:text-dark-300;
}

/* 输出图片右上角的操作按钮：有悬停能力的设备悬停或聚焦时显示，触屏设备常显 */
.history-output-actions {
  @apply absolute right-1.5 top-1.5 flex gap-1 opacity-0 transition-opacity duration-fast;
}

.history-output:hover .history-output-actions,
.history-output:focus-within .history-output-actions {
  @apply opacity-100;
}

@media (hover: none) {
  .history-output-actions {
    @apply opacity-100;
  }
}

.history-output-btn {
  @apply inline-flex h-8 w-8 items-center justify-center rounded-control bg-gray-900/60 text-white backdrop-blur transition-colors hover:bg-gray-900/80;
}

/* 发送动画到达时历史入口轻弹一次 */
.history-bump {
  animation: history-bump var(--motion-fast) var(--motion-ease);
}

@keyframes history-bump {
  50% {
    transform: scale(1.12);
  }
}

@media (prefers-reduced-motion: reduce) {
  /* 历史面板使用全局 pop-float，减少动态效果由全局样式处理。
     本地样式处理 history-details 折叠动画和入口弹跳。 */
  .history-details-enter-active,
  .history-details-leave-active {
    transition-duration: 1ms;
  }

  .history-bump {
    animation-duration: 1ms;
  }
}
</style>
