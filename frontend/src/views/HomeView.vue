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
  purposeID: string
  capabilityID: string
  difficultyID: string
  modelID: string
  tagIDs: string[]
  sort: SortOrder
}

function emptyFilters(): Filters {
  return {
    keyword: '',
    purposeID: '',
    capabilityID: '',
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
    kind: tag.kind,
  })),
)

const tagGroups = computed(() =>
  [
    {
      label: t('home.themeTags'),
      options: tagOptions.value.filter((tag) => tag.kind === 'theme'),
    },
    {
      label: t('home.formTags'),
      options: tagOptions.value.filter((tag) => tag.kind === 'form'),
    },
  ].filter((group) => group.options.length > 0),
)

/** anyFilterActive 判断是否有任意一维筛选生效，供「重置」按钮决定是否可用。 */
const anyFilterActive = computed(
  () =>
    filters.value.keyword !== '' ||
    filters.value.purposeID !== '' ||
    filters.value.capabilityID !== '' ||
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
      purpose_id: filters.value.purposeID,
      capability_id: filters.value.capabilityID,
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
  <section class="pb-home pb-page">
    <header class="pb-home__head">
      <div class="pb-home__heading">
        <span class="pb-eyebrow" aria-hidden="true">Benchmark Library</span>
        <h1 class="pb-page__title">{{ t('home.title') }}</h1>
        <p class="pb-home__lead">{{ t('app.tagline') }}</p>
      </div>
      <div class="pb-page__actions">
        <RouterLink v-if="auth.isAuthenticated" to="/upload" data-testid="home-upload">
          <el-button type="primary">{{ t('nav.upload') }}</el-button>
        </RouterLink>
      </div>
    </header>

    <div class="pb-filters" data-testid="home-filters">
      <el-input
        v-model="filters.keyword"
        class="pb-filters__keyword"
        data-testid="filter-keyword"
        :placeholder="t('home.keywordPlaceholder')"
        clearable
        @change="applyFilters"
      >
        <template #prefix>
          <svg
            viewBox="0 0 24 24"
            width="15"
            height="15"
            fill="none"
            stroke="currentColor"
            stroke-width="1.7"
            stroke-linecap="round"
            aria-hidden="true"
          >
            <circle cx="11" cy="11" r="6.5" />
            <path d="m16 16 4 4" />
          </svg>
        </template>
      </el-input>

      <el-select
        v-model="filters.purposeID"
        class="pb-filters__select"
        data-testid="filter-purpose"
        :aria-label="t('home.purpose')"
        :placeholder="t('home.purpose')"
        clearable
        @change="applyFilters"
      >
        <el-option
          v-for="item in dictionary.data.purposes"
          :key="item.id"
          :label="localized(item.name_zh, item.name_en)"
          :value="String(item.id)"
        />
      </el-select>

      <el-select
        v-model="filters.capabilityID"
        class="pb-filters__select"
        data-testid="filter-capability"
        :aria-label="t('home.capability')"
        :placeholder="t('home.capability')"
        clearable
        @change="applyFilters"
      >
        <el-option
          v-for="item in dictionary.data.capabilities"
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
        <el-option-group
          v-for="group in tagGroups"
          :key="group.label"
          :label="group.label"
        >
          <el-option
            v-for="item in group.options"
            :key="item.value"
            :label="item.label"
            :value="item.value"
          />
        </el-option-group>
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
        class="pb-filters__reset"
        data-testid="filter-reset"
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

    <div
      v-else-if="!loading && items.length === 0"
      class="pb-empty"
      data-testid="home-empty"
    >
      <svg
        viewBox="0 0 48 48"
        width="40"
        height="40"
        fill="none"
        stroke="currentColor"
        stroke-width="1.3"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        <rect x="9" y="7" width="30" height="34" rx="3" />
        <path d="M16 16h16M16 23h16M16 30h10" />
      </svg>
      <span class="pb-empty__title">{{ t('home.empty') }}</span>
    </div>

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
          <span class="pb-list__tag pb-list__tag--purpose">
            {{ t('dimension.purpose') }} · {{ localized(item.purpose_name_zh, item.purpose_name_en) }}
          </span>
          <span class="pb-list__tag pb-list__tag--capability">
            {{ t('dimension.capability') }} · {{ localized(item.capability_name_zh, item.capability_name_en) }}
          </span>
          <span v-if="item.difficulty_name_zh && item.difficulty_name_en" class="pb-list__tag">
            {{ t('dimension.difficulty') }} ·
            {{ localized(item.difficulty_name_zh, item.difficulty_name_en) }}
          </span>
          <span class="pb-list__tag">{{ t('home.comments') }} {{ item.comment_count }}</span>
          <span class="pb-list__tag">{{ t('home.favorites') }} {{ item.favorite_count }}</span>
          <span v-if="item.major_modified" class="pb-list__tag pb-list__tag--warn">
            {{ t('prompt.majorChange') }}
          </span>
        </div>

        <div class="pb-list__stats">
          <span class="pb-list__stat" :data-testid="`prompt-average-${item.id}`">
            <span class="pb-list__stat-label">{{ t('home.averageLabel') }}</span>
            <span class="pb-list__stat-value">{{ item.average_adjusted.toFixed(2) }}</span>
          </span>
          <span class="pb-list__stat" :data-testid="`prompt-discrimination-${item.id}`">
            <span class="pb-list__stat-label">{{ t('home.discriminationLabel') }}</span>
            <span class="pb-list__stat-value">
              <template v-if="item.data_sufficient">{{ item.discrimination.toFixed(2) }}</template>
              <template v-else>{{ t('home.dataInsufficient') }}</template>
            </span>
          </span>
          <el-button
            class="pb-list__copy"
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
  padding-top: var(--pb-space-xl);
}

.pb-home__head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--pb-space-xl);
  flex-wrap: wrap;
  padding-bottom: var(--pb-space-lg);
  border-bottom: 1px solid var(--pb-line);
}

.pb-home__heading {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-xs);
  min-width: 0;
}

.pb-home__lead {
  color: var(--pb-ink-soft);
  font-size: 15px;
  margin: var(--pb-space-xs) 0 0;
  letter-spacing: 0.02em;
}

.pb-filters {
  display: grid;
  grid-template-columns: minmax(220px, 2fr) repeat(5, minmax(120px, 1fr)) minmax(120px, 1fr) auto;
  gap: var(--pb-space-sm);
  align-items: center;
  padding: var(--pb-space-md);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
  box-shadow: var(--pb-shadow-sm);
}

.pb-filters__keyword,
.pb-filters__select {
  width: 100%;
}

.pb-filters__reset {
  white-space: nowrap;
}

.pb-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-md);
}

.pb-list__item {
  position: relative;
  padding: var(--pb-space-lg) var(--pb-space-xl);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
  box-shadow: var(--pb-shadow-sm);
  transition:
    transform var(--pb-duration) var(--pb-ease),
    box-shadow var(--pb-duration) var(--pb-ease),
    border-color var(--pb-duration) var(--pb-ease);
}

.pb-list__item::before {
  content: "";
  position: absolute;
  left: 0;
  top: var(--pb-space-lg);
  bottom: var(--pb-space-lg);
  width: 2px;
  border-radius: 2px;
  background: var(--pb-accent);
  opacity: 0;
  transition: opacity var(--pb-duration) var(--pb-ease);
}

.pb-list__item:hover {
  transform: translateY(-2px);
  border-color: var(--pb-line-strong);
  box-shadow: var(--pb-shadow-md);
}

.pb-list__item:hover::before {
  opacity: 1;
}

.pb-list__head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--pb-space-md);
}

.pb-list__title {
  color: var(--pb-ink);
  font-size: 19px;
  font-weight: 600;
  letter-spacing: 0.006em;
  line-height: 1.35;
}

.pb-list__title:hover {
  color: var(--pb-accent);
}

.pb-list__version {
  flex-shrink: 0;
  padding: 1px 8px;
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-pill);
  color: var(--pb-ink-faint);
  font-family: var(--pb-font-mono);
  font-size: 11px;
  letter-spacing: 0.02em;
}

.pb-list__description {
  margin: var(--pb-space-sm) 0 var(--pb-space-md);
  color: var(--pb-ink-soft);
  font-size: 14px;
  line-height: 1.7;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.pb-list__meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-xs);
}

.pb-list__tag {
  padding: 2px 9px;
  border-radius: var(--pb-radius-pill);
  background: var(--pb-surface-muted);
  color: var(--pb-ink-soft);
  font-size: 12px;
  line-height: 1.7;
  white-space: nowrap;
}

.pb-list__tag--warn {
  background: var(--pb-accent-soft);
  color: var(--pb-accent-strong);
}

.pb-list__tag--purpose {
  border: 1px solid var(--pb-line);
  background: var(--pb-surface);
}

.pb-list__tag--capability {
  background: var(--pb-accent-soft);
  color: var(--pb-accent-strong);
}

.pb-list__stats {
  display: flex;
  align-items: center;
  gap: var(--pb-space-xl);
  margin-top: var(--pb-space-md);
  padding-top: var(--pb-space-md);
  border-top: 1px solid var(--pb-line);
}

.pb-list__stat {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  font-size: 13px;
}

.pb-list__stat-label {
  color: var(--pb-ink-faint);
  letter-spacing: 0.04em;
}

.pb-list__stat-value {
  color: var(--pb-ink);
  font-family: var(--pb-font-mono);
  font-size: 13px;
  font-variant-numeric: tabular-nums;
}

.pb-list__copy {
  margin-left: auto;
  color: var(--pb-accent);
}

.pb-home__notice {
  margin: 0;
  padding: var(--pb-space-sm) var(--pb-space-md);
  border-radius: var(--pb-radius-md);
  background: var(--pb-accent-soft);
  color: var(--pb-accent-strong);
  font-size: 13.5px;
  word-break: break-all;
}

.pb-home__pagination {
  justify-content: flex-end;
}

@media (max-width: 1100px) {
  .pb-filters {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .pb-filters__keyword {
    grid-column: 1 / -1;
  }
}

@media (max-width: 640px) {
  .pb-filters {
    grid-template-columns: 1fr 1fr;
  }

  .pb-filters__reset {
    grid-column: 1 / -1;
  }

  .pb-list__item {
    padding: var(--pb-space-lg);
  }

  .pb-list__stats {
    flex-wrap: wrap;
    gap: var(--pb-space-md);
  }

  .pb-list__copy {
    margin-left: 0;
  }
}
</style>
