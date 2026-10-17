// 模型管理（需求 §17.3、§13）。
// model_key 在模型之间必须唯一，重复的键由后端判定并返回 duplicate_model。

<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  type AdminModel,
  type AdminModelImportEntry,
  type AdminModelImportLog,
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
const importOverwrite = ref(false)
const importItems = ref<AdminModelImportEntry[]>([])
const importDuplicates = ref(0)
const previewError = ref('')
const runError = ref('')

function openImport(): void {
  importVisible.value = true
  importLoading.value = true
  importOverwrite.value = false
  importItems.value = []
  importDuplicates.value = 0
  previewError.value = ''
  runError.value = ''
  void loadPreview()
}

/** loadPreview 拉取 models.dev 的导入清单，上游不可用时后端返回 502。 */
async function loadPreview(): Promise<void> {
  try {
    const result = await adminApi.previewModelImport()
    importItems.value = result.items
    importDuplicates.value = result.duplicate_count
    previewError.value = ''
  } catch (error) {
    importItems.value = []
    importDuplicates.value = 0
    previewError.value = messageOf(error)
  } finally {
    importLoading.value = false
  }
}

async function runImport(): Promise<void> {
  runError.value = ''
  try {
    const result = await adminApi.importModels(importOverwrite.value)
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
  }
}

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
      <el-table-column :label="t('admin.common.actions')" width="300">
        <template #default="{ row }: { row: AdminModel }">
          <el-button size="small" text data-testid="admin-models-edit" @click="openEdit(row)">
            {{ t('admin.common.edit') }}
          </el-button>
          <el-button size="small" text data-testid="admin-models-enable" @click="toggleEnabled(row)">
            {{ row.is_enabled ? t('admin.models.disable') : t('admin.models.enable') }}
          </el-button>
          <el-button size="small" text data-testid="admin-models-delete" @click="removeModel(row)">
            {{ t('admin.common.remove') }}
          </el-button>
          <el-button size="small" text data-testid="admin-models-related" @click="openRelated(row)">
            {{ t('admin.models.relatedPrompts') }}
          </el-button>
          <el-button size="small" text data-testid="admin-models-stats" @click="openStats(row)">
            {{ t('admin.models.modelStats') }}
          </el-button>
          <el-button size="small" text data-testid="admin-models-snapshots" @click="openSnapshots(row)">
            {{ t('admin.models.snapshots') }}
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

    <el-dialog v-model="importVisible" :title="t('admin.models.importTitle')" width="720px">
      <div v-if="previewError !== ''">
        <p class="pb-admin-module__error">
          {{ t('admin.models.previewFailed') }}：{{ previewError }}
        </p>
        <p class="pb-admin-module__hint">{{ t('admin.models.badGatewayHint') }}</p>
      </div>
      <template v-else>
        <p class="pb-admin-module__hint">
          {{
            t('admin.models.importSummary', {
              total: importItems.length,
              duplicates: importDuplicates,
            })
          }}
        </p>
        <p class="pb-admin-module__hint">{{ t('admin.models.importDuplicateHint') }}</p>
        <div class="pb-admin-module__switcher">
          <el-switch
            v-model="importOverwrite"
            size="small"
            data-testid="admin-models-import-overwrite"
          />
          <span class="pb-admin-module__hint">{{ t('admin.models.importOverwrite') }}</span>
        </div>
        <p v-if="!importOverwrite" class="pb-admin-module__hint">
          {{ t('admin.models.importSkipHint') }}
        </p>
        <el-table v-loading="importLoading" :data="importItems" size="small" max-height="360">
          <el-table-column prop="provider_name" :label="t('admin.models.providerName')" width="140" />
          <el-table-column prop="model_id" :label="t('admin.models.modelKey')" min-width="160" />
          <el-table-column :label="t('admin.models.name')" min-width="200">
            <template #default="{ row }: { row: AdminModelImportEntry }">
              <span>{{ row.name }}</span>
              <el-tag
                v-if="row.duplicate"
                class="pb-admin-module__badge"
                type="warning"
                size="small"
              >
                {{ duplicateText(row) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="release_date" :label="t('admin.models.releaseDate')" width="120" />
          <el-table-column :label="t('admin.models.inputModalities')" width="140">
            <template #default="{ row }: { row: AdminModelImportEntry }">
              {{ row.input_modalities.join(',') }}
            </template>
          </el-table-column>
          <el-table-column
            prop="context_length"
            :label="t('admin.models.contextLength')"
            width="110"
          />
        </el-table>
      </template>
      <p v-if="runError !== ''" class="pb-admin-module__error">{{ runError }}</p>
      <template #footer>
        <el-button
          type="primary"
          :disabled="previewError !== ''"
          data-testid="admin-models-import-run"
          @click="runImport"
        >
          {{ t('admin.models.importRun') }}
        </el-button>
        <el-button @click="importVisible = false">{{ t('admin.common.cancel') }}</el-button>
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
</style>
