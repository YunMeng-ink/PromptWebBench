<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { type PromptDetail, promptApi } from '@/api/prompt'
import { type PromptModelRating, ratingApi } from '@/api/rating'
import { type Favorite, socialApi } from '@/api/social'
import { type PromptStats, statsApi } from '@/api/stats'
import CommentSection from '@/components/CommentSection.vue'
import ReportDialog from '@/components/ReportDialog.vue'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { useAuthStore } from '@/stores/auth'
import { confidenceKey, discriminationOf, formatScore, meanOf, modelLabel } from './promptStats'

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
const stats = ref<PromptStats | null>(null)
const statsFailure = ref('')
const loading = ref(true)
const failure = ref('')
const notice = ref('')
const copied = ref(false)

/** 收藏与举报。收藏需要先选收藏夹，因此把列表一并取回。 */
const favorited = ref(false)
const favoriteDialog = ref(false)
const favoriteList = ref<Favorite[]>([])
const favoriteNewName = ref('')
const favoriteNewVisibility = ref<'public' | 'private'>('private')
const reportVisible = ref(false)

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

/**
 * discriminationText 返回区分度的展示文案。
 * 数据不足时给出 C-06 规定的「数据不足，暂不计算区分度」，
 * 而不是一个 0 或一个空位。
 */
function discriminationText(window: PromptStats['history']): string {
  const value = discriminationOf(window)
  return value === null ? t('prompt.dataInsufficient') : value.toFixed(2)
}

/** confidenceText 把置信度等级翻译为当前语言的文案。 */
function confidenceText(level: PromptStats['history']['confidence']): string {
  return t(`prompt.confidence${confidenceKey(level)}`)
}

/** meanText 返回某个模型在窗口内的原始平均完成度。 */
function meanText(window: PromptStats['models'][number]['history']): string {
  return formatScore(meanOf(window))
}

/** tagText 返回推荐标签的文案，按当前语言取后端给出的双语之一。 */
function tagText(tag: PromptStats['recommendation'][number]): string {
  return locale.value === 'en' ? tag.label_en : tag.label_zh
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
    await reloadStats()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function deleteRating(item: PromptModelRating): Promise<void> {
  try {
    await ratingApi.remove(promptID.value as number, item.model_id)
    notice.value = t('prompt.ratingDeleted')
    await reloadRatings()
    await reloadStats()
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

/**
 * reloadStats 拉取测量结果。
 *
 * 统计计算失败与提示词本身无关，因此单独记录失败信息：正文、版本与评分列表
 * 照常展示，只有测量结果区显示不可用，不让整页打不开。
 */
async function reloadStats(): Promise<void> {
  if (promptID.value === null) {
    return
  }
  try {
    stats.value = await statsApi.byPrompt(promptID.value)
    statsFailure.value = ''
  } catch (error) {
    stats.value = null
    statsFailure.value = messageOf(error)
  }
}

/** openFavoriteDialog 打开收藏夹选择框并拉取当前用户的收藏夹。 */
async function openFavoriteDialog(): Promise<void> {
  if (promptID.value === null) {
    return
  }
  try {
    const result = await socialApi.myFavorites(1, 50)
    favoriteList.value = result.items
    favoriteNewName.value = ''
    // 每次打开都回到默认档位：沿用上一次的选择会让第二次创建静默地公开，
    // 而用户以为自己只是又建了一个默认（私有）的收藏夹。
    favoriteNewVisibility.value = 'private'
    favoriteDialog.value = true
  } catch (error) {
    failure.value = messageOf(error)
  }
}

/** addToFavorite 把当前提示词加入指定收藏夹。 */
async function addToFavorite(favorite: Favorite): Promise<void> {
  if (promptID.value === null) {
    return
  }
  try {
    await socialApi.addItem(favorite.id, promptID.value)
    favorited.value = true
    notice.value = t('prompt.favoriteAdded')
    favoriteDialog.value = false
  } catch (error) {
    failure.value = messageOf(error)
  }
}

/** createAndAdd 新建收藏夹后立即把当前提示词加入。 */
async function createAndAdd(): Promise<void> {
  if (favoriteNewName.value.trim() === '' || promptID.value === null) {
    return
  }
  try {
    const created = await socialApi.createFavorite({
      name: favoriteNewName.value,
      visibility: favoriteNewVisibility.value,
    })
    await socialApi.addItem(created.favorite.id, promptID.value)
    favorited.value = true
    notice.value = t('prompt.favoriteAdded')
    favoriteDialog.value = false
  } catch (error) {
    failure.value = messageOf(error)
  }
}

/** reloadFavorited 拉取当前提示词的收藏状态。 */
async function reloadFavorited(): Promise<void> {
  if (promptID.value === null || !auth.isAuthenticated) {
    favorited.value = false
    return
  }
  try {
    const result = await socialApi.isFavorited(promptID.value)
    favorited.value = result.favorited
  } catch {
    // 收藏状态读取失败不影响页面主体，静默保持未收藏。
    favorited.value = false
  }
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
  await reloadStats()
  await reloadFavorited()
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
          <el-button
            v-if="auth.isAuthenticated"
            size="small"
            :type="favorited ? 'primary' : 'default'"
            :plain="favorited"
            data-testid="detail-favorite"
            @click="openFavoriteDialog"
          >
            {{ t('prompt.favoriteAdd') }}
          </el-button>
          <el-button
            v-if="auth.isAuthenticated && !canEdit"
            size="small"
            data-testid="detail-report"
            @click="reportVisible = true"
          >
            {{ t('prompt.reportAction') }}
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

      <section class="pb-stats" data-testid="stats-section">
        <h2 class="pb-stats__title">{{ t('prompt.statsSection') }}</h2>
        <p class="pb-stats__hint">{{ t('prompt.statsHint') }}</p>

        <p v-if="statsFailure" class="pb-prompt-detail__error" data-testid="stats-error">
          {{ t('prompt.statsUnavailable') }}
        </p>

        <template v-else-if="stats">
          <ul v-if="stats.recommendation.length > 0" class="pb-stats__tags" data-testid="stats-tags">
            <li v-for="tag in stats.recommendation" :key="tag.id" class="pb-stats__tag">
              <span class="pb-stats__tag-label" :data-testid="`stats-tag-${tag.kind}`">
                {{ tagText(tag) }}
              </span>
              <span class="pb-stats__tag-confidence">
                {{ t('prompt.confidence') }}：{{ confidenceText(tag.confidence) }}
              </span>
              <span class="pb-stats__tag-reason">{{ t('prompt.tagReason') }}：{{ tag.reason }}</span>
            </li>
          </ul>

          <h3 class="pb-stats__subtitle">{{ t('prompt.discriminationSection') }}</h3>
          <dl class="pb-stats__discrimination">
            <div>
              <dt>{{ t('prompt.historyDiscrimination') }}</dt>
              <dd data-testid="stats-history-discrimination">
                {{ discriminationText(stats.history) }}
              </dd>
            </div>
            <div>
              <dt>{{ t('prompt.recentDiscrimination') }}</dt>
              <dd data-testid="stats-recent-discrimination">
                {{ discriminationText(stats.recent) }}
              </dd>
            </div>
            <div>
              <dt>{{ t('prompt.discriminationConfidence') }}</dt>
              <dd data-testid="stats-discrimination-confidence">
                {{ confidenceText(stats.recent.confidence) }}
              </dd>
            </div>
            <div>
              <dt>{{ t('prompt.validModelCount') }}</dt>
              <dd data-testid="stats-valid-models">{{ stats.recent.valid_model_count }}</dd>
            </div>
            <div>
              <dt>{{ t('prompt.totalRatingCount') }}</dt>
              <dd data-testid="stats-total-ratings">{{ stats.recent.total_rating_count }}</dd>
            </div>
          </dl>

          <h3 class="pb-stats__subtitle">{{ t('prompt.modelCompletion') }}</h3>
          <table class="pb-stats__table" data-testid="stats-models">
            <thead>
              <tr>
                <th scope="col">{{ t('prompt.model') }}</th>
                <th scope="col">{{ t('prompt.historyMean') }}</th>
                <th scope="col">{{ t('prompt.recentMean') }}</th>
                <th scope="col">{{ t('prompt.historyRatingCount') }}</th>
                <th scope="col">{{ t('prompt.recentRatingCount') }}</th>
                <th scope="col">{{ t('prompt.adjustedScore') }}</th>
                <th scope="col">{{ t('prompt.userCount') }}</th>
                <th scope="col">{{ t('prompt.confidence') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="model in stats.models" :key="model.model_id">
                <td :data-testid="`stats-model-${model.model_id}`">
                  {{ modelLabel(model.model_name, model.model_id) }}
                </td>
                <td>{{ meanText(model.history) }}</td>
                <td>{{ meanText(model.recent) }}</td>
                <td>{{ model.history.rating_count }}</td>
                <td>{{ model.recent.rating_count }}</td>
                <td>{{ model.recent.adjusted.toFixed(2) }}</td>
                <td>{{ model.recent.user_count }}</td>
                <td>
                  {{
                    model.recent.has_data
                      ? confidenceText(model.recent.confidence)
                      : t('prompt.noObservation')
                  }}
                </td>
              </tr>
            </tbody>
          </table>
        </template>
      </section>

      <CommentSection v-if="promptID !== null" :prompt-id="promptID" />

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

    <el-dialog
      v-if="promptID !== null"
      v-model="favoriteDialog"
      :title="t('prompt.favoriteSelectTitle')"
      width="420px"
    >
      <p v-if="favoriteList.length === 0" data-testid="favorite-dialog-empty">
        {{ t('prompt.favoriteEmpty') }}
      </p>
      <ul v-else class="pb-prompt-detail__favorites" data-testid="favorite-dialog-list">
        <li v-for="favorite in favoriteList" :key="favorite.id">
          <el-button
            size="small"
            text
            :data-testid="`favorite-choice-${favorite.id}`"
            @click="addToFavorite(favorite)"
          >
            {{ favorite.name }}
          </el-button>
        </li>
      </ul>

      <el-divider />

      <el-form label-width="90px">
        <el-form-item :label="t('prompt.favoriteCreateHint')">
          <el-input v-model="favoriteNewName" maxlength="100" data-testid="favorite-new-name" />
        </el-form-item>
        <el-form-item :label="t('prompt.favoriteVisibility')">
          <el-radio-group v-model="favoriteNewVisibility" data-testid="favorite-visibility">
            <el-radio value="public">{{ t('prompt.favoritePublic') }}</el-radio>
            <el-radio value="private">{{ t('prompt.favoritePrivate') }}</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button data-testid="favorite-dialog-cancel" @click="favoriteDialog = false">
          {{ t('auth.cancel') }}
        </el-button>
        <el-button type="primary" data-testid="favorite-dialog-create" @click="createAndAdd">
          {{ t('prompt.favoriteCreate') }}
        </el-button>
      </template>
    </el-dialog>

    <ReportDialog
      v-if="promptID !== null"
      v-model:visible="reportVisible"
      object-type="prompt"
      :object-id="promptID"
      @submitted="notice = t('prompt.reportSubmitted')"
    />
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

.pb-stats__title {
  font-size: 18px;
  margin: var(--pb-space-lg) 0 var(--pb-space-xs);
}

.pb-stats__hint {
  color: var(--pb-color-text-faint);
  font-size: 13px;
  margin: 0 0 var(--pb-space-sm);
}

.pb-stats__subtitle {
  font-size: 15px;
  margin: var(--pb-space-md) 0 var(--pb-space-xs);
}

.pb-stats__discrimination {
  display: grid;
  gap: var(--pb-space-sm);
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  margin: 0;
}

.pb-stats__discrimination dt {
  color: var(--pb-color-text-faint);
  font-size: 12px;
}

.pb-stats__discrimination dd {
  margin: 2px 0 0;
}

.pb-stats__tags {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-xs);
  list-style: none;
  margin: 0 0 var(--pb-space-sm);
  padding: 0;
}

.pb-stats__tag {
  align-items: baseline;
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-sm);
}

.pb-stats__tag-label {
  background-color: var(--el-color-primary-light-9);
  border-radius: var(--pb-radius-sm);
  color: var(--el-color-primary-dark-2);
  font-weight: 600;
  padding: 2px var(--pb-space-xs);
}

.pb-stats__tag-confidence,
.pb-stats__tag-reason {
  color: var(--pb-color-text-soft);
  font-size: 13px;
}

.pb-stats__table {
  border-collapse: collapse;
  font-size: 13px;
  width: 100%;
}

.pb-stats__table th,
.pb-stats__table td {
  border-bottom: 1px solid var(--pb-color-border);
  padding: var(--pb-space-xs) var(--pb-space-sm);
  text-align: left;
}

.pb-stats__table th {
  color: var(--pb-color-text-faint);
  font-weight: 500;
}

.pb-prompt-detail__favorites {
  list-style: none;
  margin: 0;
  padding: 0;
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