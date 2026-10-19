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

const CAPABILITY_GROUPS = ['cognitive', 'execution', 'reliability'] as const

type CapabilityGroup = (typeof CAPABILITY_GROUPS)[number]

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
// 初值为真：挂载后要先取字典与登录态，列表请求才发出。若从假开始，
// 这段时间会先渲染「没有符合条件的提示词」与 0 条计数，数据到达后再跳成真实结果。
const loading = ref(true)
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

/** groupLabel 返回考察维度分组的界面文案。 */
function groupLabel(group: CapabilityGroup): string {
  switch (group) {
    case 'execution':
      return t('dimension.groupExecution')
    case 'reliability':
      return t('dimension.groupReliability')
    default:
      return t('dimension.groupCognitive')
  }
}

const capabilityOptionGroups = computed(() =>
  CAPABILITY_GROUPS.map((group) => ({
    key: group,
    label: groupLabel(group),
    options: dictionary.data.capabilities
      .filter((item) => item.group_key === group)
      .map((item) => ({
        value: String(item.id),
        label: localized(item.name_zh, item.name_en),
      })),
  })).filter((group) => group.options.length > 0),
)

const capabilityGroupByID = computed(
  () => new Map(dictionary.data.capabilities.map((item) => [item.id, item.group_key])),
)

const capabilityByID = computed(
  () => new Map(dictionary.data.capabilities.map((item) => [item.id, item])),
)

/** capabilityNames 用字典把全部维度 ID 转成当前语言的名称。 */
function capabilityNames(item: PromptListItem): string {
  const ids = (item.capability_ids?.length ?? 0) > 0 ? item.capability_ids : [item.capability_id]
  const names = ids
    .map((id) => {
      const capability = capabilityByID.value.get(id)
      return capability ? localized(capability.name_zh, capability.name_en) : ''
    })
    .filter((name) => name !== '')
  return names.length > 0
    ? names.join(' · ')
    : localized(item.capability_name_zh, item.capability_name_en)
}

/** capabilityGroupOf 把维度 ID 映射到分组；字典尚未加载时按认知组处理。 */
function capabilityGroupOf(id: number): CapabilityGroup {
  return capabilityGroupByID.value.get(id) ?? 'cognitive'
}

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

/** activeFilterCount 统计当前生效的条件数，标签按已选项数累计。 */
const activeFilterCount = computed(() => {
  let count = 0
  if (filters.value.keyword !== '') count += 1
  if (filters.value.purposeID !== '') count += 1
  if (filters.value.capabilityID !== '') count += 1
  if (filters.value.difficultyID !== '') count += 1
  if (filters.value.modelID !== '') count += 1
  count += filters.value.tagIDs.length
  if (filters.value.sort !== 'latest') count += 1
  return count
})

/** meterStyle 把 1～5 分的取值映射为量表条宽度。 */
function meterStyle(value: number): Record<string, string> {
  const clamped = Math.min(5, Math.max(0, value))
  return { '--pb-meter': `${(clamped / 5) * 100}%` }
}

/** staggerStyle 为列表项提供递减的入场延迟，首屏最多延迟 10 项。 */
function staggerStyle(index: number): Record<string, string> {
  return { animationDelay: `${Math.min(index, 10) * 38}ms` }
}

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
  <section class="pb-home">
    <section class="pb-hero" data-testid="home-hero">
      <div class="pb-hero__copy">
        <span class="pb-eyebrow" aria-hidden="true">Benchmark Library</span>
        <h1 class="pb-home__title">{{ t('home.title') }}</h1>
        <p class="pb-home__lead">{{ t('home.lead') }}</p>

        <div class="pb-hero__actions">
          <RouterLink v-if="auth.isAuthenticated" to="/upload" data-testid="home-upload">
            <el-button type="primary">{{ t('nav.upload') }}</el-button>
          </RouterLink>
          <RouterLink class="pb-hero__method" to="/leaderboard" data-testid="home-leaderboard">
            {{ t('home.leaderboard') }}
            <svg
              viewBox="0 0 24 24"
              width="15"
              height="15"
              fill="none"
              stroke="currentColor"
              stroke-width="1.6"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <path d="M5 12h13M13 6l6 6-6 6" />
            </svg>
          </RouterLink>
          <RouterLink class="pb-hero__method" to="/methodology">
            {{ t('home.methodology') }}
            <svg
              viewBox="0 0 24 24"
              width="15"
              height="15"
              fill="none"
              stroke="currentColor"
              stroke-width="1.6"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <path d="M5 12h13M13 6l6 6-6 6" />
            </svg>
          </RouterLink>
        </div>

        <div class="pb-axis-legend">
          <div class="pb-axis-legend__item">
            <span class="pb-axis-mark pb-axis-mark--purpose" aria-hidden="true" />
            <div class="pb-axis-legend__text">
              <span class="pb-axis-legend__title">{{ t('dimension.purpose') }}</span>
              <span class="pb-axis-legend__desc">{{ t('home.axisPurposeDesc') }}</span>
            </div>
          </div>
          <div class="pb-axis-legend__item">
            <span class="pb-axis-mark pb-axis-mark--capability" aria-hidden="true" />
            <div class="pb-axis-legend__text">
              <span class="pb-axis-legend__title">{{ t('dimension.capability') }}</span>
              <span class="pb-axis-legend__desc">{{ t('home.axisCapabilityDesc') }}</span>
            </div>
          </div>
        </div>
      </div>

      <aside class="pb-ledger" :aria-label="t('home.currentResults')">
        <span class="pb-ledger__rings" aria-hidden="true" />
        <div class="pb-ledger__head">
          <span class="pb-ledger__label">{{ t('home.index') }}</span>
          <span class="pb-ledger__code">PB / INDEX</span>
        </div>
        <p class="pb-ledger__total">
          <span class="pb-ledger__number">{{ loading && items.length === 0 ? '—' : total }}</span>
          <span class="pb-ledger__unit">{{ t('home.entries') }}</span>
        </p>
        <p class="pb-ledger__caption">{{ t('home.currentResults') }}</p>
        <dl class="pb-ledger__stats">
          <div>
            <dt>
              <span class="pb-axis-mark pb-axis-mark--purpose" aria-hidden="true" />
              {{ t('dimension.purpose') }}
            </dt>
            <dd>{{ dictionary.data.purposes.length }}</dd>
          </div>
          <div>
            <dt>
              <span class="pb-axis-mark pb-axis-mark--capability" aria-hidden="true" />
              {{ t('dimension.capability') }}
            </dt>
            <dd>{{ dictionary.data.capabilities.length }}</dd>
          </div>
          <div>
            <dt>{{ t('home.modelsCount') }}</dt>
            <dd>{{ dictionary.data.models.length }}</dd>
          </div>
        </dl>
        <p class="pb-ledger__note">{{ t('home.indexNote') }}</p>
      </aside>
    </section>

    <section id="library" class="pb-library" aria-labelledby="library-heading">
      <header class="pb-library__head">
        <div class="pb-library__heading">
          <span class="pb-eyebrow" aria-hidden="true">Index</span>
          <h2 id="library-heading" class="pb-library__title">{{ t('home.libraryTitle') }}</h2>
        </div>
        <p class="pb-library__count">{{ loading && items.length === 0 ? '' : t('home.total', { count: total }) }}</p>
      </header>

      <div class="pb-filters" data-testid="home-filters">
        <div class="pb-filters__top">
          <div class="pb-filter-field pb-filter-field--search">
            <span class="pb-filter-field__label">{{ t('home.keywordLabel') }}</span>
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
          </div>

          <div class="pb-filter-field">
            <span class="pb-filter-field__label">{{ t('home.sortLabel') }}</span>
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
          </div>

          <el-button
            class="pb-filters__reset"
            data-testid="filter-reset"
            :disabled="!anyFilterActive"
            @click="resetFilters"
          >
            {{ t('home.reset') }}
          </el-button>
        </div>

        <div class="pb-filters__axes">
          <div class="pb-filter-field">
            <span class="pb-filter-field__label">
              <span class="pb-axis-mark pb-axis-mark--purpose" aria-hidden="true" />
              {{ t('home.purpose') }}
            </span>
            <el-select
              v-model="filters.purposeID"
              class="pb-filters__select"
              data-testid="filter-purpose"
              :aria-label="t('home.purpose')"
              :placeholder="t('home.all')"
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
          </div>

          <div class="pb-filter-field">
            <span class="pb-filter-field__label">
              <span class="pb-axis-mark pb-axis-mark--capability" aria-hidden="true" />
              {{ t('home.capability') }}
            </span>
            <el-select
              v-model="filters.capabilityID"
              class="pb-filters__select"
              data-testid="filter-capability"
              :aria-label="t('home.capability')"
              :placeholder="t('home.all')"
              clearable
              @change="applyFilters"
            >
              <el-option-group
                v-for="group in capabilityOptionGroups"
                :key="group.key"
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
          </div>

          <div class="pb-filter-field">
            <span class="pb-filter-field__label">{{ t('home.difficulty') }}</span>
            <el-select
              v-model="filters.difficultyID"
              class="pb-filters__select"
              data-testid="filter-difficulty"
              :aria-label="t('home.difficulty')"
              :placeholder="t('home.all')"
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
          </div>

          <div class="pb-filter-field">
            <span class="pb-filter-field__label">{{ t('home.model') }}</span>
            <el-select
              v-model="filters.modelID"
              class="pb-filters__select"
              data-testid="filter-model"
              :aria-label="t('home.model')"
              :placeholder="t('home.all')"
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
          </div>

          <div class="pb-filter-field">
            <span class="pb-filter-field__label">{{ t('home.tags') }}</span>
            <el-select
              v-model="filters.tagIDs"
              class="pb-filters__select"
              data-testid="filter-tags"
              multiple
              collapse-tags
              :aria-label="t('home.tags')"
              :placeholder="t('home.all')"
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
          </div>
        </div>

        <p v-if="anyFilterActive" class="pb-filters__active">
          <span class="pb-filters__active-dot" aria-hidden="true" />
          {{ t('home.activeFilters', { count: activeFilterCount }) }}
        </p>
      </div>

      <p v-if="notice" class="pb-home__notice" data-testid="home-notice">{{ notice }}</p>

      <p v-if="failed" class="pb-home__notice" data-testid="home-failed">
        {{ t('home.loadFailed') }}
      </p>

      <div
        v-else-if="loading && items.length === 0"
        class="pb-skeleton"
        data-testid="home-loading"
        aria-busy="true"
      >
        <div v-for="index in 3" :key="index" class="pb-skeleton__card">
          <span class="pb-skeleton__line pb-skeleton__line--short" />
          <span class="pb-skeleton__line pb-skeleton__line--title" />
          <span class="pb-skeleton__line" />
          <span class="pb-skeleton__line pb-skeleton__line--medium" />
        </div>
      </div>

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
        <el-button v-if="anyFilterActive" text data-testid="home-empty-reset" @click="resetFilters">
          {{ t('home.reset') }}
        </el-button>
      </div>

      <ul v-else class="pb-list" data-testid="home-list">
        <li
          v-for="(item, index) in items"
          :key="item.id"
          class="pb-list__item"
          :class="`pb-list__item--${capabilityGroupOf(item.capability_id)}`"
          :style="staggerStyle(index)"
          :data-testid="`prompt-card-${item.id}`"
        >
          <span class="pb-list__rail" aria-hidden="true" />

          <div class="pb-list__head">
            <div class="pb-list__chips">
              <span class="pb-list__chip pb-list__chip--purpose">
                <span class="pb-axis-mark pb-axis-mark--purpose" aria-hidden="true" />
                {{ t('dimension.purpose') }} ·
                {{ localized(item.purpose_name_zh, item.purpose_name_en) }}
              </span>
              <span class="pb-list__chip pb-list__chip--capability">
                <span class="pb-axis-mark" aria-hidden="true" />
                {{ t('dimension.capability') }} ·
                {{ capabilityNames(item) }}
              </span>
              <span
                v-if="item.difficulty_name_zh && item.difficulty_name_en"
                class="pb-list__chip"
              >
                {{ t('dimension.difficulty') }} ·
                {{ localized(item.difficulty_name_zh, item.difficulty_name_en) }}
              </span>
              <span v-if="item.major_modified" class="pb-list__chip pb-list__chip--warn">
                {{ t('prompt.majorChange') }}
              </span>
            </div>
            <span class="pb-list__version" :data-testid="`prompt-version-${item.id}`">
              {{ item.version_no }}
            </span>
          </div>

          <RouterLink
            class="pb-list__title"
            :to="`/prompts/${item.id}`"
            :data-testid="`prompt-link-${item.id}`"
          >
            {{ item.title }}
          </RouterLink>

          <p v-if="item.description" class="pb-list__description">{{ item.description }}</p>

          <div class="pb-list__footer">
            <div class="pb-list__metrics">
              <div class="pb-metric">
                <span class="pb-metric__label">{{ t('home.averageLabel') }}</span>
                <span class="pb-metric__row">
                  <span class="pb-metric__value" :data-testid="`prompt-average-${item.id}`">
                    {{ item.average_adjusted.toFixed(2) }}
                  </span>
                  <span class="pb-metric__unit">/ 5</span>
                </span>
                <span class="pb-meter pb-meter--score" aria-hidden="true">
                  <span class="pb-meter__fill" :style="meterStyle(item.average_adjusted)" />
                </span>
              </div>

              <div class="pb-metric">
                <span class="pb-metric__label">{{ t('home.discriminationLabel') }}</span>
                <span class="pb-metric__row">
                  <span
                    class="pb-metric__value"
                    :class="{ 'pb-metric__value--text': !item.data_sufficient }"
                    :data-testid="`prompt-discrimination-${item.id}`"
                  >
                    <template v-if="item.data_sufficient">{{ item.discrimination.toFixed(2) }}</template>
                    <template v-else>{{ t('home.dataInsufficient') }}</template>
                  </span>
                </span>
                <span
                  class="pb-meter pb-meter--discrimination"
                  :class="{ 'pb-meter--empty': !item.data_sufficient }"
                  aria-hidden="true"
                >
                  <span
                    v-if="item.data_sufficient"
                    class="pb-meter__fill"
                    :style="meterStyle(item.discrimination)"
                  />
                </span>
              </div>
            </div>

            <div class="pb-list__side">
              <span class="pb-list__stat">
                {{ t('home.comments') }}
                <span class="pb-list__stat-value">{{ item.comment_count }}</span>
              </span>
              <span class="pb-list__stat">
                {{ t('home.favorites') }}
                <span class="pb-list__stat-value">{{ item.favorite_count }}</span>
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
  </section>
</template>

<style scoped>
.pb-home {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-2xl);
  padding-top: var(--pb-space-lg);
}

/* ---------- 首屏 ---------- */

.pb-hero {
  position: relative;
  display: grid;
  grid-template-columns: minmax(0, 1.45fr) minmax(300px, 0.8fr);
  gap: var(--pb-space-2xl);
  align-items: stretch;
  overflow: hidden;
}

.pb-hero::before {
  content: "PB";
  position: absolute;
  right: 26%;
  bottom: -72px;
  color: var(--pb-ink);
  font-family: var(--pb-font-display);
  font-size: 260px;
  line-height: 1;
  opacity: 0.028;
  pointer-events: none;
}

.pb-hero__copy {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--pb-space-sm);
  padding: var(--pb-space-xl) 0 var(--pb-space-lg);
  animation: pb-rise 620ms var(--pb-ease-out) both;
}

.pb-home__title {
  margin: 0;
  font-family: var(--pb-font-display);
  font-size: clamp(38px, 5.6vw, 58px);
  font-weight: 500;
  letter-spacing: -0.022em;
  line-height: 1.08;
  text-wrap: balance;
}

.pb-home__lead {
  max-width: 52ch;
  margin: var(--pb-space-sm) 0 0;
  color: var(--pb-ink-soft);
  font-size: 15.5px;
  line-height: 1.8;
}

.pb-hero__actions {
  display: flex;
  align-items: center;
  gap: var(--pb-space-lg);
  flex-wrap: wrap;
  margin-top: var(--pb-space-md);
}

.pb-hero__method {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  color: var(--pb-ink-soft);
  font-size: 13.5px;
  letter-spacing: 0.01em;
  transition:
    color var(--pb-duration) var(--pb-ease),
    gap var(--pb-duration) var(--pb-ease);
}

.pb-hero__method:hover {
  gap: 11px;
  color: var(--pb-accent);
}

.pb-axis-legend {
  display: flex;
  gap: var(--pb-space-xl);
  flex-wrap: wrap;
  margin-top: var(--pb-space-xl);
  padding-top: var(--pb-space-md);
  border-top: 1px solid var(--pb-line);
}

.pb-axis-legend__item {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  min-width: 180px;
}

.pb-axis-legend__item .pb-axis-mark {
  margin-top: 6px;
}

.pb-axis-legend__text {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.pb-axis-legend__title {
  color: var(--pb-ink);
  font-size: 12.5px;
  font-weight: 600;
  letter-spacing: 0.04em;
}

.pb-axis-legend__desc {
  color: var(--pb-ink-faint);
  font-size: 12px;
  line-height: 1.6;
}

/* 右侧索引卡：用真实计数构成首屏的数据锚点。 */
.pb-ledger {
  position: relative;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
  min-width: 0;
  padding: var(--pb-space-lg);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-xl);
  background:
    linear-gradient(150deg, var(--pb-surface) 0%, var(--pb-surface-muted) 100%);
  box-shadow:
    var(--pb-shadow-md),
    var(--pb-shadow-inset);
  animation: pb-rise 720ms 90ms var(--pb-ease-out) both;
}

.pb-ledger__rings {
  position: absolute;
  right: -110px;
  top: -110px;
  width: 280px;
  height: 280px;
  border-radius: 50%;
  background:
    radial-gradient(circle, transparent 0 42%, var(--pb-line) 42% 42.4%, transparent 42.4%),
    radial-gradient(circle, transparent 0 60%, var(--pb-line) 60% 60.4%, transparent 60.4%),
    radial-gradient(circle, transparent 0 78%, var(--pb-line) 78% 78.4%, transparent 78.4%);
  opacity: 0.7;
  pointer-events: none;
}

.pb-ledger__head {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--pb-space-md);
  padding-bottom: var(--pb-space-sm);
  border-bottom: 1px solid var(--pb-line);
  color: var(--pb-ink-faint);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.pb-ledger__code {
  font-family: var(--pb-font-mono);
  font-weight: 500;
  letter-spacing: 0.08em;
  opacity: 0.72;
}

.pb-ledger__total {
  position: relative;
  display: flex;
  align-items: baseline;
  gap: 8px;
  margin: var(--pb-space-md) 0 0;
}

.pb-ledger__number {
  color: var(--pb-ink);
  font-family: var(--pb-font-display);
  font-size: 64px;
  font-weight: 500;
  line-height: 0.95;
  letter-spacing: -0.04em;
  font-variant-numeric: tabular-nums;
}

.pb-ledger__unit {
  color: var(--pb-ink-faint);
  font-size: 12px;
  letter-spacing: 0.08em;
}

.pb-ledger__caption {
  margin: 2px 0 var(--pb-space-sm);
  color: var(--pb-ink-faint);
  font-size: 12px;
}

.pb-ledger__stats {
  position: relative;
  display: flex;
  flex-direction: column;
  margin: 0;
  border-top: 1px dashed var(--pb-line-strong);
}

.pb-ledger__stats > div {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--pb-space-md);
  padding: 9px 0;
  border-bottom: 1px dashed var(--pb-line);
}

.pb-ledger__stats > div:last-child {
  border-bottom: 0;
}

.pb-ledger__stats dt {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: var(--pb-ink-soft);
  font-size: 12.5px;
}

.pb-ledger__stats dd {
  margin: 0;
  color: var(--pb-ink);
  font-family: var(--pb-font-display);
  font-size: 21px;
  font-variant-numeric: tabular-nums;
}

.pb-ledger__note {
  position: relative;
  margin: auto 0 0;
  padding-top: var(--pb-space-sm);
  color: var(--pb-ink-faint);
  font-size: 11px;
  line-height: 1.6;
}

/* ---------- 索引标题与筛选 ---------- */

.pb-library {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-md);
  scroll-margin-top: calc(var(--pb-header-height) + var(--pb-space-lg));
}

.pb-library__head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--pb-space-lg);
  flex-wrap: wrap;
  padding-bottom: var(--pb-space-sm);
  border-bottom: 1px solid var(--pb-line);
}

.pb-library__heading {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-xs);
}

.pb-library__title {
  margin: 0;
  font-family: var(--pb-font-display);
  font-size: 26px;
  font-weight: 500;
  letter-spacing: -0.01em;
}

.pb-library__count {
  margin: 0;
  color: var(--pb-ink-faint);
  font-size: 12.5px;
  letter-spacing: 0.04em;
}

.pb-filters {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-md);
  padding: var(--pb-space-md);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
  box-shadow:
    var(--pb-shadow-sm),
    var(--pb-shadow-inset);
}

.pb-filters__top {
  display: grid;
  grid-template-columns: minmax(220px, 1fr) minmax(150px, 190px) auto;
  gap: var(--pb-space-sm);
  align-items: end;
}

.pb-filters__axes {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: var(--pb-space-sm);
}

.pb-filter-field {
  display: flex;
  flex-direction: column;
  gap: 5px;
  min-width: 0;
}

.pb-filter-field__label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--pb-ink-faint);
  font-size: 10.5px;
  font-weight: 600;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.pb-filters__keyword,
.pb-filters__select {
  width: 100%;
}

.pb-filters__reset {
  white-space: nowrap;
}

.pb-filters__active {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  align-self: flex-start;
  margin: 0;
  padding: 2px 10px;
  border-radius: var(--pb-radius-pill);
  background: var(--pb-accent-soft);
  color: var(--pb-accent-strong);
  font-size: 11.5px;
  letter-spacing: 0.02em;
}

.pb-filters__active-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: currentColor;
}

/* ---------- 列表 ---------- */

.pb-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-md);
}

.pb-list__item {
  --pb-group: var(--pb-group-cognitive);
  --pb-group-strong: var(--pb-group-cognitive-strong);
  --pb-group-soft: var(--pb-group-cognitive-soft);
  position: relative;
  overflow: hidden;
  padding: var(--pb-space-lg) var(--pb-space-xl) var(--pb-space-lg) 28px;
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
  box-shadow:
    var(--pb-shadow-sm),
    var(--pb-shadow-inset);
  animation: pb-rise 460ms var(--pb-ease-out) both;
  transition:
    transform var(--pb-duration) var(--pb-ease-out),
    box-shadow var(--pb-duration) var(--pb-ease),
    border-color var(--pb-duration) var(--pb-ease);
}

.pb-list__item--execution {
  --pb-group: var(--pb-group-execution);
  --pb-group-strong: var(--pb-group-execution-strong);
  --pb-group-soft: var(--pb-group-execution-soft);
}

.pb-list__item--reliability {
  --pb-group: var(--pb-group-reliability);
  --pb-group-strong: var(--pb-group-reliability-strong);
  --pb-group-soft: var(--pb-group-reliability-soft);
}

.pb-list__item:hover {
  transform: translateY(-3px);
  border-color: var(--pb-line-strong);
  box-shadow:
    var(--pb-shadow-hover),
    var(--pb-shadow-inset);
}

.pb-list__rail {
  position: absolute;
  left: 0;
  top: 18px;
  bottom: 18px;
  width: 3px;
  border-radius: 0 2px 2px 0;
  background: linear-gradient(180deg, var(--pb-group), transparent 92%);
  opacity: 0.55;
  transition: opacity var(--pb-duration) var(--pb-ease);
}

.pb-list__item:hover .pb-list__rail {
  opacity: 1;
}

.pb-list__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--pb-space-md);
}

.pb-list__chips {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--pb-space-xs);
  min-width: 0;
}

.pb-list__chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 9px;
  border-radius: var(--pb-radius-pill);
  background: var(--pb-surface-muted);
  color: var(--pb-ink-soft);
  font-size: 11.5px;
  line-height: 1.7;
  white-space: nowrap;
}

.pb-list__chip--purpose {
  border: 1px solid var(--pb-purpose-ring);
  background: var(--pb-purpose-soft);
  color: var(--pb-purpose-strong);
}

.pb-list__chip--capability {
  background: var(--pb-group-soft);
  color: var(--pb-group-strong);
}

.pb-list__chip--capability .pb-axis-mark {
  background: var(--pb-group);
}

.pb-list__chip--warn {
  background: var(--pb-accent-soft);
  color: var(--pb-accent-strong);
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

.pb-list__title {
  display: block;
  margin-top: 10px;
  color: var(--pb-ink);
  font-family: var(--pb-font-display);
  font-size: 23px;
  font-weight: 500;
  letter-spacing: -0.008em;
  line-height: 1.3;
  text-wrap: balance;
  transition: color var(--pb-duration) var(--pb-ease);
}

.pb-list__title:hover {
  color: var(--pb-accent);
}

.pb-list__description {
  max-width: 78ch;
  margin: 7px 0 0;
  color: var(--pb-ink-soft);
  font-size: 13.5px;
  line-height: 1.7;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.pb-list__footer {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--pb-space-lg);
  flex-wrap: wrap;
  margin-top: var(--pb-space-md);
  padding-top: var(--pb-space-sm);
  border-top: 1px dashed var(--pb-line);
}

.pb-list__metrics {
  display: flex;
  align-items: flex-end;
  gap: var(--pb-space-xl);
  flex-wrap: wrap;
}

.pb-metric {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 132px;
  max-width: 190px;
}

.pb-metric__label {
  color: var(--pb-ink-faint);
  font-size: 10.5px;
  font-weight: 600;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.pb-metric__row {
  display: flex;
  align-items: baseline;
  gap: 5px;
}

.pb-metric__value {
  color: var(--pb-ink);
  font-family: var(--pb-font-display);
  font-size: 22px;
  line-height: 1.1;
  font-variant-numeric: tabular-nums;
}

.pb-metric__value--text {
  color: var(--pb-ink-faint);
  font-family: var(--pb-font-body);
  font-size: 11.5px;
  line-height: 1.5;
}

.pb-metric__unit {
  color: var(--pb-ink-faint);
  font-size: 10.5px;
}

.pb-meter {
  display: block;
  width: 100%;
  height: 3px;
  border-radius: 2px;
  background: var(--pb-line);
  overflow: hidden;
}

.pb-meter__fill {
  display: block;
  width: var(--pb-meter, 0%);
  height: 100%;
  border-radius: inherit;
  transform-origin: left center;
  animation: pb-grow 620ms var(--pb-ease-out) both;
}

.pb-meter--score .pb-meter__fill {
  background: linear-gradient(90deg, var(--pb-accent), var(--pb-accent-strong));
}

.pb-meter--discrimination .pb-meter__fill {
  background: linear-gradient(90deg, var(--pb-purpose), var(--pb-group-cognitive));
}

.pb-meter--empty {
  background: repeating-linear-gradient(
    90deg,
    var(--pb-line) 0 4px,
    transparent 4px 8px
  );
}

.pb-list__side {
  display: flex;
  align-items: center;
  gap: var(--pb-space-md);
  flex-wrap: wrap;
}

.pb-list__stat {
  color: var(--pb-ink-faint);
  font-size: 11.5px;
  white-space: nowrap;
}

.pb-list__stat-value {
  margin-left: 3px;
  color: var(--pb-ink-soft);
  font-family: var(--pb-font-mono);
  font-size: 11.5px;
  font-variant-numeric: tabular-nums;
}

.pb-list__copy {
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

/* ---------- 加载骨架 ---------- */

.pb-skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-md);
}

.pb-skeleton__card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: var(--pb-space-lg) var(--pb-space-xl);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
}

.pb-skeleton__line {
  height: 11px;
  border-radius: 6px;
  background: linear-gradient(
    90deg,
    var(--pb-surface-muted) 20%,
    var(--pb-surface) 45%,
    var(--pb-surface-muted) 70%
  );
  background-size: 220% 100%;
  animation: pb-shimmer 1.5s linear infinite;
}

.pb-skeleton__line--short {
  width: 120px;
}

.pb-skeleton__line--title {
  width: 46%;
  height: 20px;
}

.pb-skeleton__line--medium {
  width: 68%;
}

@media (max-width: 1080px) {
  .pb-hero {
    grid-template-columns: minmax(0, 1fr) minmax(280px, 0.72fr);
    gap: var(--pb-space-xl);
  }

  .pb-hero::before {
    display: none;
  }

  .pb-filters__top {
    grid-template-columns: minmax(200px, 1fr) minmax(140px, 180px) auto;
  }
}

@media (max-width: 900px) {
  .pb-hero {
    grid-template-columns: 1fr;
  }

  .pb-hero__copy {
    padding-bottom: 0;
  }

  .pb-ledger {
    max-width: 560px;
  }
}

@media (max-width: 640px) {
  .pb-home {
    gap: var(--pb-space-xl);
    padding-top: 0;
  }

  .pb-home__title {
    font-size: clamp(34px, 10vw, 42px);
  }

  .pb-home__lead {
    font-size: 14.5px;
  }

  .pb-axis-legend {
    flex-direction: column;
    gap: var(--pb-space-sm);
  }

  .pb-filters__top {
    grid-template-columns: 1fr;
  }

  .pb-filters__reset {
    width: 100%;
  }

  .pb-filters__axes {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .pb-list__item {
    padding: var(--pb-space-lg) var(--pb-space-md) var(--pb-space-lg) 22px;
  }

  .pb-list__title {
    font-size: 20px;
  }

  .pb-list__footer {
    align-items: flex-start;
    gap: var(--pb-space-md);
  }

  .pb-list__metrics {
    gap: var(--pb-space-lg);
    width: 100%;
  }

  .pb-metric {
    min-width: 0;
    flex: 1 1 120px;
    max-width: none;
  }

  .pb-list__side {
    width: 100%;
  }

  .pb-list__copy {
    margin-left: auto;
  }

  .pb-skeleton__card {
    padding: var(--pb-space-lg) var(--pb-space-md);
  }
}
</style>
