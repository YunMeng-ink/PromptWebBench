// 公告管理（需求 §2.4、§17.8）。
// 公告只有发布、编辑与撤回三种写操作，已撤回的公告在后端不可再修改，
// 因此行内的编辑与撤回按钮只对 live 状态的公告可用。

<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { type AdminAnnouncement, adminApi } from '@/api/admin'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { formatDateTime } from '@/utils/datetime'

const { t, locale } = useI18n()
const { messageOf } = useErrorMessage()

const rows = ref<AdminAnnouncement[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(false)
const failure = ref('')

const filters = ref({ state: '' })

const formOpen = ref(false)
const editingId = ref<number | null>(null)
const form = ref({ title: '', body: '' })
const submitting = ref(false)

/** STATE_KEYS 把公告状态对应到语言包键；未登记取值显示通用占位文案。 */
const STATE_KEYS: Record<string, string> = {
  draft: 'admin.announcements.stateDraft',
  live: 'admin.announcements.stateLive',
  withdrawn: 'admin.announcements.stateWithdrawn',
}

/** formTitle 按创建或编辑模式给出对话框标题。 */
const formTitle = computed(() =>
  editingId.value === null
    ? t('admin.announcements.createTitle')
    : t('admin.announcements.editTitle'),
)

/** stateLabel 返回公告状态的文案。 */
function stateLabel(state: string): string {
  const key = STATE_KEYS[state]
  return key === undefined ? t('admin.common.unknown') : t(key)
}

/** stateTagType 给出状态标签的配色。 */
function stateTagType(state: string): 'success' | 'info' | 'warning' {
  if (state === 'live') {
    return 'success'
  }
  return state === 'withdrawn' ? 'info' : 'warning'
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
  return formatDateTime(raw, locale.value)
}

async function loadAnnouncements(): Promise<void> {
  loading.value = true
  try {
    const result = await adminApi.announcements({
      page: page.value,
      page_size: pageSize,
      state: filters.value.state,
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
  void loadAnnouncements()
}

function resetFilters(): void {
  filters.value = { state: '' }
  search()
}

function changePage(next: number): void {
  page.value = next
  void loadAnnouncements()
}

function openCreate(): void {
  editingId.value = null
  form.value = { title: '', body: '' }
  formOpen.value = true
}

function openEdit(row: AdminAnnouncement): void {
  editingId.value = row.id
  form.value = { title: row.title, body: row.body }
  formOpen.value = true
}

async function submitForm(): Promise<void> {
  if (submitting.value) {
    return
  }
  submitting.value = true
  try {
    const id = editingId.value
    if (id === null) {
      await adminApi.createAnnouncement(form.value.title, form.value.body)
      ElMessage.success(t('admin.announcements.published'))
    } else {
      await adminApi.updateAnnouncement(id, form.value.title, form.value.body)
      ElMessage.success(t('admin.announcements.updated'))
    }
    formOpen.value = false
    await loadAnnouncements()
  } catch (error) {
    ElMessage.error(messageOf(error))
  } finally {
    submitting.value = false
  }
}

async function withdraw(row: AdminAnnouncement): Promise<void> {
  try {
    await ElMessageBox.confirm(
      t('admin.announcements.withdrawConfirm'),
      t('admin.announcements.withdraw'),
      {
        confirmButtonText: t('admin.common.confirm'),
        cancelButtonText: t('admin.common.cancel'),
        type: 'warning',
      },
    )
  } catch {
    // 用户放弃确认属于正常的取消路径，结束本次撤回。
    return
  }
  try {
    await adminApi.withdrawAnnouncement(row.id)
    ElMessage.success(t('admin.announcements.withdrawn'))
    await loadAnnouncements()
  } catch (error) {
    ElMessage.error(messageOf(error))
  }
}

onMounted(async () => {
  await loadAnnouncements()
})
</script>

<template>
  <section class="pb-admin-module">
    <header class="pb-admin-module__head">
      <h2 class="pb-admin-module__title">{{ t('admin.announcements.title') }}</h2>
      <el-button
        type="primary"
        size="small"
        data-testid="admin-announcements-create"
        @click="openCreate"
      >
        {{ t('admin.announcements.create') }}
      </el-button>
    </header>

    <p class="pb-admin-module__hint">{{ t('admin.announcements.readOnlyHint') }}</p>

    <div class="pb-admin-module__filters">
      <el-select
        v-model="filters.state"
        class="pb-admin-module__filter"
        :placeholder="t('admin.announcements.stateFilter')"
        data-testid="admin-announcements-state"
        @change="search"
      >
        <el-option :label="t('admin.common.all')" value="" />
        <el-option :label="t('admin.announcements.stateLive')" value="live" />
        <el-option :label="t('admin.announcements.stateWithdrawn')" value="withdrawn" />
      </el-select>
      <el-button type="primary" size="small" data-testid="admin-announcements-search" @click="search">
        {{ t('admin.common.search') }}
      </el-button>
      <el-button size="small" data-testid="admin-announcements-reset" @click="resetFilters">
        {{ t('admin.common.reset') }}
      </el-button>
    </div>

    <p v-if="failure" class="pb-admin-module__error" data-testid="admin-announcements-error">
      {{ failure }}
    </p>
    <p
      v-if="!loading && rows.length === 0"
      class="pb-admin-module__empty"
      data-testid="admin-announcements-empty"
    >
      {{ t('admin.common.empty') }}
    </p>

    <el-table
      v-if="rows.length > 0"
      :data="rows"
      size="small"
      data-testid="admin-announcements-table"
    >
      <el-table-column prop="id" :label="t('admin.common.id')" width="80" />
      <el-table-column
        prop="title"
        :label="t('admin.announcements.fieldTitle')"
        min-width="200"
      />
      <el-table-column :label="t('admin.announcements.body')" min-width="260">
        <template #default="{ row }: { row: AdminAnnouncement }">
          {{ truncate(row.body, 80) }}
        </template>
      </el-table-column>
      <el-table-column :label="t('admin.announcements.stateFilter')" width="120">
        <template #default="{ row }: { row: AdminAnnouncement }">
          <el-tag :type="stateTagType(row.state)" size="small">{{ stateLabel(row.state) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column
        prop="publisher_name"
        :label="t('admin.announcements.publisher')"
        width="140"
      />
      <el-table-column :label="t('admin.announcements.publishedAt')" width="180">
        <template #default="{ row }: { row: AdminAnnouncement }">
          {{ formatTime(row.published_at) }}
        </template>
      </el-table-column>
      <el-table-column :label="t('admin.announcements.withdrawnAt')" width="180">
        <template #default="{ row }: { row: AdminAnnouncement }">
          {{ formatTime(row.withdrawn_at) }}
        </template>
      </el-table-column>
      <el-table-column :label="t('admin.common.actions')" width="150">
        <template #default="{ row }: { row: AdminAnnouncement }">
          <el-button
            size="small"
            text
            :disabled="row.state !== 'live'"
            data-testid="admin-announcements-edit"
            @click="openEdit(row)"
          >
            {{ t('admin.common.edit') }}
          </el-button>
          <el-button
            size="small"
            text
            :disabled="row.state !== 'live'"
            data-testid="admin-announcements-withdraw"
            @click="withdraw(row)"
          >
            {{ t('admin.announcements.withdraw') }}
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
      :model-value="formOpen"
      :title="formTitle"
      width="640px"
      @update:model-value="formOpen = false"
    >
      <el-form label-width="120px" @submit.prevent>
        <el-form-item :label="t('admin.announcements.fieldTitle')">
          <el-input
            v-model="form.title"
            :maxlength="200"
            show-word-limit
            data-testid="admin-announcements-form-title"
          />
        </el-form-item>
        <el-form-item :label="t('admin.announcements.fieldBody')">
          <el-input
            v-model="form.body"
            type="textarea"
            :rows="4"
            :maxlength="5000"
            show-word-limit
            data-testid="admin-announcements-form-body"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="formOpen = false">{{ t('admin.common.cancel') }}</el-button>
        <el-button
          type="primary"
          :loading="submitting"
          data-testid="admin-announcements-form-submit"
          @click="submitForm"
        >
          {{ editingId === null ? t('admin.announcements.publish') : t('admin.common.save') }}
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
</style>
