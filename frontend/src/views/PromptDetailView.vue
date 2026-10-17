<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { type PromptDetail, promptApi } from '@/api/prompt'
import { type PromptModelRating, ratingApi } from '@/api/rating'
import { type Favorite, socialApi } from '@/api/social'
import { type PromptStats, statsApi } from '@/api/stats'
import CommentSection from '@/components/CommentSection.vue'
import ReportDialog from '@/components/ReportDialog.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { useAuthStore } from '@/stores/auth'
import { type CopyLabels, promptBodyText, promptFullText } from './libraryText'
import { confidenceKey, discriminationOf, formatScore, meanOf, modelLabel } from './promptStats'

const route = useRoute()
const router = useRouter()
const { t, locale } = useI18n()
const auth = useAuthStore()
const { messageOf } = useErrorMessage()
const { copied, copy } = useClipboard()

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

/** groupClass 把考察维度的分组映射为详情页的强调色类名。 */
const groupClass = computed(() => {
  const group = detail.value?.capability.group_key ?? 'cognitive'
  return `pb-prompt-detail--${group}`
})

/** meterStyle 把 1～5 分的取值映射为量表条宽度。 */
function meterStyle(value: number): Record<string, string> {
  const clamped = Math.min(5, Math.max(0, value))
  return { '--pb-meter': `${(clamped / 5) * 100}%` }
}

/** discriminationMeter 在数据充足时给出区分度量表条样式。 */
function discriminationMeter(window: PromptStats['history']): Record<string, string> | null {
  const value = discriminationOf(window)
  return value === null ? null : meterStyle(value)
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

/** copyLabels 是复制文本里的字段名，全部取自语言包。 */
const copyLabels = computed<CopyLabels>(() => ({
  title: t('library.fieldTitle'),
  purpose: t('library.fieldPurpose'),
  capability: t('library.fieldCapability'),
  version: t('library.fieldVersion'),
  tags: t('library.fieldTags'),
  description: t('library.fieldDescription'),
  kind: t('library.fieldKind'),
  source: t('library.fieldSource'),
  body: t('library.fieldBody'),
}))

/** copyBody 复制提示词正文（需求 §10.8「复制提示词」）。 */
async function copyBody(): Promise<void> {
  if (detail.value === null) {
    return
  }
  await copy(promptBodyText(detail.value.version.body), t('home.copied'))
}

/** copyFull 复制带字段名的完整信息（需求 §10.8「复制完整信息」）。 */
async function copyFull(): Promise<void> {
  if (detail.value === null) {
    return
  }
  const current = detail.value
  const purposeName = locale.value === 'en' ? current.purpose.name_en : current.purpose.name_zh
  const capabilityName =
    locale.value === 'en' ? current.capability.name_en : current.capability.name_zh
  await copy(
    promptFullText(
      {
        title: current.version.title,
        purpose: purposeName,
        capability: capabilityName,
        versionNo: current.version.version_no,
        tags: current.tags.map((tag) => (locale.value === 'en' ? tag.name_en : tag.name_zh)),
        description: current.version.description,
        body: current.version.body,
      },
      copyLabels.value,
    ),
    t('home.copied'),
  )
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

/** loadDetail 加载当前路由参数对应的提示词详情与全部附属数据。 */
async function loadDetail(): Promise<void> {
  loading.value = true
  failure.value = ''
  detail.value = null
  ratings.value = []
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
}

onMounted(async () => {
  await auth.connect()
  await loadDetail()
})

// 同路由参数切换（前进/后退或站内跳转）时组件复用，onMounted 不会再执行：
// 监视参数变化并整体重载，防止内容、评分与操作对象错位。
watch(promptID, () => {
  void loadDetail()
})
</script>

<template>
  <section class="pb-prompt-detail" :class="groupClass">
    <p v-if="loading" class="pb-prompt-detail__hint">{{ t('home.loadFailed') }}</p>

    <template v-else-if="detail">
      <div class="pb-prompt-detail__head">
        <span class="pb-prompt-detail__halo" aria-hidden="true" />
        <div class="pb-prompt-detail__headline">
          <div class="pb-prompt-detail__chips">
            <span class="pb-prompt-detail__chip pb-prompt-detail__chip--purpose">
              <span class="pb-axis-mark pb-axis-mark--purpose" aria-hidden="true" />
              {{ t('dimension.purpose') }} ·
              {{ localized(detail.purpose.name_zh, detail.purpose.name_en) }}
            </span>
            <span class="pb-prompt-detail__chip pb-prompt-detail__chip--capability">
              <span class="pb-axis-mark" aria-hidden="true" />
              {{ t('dimension.capability') }} ·
              {{ localized(detail.capability.name_zh, detail.capability.name_en) }}
            </span>
            <span v-if="detail.difficulty" class="pb-prompt-detail__chip">
              {{ t('dimension.difficulty') }} ·
              {{ localized(detail.difficulty.name_zh, detail.difficulty.name_en) }}
            </span>
          </div>
          <h1 class="pb-prompt-detail__title" data-testid="detail-title">
            {{ detail.version.title }}
          </h1>
        </div>
        <div class="pb-prompt-detail__actions">
          <el-button size="small" data-testid="detail-copy" @click="copyBody">
            {{ t('home.copyBody') }}
          </el-button>
          <el-button size="small" data-testid="detail-copy-full" @click="copyFull">
            {{ t('home.copyFull') }}
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
          <dt>{{ t('prompt.purpose') }}</dt>
          <dd data-testid="detail-purpose">
            {{ localized(detail.purpose.name_zh, detail.purpose.name_en) }}
          </dd>
        </div>
        <div>
          <dt>{{ t('prompt.capability') }}</dt>
          <dd data-testid="detail-capability">
            {{ localized(detail.capability.name_zh, detail.capability.name_en) }}
          </dd>
        </div>
        <div v-if="detail.difficulty">
          <dt>{{ t('prompt.difficulty') }}</dt>
          <dd data-testid="detail-difficulty">
            {{ localized(detail.difficulty.name_zh, detail.difficulty.name_en) }}
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

      <div class="pb-prompt-detail__manuscript">
        <div class="pb-prompt-detail__manuscript-head">
          <span>Prompt · {{ detail.prompt.version_no }}</span>
          <span>{{ detail.prompt.updated_at }}</span>
        </div>
        <pre class="pb-prompt-detail__body" data-testid="detail-body">{{ detail.version.body }}</pre>
      </div>

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
        <p class="pb-stats__hint">
          {{ t('prompt.statsHint') }}
          <RouterLink class="pb-stats__method-link" to="/methodology" data-testid="stats-methodology">
            {{ t('prompt.methodologyLink') }}
          </RouterLink>
        </p>
        <p v-if="stats" class="pb-stats__computed" data-testid="stats-computed-at">
          {{ t('prompt.statsComputedAt') }} · {{ stats.computed_at }}
        </p>

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
            <div class="pb-stats__tile">
              <dt>{{ t('prompt.historyDiscrimination') }}</dt>
              <dd data-testid="stats-history-discrimination">
                {{ discriminationText(stats.history) }}
              </dd>
              <span
                v-if="discriminationMeter(stats.history)"
                class="pb-meter pb-meter--discrimination"
                aria-hidden="true"
              >
                <span class="pb-meter__fill" :style="discriminationMeter(stats.history) ?? {}" />
              </span>
            </div>
            <div class="pb-stats__tile">
              <dt>{{ t('prompt.recentDiscrimination') }}</dt>
              <dd data-testid="stats-recent-discrimination">
                {{ discriminationText(stats.recent) }}
              </dd>
              <span
                v-if="discriminationMeter(stats.recent)"
                class="pb-meter pb-meter--discrimination"
                aria-hidden="true"
              >
                <span class="pb-meter__fill" :style="discriminationMeter(stats.recent) ?? {}" />
              </span>
            </div>
            <div class="pb-stats__tile">
              <dt>{{ t('prompt.discriminationConfidence') }}</dt>
              <dd data-testid="stats-discrimination-confidence">
                {{ confidenceText(stats.recent.confidence) }}
              </dd>
            </div>
            <div class="pb-stats__tile">
              <dt>{{ t('prompt.validModelCount') }}</dt>
              <dd data-testid="stats-valid-models">{{ stats.recent.valid_model_count }}</dd>
            </div>
            <div class="pb-stats__tile">
              <dt>{{ t('prompt.totalRatingCount') }}</dt>
              <dd data-testid="stats-total-ratings">{{ stats.recent.total_rating_count }}</dd>
            </div>
          </dl>

          <h3 class="pb-stats__subtitle">{{ t('prompt.modelCompletion') }}</h3>
          <div class="pb-stats__table-wrap">
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
          </div>
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

    <p v-if="copied" class="pb-prompt-detail__notice" data-testid="detail-copied">{{ copied }}</p>
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
  --pb-detail-group: var(--pb-group-cognitive);
  --pb-detail-group-strong: var(--pb-group-cognitive-strong);
  --pb-detail-group-soft: var(--pb-group-cognitive-soft);
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-lg);
  padding: var(--pb-space-xl) 0 var(--pb-space-2xl);
}

.pb-prompt-detail--execution {
  --pb-detail-group: var(--pb-group-execution);
  --pb-detail-group-strong: var(--pb-group-execution-strong);
  --pb-detail-group-soft: var(--pb-group-execution-soft);
}

.pb-prompt-detail--reliability {
  --pb-detail-group: var(--pb-group-reliability);
  --pb-detail-group-strong: var(--pb-group-reliability-strong);
  --pb-detail-group-soft: var(--pb-group-reliability-soft);
}

.pb-prompt-detail__head {
  position: relative;
  overflow: hidden;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--pb-space-lg);
  padding: var(--pb-space-xl);
  background:
    radial-gradient(ellipse 70% 120% at 100% 0%, var(--pb-detail-group-soft), transparent 64%),
    var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-xl);
  box-shadow:
    var(--pb-shadow-sm),
    var(--pb-shadow-inset);
}

.pb-prompt-detail__halo {
  position: absolute;
  right: -120px;
  top: -150px;
  width: 340px;
  height: 340px;
  border-radius: 50%;
  border: 1px solid var(--pb-detail-group);
  opacity: 0.16;
  pointer-events: none;
}

.pb-prompt-detail__halo::before,
.pb-prompt-detail__halo::after {
  content: "";
  position: absolute;
  border-radius: 50%;
  border: 1px solid var(--pb-detail-group);
}

.pb-prompt-detail__halo::before {
  inset: 42px;
  opacity: 0.7;
}

.pb-prompt-detail__halo::after {
  inset: 88px;
  opacity: 0.45;
}

.pb-prompt-detail__headline {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
  min-width: 0;
}

.pb-prompt-detail__chips {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--pb-space-xs);
}

.pb-prompt-detail__chip {
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

.pb-prompt-detail__chip--purpose {
  border: 1px solid var(--pb-purpose-ring);
  background: var(--pb-purpose-soft);
  color: var(--pb-purpose-strong);
}

.pb-prompt-detail__chip--capability {
  background: var(--pb-detail-group-soft);
  color: var(--pb-detail-group-strong);
}

.pb-prompt-detail__chip--capability .pb-axis-mark {
  background: var(--pb-detail-group);
}

.pb-prompt-detail__title {
  margin: 0;
  font-family: var(--pb-font-display);
  font-size: clamp(30px, 4.2vw, 42px);
  font-weight: 500;
  letter-spacing: -0.018em;
  line-height: 1.15;
  text-wrap: balance;
}

.pb-prompt-detail__actions {
  position: relative;
  display: flex;
  gap: var(--pb-space-xs);
  flex-wrap: wrap;
  justify-content: flex-end;
  max-width: 420px;
}

/*
 * 窄屏下标题与操作区分行，操作按钮折行排布：
 * 横排的按钮组在 375px 下放不下，折行是唯一不隐藏任何入口的排布方式。
 */
@media (max-width: 760px) {
  .pb-prompt-detail__head {
    flex-direction: column;
    padding: var(--pb-space-lg);
  }

  .pb-prompt-detail__actions {
    justify-content: flex-start;
    width: 100%;
    max-width: none;
  }
}

.pb-prompt-detail__warn {
  margin: 0;
  padding: var(--pb-space-sm) var(--pb-space-md);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-md);
  background: var(--el-color-warning-light-9);
  color: var(--pb-warning);
  font-size: 13.5px;
}

.pb-prompt-detail__meta {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: var(--pb-space-md) var(--pb-space-xl);
  margin: 0;
  padding: var(--pb-space-lg) var(--pb-space-xl);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
}

.pb-prompt-detail__meta dt {
  color: var(--pb-ink-faint);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.pb-prompt-detail__meta dd {
  margin: 5px 0 0;
  color: var(--pb-ink);
  font-size: 14px;
  font-variant-numeric: tabular-nums;
}

.pb-prompt-detail__description {
  max-width: 72ch;
  margin: 0;
  padding-left: var(--pb-space-md);
  border-left: 2px solid var(--pb-detail-group);
  color: var(--pb-ink-soft);
  font-size: 15px;
  line-height: 1.8;
}

.pb-prompt-detail__manuscript {
  overflow: hidden;
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
  box-shadow:
    var(--pb-shadow-sm),
    var(--pb-shadow-inset);
}

.pb-prompt-detail__manuscript-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--pb-space-md);
  padding: 9px var(--pb-space-xl);
  border-bottom: 1px solid var(--pb-line);
  background: var(--pb-surface-muted);
  color: var(--pb-ink-faint);
  font-family: var(--pb-font-mono);
  font-size: 10.5px;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.pb-prompt-detail__body {
  margin: 0;
  padding: var(--pb-space-xl);
  border-left: 3px solid var(--pb-detail-group);
  color: var(--pb-ink);
  font-family: var(--pb-font-mono);
  font-size: 13.5px;
  line-height: 1.9;
  overflow-x: auto;
  white-space: pre-wrap;
  word-break: break-word;
  background:
    linear-gradient(180deg, var(--pb-surface) 0%, color-mix(in srgb, var(--pb-surface) 88%, var(--pb-surface-muted)) 100%);
}

.pb-ratings,
.pb-stats,
.pb-versions {
  margin: 0;
  padding: var(--pb-space-xl);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
  box-shadow:
    var(--pb-shadow-sm),
    var(--pb-shadow-inset);
}

.pb-ratings__title,
.pb-versions__title,
.pb-stats__title {
  display: flex;
  align-items: center;
  gap: 10px;
  margin: 0 0 var(--pb-space-xs);
  font-family: var(--pb-font-display);
  font-size: 23px;
  font-weight: 500;
  letter-spacing: -0.01em;
}

.pb-ratings__title::before,
.pb-versions__title::before,
.pb-stats__title::before {
  content: "";
  width: 7px;
  height: 7px;
  border-radius: 2px;
  background: var(--pb-detail-group);
  transform: rotate(45deg);
}

.pb-ratings__hint,
.pb-stats__hint {
  margin: 0 0 var(--pb-space-md);
  color: var(--pb-ink-faint);
  font-size: 13px;
}

.pb-stats__method-link {
  margin-left: 6px;
  color: var(--pb-accent);
  border-bottom: 1px solid var(--pb-accent-ring);
  transition: border-color var(--pb-duration) var(--pb-ease);
}

.pb-stats__method-link:hover {
  border-color: var(--pb-accent);
}

.pb-stats__computed {
  margin: calc(-1 * var(--pb-space-xs)) 0 var(--pb-space-md);
  color: var(--pb-ink-faint);
  font-family: var(--pb-font-mono);
  font-size: 11px;
  letter-spacing: 0.02em;
}

.pb-ratings__list,
.pb-versions__list {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
  list-style: none;
  margin: 0;
  padding: 0;
}

.pb-ratings__item {
  padding: var(--pb-space-md);
  background: color-mix(in srgb, var(--pb-surface-muted) 45%, transparent);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-md);
  transition:
    border-color var(--pb-duration) var(--pb-ease),
    background-color var(--pb-duration) var(--pb-ease);
}

.pb-ratings__item:hover {
  border-color: var(--pb-line-strong);
  background: var(--pb-surface-muted);
}

.pb-ratings__row {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: var(--pb-space-md);
}

.pb-ratings__model {
  min-width: 180px;
  color: var(--pb-ink);
  font-family: var(--pb-font-display);
  font-size: 17px;
  font-weight: 500;
}

.pb-ratings__count,
.pb-ratings__mean,
.pb-ratings__mine {
  color: var(--pb-ink-soft);
  font-family: var(--pb-font-mono);
  font-size: 13px;
  font-variant-numeric: tabular-nums;
}

.pb-ratings__actions {
  display: flex;
  align-items: center;
  gap: var(--pb-space-xs);
  margin-top: var(--pb-space-xs);
}

.pb-ratings__edits {
  color: var(--pb-ink-faint);
  font-size: 12px;
}

.pb-versions__item {
  display: flex;
  align-items: baseline;
  gap: var(--pb-space-md);
  padding: var(--pb-space-sm) var(--pb-space-md);
  background: color-mix(in srgb, var(--pb-surface-muted) 40%, transparent);
  border-left: 2px solid var(--pb-line);
  border-radius: 0 var(--pb-radius-sm) var(--pb-radius-sm) 0;
}

.pb-versions__item:first-child {
  border-left-color: var(--pb-detail-group);
}

.pb-stats__subtitle {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: var(--pb-space-lg) 0 var(--pb-space-sm);
  color: var(--pb-ink-soft);
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.pb-stats__subtitle::after {
  content: "";
  flex: 1;
  height: 1px;
  background: var(--pb-line);
}

.pb-stats__discrimination {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: var(--pb-space-sm);
  margin: 0;
}

.pb-stats__tile {
  display: flex;
  flex-direction: column;
  gap: 5px;
  padding: var(--pb-space-md);
  background: color-mix(in srgb, var(--pb-surface-muted) 62%, transparent);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-md);
}

.pb-stats__tile dt {
  color: var(--pb-ink-faint);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.pb-stats__tile dd {
  margin: 0;
  color: var(--pb-ink);
  font-family: var(--pb-font-display);
  font-size: 23px;
  line-height: 1.15;
  font-variant-numeric: tabular-nums;
}

.pb-meter {
  display: block;
  width: 100%;
  height: 3px;
  margin-top: 2px;
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

.pb-meter--discrimination .pb-meter__fill {
  background: linear-gradient(90deg, var(--pb-purpose), var(--pb-detail-group));
}

.pb-stats__tags {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
  list-style: none;
  margin: 0 0 var(--pb-space-md);
  padding: 0;
}

.pb-stats__tag {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: var(--pb-space-sm);
}

.pb-stats__tag-label {
  padding: 2px 10px;
  border-radius: var(--pb-radius-pill);
  background: var(--pb-accent-soft);
  color: var(--pb-accent-strong);
  font-size: 12.5px;
  font-weight: 600;
}

.pb-stats__tag-confidence,
.pb-stats__tag-reason {
  color: var(--pb-ink-soft);
  font-size: 13px;
}

.pb-stats__table-wrap {
  overflow-x: auto;
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-md);
}

.pb-stats__table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
  font-variant-numeric: tabular-nums;
}

.pb-stats__table th,
.pb-stats__table td {
  padding: 11px 12px;
  border-bottom: 1px solid var(--pb-line);
  text-align: left;
  white-space: nowrap;
}

.pb-stats__table th {
  background: var(--pb-surface-muted);
  color: var(--pb-ink-faint);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.pb-stats__table tbody tr:last-child td {
  border-bottom: 0;
}

.pb-stats__table tbody tr:hover {
  background: color-mix(in srgb, var(--pb-surface-muted) 55%, transparent);
}

.pb-prompt-detail__favorites {
  list-style: none;
  margin: 0;
  padding: 0;
}

.pb-versions__no {
  min-width: 56px;
  font-family: var(--pb-font-mono);
  font-weight: 600;
}

.pb-versions__major {
  color: var(--pb-accent-strong);
  font-size: 12px;
}

.pb-versions__date {
  margin-left: auto;
  color: var(--pb-ink-faint);
  font-size: 12px;
}

.pb-prompt-detail__error {
  margin: 0;
  padding: var(--pb-space-sm) var(--pb-space-md);
  border-radius: var(--pb-radius-md);
  background: var(--pb-accent-soft);
  color: var(--pb-danger);
  font-size: 13.5px;
}

.pb-prompt-detail__notice {
  margin: 0;
  color: var(--pb-ink-soft);
  font-size: 13.5px;
  word-break: break-all;
}

.pb-prompt-detail__hint {
  color: var(--pb-ink-soft);
}

@media (max-width: 640px) {
  .pb-prompt-detail {
    padding-top: var(--pb-space-md);
  }

  .pb-prompt-detail__meta,
  .pb-ratings,
  .pb-stats,
  .pb-versions {
    padding: var(--pb-space-lg);
  }

  .pb-prompt-detail__manuscript-head {
    flex-wrap: wrap;
    padding: 9px var(--pb-space-lg);
  }

  .pb-prompt-detail__body {
    padding: var(--pb-space-lg);
    font-size: 13px;
  }

  .pb-ratings__model {
    min-width: 0;
  }

  .pb-versions__item {
    flex-wrap: wrap;
  }

  .pb-versions__date {
    margin-left: 0;
  }
}
</style>