// 举报管理（需求 §17.6）。
// 举报既不能新建也不能删除，列表只提供筛选与处置；处置结果由后端校验，
// 被举报对象已不存在时后端返回 invalid_state，这里按接口错误文案提示。

<script setup lang="ts">
import { ElMessage } from 'element-plus'
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { type AdminReport, adminApi, type ReportStatus } from '@/api/admin'
import { useErrorMessage } from '@/composables/useErrorMessage'

const { t } = useI18n()
const { messageOf } = useErrorMessage()

const rows = ref<AdminReport[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(false)
const failure = ref('')

const filters = ref({ objectType: '', status: '' })

const selected = ref<AdminReport | null>(null)
const handleStatus = ref<ReportStatus>('resolved')
const handleNote = ref('')
const deleteContent = ref(false)
const banAuthor = ref(false)
const submitting = ref(false)

/** OBJECT_TYPE_KEYS 等映射表把后端英文键对应到语言包键；未登记取值按原文显示。 */
const OBJECT_TYPE_KEYS: Record<string, string> = {
  prompt: 'admin.reports.typePrompt',
  comment: 'admin.reports.typeComment',
}

const REASON_KEYS: Record<string, string> = {
  bad_content: 'prompt.reportReasonBadContent',
  illegal: 'prompt.reportReasonIllegal',
  porn: 'prompt.reportReasonPorn',
  attack: 'prompt.reportReasonAttack',
  privacy: 'prompt.reportReasonPrivacy',
  spam: 'prompt.reportReasonSpam',
  dangerous: 'prompt.reportReasonDangerous',
  other: 'prompt.reportReasonOther',
}

const STATUS_KEYS: Record<string, string> = {
  pending: 'admin.reports.statusPending',
  ignored: 'admin.reports.statusIgnored',
  resolved: 'admin.reports.statusResolved',
}

function labelOf(keys: Record<string, string>, value: string): string {
  const key = keys[value]
  return key === undefined ? value : t(key)
}

/** statusTagType 给出状态标签的配色。 */
function statusTagType(status: string): 'success' | 'info' | 'warning' {
  if (status === 'resolved') {
    return 'success'
  }
  return status === 'ignored' ? 'info' : 'warning'
}

/** truncate 把长文本压成单行摘要。 */
function truncate(text: string, limit: number): string {
  return text.length <= limit ? text : `${text.slice(0, limit)}…`
}

/** formatTime 把 UTC 时刻按本机时区呈现；空值显示占位文案。 */
function formatTime(raw: string | undefined): string {
  if (raw === undefined || raw === '') {
    return t('admin.common.none')
  }
  const parsed = new Date(raw)
  return Number.isNaN(parsed.getTime()) ? raw : parsed.toLocaleString()
}

async function loadReports(): Promise<void> {
  loading.value = true
  try {
    const result = await adminApi.reports({
      page: page.value,
      page_size: pageSize,
      object_type: filters.value.objectType,
      status: filters.value.status,
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

function search(): void {
  page.value = 1
  void loadReports()
}

function resetFilters(): void {
  filters.value = { objectType: '', status: '' }
  search()
}

function changePage(next: number): void {
  page.value = next
  void loadReports()
}

/** openHandle 打开处置对话框，并把表单恢复到待提交的初始取值。 */
function openHandle(row: AdminReport): void {
  selected.value = row
  handleStatus.value = 'resolved'
  handleNote.value = ''
  deleteContent.value = false
  banAuthor.value = false
}

async function submitHandle(): Promise<void> {
  const row = selected.value
  if (row === null || submitting.value) {
    return
  }
  submitting.value = true
  try {
    await adminApi.handleReport(row.id, {
      status: handleStatus.value,
      note: handleNote.value,
      delete_content: deleteContent.value,
      ban_author: banAuthor.value,
    })
    ElMessage.success(t('admin.reports.handled'))
    selected.value = null
    await loadReports()
  } catch (error) {
    ElMessage.error(messageOf(error))
  } finally {
    submitting.value = false
  }
}

onMounted(async () => {
  await loadReports()
})
</script>

<template>
  <section class="pb-admin-module">
    <header class="pb-admin-module__head">
      <h2 class="pb-admin-module__title">{{ t('admin.reports.title') }}</h2>
    </header>

    <div class="pb-admin-module__filters">
      <el-select
        v-model="filters.objectType"
        class="pb-admin-module__filter"
        :placeholder="t('admin.reports.objectType')"
        data-testid="admin-reports-object-type"
        @change="search"
      >
        <el-option :label="t('admin.common.all')" value="" />
        <el-option :label="t('admin.reports.typePrompt')" value="prompt" />
        <el-option :label="t('admin.reports.typeComment')" value="comment" />
      </el-select>
      <el-select
        v-model="filters.status"
        class="pb-admin-module__filter"
        :placeholder="t('admin.reports.status')"
        data-testid="admin-reports-status"
        @change="search"
      >
        <el-option :label="t('admin.common.all')" value="" />
        <el-option :label="t('admin.reports.statusPending')" value="pending" />
        <el-option :label="t('admin.reports.statusIgnored')" value="ignored" />
        <el-option :label="t('admin.reports.statusResolved')" value="resolved" />
      </el-select>
      <el-button type="primary" size="small" data-testid="admin-reports-search" @click="search">
        {{ t('admin.common.search') }}
      </el-button>
      <el-button size="small" data-testid="admin-reports-reset" @click="resetFilters">
        {{ t('admin.common.reset') }}
      </el-button>
    </div>

    <p v-if="failure" class="pb-admin-module__error" data-testid="admin-reports-error">
      {{ failure }}
    </p>
    <p
      v-if="!loading && rows.length === 0"
      class="pb-admin-module__empty"
      data-testid="admin-reports-empty"
    >
      {{ t('admin.common.empty') }}
    </p>

    <el-table v-if="rows.length > 0" :data="rows" size="small" data-testid="admin-reports-table">
      <el-table-column prop="id" :label="t('admin.common.id')" width="80" />
      <el-table-column :label="t('admin.reports.objectType')" width="110">
        <template #default="{ row }: { row: AdminReport }">
          {{ labelOf(OBJECT_TYPE_KEYS, row.object_type) }}
        </template>
      </el-table-column>
      <el-table-column :label="t('admin.reports.target')" min-width="240">
        <template #default="{ row }: { row: AdminReport }">
          <template v-if="row.object_type === 'prompt'">
            <router-link
              v-if="row.prompt_title"
              class="pb-admin-module__link"
              :to="`/prompts/${row.object_id}`"
            >
              {{ row.prompt_title }}
            </router-link>
            <span v-else class="pb-admin-module__gone">{{ t('admin.reports.promptGone') }}</span>
          </template>
          <template v-else>
            <template v-if="row.comment_body">
              {{ truncate(row.comment_body, 60) }}
              <router-link
                v-if="row.comment_prompt_id > 0"
                class="pb-admin-module__link"
                :to="`/prompts/${row.comment_prompt_id}`"
              >
                {{ t('admin.reports.commentOn') }} #{{ row.comment_prompt_id }}
              </router-link>
            </template>
            <span v-else class="pb-admin-module__gone">{{ t('admin.reports.commentGone') }}</span>
          </template>
        </template>
      </el-table-column>
      <el-table-column :label="t('admin.reports.detail')" width="140">
        <template #default="{ row }: { row: AdminReport }">
          {{ labelOf(REASON_KEYS, row.reason) }}
        </template>
      </el-table-column>
      <el-table-column :label="t('admin.reports.status')" width="110">
        <template #default="{ row }: { row: AdminReport }">
          <el-tag :type="statusTagType(row.status)" size="small">
            {{ labelOf(STATUS_KEYS, row.status) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="reporter_name" :label="t('admin.reports.reporter')" width="140" />
      <el-table-column :label="t('admin.common.detail')" min-width="180">
        <template #default="{ row }: { row: AdminReport }">
          {{ truncate(row.detail, 80) }}
        </template>
      </el-table-column>
      <el-table-column :label="t('admin.common.createdAt')" width="180">
        <template #default="{ row }: { row: AdminReport }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column :label="t('admin.common.actions')" width="100">
        <template #default="{ row }: { row: AdminReport }">
          <el-button
            size="small"
            text
            data-testid="admin-reports-handle"
            @click="openHandle(row)"
          >
            {{ t('admin.reports.handle') }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-pagination
      class="pb-admin-module__pager"
      layout="prev, pager, next"
      :current-page="page"
      :page-size="pageSize"
      :total="total"
      @current-change="changePage"
    />

    <el-dialog
      :model-value="selected !== null"
      :title="t('admin.reports.handleTitle', { id: selected?.id ?? 0 })"
      width="640px"
      @update:model-value="selected = null"
    >
      <template v-if="selected">
        <el-descriptions :column="1" border size="small">
          <el-descriptions-item :label="t('admin.reports.reporter')">
            {{ selected.reporter_name || t('admin.common.none') }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('admin.reports.objectType')">
            {{ labelOf(OBJECT_TYPE_KEYS, selected.object_type) }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('admin.common.id')">
            {{ selected.object_id }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('admin.reports.detail')">
            {{ labelOf(REASON_KEYS, selected.reason) }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('admin.common.detail')">
            {{ selected.detail || t('admin.common.none') }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('admin.common.createdAt')">
            {{ formatTime(selected.created_at) }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('admin.reports.handler')">
            {{ selected.handler_name || t('admin.common.none') }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('admin.reports.handleNote')">
            {{ selected.handle_note || t('admin.common.none') }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('admin.reports.handledAt')">
            {{ formatTime(selected.handled_at) }}
          </el-descriptions-item>
        </el-descriptions>

        <el-form class="pb-admin-module__form" label-width="120px" @submit.prevent>
          <el-form-item :label="t('admin.reports.status')">
            <!-- 单选项取值在 Element Plus 新旧版本中分别由 value 与 label 承载，两者同值以兼容。 -->
            <el-radio-group v-model="handleStatus" data-testid="admin-reports-handle-status">
              <el-radio :value="'pending'" :label="'pending'">
                {{ t('admin.reports.markPending') }}
              </el-radio>
              <el-radio :value="'ignored'" :label="'ignored'">
                {{ t('admin.reports.markIgnored') }}
              </el-radio>
              <el-radio :value="'resolved'" :label="'resolved'">
                {{ t('admin.reports.markResolved') }}
              </el-radio>
            </el-radio-group>
          </el-form-item>
          <el-form-item :label="t('admin.reports.handleNote')">
            <el-input
              v-model="handleNote"
              type="textarea"
              :rows="3"
              :maxlength="500"
              show-word-limit
              data-testid="admin-reports-handle-note"
            />
          </el-form-item>
          <el-form-item :label="t('admin.reports.deleteContent')">
            <el-switch v-model="deleteContent" data-testid="admin-reports-handle-delete" />
          </el-form-item>
          <el-form-item :label="t('admin.reports.banAuthor')">
            <el-switch v-model="banAuthor" data-testid="admin-reports-handle-ban" />
          </el-form-item>
        </el-form>
      </template>
      <template #footer>
        <el-button @click="selected = null">{{ t('admin.common.cancel') }}</el-button>
        <el-button
          type="primary"
          :loading="submitting"
          data-testid="admin-reports-handle-submit"
          @click="submitHandle"
        >
          {{ t('admin.common.save') }}
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

.pb-admin-module__filters {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-xs);
  margin: var(--pb-space-sm) 0;
}

.pb-admin-module__filter {
  max-width: 200px;
  width: 200px;
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

.pb-admin-module__link {
  color: var(--el-color-primary);
  text-decoration: none;
}

.pb-admin-module__gone {
  color: var(--pb-color-text-faint);
}

.pb-admin-module__form {
  margin-top: var(--pb-space-sm);
}
</style>
