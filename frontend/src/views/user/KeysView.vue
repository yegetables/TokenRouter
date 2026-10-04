<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col gap-2">
          <div class="flex flex-col gap-2 lg:flex-row lg:items-start lg:justify-between">
            <div class="flex min-w-0 flex-1 items-center gap-2">
              <SearchInput
                v-model="filterSearch"
                :placeholder="t('keys.searchPlaceholder')"
                class="min-w-0 flex-1 sm:w-56 sm:flex-none lg:w-48 xl:w-64"
                @search="onFilterChange"
              />
              <FilterDropdown :active-count="activeFilterCount" :columns="2" keep-mounted @reset="resetKeyFilters">
                <FilterField :label="t('keys.group')">
                  <Select :model-value="filterGroupId" :options="groupFilterOptions" @update:model-value="onGroupFilterChange" />
                </FilterField>
                <FilterField :label="t('common.status')">
                  <Select :model-value="filterStatus" :options="statusFilterOptions" @update:model-value="onStatusFilterChange" />
                </FilterField>
              </FilterDropdown>
            </div>
            <div class="flex shrink-0 justify-end gap-2">
              <button
                @click="loadApiKeys"
                :disabled="loading"
                class="btn btn-secondary shrink-0 btn-icon"
                :title="t('common.refresh')"
              >
                <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
              </button>
              <div class="relative" ref="columnDropdownRef">
                <button
                  @click.stop="showColumnDropdown = !showColumnDropdown"
                  class="btn btn-secondary shrink-0 btn-icon"
                  :title="t('keys.columnSettings')"
                >
                  <Icon name="grid" size="sm" />
                </button>
                <MotionTransition name="dropdown-fade">
                  <div
                    v-if="showColumnDropdown" :inert="!(showColumnDropdown) || undefined"
                    class="dropdown right-0 top-full z-50 mt-2 max-h-menu w-52 overflow-y-auto p-2"
                  >
                    <button
                      v-for="column in toggleableColumns"
                      :key="column.key"
                      @click="toggleColumn(column.key)"
                      class="dropdown-item-sm justify-between rounded-control"
                    >
                      <span>{{ column.label }}</span>
                      <Icon
                        v-if="isColumnVisible(column.key)"
                        name="check"
                        size="sm"
                        class="text-primary-500"
                        :stroke-width="2"
                        :animate-on-hover="false"
                      />
                    </button>
                  </div>
                </MotionTransition>
              </div>
              <ScopeDropdown v-if="teamFeatureEnabled" v-model="scope" @change="onScopeChange" />
              <button @click="openCreateModal" class="btn btn-primary" data-tour="keys-create-btn">
                <Icon name="plus" size="sm" class="mr-2" />
                {{ t('keys.createKey') }}
              </button>
            </div>
          </div>
          <EndpointPopover
            v-if="publicSettings?.api_base_url || (publicSettings?.custom_endpoints?.length ?? 0) > 0"
            :api-base-url="publicSettings?.api_base_url || ''"
            :custom-endpoints="publicSettings?.custom_endpoints || []"
          />
        </div>
      </template>

      <template #table>
        <DataTable
          column-order-storage-key="user-keys-column-order"
          :columns="columns"
          :data="apiKeys"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <template #cell-id="{ value }">
            <span class="font-mono text-xs text-gray-500 dark:text-gray-400">#{{ value }}</span>
          </template>

          <template #cell-key="{ value, row }">
            <div class="flex items-center gap-2">
              <code class="code text-xs">
                {{ maskKey(value) }}
              </code>
              <button
                @click="copyToClipboard(value, row.id)"
                class="rounded-control p-1 transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
                :class="
                  copiedKeyId === row.id
                    ? 'text-green-500'
                    : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300'
                "
                :title="copiedKeyId === row.id ? t('keys.copied') : t('keys.copyToClipboard')"
              >
                <Icon
                  v-if="copiedKeyId === row.id"
                  name="check"
                  size="sm"
                  :stroke-width="2"
                  :animate-on-hover="false"
                />
                <Icon v-else name="clipboard" size="sm" />
              </button>
            </div>
          </template>

          <template #cell-name="{ value, row }">
            <div class="flex items-center gap-1.5">
              <span class="font-medium text-gray-900 dark:text-white">{{ value }}</span>
              <Icon
                v-if="row.ip_whitelist?.length > 0 || row.ip_blacklist?.length > 0"
                name="shield"
                size="sm"
                class="text-blue-500"
                :title="t('keys.ipRestrictionEnabled')"
              />
              <!-- 并发大于 0 时，名称后面显示并发数和上限。 -->
              <span
                v-if="(row.current_concurrency ?? 0) > 0"
                data-test="key-concurrency"
                :class="['ml-1 inline-flex items-center gap-1 text-xs tabular-nums', concurrencyTone(row).text]"
                :title="concurrencyTitle(row)"
                :aria-label="concurrencyTitle(row)"
              >
                <span :class="['h-1.5 w-1.5 rounded-full', concurrencyTone(row).dot]" aria-hidden="true" />
                <span>
                  <span>{{ row.current_concurrency }}</span>
                  <span v-if="(row.concurrency_limit ?? 0) > 0" class="text-gray-400 dark:text-dark-500">/{{ row.concurrency_limit }}</span>
                </span>
              </span>
            </div>
          </template>

          <template #cell-group="{ row }">
            <button
              v-if="row.is_composite"
              type="button"
              data-test="composite-group-summary"
              class="-mx-1 -my-1 flex max-w-[22rem] flex-wrap items-center gap-1.5 rounded-control p-1 text-left transition duration-normal hover:bg-gray-100 dark:hover:bg-dark-700"
              :title="t('keys.composite.editMappings')"
              @click="editKey(row)"
            >
              <!-- 复合 Key 用品牌色胶囊展示“前缀 / 分组”，超出部分折叠为计数胶囊。 -->
              <span
                v-for="binding in visibleCompositeGroups(row)"
                :key="`${row.id}-${binding.group_id}`"
                :class="[
                  'inline-flex min-w-0 items-center gap-1.5 rounded-compact px-2 py-0.5 text-xs leading-5',
                  compositeGroupChipClass(binding.group?.display_brand)
                ]"
              >
                <span class="max-w-24 truncate font-mono font-medium">{{ binding.prefix }}</span>
                <span class="opacity-40">/</span>
                <span class="max-w-28 truncate opacity-75">{{ binding.group?.name || `#${binding.group_id}` }}</span>
              </span>
              <span
                v-if="hiddenCompositeGroupCount(row) > 0"
                class="inline-flex items-center rounded-compact bg-gray-100 px-2 py-0.5 text-xs leading-5 text-gray-500 dark:bg-dark-800 dark:text-dark-400"
              >
                {{ t('keys.composite.moreMappings', { count: hiddenCompositeGroupCount(row) }) }}
              </span>
            </button>
            <div v-else class="group/dropdown relative">
              <button
                :ref="(el) => setGroupButtonRef(row.id, el)"
                @click="openGroupSelector(row)"
                class="-mx-2 -my-1 flex cursor-pointer items-center gap-2 rounded-control px-2 py-1 transition duration-normal hover:bg-gray-100 dark:hover:bg-dark-700"
                :title="t('keys.clickToChangeGroup')"
              >
                <GroupBadge
                  v-if="row.group"
                  :name="row.group.name"
                  :display-brand="row.group.display_brand"
                  :rate-multiplier="row.group.rate_multiplier"
                  :user-rate-multiplier="userGroupRates[row.group.id]"
                />
                <span v-else class="text-sm text-gray-400 dark:text-dark-500">{{
                  t('keys.noGroup')
                }}</span>
                <Icon
                  name="sort"
                  size="xs"
                  :animate-on-hover="false"
                  class="h-3.5 w-3.5 text-gray-400 opacity-60 transition-opacity group-hover/dropdown:opacity-100"
                />
              </button>
            </div>
          </template>

          <template #cell-usage="{ row }">
            <!-- 窄屏卡片的用量靠右排列，桌面表格保持左对齐。 -->
            <div class="text-sm">
              <div v-if="usageLoading && !usageStats[row.id]" class="flex h-10 flex-col items-end justify-center gap-2 lg:items-start" role="status" :aria-label="t('common.loading')" aria-busy="true" data-loading-skeleton>
                <Skeleton :width="96" :height="12" />
                <Skeleton :width="112" :height="12" />
              </div>
              <div v-else class="space-y-0.5">
                <div class="flex items-center justify-end gap-1.5 lg:justify-start">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('keys.today') }}:</span>
                  <span class="font-medium text-gray-900 dark:text-white">
                    {{ formatBalanceAmount(usageStats[row.id]?.today_actual_cost ?? 0, { fractionDigits: 4 }) }}
                  </span>
                </div>
                <!-- 批量接口的 total_actual_cost 统计近 30 天，使用对应文案标明范围。 -->
                <div class="flex items-center justify-end gap-1.5 lg:justify-start">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('keys.total') }}:</span>
                  <span class="font-medium text-gray-900 dark:text-white">
                    {{ formatBalanceAmount(usageStats[row.id]?.total_actual_cost ?? 0, { fractionDigits: 4 }) }}
                  </span>
                </div>
              </div>
              <!-- Quota progress (if quota is set) -->
              <div v-if="row.quota > 0" class="mt-1.5">
                <div class="flex items-center justify-end gap-1.5 lg:justify-start">
                  <span class="text-gray-500 dark:text-gray-400">{{ t('keys.quota') }}:</span>
                  <span :class="[
                    'font-medium',
                    row.quota_used >= row.quota ? 'text-red-500' :
                    row.quota_used >= row.quota * 0.8 ? 'text-yellow-500' :
                    'text-gray-900 dark:text-white'
                  ]">
                    {{ formatBalancePair(row.quota_used, row.quota, 2, 2) }}
                  </span>
                </div>
                <div class="mt-1 h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                  <div
                    :class="[
                      'h-full rounded-full transition-[width,background-color]',
                      row.quota_used >= row.quota ? 'bg-red-500' :
                      row.quota_used >= row.quota * 0.8 ? 'bg-yellow-500' :
                      'bg-primary-500'
                    ]"
                    :style="{ width: Math.min((row.quota_used / row.quota) * 100, 100) + '%' }"
                  />
                </div>
              </div>
            </div>
          </template>

          <template #cell-rate_limit="{ row }">
            <div v-if="row.rate_limit_5h > 0 || row.rate_limit_1d > 0 || row.rate_limit_7d > 0" class="space-y-1.5 min-w-[140px]">
              <!-- 5h window -->
              <div v-if="row.rate_limit_5h > 0">
                <div class="flex items-center justify-between text-xs">
                  <span class="text-gray-500 dark:text-gray-400">5h</span>
                  <span :class="[
                    'font-medium tabular-nums',
                    row.usage_5h >= row.rate_limit_5h ? 'text-red-500' :
                    row.usage_5h >= row.rate_limit_5h * 0.8 ? 'text-yellow-500' :
                    'text-gray-700 dark:text-gray-300'
                  ]">
                    {{ formatBalancePair(row.usage_5h, row.rate_limit_5h, 2, 2, false) }}
                  </span>
                </div>
                <div class="h-1 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                  <div
                    :class="[
                      'h-full rounded-full transition-[width,background-color]',
                      row.usage_5h >= row.rate_limit_5h ? 'bg-red-500' :
                      row.usage_5h >= row.rate_limit_5h * 0.8 ? 'bg-yellow-500' :
                      'bg-emerald-500'
                    ]"
                    :style="{ width: Math.min((row.usage_5h / row.rate_limit_5h) * 100, 100) + '%' }"
                  />
                </div>
                <div v-if="row.reset_5h_at && formatResetTime(row.reset_5h_at)" class="text-xs text-gray-400 dark:text-gray-500 tabular-nums">
                  ⟳ {{ formatResetTime(row.reset_5h_at) }}
                </div>
              </div>
              <!-- 1d window -->
              <div v-if="row.rate_limit_1d > 0">
                <div class="flex items-center justify-between text-xs">
                  <span class="text-gray-500 dark:text-gray-400">1d</span>
                  <span :class="[
                    'font-medium tabular-nums',
                    row.usage_1d >= row.rate_limit_1d ? 'text-red-500' :
                    row.usage_1d >= row.rate_limit_1d * 0.8 ? 'text-yellow-500' :
                    'text-gray-700 dark:text-gray-300'
                  ]">
                    {{ formatBalancePair(row.usage_1d, row.rate_limit_1d, 2, 2, false) }}
                  </span>
                </div>
                <div class="h-1 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                  <div
                    :class="[
                      'h-full rounded-full transition-[width,background-color]',
                      row.usage_1d >= row.rate_limit_1d ? 'bg-red-500' :
                      row.usage_1d >= row.rate_limit_1d * 0.8 ? 'bg-yellow-500' :
                      'bg-emerald-500'
                    ]"
                    :style="{ width: Math.min((row.usage_1d / row.rate_limit_1d) * 100, 100) + '%' }"
                  />
                </div>
                <div v-if="row.reset_1d_at && formatResetTime(row.reset_1d_at)" class="text-xs text-gray-400 dark:text-gray-500 tabular-nums">
                  ⟳ {{ formatResetTime(row.reset_1d_at) }}
                </div>
              </div>
              <!-- 7d window -->
              <div v-if="row.rate_limit_7d > 0">
                <div class="flex items-center justify-between text-xs">
                  <span class="text-gray-500 dark:text-gray-400">7d</span>
                  <span :class="[
                    'font-medium tabular-nums',
                    row.usage_7d >= row.rate_limit_7d ? 'text-red-500' :
                    row.usage_7d >= row.rate_limit_7d * 0.8 ? 'text-yellow-500' :
                    'text-gray-700 dark:text-gray-300'
                  ]">
                    {{ formatBalancePair(row.usage_7d, row.rate_limit_7d, 2, 2, false) }}
                  </span>
                </div>
                <div class="h-1 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                  <div
                    :class="[
                      'h-full rounded-full transition-[width,background-color]',
                      row.usage_7d >= row.rate_limit_7d ? 'bg-red-500' :
                      row.usage_7d >= row.rate_limit_7d * 0.8 ? 'bg-yellow-500' :
                      'bg-emerald-500'
                    ]"
                    :style="{ width: Math.min((row.usage_7d / row.rate_limit_7d) * 100, 100) + '%' }"
                  />
                </div>
                <div v-if="row.reset_7d_at && formatResetTime(row.reset_7d_at)" class="text-xs text-gray-400 dark:text-gray-500 tabular-nums">
                  ⟳ {{ formatResetTime(row.reset_7d_at) }}
                </div>
              </div>
              <!-- Reset button -->
              <button
                v-if="row.usage_5h > 0 || row.usage_1d > 0 || row.usage_7d > 0"
                @click.stop="confirmResetRateLimitFromTable(row)"
                class="mt-0.5 inline-flex items-center gap-1 rounded-compact px-1.5 py-0.5 text-xs text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                :title="t('keys.resetRateLimitUsage')"
              >
                <Icon name="refresh" size="xs" />
                {{ t('keys.resetUsage') }}
              </button>
            </div>
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
          </template>

          <template #cell-expires_at="{ value }">
            <span v-if="value" :class="[
              'text-sm',
              new Date(value) < new Date() ? 'text-red-500 dark:text-red-400' : 'text-gray-500 dark:text-dark-400'
            ]">
              {{ formatDateTime(value) }}
            </span>
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">{{ t('keys.noExpiration') }}</span>
          </template>

          <template #cell-status="{ value, row }">
            <span :class="[
              'badge gap-1',
              row.team_owner_disabled ? 'badge-danger' :
              value === 'active' ? 'badge-success' :
              value === 'quota_exhausted' ? 'badge-warning' :
              value === 'expired' ? 'badge-danger' :
              'badge-gray'
            ]">
              <Icon v-if="row.team_owner_disabled" name="lock" size="xs" />
              {{ row.team_owner_disabled ? t('keys.status.team_owner_disabled') : t('keys.status.' + value) }}
            </span>
          </template>

          <template #cell-last_used_at="{ value }">
            <span v-if="value" class="text-sm text-gray-500 dark:text-dark-400">
              {{ formatDateTime(value) }}
            </span>
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
          </template>

          <template #cell-last_used_ip="{ value }">
            <span v-if="value" class="break-all font-mono text-xs text-gray-500 dark:text-dark-400">
              {{ value }}
            </span>
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
          </template>

          <template #cell-created_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatDateTime(value) }}</span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <!-- 高频操作固定展示，低频和危险操作收进更多菜单。 -->
              <button
                @click="editKey(row)"
                class="flex flex-col items-center gap-0.5 rounded-control p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              >
                <Icon name="edit" size="sm" />
                <span class="text-xs">{{ t('common.edit') }}</span>
              </button>
              <!-- Owner 锁定由团队管理员解除。 -->
              <span
                v-if="row.team_owner_disabled"
                class="flex cursor-not-allowed flex-col items-center gap-0.5 rounded-control p-1.5 text-amber-600 dark:text-amber-400"
                :title="t('keys.teamOwnerDisabledHint')"
              >
                <Icon name="lock" size="sm" />
                <span class="text-xs">{{ t('keys.teamOwnerLocked') }}</span>
              </span>
              <!-- Toggle Status Button -->
              <button
                v-else
                @click="toggleKeyStatus(row)"
                :class="[
                  'flex flex-col items-center gap-0.5 rounded-control p-1.5 transition-colors',
                  row.status === 'active'
                    ? 'text-gray-500 hover:bg-yellow-50 hover:text-yellow-600 dark:hover:bg-yellow-900/20 dark:hover:text-yellow-400'
                    : 'text-gray-500 hover:bg-green-50 hover:text-green-600 dark:hover:bg-green-900/20 dark:hover:text-green-400'
                ]"
              >
                <Icon v-if="row.status === 'active'" name="ban" size="sm" />
                <Icon v-else name="checkCircle" size="sm" :animate-on-hover="false" />
                <span class="text-xs">{{ row.status === 'active' ? t('keys.disable') : t('keys.enable') }}</span>
              </button>
              <button
                class="key-action-menu-trigger flex flex-col items-center gap-0.5 rounded-control p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:hover:bg-dark-700 dark:hover:text-white"
                :class="{ 'bg-gray-100 text-gray-900 dark:bg-dark-700 dark:text-white': actionMenuKey?.id === row.id }"
                aria-haspopup="menu"
                :aria-expanded="actionMenuKey?.id === row.id"
                :aria-controls="actionMenuKey?.id === row.id ? `key-action-menu-${row.id}` : undefined"
                @click="openKeyActionMenu(row, $event)"
              >
                <Icon name="more" size="sm" />
                <span class="text-xs">{{ t('common.more') }}</span>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('keys.noKeysYet')"
              :description="t('keys.createFirstKey')"
              :action-text="t('keys.createKey')"
              @action="openCreateModal"
            />
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <!-- 创建和编辑密钥共用弹窗，表单随窄屏收缩。 -->
    <BaseDialog
      :show="showCreateModal || showEditModal"
      :title="showEditModal ? t('keys.editKey') : t('keys.createKey')"
      width="normal"
      @close="closeModals"
    >
      <form id="key-form" @submit.prevent="handleSubmit" class="key-form-controls min-w-0 max-w-full space-y-5">
        <div>
          <label class="input-label">{{ t('keys.nameLabel') }}</label>
          <input
            v-model="formData.name"
            type="text"
            required
            class="input"
            :placeholder="t('keys.namePlaceholder')"
            data-tour="key-form-name"
          />
        </div>

        <!-- 复合模式使用项目 Toggle，切换后改为完整映射编辑。 -->
        <div class="flex items-center justify-between gap-4">
          <div>
            <label class="input-label mb-0">{{ t('keys.composite.label') }}</label>
            <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('keys.composite.hint') }}</p>
          </div>
          <Toggle
            :model-value="formData.is_composite"
            size="sm"
            data-test="composite-key-toggle"
            @update:model-value="onCompositeModeChange"
          />
        </div>

        <div class="space-y-3">
          <label class="input-label">{{ t('keys.billing.modeLabel') }}</label>
          <Select
            v-model="formData.billing_mode"
            :options="billingModeOptions"
            :placeholder="t('keys.billing.modeLabel')"
            data-test="api-key-billing-mode"
            @change="onBillingModeChange"
          />

          <div v-if="formData.billing_mode === 'subscription'">
            <label class="input-label">{{ t('keys.billing.subscriptionLabel') }}</label>
            <Select
              v-model="formData.preferred_subscription_id"
              :options="billingSubscriptionOptions"
              :placeholder="t('keys.billing.selectSubscription')"
              :searchable="true"
              :disabled="billingOptionsLoading"
              data-test="api-key-preferred-subscription"
              @change="onPreferredSubscriptionChange"
            />
          </div>
        </div>

        <div v-if="!formData.is_composite">
          <label class="input-label">{{ t('keys.groupLabel') }}</label>
          <Select
            v-model="formData.group_id"
            :options="formGroupOptions"
            :placeholder="t('keys.selectGroup')"
            :searchable="true"
            :search-placeholder="t('keys.searchGroup')"
            :disabled="formGroupsLoading"
            data-tour="key-form-group"
          >
            <template #selected="{ option }">
              <GroupBadge
                v-if="option"
                :name="(option as unknown as GroupOption).label"
                :display-brand="(option as unknown as GroupOption).displayBrand"
                :rate-multiplier="(option as unknown as GroupOption).rate"
                :user-rate-multiplier="(option as unknown as GroupOption).userRate"
              />
              <span v-else class="text-gray-400">{{ t('keys.selectGroup') }}</span>
            </template>
            <template #option="{ option, selected }">
              <GroupOptionItem
                :name="(option as unknown as GroupOption).label"
                :display-brand="(option as unknown as GroupOption).displayBrand"
                :rate-multiplier="(option as unknown as GroupOption).rate"
                :user-rate-multiplier="(option as unknown as GroupOption).userRate"
                :description="(option as unknown as GroupOption).description"
                :selected="selected"
              />
            </template>
          </Select>
        </div>

        <RuleListEditor
          v-else
          :items="formData.composite_groups"
          :item-key="(binding) => binding.local_id"
          :add-label="t('keys.composite.addMapping')"
          add-placement="footer"
          :min="1"
          :max="20"
          reorderable
          data-test="composite-group-editor"
          @add="addCompositeBinding"
          @remove="removeCompositeBinding"
          @move="moveCompositeBinding"
        >
          <template #row="{ item: binding, index }">
            <div class="grid min-w-0 grid-cols-1 items-start gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(7rem,0.65fr)]">
              <Select
                v-model="binding.group_id"
                :options="formGroupOptions"
                :placeholder="t('keys.selectGroup')"
                :searchable="true"
                :disabled="formGroupsLoading"
                class="min-w-0"
              />
              <div class="min-w-0">
                <input
                  v-model="binding.prefix"
                  type="text"
                  maxlength="32"
                  class="input min-w-0 font-mono"
                  :class="{ 'input-error': compositeBindingError(index) }"
                  :placeholder="t('keys.composite.prefixPlaceholder')"
                />
                <p v-if="compositeBindingError(index)" class="input-error-text">
                  {{ compositeBindingError(index) }}
                </p>
              </div>
            </div>
          </template>
        </RuleListEditor>

        <!-- 单 Key Fast 策略使用项目统一选择框，系统策略仍在服务端最终裁决。 -->
        <div>
          <label class="input-label">{{ t('keys.fastModePolicyLabel') }}</label>
          <Select
            v-model="formData.fast_mode_policy"
            :options="fastModePolicyOptions"
            :placeholder="t('keys.fastModePolicyLabel')"
            data-test="fast-mode-policy-select"
          />
        </div>

        <!-- 分组停用时的请求级自动降级开关。 -->
        <div class="flex items-center justify-between">
          <label class="input-label mb-0">{{ t('keys.fallbackWhenGroupUnavailable') }}</label>
          <Toggle v-model="formData.fallback_when_group_unavailable" size="sm" />
        </div>

        <!-- 模型重定向按行编辑，删除全部行会在更新时提交空对象。 -->
        <ModelMappingEditor
          v-model="formData.model_mapping_rows"
          :title="t('keys.modelRedirect.label')"
          :hint="t('keys.modelRedirect.hint')"
          :add-label="t('keys.modelRedirect.addRule')"
          :empty-text="t('keys.modelRedirect.empty')"
          :max="MODEL_REDIRECT_MAX_RULES"
          :source-label="t('keys.modelRedirect.source')"
          :target-label="t('keys.modelRedirect.target')"
          :source-placeholder="t('keys.modelRedirect.sourcePlaceholder')"
          :target-placeholder="t('keys.modelRedirect.targetPlaceholder')"
          :field-errors="modelMappingFieldErrors"
          test-id="model-mapping"
          data-test="model-mapping-editor"
        >
          <template #title-suffix>
            <ModelRedirectHelp />
          </template>
        </ModelMappingEditor>

        <!-- Custom Key Section (only for create) -->
        <div v-if="!showEditModal" class="space-y-3">
          <div class="flex items-center justify-between">
            <label class="input-label mb-0">{{ t('keys.customKeyLabel') }}</label>
            <Toggle v-model="formData.use_custom_key" size="sm" off-tone="soft" />
          </div>
          <div v-if="formData.use_custom_key">
            <input
              v-model="formData.custom_key"
              type="text"
              class="input font-mono"
              :placeholder="t('keys.customKeyPlaceholder')"
              :class="{ 'border-red-500 dark:border-red-500': customKeyError }"
            />
            <p v-if="customKeyError" class="mt-1 text-sm text-red-500">{{ customKeyError }}</p>
            <p v-else class="input-hint">{{ t('keys.customKeyHint') }}</p>
          </div>
        </div>

        <div v-if="showEditModal" class="space-y-3">
          <div class="flex items-center justify-between gap-4">
            <label class="input-label mb-0">{{ t('keys.statusLabel') }}</label>
            <Toggle
              :model-value="formData.status === 'active'"
              :disabled="Boolean(selectedKey?.team_owner_disabled)"
              :aria-label="t('keys.statusLabel')"
              :aria-describedby="selectedKey?.team_owner_disabled ? 'team-owner-disabled-hint' : undefined"
              data-test="key-status-toggle"
              @update:model-value="onStatusToggle"
            />
          </div>
          <p
            v-if="selectedKey?.team_owner_disabled"
            id="team-owner-disabled-hint"
            class="mt-2 flex items-start gap-2 rounded-control border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:border-amber-900/60 dark:bg-amber-900/20 dark:text-amber-300"
          >
            <Icon name="lock" size="sm" class="mt-0.5 shrink-0" />
            <span>{{ t('keys.teamOwnerDisabledHint') }}</span>
          </p>
        </div>

        <!-- IP Restriction Section -->
        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <label class="input-label mb-0">{{ t('keys.ipRestriction') }}</label>
            <Toggle v-model="formData.enable_ip_restriction" size="sm" off-tone="soft" />
          </div>

          <div v-if="formData.enable_ip_restriction" class="space-y-4 pt-2">
            <div>
              <label class="input-label">{{ t('keys.ipWhitelist') }}</label>
              <textarea
                v-model="formData.ip_whitelist"
                rows="3"
                class="input font-mono text-sm"
                :placeholder="t('keys.ipWhitelistPlaceholder')"
              />
              <p class="input-hint">{{ t('keys.ipWhitelistHint') }}</p>
            </div>

            <div>
              <label class="input-label">{{ t('keys.ipBlacklist') }}</label>
              <textarea
                v-model="formData.ip_blacklist"
                rows="3"
                class="input font-mono text-sm"
                :placeholder="t('keys.ipBlacklistPlaceholder')"
              />
              <p class="input-hint">{{ t('keys.ipBlacklistHint') }}</p>
            </div>
          </div>
        </div>

        <!-- Quota Limit Section -->
        <div class="space-y-3">
          <label class="input-label">{{ t('keys.quotaLimit') }}</label>
          <!-- Switch commented out - always show input, 0 = unlimited
          <div class="flex items-center justify-between">
            <label class="input-label mb-0">{{ t('keys.quotaLimit') }}</label>
            <Toggle v-model="formData.enable_quota" size="sm" off-tone="soft" />
          </div>
          -->

          <div class="space-y-4">
            <div>
              <div class="relative">
                <span class="input-icon text-gray-500">{{ balanceUnitSymbol }}</span>
                <input
                  v-model.number="formData.quota"
                  type="number"
                  step="0.01"
                  min="0"
                  class="input input-has-icon input-icon-text"
                  :placeholder="t('keys.quotaAmountPlaceholder')"
                />
              </div>
              <p class="input-hint">{{ t('keys.quotaAmountHint') }}</p>
            </div>

            <!-- Quota used display (only in edit mode) -->
            <div v-if="showEditModal && selectedKey && selectedKey.quota > 0">
              <label class="input-label">{{ t('keys.quotaUsed') }}</label>
              <div class="flex items-center gap-2">
                <div class="flex-1 h-9 rounded-control bg-gray-100 px-3 py-1.5 dark:bg-dark-700">
                  <span class="font-medium text-gray-900 dark:text-white">
                    {{ formatBalanceAmount(selectedKey.quota_used, { fractionDigits: 4 }) }}
                  </span>
                  <span class="mx-2 text-gray-400">/</span>
                  <span class="text-gray-500 dark:text-gray-400">
                    {{ formatBalanceAmount(selectedKey.quota, { fractionDigits: 2 }) }}
                  </span>
                </div>
                <button
                  type="button"
                  @click="confirmResetQuota"
                  class="btn btn-secondary text-sm"
                  :title="t('keys.resetQuotaUsed')"
                >
                  {{ t('keys.reset') }}
                </button>
              </div>
            </div>
          </div>
        </div>

        <!-- Key 请求限制与金额限额分别配置。 -->
        <div class="space-y-3">
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div>
              <label for="key-concurrency-limit" class="input-label">{{ t('keys.concurrencyLimit') }}</label>
              <input id="key-concurrency-limit" v-model.number="formData.concurrency_limit" type="number" min="0" max="2147483647" step="1" class="input" placeholder="0" />
            </div>
            <div>
              <label for="key-rpm-limit" class="input-label">{{ t('keys.rpmLimit') }}</label>
              <input id="key-rpm-limit" v-model.number="formData.rpm_limit" type="number" min="0" max="2147483647" step="1" class="input" placeholder="0" />
            </div>
          </div>
          <p class="input-hint">{{ t('keys.requestLimitsHint') }}</p>
        </div>

        <!-- Rate Limit Section -->
        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <label class="input-label mb-0">{{ t('keys.rateLimitSection') }}</label>
            <Toggle v-model="formData.enable_rate_limit" size="sm" off-tone="soft" />
          </div>

          <div v-if="formData.enable_rate_limit" class="space-y-4 pt-2">
            <p class="input-hint -mt-2">{{ t('keys.rateLimitHint') }}</p>
            <!-- 5-Hour Limit -->
            <div>
              <label class="input-label">{{ t('keys.rateLimit5h') }}</label>
              <div class="relative">
                <span class="input-icon text-gray-500">{{ balanceUnitSymbol }}</span>
                <input
                  v-model.number="formData.rate_limit_5h"
                  type="number"
                  step="0.01"
                  min="0"
                  class="input input-has-icon input-icon-text"
                  :placeholder="'0'"
                />
              </div>
              <!-- Usage info (edit mode only) -->
              <div v-if="showEditModal && selectedKey && selectedKey.rate_limit_5h > 0" class="mt-2">
                <div class="flex items-center gap-2">
                  <div class="flex-1 h-9 rounded-control bg-gray-100 px-3 py-1.5 text-sm dark:bg-dark-700">
                    <span :class="[
                      'font-medium',
                      selectedKey.usage_5h >= selectedKey.rate_limit_5h ? 'text-red-500' :
                      selectedKey.usage_5h >= selectedKey.rate_limit_5h * 0.8 ? 'text-yellow-500' :
                      'text-gray-900 dark:text-white'
                    ]">
                      {{ formatBalanceAmount(selectedKey.usage_5h, { fractionDigits: 4 }) }}
                    </span>
                    <span class="mx-2 text-gray-400">/</span>
                    <span class="text-gray-500 dark:text-gray-400">
                      {{ formatBalanceAmount(selectedKey.rate_limit_5h, { fractionDigits: 2 }) }}
                    </span>
                  </div>
                </div>
                <div class="mt-1 h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                  <div
                    :class="[
                      'h-full rounded-full transition-[width,background-color]',
                      selectedKey.usage_5h >= selectedKey.rate_limit_5h ? 'bg-red-500' :
                      selectedKey.usage_5h >= selectedKey.rate_limit_5h * 0.8 ? 'bg-yellow-500' :
                      'bg-green-500'
                    ]"
                    :style="{ width: Math.min((selectedKey.usage_5h / selectedKey.rate_limit_5h) * 100, 100) + '%' }"
                  />
                </div>
              </div>
            </div>

            <!-- Daily Limit -->
            <div>
              <label class="input-label">{{ t('keys.rateLimit1d') }}</label>
              <div class="relative">
                <span class="input-icon text-gray-500">{{ balanceUnitSymbol }}</span>
                <input
                  v-model.number="formData.rate_limit_1d"
                  type="number"
                  step="0.01"
                  min="0"
                  class="input input-has-icon input-icon-text"
                  :placeholder="'0'"
                />
              </div>
              <!-- Usage info (edit mode only) -->
              <div v-if="showEditModal && selectedKey && selectedKey.rate_limit_1d > 0" class="mt-2">
                <div class="flex items-center gap-2">
                  <div class="flex-1 h-9 rounded-control bg-gray-100 px-3 py-1.5 text-sm dark:bg-dark-700">
                    <span :class="[
                      'font-medium',
                      selectedKey.usage_1d >= selectedKey.rate_limit_1d ? 'text-red-500' :
                      selectedKey.usage_1d >= selectedKey.rate_limit_1d * 0.8 ? 'text-yellow-500' :
                      'text-gray-900 dark:text-white'
                    ]">
                      {{ formatBalanceAmount(selectedKey.usage_1d, { fractionDigits: 4 }) }}
                    </span>
                    <span class="mx-2 text-gray-400">/</span>
                    <span class="text-gray-500 dark:text-gray-400">
                      {{ formatBalanceAmount(selectedKey.rate_limit_1d, { fractionDigits: 2 }) }}
                    </span>
                  </div>
                </div>
                <div class="mt-1 h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                  <div
                    :class="[
                      'h-full rounded-full transition-[width,background-color]',
                      selectedKey.usage_1d >= selectedKey.rate_limit_1d ? 'bg-red-500' :
                      selectedKey.usage_1d >= selectedKey.rate_limit_1d * 0.8 ? 'bg-yellow-500' :
                      'bg-green-500'
                    ]"
                    :style="{ width: Math.min((selectedKey.usage_1d / selectedKey.rate_limit_1d) * 100, 100) + '%' }"
                  />
                </div>
              </div>
            </div>

            <!-- 7-Day Limit -->
            <div>
              <label class="input-label">{{ t('keys.rateLimit7d') }}</label>
              <div class="relative">
                <span class="input-icon text-gray-500">{{ balanceUnitSymbol }}</span>
                <input
                  v-model.number="formData.rate_limit_7d"
                  type="number"
                  step="0.01"
                  min="0"
                  class="input input-has-icon input-icon-text"
                  :placeholder="'0'"
                />
              </div>
              <!-- Usage info (edit mode only) -->
              <div v-if="showEditModal && selectedKey && selectedKey.rate_limit_7d > 0" class="mt-2">
                <div class="flex items-center gap-2">
                  <div class="flex-1 h-9 rounded-control bg-gray-100 px-3 py-1.5 text-sm dark:bg-dark-700">
                    <span :class="[
                      'font-medium',
                      selectedKey.usage_7d >= selectedKey.rate_limit_7d ? 'text-red-500' :
                      selectedKey.usage_7d >= selectedKey.rate_limit_7d * 0.8 ? 'text-yellow-500' :
                      'text-gray-900 dark:text-white'
                    ]">
                      {{ formatBalanceAmount(selectedKey.usage_7d, { fractionDigits: 4 }) }}
                    </span>
                    <span class="mx-2 text-gray-400">/</span>
                    <span class="text-gray-500 dark:text-gray-400">
                      {{ formatBalanceAmount(selectedKey.rate_limit_7d, { fractionDigits: 2 }) }}
                    </span>
                  </div>
                </div>
                <div class="mt-1 h-1.5 w-full overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                  <div
                    :class="[
                      'h-full rounded-full transition-[width,background-color]',
                      selectedKey.usage_7d >= selectedKey.rate_limit_7d ? 'bg-red-500' :
                      selectedKey.usage_7d >= selectedKey.rate_limit_7d * 0.8 ? 'bg-yellow-500' :
                      'bg-green-500'
                    ]"
                    :style="{ width: Math.min((selectedKey.usage_7d / selectedKey.rate_limit_7d) * 100, 100) + '%' }"
                  />
                </div>
              </div>
            </div>

            <!-- Reset Rate Limit button (edit mode only) -->
            <div v-if="showEditModal && selectedKey && (selectedKey.rate_limit_5h > 0 || selectedKey.rate_limit_1d > 0 || selectedKey.rate_limit_7d > 0)">
              <button
                type="button"
                @click="confirmResetRateLimit"
                class="btn btn-secondary text-sm"
              >
                {{ t('keys.resetRateLimitUsage') }}
              </button>
            </div>
          </div>
        </div>

        <!-- Expiration Section -->
        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <label class="input-label mb-0">{{ t('keys.expiration') }}</label>
            <Toggle v-model="formData.enable_expiration" size="sm" off-tone="soft" />
          </div>

          <div v-if="formData.enable_expiration" class="space-y-4 pt-2">
            <!-- Quick select buttons (for both create and edit mode) -->
            <div class="flex flex-wrap gap-2">
              <button
                v-for="days in ['7', '30', '90']"
                :key="days"
                type="button"
                @click="setExpirationDays(parseInt(days))"
                :class="[
                  'rounded-control px-3 py-1.5 text-sm transition-colors',
                  formData.expiration_preset === days
                    ? 'bg-primary-100 text-primary-700 dark:bg-primary-500/8 dark:text-primary-500'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-400 dark:hover:bg-dark-600'
                ]"
              >
                {{ showEditModal ? t('keys.extendDays', { days }) : t('keys.expiresInDays', { days }) }}
              </button>
              <button
                type="button"
                @click="formData.expiration_preset = 'custom'"
                :class="[
                  'rounded-control px-3 py-1.5 text-sm transition-colors',
                  formData.expiration_preset === 'custom'
                    ? 'bg-primary-100 text-primary-700 dark:bg-primary-500/8 dark:text-primary-500'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-400 dark:hover:bg-dark-600'
                ]"
              >
                {{ t('keys.customDate') }}
              </button>
            </div>

            <!-- Date picker (always show for precise adjustment) -->
            <div>
              <label class="input-label">{{ t('keys.expirationDate') }}</label>
              <input
                v-model="formData.expiration_date"
                type="datetime-local"
                class="input"
              />
              <p class="input-hint">{{ t('keys.expirationDateHint') }}</p>
            </div>

            <!-- Current expiration display (only in edit mode) -->
            <div v-if="showEditModal && selectedKey?.expires_at" class="text-sm">
              <span class="text-gray-500 dark:text-gray-400">{{ t('keys.currentExpiration') }}: </span>
              <span class="font-medium text-gray-900 dark:text-white">
                {{ formatDateTime(selectedKey.expires_at) }}
              </span>
            </div>
          </div>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeModals" type="button" class="btn btn-secondary py-1.5">
            {{ t('common.cancel') }}
          </button>
          <button
            form="key-form"
            type="submit"
            :disabled="submitting"
            class="btn btn-primary py-1.5"
            data-tour="key-form-submit"
          >
            <Icon
              name="loader"
              size="sm"
              :animate-on-hover="false"
              v-if="submitting"
              class="-ml-1 mr-2 h-4 w-4 animate-spin"
            />
            {{
              submitting
                ? t('keys.saving')
                : showEditModal
                  ? t('common.update')
                  : t('common.create')
            }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- 轮换前明确告知旧凭据失效，提交期间保留弹窗以防重复操作。 -->
    <ConfirmDialog
      :show="rotationKey !== null"
      :title="t('keys.rotateKey')"
      :message="t('keys.rotateConfirmMessage', { name: rotationKey?.name })"
      :confirm-text="t('keys.confirmRotate')"
      :danger="true"
      :loading="rotatingKey"
      @confirm="handleRotate"
      @cancel="cancelRotate"
    />

    <!-- 轮换成功后展示新凭据，便于立即复制到客户端。 -->
    <BaseDialog
      :show="rotatedKey !== null"
      :title="t('keys.keyRotatedSuccess')"
      width="narrow"
      @close="rotatedKey = null"
    >
      <div class="space-y-4">
        <p class="text-sm text-gray-600 dark:text-gray-400">{{ t('keys.rotatedKeyHint') }}</p>
        <code class="block break-all rounded-surface bg-gray-50 p-4 text-sm text-gray-900 dark:bg-dark-800 dark:text-gray-100">
          {{ rotatedKey?.key }}
        </code>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="rotatedKey = null">
            {{ t('common.close') }}
          </button>
          <button
            type="button"
            class="btn btn-primary"
            @click="rotatedKey && copyToClipboard(rotatedKey.key, rotatedKey.id)"
          >
            {{ t('common.copy') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Delete Confirmation Dialog -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('keys.deleteKey')"
      :message="t('keys.deleteConfirmMessage', { name: selectedKey?.name })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="handleDelete"
      @cancel="showDeleteDialog = false"
    />

    <!-- Reset Quota Confirmation Dialog -->
    <ConfirmDialog
      :show="showResetQuotaDialog"
      :title="t('keys.resetQuotaTitle')"
      :message="t('keys.resetQuotaConfirmMessage', { name: selectedKey?.name, used: selectedKey?.quota_used?.toFixed(4) })"
      :confirm-text="t('keys.reset')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="resetQuotaUsed"
      @cancel="showResetQuotaDialog = false"
    />

    <!-- Reset Rate Limit Confirmation Dialog -->
    <ConfirmDialog
      :show="showResetRateLimitDialog"
      :title="t('keys.resetRateLimitTitle')"
      :message="t('keys.resetRateLimitConfirmMessage', { name: selectedKey?.name })"
      :confirm-text="t('keys.reset')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="resetRateLimitUsage"
      @cancel="showResetRateLimitDialog = false"
    />

    <!-- Use Key Modal -->
    <UseKeyModal
      :show="showUseKeyModal"
      :api-key="selectedKey?.key || ''"
      :base-url="publicSettings?.api_base_url || ''"
      :group="selectedKeyGroup"
      :composite-groups="selectedCompositeGroups"
      @close="closeUseKeyModal"
    />

    <TfCliImportDialog
      :show="showTfCliImportDialog"
      :api-key="tfImportKey"
      @close="closeTfCliImportDialog"
    />

    <KeyActionMenu
      :show="Boolean(actionMenuKey)"
      :api-key="actionMenuKey"
      :position="actionMenuPosition"
      :allow-import="!publicSettings?.hide_ccs_import_button"
      @close="closeKeyActionMenu"
      @use="openUseKeyModal"
      @import-tf="openTfCliImportDialog"
      @import="importToCcswitch"
      @duplicate="duplicateKey"
      @rotate="confirmRotate"
      @delete="confirmDelete"
    />

    <!-- 按可用协议选择客户端和模型。 -->
    <BaseDialog
      :show="showCcsClientSelect"
      :title="t('keys.ccsClientSelect.title')"
      width="narrow"
      @close="closeCcsClientSelect"
    >
      <div class="space-y-4">
        <p class="text-sm text-gray-600 dark:text-gray-400">{{ t('keys.ccsClientSelect.description') }}</p>
        <Select v-model="ccClient" :options="ccClientOptions" />
        <Select v-model="ccModel" :options="ccModelOptions" searchable :placeholder="t('keys.useKeyModal.selectModel')" />
        <p v-if="!ccModelOptions.length" class="input-hint">{{ t('keys.useKeyModal.noModels') }}</p>
        <button type="button" class="btn btn-primary" :disabled="!ccModel" @click="handleCcsClientSelect(ccClient)">{{ t('common.confirm') }}</button>
      </div>
      <template #footer>
        <div class="flex justify-end">
          <button @click="closeCcsClientSelect" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Group Selector Dropdown (Teleported to body to avoid overflow clipping) -->
    <Teleport to="body">
      <MotionTransition name="dropdown-fade">
        <div
          v-if="groupSelectorKeyId !== null && dropdownPosition" :inert="!(groupSelectorKeyId !== null && dropdownPosition) || undefined"
          ref="dropdownRef"
          class="dropdown animate-in fade-in slide-in-from-top-2 fixed z-teleport-dropdown w-max max-w-[calc(100vw-16px)] overflow-hidden duration-normal sm:min-w-[380px] py-0"
          style="pointer-events: auto !important;"
          :style="{
            top: dropdownPosition.top !== undefined ? dropdownPosition.top + 'px' : undefined,
            bottom: dropdownPosition.bottom !== undefined ? dropdownPosition.bottom + 'px' : undefined,
            left: dropdownPosition.left + 'px'
          }"
        >
          <!-- 分组搜索与页头共用输入框组件，背景和焦点样式随全局主题更新。 -->
          <div class="border-b border-gray-100 p-2 dark:border-dark-700">
            <SearchInput
              v-model="groupSearchQuery"
              :placeholder="t('keys.searchGroup')"
              @click.stop
            />
          </div>
          <!-- Group list -->
          <div class="max-h-80 overflow-y-auto p-1.5">
            <button
              v-for="option in filteredGroupOptions"
              :key="option.value ?? 'null'"
              @click="changeGroup(selectedKeyForGroup!, option.value)"
              :class="[
                'flex w-full items-center justify-between rounded-control px-3 py-2.5 text-sm transition-colors',
                'border-b border-gray-100 last:border-0 dark:border-dark-700',
                selectedKeyForGroup?.group_id === option.value ||
                (!selectedKeyForGroup?.group_id && option.value === null)
                  ? 'bg-primary-50 dark:bg-primary-500/8 dark:text-primary-500'
                  : 'hover:bg-gray-100 dark:hover:bg-dark-700'
              ]"
              :title="option.description || undefined"
            >
              <GroupOptionItem
                :name="option.label"
                :display-brand="option.displayBrand"
                :rate-multiplier="option.rate"
                :user-rate-multiplier="option.userRate"
                :description="option.description"
                :selected="
                  selectedKeyForGroup?.group_id === option.value ||
                  (!selectedKeyForGroup?.group_id && option.value === null)
                "
              />
            </button>
            <!-- Empty state when search has no results -->
            <div v-if="filteredGroupOptions.length === 0" class="py-4 text-center text-sm text-gray-400 dark:text-gray-500">
              {{ t('keys.noGroupFound') }}
            </div>
          </div>
        </div>
      </MotionTransition>
    </Teleport>
  </AppLayout>
</template>

<script setup lang="ts">
import Skeleton from '@/components/common/Skeleton.vue'
import MotionTransition from '@/components/common/MotionTransition.vue'
	import { watch, ref, reactive, computed, onMounted, onUnmounted, type ComponentPublicInstance } from 'vue'
	import { useI18n } from 'vue-i18n'
	import { useRoute } from 'vue-router'
	import { useAppStore } from '@/stores/app'
import { useOnboardingStore } from '@/stores/onboarding'
import { useClipboard } from '@/composables/useClipboard'
import { COPY_FEEDBACK_MS } from '@/constants/ui'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { useBalanceDisplay } from '@/composables/useBalanceDisplay'

const { t } = useI18n()
import { keysAPI, authAPI, usageAPI, userGroupsAPI } from '@/api'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
	import DataTable from '@/components/common/DataTable.vue'
	import Pagination from '@/components/common/Pagination.vue'
	import BaseDialog from '@/components/common/BaseDialog.vue'
	import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
	import EmptyState from '@/components/common/EmptyState.vue'
	import Select from '@/components/common/Select.vue'
	import FilterDropdown from '@/components/common/FilterDropdown.vue'
	import FilterField from '@/components/common/FilterField.vue'
	import Toggle from '@/components/common/Toggle.vue'
	import SearchInput from '@/components/common/SearchInput.vue'
	import RuleListEditor from '@/components/common/RuleListEditor.vue'
	import ModelMappingEditor from '@/components/common/ModelMappingEditor.vue'
	import Icon from '@/components/icons/Icon.vue'
	import KeyActionMenu from '@/components/keys/KeyActionMenu.vue'
	import UseKeyModal from '@/components/keys/UseKeyModal.vue'
	import ModelRedirectHelp from '@/components/keys/ModelRedirectHelp.vue'
	import TfCliImportDialog from '@/components/keys/TfCliImportDialog.vue'
	import EndpointPopover from '@/components/keys/EndpointPopover.vue'
	import GroupBadge from '@/components/common/GroupBadge.vue'
	import GroupOptionItem from '@/components/common/GroupOptionItem.vue'
	import ScopeDropdown, { type DataScope } from '@/components/team/ScopeDropdown.vue'
	import type {
	  ApiKey,
	  ApiKeyBillingMode,
	  ApiKeyBillingSubscriptionOption,
	  ApiKeyFastModePolicy,
	  CreateApiKeyRequest,
	  Group,
	  PublicSettings,
	  UpdateApiKeyRequest
	} from '@/types'
import type { Column } from '@/components/common/types'
import type { BatchApiKeyUsageStats } from '@/api/usage'
import { formatDateTime } from '@/utils/format'
import {
  KEY_REDIRECT_RULES,
  firstModelMappingIssue,
  mappingRowsToRecord,
  validateModelMappingRows,
  type ModelMappingRow
} from '@/utils/modelMappingRules'
import { resolveProviderBrand } from '@/utils/providerBrand'
import { getFloatingPanelPosition } from '@/utils/floatingPanel'
import {
  buildCcSwitchImportDeeplink,
  buildCcSwitchUsageScript,
  type CcSwitchClientType
} from '@/utils/ccswitchImport'
import { availableClients, CLIENT_LABELS, clientProtocol, modelsForProtocol, groupForKeyConfig } from '@/utils/clientConfig'

// Helper to format date for datetime-local input
const formatDateTimeLocal = (isoDate: string): string => {
  const date = new Date(isoDate)
  const pad = (n: number) => n.toString().padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

interface GroupOption {
  value: number
  label: string
  description: string | null
  displayBrand: string | null
  rate: number
  userRate: number | null
}

const appStore = useAppStore()
const route = useRoute()
const scope = ref<DataScope>(route?.query?.scope === 'team' ? 'team' : 'personal')
const onboardingStore = useOnboardingStore()
const { copyToClipboard: clipboardCopy } = useClipboard()
const { balanceUnitName, balanceUnitSymbol, formatBalanceAmount } = useBalanceDisplay()

const formatBalancePair = (
  used: number | null | undefined,
  limit: number | null | undefined,
  usedDigits: number,
  limitDigits: number,
  spaced: boolean = true
) => {
  const separator = spaced ? ' / ' : '/'
  return `${formatBalanceAmount(used, { fractionDigits: usedDigits })}${separator}${formatBalanceAmount(limit, { fractionDigits: limitDigits })}`
}

// COMPOSITE_GROUP_PREVIEW_LIMIT 是列表中复合 Key 直接展示的映射胶囊数量上限。
const COMPOSITE_GROUP_PREVIEW_LIMIT = 4

// visibleCompositeGroups 返回表格中需要直接展示的复合映射。
// 比预览上限多一条时展示全部映射。
const visibleCompositeGroups = (row: ApiKey) => {
  const groups = row.composite_groups ?? []
  if (groups.length <= COMPOSITE_GROUP_PREVIEW_LIMIT + 1) {
    return groups
  }
  return groups.slice(0, COMPOSITE_GROUP_PREVIEW_LIMIT)
}

// compositeGroupChipClass 返回复合映射胶囊的配色，与 GroupBadge 共用分组展示品牌的色板。
// 未配置展示品牌时使用中性底色和内描边，与普通分组徽章保持一致。
const compositeGroupChipClass = (displayBrand?: string | null) => {
  const brand = displayBrand?.trim()
  if (!brand) {
    return 'ring-1 ring-inset bg-gray-100 text-gray-900 ring-gray-200 dark:bg-dark-800 dark:text-dark-50 dark:ring-dark-600'
  }
  return `ring-1 ring-inset ${resolveProviderBrand(brand).badgeClass}`
}

// concurrencyTone 按 Key 并发上限的占用比例取颜色：用满为红，达到八成为琥珀，其余为绿点灰字。
const concurrencyTone = (row: ApiKey) => {
  const current = row.current_concurrency ?? 0
  const limit = row.concurrency_limit ?? 0
  if (limit > 0 && current >= limit) {
    return { dot: 'bg-red-500', text: 'font-medium text-red-600 dark:text-red-400' }
  }
  if (limit > 0 && current >= limit * 0.8) {
    return { dot: 'bg-amber-500', text: 'font-medium text-amber-600 dark:text-amber-400' }
  }
  return { dot: 'bg-emerald-500', text: 'text-gray-500 dark:text-dark-300' }
}

// concurrencyTitle 返回并发标记的悬停说明。
const concurrencyTitle = (row: ApiKey) => {
  const current = row.current_concurrency ?? 0
  const limit = row.concurrency_limit ?? 0
  return limit > 0
    ? t('keys.concurrencyInUse', { current, limit })
    : t('keys.concurrencyInUseUnlimited', { current })
}

// hiddenCompositeGroupCount 返回被折叠的复合映射数量。
const hiddenCompositeGroupCount = (row: ApiKey) => {
  return (row.composite_groups?.length ?? 0) - visibleCompositeGroups(row).length
}

const allColumns = computed<Column[]>(() => [
  { key: 'name', label: t('common.name'), sortable: true },
  { key: 'id', label: t('keys.id'), sortable: true },
  { key: 'key', label: t('keys.apiKey'), sortable: false },
  { key: 'group', label: t('keys.group'), sortable: false },
  { key: 'usage', label: t('keys.usage'), sortable: true },
  { key: 'rate_limit', label: t('keys.rateLimitColumn'), sortable: false },
  { key: 'expires_at', label: t('keys.expiresAt'), sortable: true },
  { key: 'status', label: t('common.status'), sortable: true },
  { key: 'last_used_at', label: t('keys.lastUsedAt'), sortable: true },
  { key: 'last_used_ip', label: t('keys.lastUsedIP'), sortable: false },
  { key: 'created_at', label: t('keys.created'), sortable: true },
  { key: 'actions', label: t('common.actions'), sortable: false }
])

const ALWAYS_VISIBLE_COLUMNS = new Set(['name', 'actions'])
const DEFAULT_HIDDEN_COLUMNS = ['id', 'rate_limit', 'last_used_at', 'last_used_ip']
const HIDDEN_COLUMNS_KEY = 'api-key-hidden-columns'
const COLUMN_SETTINGS_VERSION_KEY = 'api-key-column-settings-version'
const COLUMN_SETTINGS_VERSION = 3
const VERSION_NEW_HIDDEN_COLUMNS: Record<number, string[]> = {
  2: ['last_used_ip'],
  3: ['id']
}

const toggleableColumns = computed(() =>
  allColumns.value.filter((column) => !ALWAYS_VISIBLE_COLUMNS.has(column.key))
)
const hiddenColumns = reactive<Set<string>>(new Set())

// 加载 API Key 表格列设置；新列默认展示，低频列首次加载默认隐藏。
const loadSavedColumns = () => {
  hiddenColumns.clear()
  try {
    const saved = localStorage.getItem(HIDDEN_COLUMNS_KEY)
    const validColumnKeys = new Set(allColumns.value.map((column) => column.key))
    if (saved) {
      const parsed = JSON.parse(saved) as string[]
      parsed
        .filter((key) =>
          typeof key === 'string' &&
          validColumnKeys.has(key) &&
          !ALWAYS_VISIBLE_COLUMNS.has(key)
        )
        .forEach((key) => hiddenColumns.add(key))
      const rawStoredVersion = Number(localStorage.getItem(COLUMN_SETTINGS_VERSION_KEY) ?? '1')
      // 无效版本值按版本 1 处理，低频列按版本迁移规则隐藏。
      const storedVersion = Number.isInteger(rawStoredVersion) && rawStoredVersion >= 1
        ? rawStoredVersion
        : 1
      if (storedVersion < COLUMN_SETTINGS_VERSION) {
        for (let version = storedVersion + 1; version <= COLUMN_SETTINGS_VERSION; version++) {
          for (const key of VERSION_NEW_HIDDEN_COLUMNS[version] ?? []) {
            if (validColumnKeys.has(key) && !ALWAYS_VISIBLE_COLUMNS.has(key)) {
              hiddenColumns.add(key)
            }
          }
        }
        saveColumnsToStorage()
      } else {
        localStorage.setItem(COLUMN_SETTINGS_VERSION_KEY, String(COLUMN_SETTINGS_VERSION))
      }
    } else {
      DEFAULT_HIDDEN_COLUMNS.forEach((key) => hiddenColumns.add(key))
      localStorage.setItem(COLUMN_SETTINGS_VERSION_KEY, String(COLUMN_SETTINGS_VERSION))
    }
  } catch (error) {
    console.error('Failed to load API key table columns:', error)
    DEFAULT_HIDDEN_COLUMNS.forEach((key) => hiddenColumns.add(key))
  }
}

const saveColumnsToStorage = () => {
  try {
    localStorage.setItem(HIDDEN_COLUMNS_KEY, JSON.stringify([...hiddenColumns]))
    localStorage.setItem(COLUMN_SETTINGS_VERSION_KEY, String(COLUMN_SETTINGS_VERSION))
  } catch (error) {
    console.error('Failed to save API key table columns:', error)
  }
}

const toggleColumn = (key: string) => {
  if (ALWAYS_VISIBLE_COLUMNS.has(key)) return
  if (hiddenColumns.has(key)) {
    hiddenColumns.delete(key)
  } else {
    hiddenColumns.add(key)
  }
  saveColumnsToStorage()
}

const isColumnVisible = (key: string) => !hiddenColumns.has(key)

const columns = computed<Column[]>(() =>
  allColumns.value.filter((column) => ALWAYS_VISIBLE_COLUMNS.has(column.key) || !hiddenColumns.has(column.key))
)

const apiKeys = ref<ApiKey[]>([])
const groups = ref<Group[]>([])
// 表单分组独立于列表筛选分组，指定订阅时只收窄表单选择范围。
const formGroups = ref<Group[]>([])
const billingSubscriptions = ref<ApiKeyBillingSubscriptionOption[]>([])
const billingOptionsLoading = ref(false)
const formGroupsLoading = ref(false)
// 加载态从公共设置请求开始，持续到首次 Key 请求完成。
const loading = ref(true)
const usageLoading = ref(false)
const submitting = ref(false)
const now = ref(new Date())
let resetTimer: ReturnType<typeof setInterval> | null = null
const usageStats = ref<Record<string, BatchApiKeyUsageStats>>({})
const userGroupRates = ref<Record<number, number>>({})

const pagination = ref({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0
})
const sortState = ref({
  sort_by: 'created_at',
  sort_order: 'desc' as 'asc' | 'desc'
})

// Filter state
const filterSearch = ref('')
const filterStatus = ref('')
const filterGroupId = ref<string | number>('')

const showCreateModal = ref(false)
const showEditModal = ref(false)
const showDeleteDialog = ref(false)
const rotationKey = ref<ApiKey | null>(null)
const rotatedKey = ref<ApiKey | null>(null)
const rotatingKey = ref(false)
const showResetQuotaDialog = ref(false)
const showResetRateLimitDialog = ref(false)
const showUseKeyModal = ref(false)
const showTfCliImportDialog = ref(false)
const showCcsClientSelect = ref(false)
const showColumnDropdown = ref(false)
const pendingCcsRow = ref<ApiKey | null>(null)
const tfImportKey = ref<ApiKey | null>(null)
const selectedKey = ref<ApiKey | null>(null)
const actionMenuKey = ref<ApiKey | null>(null)
const actionMenuPosition = ref<{ top: number; left: number } | null>(null)
const copiedKeyId = ref<number | null>(null)
const groupSelectorKeyId = ref<number | null>(null)
const publicSettings = ref<PublicSettings | null>(null)
const teamFeatureEnabled = computed(() => publicSettings.value?.team_enabled !== false)
const dropdownRef = ref<HTMLElement | null>(null)
const columnDropdownRef = ref<HTMLElement | null>(null)
const dropdownPosition = ref<{ top?: number; bottom?: number; left: number } | null>(null)
const groupButtonRefs = ref<Map<number, HTMLElement>>(new Map())
let abortController: AbortController | null = null
let compositeBindingSequence = 0
let formGroupsRequestID = 0

// 新建本地映射行时使用稳定 ID，排序不会导致输入框重建。
const newCompositeBinding = (groupId: number | null = null, prefix = '') => ({
  local_id: ++compositeBindingSequence,
  group_id: groupId,
  prefix
})

// 获取当前正在切换分组的 API Key。
const selectedKeyForGroup = computed(() => {
  if (groupSelectorKeyId.value === null) return null
  return apiKeys.value.find((k) => k.id === groupSelectorKeyId.value) || null
})

const setGroupButtonRef = (keyId: number, el: Element | ComponentPublicInstance | null) => {
  if (el instanceof HTMLElement) {
    groupButtonRefs.value.set(keyId, el)
  } else {
    groupButtonRefs.value.delete(keyId)
  }
}

const formData = ref({
  name: '',
  group_id: null as number | null,
  is_composite: false,
  composite_groups: [] as Array<ReturnType<typeof newCompositeBinding>>,
  status: 'active' as 'active' | 'inactive',
  fast_mode_policy: 'follow_request' as ApiKeyFastModePolicy,
  billing_mode: 'auto' as ApiKeyBillingMode,
  preferred_subscription_id: null as number | null,
  model_mapping_rows: [] as ModelMappingRow[],
  use_custom_key: false,
  custom_key: '',
  enable_ip_restriction: false,
  ip_whitelist: '',
  ip_blacklist: '',
  // Quota settings (empty = unlimited)
  enable_quota: false,
  quota: null as number | null,
  // Rate limit settings
  enable_rate_limit: false,
  concurrency_limit: 0,
  rpm_limit: 0,
  rate_limit_5h: null as number | null,
  rate_limit_1d: null as number | null,
  rate_limit_7d: null as number | null,
  enable_expiration: false,
  expiration_preset: '30' as '7' | '30' | '90' | 'custom',
  expiration_date: '',
  fallback_when_group_unavailable: true
})

const MODEL_REDIRECT_MAX_RULES = 100

// 前端与后端共享相同的大小写敏感、单尾通配符和长度约束。
const modelMappingIssues = computed(() =>
  validateModelMappingRows(formData.value.model_mapping_rows, KEY_REDIRECT_RULES)
)

const modelMappingFieldErrors = computed(() =>
  modelMappingIssues.value.map((issue) => ({
    from: issue.from ? t(`keys.modelRedirect.${issue.from}`) : undefined,
    to: issue.to ? t(`keys.modelRedirect.${issue.to}`) : undefined
  }))
)

const modelMappingFormError = computed(() => {
  if (formData.value.model_mapping_rows.length > MODEL_REDIRECT_MAX_RULES) {
    return t('keys.modelRedirect.tooManyRules')
  }
  const issue = firstModelMappingIssue(modelMappingIssues.value)
  return issue ? t(`keys.modelRedirect.${issue}`) : ''
})

// 自定义Key验证
const customKeyError = computed(() => {
  if (!formData.value.use_custom_key || !formData.value.custom_key) {
    return ''
  }
  const key = formData.value.custom_key
  if (key.length < 16) {
    return t('keys.customKeyTooShort')
  }
  // 检查字符：只允许字母、数字、下划线、连字符
  if (!/^[a-zA-Z0-9_-]+$/.test(key)) {
    return t('keys.customKeyInvalidChars')
  }
  return ''
})

// 单 Key Fast 策略选项；Select 组件负责键盘与弹层交互。
const fastModePolicyOptions = computed(() => [
  { value: 'follow_request', label: t('keys.fastModePolicy.followRequest') },
  { value: 'force_on', label: t('keys.fastModePolicy.forceOn') },
  { value: 'force_off', label: t('keys.fastModePolicy.forceOff') }
])

// 结算模式由服务端最终校验；这里保持存量 Key 的自动模式可见且可恢复。
const billingModeOptions = computed(() => [
  { value: 'auto', label: t('keys.billing.modes.auto') },
  { value: 'subscription', label: t('keys.billing.modes.subscription') },
  { value: 'balance', label: t('keys.billing.modes.balance') }
])

const billingSubscriptionOptions = computed<Array<{
  value: number
  label: string
  description?: string
  disabled?: boolean
}>>(() => {
  const options: Array<{
    value: number
    label: string
    description?: string
    disabled?: boolean
  }> = billingSubscriptions.value.map((subscription) => ({
    value: subscription.id,
    label: `${subscription.plan_name} #${subscription.id}`,
    description: subscription.groups_restricted
      ? t('keys.billing.groupsRestricted')
      : t('keys.billing.allGroups')
  }))
  const selectedID = formData.value.preferred_subscription_id
  if (selectedID && !options.some((option) => option.value === selectedID)) {
    options.unshift({
      value: selectedID,
      label: t('keys.billing.unavailableSubscription', { id: selectedID }),
      disabled: true
    })
  }
  return options
})

const shouldSubmitEditStatus = (key: ApiKey, status: 'active' | 'inactive') => {
  // Owner 锁定期间仍允许编辑其它字段，但不能通过普通更新接口触碰状态。
  if (key.team_owner_disabled) return false
  if (key.status === 'quota_exhausted' || key.status === 'expired') {
    return status === 'active'
  }
  return true
}

// 筛选下拉选项。
const groupFilterOptions = computed(() => [
  { value: '', label: t('keys.allGroups') },
  { value: 0, label: t('keys.noGroup') },
  ...groups.value.map((g) => ({ value: g.id, label: g.name }))
])

const statusFilterOptions = computed(() => [
  { value: '', label: t('keys.allStatus') },
  { value: 'active', label: t('keys.status.active') },
  { value: 'inactive', label: t('keys.status.inactive') },
  { value: 'disabled', label: t('keys.status.disabled') },
  { value: 'quota_exhausted', label: t('keys.status.quota_exhausted') },
  { value: 'expired', label: t('keys.status.expired') }
])

const activeFilterCount = computed(() => [filterGroupId.value !== '', filterStatus.value !== ''].filter(Boolean).length)

const resetKeyFilters = () => {
  filterGroupId.value = ''
  filterStatus.value = ''
  pagination.value.page = 1
  loadApiKeys()
}

const onFilterChange = () => {
  pagination.value.page = 1
  loadApiKeys()
}

const onGroupFilterChange = (value: string | number | boolean | null) => {
  filterGroupId.value = value as string | number
  onFilterChange()
}

const onStatusFilterChange = (value: string | number | boolean | null) => {
  filterStatus.value = value as string
  onFilterChange()
}

// 用户侧分组选项包含名称、描述、品牌和倍率。
const buildGroupOptions = (source: Group[]) =>
  source.map((group) => ({
    value: group.id,
    label: group.name,
    description: group.description,
    displayBrand: group.display_brand?.trim() || null,
    rate: group.rate_multiplier,
    userRate: userGroupRates.value[group.id] ?? null,
  }))

// 指定订阅时仅使用服务端返回的权限与套餐分组交集。
const formGroupOptions = computed(() => buildGroupOptions(formGroups.value))
const allGroupOptions = computed(() => buildGroupOptions(groups.value))

// 切换复合模式时保留普通 Key 的原分组，前缀仍要求用户明确填写。
const onCompositeModeChange = (enabled: boolean) => {
  if (enabled) {
    if (formData.value.composite_groups.length === 0) {
      formData.value.composite_groups = [newCompositeBinding(formData.value.group_id)]
    }
    formData.value.is_composite = true
    return
  }
  formData.value.is_composite = false
  if (selectedKey.value?.is_composite) {
    formData.value.group_id = null
  } else if (formData.value.group_id === null) {
    formData.value.group_id = formData.value.composite_groups[0]?.group_id ?? null
  }
}

const addCompositeBinding = () => {
  if (formData.value.composite_groups.length >= 20) return
  formData.value.composite_groups.push(newCompositeBinding())
}

const removeCompositeBinding = (index: number) => {
  if (formData.value.composite_groups.length <= 1) return
  formData.value.composite_groups.splice(index, 1)
}

const moveCompositeBinding = (index: number, target: number) => {
  if (target < 0 || target >= formData.value.composite_groups.length) return
  const [binding] = formData.value.composite_groups.splice(index, 1)
  if (binding) formData.value.composite_groups.splice(target, 0, binding)
}

// compositeBindingError 即时检查前缀和分组的行内错误。
const compositeBindingError = (index: number) => {
  const binding = formData.value.composite_groups[index]
  if (!binding?.group_id) return t('keys.composite.groupRequired')
  const prefix = binding.prefix.trim()
  if (!prefix) return t('keys.composite.prefixRequired')
  if (!/^[A-Za-z0-9_-]{1,32}$/.test(prefix)) return t('keys.composite.prefixInvalid')
  const duplicatePrefix = formData.value.composite_groups.some(
    (item, itemIndex) => itemIndex !== index && item.prefix.trim().toLowerCase() === prefix.toLowerCase()
  )
  if (duplicatePrefix) return t('keys.composite.prefixDuplicate')
  const duplicateGroup = formData.value.composite_groups.some(
    (item, itemIndex) => itemIndex !== index && item.group_id === binding.group_id
  )
  if (duplicateGroup) return t('keys.composite.groupDuplicate')
  return ''
}

const compositeFormError = computed(() => {
  if (!formData.value.is_composite) return ''
  if (formData.value.composite_groups.length === 0) return t('keys.composite.mappingRequired')
  if (formData.value.composite_groups.length > 20) return t('keys.composite.tooManyMappings')
  for (let index = 0; index < formData.value.composite_groups.length; index++) {
    const error = compositeBindingError(index)
    if (error) return error
  }
  return ''
})

// 分组下拉搜索。
const groupSearchQuery = ref('')
const filteredGroupOptions = computed(() => {
  const query = groupSearchQuery.value.trim().toLowerCase()
  if (!query) return allGroupOptions.value
  return allGroupOptions.value.filter((opt) => {
    return opt.label.toLowerCase().includes(query) ||
      (opt.displayBrand && opt.displayBrand.toLowerCase().includes(query)) ||
      (opt.description && opt.description.toLowerCase().includes(query))
  })
})

// 指定订阅切换后，普通 Key 与复合 Key 都不能继续保留套餐未覆盖的分组。
const pruneFormGroupBindings = (allowedGroupIDs: Set<number>) => {
  if (formData.value.group_id !== null && !allowedGroupIDs.has(formData.value.group_id)) {
    formData.value.group_id = null
  }
  formData.value.composite_groups = formData.value.composite_groups.filter((binding) =>
    binding.group_id !== null && allowedGroupIDs.has(binding.group_id)
  )
}

const onBillingModeChange = (value: string | number | boolean | null) => {
  const mode: ApiKeyBillingMode = value === 'subscription' || value === 'balance' ? value : 'auto'
  formData.value.billing_mode = mode
  if (mode !== 'subscription') {
    formData.value.preferred_subscription_id = null
    void loadFormGroups()
    return
  }

  // 选定订阅后按兼容性裁剪分组，仍可用的映射继续保留。
  formData.value.preferred_subscription_id = null
  formGroups.value = []
}

const onPreferredSubscriptionChange = (value: string | number | boolean | null) => {
  const subscriptionID = typeof value === 'number' && value > 0 ? value : null
  formData.value.preferred_subscription_id = subscriptionID
  const subscription = billingSubscriptions.value.find((item) => item.id === subscriptionID)
  if (subscription?.groups_restricted) {
    pruneFormGroupBindings(new Set(subscription.applicable_groups))
  }
  void loadFormGroups()
}

const maskKey = (key: string): string => {
  if (key.length <= 12) return key
  return `${key.slice(0, 8)}...${key.slice(-4)}`
}

const copyToClipboard = async (text: string, keyId: number) => {
  const success = await clipboardCopy(text, t('keys.copied'))
  if (success) {
    copiedKeyId.value = keyId
    setTimeout(() => {
      copiedKeyId.value = null
    }, COPY_FEEDBACK_MS)
  }
}

const isAbortError = (error: unknown) => {
  if (!error || typeof error !== 'object') return false
  const { name, code } = error as { name?: string; code?: string }
  return name === 'AbortError' || code === 'ERR_CANCELED'
}

const loadApiKeys = async () => {
  abortController?.abort()
  const controller = new AbortController()
  abortController = controller
  const { signal } = controller
  loading.value = true
  // 新一轮列表请求立即清理上一页用量状态，取消竞态不会留下过期单元格。
  usageLoading.value = false
  usageStats.value = {}
  try {
    // 构建筛选条件。
    const filters: {
      search?: string
      status?: string
      group_id?: number | string
      sort_by?: string
      sort_order?: 'asc' | 'desc'
      scope?: DataScope
    } = {}
    if (filterSearch.value) filters.search = filterSearch.value
    if (filterStatus.value) filters.status = filterStatus.value
    if (filterGroupId.value !== '') filters.group_id = filterGroupId.value
    filters.sort_by = sortState.value.sort_by
    filters.sort_order = sortState.value.sort_order
    filters.scope = scope.value

    const response = await keysAPI.list(pagination.value.page, pagination.value.page_size, filters, {
      signal
    })
    if (signal.aborted) return
    apiKeys.value = response.items
    pagination.value.total = response.total
    pagination.value.pages = response.pages
    // Key 列表先解除加载态，用量单元格在后台独立填充。
    loading.value = false
    void loadUsageStats(response.items, controller)
  } catch (error) {
    if (isAbortError(error)) {
      return
    }
    appStore.showError(t('keys.failedToLoad'))
  } finally {
    if (abortController === controller) {
      loading.value = false
    }
  }
}

const loadUsageStats = async (items: ApiKey[], controller: AbortController) => {
  usageStats.value = {}
  if (items.length === 0) {
    usageLoading.value = false
    return
  }
  usageLoading.value = true
  try {
    const keyIds = items.map((key) => key.id)
    const usageResponse = await usageAPI.getDashboardApiKeysUsage(keyIds, { signal: controller.signal })
    if (controller.signal.aborted || abortController !== controller) return
    usageStats.value = usageResponse.stats
  } catch (error) {
    if (!isAbortError(error)) {
      console.error('Failed to load usage stats:', error)
    }
  } finally {
    if (abortController === controller) {
      usageLoading.value = false
    }
  }
}

const loadGroups = async () => {
  try {
    const available = await userGroupsAPI.getAvailable(scope.value)
    groups.value = available
    // 自动与余额模式使用用户已选分组，表单打开时的重复请求也保留该选择。
    if (formData.value.billing_mode !== 'subscription') {
      formGroups.value = available
    }
  } catch (error) {
    console.error('Failed to load groups:', error)
  }
}

// 指定订阅时由服务端返回付款主体权限与套餐分组的交集，表格筛选不受影响。
const loadFormGroups = async () => {
  const requestID = ++formGroupsRequestID
  if (formData.value.billing_mode !== 'subscription') {
    formGroups.value = groups.value
    formGroupsLoading.value = false
    return
  }

  const subscriptionID = formData.value.preferred_subscription_id
  if (!subscriptionID) {
    formGroups.value = []
    formGroupsLoading.value = false
    return
  }

  formGroupsLoading.value = true
  try {
    const available = await userGroupsAPI.getAvailable(scope.value, subscriptionID)
    if (requestID !== formGroupsRequestID) return
    formGroups.value = available
    pruneFormGroupBindings(new Set(available.map((group) => group.id)))
  } catch (error) {
    if (requestID === formGroupsRequestID) {
      formGroups.value = []
    }
    console.error('Failed to load API key form groups:', error)
  } finally {
    if (requestID === formGroupsRequestID) {
      formGroupsLoading.value = false
    }
  }
}

const loadBillingOptions = async () => {
  billingOptionsLoading.value = true
  try {
    billingSubscriptions.value = await keysAPI.getBillingOptions(scope.value)
  } catch (error) {
    billingSubscriptions.value = []
    console.error('Failed to load API key billing options:', error)
  } finally {
    billingOptionsLoading.value = false
  }
}

const loadUserGroupRates = async () => {
  try {
    userGroupRates.value = await userGroupsAPI.getUserGroupRates(scope.value)
  } catch (error) {
    console.error('Failed to load user group rates:', error)
  }
}

const loadPublicSettings = async () => {
  try {
    publicSettings.value = await authAPI.getPublicSettings()
    if (!teamFeatureEnabled.value && scope.value === 'team') scope.value = 'personal'
  } catch (error) {
    console.error('Failed to load public settings:', error)
  }
}

const openUseKeyModal = (key: ApiKey) => {
  selectedKey.value = key
  showUseKeyModal.value = true
}

const closeUseKeyModal = () => {
  showUseKeyModal.value = false
  selectedKey.value = null
}

const openTfCliImportDialog = (key: ApiKey) => {
  tfImportKey.value = key
  showTfCliImportDialog.value = true
}

const closeTfCliImportDialog = () => {
  showTfCliImportDialog.value = false
  tfImportKey.value = null
}

const handlePageChange = (page: number) => {
  pagination.value.page = page
  loadApiKeys()
}

const handlePageSizeChange = (pageSize: number) => {
  pagination.value.page_size = pageSize
  pagination.value.page = 1
  loadApiKeys()
}

const handleSort = (key: string, order: 'asc' | 'desc') => {
  sortState.value.sort_by = key
  sortState.value.sort_order = order
  pagination.value.page = 1
  loadApiKeys()
}

const openCreateModal = () => {
  showCreateModal.value = true
  void Promise.all([loadBillingOptions(), loadFormGroups()])
}

const editKey = (key: ApiKey) => {
  selectedKey.value = key
  fillKeyForm(key)
  formGroups.value = []
  showEditModal.value = true
  void Promise.all([loadBillingOptions(), loadFormGroups()])
}

// fillKeyForm 把一个已有 Key 的配置回填进表单，编辑弹窗和复制配置共用。
const fillKeyForm = (key: ApiKey) => {
  const hasIPRestriction = (key.ip_whitelist?.length > 0) || (key.ip_blacklist?.length > 0)
  const hasExpiration = !!key.expires_at
  formData.value = {
    name: key.name,
    group_id: key.group_id,
    is_composite: key.is_composite ?? false,
    composite_groups: (key.composite_groups || []).map((binding) =>
      newCompositeBinding(binding.group_id, binding.prefix)
    ),
    // 后端的终态统一映射为不可用，编辑表单只提交 active/inactive。
    status: key.status === 'active' ? 'active' : 'inactive',
    fast_mode_policy: key.fast_mode_policy ?? 'follow_request',
    billing_mode: key.billing_mode ?? 'auto',
    preferred_subscription_id: key.preferred_subscription_id ?? null,
    model_mapping_rows: Object.entries(key.model_mapping ?? {})
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([from, to]) => ({ from, to })),
    use_custom_key: false,
    custom_key: '',
    enable_ip_restriction: hasIPRestriction,
    ip_whitelist: (key.ip_whitelist || []).join('\n'),
    ip_blacklist: (key.ip_blacklist || []).join('\n'),
    enable_quota: key.quota > 0,
    quota: key.quota > 0 ? key.quota : null,
    enable_rate_limit: (key.rate_limit_5h > 0) || (key.rate_limit_1d > 0) || (key.rate_limit_7d > 0),
    concurrency_limit: key.concurrency_limit ?? 0,
    rpm_limit: key.rpm_limit ?? 0,
    rate_limit_5h: key.rate_limit_5h || null,
    rate_limit_1d: key.rate_limit_1d || null,
    rate_limit_7d: key.rate_limit_7d || null,
    enable_expiration: hasExpiration,
    expiration_preset: 'custom',
    expiration_date: key.expires_at ? formatDateTimeLocal(key.expires_at) : '',
    fallback_when_group_unavailable: key.fallback_when_group_unavailable ?? false
  }
}

// duplicateKey 用源 Key 的配置打开创建弹窗，名称追加序号和源 Key 区分，新密钥值由后端生成。
const duplicateKey = (key: ApiKey) => {
  selectedKey.value = null
  fillKeyForm(key)
  // 序号只对已加载列表去重；分页外的同名 Key 可能漏检，后端创建允许重名，用户可在弹窗里改。
  const taken = new Set(apiKeys.value.map((item) => item.name))
  taken.add(key.name)
  for (let n = 1; ; n++) {
    const candidate = `${key.name}-${n}`
    if (!taken.has(candidate)) {
      formData.value.name = candidate
      break
    }
  }
  formGroups.value = []
  showCreateModal.value = true
  void Promise.all([loadBillingOptions(), loadFormGroups()])
}

// 状态开关只维护编辑表单的 active/inactive 两种可提交值。
const onStatusToggle = (enabled: boolean) => {
  if (selectedKey.value?.team_owner_disabled) return
  formData.value.status = enabled ? 'active' : 'inactive'
}

const toggleKeyStatus = async (key: ApiKey) => {
  // 模板已经隐藏恢复按钮，这里保留防御检查避免其它调用路径误触发请求。
  if (key.team_owner_disabled) {
    appStore.showError(t('keys.teamOwnerDisabledHint'))
    return
  }
  const newStatus = key.status === 'active' ? 'inactive' : 'active'
  try {
    await keysAPI.toggleStatus(key.id, newStatus)
    appStore.showSuccess(
      newStatus === 'active' ? t('keys.keyEnabledSuccess') : t('keys.keyDisabledSuccess')
    )
    loadApiKeys()
  } catch (error) {
    appStore.showError(t('keys.failedToUpdateStatus'))
  }
}

// 更多菜单按视口坐标定位并挂载到 body，脱离固定操作列的裁剪区域。
const openKeyActionMenu = (key: ApiKey, event: MouseEvent) => {
  if (actionMenuKey.value?.id === key.id) {
    closeKeyActionMenu()
    return
  }
  const target = event.currentTarget as HTMLElement | null
  if (!target) return
  const rect = target.getBoundingClientRect()
  // 固定高菜单(高度随 CCS 导入项显隐):下方放不下即整体上翻;窄屏保持右缘对齐触发器。
  const position = getFloatingPanelPosition(rect, window.innerWidth, window.innerHeight, {
    maxWidth: 192,
    fixedHeight: publicSettings.value?.hide_ccs_import_button ? 214 : 254,
    viewportPadding: 8,
    gap: 4,
    pinLeftOnMobile: false
  })
  // fixedHeight 模式下 top 恒非空。
  actionMenuPosition.value = { top: position.top ?? 8, left: position.left }
  actionMenuKey.value = key
}

const closeKeyActionMenu = () => {
  actionMenuKey.value = null
  actionMenuPosition.value = null
}

const openGroupSelector = (key: ApiKey) => {
	if (key.is_composite || key.billing_mode === 'subscription') {
		editKey(key)
		return
	}
  if (groupSelectorKeyId.value === key.id) {
    groupSelectorKeyId.value = null
    dropdownPosition.value = null
  } else {
    const buttonEl = groupButtonRefs.value.get(key.id)
    if (buttonEl) {
      const rect = buttonEl.getBoundingClientRect()
      // 面板左缘对齐触发器,预估最大高度 400 决定翻转;窄屏面板近满宽,钉到视口左缘。
      const position = getFloatingPanelPosition(rect, window.innerWidth, window.innerHeight, {
        align: 'left',
        maxWidth: 380,
        viewportPadding: 8,
        gap: 4,
        maxHeightRatio: 1,
        minComfortableHeight: 400
      })
      if (position.bottom !== null) {
        // 下方空间不足时向上弹出。
        dropdownPosition.value = { bottom: position.bottom, left: position.left }
      } else {
        // 默认向下弹出。
        dropdownPosition.value = { top: position.top ?? undefined, left: position.left }
      }
    }
    groupSelectorKeyId.value = key.id
    groupSearchQuery.value = ''
  }
}

const submitGroupChange = async (
  key: ApiKey,
  newGroupId: number | null
) => {
  await keysAPI.update(key.id, { group_id: newGroupId })
  appStore.showSuccess(t('keys.groupChangedSuccess'))
  loadApiKeys()
}

const changeGroup = async (key: ApiKey, newGroupId: number | null) => {
  groupSelectorKeyId.value = null
  dropdownPosition.value = null
  if (key.group_id === newGroupId) return

  try {
    await submitGroupChange(key, newGroupId)
  } catch (error) {
    appStore.showError(t('keys.failedToChangeGroup'))
  }
}

const closeGroupSelector = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  // 判断点击是否发生在下拉框或触发按钮内，同时关闭列设置菜单。
  if (!target.closest('.group\\/dropdown') && !dropdownRef.value?.contains(target)) {
    groupSelectorKeyId.value = null
    dropdownPosition.value = null
  }
  if (columnDropdownRef.value && !columnDropdownRef.value.contains(target)) {
    showColumnDropdown.value = false
  }
}

const confirmDelete = (key: ApiKey) => {
  selectedKey.value = key
  showDeleteDialog.value = true
}

const confirmRotate = (key: ApiKey) => {
  rotationKey.value = key
}

const cancelRotate = () => {
  if (!rotatingKey.value) rotationKey.value = null
}

const handleRotate = async () => {
  if (!rotationKey.value || rotatingKey.value) return
  rotatingKey.value = true
  try {
    const updated = await keysAPI.rotate(rotationKey.value.id)
    // 先将页面凭据替换为新值，列表刷新失败时复制功能也能取得新凭据。
    apiKeys.value = apiKeys.value.map(key =>
      key.id === updated.id ? { ...key, key: updated.key, updated_at: updated.updated_at } : key
    )
    if (selectedKey.value?.id === updated.id) selectedKey.value = updated
    if (copiedKeyId.value === updated.id) copiedKeyId.value = null
    rotationKey.value = null
    rotatedKey.value = updated
    void loadApiKeys()
  } catch (error: unknown) {
    const message = error instanceof Error
      ? error.message
      : (error as { message?: string } | null)?.message
    appStore.showError(message || t('keys.failedToRotate'))
  } finally {
    rotatingKey.value = false
  }
}

const buildKeyFormPayload = () => {
  // 仅在启用 IP 限制时解析名单。
  const parseIPList = (text: string): string[] =>
    text.split('\n').map(ip => ip.trim()).filter(ip => ip.length > 0)
  const ipWhitelist = formData.value.enable_ip_restriction ? parseIPList(formData.value.ip_whitelist) : []
  const ipBlacklist = formData.value.enable_ip_restriction ? parseIPList(formData.value.ip_blacklist) : []

  // 计算额度值，空值和 0 都按不限额处理。
  const quota = formData.value.quota && formData.value.quota > 0 ? formData.value.quota : 0

  // 计算过期时间。
  let expiresInDays: number | undefined
  let expiresAt: string | null | undefined
  if (formData.value.enable_expiration && formData.value.expiration_date) {
    if (!showEditModal.value) {
      // 创建模式：按选择日期换算剩余天数。
      const expDate = new Date(formData.value.expiration_date)
      const now = new Date()
      const diffDays = Math.ceil((expDate.getTime() - now.getTime()) / (1000 * 60 * 60 * 24))
      expiresInDays = diffDays > 0 ? diffDays : 1
    } else {
      // 编辑模式：直接提交自定义日期。
      expiresAt = new Date(formData.value.expiration_date).toISOString()
    }
  } else if (showEditModal.value) {
    // 编辑模式：关闭过期或清空日期时发送空字符串以清除。
    expiresAt = ''
  }

  // 计算限速值，关闭开关时提交 0。
  const rateLimitData = formData.value.enable_rate_limit ? {
    rate_limit_5h: formData.value.rate_limit_5h && formData.value.rate_limit_5h > 0 ? formData.value.rate_limit_5h : 0,
    rate_limit_1d: formData.value.rate_limit_1d && formData.value.rate_limit_1d > 0 ? formData.value.rate_limit_1d : 0,
    rate_limit_7d: formData.value.rate_limit_7d && formData.value.rate_limit_7d > 0 ? formData.value.rate_limit_7d : 0,
  } : { rate_limit_5h: 0, rate_limit_1d: 0, rate_limit_7d: 0 }

  return {
    ipWhitelist,
    ipBlacklist,
    quota,
    expiresInDays,
    expiresAt,
    rateLimitData,
    modelMapping: mappingRowsToRecord(formData.value.model_mapping_rows)
  }
}

const submitKeyForm = async () => {
  const { ipWhitelist, ipBlacklist, quota, expiresInDays, expiresAt, rateLimitData, modelMapping } = buildKeyFormPayload()
  submitting.value = true
  try {
    if (showEditModal.value && selectedKey.value) {
      const updates: UpdateApiKeyRequest = {
        name: formData.value.name,
        group_id: formData.value.is_composite ? undefined : formData.value.group_id,
        is_composite: formData.value.is_composite,
        composite_groups: formData.value.is_composite
          ? formData.value.composite_groups.map((binding) => ({
              group_id: binding.group_id!,
              prefix: binding.prefix.trim()
            }))
          : undefined,
        fast_mode_policy: formData.value.fast_mode_policy,
        model_mapping: modelMapping,
        ip_whitelist: ipWhitelist,
        ip_blacklist: ipBlacklist,
        quota: quota,
        expires_at: expiresAt,
        concurrency_limit: Number(formData.value.concurrency_limit || 0),
        rpm_limit: Number(formData.value.rpm_limit || 0),
        rate_limit_5h: rateLimitData.rate_limit_5h,
        rate_limit_1d: rateLimitData.rate_limit_1d,
        rate_limit_7d: rateLimitData.rate_limit_7d,
        fallback_when_group_unavailable: formData.value.fallback_when_group_unavailable
      }
      const originalBillingMode = selectedKey.value.billing_mode ?? 'auto'
      const originalPreferredSubscriptionID = selectedKey.value.preferred_subscription_id ?? null
      if (
        formData.value.billing_mode !== originalBillingMode ||
        formData.value.preferred_subscription_id !== originalPreferredSubscriptionID
      ) {
        updates.billing_mode = formData.value.billing_mode
        updates.preferred_subscription_id = formData.value.billing_mode === 'subscription'
          ? formData.value.preferred_subscription_id
          : null
      }
      if (shouldSubmitEditStatus(selectedKey.value, formData.value.status)) {
        updates.status = formData.value.status
      }
      await keysAPI.update(selectedKey.value.id, updates)
      appStore.showSuccess(t('keys.keyUpdatedSuccess'))
    } else {
      const customKey = formData.value.use_custom_key ? formData.value.custom_key : undefined
      const payload: CreateApiKeyRequest = {
        name: formData.value.name,
        scope: scope.value,
        group_id: formData.value.is_composite ? undefined : formData.value.group_id,
        is_composite: formData.value.is_composite,
        composite_groups: formData.value.is_composite
          ? formData.value.composite_groups.map((binding) => ({
              group_id: binding.group_id!,
              prefix: binding.prefix.trim()
            }))
          : undefined,
        fast_mode_policy: formData.value.fast_mode_policy,
        billing_mode: formData.value.billing_mode,
        preferred_subscription_id: formData.value.billing_mode === 'subscription'
          ? formData.value.preferred_subscription_id
          : null,
        model_mapping: modelMapping,
        custom_key: customKey,
        ip_whitelist: ipWhitelist,
        ip_blacklist: ipBlacklist,
        quota,
        expires_in_days: expiresInDays,
        concurrency_limit: Number(formData.value.concurrency_limit || 0),
        rpm_limit: Number(formData.value.rpm_limit || 0),
        rate_limit_5h: rateLimitData.rate_limit_5h,
        rate_limit_1d: rateLimitData.rate_limit_1d,
        rate_limit_7d: rateLimitData.rate_limit_7d,
        fallback_when_group_unavailable: formData.value.fallback_when_group_unavailable
      }
      await keysAPI.createWithPayload(payload)
      appStore.showSuccess(t('keys.keyCreatedSuccess'))
      // 仅在引导进行到提交步骤且创建成功后推进引导。
      if (onboardingStore.isCurrentStep('[data-tour="key-form-submit"]')) {
        onboardingStore.nextStep(500)
      }
    }
    closeModals()
    loadApiKeys()
  } catch (error: any) {
    const errorMsg = error?.reason === 'API_KEY_LIMIT_REACHED'
      ? t('keys.apiKeyLimitReached', {
          current: error?.metadata?.current ?? '?',
          limit: error?.metadata?.limit ?? '?'
        })
      : error?.message || t('keys.failedToSave')
    appStore.showError(errorMsg)
    // 创建失败时不推进引导。
  } finally {
    submitting.value = false
  }
}

// 切换作用域后清空分页与筛选缓存，并只重新加载当前内容区域的数据。
const onScopeChange = () => {
  pagination.value.page = 1
  filterGroupId.value = ''
  void Promise.all([loadApiKeys(), loadGroups(), loadUserGroupRates(), loadBillingOptions(), loadFormGroups()])
}

const handleSubmit = async () => {
  // 空输入按 0 提交，其余值需满足数据库整数范围。
  const requestLimits = [formData.value.concurrency_limit, formData.value.rpm_limit]
  if (requestLimits.some(value => !Number.isInteger(Number(value || 0)) || Number(value || 0) < 0 || Number(value || 0) > 2147483647)) {
    appStore.showError(t('keys.requestLimitsInvalid'))
    return
  }

	if (modelMappingFormError.value) {
		appStore.showError(modelMappingFormError.value)
		return
	}

	if (formData.value.is_composite && compositeFormError.value) {
		appStore.showError(compositeFormError.value)
		return
	}

  if (formData.value.billing_mode === 'subscription' && !formData.value.preferred_subscription_id) {
    appStore.showError(t('keys.billing.subscriptionRequired'))
    return
  }

  // 普通模式必须显式选择单个分组，复合转普通时同样不沿用隐式值。
  if (!formData.value.is_composite && formData.value.group_id === null) {
    appStore.showError(t('keys.groupRequired'))
    return
  }

  // 启用自定义 Key 时校验输入。
  if (!showEditModal.value && formData.value.use_custom_key) {
    if (!formData.value.custom_key) {
      appStore.showError(t('keys.customKeyRequired'))
      return
    }
    if (customKeyError.value) {
      appStore.showError(customKeyError.value)
      return
    }
  }

  await submitKeyForm()
}

/**
 * 处理删除 API Key 的操作
 * 优化：错误处理改进，优先显示后端返回的具体错误消息（如权限不足等），
 * 若后端未返回消息则显示默认的国际化文本
 */
const handleDelete = async () => {
  if (!selectedKey.value) return

  try {
    await keysAPI.delete(selectedKey.value.id)
    appStore.showSuccess(t('keys.keyDeletedSuccess'))
    showDeleteDialog.value = false
    loadApiKeys()
  } catch (error: any) {
    // 优先使用后端返回的错误消息，提供更具体的错误信息给用户
    const errorMsg = error?.message || t('keys.failedToDelete')
    appStore.showError(errorMsg)
  }
}

const closeModals = () => {
  showCreateModal.value = false
  showEditModal.value = false
  selectedKey.value = null
  formGroupsRequestID++
  formGroups.value = []
  formData.value = {
    name: '',
    group_id: null,
    is_composite: false,
    composite_groups: [],
    status: 'active',
    fast_mode_policy: 'follow_request',
    billing_mode: 'auto',
    preferred_subscription_id: null,
    model_mapping_rows: [],
    use_custom_key: false,
    custom_key: '',
    enable_ip_restriction: false,
    ip_whitelist: '',
    ip_blacklist: '',
    enable_quota: false,
    quota: null,
    enable_rate_limit: false,
    concurrency_limit: 0,
    rpm_limit: 0,
    rate_limit_5h: null,
    rate_limit_1d: null,
    rate_limit_7d: null,
    enable_expiration: false,
    expiration_preset: '30',
    expiration_date: '',
    fallback_when_group_unavailable: true
  }
}

// 展示重置额度确认弹窗。
const confirmResetQuota = () => {
  showResetQuotaDialog.value = true
}

// 根据快捷天数设置过期日期。
const setExpirationDays = (days: number) => {
  formData.value.expiration_preset = days.toString() as '7' | '30' | '90'
  const expDate = new Date()
  expDate.setDate(expDate.getDate() + days)
  formData.value.expiration_date = formatDateTimeLocal(expDate.toISOString())
}

// 重置 API Key 已用额度。
const resetQuotaUsed = async () => {
  if (!selectedKey.value) return
  showResetQuotaDialog.value = false
  try {
    await keysAPI.update(selectedKey.value.id, { reset_quota: true })
    appStore.showSuccess(t('keys.quotaResetSuccess'))
    // 同步更新本地状态。
    if (selectedKey.value) {
      selectedKey.value.quota_used = 0
    }
  } catch (error: any) {
    const errorMsg = error.response?.data?.detail || t('keys.failedToResetQuota')
    appStore.showError(errorMsg)
  }
}

// 从编辑弹窗展示重置限速确认弹窗。
const confirmResetRateLimit = () => {
  showResetRateLimitDialog.value = true
}

// 从表格行展示重置限速确认弹窗。
const confirmResetRateLimitFromTable = (row: ApiKey) => {
  selectedKey.value = row
  showResetRateLimitDialog.value = true
}

// 重置 API Key 限速用量。
const resetRateLimitUsage = async () => {
  if (!selectedKey.value) return
  showResetRateLimitDialog.value = false
  try {
    await keysAPI.update(selectedKey.value.id, { reset_rate_limit_usage: true })
    appStore.showSuccess(t('keys.rateLimitResetSuccess'))
    // 刷新 Key 数据。
    await loadApiKeys()
    // 用刷新后的数据更新当前编辑对象。
    const refreshedKey = apiKeys.value.find(k => k.id === selectedKey.value!.id)
    if (refreshedKey) {
      selectedKey.value = refreshedKey
    }
  } catch (error: any) {
    const errorMsg = error.response?.data?.detail || t('keys.failedToResetRateLimit')
    appStore.showError(errorMsg)
  }
}

// Key 内嵌分组缺少目录时，从已加载的可见分组中读取。
const groupWithModels = (key: ApiKey | null): Group | undefined => {
  if (!key?.group_id) return undefined
  const available = groups.value.find(group => group.id === key.group_id)
  return groupForKeyConfig(available ? { ...key.group, ...available } : key.group ?? undefined, key.model_mapping)
}
const selectedKeyGroup = computed(() => groupWithModels(selectedKey.value))
const selectedCompositeGroups = computed(() => (selectedKey.value?.composite_groups ?? []).map(binding => ({
  ...binding, group: groupForKeyConfig(groups.value.find(group => group.id === binding.group_id) ?? binding.group, selectedKey.value?.model_mapping),
})))
const ccClient = ref<CcSwitchClientType>('claude')
const ccModel = ref('')
const ccGroup = computed(() => groupWithModels(pendingCcsRow.value))
const ccClientOptions = computed(() => availableClients(ccGroup.value?.allowed_protocols ?? []).filter(client => client !== 'opencode').map(client => ({ value: client, label: CLIENT_LABELS[client] })))
const ccModelOptions = computed(() => {
  const protocol = clientProtocol(ccClient.value, ccGroup.value?.allowed_protocols ?? [])
  return protocol ? modelsForProtocol(ccGroup.value, protocol).map(model => ({ value: model, label: model })) : []
})
watch(ccClientOptions, options => { if (!options.some(option => option.value === ccClient.value)) ccClient.value = options[0]?.value as CcSwitchClientType ?? 'claude' })
watch(ccModelOptions, options => { if (!options.some(option => option.value === ccModel.value)) ccModel.value = options[0]?.value ?? '' })
const importToCcswitch = (row: ApiKey) => {
  pendingCcsRow.value = row
  showCcsClientSelect.value = true
}

const executeCcsImport = (row: ApiKey, clientType: CcSwitchClientType) => {
  const baseUrl = publicSettings.value?.api_base_url || window.location.origin

  const usageScript = buildCcSwitchUsageScript(baseUrl, balanceUnitName.value)
  const providerName = (publicSettings.value?.site_name || 'tokenrouter').trim() || 'tokenrouter'
  const deeplink = buildCcSwitchImportDeeplink({
    baseUrl,
    model: ccModel.value,
    clientType,
    providerName,
    apiKey: row.key,
    usageScript
  })

  try {
    window.open(deeplink, '_self')

    // 通过窗口焦点粗略判断协议处理器是否拉起成功。
    setTimeout(() => {
      if (document.hasFocus()) {
        // 仍然聚焦通常说明协议处理器未成功拉起。
        appStore.showError(t('keys.ccSwitchNotInstalled'))
      }
    }, 100)
  } catch (error) {
    appStore.showError(t('keys.ccSwitchNotInstalled'))
  }
}

const handleCcsClientSelect = (clientType: CcSwitchClientType) => {
  if (pendingCcsRow.value && ccModelOptions.value.some(option => option.value === ccModel.value)) {
    executeCcsImport(pendingCcsRow.value, clientType)
  }
  showCcsClientSelect.value = false
  pendingCcsRow.value = null
}

const closeCcsClientSelect = () => {
  showCcsClientSelect.value = false
  pendingCcsRow.value = null
}

function formatResetTime(resetAt: string | null): string {
  if (!resetAt) return ''
  const diff = new Date(resetAt).getTime() - now.value.getTime()
  if (diff <= 0) return t('keys.resetNow')
  const days = Math.floor(diff / 86400000)
  const hours = Math.floor((diff % 86400000) / 3600000)
  const mins = Math.floor((diff % 3600000) / 60000)
  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${mins}m`
  return `${mins}m`
}

onMounted(async () => {
  loadSavedColumns()
  document.addEventListener('click', closeGroupSelector)
  resetTimer = setInterval(() => { now.value = new Date() }, 60000)
  await loadPublicSettings()
  await Promise.all([loadApiKeys(), loadGroups(), loadUserGroupRates(), loadBillingOptions(), loadFormGroups()])
})

onUnmounted(() => {
  document.removeEventListener('click', closeGroupSelector)
  abortController?.abort()
	if (resetTimer) clearInterval(resetTimer)
})
</script>

<style scoped>
/* 创建密钥弹窗的单行控件统一为 36px，多行文本域保留自然高度。
   下拉触发器已由 .input 基线(min-h-9、py-1.5)提供同一尺寸，不再单列。 */
.key-form-controls :deep(input.input),
.key-form-controls :deep(.btn) {
  height: 2.25rem;
  min-height: 0;
}

.key-form-controls :deep(input.input),
.key-form-controls :deep(.btn) {
  padding-top: 0.375rem;
  padding-bottom: 0.375rem;
}
</style>
