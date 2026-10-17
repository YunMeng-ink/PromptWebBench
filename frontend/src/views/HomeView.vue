<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { type PromptListItem, promptApi, type SortOrder } from '@/api/prompt'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { useAuthStore } from '@/stores/auth'
import { useDictionaryStore } from '@/stores/dictionary'

const { t, locale } = useI18n()
const auth = useAuthStore()
const dictionary = useDictionaryStore()
const { messageOf } = useErrorMessage()

interface Filters {
  keyword: string
  categoryID: string
  abilityID: string
  difficultyID: string
  modelID: string
  tagIDs: string[]
  sort: SortOrder
}

function emptyFilters(): Filters {
  return {
    keyword: '',
    categoryID: '',
    abilityID: '',
    difficultyID: '',
    modelID: '',
    tagIDs: [],
    sort: 'latest',
  }
}

const filters = ref<Filters>(emptyFilters())
const page = ref(1)
const pageSize = 20

const items = ref<PromptListItem[]>([])
const total = ref(0)
const loading = ref(false)
const failed = ref(false)
const notice = ref('')

const sortOptions = computed(() => [
  { value: 'latest' as const, label: t('home.sortLatest') },
  { value: 'top_rated' as const, label: t('home.sortTopRated') },
  { value: 'discrimination' as const, label: t('home.sortDiscrimination') },
])

/** 显示名按当前语言在中文与英文之间切换，字典同时携带两种取值。 */
function localized(zh: string, en: string): string {
  return locale.value === 'en' ? en : zh
}

const tagOptions = computed(() =>
  dictionary.data.tags.map((tag) => ({
    value: String(tag.id),
    label: localized(tag.name_zh, tag.name_en),
  })),
)

/** anyFilterActive 判断是否有任意一维筛选生效，供「重置」按钮决定是否可用。 */
const anyFilterActive = computed(
  () =>
    filters.value.keyword !== '' ||
    filters.value.categoryID !== '' ||
    filters.value.abilityID !== '' ||
    filters.value.difficultyID !== '' ||
    filters.value.modelID !== '' ||
    filters.value.tagIDs.length > 0 ||
    filters.value.sort !== 'latest',
)

async function loadList(): Promise<void> {
  loading.value = true
  failed.value = false
  try {
    const result = await promptApi.list({
      page: page.value,
      page_size: pageSize,
      category_id: filters.value.categoryID,
      ability_id: filters.value.abilityID,
      difficulty_id: filters.value.difficultyID,
      keyword: filters.value.keyword,
      model_id: filters.value.modelID,
      tag_ids: filters.value.tagIDs,
      sort: filters.value.sort,
    })
    items.value = result.items
    total.value = result.total
  } catch (error) {
    failed.value = true
    notice.value = messageOf(error)
    items.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

/** applyFilters 从第一页重新查询。任何一维变化都回到第一页。 */
function applyFilters(): void {
  page.value = 1
  void loadList()
}

function resetFilters(): void {
  filters.value = emptyFilters()
  applyFilters()
}

function onPageChange(next: number): void {
  page.value = next
  void loadList()
}

/** 复制正文到剪贴板。剪贴板接口在非安全上下文不可用，此时提示用户手工复制。 */
async function copyBody(item: PromptListItem): Promise<void> {
  const detail = await promptApi.detail(item.id)
  try {
    await navigator.clipboard.writeText(detail.version.body)
    notice.value = t('home.copied')
  } catch {
    notice.value = t('home.copyFailed')
  }
}

onMounted(async () => {
  await Promise.all([dictionary.load(), auth.connect()])
  await loadList()
})

watch(locale, () => {
  void loadList()
})
</script>

<template>
  <section class="pb-home">
    <div class="pb-home__bar">
      <h1 class="pb-home__title">{{ t('home.title') }}</h1>
      <RouterLink v-if="auth.isAuthenticated" to="/upload" data-testid="home-upload">
        <el-button type="primary" size="small">{{ t('nav.upload') }}</el-button>
      </RouterLink>
    </div>

    <div class="pb-filters" data-testid="home-filters">
      <el-input
        v-model="filters.keyword"
        data-testid="filter-keyword"
        :placeholder="t('home.keywordPlaceholder')"
        clearable
        @change="applyFilters"
      />

      <el-select
        v-model="filters.categoryID"
        class="pb-filters__select"
        data-testid="filter-category"
        :aria-label="t('home.category')"
        :placeholder="t('home.category')"
        clearable
        @change="applyFilters"
      >
        <el-option
          v-for="item in dictionary.data.categories"
          :key="item.id"
          :label="localized(item.name_zh, item.name_en)"
          :value="String(item.id)"
        />
      </el-select>

      <el-select
        v-model="filters.abilityID"
        class="pb-filters__select"
        data-testid="filter-ability"
        :aria-label="t('home.ability')"
        :placeholder="t('home.ability')"
        clearable
        @change="applyFilters"
      >
        <el-option
          v-for="item in dictionary.data.abilities"
          :key="item.id"
          :label="localized(item.name_zh, item.name_en)"
          :value="String(item.id)"
        />
      </el-select>

      <el-select
        v-model="filters.difficultyID"
        class="pb-filters__select"
        data-testid="filter-difficulty"
        :aria-label="t('home.difficulty')"
        :placeholder="t('home.difficulty')"
        clearable
        @change="applyFilters"
      >
        <el-option
          v-for="item in dictionary.data.difficulties"
          :key="item.id"
          :label="localized(item.name_zh, item.name_en)"
          :value="String(item.id)"
        />
      </el-select>

      <el-select
        v-model="filters.modelID"
        class="pb-filters__select"
        data-testid="filter-model"
        :aria-label="t('home.model')"
        :placeholder="t('home.model')"
        clearable
        @change="applyFilters"
      >
        <el-option
          v-for="item in dictionary.data.models"
          :key="item.id"
          :label="item.name"
          :value="String(item.id)"
        />
      </el-select>

      <el-select
        v-model="filters.tagIDs"
        class="pb-filters__select"
        data-testid="filter-tags"
        multiple
        collapse-tags
        :aria-label="t('home.tags')"
        :placeholder="t('home.tags')"
        @change="applyFilters"
      >
        <el-option
          v-for="item in tagOptions"
          :key="item.value"
          :label="item.label"
          :value="item.value"
        />
      </el-select>

      <el-select
        v-model="filters.sort"
        class="pb-filters__select"
        data-testid="filter-sort"
        :aria-label="t('home.sort')"
        @change="applyFilters"
      >
        <el-option
          v-for="item in sortOptions"
          :key="item.value"
          :label="item.label"
          :value="item.value"
        />
      </el-select>

      <el-button
        data-testid="filter-reset"
        size="small"
        :disabled="!anyFilterActive"
        @click="resetFilters"
      >
        {{ t('home.reset') }}
      </el-button>
    </div>

    <p v-if="notice" class="pb-home__notice" data-testid="home-notice">{{ notice }}</p>

    <p v-if="failed" class="pb-home__notice" data-testid="home-failed">
      {{ t('home.loadFailed') }}
    </p>

    <p v-else-if="!loading && items.length === 0" class="pb-home__empty" data-testid="home-empty">
      {{ t('home.empty') }}
    </p>

    <ul v-else class="pb-list" data-testid="home-list">
      <li
        v-for="item in items"
        :key="item.id"
        class="pb-list__item"
        :data-testid="`prompt-card-${item.id}`"
      >
        <div class="pb-list__head">
          <RouterLink
            class="pb-list__title"
            :to="`/prompts/${item.id}`"
            :data-testid="`prompt-link-${item.id}`"
          >
            {{ item.title }}
          </RouterLink>
          <span class="pb-list__version" :data-testid="`prompt-version-${item.id}`">
            {{ item.version_no }}
          </span>
        </div>

        <p v-if="item.description" class="pb-list__description">{{ item.description }}</p>

        <div class="pb-list__meta">
          <span class="pb-list__tag">{{ localized(item.category_name_zh, item.category_name_en) }}</span>
          <span v-if="item.ability_name_zh && item.ability_name_en" class="pb-list__tag">
            {{ localized(item.ability_name_zh, item.ability_name_en) }}
          </span>
          <span v-if="item.difficulty_name_zh && item.difficulty_name_en" class="pb-list__tag">
            {{ localized(item.difficulty_name_zh, item.difficulty_name_en) }}
          </span>
          <span class="pb-list__tag">{{ t('home.comments') }} {{ item.comment_count }}</span>
          <span class="pb-list__tag">{{ t('home.favorites') }} {{ item.favorite_count }}</span>
          <span v-if="item.major_modified" class="pb-list__tag pb-list__tag--warn">
            {{ t('prompt.majorChange') }}
          </span>
        </div>

        <div class="pb-list__stats">
          <span :data-testid="`prompt-average-${item.id}`">
            {{ t('home.averageLabel') }} {{ item.average_adjusted.toFixed(2) }}
          </span>
          <span :data-testid="`prompt-discrimination-${item.id}`">
            <template v-if="item.data_sufficient">
              {{ t('home.discriminationLabel') }} {{ item.discrimination.toFixed(2) }}
            </template>
            <template v-else>{{ t('home.dataInsufficient') }}</template>
          </span>
          <el-button
            size="small"
            text
            :data-testid="`prompt-copy-${item.id}`"
            @click="copyBody(item)"
          >
            {{ t('home.copyBody') }}
          </el-button>
        </div>
      </li>
    </ul>

    <el-pagination
      v-if="total > pageSize"
      class="pb-home__pagination"
      data-testid="home-pagination"
      layout="prev, pager, next, total"
      :current-page="page"
      :page-size="pageSize"
      :total="total"
      @current-change="onPageChange"
    />
  </section>
</template>

<style scoped>
.pb-home {
  padding: var(--pb-space-lg) 0;
}

.pb-home__bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--pb-space-md);
  margin-bottom: var(--pb-space-md);
}

.pb-home__title {
  font-size: 24px;
  margin: 0;
}

.pb-filters {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: var(--pb-space-sm);
  align-items: center;
  margin-bottom: var(--pb-space-md);
}

.pb-filters__select {
  width: 100%;
}

.pb-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
}

.pb-list__item {
  border: 1px solid var(--pb-color-border);
  border-radius: var(--pb-radius-md);
  background-color: var(--pb-color-surface);
  padding: var(--pb-space-md);
}

.pb-list__head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--pb-space-sm);
}

.pb-list__title {
  font-size: 17px;
  font-weight: 600;
  color: var(--pb-color-text);
}

.pb-list__version {
  color: var(--pb-color-text-faint);
  font-size: 13px;
  flex-shrink: 0;
}

.pb-list__description {
  margin: var(--pb-space-xs) 0;
  color: var(--pb-color-text-soft);
}

.pb-list__meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-xs);
}

.pb-list__tag {
  background-color: var(--pb-color-surface-muted);
  border-radius: var(--pb-radius-sm);
  color: var(--pb-color-text-soft);
  font-size: 12px;
  padding: 2px 8px;
}

.pb-list__tag--warn {
  color: var(--el-color-warning);
}

.pb-list__stats {
  display: flex;
  align-items: center;
  gap: var(--pb-space-md);
  margin-top: var(--pb-space-xs);
  color: var(--pb-color-text-faint);
  font-size: 13px;
}

.pb-home__notice {
  color: var(--el-color-danger);
  margin: 0 0 var(--pb-space-sm);
  word-break: break-all;
}

.pb-home__empty {
  color: var(--pb-color-text-soft);
  margin: 0;
}

.pb-home__pagination {
  margin-top: var(--pb-space-md);
  justify-content: flex-end;
}
</style>