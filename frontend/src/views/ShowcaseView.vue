<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { type ShowcaseEntry, type ShowcaseGroup, showcaseApi } from '@/api/showcase'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { formatDate } from '@/utils/datetime'

const { t, locale } = useI18n()
const route = useRoute()
const router = useRouter()
const { messageOf } = useErrorMessage()

const groups = ref<ShowcaseGroup[]>([])
const entries = ref<ShowcaseEntry[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 12
const loading = ref(true)
const entriesLoading = ref(false)
const failure = ref('')
const entriesFailure = ref('')
const providerKey = ref('')
const selectedModelID = ref<number | null>(null)

/** providers 是展示区涉及的模型类别（提供厂商），带素材计数。 */
const providers = computed(() => {
  const map = new Map<string, { key: string; name: string; count: number }>()
  for (const group of groups.value) {
    const key = group.provider_key || ''
    const current = map.get(key)
    const count = group.image_count + group.output_count
    if (current) {
      current.count += count
    } else {
      map.set(key, {
        key,
        name: group.provider_name || group.provider_key || t('showcase.providerOther'),
        count,
      })
    }
  }
  return [...map.values()].sort((left, right) => left.name.localeCompare(right.name))
})

/** visibleGroups 是当前类别下的模型。 */
const visibleGroups = computed(() =>
  groups.value.filter(
    (group) => providerKey.value === '' || group.provider_key === providerKey.value,
  ),
)

/** selectedGroup 是当前展示素材的模型。 */
const selectedGroup = computed(
  () => groups.value.find((group) => group.model_id === selectedModelID.value) ?? null,
)

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

function providerName(group: ShowcaseGroup): string {
  return group.provider_name || group.provider_key || t('showcase.providerOther')
}

function selectProvider(key: string): void {
  providerKey.value = key
  const first = visibleGroups.value[0]
  selectModel(first ? first.model_id : null)
}

function selectModel(modelID: number | null): void {
  selectedModelID.value = modelID
  page.value = 1
  entries.value = []
  total.value = 0
  if (modelID !== null) {
    void syncQuery(modelID)
    void loadEntries()
  } else {
    void syncQuery(null)
  }
}

/** syncQuery 把当前模型写回地址栏，刷新与分享后保持同一视图。 */
async function syncQuery(modelID: number | null): Promise<void> {
  const query = { ...route.query }
  if (modelID === null) {
    delete query.model
  } else {
    query.model = String(modelID)
  }
  await router.replace({ query })
}

async function loadGroups(): Promise<void> {
  loading.value = true
  failure.value = ''
  try {
    const result = await showcaseApi.groups()
    groups.value = result.items
    const requested = Number.parseInt(String(route.query.model ?? ''), 10)
    const byQuery = Number.isNaN(requested)
      ? undefined
      : groups.value.find((group) => group.model_id === requested)
    selectedModelID.value = byQuery?.model_id ?? groups.value[0]?.model_id ?? null
    if (byQuery) {
      providerKey.value = byQuery.provider_key
    }
    if (selectedModelID.value !== null) {
      await loadEntries()
    }
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    loading.value = false
  }
}

async function loadEntries(): Promise<void> {
  if (selectedModelID.value === null) {
    return
  }
  entriesLoading.value = true
  entriesFailure.value = ''
  try {
    const result = await showcaseApi.entries(selectedModelID.value, page.value, pageSize)
    entries.value = result.items
    total.value = result.total
  } catch (error) {
    entries.value = []
    total.value = 0
    entriesFailure.value = messageOf(error)
  } finally {
    entriesLoading.value = false
  }
}

function changePage(next: number): void {
  page.value = next
  void loadEntries()
}

/** previewList 让图片查看器在当前页的图片之间前后切换。 */
function previewList(): string[] {
  return entries.value
    .filter((item) => item.kind === 'image' && item.url)
    .map((item) => item.url as string)
}

onMounted(loadGroups)
</script>

<template>
  <section class="pb-showcase" data-testid="showcase-page">
    <header class="pb-showcase__head">
      <div class="pb-showcase__heading">
        <span class="pb-eyebrow" aria-hidden="true">Gallery</span>
        <h1 class="pb-showcase__title">{{ t('showcase.title') }}</h1>
        <p class="pb-showcase__lead">{{ t('showcase.lead') }}</p>
      </div>
      <RouterLink class="pb-showcase__back" to="/" data-testid="showcase-home-link">
        {{ t('showcase.browsePrompts') }}
      </RouterLink>
    </header>

    <p v-if="loading" class="pb-showcase__hint">{{ t('showcase.loading') }}</p>
    <p v-else-if="failure" class="pb-showcase__error" data-testid="showcase-error">
      {{ t('showcase.failed') }} {{ failure }}
    </p>
    <p v-else-if="groups.length === 0" class="pb-showcase__empty" data-testid="showcase-empty">
      {{ t('showcase.empty') }}
    </p>

    <div v-else class="pb-showcase__layout">
      <aside class="pb-showcase__side">
        <div class="pb-showcase__side-block">
          <span class="pb-showcase__side-label">{{ t('showcase.providerLabel') }}</span>
          <div class="pb-showcase__providers">
            <button
              type="button"
              class="pb-showcase__provider"
              :class="{ 'pb-showcase__provider--active': providerKey === '' }"
              data-testid="showcase-provider-all"
              @click="selectProvider('')"
            >
              <span>{{ t('showcase.providerAll') }}</span>
              <span class="pb-showcase__count">{{ groups.length }}</span>
            </button>
            <button
              v-for="provider in providers"
              :key="provider.key"
              type="button"
              class="pb-showcase__provider"
              :class="{ 'pb-showcase__provider--active': providerKey === provider.key }"
              :data-testid="`showcase-provider-${provider.key || 'other'}`"
              @click="selectProvider(provider.key)"
            >
              <span class="pb-showcase__provider-name">{{ provider.name }}</span>
              <span class="pb-showcase__count">{{ provider.count }}</span>
            </button>
          </div>
        </div>

        <div class="pb-showcase__side-block">
          <span class="pb-showcase__side-label">{{ t('showcase.modelLabel') }}</span>
          <div class="pb-showcase__models">
            <button
              v-for="group in visibleGroups"
              :key="group.model_id"
              type="button"
              class="pb-showcase__model"
              :class="{ 'pb-showcase__model--active': group.model_id === selectedModelID }"
              :data-testid="`showcase-model-${group.model_id}`"
              @click="selectModel(group.model_id)"
            >
              <span class="pb-showcase__model-name">{{ group.model_name }}</span>
              <span class="pb-showcase__model-meta">
                {{ t('showcase.imageCount', { count: group.image_count }) }} ·
                {{ t('showcase.outputCount', { count: group.output_count }) }}
              </span>
            </button>
          </div>
          <p v-if="visibleGroups.length === 0" class="pb-showcase__side-empty">
            {{ t('showcase.noModelInProvider') }}
          </p>
        </div>
      </aside>

      <div class="pb-showcase__main">
        <div v-if="selectedGroup" class="pb-showcase__main-head">
          <div class="pb-showcase__main-title">
            <h2 data-testid="showcase-current-model">{{ selectedGroup.model_name }}</h2>
            <span class="pb-showcase__main-provider">{{ providerName(selectedGroup) }}</span>
          </div>
          <span class="pb-showcase__main-total" data-testid="showcase-total">
            {{ t('showcase.total', { count: total }) }}
          </span>
        </div>

        <p v-if="entriesLoading" class="pb-showcase__hint">{{ t('showcase.loadingEntries') }}</p>
        <p v-else-if="entriesFailure" class="pb-showcase__error">
          {{ t('showcase.entriesFailed') }} {{ entriesFailure }}
        </p>
        <p v-else-if="entries.length === 0" class="pb-showcase__empty" data-testid="showcase-model-empty">
          {{ t('showcase.emptyModel') }}
        </p>

        <ul v-else class="pb-showcase__grid" data-testid="showcase-entries">
          <li
            v-for="entry in entries"
            :key="`${entry.kind}-${entry.id}`"
            class="pb-showcase__card"
            :class="`pb-showcase__card--${entry.kind}`"
            :data-testid="`showcase-entry-${entry.kind}-${entry.id}`"
          >
            <div v-if="entry.kind === 'image'" class="pb-showcase__media">
              <el-image
                :src="entry.url"
                :alt="t('showcase.imageAlt', { model: selectedGroup?.model_name ?? '' })"
                :preview-src-list="previewList()"
                preview-teleported
                hide-on-click-modal
                fit="cover"
                loading="lazy"
                class="pb-showcase__image"
              />
            </div>
            <pre v-else class="pb-showcase__output">{{ entry.content }}</pre>

            <div class="pb-showcase__card-body">
              <RouterLink
                class="pb-showcase__prompt"
                :to="`/prompts/${entry.prompt_id}`"
                :data-testid="`showcase-prompt-${entry.kind}-${entry.id}`"
              >
                {{ entry.prompt_title }}
              </RouterLink>
              <div class="pb-showcase__card-meta">
                <span class="pb-showcase__kind">
                  {{ entry.kind === 'image' ? t('showcase.kindImage') : t('showcase.kindOutput') }}
                </span>
                <span>{{ t('showcase.contributedBy', { user: entry.username }) }}</span>
                <span>{{ formatDate(entry.created_at, locale) }}</span>
              </div>
            </div>
          </li>
        </ul>

        <el-pagination
          v-if="totalPages > 1"
          class="pb-showcase__pagination"
          layout="prev, pager, next"
          :current-page="page"
          :page-size="pageSize"
          :total="total"
          background
          hide-on-single-page
          @current-change="changePage"
        />
      </div>
    </div>
  </section>
</template>

<style scoped>
.pb-showcase {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-xl);
  padding: var(--pb-space-xl) 0 var(--pb-space-2xl);
}

.pb-showcase__head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--pb-space-lg);
  padding-bottom: var(--pb-space-md);
  border-bottom: 1px solid var(--pb-line);
}

.pb-showcase__heading {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-xs);
  max-width: 68ch;
}

.pb-showcase__title {
  margin: 0;
  font-family: var(--pb-font-display);
  font-size: clamp(30px, 4vw, 42px);
  font-weight: 500;
  letter-spacing: -0.018em;
  line-height: 1.15;
}

.pb-showcase__lead {
  margin: 0;
  color: var(--pb-ink-soft);
  font-size: 14.5px;
  line-height: 1.8;
}

.pb-showcase__back {
  flex: none;
  padding-bottom: 2px;
  border-bottom: 1px solid var(--pb-accent-ring);
  color: var(--pb-accent);
  font-size: 13px;
  transition: border-color var(--pb-duration) var(--pb-ease);
}

.pb-showcase__back:hover {
  border-color: var(--pb-accent);
}

.pb-showcase__layout {
  display: grid;
  grid-template-columns: 236px minmax(0, 1fr);
  gap: var(--pb-space-xl);
  align-items: start;
}

.pb-showcase__side {
  position: sticky;
  top: calc(var(--pb-header-height) + var(--pb-space-lg));
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-lg);
  padding: var(--pb-space-md);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
  box-shadow:
    var(--pb-shadow-sm),
    var(--pb-shadow-inset);
}

.pb-showcase__side-block {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-xs);
  min-width: 0;
}

.pb-showcase__side-label {
  color: var(--pb-ink-faint);
  font-size: 9.5px;
  font-weight: 600;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.pb-showcase__providers,
.pb-showcase__models {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.pb-showcase__provider,
.pb-showcase__model {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--pb-space-sm);
  width: 100%;
  padding: 7px 10px;
  border: 0;
  border-radius: var(--pb-radius-sm);
  background: transparent;
  color: var(--pb-ink-soft);
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
  transition:
    background-color var(--pb-duration) var(--pb-ease),
    color var(--pb-duration) var(--pb-ease);
}

.pb-showcase__provider:hover,
.pb-showcase__model:hover {
  background: var(--pb-surface-muted);
  color: var(--pb-ink);
}

.pb-showcase__provider--active,
.pb-showcase__model--active {
  background: var(--pb-accent-soft);
  color: var(--pb-accent-strong);
}

.pb-showcase__provider-name,
.pb-showcase__model-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pb-showcase__count {
  flex: none;
  color: var(--pb-ink-faint);
  font-family: var(--pb-font-mono);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
}

.pb-showcase__model {
  align-items: flex-start;
  flex-direction: column;
  gap: 2px;
}

.pb-showcase__model-meta {
  color: var(--pb-ink-faint);
  font-size: 10.5px;
}

.pb-showcase__side-empty {
  margin: 0;
  padding: var(--pb-space-sm) 10px;
  color: var(--pb-ink-faint);
  font-size: 12px;
}

.pb-showcase__main {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-md);
  min-width: 0;
}

.pb-showcase__main-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--pb-space-md);
  padding-bottom: var(--pb-space-sm);
  border-bottom: 1px solid var(--pb-line);
}

.pb-showcase__main-title {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: var(--pb-space-sm);
  min-width: 0;
}

.pb-showcase__main-title h2 {
  margin: 0;
  font-family: var(--pb-font-display);
  font-size: 25px;
  font-weight: 500;
  letter-spacing: -0.01em;
}

.pb-showcase__main-provider {
  color: var(--pb-ink-faint);
  font-size: 12px;
}

.pb-showcase__main-total {
  flex: none;
  color: var(--pb-ink-faint);
  font-family: var(--pb-font-mono);
  font-size: 11.5px;
  font-variant-numeric: tabular-nums;
}

.pb-showcase__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
  gap: var(--pb-space-md);
  list-style: none;
  margin: 0;
  padding: 0;
}

.pb-showcase__card {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
  box-shadow:
    var(--pb-shadow-sm),
    var(--pb-shadow-inset);
  transition:
    border-color var(--pb-duration) var(--pb-ease),
    transform var(--pb-duration) var(--pb-ease),
    box-shadow var(--pb-duration) var(--pb-ease);
  animation: pb-showcase-rise 420ms var(--pb-ease-out) both;
}

.pb-showcase__card:hover {
  border-color: var(--pb-line-strong);
  transform: translateY(-2px);
  box-shadow: var(--pb-shadow-md);
}

.pb-showcase__media {
  aspect-ratio: 4 / 3;
  overflow: hidden;
  background: var(--pb-surface-muted);
  border-bottom: 1px solid var(--pb-line);
}

.pb-showcase__image {
  display: block;
  width: 100%;
  height: 100%;
}

.pb-showcase__output {
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 9;
  overflow: hidden;
  margin: 0;
  padding: var(--pb-space-md);
  min-height: 180px;
  max-height: 260px;
  border-bottom: 1px solid var(--pb-line);
  background: color-mix(in srgb, var(--pb-surface-muted) 55%, transparent);
  color: var(--pb-ink);
  font-family: var(--pb-font-mono);
  font-size: 12px;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
}

.pb-showcase__card-body {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: var(--pb-space-md);
  min-width: 0;
}

.pb-showcase__prompt {
  color: var(--pb-ink);
  font-family: var(--pb-font-display);
  font-size: 15.5px;
  line-height: 1.4;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
  transition: color var(--pb-duration) var(--pb-ease);
}

.pb-showcase__prompt:hover {
  color: var(--pb-accent-strong);
}

.pb-showcase__card-meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px 10px;
  color: var(--pb-ink-faint);
  font-size: 11px;
}

.pb-showcase__kind {
  padding: 1px 8px;
  border-radius: var(--pb-radius-pill);
  background: var(--pb-accent-soft);
  color: var(--pb-accent-strong);
  font-size: 10.5px;
  font-weight: 600;
}

.pb-showcase__pagination {
  align-self: center;
  margin-top: var(--pb-space-sm);
}

.pb-showcase__hint,
.pb-showcase__empty {
  margin: 0;
  padding: var(--pb-space-2xl) var(--pb-space-lg);
  border: 1px dashed var(--pb-line-strong);
  border-radius: var(--pb-radius-lg);
  color: var(--pb-ink-faint);
  font-size: 13.5px;
  text-align: center;
}

.pb-showcase__error {
  margin: 0;
  padding: var(--pb-space-sm) var(--pb-space-md);
  border-radius: var(--pb-radius-md);
  background: var(--pb-accent-soft);
  color: var(--pb-danger);
  font-size: 13.5px;
}

@keyframes pb-showcase-rise {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (max-width: 900px) {
  .pb-showcase__head {
    flex-direction: column;
    align-items: flex-start;
  }

  .pb-showcase__layout {
    grid-template-columns: 1fr;
  }

  .pb-showcase__side {
    position: static;
    flex-direction: row;
    flex-wrap: wrap;
    gap: var(--pb-space-md);
  }

  .pb-showcase__side-block {
    flex: 1 1 240px;
  }

  .pb-showcase__providers,
  .pb-showcase__models {
    flex-direction: row;
    flex-wrap: wrap;
  }

  .pb-showcase__provider,
  .pb-showcase__model {
    width: auto;
  }
}

@media (prefers-reduced-motion: reduce) {
  .pb-showcase__card {
    animation: none;
  }
}
</style>
