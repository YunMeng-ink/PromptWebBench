// 标签管理与推荐标签（需求 §17.5 与第 6 章）。
// 标签页签维护标签本身，标识重复等校验错误留在对话框里就地显示；
// 推荐标签页签按统计触发的记录做确认与忽略，批量处置与单行处置走同一个接口。

<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  type AdminRecommendTag,
  type AdminTag,
  type AdminTagInput,
  adminApi,
  RECOMMEND_TAG_KINDS,
  RECOMMEND_TAG_STATES,
  type RecommendTagState,
} from '@/api/admin'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { formatDateTime } from '@/utils/datetime'

const { t, locale } = useI18n()
const { messageOf } = useErrorMessage()

/** TabName 是两个页签的取值。 */
type TabName = 'tags' | 'recommend'

const activeTab = ref<TabName>('tags')

// ---------- 标签页签 ----------

const tags = ref<AdminTag[]>([])
const tagTotal = ref(0)
const tagPage = ref(1)
const tagPageSize = 20
const tagLoading = ref(false)
const tagFailure = ref('')
const tagKeyword = ref('')

const formVisible = ref(false)
const editingTag = ref<AdminTag | null>(null)
const form = ref<AdminTagInput>({
  slug: '',
  name_zh: '',
  name_en: '',
  color: '#409eff',
  description: '',
  kind: 'theme',
  priority: 0,
  is_enabled: true,
})
const formFailure = ref('')
const saving = ref(false)

// ---------- 推荐标签页签 ----------

const recommendRows = ref<AdminRecommendTag[]>([])
const recommendTotal = ref(0)
const recommendPage = ref(1)
const recommendPageSize = 20
const recommendLoading = ref(false)
const recommendFailure = ref('')
const recommendStates = ref<RecommendTagState[]>(['pending'])
const recommendKind = ref('')
const selectedIds = ref<number[]>([])

/** stateText 把推荐标签状态映射为文案；未登记的取值原样显示。 */
function stateText(state: string): string {
  const labels: Record<string, string> = {
    pending: t('admin.tags.statePending'),
    review: t('admin.tags.stateReview'),
    confirmed: t('admin.tags.stateConfirmed'),
    ignored: t('admin.tags.stateIgnored'),
    expired: t('admin.tags.stateExpired'),
  }
  return labels[state] ?? state
}

/** kindText 把推荐标签类型映射为文案；未登记的取值原样显示。 */
function kindText(kind: string): string {
  const labels: Record<string, string> = {
    obsolete: t('admin.tags.kindObsolete'),
    suspected_obsolete: t('admin.tags.kindSuspectedObsolete'),
    saturation: t('admin.tags.kindSaturation'),
    suspected_saturation: t('admin.tags.kindSuspectedSaturation'),
  }
  return labels[kind] ?? kind
}

/** confidenceText 把置信度映射为文案；未登记的取值原样显示。 */
function confidenceText(confidence: string): string {
  const labels: Record<string, string> = {
    high: t('admin.tags.confidenceHigh'),
    medium: t('admin.tags.confidenceMedium'),
    low: t('admin.tags.confidenceLow'),
  }
  return labels[confidence] ?? confidence
}

/** formatTime 把接口返回的时刻按本机时区呈现。 */
function formatTime(raw: string): string {
  if (raw === '') {
    return ''
  }
  return formatDateTime(raw, locale.value)
}

async function loadTags(): Promise<void> {
  tagLoading.value = true
  try {
    const result = await adminApi.tags({
      page: tagPage.value,
      page_size: tagPageSize,
      keyword: tagKeyword.value,
    })
    tags.value = result.items
    tagTotal.value = result.total
    tagFailure.value = ''
  } catch (error) {
    tagFailure.value = messageOf(error)
  } finally {
    tagLoading.value = false
  }
}

function searchTags(): void {
  tagPage.value = 1
  void loadTags()
}

function resetTagFilters(): void {
  tagKeyword.value = ''
  searchTags()
}

function changeTagPage(next: number): void {
  tagPage.value = next
  void loadTags()
}

async function loadRecommendTags(): Promise<void> {
  recommendLoading.value = true
  try {
    const result = await adminApi.recommendTags(
      recommendStates.value,
      recommendKind.value,
      recommendPage.value,
      recommendPageSize,
    )
    recommendRows.value = result.items
    recommendTotal.value = result.total
    // 换了页或换了筛选之后，旧选中项已经不在表里，选中集合一并清空。
    selectedIds.value = []
    recommendFailure.value = ''
  } catch (error) {
    recommendFailure.value = messageOf(error)
  } finally {
    recommendLoading.value = false
  }
}

function searchRecommendTags(): void {
  recommendPage.value = 1
  void loadRecommendTags()
}

function changeRecommendPage(next: number): void {
  recommendPage.value = next
  void loadRecommendTags()
}

function onSelectionChange(rows: AdminRecommendTag[]): void {
  selectedIds.value = rows.map((row) => row.id)
}

/** reviewTags 处置推荐标签：确认的挂到提示词上，忽略的只改状态。 */
async function reviewTags(ids: number[], state: 'confirmed' | 'ignored'): Promise<void> {
  if (ids.length === 0) {
    return
  }
  try {
    await adminApi.reviewRecommendTags(ids, state)
    ElMessage.success(t('admin.tags.reviewDone', { count: ids.length }))
    await loadRecommendTags()
  } catch (error) {
    recommendFailure.value = messageOf(error)
  }
}

function openCreate(): void {
  editingTag.value = null
  form.value = {
    slug: '',
    name_zh: '',
    name_en: '',
    color: '#409eff',
    description: '',
    kind: 'theme',
    priority: 0,
    is_enabled: true,
  }
  formFailure.value = ''
  formVisible.value = true
}

function openEdit(row: AdminTag): void {
  editingTag.value = row
  form.value = {
    slug: row.slug,
    name_zh: row.name_zh,
    name_en: row.name_en,
    color: row.color,
    description: row.description,
    kind: row.kind,
    priority: row.priority,
    is_enabled: row.is_enabled,
  }
  formFailure.value = ''
  formVisible.value = true
}

function closeForm(): void {
  formVisible.value = false
  editingTag.value = null
}

async function submitForm(): Promise<void> {
  saving.value = true
  try {
    if (editingTag.value === null) {
      await adminApi.createTag(form.value)
      ElMessage.success(t('admin.tags.created'))
    } else {
      await adminApi.updateTag(editingTag.value.id, form.value)
      ElMessage.success(t('admin.tags.updated'))
    }
    closeForm()
    await loadTags()
  } catch (error) {
    // 标识重复一类的校验错误留在对话框里，方便就地改标识后重试。
    formFailure.value = messageOf(error)
  } finally {
    saving.value = false
  }
}

/** removeTag 先二次确认：删除会把该标签从全部提示词上移除。 */
async function removeTag(row: AdminTag): Promise<void> {
  try {
    await ElMessageBox.confirm(t('admin.tags.deleteHint'), t('admin.common.confirmDelete'), {
      confirmButtonText: t('admin.common.confirm'),
      cancelButtonText: t('admin.common.cancel'),
      type: 'warning',
    })
  } catch {
    // 确认框被取消或关闭时不发请求。
    return
  }
  try {
    await adminApi.deleteTag(row.id)
    ElMessage.success(t('admin.tags.deleted'))
    await loadTags()
  } catch (error) {
    tagFailure.value = messageOf(error)
  }
}

onMounted(async () => {
  await loadTags()
  await loadRecommendTags()
})
</script>

<template>
  <section class="pb-admin-module">
    <header class="pb-admin-module__head">
      <h2 class="pb-admin-module__title" data-testid="admin-tags-title">
        {{ t('admin.tags.title') }}
      </h2>
    </header>

    <p v-if="tagFailure" class="pb-admin-module__error" data-testid="admin-tags-error">
      {{ tagFailure }}
    </p>
    <p v-if="recommendFailure" class="pb-admin-module__error" data-testid="admin-tags-error">
      {{ recommendFailure }}
    </p>

    <el-tabs v-model="activeTab">
      <el-tab-pane :label="t('admin.tags.title')" name="tags">
        <div class="pb-admin-module__filters">
          <el-input
            v-model="tagKeyword"
            class="pb-admin-module__filter"
            :placeholder="t('admin.tags.keyword')"
            clearable
            data-testid="admin-tags-keyword"
            @keyup.enter="searchTags"
          />
          <el-button type="primary" size="small" data-testid="admin-tags-search" @click="searchTags">
            {{ t('admin.common.search') }}
          </el-button>
          <el-button size="small" data-testid="admin-tags-reset" @click="resetTagFilters">
            {{ t('admin.common.reset') }}
          </el-button>
          <el-button type="primary" size="small" data-testid="admin-tags-create" @click="openCreate">
            {{ t('admin.tags.create') }}
          </el-button>
        </div>

        <p v-if="!tagLoading && tags.length === 0" class="pb-admin-module__empty">
          {{ t('admin.common.empty') }}
        </p>

        <el-table v-if="tags.length > 0" :data="tags" size="small" data-testid="admin-tags-table">
          <el-table-column prop="slug" :label="t('admin.tags.slug')" width="150" />
          <el-table-column prop="name_zh" :label="t('admin.tags.nameZh')" width="140" />
          <el-table-column prop="name_en" :label="t('admin.tags.nameEn')" width="140" />
          <el-table-column :label="t('admin.tags.color')" width="90">
            <template #default="{ row }: { row: AdminTag }">
              <span class="pb-admin-module__swatch" :style="{ backgroundColor: row.color }" />
            </template>
          </el-table-column>
          <el-table-column :label="t('admin.tags.kind')" width="110">
            <template #default="{ row }: { row: AdminTag }">
              {{ row.kind === 'form' ? t('admin.tags.kindForm') : t('admin.tags.kindTheme') }}
            </template>
          </el-table-column>
          <el-table-column prop="description" :label="t('admin.tags.description')" min-width="200" />
          <el-table-column prop="priority" :label="t('admin.tags.priority')" width="100" />
          <el-table-column :label="t('admin.common.enabled')" width="110">
            <template #default="{ row }: { row: AdminTag }">
              {{ row.is_enabled ? t('admin.common.enabled') : t('admin.common.disabled') }}
            </template>
          </el-table-column>
          <el-table-column prop="usage_count" :label="t('admin.tags.usageCount')" width="100" />
          <el-table-column :label="t('admin.common.actions')" width="150">
            <template #default="{ row }: { row: AdminTag }">
              <el-button size="small" text data-testid="admin-tags-edit" @click="openEdit(row)">
                {{ t('admin.common.edit') }}
              </el-button>
              <el-button size="small" text data-testid="admin-tags-delete" @click="removeTag(row)">
                {{ t('admin.common.remove') }}
              </el-button>
            </template>
          </el-table-column>
        </el-table>

        <el-pagination
          class="pb-admin-module__pager"
          layout="prev, pager, next"
          :current-page="tagPage"
          :page-size="tagPageSize"
          :total="tagTotal"
          @current-change="changeTagPage"
        />
      </el-tab-pane>

      <el-tab-pane :label="t('admin.tags.recommendTitle')" name="recommend">
        <p class="pb-admin-module__hint">{{ t('admin.tags.recommendHint') }}</p>

        <div class="pb-admin-module__filters">
          <el-select
            v-model="recommendStates"
            class="pb-admin-module__filter"
            multiple
            :placeholder="t('admin.tags.stateFilter')"
            data-testid="admin-tags-recommend-states"
            @change="searchRecommendTags"
          >
            <el-option
              v-for="state in RECOMMEND_TAG_STATES"
              :key="state"
              :label="stateText(state)"
              :value="state"
            />
          </el-select>
          <el-select
            v-model="recommendKind"
            class="pb-admin-module__filter"
            :placeholder="t('admin.tags.kindFilter')"
            clearable
            data-testid="admin-tags-recommend-kind"
            @change="searchRecommendTags"
          >
            <el-option
              v-for="kind in RECOMMEND_TAG_KINDS"
              :key="kind"
              :label="kindText(kind)"
              :value="kind"
            />
          </el-select>
        </div>

        <div v-if="selectedIds.length > 0" class="pb-admin-module__filters">
          <span class="pb-admin-module__hint">
            {{ t('admin.tags.selected', { count: selectedIds.length }) }}
          </span>
          <el-button
            type="primary"
            size="small"
            data-testid="admin-tags-batch-confirm"
            @click="reviewTags(selectedIds, 'confirmed')"
          >
            {{ t('admin.tags.batchConfirm') }}
          </el-button>
          <el-button
            size="small"
            data-testid="admin-tags-batch-ignore"
            @click="reviewTags(selectedIds, 'ignored')"
          >
            {{ t('admin.tags.batchIgnore') }}
          </el-button>
        </div>

        <p
          v-if="!recommendLoading && recommendRows.length === 0"
          class="pb-admin-module__empty"
          data-testid="admin-tags-empty"
        >
          {{ t('admin.tags.empty') }}
        </p>

        <el-table
          v-if="recommendRows.length > 0"
          :data="recommendRows"
          size="small"
          data-testid="admin-tags-recommend-table"
          @selection-change="onSelectionChange"
        >
          <el-table-column type="selection" width="45" />
          <el-table-column :label="t('prompt.title')" min-width="180">
            <template #default="{ row }: { row: AdminRecommendTag }">
              <router-link :to="`/prompts/${row.prompt_id}`">{{ row.prompt_title }}</router-link>
            </template>
          </el-table-column>
          <el-table-column :label="t('prompt.model')" min-width="130">
            <template #default="{ row }: { row: AdminRecommendTag }">
              {{ row.model_name || t('admin.common.none') }}
            </template>
          </el-table-column>
          <el-table-column :label="t('admin.invites.kind')" width="120">
            <template #default="{ row }: { row: AdminRecommendTag }">{{ kindText(row.kind) }}</template>
          </el-table-column>
          <el-table-column :label="t('admin.reports.status')" width="110">
            <template #default="{ row }: { row: AdminRecommendTag }">
              {{ stateText(row.state) }}
            </template>
          </el-table-column>
          <el-table-column prop="label_zh" :label="t('admin.tags.nameZh')" width="140" />
          <el-table-column :label="t('admin.tags.confidence')" width="100">
            <template #default="{ row }: { row: AdminRecommendTag }">
              {{ confidenceText(row.confidence) }}
            </template>
          </el-table-column>
          <el-table-column :label="t('admin.tags.reason')" min-width="200">
            <template #default="{ row }: { row: AdminRecommendTag }">
              <el-tooltip :content="row.reason" placement="top">
                <span class="pb-admin-module__truncate">{{ row.reason }}</span>
              </el-tooltip>
            </template>
          </el-table-column>
          <el-table-column :label="t('admin.common.createdAt')" width="180">
            <template #default="{ row }: { row: AdminRecommendTag }">
              {{ formatTime(row.triggered_at) }}
            </template>
          </el-table-column>
          <el-table-column :label="t('admin.common.actions')" width="140">
            <template #default="{ row }: { row: AdminRecommendTag }">
              <el-button
                size="small"
                text
                data-testid="admin-tags-confirm"
                @click="reviewTags([row.id], 'confirmed')"
              >
                {{ t('admin.tags.confirm') }}
              </el-button>
              <el-button
                size="small"
                text
                data-testid="admin-tags-ignore"
                @click="reviewTags([row.id], 'ignored')"
              >
                {{ t('admin.tags.ignore') }}
              </el-button>
            </template>
          </el-table-column>
        </el-table>

        <el-pagination
          class="pb-admin-module__pager"
          layout="prev, pager, next"
          :current-page="recommendPage"
          :page-size="recommendPageSize"
          :total="recommendTotal"
          @current-change="changeRecommendPage"
        />
      </el-tab-pane>
    </el-tabs>

    <el-dialog
      :model-value="formVisible"
      :title="editingTag === null ? t('admin.tags.createTitle') : t('admin.tags.editTitle')"
      width="560px"
      @update:model-value="closeForm"
    >
      <p v-if="formFailure" class="pb-admin-module__error">{{ formFailure }}</p>
      <el-form label-width="100px">
        <el-form-item :label="t('admin.tags.slug')">
          <el-input v-model="form.slug" data-testid="admin-tags-form-slug" />
        </el-form-item>
        <el-form-item :label="t('admin.tags.nameZh')">
          <el-input v-model="form.name_zh" data-testid="admin-tags-form-name-zh" />
        </el-form-item>
        <el-form-item :label="t('admin.tags.nameEn')">
          <el-input v-model="form.name_en" />
        </el-form-item>
        <el-form-item :label="t('admin.tags.color')">
          <el-color-picker v-model="form.color" />
        </el-form-item>
        <el-form-item :label="t('admin.tags.kind')">
          <el-select v-model="form.kind">
            <el-option :label="t('admin.tags.kindTheme')" value="theme" />
            <el-option :label="t('admin.tags.kindForm')" value="form" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('admin.tags.description')">
          <el-input v-model="form.description" type="textarea" :rows="3" />
        </el-form-item>
        <el-form-item :label="t('admin.tags.priority')">
          <el-input-number v-model="form.priority" :min="-1000" :max="1000" />
        </el-form-item>
        <el-form-item :label="t('admin.common.enabled')">
          <el-switch v-model="form.is_enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="closeForm">{{ t('admin.common.cancel') }}</el-button>
        <el-button
          type="primary"
          :loading="saving"
          data-testid="admin-tags-form-submit"
          @click="submitForm"
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
  max-width: 220px;
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

.pb-admin-module__swatch {
  display: inline-block;
  height: 14px;
  width: 14px;
}

.pb-admin-module__truncate {
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
