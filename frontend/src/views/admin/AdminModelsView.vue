// 模型管理（需求 §17.3、§13）。
// model_key 在模型之间必须唯一，重复的键由后端判定并返回 duplicate_model。

<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  type AdminModel,
  type AdminModelImportEntry,
  type AdminModelImportFilter,
  type AdminModelImportKey,
  type AdminModelImportLog,
  type AdminModelImportPreview,
  type AdminModelImportSelection,
  type AdminModelImportStatus,
  type AdminModelInput,
  type AdminModelPrompt,
  type AdminModelSnapshot,
  type AdminModelStat,
  adminApi,
  INPUT_MODALITIES,
} from '@/api/admin'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { useDictionaryStore } from '@/stores/dictionary'

/** ModelForm 是新建与编辑对话框共用的表单结构。 */
interface ModelForm {
  name: string
  model_key: string
  provider_key: string
  provider_name: string
  description: string
  release_date: string
  context_length?: number
  is_enabled: boolean
  note: string
  input_modalities: string[]
  aliases: string[]
  capability_ids: number[]
}

const { t, locale } = useI18n()
const { messageOf } = useErrorMessage()
const dictionary = useDictionaryStore()

const pageSize = 20

const rows = ref<AdminModel[]>([])
const total = ref(0)
const page = ref(1)
const loading = ref(false)
const failure = ref('')
const keyword = ref('')
const includeDeleted = ref(false)

async function loadModels(): Promise<void> {
  loading.value = true
  try {
    const result = await adminApi.models({
      page: page.value,
      page_size: pageSize,
      keyword: keyword.value,
      include_deleted: includeDeleted.value,
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
  void loadModels()
}

function resetFilters(): void {
  keyword.value = ''
  includeDeleted.value = false
  search()
}

function changePage(next: number): void {
  page.value = next
  void loadModels()
}

/** formatTime 把 UTC 时刻按本机时区呈现。 */
function formatTime(raw: string): string {
  if (raw === '') {
    return ''
  }
  const parsed = new Date(raw)
  return Number.isNaN(parsed.getTime()) ? raw : parsed.toLocaleString()
}

/** importSourceLabel 把导入来源转成文案，未登记的取值原样显示。 */
function importSourceLabel(source: string): string {
  if (source === 'models_dev') {
    return t('admin.models.sourceModelsDev')
  }
  if (source === 'manual') {
    return t('admin.models.sourceManual')
  }
  return source
}

/** duplicateText 拼出重复条目已存在的模型与提供方。 */
function duplicateText(entry: AdminModelImportEntry): string {
  if (entry.duplicate === undefined) {
    return ''
  }
  return `${entry.duplicate.name} · ${entry.duplicate.provider_name}`
}

/** payloadText 把导入记录的载荷压成单行，超过 120 个字符时截断。 */
function payloadText(value: unknown): string {
  if (value === undefined || value === null) {
    return t('admin.common.none')
  }
  const text = JSON.stringify(value)
  return text.length > 120 ? `${text.slice(0, 120)}…` : text
}

/** snapshotText 渲染原始快照，后端未保存快照时显示占位文案。 */
function snapshotText(raw: unknown): string {
  if (raw === undefined || raw === null) {
    return t('admin.common.none')
  }
  return JSON.stringify(raw, null, 2)
}

// ---------- 新建与编辑 ----------

const formVisible = ref(false)
const editingID = ref<number | null>(null)
const formError = ref('')
const form = ref<ModelForm>(emptyForm())
const canSubmit = computed(
  () => form.value.name.trim() !== '' && form.value.model_key.trim() !== '',
)

function emptyForm(): ModelForm {
  return {
    name: '',
    model_key: '',
    provider_key: '',
    provider_name: '',
    description: '',
    release_date: '',
    context_length: undefined,
    is_enabled: true,
    note: '',
    input_modalities: [],
    aliases: [],
    capability_ids: [],
  }
}

function openCreate(): void {
  editingID.value = null
  form.value = emptyForm()
  formError.value = ''
  formVisible.value = true
}

function openEdit(row: AdminModel): void {
  editingID.value = row.id
  form.value = {
    name: row.name,
    model_key: row.model_key,
    provider_key: row.provider_key,
    provider_name: row.provider_name,
    description: row.description,
    release_date: row.release_date,
    context_length: row.context_length,
    is_enabled: row.is_enabled,
    note: row.note,
    input_modalities: [...row.input_modalities],
    aliases: [...row.aliases],
    capability_ids: [...row.capability_ids],
  }
  formError.value = ''
  formVisible.value = true
}

async function submitForm(): Promise<void> {
  const id = editingID.value
  const input: AdminModelInput = {
    name: form.value.name.trim(),
    model_key: form.value.model_key.trim(),
    provider_key: form.value.provider_key.trim(),
    provider_name: form.value.provider_name.trim(),
    input_modalities: form.value.input_modalities,
    aliases: form.value.aliases,
    description: form.value.description,
    release_date: form.value.release_date,
    context_length: form.value.context_length,
    is_enabled: form.value.is_enabled,
    capability_ids: form.value.capability_ids,
    note: form.value.note,
  }
  try {
    if (id === null) {
      await adminApi.createModel(input)
      ElMessage.success(t('admin.models.created'))
    } else {
      await adminApi.updateModel(id, input)
      ElMessage.success(t('admin.models.updated'))
    }
    formVisible.value = false
    await loadModels()
  } catch (error) {
    formError.value = messageOf(error)
  }
}

// ---------- 启用、禁用与删除 ----------

async function toggleEnabled(row: AdminModel): Promise<void> {
  try {
    await adminApi.setModelEnabled(row.id, !row.is_enabled)
    ElMessage.success(t('admin.models.stateChanged'))
    failure.value = ''
    await loadModels()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

/** runModelAction 分发操作列「更多」菜单的动作。 */
function runModelAction(command: string, row: AdminModel): void {
  switch (command) {
    case 'toggle':
      void toggleEnabled(row)
      break
    case 'related':
      openRelated(row)
      break
    case 'stats':
      openStats(row)
      break
    case 'snapshots':
      openSnapshots(row)
      break
    case 'delete':
      void removeModel(row)
      break
  }
}

async function removeModel(row: AdminModel): Promise<void> {
  try {
    await ElMessageBox.confirm(t('admin.common.confirmDelete'), t('admin.common.remove'), {
      confirmButtonText: t('admin.common.confirm'),
      cancelButtonText: t('admin.common.cancel'),
      type: 'warning',
    })
  } catch {
    return
  }
  try {
    await adminApi.deleteModel(row.id)
    ElMessage.success(t('admin.models.deleted'))
    failure.value = ''
    await loadModels()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

// ---------- models.dev 导入 ----------

const importVisible = ref(false)
const importLoading = ref(false)
const importRunning = ref(false)
const importOverwrite = ref(false)
const importPreview = ref<AdminModelImportPreview | null>(null)
const importPage = ref(1)
const importPageSize = ref(50)
const importKeyword = ref('')
const importProvider = ref('')
const importStatus = ref<AdminModelImportStatus>('')
const previewError = ref('')
const runError = ref('')

/** 已选条目的键。跨页选择保存在这里，表格只渲染当前页。 */
const importSelected = ref(new Set<string>())
/** 选择全部筛选结果时，记录当时的筛选条件与总数。 */
const importSelectAll = ref<{ filter: AdminModelImportFilter; total: number } | null>(null)
/** 全选筛选结果后被取消的条目。 */
const importExcluded = ref(new Set<string>())

let previewSequence = 0
let keywordTimer: number | undefined

const importItems = computed(() => importPreview.value?.items ?? [])
const importTotal = computed(() => importPreview.value?.total ?? 0)
const importCatalogTotal = computed(() => importPreview.value?.catalog_total ?? 0)
const importDuplicateCount = computed(() => importPreview.value?.duplicate_count ?? 0)
const importProviders = computed(() => importPreview.value?.providers ?? [])
const importFetchedAt = computed(() => formatTime(importPreview.value?.fetched_at ?? ''))
const importSelectedCount = computed(() => {
  if (importSelectAll.value !== null) {
    return Math.max(0, importSelectAll.value.total - importExcluded.value.size)
  }
  return importSelected.value.size
})
const importPageSelected = computed(
  () => importItems.value.length > 0 && importItems.value.every((item) => isImportSelected(item)),
)
const importPageIndeterminate = computed(
  () => !importPageSelected.value && importItems.value.some((item) => isImportSelected(item)),
)

function importKey(providerID: string, modelID: string): string {
  return `${providerID}\u0000${modelID}`
}

function importKeyOf(entry: AdminModelImportEntry): string {
  return importKey(entry.provider_id, entry.model_id)
}

function parseImportKey(key: string): AdminModelImportKey {
  const separator = key.indexOf('\u0000')
  return { provider_id: key.slice(0, separator), model_id: key.slice(separator + 1) }
}

function isImportSelected(entry: AdminModelImportEntry): boolean {
  const key = importKeyOf(entry)
  if (importSelectAll.value !== null) {
    return !importExcluded.value.has(key)
  }
  return importSelected.value.has(key)
}

function setImportSelected(entry: AdminModelImportEntry, selected: boolean): void {
  const key = importKeyOf(entry)
  if (importSelectAll.value !== null) {
    if (selected) {
      importExcluded.value.delete(key)
    } else {
      importExcluded.value.add(key)
    }
    return
  }
  if (selected) {
    importSelected.value.add(key)
  } else {
    importSelected.value.delete(key)
  }
}

function setImportPageSelected(selected: boolean): void {
  for (const item of importItems.value) {
    setImportSelected(item, selected)
  }
}

function importRowClass({ row }: { row: AdminModelImportEntry }): string {
  return isImportSelected(row) ? 'pb-import__row--selected' : ''
}

/** selectAllImportMatches 选中当前筛选下的全部条目，包括尚未翻到的页。 */
function selectAllImportMatches(): void {
  importSelectAll.value = { filter: currentImportFilter(), total: importTotal.value }
  importExcluded.value.clear()
  importSelected.value.clear()
}

function clearImportSelection(): void {
  importSelectAll.value = null
  importExcluded.value.clear()
  importSelected.value.clear()
}

function currentImportFilter(): AdminModelImportFilter {
  return {
    keyword: importKeyword.value.trim(),
    provider_id: importProvider.value,
    status: importStatus.value,
  }
}

function openImport(): void {
  importVisible.value = true
  importOverwrite.value = false
  importPage.value = 1
  importKeyword.value = ''
  importProvider.value = ''
  importStatus.value = ''
  importPreview.value = null
  previewError.value = ''
  runError.value = ''
  clearImportSelection()
  void loadPreview()
}

/** loadPreview 拉取一页导入清单，上游不可用时后端返回 502。 */
async function loadPreview(refresh = false): Promise<void> {
  const sequence = ++previewSequence
  importLoading.value = true
  try {
    const result = await adminApi.previewModelImport({
      page: importPage.value,
      page_size: importPageSize.value,
      keyword: importKeyword.value.trim(),
      provider_id: importProvider.value,
      status: importStatus.value,
      refresh,
    })
    if (sequence !== previewSequence) {
      return
    }
    importPreview.value = result
    previewError.value = ''
  } catch (error) {
    if (sequence !== previewSequence) {
      return
    }
    importPreview.value = null
    previewError.value = messageOf(error)
  } finally {
    if (sequence === previewSequence) {
      importLoading.value = false
    }
  }
}

/**
 * dropFilterSelection 在筛选条件变化时放弃「全部筛选结果」选择。
 * 该选择绑定上一次的筛选条件，继续保留会让已选条数与眼前的结果不一致。
 */
function dropFilterSelection(): void {
  if (importSelectAll.value !== null) {
    clearImportSelection()
  }
}

function scheduleImportSearch(): void {
  window.clearTimeout(keywordTimer)
  keywordTimer = window.setTimeout(() => {
    importPage.value = 1
    dropFilterSelection()
    void loadPreview()
  }, 260)
}

function applyImportSearch(): void {
  window.clearTimeout(keywordTimer)
  importPage.value = 1
  dropFilterSelection()
  void loadPreview()
}

function resetImportFilters(): void {
  window.clearTimeout(keywordTimer)
  importKeyword.value = ''
  importProvider.value = ''
  importStatus.value = ''
  importPage.value = 1
  dropFilterSelection()
  void loadPreview()
}

function changeImportProvider(): void {
  importPage.value = 1
  dropFilterSelection()
  void loadPreview()
}

function changeImportStatus(): void {
  importPage.value = 1
  dropFilterSelection()
  void loadPreview()
}

function changeImportPage(next: number): void {
  importPage.value = next
  void loadPreview()
}

function changeImportPageSize(size: number): void {
  importPageSize.value = size
  importPage.value = 1
  void loadPreview()
}

/** refreshImportCatalog 丢弃服务端目录快照并重新读取 models.dev。 */
function refreshImportCatalog(): void {
  clearImportSelection()
  importPage.value = 1
  void loadPreview(true)
}

async function runImport(): Promise<void> {
  if (importRunning.value || importSelectedCount.value === 0) {
    return
  }
  runError.value = ''
  importRunning.value = true

  const selection: AdminModelImportSelection =
    importSelectAll.value !== null
      ? {
          mode: 'filter',
          filter: importSelectAll.value.filter,
          exclude: [...importExcluded.value].map(parseImportKey),
        }
      : {
          mode: 'keys',
          keys: [...importSelected.value].map(parseImportKey),
        }

  try {
    const result = await adminApi.importModels(importOverwrite.value, selection)
    ElMessage.success(
      t('admin.models.importDone', {
        created: result.created,
        overwritten: result.overwritten,
        skipped: result.skipped,
      }),
    )
    importVisible.value = false
    await loadModels()
  } catch (error) {
    runError.value = `${t('admin.models.importFailed')}：${messageOf(error)}`
  } finally {
    importRunning.value = false
  }
}

onBeforeUnmount(() => window.clearTimeout(keywordTimer))

// ---------- 关联提示词 ----------

const relatedVisible = ref(false)
const relatedModelID = ref(0)
const relatedRows = ref<AdminModelPrompt[]>([])
const relatedTotal = ref(0)
const relatedPage = ref(1)
const relatedLoading = ref(false)
const relatedError = ref('')

function openRelated(row: AdminModel): void {
  relatedModelID.value = row.id
  relatedPage.value = 1
  relatedVisible.value = true
  void loadRelated()
}

async function loadRelated(): Promise<void> {
  relatedLoading.value = true
  try {
    const result = await adminApi.modelPrompts(relatedModelID.value, relatedPage.value, pageSize)
    relatedRows.value = result.items
    relatedTotal.value = result.total
    relatedError.value = ''
  } catch (error) {
    relatedError.value = messageOf(error)
  } finally {
    relatedLoading.value = false
  }
}

function changeRelatedPage(next: number): void {
  relatedPage.value = next
  void loadRelated()
}

// ---------- 完成度统计 ----------

const statsVisible = ref(false)
const statsModelID = ref(0)
const statsRows = ref<AdminModelStat[]>([])
const statsTotal = ref(0)
const statsPage = ref(1)
const statsLoading = ref(false)
const statsError = ref('')

function openStats(row: AdminModel): void {
  statsModelID.value = row.id
  statsPage.value = 1
  statsVisible.value = true
  void loadStats()
}

async function loadStats(): Promise<void> {
  statsLoading.value = true
  try {
    const result = await adminApi.modelStats(statsModelID.value, statsPage.value, pageSize)
    statsRows.value = result.items
    statsTotal.value = result.total
    statsError.value = ''
  } catch (error) {
    statsError.value = messageOf(error)
  } finally {
    statsLoading.value = false
  }
}

function changeStatsPage(next: number): void {
  statsPage.value = next
  void loadStats()
}

// ---------- 原始快照 ----------

const snapshotVisible = ref(false)
const snapshotModelID = ref(0)
const snapshotRows = ref<AdminModelSnapshot[]>([])
const snapshotTotal = ref(0)
const snapshotPage = ref(1)
const snapshotLoading = ref(false)
const snapshotError = ref('')

function openSnapshots(row: AdminModel): void {
  snapshotModelID.value = row.id
  snapshotPage.value = 1
  snapshotVisible.value = true
  void loadSnapshots()
}

async function loadSnapshots(): Promise<void> {
  snapshotLoading.value = true
  try {
    const result = await adminApi.modelSnapshots(
      snapshotModelID.value,
      snapshotPage.value,
      pageSize,
    )
    snapshotRows.value = result.items
    snapshotTotal.value = result.total
    snapshotError.value = ''
  } catch (error) {
    snapshotError.value = messageOf(error)
  } finally {
    snapshotLoading.value = false
  }
}

function changeSnapshotPage(next: number): void {
  snapshotPage.value = next
  void loadSnapshots()
}

// ---------- 导入记录 ----------

const logVisible = ref(false)
const logRows = ref<AdminModelImportLog[]>([])
const logTotal = ref(0)
const logPage = ref(1)
const logLoading = ref(false)
const logError = ref('')

function openImportLogs(): void {
  logPage.value = 1
  logVisible.value = true
  void loadImportLogs()
}

async function loadImportLogs(): Promise<void> {
  logLoading.value = true
  try {
    const result = await adminApi.modelImportLogs('', logPage.value, pageSize)
    logRows.value = result.items
    logTotal.value = result.total
    logError.value = ''
  } catch (error) {
    logError.value = messageOf(error)
  } finally {
    logLoading.value = false
  }
}

function changeLogPage(next: number): void {
  logPage.value = next
  void loadImportLogs()
}

onMounted(async () => {
  await Promise.all([loadModels(), dictionary.load()])
})
</script>

<template>
  <section class="pb-admin-module">
    <header class="pb-admin-module__head">
      <h2 class="pb-admin-module__title">{{ t('admin.models.title') }}</h2>
      <div class="pb-admin-module__actions">
        <el-button
          type="primary"
          size="small"
          data-testid="admin-models-create"
          @click="openCreate"
        >
          {{ t('admin.models.create') }}
        </el-button>
        <el-button size="small" data-testid="admin-models-import" @click="openImport">
          {{ t('admin.models.import') }}
        </el-button>
        <el-button size="small" data-testid="admin-models-import-logs" @click="openImportLogs">
          {{ t('admin.models.importLogs') }}
        </el-button>
      </div>
    </header>

    <div class="pb-admin-module__filters">
      <el-input
        v-model="keyword"
        class="pb-admin-module__filter"
        :placeholder="t('admin.models.keyword')"
        clearable
        data-testid="admin-models-keyword"
        @keyup.enter="search"
      />
      <div class="pb-admin-module__switcher">
        <el-switch
          v-model="includeDeleted"
          size="small"
          data-testid="admin-models-include-deleted"
          @change="search"
        />
        <span class="pb-admin-module__hint">{{ t('admin.models.includeDeleted') }}</span>
      </div>
      <el-button type="primary" size="small" data-testid="admin-models-search" @click="search">
        {{ t('admin.common.search') }}
      </el-button>
      <el-button size="small" data-testid="admin-models-reset" @click="resetFilters">
        {{ t('admin.common.reset') }}
      </el-button>
    </div>

    <p v-if="failure" class="pb-admin-module__error" data-testid="admin-models-error">{{ failure }}</p>
    <p
      v-if="!loading && rows.length === 0"
      class="pb-admin-module__empty"
      data-testid="admin-models-empty"
    >
      {{ t('admin.common.empty') }}
    </p>

    <el-table v-if="rows.length > 0" :data="rows" size="small" data-testid="admin-models-table">
      <el-table-column prop="name" :label="t('admin.models.name')" min-width="150" />
      <el-table-column prop="model_key" :label="t('admin.models.modelKey')" min-width="170" />
      <el-table-column prop="provider_name" :label="t('admin.models.providerName')" min-width="130" />
      <el-table-column :label="t('admin.models.inputModalities')" min-width="130">
        <template #default="{ row }: { row: AdminModel }">
          {{ row.input_modalities.join(',') }}
        </template>
      </el-table-column>
      <el-table-column :label="t('admin.common.enabled')" width="90">
        <template #default="{ row }: { row: AdminModel }">
          <el-tag size="small" :type="row.is_enabled ? 'success' : 'info'">
            {{ row.is_enabled ? t('admin.common.enabled') : t('admin.common.disabled') }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('admin.models.importSource')" width="110">
        <template #default="{ row }: { row: AdminModel }">
          {{ importSourceLabel(row.import_source) }}
        </template>
      </el-table-column>
      <el-table-column prop="rating_count" :label="t('admin.models.ratingCount')" width="90" />
      <el-table-column prop="prompt_count" :label="t('admin.models.promptCount')" width="90" />
      <el-table-column :label="t('admin.common.updatedAt')" width="170">
        <template #default="{ row }: { row: AdminModel }">{{ formatTime(row.updated_at) }}</template>
      </el-table-column>
      <el-table-column :label="t('admin.common.actions')" width="152" fixed="right">
        <template #default="{ row }: { row: AdminModel }">
          <div class="pb-admin-module__row-actions">
            <el-button size="small" text data-testid="admin-models-edit" @click="openEdit(row)">
              {{ t('admin.common.edit') }}
            </el-button>
            <el-dropdown
              trigger="click"
              placement="bottom-end"
              @command="(command: string) => runModelAction(command, row)"
            >
              <el-button size="small" text data-testid="admin-models-more">
                {{ t('admin.common.more') }}
                <span class="pb-admin-module__caret" aria-hidden="true">▾</span>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="toggle" data-testid="admin-models-enable">
                    {{ row.is_enabled ? t('admin.models.disable') : t('admin.models.enable') }}
                  </el-dropdown-item>
                  <el-dropdown-item command="related" data-testid="admin-models-related">
                    {{ t('admin.models.relatedPrompts') }}
                  </el-dropdown-item>
                  <el-dropdown-item command="stats" data-testid="admin-models-stats">
                    {{ t('admin.models.modelStats') }}
                  </el-dropdown-item>
                  <el-dropdown-item command="snapshots" data-testid="admin-models-snapshots">
                    {{ t('admin.models.snapshots') }}
                  </el-dropdown-item>
                  <el-dropdown-item
                    command="delete"
                    divided
                    class="pb-admin-module__danger"
                    data-testid="admin-models-delete"
                  >
                    {{ t('admin.common.remove') }}
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
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
      v-model="formVisible"
      :title="editingID === null ? t('admin.models.createTitle') : t('admin.models.editTitle')"
      width="640px"
    >
      <el-form :model="form" label-width="130px">
        <el-form-item :label="t('admin.models.name')">
          <el-input v-model="form.name" data-testid="admin-models-form-name" />
        </el-form-item>
        <el-form-item :label="t('admin.models.modelKey')">
          <el-input v-model="form.model_key" data-testid="admin-models-form-key" />
        </el-form-item>
        <el-form-item :label="t('admin.models.providerKey')">
          <el-input v-model="form.provider_key" />
        </el-form-item>
        <el-form-item :label="t('admin.models.providerName')">
          <el-input v-model="form.provider_name" />
        </el-form-item>
        <el-form-item :label="t('admin.models.description')">
          <el-input v-model="form.description" type="textarea" :rows="2" />
        </el-form-item>
        <el-form-item :label="t('admin.models.releaseDate')">
          <el-date-picker
            v-model="form.release_date"
            class="pb-admin-module__control"
            type="date"
            value-format="YYYY-MM-DD"
          />
        </el-form-item>
        <el-form-item :label="t('admin.models.contextLength')">
          <el-input-number v-model="form.context_length" class="pb-admin-module__control" :min="0" />
        </el-form-item>
        <el-form-item :label="t('admin.models.inputModalities')">
          <el-checkbox-group v-model="form.input_modalities">
            <el-checkbox v-for="modality in INPUT_MODALITIES" :key="modality" :label="modality">
              {{ modality }}
            </el-checkbox>
          </el-checkbox-group>
        </el-form-item>
        <el-form-item :label="t('admin.models.aliases')">
          <el-select
            v-model="form.aliases"
            class="pb-admin-module__control"
            multiple
            filterable
            allow-create
            default-first-option
            :reserve-keyword="false"
          />
        </el-form-item>
        <el-form-item :label="t('admin.models.capabilities')">
          <el-select
            v-model="form.capability_ids"
            class="pb-admin-module__control"
            multiple
            filterable
            :reserve-keyword="false"
          >
            <el-option
              v-for="capability in dictionary.data.capabilities"
              :key="capability.id"
              :label="
                locale === 'en' ? capability.name_en : capability.name_zh
              "
              :value="capability.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('admin.common.enabled')">
          <el-switch v-model="form.is_enabled" />
        </el-form-item>
        <el-form-item :label="t('admin.models.note')">
          <el-input v-model="form.note" type="textarea" :rows="2" />
        </el-form-item>
      </el-form>
      <p v-if="formError" class="pb-admin-module__error">{{ formError }}</p>
      <template #footer>
        <el-button data-testid="admin-models-form-cancel" @click="formVisible = false">
          {{ t('admin.common.cancel') }}
        </el-button>
        <el-button
          type="primary"
          :disabled="!canSubmit"
          data-testid="admin-models-form-submit"
          @click="submitForm"
        >
          {{ editingID === null ? t('admin.common.create') : t('admin.common.save') }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="importVisible"
      class="pb-import-dialog"
      :title="t('admin.models.importTitle')"
      width="min(1080px, 94vw)"
      top="6vh"
      :close-on-click-modal="!importRunning"
      :close-on-press-escape="!importRunning"
      :show-close="!importRunning"
    >
      <div class="pb-import">
        <div v-if="previewError !== ''" class="pb-import__failure">
          <p class="pb-admin-module__error">
            {{ t('admin.models.previewFailed') }}：{{ previewError }}
          </p>
          <p class="pb-admin-module__hint">{{ t('admin.models.badGatewayHint') }}</p>
          <el-button
            size="small"
            data-testid="admin-models-import-retry"
            @click="loadPreview(true)"
          >
            {{ t('admin.models.importRetry') }}
          </el-button>
        </div>
        <div v-else-if="importPreview === null" class="pb-import__loading">
          <span class="pb-import__pulse" />
          <span>{{ t('admin.models.importLoading') }}</span>
        </div>
        <template v-else>
          <header class="pb-import__summary">
            <div class="pb-import__figures">
              <span class="pb-import__figure" data-testid="admin-models-import-figure-total">
                <strong>{{ importCatalogTotal }}</strong>
                <span>{{ t('admin.models.importFigureTotal') }}</span>
              </span>
              <span class="pb-import__figure" data-testid="admin-models-import-figure-duplicate">
                <strong>{{ importDuplicateCount }}</strong>
                <span>{{ t('admin.models.importFigureDuplicate') }}</span>
              </span>
              <span class="pb-import__figure" data-testid="admin-models-import-figure-matched">
                <strong>{{ importTotal }}</strong>
                <span>{{ t('admin.models.importFigureMatched') }}</span>
              </span>
            </div>
            <div class="pb-import__catalog">
              <span v-if="importFetchedAt" class="pb-admin-module__hint">
                {{ t('admin.models.importFetchedAt', { time: importFetchedAt }) }}
              </span>
              <el-button
                size="small"
                text
                :loading="importLoading"
                data-testid="admin-models-import-refresh"
                @click="refreshImportCatalog"
              >
                {{ t('admin.models.importRefresh') }}
              </el-button>
            </div>
          </header>

          <div class="pb-import__toolbar">
            <el-input
              v-model="importKeyword"
              class="pb-import__search"
              :placeholder="t('admin.models.importSearch')"
              clearable
              data-testid="admin-models-import-search"
              @input="scheduleImportSearch"
              @keyup.enter="applyImportSearch"
              @clear="applyImportSearch"
            />
            <el-select
              v-model="importProvider"
              class="pb-import__provider"
              clearable
              filterable
              :placeholder="t('admin.models.importProviderAll')"
              data-testid="admin-models-import-provider"
              @change="changeImportProvider"
            >
              <el-option
                v-for="provider in importProviders"
                :key="provider.id"
                :value="provider.id"
                :label="`${provider.name} · ${provider.total}`"
              />
            </el-select>
            <el-radio-group
              v-model="importStatus"
              size="small"
              data-testid="admin-models-import-status"
              @change="changeImportStatus"
            >
              <el-radio-button value="">{{ t('admin.models.importStatusAll') }}</el-radio-button>
              <el-radio-button value="new">{{ t('admin.models.importStatusNew') }}</el-radio-button>
              <el-radio-button value="duplicate">
                {{ t('admin.models.importStatusDuplicate') }}
              </el-radio-button>
            </el-radio-group>
            <el-button
              size="small"
              text
              data-testid="admin-models-import-reset"
              @click="resetImportFilters"
            >
              {{ t('admin.common.reset') }}
            </el-button>
          </div>

          <Transition name="pb-import-bar">
            <div v-if="importSelectedCount > 0" class="pb-import__selection">
              <span class="pb-import__selection-count" data-testid="admin-models-import-selected">
                {{
                  importSelectAll
                    ? t('admin.models.importSelectedFiltered', { count: importSelectedCount })
                    : t('admin.models.importSelected', { count: importSelectedCount })
                }}
              </span>
              <span class="pb-import__selection-actions">
                <el-button
                  size="small"
                  text
                  data-testid="admin-models-import-clear"
                  @click="clearImportSelection"
                >
                  {{ t('admin.models.importClearSelection') }}
                </el-button>
              </span>
            </div>
          </Transition>

          <div class="pb-import__table" :class="{ 'is-refreshing': importLoading }">
            <el-table
              :data="importItems"
              size="small"
              max-height="400"
              :row-class-name="importRowClass"
              data-testid="admin-models-import-table"
            >
              <el-table-column width="46" align="center">
                <template #header>
                  <el-checkbox
                    :model-value="importPageSelected"
                    :indeterminate="importPageIndeterminate"
                    data-testid="admin-models-import-select-page"
                    @change="setImportPageSelected(Boolean($event))"
                  />
                </template>
                <template #default="{ row }: { row: AdminModelImportEntry }">
                  <el-checkbox
                    :model-value="isImportSelected(row)"
                    data-testid="admin-models-import-select"
                    @change="setImportSelected(row, Boolean($event))"
                  />
                </template>
              </el-table-column>
              <el-table-column :label="t('admin.models.name')" min-width="210">
                <template #default="{ row }: { row: AdminModelImportEntry }">
                  <div class="pb-import__name">
                    <span class="pb-import__name-text" :title="row.description">
                      {{ row.name }}
                    </span>
                    <el-tag
                      v-if="row.duplicate"
                      class="pb-admin-module__badge"
                      type="warning"
                      size="small"
                      effect="plain"
                      :title="duplicateText(row)"
                    >
                      {{ t('admin.models.importStatusDuplicate') }}
                    </el-tag>
                  </div>
                </template>
              </el-table-column>
              <el-table-column
                prop="provider_name"
                :label="t('admin.models.providerName')"
                width="150"
                show-overflow-tooltip
              />
              <el-table-column
                prop="model_id"
                :label="t('admin.models.modelKey')"
                min-width="170"
                show-overflow-tooltip
              />
              <el-table-column prop="release_date" :label="t('admin.models.releaseDate')" width="110" />
              <el-table-column :label="t('admin.models.inputModalities')" width="130">
                <template #default="{ row }: { row: AdminModelImportEntry }">
                  <span class="pb-import__modalities">
                    {{ row.input_modalities.length > 0 ? row.input_modalities.join(' · ') : t('admin.common.none') }}
                  </span>
                </template>
              </el-table-column>
              <el-table-column
                prop="context_length"
                :label="t('admin.models.contextLength')"
                width="110"
              />
            </el-table>
            <p v-if="!importLoading && importItems.length === 0" class="pb-admin-module__empty">
              {{ t('admin.models.importEmpty') }}
            </p>
          </div>

          <div class="pb-import__foot">
            <span class="pb-import__foot-info">
              <span class="pb-admin-module__hint">
                {{ t('admin.models.importPageHint', { total: importTotal }) }}
              </span>
              <el-button
                v-if="importSelectAll === null && importTotal > importItems.length"
                size="small"
                text
                data-testid="admin-models-import-select-all"
                @click="selectAllImportMatches"
              >
                {{ t('admin.models.importSelectAll', { count: importTotal }) }}
              </el-button>
            </span>
            <el-pagination
              v-if="importTotal > 0"
              layout="sizes, prev, pager, next"
              :current-page="importPage"
              :page-size="importPageSize"
              :page-sizes="[20, 50, 100]"
              :total="importTotal"
              @current-change="changeImportPage"
              @size-change="changeImportPageSize"
            />
          </div>
        </template>
      </div>
      <p v-if="runError !== ''" class="pb-admin-module__error">{{ runError }}</p>
      <Transition name="pb-import-bar">
        <div v-if="importRunning" class="pb-import__running" data-testid="admin-models-import-running">
          <span class="pb-import__pulse" />
          {{ t('admin.models.importRunning', { count: importSelectedCount }) }}
        </div>
      </Transition>
      <template #footer>
        <div class="pb-import__footer">
          <label class="pb-import__overwrite">
            <el-switch
              v-model="importOverwrite"
              size="small"
              :disabled="importRunning"
              data-testid="admin-models-import-overwrite"
            />
            <span>{{ t('admin.models.importOverwrite') }}</span>
            <el-tooltip :content="t('admin.models.importDuplicateHint')" placement="top">
              <span class="pb-import__tip">?</span>
            </el-tooltip>
          </label>
          <div class="pb-import__footer-actions">
            <el-button :disabled="importRunning" @click="importVisible = false">
              {{ t('admin.common.cancel') }}
            </el-button>
            <el-button
              type="primary"
              :disabled="previewError !== '' || importSelectedCount === 0"
              :loading="importRunning"
              data-testid="admin-models-import-run"
              @click="runImport"
            >
              {{ t('admin.models.importRunSelected', { count: importSelectedCount }) }}
            </el-button>
          </div>
        </div>
      </template>
    </el-dialog>

    <el-dialog v-model="relatedVisible" :title="t('admin.models.relatedPrompts')" width="720px">
      <p v-if="relatedError" class="pb-admin-module__error">{{ relatedError }}</p>
      <p v-if="!relatedLoading && relatedRows.length === 0" class="pb-admin-module__empty">
        {{ t('admin.common.empty') }}
      </p>
      <el-table v-if="relatedRows.length > 0" :data="relatedRows" size="small">
        <el-table-column :label="t('admin.models.name')" min-width="240">
          <template #default="{ row }: { row: AdminModelPrompt }">
            <router-link :to="`/prompts/${row.id}`">{{ row.title }}</router-link>
          </template>
        </el-table-column>
        <el-table-column prop="visibility" label="visibility" width="140" />
        <el-table-column :label="t('admin.common.createdAt')" width="180">
          <template #default="{ row }: { row: AdminModelPrompt }">
            {{ formatTime(row.created_at) }}
          </template>
        </el-table-column>
      </el-table>
      <el-pagination
        v-if="relatedTotal > pageSize"
        class="pb-admin-module__pager"
        layout="prev, pager, next"
        :current-page="relatedPage"
        :page-size="pageSize"
        :total="relatedTotal"
        @current-change="changeRelatedPage"
      />
      <template #footer>
        <el-button @click="relatedVisible = false">{{ t('admin.common.close') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="statsVisible" :title="t('admin.models.modelStats')" width="760px">
      <p v-if="statsError" class="pb-admin-module__error">{{ statsError }}</p>
      <p v-if="!statsLoading && statsRows.length === 0" class="pb-admin-module__empty">
        {{ t('admin.common.empty') }}
      </p>
      <el-table v-if="statsRows.length > 0" :data="statsRows" size="small">
        <el-table-column :label="t('admin.models.name')" min-width="180">
          <template #default="{ row }: { row: AdminModelStat }">{{ row.prompt_title }}</template>
        </el-table-column>
        <el-table-column
          prop="history_rating_count"
          :label="t('admin.models.historyCount')"
          width="110"
        />
        <el-table-column
          prop="history_adjusted"
          :label="t('admin.models.historyAdjusted')"
          width="110"
        />
        <el-table-column
          prop="recent_rating_count"
          :label="t('admin.models.recentCount')"
          width="110"
        />
        <el-table-column
          prop="recent_adjusted"
          :label="t('admin.models.recentAdjusted')"
          width="110"
        />
        <el-table-column :label="t('admin.models.computedAt')" width="170">
          <template #default="{ row }: { row: AdminModelStat }">
            {{ formatTime(row.computed_at) }}
          </template>
        </el-table-column>
      </el-table>
      <el-pagination
        v-if="statsTotal > pageSize"
        class="pb-admin-module__pager"
        layout="prev, pager, next"
        :current-page="statsPage"
        :page-size="pageSize"
        :total="statsTotal"
        @current-change="changeStatsPage"
      />
      <template #footer>
        <el-button @click="statsVisible = false">{{ t('admin.common.close') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="snapshotVisible" :title="t('admin.models.snapshots')" width="720px">
      <p v-if="snapshotError" class="pb-admin-module__error">{{ snapshotError }}</p>
      <p v-if="!snapshotLoading && snapshotRows.length === 0" class="pb-admin-module__empty">
        {{ t('admin.common.empty') }}
      </p>
      <el-table v-if="snapshotRows.length > 0" :data="snapshotRows" size="small">
        <el-table-column :label="t('admin.common.detail')" min-width="420">
          <template #default="{ row }: { row: AdminModelSnapshot }">
            <pre class="pb-admin-module__json">{{ snapshotText(row.raw) }}</pre>
          </template>
        </el-table-column>
        <el-table-column :label="t('admin.common.createdAt')" width="180">
          <template #default="{ row }: { row: AdminModelSnapshot }">
            {{ formatTime(row.created_at) }}
          </template>
        </el-table-column>
      </el-table>
      <el-pagination
        v-if="snapshotTotal > pageSize"
        class="pb-admin-module__pager"
        layout="prev, pager, next"
        :current-page="snapshotPage"
        :page-size="pageSize"
        :total="snapshotTotal"
        @current-change="changeSnapshotPage"
      />
      <template #footer>
        <el-button @click="snapshotVisible = false">{{ t('admin.common.close') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="logVisible" :title="t('admin.models.importLogs')" width="900px">
      <p v-if="logError" class="pb-admin-module__error">{{ logError }}</p>
      <p v-if="!logLoading && logRows.length === 0" class="pb-admin-module__empty">
        {{ t('admin.common.empty') }}
      </p>
      <el-table v-if="logRows.length > 0" :data="logRows" size="small">
        <el-table-column prop="action" :label="t('admin.common.actions')" width="110" />
        <el-table-column prop="provider_key" :label="t('admin.models.providerKey')" width="150" />
        <el-table-column prop="model_key" :label="t('admin.models.modelKey')" min-width="170" />
        <el-table-column prop="operator_name" label="operator_name" width="140" />
        <el-table-column :label="t('admin.common.createdAt')" width="170">
          <template #default="{ row }: { row: AdminModelImportLog }">
            {{ formatTime(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column :label="t('admin.common.detail')" min-width="220">
          <template #default="{ row }: { row: AdminModelImportLog }">
            {{ payloadText(row.payload) }}
          </template>
        </el-table-column>
      </el-table>
      <el-pagination
        v-if="logTotal > pageSize"
        class="pb-admin-module__pager"
        layout="prev, pager, next"
        :current-page="logPage"
        :page-size="pageSize"
        :total="logTotal"
        @current-change="changeLogPage"
      />
      <template #footer>
        <el-button @click="logVisible = false">{{ t('admin.common.close') }}</el-button>
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

.pb-admin-module__actions {
  display: flex;
  gap: var(--pb-space-xs);
}

.pb-admin-module__hint {
  color: var(--pb-color-text-faint);
  font-size: 13px;
  margin: var(--pb-space-xs) 0;
}

.pb-admin-module__filters {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-xs);
  margin: var(--pb-space-sm) 0;
}

.pb-admin-module__filter {
  max-width: 200px;
}

.pb-admin-module__switcher {
  align-items: center;
  display: flex;
  gap: var(--pb-space-xs);
}

.pb-admin-module__control {
  width: 100%;
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

.pb-admin-module__row-actions {
  align-items: center;
  display: flex;
  gap: 2px;
  white-space: nowrap;
}

.pb-admin-module__caret {
  font-size: 10px;
  margin-left: 3px;
}

:global(.el-dropdown-menu__item.pb-admin-module__danger) {
  color: var(--el-color-danger);
}

.pb-admin-module__badge {
  margin-left: var(--pb-space-xs);
}

.pb-admin-module__json {
  font-size: 12px;
  margin: 0;
  max-height: 160px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}

/* ---------- models.dev 导入工作台 ---------- */

.pb-import {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
}

.pb-import__failure {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
  align-items: flex-start;
}

.pb-import__loading {
  align-items: center;
  color: var(--pb-ink-soft);
  display: flex;
  font-size: 14px;
  gap: var(--pb-space-sm);
  justify-content: center;
  min-height: 220px;
}

.pb-import__summary {
  align-items: center;
  background:
    linear-gradient(120deg, var(--pb-surface-muted), transparent 68%),
    var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-md);
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-md);
  justify-content: space-between;
  padding: 12px 16px;
}

.pb-import__figures {
  align-items: baseline;
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-lg);
}

.pb-import__figure {
  align-items: baseline;
  color: var(--pb-ink-faint);
  display: flex;
  font-size: 13px;
  gap: 6px;
}

.pb-import__figure strong {
  color: var(--pb-ink);
  font-family: var(--pb-font-display);
  font-size: 22px;
  font-variant-numeric: tabular-nums;
  font-weight: 500;
  line-height: 1;
}

.pb-import__catalog {
  align-items: center;
  display: flex;
  gap: var(--pb-space-xs);
}

.pb-import__toolbar {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-sm);
}

.pb-import__search {
  width: 280px;
}

.pb-import__provider {
  width: 220px;
}

.pb-import__selection {
  align-items: center;
  background: var(--pb-accent-soft);
  border: 1px solid var(--pb-accent-ring);
  border-radius: var(--pb-radius-sm);
  color: var(--pb-accent-strong);
  display: flex;
  flex-wrap: wrap;
  font-size: 13px;
  gap: var(--pb-space-sm);
  justify-content: space-between;
  padding: 6px 12px;
}

.pb-import__selection-count {
  font-variant-numeric: tabular-nums;
  font-weight: 500;
}

.pb-import__selection-actions {
  display: flex;
  gap: var(--pb-space-xs);
}

.pb-import__table {
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-md);
  overflow: hidden;
  transition: opacity var(--pb-duration) var(--pb-ease);
}

.pb-import__table.is-refreshing {
  opacity: 0.55;
}

.pb-import__name {
  align-items: center;
  display: flex;
  gap: 6px;
  min-width: 0;
}

.pb-import__name-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pb-import__modalities {
  color: var(--pb-ink-soft);
}

.pb-import__foot {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-sm);
  justify-content: space-between;
}

.pb-import__foot-info {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-xs);
}

.pb-import__running {
  align-items: center;
  background: var(--pb-surface-muted);
  border-radius: var(--pb-radius-sm);
  color: var(--pb-ink-soft);
  display: flex;
  font-size: 13px;
  gap: var(--pb-space-sm);
  margin-top: var(--pb-space-sm);
  padding: 8px 12px;
}

.pb-import__pulse {
  animation: pb-import-pulse 1.1s ease-in-out infinite;
  background: var(--pb-accent);
  border-radius: 50%;
  flex: none;
  height: 8px;
  width: 8px;
}

@keyframes pb-import-pulse {
  0%,
  100% {
    opacity: 0.35;
    transform: scale(0.8);
  }

  50% {
    opacity: 1;
    transform: scale(1);
  }
}

.pb-import__footer {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-md);
  justify-content: space-between;
}

.pb-import__overwrite {
  align-items: center;
  color: var(--pb-ink-soft);
  display: flex;
  font-size: 13px;
  gap: var(--pb-space-sm);
}

.pb-import__tip {
  align-items: center;
  border: 1px solid var(--pb-line-strong);
  border-radius: 50%;
  color: var(--pb-ink-faint);
  cursor: help;
  display: inline-flex;
  font-size: 11px;
  height: 16px;
  justify-content: center;
  width: 16px;
}

.pb-import__footer-actions {
  display: flex;
  gap: var(--pb-space-sm);
}

.pb-import :deep(.el-table__row) {
  transition: background-color var(--pb-duration-fast) var(--pb-ease);
}

.pb-import :deep(.el-table__row:hover > td.el-table__cell) {
  background-color: var(--pb-surface-muted);
}

.pb-import :deep(.pb-import__row--selected > td.el-table__cell) {
  background-color: var(--pb-accent-soft);
}

.pb-import :deep(.pb-import__row--selected:hover > td.el-table__cell) {
  background-color: var(--pb-accent-soft);
}

.pb-import-bar-enter-active,
.pb-import-bar-leave-active {
  transition:
    opacity 160ms var(--pb-ease),
    transform 160ms var(--pb-ease);
}

.pb-import-bar-enter-from,
.pb-import-bar-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}

@media (max-width: 720px) {
  .pb-import__search,
  .pb-import__provider {
    width: 100%;
  }

  .pb-import__figures {
    gap: var(--pb-space-md);
  }

  .pb-import__footer {
    align-items: stretch;
    flex-direction: column;
  }

  .pb-import__footer-actions {
    justify-content: flex-end;
  }
}
</style>
