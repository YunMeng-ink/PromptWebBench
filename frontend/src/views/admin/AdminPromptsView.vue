// 提示词管理（需求 §17.2）。
// 列表按关键词、可见性、删除状态、上传者编号、考察目的与考察维度筛选；标签维护、版本历史、
// 评分、评论与推荐标签审核集中在详情抽屉内，删除与回收站操作放在列表操作列。

<script setup lang="ts">
import { ElMessage, ElMessageBox } from 'element-plus'
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  type AdminComment,
  type AdminPrompt,
  type AdminPromptDetail,
  type AdminRating,
  type AdminRecommendTag,
  adminApi,
} from '@/api/admin'
import type { PromptVersion } from '@/api/prompt'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { useDictionaryStore } from '@/stores/dictionary'
import { formatDateTime } from '@/utils/datetime'

const { t, locale } = useI18n()
const { messageOf } = useErrorMessage()
const dictionary = useDictionaryStore()

/** VISIBILITY_LABELS 把可见性英文键映射到 prompt.visibility* 文案键。 */
const VISIBILITY_LABELS: Record<string, string> = {
  public: 'prompt.visibilityPublic',
  private: 'prompt.visibilityPrivate',
  developer: 'prompt.visibilityDeveloper',
}

/** TAG_KIND_LABELS 把推荐标签类型英文键映射到 admin.tags.kind* 文案键。 */
const TAG_KIND_LABELS: Record<string, string> = {
  obsolete: 'admin.tags.kindObsolete',
  suspected_obsolete: 'admin.tags.kindSuspectedObsolete',
  saturation: 'admin.tags.kindSaturation',
  suspected_saturation: 'admin.tags.kindSuspectedSaturation',
}

/** TAG_STATE_LABELS 把推荐标签状态英文键映射到 admin.tags.state* 文案键。 */
const TAG_STATE_LABELS: Record<string, string> = {
  pending: 'admin.tags.statePending',
  review: 'admin.tags.stateReview',
  confirmed: 'admin.tags.stateConfirmed',
  ignored: 'admin.tags.stateIgnored',
  expired: 'admin.tags.stateExpired',
}

/** TAG_CONFIDENCE_LABELS 把置信度英文键映射到 admin.tags.confidence* 文案键。 */
const TAG_CONFIDENCE_LABELS: Record<string, string> = {
  high: 'admin.tags.confidenceHigh',
  medium: 'admin.tags.confidenceMedium',
  low: 'admin.tags.confidenceLow',
}

const rows = ref<AdminPrompt[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(false)
const failure = ref('')

const filters = ref({
  keyword: '',
  visibility: '',
  deleted: '',
  uploader: '',
  purpose: '',
  capability: '',
})

const detailVisible = ref(false)
const detail = ref<AdminPromptDetail | null>(null)
const activeTab = ref('versions')
const majorNote = ref('')
const selectedTagId = ref<number | null>(null)

const ratings = ref<AdminRating[]>([])
const ratingsTotal = ref(0)
const ratingsPage = ref(1)
const comments = ref<AdminComment[]>([])
const commentsTotal = ref(0)
const commentsPage = ref(1)
const recommends = ref<AdminRecommendTag[]>([])

/** labelOf 按映射表取文案；未登记的取值原样呈现。 */
function labelOf(table: Record<string, string>, value: string): string {
  const key = table[value]
  return key === undefined ? value : t(key)
}

/** localizedName 按当前语言在中文名与英文名之间选择。 */
function localizedName(nameZh: string, nameEn: string): string {
  return locale.value.toLowerCase().startsWith('zh') ? nameZh : nameEn
}

/** formatTime 把 UTC 时刻按本机时区呈现。 */
function formatTime(raw: string): string {
  if (raw === '') {
    return ''
  }
  return formatDateTime(raw, locale.value)
}

/** availableTags 是尚未挂到当前提示词上的标签，避免重复挂载。 */
const availableTags = computed(() => {
  const attached = new Set((detail.value?.tags ?? []).map((tag) => tag.id))
  return dictionary.data.tags.filter((tag) => !attached.has(tag.id))
})

async function loadPrompts(): Promise<void> {
  loading.value = true
  try {
    const result = await adminApi.prompts({
      page: page.value,
      page_size: pageSize,
      keyword: filters.value.keyword,
      visibility: filters.value.visibility,
      deleted: filters.value.deleted,
      purpose_id: filters.value.purpose,
      capability_id: filters.value.capability,
      uploader_id: filters.value.uploader,
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
  void loadPrompts()
}

function resetFilters(): void {
  filters.value = {
    keyword: '',
    visibility: '',
    deleted: '',
    uploader: '',
    purpose: '',
    capability: '',
  }
  search()
}

function changePage(next: number): void {
  page.value = next
  void loadPrompts()
}

async function loadDetail(id: number): Promise<void> {
  try {
    detail.value = await adminApi.prompt(id)
    failure.value = ''
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function loadRatings(id: number): Promise<void> {
  try {
    const result = await adminApi.promptRatings(id, ratingsPage.value, pageSize)
    ratings.value = result.items
    ratingsTotal.value = result.total
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function loadComments(id: number): Promise<void> {
  try {
    const result = await adminApi.promptComments(id, commentsPage.value, pageSize)
    comments.value = result.items
    commentsTotal.value = result.total
  } catch (error) {
    failure.value = messageOf(error)
  }
}

/** loadRecommends 取全局推荐标签并按当前提示词过滤；接口不提供按提示词筛选。 */
async function loadRecommends(id: number): Promise<void> {
  try {
    const result = await adminApi.recommendTags([], '', 1, 50)
    recommends.value = result.items.filter((item) => item.prompt_id === id)
  } catch (error) {
    failure.value = messageOf(error)
  }
}

/** refreshDetail 重新拉取详情与三个页签；id 省略时沿用当前详情。 */
async function refreshDetail(id?: number): Promise<void> {
  const target = id ?? detail.value?.prompt.id
  if (target === undefined) {
    return
  }
  await loadDetail(target)
  await loadRatings(target)
  await loadComments(target)
  await loadRecommends(target)
}

function openDetail(row: AdminPrompt): void {
  detailVisible.value = true
  activeTab.value = 'versions'
  majorNote.value = ''
  selectedTagId.value = null
  ratingsPage.value = 1
  commentsPage.value = 1
  void refreshDetail(row.id)
}

function changeRatingsPage(next: number): void {
  ratingsPage.value = next
  const target = detail.value?.prompt.id
  if (target !== undefined) {
    void loadRatings(target)
  }
}

function changeCommentsPage(next: number): void {
  commentsPage.value = next
  const target = detail.value?.prompt.id
  if (target !== undefined) {
    void loadComments(target)
  }
}

async function attachTag(): Promise<void> {
  const target = detail.value?.prompt.id
  const tagId = selectedTagId.value
  if (target === undefined || tagId === null) {
    return
  }
  try {
    await adminApi.attachPromptTag(target, tagId)
    ElMessage.success(t('admin.prompts.tagAdded'))
    selectedTagId.value = null
    await refreshDetail(target)
  } catch (error) {
    ElMessage.error(messageOf(error))
  }
}

async function detachTag(tagId: number): Promise<void> {
  const target = detail.value?.prompt.id
  if (target === undefined) {
    return
  }
  try {
    await adminApi.detachPromptTag(target, tagId)
    ElMessage.success(t('admin.prompts.tagRemoved'))
    await refreshDetail(target)
  } catch (error) {
    ElMessage.error(messageOf(error))
  }
}

async function reviewRecommend(
  item: AdminRecommendTag,
  state: 'confirmed' | 'ignored',
): Promise<void> {
  const target = detail.value?.prompt.id
  if (target === undefined) {
    return
  }
  try {
    await adminApi.reviewRecommendTags([item.id], state)
    ElMessage.success(t('admin.tags.reviewDone'))
    await loadRecommends(target)
  } catch (error) {
    ElMessage.error(messageOf(error))
  }
}

/** confirmAction 弹出二次确认；用户取消时返回 false。 */
async function confirmAction(message: string): Promise<boolean> {
  return ElMessageBox.confirm(message, {
    confirmButtonText: t('admin.common.confirm'),
    cancelButtonText: t('admin.common.cancel'),
    type: 'warning',
  })
    .then(() => true)
    .catch(() => false)
}

async function markMajor(): Promise<void> {
  const target = detail.value?.prompt.id
  if (target === undefined) {
    return
  }
  if (!(await confirmAction(t('admin.prompts.majorMarkHint')))) {
    return
  }
  try {
    const result = await adminApi.markPromptMajor(target, majorNote.value)
    ElMessage.success(
      t('admin.prompts.majorMarkDone', {
        version: result.version_no,
        count: result.archived_ratings,
      }),
    )
    majorNote.value = ''
    await refreshDetail(target)
    await loadPrompts()
  } catch (error) {
    ElMessage.error(messageOf(error))
  }
}

async function deletePrompt(row: AdminPrompt): Promise<void> {
  if (!(await confirmAction(t('admin.common.confirmDelete')))) {
    return
  }
  try {
    await adminApi.deletePrompt(row.id)
    ElMessage.success(t('admin.prompts.deleteDone'))
    ElMessage.warning(t('admin.prompts.deleteHint'))
    if (detail.value?.prompt.id === row.id) {
      detailVisible.value = false
    }
    await loadPrompts()
  } catch (error) {
    ElMessage.error(messageOf(error))
  }
}

async function restorePrompt(row: AdminPrompt): Promise<void> {
  try {
    await adminApi.restorePrompt(row.id)
    ElMessage.success(t('admin.prompts.restoreDone'))
    await loadPrompts()
    if (detailVisible.value) {
      await refreshDetail(row.id)
    }
  } catch (error) {
    ElMessage.error(messageOf(error))
  }
}

/** purgePrompt 彻底删除；提示词仍被题库引用时后端返回 422/409，只在消息中提示。 */
async function purgePrompt(row: AdminPrompt): Promise<void> {
  if (!(await confirmAction(t('admin.common.confirmPurge')))) {
    return
  }
  try {
    await adminApi.purgePrompt(row.id)
    ElMessage.success(t('admin.prompts.purgeDone'))
    if (detail.value?.prompt.id === row.id) {
      detailVisible.value = false
    }
    await loadPrompts()
  } catch (error) {
    ElMessage.error(messageOf(error))
  }
}

onMounted(async () => {
  await dictionary.load()
  await loadPrompts()
})
</script>

<template>
  <section class="pb-admin-module">
    <header class="pb-admin-module__head">
      <h2 class="pb-admin-module__title">{{ t('admin.prompts.title') }}</h2>
    </header>

    <p v-if="failure" class="pb-admin-module__error" data-testid="admin-prompts-error">
      {{ failure }}
    </p>

    <div class="pb-admin-module__filters">
      <el-input
        v-model="filters.keyword"
        class="pb-admin-module__filter"
        :placeholder="t('admin.prompts.keyword')"
        clearable
        data-testid="admin-prompts-keyword"
        @keyup.enter="search"
      />
      <el-select
        v-model="filters.visibility"
        class="pb-admin-module__filter"
        :placeholder="t('admin.prompts.visibility')"
        clearable
        data-testid="admin-prompts-visibility"
        @change="search"
      >
        <el-option :label="t('admin.common.all')" value="" />
        <el-option :label="t('prompt.visibilityPublic')" value="public" />
        <el-option :label="t('prompt.visibilityPrivate')" value="private" />
        <el-option :label="t('prompt.visibilityDeveloper')" value="developer" />
      </el-select>
      <el-select
        v-model="filters.deleted"
        class="pb-admin-module__filter"
        :placeholder="t('admin.prompts.deletedState')"
        clearable
        data-testid="admin-prompts-deleted"
        @change="search"
      >
        <el-option :label="t('admin.common.all')" value="" />
        <el-option :label="t('admin.prompts.inLibrary')" value="false" />
        <el-option :label="t('admin.prompts.inRecycle')" value="true" />
      </el-select>
      <el-input
        v-model="filters.uploader"
        class="pb-admin-module__filter"
        :placeholder="t('admin.prompts.uploader')"
        clearable
        data-testid="admin-prompts-uploader"
        @keyup.enter="search"
      />
      <el-select
        v-model="filters.purpose"
        class="pb-admin-module__filter"
        :placeholder="t('admin.prompts.purpose')"
        clearable
        data-testid="admin-prompts-purpose"
        @change="search"
      >
        <el-option
          v-for="item in dictionary.data.purposes"
          :key="item.id"
          :label="localizedName(item.name_zh, item.name_en)"
          :value="String(item.id)"
        />
      </el-select>
      <el-select
        v-model="filters.capability"
        class="pb-admin-module__filter"
        :placeholder="t('admin.prompts.capability')"
        clearable
        data-testid="admin-prompts-capability"
        @change="search"
      >
        <el-option
          v-for="item in dictionary.data.capabilities"
          :key="item.id"
          :label="localizedName(item.name_zh, item.name_en)"
          :value="String(item.id)"
        />
      </el-select>
      <el-button type="primary" size="small" data-testid="admin-prompts-search" @click="search">
        {{ t('admin.common.search') }}
      </el-button>
      <el-button size="small" data-testid="admin-prompts-reset" @click="resetFilters">
        {{ t('admin.common.reset') }}
      </el-button>
    </div>

    <p class="pb-admin-module__hint">{{ t('admin.prompts.lookupHint') }}</p>

    <p v-if="!loading && rows.length === 0" class="pb-admin-module__empty" data-testid="admin-prompts-empty">
      {{ t('admin.common.empty') }}
    </p>

    <el-table
      v-if="rows.length > 0"
      :data="rows"
      size="small"
      data-testid="admin-prompts-table"
      @row-click="(row: AdminPrompt) => openDetail(row)"
    >
      <el-table-column prop="id" :label="t('admin.common.id')" width="70" />
      <el-table-column prop="title" :label="t('admin.prompts.title')" min-width="180" />
      <el-table-column :label="t('admin.prompts.visibility')" width="110">
        <template #default="{ row }: { row: AdminPrompt }">
          {{ labelOf(VISIBILITY_LABELS, row.visibility) }}
        </template>
      </el-table-column>
      <el-table-column :label="t('admin.prompts.purpose')" width="130">
        <template #default="{ row }: { row: AdminPrompt }">
          {{ localizedName(row.purpose_name_zh, row.purpose_name_en) }}
        </template>
      </el-table-column>
      <el-table-column :label="t('admin.prompts.capability')" width="120">
        <template #default="{ row }: { row: AdminPrompt }">
          {{ localizedName(row.capability_name_zh, row.capability_name_en) }}
        </template>
      </el-table-column>
      <el-table-column prop="uploader_name" :label="t('admin.prompts.uploader')" width="120" />
      <el-table-column prop="version_no" :label="t('admin.prompts.version')" width="100" />
      <el-table-column :label="t('admin.prompts.deleted')" width="90">
        <template #default="{ row }: { row: AdminPrompt }">
          <el-tag v-if="row.deleted" size="small" type="danger">
            {{ t('admin.prompts.deleted') }}
          </el-tag>
          <span v-else>{{ t('admin.common.none') }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="comment_count" :label="t('admin.prompts.comments')" width="90" />
      <el-table-column prop="favorite_count" :label="t('admin.prompts.favorites')" width="90" />
      <el-table-column :label="t('admin.common.updatedAt')" width="170">
        <template #default="{ row }: { row: AdminPrompt }">{{ formatTime(row.updated_at) }}</template>
      </el-table-column>
      <el-table-column :label="t('admin.common.actions')" width="250">
        <template #default="{ row }: { row: AdminPrompt }">
          <el-button
            size="small"
            text
            data-testid="admin-prompts-detail"
            @click.stop="openDetail(row)"
          >
            {{ t('admin.common.detail') }}
          </el-button>
          <el-button
            size="small"
            text
            data-testid="admin-prompts-delete"
            @click.stop="deletePrompt(row)"
          >
            {{ t('admin.prompts.deletePrompt') }}
          </el-button>
          <el-button
            v-if="row.deleted"
            size="small"
            text
            data-testid="admin-prompts-restore"
            @click.stop="restorePrompt(row)"
          >
            {{ t('admin.prompts.restorePrompt') }}
          </el-button>
          <el-button
            v-if="row.deleted"
            size="small"
            text
            type="danger"
            data-testid="admin-prompts-purge"
            @click.stop="purgePrompt(row)"
          >
            {{ t('admin.prompts.purgePrompt') }}
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

    <el-drawer
      v-model="detailVisible"
      size="720px"
      :title="t('admin.prompts.detailTitle', { id: detail?.prompt.id ?? 0 })"
    >
      <div v-if="detail" class="pb-admin-module__detail">
        <h3 class="pb-admin-module__detail-title" data-testid="admin-prompts-detail-title">
          {{ detail.prompt.title }}
        </h3>
        <div class="pb-admin-module__tags">
          <el-tag size="small" type="info">
            {{ labelOf(VISIBILITY_LABELS, detail.prompt.visibility) }}
          </el-tag>
          <el-tag v-if="detail.prompt.major_modified" size="small" type="warning">
            {{ t('admin.prompts.majorMarked') }}
          </el-tag>
          <el-tag v-if="detail.prompt.is_high_risk" size="small" type="danger">
            {{ t('prompt.sensitiveHigh') }}
          </el-tag>
        </div>

        <el-descriptions :column="1" border size="small">
          <el-descriptions-item :label="t('admin.prompts.body')">
            <pre class="pb-admin-module__body">{{ detail.body }}</pre>
          </el-descriptions-item>
          <el-descriptions-item :label="t('admin.prompts.purpose')">
            {{ localizedName(detail.prompt.purpose_name_zh, detail.prompt.purpose_name_en) }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('admin.prompts.capability')">
            {{ localizedName(detail.prompt.capability_name_zh, detail.prompt.capability_name_en) }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('admin.prompts.uploader')">
            {{ detail.prompt.uploader_name || t('admin.common.none') }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('admin.prompts.version')">
            {{ detail.prompt.version_no }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('admin.prompts.visibility')">
            {{ labelOf(VISIBILITY_LABELS, detail.prompt.visibility) }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('admin.prompts.majorMarked')">
            <el-tag v-if="detail.prompt.major_modified" size="small" type="warning">
              {{ t('admin.prompts.majorMarked') }}
            </el-tag>
            <span v-else>{{ t('admin.common.none') }}</span>
          </el-descriptions-item>
          <el-descriptions-item :label="t('admin.prompts.comments')">
            {{ detail.prompt.comment_count }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('admin.prompts.favorites')">
            {{ detail.prompt.favorite_count }}
          </el-descriptions-item>
          <el-descriptions-item :label="t('admin.prompts.tags')">
            <div class="pb-admin-module__tags">
              <el-tag
                v-for="tag in detail.tags"
                :key="tag.id"
                class="pb-admin-module__tag"
                :title="t('admin.prompts.removeTag')"
                closable
                data-testid="admin-prompts-tag-remove"
                @close="detachTag(tag.id)"
              >
                {{ localizedName(tag.name_zh, tag.name_en) }}
              </el-tag>
              <span v-if="detail.tags.length === 0" class="pb-admin-module__hint">
                {{ t('admin.prompts.noTags') }}
              </span>
            </div>
          </el-descriptions-item>
        </el-descriptions>

        <div class="pb-admin-module__tag-add">
          <el-select
            v-model="selectedTagId"
            class="pb-admin-module__filter"
            :placeholder="t('admin.prompts.addTag')"
            clearable
            data-testid="admin-prompts-tag-add"
          >
            <el-option
              v-for="tag in availableTags"
              :key="tag.id"
              :label="localizedName(tag.name_zh, tag.name_en)"
              :value="tag.id"
            />
          </el-select>
          <el-button size="small" type="primary" @click="attachTag">
            {{ t('admin.common.confirm') }}
          </el-button>
        </div>

        <section class="pb-admin-module__section">
          <p class="pb-admin-module__hint">{{ t('admin.prompts.majorMarkHint') }}</p>
          <div class="pb-admin-module__tag-add">
            <el-input
              v-model="majorNote"
              class="pb-admin-module__filter"
              :placeholder="t('admin.prompts.majorMarkNote')"
              data-testid="admin-prompts-major-note"
            />
            <el-button
              size="small"
              type="warning"
              data-testid="admin-prompts-major-mark"
              @click="markMajor"
            >
              {{ t('admin.prompts.majorMark') }}
            </el-button>
          </div>
        </section>

        <el-tabs v-model="activeTab">
          <el-tab-pane :label="t('admin.prompts.tabVersions')" name="versions">
            <p v-if="detail.versions.length === 0" class="pb-admin-module__empty">
              {{ t('admin.prompts.versionsEmpty') }}
            </p>
            <el-table v-else :data="detail.versions" size="small">
              <el-table-column prop="version_no" :label="t('admin.prompts.version')" width="100" />
              <el-table-column :label="t('admin.prompts.isMajor')" width="110">
                <template #default="{ row }: { row: PromptVersion }">
                  <el-tag v-if="row.is_major" size="small" type="warning">
                    {{ t('admin.prompts.isMajor') }}
                  </el-tag>
                  <span v-else>{{ t('admin.common.none') }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="change_note" :label="t('admin.prompts.changeNote')" min-width="160" />
              <el-table-column :label="t('admin.common.createdAt')" width="170">
                <template #default="{ row }: { row: PromptVersion }">
                  {{ formatTime(row.created_at) }}
                </template>
              </el-table-column>
            </el-table>
          </el-tab-pane>

          <el-tab-pane :label="t('admin.prompts.tabRatings')" name="ratings">
            <p v-if="ratings.length === 0" class="pb-admin-module__empty">
              {{ t('admin.prompts.ratingsEmpty') }}
            </p>
            <el-table v-else :data="ratings" size="small">
              <el-table-column prop="id" :label="t('admin.common.id')" width="70" />
              <el-table-column prop="user_name" :label="t('admin.prompts.uploader')" width="120" />
              <el-table-column prop="model_name" :label="t('admin.prompts.model')" width="150" />
              <el-table-column prop="score" :label="t('admin.prompts.score')" width="90" />
              <el-table-column prop="edit_count" :label="t('admin.common.edit')" width="90" />
              <el-table-column :label="t('admin.common.createdAt')" width="170">
                <template #default="{ row }: { row: AdminRating }">
                  {{ formatTime(row.created_at) }}
                </template>
              </el-table-column>
              <el-table-column :label="t('admin.prompts.deleted')" width="100">
                <template #default="{ row }: { row: AdminRating }">
                  <el-tag v-if="row.deleted" size="small" type="danger">
                    {{ t('admin.prompts.ratingDeleted') }}
                  </el-tag>
                  <span v-else>{{ t('admin.common.none') }}</span>
                </template>
              </el-table-column>
            </el-table>
            <el-pagination
              v-if="ratingsTotal > pageSize"
              class="pb-admin-module__pager"
              layout="prev, pager, next"
              :current-page="ratingsPage"
              :page-size="pageSize"
              :total="ratingsTotal"
              @current-change="changeRatingsPage"
            />
          </el-tab-pane>

          <el-tab-pane :label="t('admin.prompts.tabComments')" name="comments">
            <p v-if="comments.length === 0" class="pb-admin-module__empty">
              {{ t('admin.prompts.commentsEmpty') }}
            </p>
            <el-table v-else :data="comments" size="small">
              <el-table-column prop="id" :label="t('admin.common.id')" width="70" />
              <el-table-column prop="author_name" :label="t('admin.prompts.uploader')" width="120" />
              <el-table-column prop="body" :label="t('admin.prompts.body')" min-width="200" />
              <el-table-column prop="like_count" :label="t('admin.prompts.favorites')" width="90" />
              <el-table-column :label="t('admin.common.createdAt')" width="170">
                <template #default="{ row }: { row: AdminComment }">
                  {{ formatTime(row.created_at) }}
                </template>
              </el-table-column>
              <el-table-column :label="t('admin.prompts.deleted')" width="100">
                <template #default="{ row }: { row: AdminComment }">
                  <el-tag v-if="row.deleted" size="small" type="danger">
                    {{ t('admin.prompts.deleted') }}
                  </el-tag>
                  <span v-else>{{ t('admin.common.none') }}</span>
                </template>
              </el-table-column>
            </el-table>
            <el-pagination
              v-if="commentsTotal > pageSize"
              class="pb-admin-module__pager"
              layout="prev, pager, next"
              :current-page="commentsPage"
              :page-size="pageSize"
              :total="commentsTotal"
              @current-change="changeCommentsPage"
            />
          </el-tab-pane>

          <el-tab-pane :label="t('admin.prompts.tabRecommend')" name="recommend">
            <p v-if="recommends.length === 0" class="pb-admin-module__empty">
              {{ t('admin.common.empty') }}
            </p>
            <el-table v-else :data="recommends" size="small">
              <el-table-column prop="label_zh" :label="t('admin.prompts.tags')" width="140" />
              <el-table-column :label="t('admin.prompts.tagKindState')" width="160">
                <template #default="{ row }: { row: AdminRecommendTag }">
                  {{ labelOf(TAG_KIND_LABELS, row.kind) }} / {{ labelOf(TAG_STATE_LABELS, row.state) }}
                </template>
              </el-table-column>
              <el-table-column :label="t('admin.tags.confidence')" width="100">
                <template #default="{ row }: { row: AdminRecommendTag }">
                  {{ labelOf(TAG_CONFIDENCE_LABELS, row.confidence) }}
                </template>
              </el-table-column>
              <el-table-column prop="reason" :label="t('admin.tags.reason')" min-width="160" />
              <el-table-column :label="t('admin.common.actions')" width="150">
                <template #default="{ row }: { row: AdminRecommendTag }">
                  <el-button
                    size="small"
                    text
                    type="primary"
                    @click="reviewRecommend(row, 'confirmed')"
                  >
                    {{ t('admin.tags.confirm') }}
                  </el-button>
                  <el-button
                    size="small"
                    text
                    type="danger"
                    @click="reviewRecommend(row, 'ignored')"
                  >
                    {{ t('admin.tags.ignore') }}
                  </el-button>
                </template>
              </el-table-column>
            </el-table>
          </el-tab-pane>
        </el-tabs>
      </div>

      <template #footer>
        <el-button data-testid="admin-prompts-detail-close" @click="detailVisible = false">
          {{ t('admin.common.close') }}
        </el-button>
      </template>
    </el-drawer>
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

.pb-admin-module__detail-title {
  font-size: 16px;
  margin: 0;
}

.pb-admin-module__tags {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-xs);
  margin: var(--pb-space-xs) 0;
}

.pb-admin-module__tag {
  max-width: 160px;
}

.pb-admin-module__tag-add {
  display: flex;
  gap: var(--pb-space-xs);
  margin: var(--pb-space-xs) 0;
}

.pb-admin-module__section {
  margin: var(--pb-space-md) 0;
}

.pb-admin-module__body {
  font-size: 12px;
  margin: 0;
  max-height: 240px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
