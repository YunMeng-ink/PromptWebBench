// 操作日志（需求 §17.8、C-43）。
// 只提供查询与筛选：日志不可编辑也不可删除，因此本页没有写操作入口。

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { type AdminOperationLog, adminApi } from '@/api/admin'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { formatDateTime } from '@/utils/datetime'

const { t, te, locale } = useI18n()
const { messageOf } = useErrorMessage()

const rows = ref<AdminOperationLog[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(false)
const failure = ref('')

const filters = ref({ action: '', objectType: '', operator: '' })
const actions = ref<string[]>([])
const requiredActions = ref<string[]>([])
const selected = ref<AdminOperationLog | null>(null)

/** actionLabel 返回操作类型文案；未登记取值直接显示英文键，缺值时不拼出裸键。 */
function actionLabel(action: string | null | undefined): string {
  if (!action) {
    return ''
  }
  const key = `admin.logs.actionLabels.${action}`
  return te(key) ? t(key) : action
}

async function loadLogs(): Promise<void> {
  loading.value = true
  try {
    const result = await adminApi.operationLogs({
      page: page.value,
      page_size: pageSize,
      action: filters.value.action,
      object_type: filters.value.objectType,
      operator: filters.value.operator,
    })
    rows.value = result.items
    total.value = result.total
    failure.value = ''
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    loading.value = false
  }
}

async function loadActions(): Promise<void> {
  try {
    const result = await adminApi.operationLogActions()
    actions.value = result.actions
    requiredActions.value = result.required
  } catch (error) {
    failure.value = messageOf(error)
  }
}

function search(): void {
  page.value = 1
  void loadLogs()
}

function resetFilters(): void {
  filters.value = { action: '', objectType: '', operator: '' }
  search()
}

function changePage(next: number): void {
  page.value = next
  void loadLogs()
}

/** formatTime 把 UTC 时刻按本机时区呈现。 */
function formatTime(raw: string): string {
  if (raw === '') {
    return ''
  }
  return formatDateTime(raw, locale.value)
}

/** jsonText 把日志快照渲染为可读文本。 */
function jsonText(value: unknown): string {
  if (value === undefined || value === null) {
    return ''
  }
  return JSON.stringify(value, null, 2)
}

onMounted(async () => {
  await loadActions()
  await loadLogs()
})
</script>

<template>
  <section class="pb-admin-module">
    <header class="pb-admin-module__head">
      <h2 class="pb-admin-module__title" data-testid="admin-logs-title">
        {{ t('admin.logs.title') }}
      </h2>
      <span class="pb-admin-module__hint" data-testid="admin-logs-readonly">
        {{ t('admin.logs.readOnly') }}
      </span>
    </header>

    <p class="pb-admin-module__hint" data-testid="admin-logs-action-count">
      {{
        t('admin.logs.actionCount', {
          count: actions.length,
          required: requiredActions.length,
        })
      }}
    </p>

    <div class="pb-admin-module__filters">
      <el-select
        v-model="filters.action"
        class="pb-admin-module__filter"
        :placeholder="t('admin.logs.action')"
        clearable
        data-testid="admin-logs-action"
        @change="search"
      >
        <el-option
          v-for="action in actions"
          :key="action"
          :label="actionLabel(action)"
          :value="action"
        />
      </el-select>
      <el-input
        v-model="filters.objectType"
        class="pb-admin-module__filter"
        :placeholder="t('admin.logs.objectType')"
        clearable
        data-testid="admin-logs-object-type"
        @keyup.enter="search"
      />
      <el-input
        v-model="filters.operator"
        class="pb-admin-module__filter"
        :placeholder="t('admin.logs.operator')"
        clearable
        data-testid="admin-logs-operator"
        @keyup.enter="search"
      />
      <el-button type="primary" size="small" data-testid="admin-logs-search" @click="search">
        {{ t('admin.common.search') }}
      </el-button>
      <el-button size="small" data-testid="admin-logs-reset" @click="resetFilters">
        {{ t('admin.common.reset') }}
      </el-button>
    </div>

    <p v-if="failure" class="pb-admin-module__error" data-testid="admin-logs-error">{{ failure }}</p>
    <p v-if="!loading && rows.length === 0" class="pb-admin-module__empty" data-testid="admin-logs-empty">
      {{ t('admin.logs.empty') }}
    </p>

    <el-table
      v-if="rows.length > 0"
      :data="rows"
      size="small"
      data-testid="admin-logs-table"
      @row-click="(row: AdminOperationLog) => (selected = row)"
    >
      <el-table-column prop="id" :label="t('admin.common.id')" width="80" />
      <el-table-column :label="t('admin.logs.action')" width="180">
        <template #default="{ row }">{{ actionLabel(row.action) }}</template>
      </el-table-column>
      <el-table-column prop="operator_name" :label="t('admin.logs.operator')" width="140" />
      <el-table-column prop="object_type" :label="t('admin.logs.objectType')" width="140" />
      <el-table-column prop="object_id" :label="t('admin.common.id')" width="90" />
      <el-table-column prop="note" :label="t('admin.logs.note')" min-width="180" />
      <el-table-column :label="t('admin.common.createdAt')" width="180">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column width="90">
        <template #default="{ row }">
          <el-button size="small" text data-testid="admin-logs-detail" @click.stop="selected = row">
            {{ t('admin.common.detail') }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-pagination
      v-if="total > pageSize"
      class="pb-admin-module__pager"
      layout="prev, pager, next"
      :current-page="page"
      :page-size="pageSize"
      :total="total"
      @current-change="changePage"
    />

    <el-dialog
      :model-value="selected !== null"
      :title="t('admin.logs.detailTitle', { id: selected?.id ?? 0 })"
      width="640px"
      @update:model-value="selected = null"
    >
      <el-descriptions v-if="selected" :column="1" border size="small">
        <el-descriptions-item :label="t('admin.logs.action')">
          {{ actionLabel(selected.action) }}
        </el-descriptions-item>
        <el-descriptions-item :label="t('admin.logs.operator')">
          {{ selected.operator_name || t('admin.common.none') }}
        </el-descriptions-item>
        <el-descriptions-item :label="t('admin.logs.objectType')">
          {{ selected.object_type || t('admin.common.none') }}
        </el-descriptions-item>
        <el-descriptions-item :label="t('admin.common.createdAt')">
          {{ formatTime(selected.created_at) }}
        </el-descriptions-item>
        <el-descriptions-item :label="t('admin.logs.ip')">
          {{ selected.ip || t('admin.common.none') }}
        </el-descriptions-item>
        <el-descriptions-item :label="t('admin.logs.note')">
          {{ selected.note || t('admin.common.none') }}
        </el-descriptions-item>
        <el-descriptions-item :label="t('admin.logs.before')">
          <pre class="pb-admin-module__json">{{ jsonText(selected.before_data) }}</pre>
        </el-descriptions-item>
        <el-descriptions-item :label="t('admin.logs.after')">
          <pre class="pb-admin-module__json">{{ jsonText(selected.after_data) }}</pre>
        </el-descriptions-item>
      </el-descriptions>
      <template #footer>
        <el-button data-testid="admin-logs-close" @click="selected = null">
          {{ t('admin.common.close') }}
        </el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style scoped>
.pb-admin-module__head {
  align-items: baseline;
  display: flex;
  gap: var(--pb-space-md);
  justify-content: space-between;
}

.pb-admin-module__title {
  font-size: 18px;
  margin: 0;
}

.pb-admin-module__hint {
  color: var(--pb-color-text-faint);
  font-size: 13px;
  margin: var(--pb-space-xs) 0;
}

.pb-admin-module__filters {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-xs);
  margin: var(--pb-space-sm) 0;
}

.pb-admin-module__filter {
  max-width: 200px;
}

.pb-admin-module__error {
  color: var(--el-color-danger);
}

.pb-admin-module__empty {
  color: var(--pb-color-text-soft);
}

.pb-admin-module__pager {
  margin-top: var(--pb-space-sm);
}

.pb-admin-module__json {
  font-size: 12px;
  margin: 0;
  max-height: 160px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
