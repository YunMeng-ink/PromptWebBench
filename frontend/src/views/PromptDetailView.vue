<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { type PromptDetail, promptApi } from '@/api/prompt'
import { type PromptModelRating, ratingApi } from '@/api/rating'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const { t, locale } = useI18n()
const auth = useAuthStore()
const { messageOf } = useErrorMessage()

const promptID = computed(() => {
  const parsed = Number.parseInt(String(route.params.id ?? ''), 10)
  return Number.isNaN(parsed) ? null : parsed
})

const detail = ref<PromptDetail | null>(null)
const ratings = ref<PromptModelRating[]>([])
const loading = ref(true)
const failure = ref('')
const notice = ref('')
const copied = ref(false)

/** pendingScore 是当前正在评分的模型与其分值，未评分时 model 为空。 */
const pendingScore = ref<number | undefined>(undefined)
const pendingModel = ref<number | undefined>(undefined)

/** 编辑已有评分时按钮文案与提交动作不同，由评分来源决定。 */
const editingExisting = ref(false)

const dialogVisible = computed({
  get: () => pendingModel.value !== undefined,
  set: (value: boolean) => {
    if (!value) {
      cancelRating()
    }
  },
})

const scores = [1, 2, 3, 4, 5] as const

function localized(zh: string, en: string): string {
  return locale.value === 'en' ? en : zh
}

const canEdit = computed(
  () =>
    detail.value !== null &&
    auth.isAuthenticated &&
    (auth.user?.id === detail.value.prompt.uploader_id || auth.hasRole('admin')),
)

/** remainingEdits 返回该模型评分剩余可修改次数；已评满时为 0。 */
function remainingEdits(item: PromptModelRating, maxEdits: number): number {
  return Math.max(0, maxEdits - (item.my_edit_count ?? 0))
}

const maxEdits = ref(3)

function startRating(item: PromptModelRating): void {
  pendingModel.value = item.model_id
  pendingScore.value = item.my_score
  editingExisting.value = item.my_score !== undefined
  notice.value = ''
  failure.value = ''
}

function cancelRating(): void {
  pendingModel.value = undefined
  pendingScore.value = undefined
  editingExisting.value = false
}

/**
 * confirmRating 按评分来源选择提交或修改。
 * 已存在评分时走修改路径，受单条修改次数上限约束；
 * 首次评分走提交路径，两者在后端是不同端点。
 */
async function confirmRating(): Promise<void> {
  if (pendingModel.value === undefined || pendingScore.value === undefined) {
    return
  }
  const payload = { model_id: pendingModel.value, score: pendingScore.value }
  try {
    if (editingExisting.value) {
      await ratingApi.update(promptID.value as number, payload)
      notice.value = t('prompt.ratingUpdated')
    } else {
      await ratingApi.submit(promptID.value as number, payload)
      notice.value = t('prompt.ratingSubmitted')
    }
    cancelRating()
    await reloadRatings()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function deleteRating(item: PromptModelRating): Promise<void> {
  try {
    await ratingApi.remove(promptID.value as number, item.model_id)
    notice.value = t('prompt.ratingDeleted')
    await reloadRatings()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function copyBody(): Promise<void> {
  if (detail.value === null) {
    return
  }
  try {
    await navigator.clipboard.writeText(detail.value.version.body)
    copied.value = true
  } catch {
    copied.value = false
    notice.value = detail.value.version.body
  }
}

async function removePrompt(): Promise<void> {
  if (promptID.value === null) {
    return
  }
  try {
    await promptApi.remove(promptID.value)
    await router.push('/')
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function reloadRatings(): Promise<void> {
  if (promptID.value === null) {
    return
  }
  const result = await ratingApi.byPrompt(promptID.value)
  ratings.value = result.items
}

onMounted(async () => {
  await auth.connect()
  if (promptID.value === null) {
    loading.value = false
    failure.value = t('prompt.notFound')
    return
  }

  try {
    detail.value = await promptApi.detail(promptID.value)
    await reloadRatings()
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <section class="pb-prompt-detail">
    <p v-if="loading" class="pb-prompt-detail__hint">{{ t('home.loadFailed') }}</p>

    <template v-else-if="detail">
      <div class="pb-prompt-detail__head">
        <h1 class="pb-prompt-detail__title" data-testid="detail-title">
          {{ detail.version.title }}
        </h1>
        <div class="pb-prompt-detail__actions">
          <el-button size="small" data-testid="detail-copy" @click="copyBody">
            {{ copied ? t('home.copied') : t('home.copyBody') }}
          </el-button>
          <RouterLink v-if="canEdit" :to="`/prompts/${detail.prompt.id}/edit`">
            <el-button size="small" data-testid="detail-edit">{{ t('prompt.editTitle') }}</el-button>
          </RouterLink>
          <el-button
            v-if="canEdit"
            size="small"
            type="danger"
            plain
            data-testid="detail-delete"
            @click="removePrompt"
          >
            {{ t('prompt.delete') }}
          </el-button>
        </div>
      </div>

      <p v-if="detail.major_modification_notice" class="pb-prompt-detail__warn" data-testid="detail-major-notice">
        {{ detail.major_modification_notice }}
      </p>

      <dl class="pb-prompt-detail__meta">
        <div>
          <dt>{{ t('prompt.version') }}</dt>
          <dd data-testid="detail-version">{{ detail.prompt.version_no }}</dd>
        </div>
        <div>
          <dt>{{ t('prompt.category') }}</dt>
          <dd data-testid="detail-category">
            {{ localized(detail.category.name_zh, detail.category.name_en) }}
          </dd>
        </div>
        <div>
          <dt>{{ t('prompt.uploader') }}</dt>
          <dd data-testid="detail-uploader">{{ detail.uploader }}</dd>
        </div>
        <div>
          <dt>{{ t('prompt.visibility') }}</dt>
          <dd data-testid="detail-visibility">
            {{ t(`prompt.visibility${detail.prompt.visibility.charAt(0).toUpperCase()}${detail.prompt.visibility.slice(1)}`) }}
          </dd>
        </div>
        <div>
          <dt>{{ t('prompt.sensitiveLevel') }}</dt>
          <dd>{{ t(`prompt.sensitive${detail.prompt.sensitive_level.charAt(0).toUpperCase()}${detail.prompt.sensitive_level.slice(1)}`) }}</dd>
        </div>
        <div>
          <dt>{{ t('prompt.updatedAt') }}</dt>
          <dd>{{ detail.prompt.updated_at }}</dd>
        </div>
      </dl>

      <p v-if="detail.version.description" class="pb-prompt-detail__description">
        {{ detail.version.description }}
      </p>

      <pre class="pb-prompt-detail__body" data-testid="detail-body">{{ detail.version.body }}</pre>

      <section class="pb-ratings">
        <h2 class="pb-ratings__title">{{ t('prompt.ratingSection') }}</h2>
        <p class="pb-ratings__hint">{{ t('prompt.ratingHint') }}</p>

        <p v-if="ratings.length === 0" data-testid="ratings-empty">{{ t('prompt.ratingEmpty') }}</p>

        <ul v-else class="pb-ratings__list" data-testid="ratings-list">
          <li v-for="item in ratings" :key="item.model_id" class="pb-ratings__item">
            <div class="pb-ratings__row">
              <span class="pb-ratings__model">{{ item.model_name }}</span>
              <span class="pb-ratings__count" :data-testid="`rating-count-${item.model_id}`">
                {{ t('prompt.ratingCount', { count: item.rating_count }) }}
              </span>
              <span class="pb-ratings__mean" :data-testid="`rating-mean-${item.model_id}`">
                {{ item.rating_count > 0 ? item.mean_score.toFixed(2) : '—' }}
              </span>
              <span class="pb-ratings__mine" :data-testid="`rating-mine-${item.model_id}`">
                {{ item.my_score ?? '—' }}
              </span>
            </div>

            <div v-if="auth.isAuthenticated" class="pb-ratings__actions">
              <el-button
                size="small"
                text
                :data-testid="`rating-start-${item.model_id}`"
                @click="startRating(item)"
              >
                {{ item.my_score === undefined ? t('prompt.submitRating') : t('prompt.updateRating') }}
              </el-button>
              <el-button
                v-if="item.my_score !== undefined"
                size="small"
                text
                :data-testid="`rating-delete-${item.model_id}`"
                @click="deleteRating(item)"
              >
                {{ t('prompt.deleteRating') }}
              </el-button>
              <span
                v-if="item.my_score !== undefined"
                class="pb-ratings__edits"
                :data-testid="`rating-edits-${item.model_id}`"
              >
                {{
                  remainingEdits(item, maxEdits) > 0
                    ? t('prompt.editsLeft', { count: remainingEdits(item, maxEdits) })
                    : t('prompt.editsExhausted')
                }}
              </span>
            </div>
          </li>
        </ul>

        <el-dialog
          v-model="dialogVisible"
          data-testid="rating-dialog"
          :title="t('prompt.myRating')"
          width="420px"
        >
          <el-radio-group v-model="pendingScore" data-testid="rating-score">
            <el-radio
              v-for="score in scores"
              :key="score"
              :value="score"
              :data-testid="`rating-score-${score}`"
            >
              {{ score }} · {{ t(`prompt.score${score}`) }}
            </el-radio>
          </el-radio-group>

          <template #footer>
            <el-button data-testid="rating-cancel" @click="cancelRating">
              {{ t('auth.cancel') }}
            </el-button>
            <el-button type="primary" data-testid="rating-confirm" @click="confirmRating">
              {{ editingExisting ? t('prompt.updateRating') : t('prompt.submitRating') }}
            </el-button>
          </template>
        </el-dialog>
      </section>

      <section v-if="detail.versions.length > 1" class="pb-versions">
        <h2 class="pb-versions__title">{{ t('prompt.versions') }}</h2>
        <ul class="pb-versions__list">
          <li v-for="version in detail.versions" :key="version.id" class="pb-versions__item">
            <span class="pb-versions__no">{{ version.version_no }}</span>
            <span class="pb-versions__note">{{ version.change_note }}</span>
            <span v-if="version.is_major" class="pb-versions__major">{{ t('prompt.majorChange') }}</span>
            <span class="pb-versions__date">{{ version.created_at }}</span>
          </li>
        </ul>
      </section>
    </template>

    <p v-else class="pb-prompt-detail__error" data-testid="detail-error">
      {{ failure || t('prompt.notFound') }}
    </p>

    <p v-if="notice" class="pb-prompt-detail__notice" data-testid="detail-notice">{{ notice }}</p>
    <p v-if="failure && detail" class="pb-prompt-detail__error" data-testid="detail-action-error">
      {{ failure }}
    </p>
  </section>
</template>

<style scoped>
.pb-prompt-detail {
  padding: var(--pb-space-lg) 0;
}

.pb-prompt-detail__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--pb-space-md);
}

.pb-prompt-detail__title {
  font-size: 24px;
  margin: 0;
}

.pb-prompt-detail__actions {
  display: flex;
  gap: var(--pb-space-xs);
  flex-shrink: 0;
}

.pb-prompt-detail__warn {
  background-color: var(--el-color-warning-light-9);
  border-radius: var(--pb-radius-sm);
  color: var(--el-color-warning-dark-2);
  margin: var(--pb-space-md) 0 0;
  padding: var(--pb-space-sm);
}

.pb-prompt-detail__meta {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: var(--pb-space-sm);
  margin: var(--pb-space-md) 0;
}

.pb-prompt-detail__meta dt {
  color: var(--pb-color-text-faint);
  font-size: 12px;
}

.pb-prompt-detail__meta dd {
  margin: 2px 0 0;
}

.pb-prompt-detail__description {
  color: var(--pb-color-text-soft);
}

.pb-prompt-detail__body {
  background-color: var(--pb-color-surface-muted);
  border-radius: var(--pb-radius-md);
  font-family: var(--pb-font-mono);
  margin: var(--pb-space-md) 0;
  overflow-x: auto;
  padding: var(--pb-space-md);
  white-space: pre-wrap;
  word-break: break-word;
}

.pb-ratings__title,
.pb-versions__title {
  font-size: 18px;
  margin: var(--pb-space-lg) 0 var(--pb-space-xs);
}

.pb-ratings__hint {
  color: var(--pb-color-text-faint);
  font-size: 13px;
  margin: 0 0 var(--pb-space-sm);
}

.pb-ratings__list,
.pb-versions__list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.pb-ratings__item {
  border-top: 1px solid var(--pb-color-border);
  padding: var(--pb-space-sm) 0;
}

.pb-ratings__row {
  display: flex;
  align-items: center;
  gap: var(--pb-space-md);
}

.pb-ratings__model {
  font-weight: 600;
  min-width: 180px;
}

.pb-ratings__count,
.pb-ratings__mean,
.pb-ratings__mine {
  color: var(--pb-color-text-soft);
  font-size: 13px;
}

.pb-ratings__actions {
  align-items: center;
  display: flex;
  gap: var(--pb-space-xs);
  margin-top: var(--pb-space-xs);
}

.pb-ratings__edits {
  color: var(--pb-color-text-faint);
  font-size: 12px;
}

.pb-versions__item {
  border-top: 1px solid var(--pb-color-border);
  display: flex;
  gap: var(--pb-space-md);
  padding: var(--pb-space-xs) 0;
}

.pb-versions__no {
  font-weight: 600;
  min-width: 56px;
}

.pb-versions__major {
  color: var(--el-color-warning);
  font-size: 12px;
}

.pb-versions__date {
  color: var(--pb-color-text-faint);
  font-size: 12px;
  margin-left: auto;
}

.pb-prompt-detail__error {
  color: var(--el-color-danger);
}

.pb-prompt-detail__notice {
  color: var(--pb-color-text-soft);
  word-break: break-all;
}

.pb-prompt-detail__hint {
  color: var(--pb-color-text-soft);
}
</style>