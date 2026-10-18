<script setup lang="ts">
// 区分度排行榜：公开只读页面，按固定周期轮询同一份服务端快照。
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  type ConfidenceLevel,
  type Leaderboard,
  type LeaderboardWindow,
  statsApi,
} from '@/api/stats'
import { useErrorMessage } from '@/composables/useErrorMessage'

const { t } = useI18n()
const { messageOf } = useErrorMessage()

const WINDOWS: { value: LeaderboardWindow; labelKey: string }[] = [
  { value: 'overall', labelKey: 'leaderboard.windowOverall' },
  { value: 'recent', labelKey: 'leaderboard.windowRecent' },
  { value: 'history', labelKey: 'leaderboard.windowHistory' },
]

const windowKey = ref<LeaderboardWindow>('overall')
const board = ref<Leaderboard | null>(null)
const loading = ref(true)
const failure = ref('')
let sequence = 0
let timer: number | undefined

const items = computed(() => board.value?.items ?? [])
const refreshSeconds = computed(() => board.value?.refresh_seconds ?? 60)
const refreshedAt = computed(() => formatTime(board.value?.refreshed_at ?? ''))

/** formatTime 把 UTC 时刻按本机时区呈现。 */
function formatTime(raw: string): string {
  if (raw === '') {
    return ''
  }
  const parsed = new Date(raw)
  return Number.isNaN(parsed.getTime()) ? raw : parsed.toLocaleTimeString()
}

function scorePercent(value: number): string {
  return `${Math.max(0, Math.min(100, (value / 5) * 100))}%`
}

function confidenceLabel(level: ConfidenceLevel): string {
  switch (level) {
    case 'high':
      return t('leaderboard.confidenceHigh')
    case 'medium':
      return t('leaderboard.confidenceMedium')
    default:
      return t('leaderboard.confidenceLow')
  }
}

async function load(): Promise<void> {
  const current = ++sequence
  loading.value = true
  try {
    const result = await statsApi.leaderboard(windowKey.value, 20)
    if (current !== sequence) {
      return
    }
    board.value = result
    failure.value = ''
  } catch (error) {
    if (current !== sequence) {
      return
    }
    failure.value = messageOf(error)
  } finally {
    if (current === sequence) {
      loading.value = false
    }
  }
}

function selectWindow(value: LeaderboardWindow): void {
  if (windowKey.value === value) {
    return
  }
  windowKey.value = value
  void load()
}

/** onVisibilityChange 在页面重新可见时补一次刷新，避免后台标签页空转。 */
function onVisibilityChange(): void {
  if (!document.hidden) {
    void load()
  }
}

onMounted(() => {
  void load()
  timer = window.setInterval(() => {
    if (!document.hidden) {
      void load()
    }
  }, refreshSeconds.value * 1000)
  document.addEventListener('visibilitychange', onVisibilityChange)
})

onBeforeUnmount(() => {
  if (timer !== undefined) {
    window.clearInterval(timer)
  }
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<template>
  <section class="pb-leaderboard" data-testid="leaderboard-page">
    <header class="pb-leaderboard__hero">
      <span class="pb-eyebrow" aria-hidden="true">Ranking / Discrimination</span>
      <div class="pb-leaderboard__headline">
        <h1 class="pb-leaderboard__title">{{ t('leaderboard.title') }}</h1>
        <span class="pb-leaderboard__auto">
          {{ t('leaderboard.autoRefresh', { seconds: refreshSeconds }) }}
        </span>
      </div>
      <p class="pb-leaderboard__lead">{{ t('leaderboard.lead') }}</p>

      <div class="pb-leaderboard__meta">
        <span v-if="refreshedAt" class="pb-leaderboard__refreshed">
          {{ t('leaderboard.refreshedAt', { time: refreshedAt }) }}
        </span>
        <button
          type="button"
          class="pb-leaderboard__refresh"
          :class="{ 'is-loading': loading }"
          data-testid="leaderboard-refresh"
          @click="load"
        >
          <svg
            viewBox="0 0 24 24"
            width="14"
            height="14"
            fill="none"
            stroke="currentColor"
            stroke-width="1.7"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <path d="M20 11a8 8 0 1 0-2.3 5.7M20 5v6h-6" />
          </svg>
          {{ t('leaderboard.refresh') }}
        </button>
      </div>

      <div class="pb-leaderboard__tabs" role="tablist" data-testid="leaderboard-windows">
        <button
          v-for="option in WINDOWS"
          :key="option.value"
          type="button"
          role="tab"
          class="pb-leaderboard__tab"
          :class="{ 'is-active': windowKey === option.value }"
          :aria-selected="windowKey === option.value"
          @click="selectWindow(option.value)"
        >
          {{ t(option.labelKey) }}
        </button>
      </div>
    </header>

    <p v-if="failure" class="pb-leaderboard__error" data-testid="leaderboard-error">
      {{ t('leaderboard.failed') }} {{ failure }}
    </p>

    <div v-if="loading && board === null" class="pb-leaderboard__skeleton" aria-hidden="true">
      <span v-for="row in 6" :key="row" class="pb-leaderboard__skeleton-row" />
    </div>

    <div v-else-if="items.length === 0" class="pb-empty" data-testid="leaderboard-empty">
      <span class="pb-empty__title">{{ t('leaderboard.emptyTitle') }}</span>
      <span class="pb-empty__hint">{{ t('leaderboard.emptyBody') }}</span>
    </div>

    <ol v-else class="pb-leaderboard__list" :class="{ 'is-refreshing': loading }" data-testid="leaderboard-list">
      <li
        v-for="(entry, index) in items"
        :key="entry.prompt_id"
        class="pb-leaderboard__item"
        :style="{ animationDelay: `${Math.min(index, 12) * 38}ms` }"
      >
        <RouterLink class="pb-leaderboard__row" :to="`/prompts/${entry.prompt_id}`">
          <span class="pb-leaderboard__rank" :class="{ 'is-top': entry.rank <= 3 }">
            {{ String(entry.rank).padStart(2, '0') }}
          </span>

          <span class="pb-leaderboard__main">
            <span class="pb-leaderboard__name">{{ entry.title }}</span>
            <span class="pb-leaderboard__sample">
              {{
                t('leaderboard.sample', {
                  models: entry.valid_model_count,
                  ratings: entry.total_rating_count,
                })
              }}
            </span>
          </span>

          <span
            class="pb-leaderboard__score"
            :title="t('leaderboard.computedAt', { time: formatTime(entry.computed_at) })"
          >
            <span class="pb-leaderboard__score-value">{{ entry.discrimination.toFixed(2) }}</span>
            <span class="pb-leaderboard__meter">
              <span
                class="pb-leaderboard__meter-fill"
                :style="{ width: scorePercent(entry.discrimination) }"
              />
            </span>
          </span>

          <span class="pb-leaderboard__confidence" :class="`is-${entry.confidence}`">
            {{ confidenceLabel(entry.confidence) }}
          </span>
        </RouterLink>
      </li>
    </ol>

    <p class="pb-leaderboard__note">{{ t('leaderboard.note') }}</p>
  </section>
</template>

<style scoped>
.pb-leaderboard {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-lg);
  padding: var(--pb-space-xl) 0 var(--pb-space-3xl);
}

.pb-leaderboard__hero {
  border-bottom: 1px solid var(--pb-line);
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
  padding-bottom: var(--pb-space-lg);
  position: relative;
}

.pb-leaderboard__hero::after {
  background: linear-gradient(90deg, var(--pb-accent), var(--pb-purpose), transparent);
  bottom: -1px;
  content: "";
  height: 2px;
  left: 0;
  position: absolute;
  width: 180px;
}

.pb-leaderboard__headline {
  align-items: baseline;
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-md);
  justify-content: space-between;
}

.pb-leaderboard__title {
  font-family: var(--pb-font-display);
  font-size: clamp(34px, 5vw, 48px);
  font-weight: 500;
  letter-spacing: -0.02em;
  line-height: 1.1;
  margin: 0;
}

.pb-leaderboard__auto {
  background: var(--pb-surface-muted);
  border-radius: var(--pb-radius-pill);
  color: var(--pb-ink-faint);
  font-size: 11.5px;
  padding: 3px 11px;
  white-space: nowrap;
}

.pb-leaderboard__lead {
  color: var(--pb-ink-soft);
  font-size: 15px;
  line-height: 1.8;
  margin: 0;
  max-width: 68ch;
}

.pb-leaderboard__meta {
  align-items: center;
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-md);
  margin-top: var(--pb-space-xs);
}

.pb-leaderboard__refreshed {
  color: var(--pb-ink-faint);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}

.pb-leaderboard__refresh {
  align-items: center;
  background: transparent;
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-pill);
  color: var(--pb-ink-soft);
  cursor: pointer;
  display: inline-flex;
  font-family: inherit;
  font-size: 12px;
  gap: 6px;
  padding: 5px 12px;
  transition:
    background-color var(--pb-duration) var(--pb-ease),
    border-color var(--pb-duration) var(--pb-ease),
    color var(--pb-duration) var(--pb-ease);
}

.pb-leaderboard__refresh:hover {
  background: var(--pb-surface-muted);
  border-color: var(--pb-line-strong);
  color: var(--pb-ink);
}

.pb-leaderboard__refresh.is-loading svg {
  animation: pb-leaderboard-spin 0.9s linear infinite;
}

.pb-leaderboard__tabs {
  display: inline-flex;
  gap: 2px;
  margin-top: var(--pb-space-sm);
  padding: 3px;
  background: var(--pb-surface-muted);
  border-radius: var(--pb-radius-pill);
  width: fit-content;
}

.pb-leaderboard__tab {
  background: transparent;
  border: 0;
  border-radius: var(--pb-radius-pill);
  color: var(--pb-ink-soft);
  cursor: pointer;
  font-family: inherit;
  font-size: 12.5px;
  padding: 6px 16px;
  transition:
    background-color var(--pb-duration) var(--pb-ease),
    color var(--pb-duration) var(--pb-ease),
    box-shadow var(--pb-duration) var(--pb-ease);
}

.pb-leaderboard__tab:hover {
  color: var(--pb-ink);
}

.pb-leaderboard__tab.is-active {
  background: var(--pb-surface);
  box-shadow: var(--pb-shadow-sm);
  color: var(--pb-ink);
}

.pb-leaderboard__list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  list-style: none;
  margin: 0;
  padding: 0;
  transition: opacity var(--pb-duration) var(--pb-ease);
}

.pb-leaderboard__list.is-refreshing {
  opacity: 0.55;
}

.pb-leaderboard__item {
  animation: pb-leaderboard-rise 460ms var(--pb-ease-out) both;
}

.pb-leaderboard__row {
  align-items: center;
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-md);
  display: grid;
  gap: var(--pb-space-md);
  grid-template-columns: 46px minmax(0, 1fr) 168px 92px;
  padding: 13px 18px;
  transition:
    background-color var(--pb-duration) var(--pb-ease),
    border-color var(--pb-duration) var(--pb-ease),
    box-shadow var(--pb-duration) var(--pb-ease),
    transform var(--pb-duration) var(--pb-ease);
}

.pb-leaderboard__row:hover {
  background: var(--pb-surface);
  border-color: var(--pb-line-strong);
  box-shadow: var(--pb-shadow-md);
  transform: translateY(-1px);
}

.pb-leaderboard__rank {
  color: var(--pb-ink-faint);
  font-family: var(--pb-font-display);
  font-size: 20px;
  font-variant-numeric: tabular-nums;
  line-height: 1;
}

.pb-leaderboard__rank.is-top {
  color: var(--pb-accent);
}

.pb-leaderboard__main {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}

.pb-leaderboard__name {
  color: var(--pb-ink);
  font-size: 14.5px;
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pb-leaderboard__sample {
  color: var(--pb-ink-faint);
  font-size: 11.5px;
}

.pb-leaderboard__score {
  display: flex;
  flex-direction: column;
  gap: 5px;
}

.pb-leaderboard__score-value {
  color: var(--pb-ink);
  font-family: var(--pb-font-display);
  font-size: 18px;
  font-variant-numeric: tabular-nums;
  line-height: 1;
}

.pb-leaderboard__meter {
  background: var(--pb-surface-sunken);
  border-radius: var(--pb-radius-pill);
  height: 3px;
  overflow: hidden;
  width: 100%;
}

.pb-leaderboard__meter-fill {
  background: linear-gradient(90deg, var(--pb-purpose), var(--pb-accent));
  border-radius: inherit;
  display: block;
  height: 100%;
  transform-origin: left center;
  animation: pb-leaderboard-grow 720ms var(--pb-ease-out) both;
}

.pb-leaderboard__confidence {
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-pill);
  font-size: 11px;
  padding: 3px 9px;
  text-align: center;
  white-space: nowrap;
}

.pb-leaderboard__confidence.is-high {
  background: var(--pb-group-execution-soft);
  border-color: transparent;
  color: var(--pb-group-execution-strong);
}

.pb-leaderboard__confidence.is-medium {
  background: var(--pb-group-reliability-soft);
  border-color: transparent;
  color: var(--pb-group-reliability-strong);
}

.pb-leaderboard__confidence.is-low {
  background: var(--pb-surface-muted);
  border-color: transparent;
  color: var(--pb-ink-faint);
}

.pb-leaderboard__note {
  color: var(--pb-ink-faint);
  font-size: 12px;
  margin: 0;
}

.pb-leaderboard__error {
  background: var(--pb-accent-soft);
  border-radius: var(--pb-radius-md);
  color: var(--pb-danger);
  font-size: 13.5px;
  margin: 0;
  padding: var(--pb-space-sm) var(--pb-space-md);
}

.pb-leaderboard__skeleton {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.pb-leaderboard__skeleton-row {
  animation: pb-leaderboard-pulse 1.3s ease-in-out infinite;
  background: var(--pb-surface-muted);
  border-radius: var(--pb-radius-md);
  height: 64px;
}

@keyframes pb-leaderboard-rise {
  from {
    opacity: 0;
    transform: translateY(8px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes pb-leaderboard-grow {
  from {
    transform: scaleX(0);
  }

  to {
    transform: scaleX(1);
  }
}

@keyframes pb-leaderboard-pulse {
  0%,
  100% {
    opacity: 0.5;
  }

  50% {
    opacity: 0.9;
  }
}

@keyframes pb-leaderboard-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 760px) {
  .pb-leaderboard__row {
    gap: var(--pb-space-sm);
    grid-template-columns: 36px minmax(0, 1fr) 84px;
    grid-template-areas:
      "rank main confidence"
      "rank score score";
    padding: 12px 14px;
  }

  .pb-leaderboard__rank {
    grid-area: rank;
    font-size: 17px;
  }

  .pb-leaderboard__main {
    grid-area: main;
  }

  .pb-leaderboard__score {
    grid-area: score;
  }

  .pb-leaderboard__confidence {
    grid-area: confidence;
    justify-self: end;
  }
}
</style>
