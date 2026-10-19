<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { ApiError } from '@/api/client'
import { type PromptDetail, promptApi } from '@/api/prompt'
import { type PromptModelRating, ratingApi } from '@/api/rating'
import { MAX_IMAGE_BYTES, MAX_IMAGE_COUNT, MAX_OUTPUT_CHARS, showcaseApi } from '@/api/showcase'
import { type Favorite, socialApi } from '@/api/social'
import { type PromptStats, statsApi } from '@/api/stats'
import CommentSection from '@/components/CommentSection.vue'
import ReportDialog from '@/components/ReportDialog.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { useAuthStore } from '@/stores/auth'
import { formatDateTime, timeZoneLabel } from '@/utils/datetime'
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
const retryable = ref(false)
const notice = ref('')

/**
 * loadSequence 标记当前加载批次。
 *
 * 同路由参数切换时前一次请求可能后到，迟到的响应会把新提示词的内容覆盖成旧内容。
 * 每次加载取一个递增序号，响应回来时序号不一致就整批丢弃。
 */
let loadSequence = 0

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

/**
 * pendingOutput 与 pendingImages 是评分弹窗中附带的模型结果：
 * 模型输出文本与结果图片。两者都是可选的，随评分提交一并上传。
 */
const pendingOutput = ref('')
const pendingImages = ref<File[]>([])
const imageInput = ref<HTMLInputElement | null>(null)

/** IMAGE_TYPES 与后端允许的图片类型一致。 */
const IMAGE_TYPES = [
  'image/jpeg',
  'image/png',
  'image/webp',
  'image/gif',
  'image/bmp',
  'image/tiff',
] as const

const dialogVisible = computed({
  get: () => pendingModel.value !== undefined,
  set: (value: boolean) => {
    if (!value) {
      cancelRating()
    }
  },
})

const scores = [1, 2, 3, 4, 5] as const

/**
 * ratingModelIDs 是评分列表中当前展示的模型集合。
 *
 * 模型字典可能有上百条，全部平铺会让页面无限变长。默认只展示已有评分与
 * 本人评过的模型；其余模型由用户通过上方的选择器按需加入。
 */
const ratingModelIDs = ref<number[]>([])
/** ratingSelectionTouched 记录用户是否调整过集合，避免刷新评分后覆盖其选择。 */
const ratingSelectionTouched = ref(false)

/** DEFAULT_RATING_MODELS 是没有任何评分时默认展示的模型数量。 */
const DEFAULT_RATING_MODELS = 8

/**
 * defaultRatingModelIDs 给出默认展示集合。
 *
 * 先纳入已有评分与本人评过的模型，再按字典顺序补足到默认条数。
 * 页面长度因此有固定上界，同时不会因为只有一条评分就把其余模型全部藏起来。
 */
function defaultRatingModelIDs(items: PromptModelRating[]): number[] {
  const active = items.filter((item) => item.rating_count > 0 || item.my_score !== undefined)
  const selected = active.map((item) => item.model_id)
  if (selected.length >= DEFAULT_RATING_MODELS) {
    return selected
  }
  const chosen = new Set(selected)
  for (const item of items) {
    if (selected.length >= DEFAULT_RATING_MODELS) {
      break
    }
    if (!chosen.has(item.model_id)) {
      chosen.add(item.model_id)
      selected.push(item.model_id)
    }
  }
  return selected
}

/** ratingOptions 是选择器的全部候选模型，附带评分与本人评分便于检索。 */
const ratingOptions = computed(() =>
  ratings.value.map((item) => ({
    value: item.model_id,
    label: item.model_name,
    rated: item.rating_count > 0,
    mine: item.my_score !== undefined,
  })),
)

/** selectedRatings 是按选择集合过滤后的评分行。 */
const selectedRatings = computed(() => {
  const selected = new Set(ratingModelIDs.value)
  return ratings.value.filter((item) => selected.has(item.model_id))
})

/** hiddenRatingCount 是当前未展示的模型数量。 */
const hiddenRatingCount = computed(() => ratings.value.length - selectedRatings.value.length)

/** ratingSelectionIsDefault 表示当前集合仍是默认集合，用于显示补充说明。 */
const ratingSelectionIsDefault = computed(
  () => !ratingSelectionTouched.value && hiddenRatingCount.value > 0,
)

/** setRatingModelIDs 记录用户的手动选择并去掉已不存在的模型。 */
function setRatingModelIDs(ids: number[]): void {
  const available = new Set(ratings.value.map((item) => item.model_id))
  ratingSelectionTouched.value = true
  ratingModelIDs.value = ids.filter((id) => available.has(id))
}

/** ensureRatingModel 把指定模型加入展示集合，供外部直接发起评分时使用。 */
function ensureRatingModel(modelID: number): void {
  if (ratingModelIDs.value.includes(modelID)) {
    return
  }
  ratingSelectionTouched.value = true
  ratingModelIDs.value = [...ratingModelIDs.value, modelID]
}

/** applyRatingQuick 按快捷条件重设展示集合。 */
function applyRatingQuick(kind: 'all' | 'rated' | 'mine'): void {
  ratingSelectionTouched.value = true
  switch (kind) {
    case 'all':
      ratingModelIDs.value = ratings.value.map((item) => item.model_id)
      break
    case 'rated':
      ratingModelIDs.value = ratings.value
        .filter((item) => item.rating_count > 0)
        .map((item) => item.model_id)
      break
    case 'mine':
      ratingModelIDs.value = ratings.value
        .filter((item) => item.my_score !== undefined)
        .map((item) => item.model_id)
      break
  }
}

/** resetRatingSelection 回到默认展示集合。 */
function resetRatingSelection(): void {
  ratingSelectionTouched.value = false
  ratingModelIDs.value = defaultRatingModelIDs(ratings.value)
}

/**
 * visibleStatsModels 是按评分区的模型选择过滤后的统计行。
 *
 * 统计接口返回全部已登记模型，其中多数没有任何评分。全部平铺会让测量结果区
 * 随模型字典增长而无限变长，因此与上方的评分选择共用同一集合：用户在评分区
 * 调整展示哪些模型，表格随之变化。
 */
const visibleStatsModels = computed(() => {
  if (stats.value === null) {
    return []
  }
  // 评分接口没有返回任何模型时不具备筛选依据，此时展示统计接口给出的全部模型，
  // 否则页面会因为一个空的筛选集合而看起来没有数据。
  if (ratings.value.length === 0) {
    return stats.value.models
  }
  const selected = new Set(ratingModelIDs.value)
  return stats.value.models.filter((model) => selected.has(model.model_id))
})

/** statsModelSummary 说明当前统计表展示了多少行，以及被隐藏的行数。 */
const statsModelSummary = computed(() => {
  const shown = visibleStatsModels.value.length
  const total = stats.value?.models.length ?? 0
  return { shown, total, hidden: total - shown }
})

/** statsHasRatedModel 报告当前选择里是否至少有一个模型带评分。 */
const statsHasRatedModel = computed(() =>
  visibleStatsModels.value.some((model) => model.history.has_data || model.recent.has_data),
)

/** formattedUpdatedAt 按浏览器所在时区呈现更新时间。 */
const formattedUpdatedAt = computed(() =>
  formatDateTime(detail.value?.prompt.updated_at, locale.value),
)

/** formattedCreatedAt 按浏览器所在时区呈现上传时间。 */
const formattedCreatedAt = computed(() =>
  formatDateTime(detail.value?.prompt.created_at, locale.value),
)

/** formattedComputedAt 按浏览器所在时区呈现统计计算时刻。 */
const formattedComputedAt = computed(() => formatDateTime(stats.value?.computed_at, locale.value))

/** formattedVersionTime 按浏览器所在时区呈现某个版本的创建时刻。 */
function formattedVersionTime(raw: string): string {
  return formatDateTime(raw, locale.value)
}

/** timezone 是当前浏览器时区，用于向用户说明时间口径。 */
const timezone = timeZoneLabel()

/** tagText 返回标签在当前语言下的名称。 */
function tagText(tag: PromptDetail['tags'][number]): string {
  return locale.value === 'en' ? tag.name_en : tag.name_zh
}

function localized(zh: string, en: string): string {
  return locale.value === 'en' ? en : zh
}

/** groupClass 把考察维度的分组映射为详情页的强调色类名。 */
const groupClass = computed(() => {
  const group =
    detail.value?.capabilities?.[0]?.group_key ?? detail.value?.capability.group_key ?? 'cognitive'
  return `pb-prompt-detail--${group}`
})

/** capabilityNames 把全部考察维度按当前语言拼成一行。 */
const capabilityNames = computed(() => {
  if (detail.value === null) {
    return ''
  }
  const rows =
    (detail.value.capabilities?.length ?? 0) > 0
      ? detail.value.capabilities
      : [detail.value.capability]
  return rows.map((item) => localized(item.name_zh, item.name_en)).join(' · ')
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

/** recommendTagText 返回推荐标签的文案，按当前语言取后端给出的双语之一。 */
function recommendTagText(tag: PromptStats['recommendation'][number]): string {
  return locale.value === 'en' ? tag.label_en : tag.label_zh
}

const maxEdits = ref(3)

function startRating(item: PromptModelRating): void {
  ensureRatingModel(item.model_id)
  pendingModel.value = item.model_id
  pendingScore.value = item.my_score
  pendingOutput.value = ''
  pendingImages.value = []
  editingExisting.value = item.my_score !== undefined
  notice.value = ''
  failure.value = ''
}

function cancelRating(): void {
  pendingModel.value = undefined
  pendingScore.value = undefined
  pendingOutput.value = ''
  pendingImages.value = []
  editingExisting.value = false
}

/** chooseImages 打开文件选择框。 */
function chooseImages(): void {
  imageInput.value?.click()
}

/** onImagePick 校验并收集待上传图片；非法文件当场拒绝并说明原因。 */
function onImagePick(event: Event): void {
  const input = event.target as HTMLInputElement
  const picked = Array.from(input.files ?? [])
  input.value = ''
  failure.value = ''
  for (const file of picked) {
    if (!IMAGE_TYPES.includes(file.type as (typeof IMAGE_TYPES)[number])) {
      failure.value = t('showcase.imageTypeInvalid', { name: file.name })
      return
    }
    if (file.size > MAX_IMAGE_BYTES) {
      failure.value = t('showcase.imageTooLarge', { name: file.name })
      return
    }
    if (pendingImages.value.length >= MAX_IMAGE_COUNT) {
      failure.value = t('showcase.imageCountExceeded', { count: MAX_IMAGE_COUNT })
      return
    }
    pendingImages.value.push(file)
  }
}

/** removePendingImage 从待上传列表移除一张图片。 */
function removePendingImage(index: number): void {
  pendingImages.value.splice(index, 1)
}

/**
 * confirmRating 按评分来源选择提交或修改，然后上传附带的模型输出与结果图片。
 *
 * 评分先落库：附图与输出都绑定在已有评分上。附加内容上传失败时评分保持有效，
 * 错误信息明确说明哪一步失败，用户可以在弹窗中重试附加内容之外的整条流程。
 */
async function confirmRating(): Promise<void> {
  if (pendingModel.value === undefined || pendingScore.value === undefined) {
    return
  }
  const modelID = pendingModel.value
  const payload = { model_id: modelID, score: pendingScore.value }
  try {
    if (editingExisting.value) {
      await ratingApi.update(promptID.value as number, payload)
      notice.value = t('prompt.ratingUpdated')
    } else {
      await ratingApi.submit(promptID.value as number, payload)
      notice.value = t('prompt.ratingSubmitted')
    }
  } catch (error) {
    failure.value = messageOf(error)
    return
  }

  const output = pendingOutput.value.trim()
  if (output !== '') {
    try {
      await showcaseApi.submitOutput(promptID.value as number, modelID, output)
      notice.value = `${notice.value} ${t('showcase.outputSubmitted')}`
    } catch (error) {
      failure.value = t('showcase.outputFailed', { message: messageOf(error) })
    }
  }

  for (const file of pendingImages.value) {
    try {
      await showcaseApi.uploadImage(promptID.value as number, modelID, file)
      notice.value = `${notice.value} ${t('showcase.imageSubmitted')}`
    } catch (error) {
      failure.value = t('showcase.imageFailed', { message: messageOf(error) })
      break
    }
  }

  cancelRating()
  await reloadRatings()
  await reloadStats()
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
  const capabilityName = capabilityNames.value
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
  const requested = promptID.value
  const result = await ratingApi.byPrompt(requested)
  if (requested !== promptID.value) {
    return
  }
  ratings.value = result.items
  if (!ratingSelectionTouched.value) {
    ratingModelIDs.value = defaultRatingModelIDs(ratings.value)
  }
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
  const requested = promptID.value
  try {
    const result = await statsApi.byPrompt(requested)
    if (requested !== promptID.value) {
      return
    }
    stats.value = result
    statsFailure.value = ''
  } catch (error) {
    if (requested !== promptID.value) {
      return
    }
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
  const current = ++loadSequence
  loading.value = true
  failure.value = ''
  retryable.value = false
  detail.value = null
  ratings.value = []
  ratingModelIDs.value = []
  ratingSelectionTouched.value = false
  stats.value = null
  statsFailure.value = ''
  if (promptID.value === null) {
    loading.value = false
    failure.value = t('prompt.notFound')
    return
  }

  try {
    const result = await promptApi.detail(promptID.value)
    if (current !== loadSequence) {
      return
    }
    detail.value = result
  } catch (error) {
    if (current !== loadSequence) {
      return
    }
    // 404 说明这条提示词确实不存在或不可见，重试没有意义；
    // 其余错误（网络中断、服务不可用）保留可重试入口。
    if (error instanceof ApiError && error.status === 404) {
      failure.value = t('prompt.notFound')
    } else {
      failure.value = messageOf(error)
      retryable.value = true
    }
    detail.value = null
    loading.value = false
    return
  }

  loading.value = false
  // 评分与统计读取失败只影响各自的区块，正文与操作入口照常可用。
  try {
    await reloadRatings()
  } catch (error) {
    if (current === loadSequence) {
      failure.value = messageOf(error)
    }
  }
  if (current !== loadSequence) {
    return
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
    <div v-if="loading" class="pb-detail-skeleton" data-testid="detail-loading" aria-busy="true">
      <p class="pb-detail-skeleton__label">{{ t('prompt.loading') }}</p>
      <div class="pb-detail-skeleton__head">
        <span class="pb-detail-skeleton__line pb-detail-skeleton__line--chips" />
        <span class="pb-detail-skeleton__line pb-detail-skeleton__line--title" />
      </div>
      <div class="pb-detail-skeleton__meta">
        <span v-for="index in 6" :key="index" class="pb-detail-skeleton__line" />
      </div>
      <div class="pb-detail-skeleton__body">
        <span v-for="index in 8" :key="index" class="pb-detail-skeleton__line" />
      </div>
    </div>

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
              {{ capabilityNames }}
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
          <dd data-testid="detail-capability">{{ capabilityNames }}</dd>
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
          <dd data-testid="detail-updated-at">{{ formattedUpdatedAt }}</dd>
        </div>
        <div>
          <dt>{{ t('prompt.createdAt') }}</dt>
          <dd data-testid="detail-created-at">{{ formattedCreatedAt }}</dd>
        </div>
      </dl>

      <div v-if="detail.tags.length > 0" class="pb-prompt-detail__tags" data-testid="detail-tags">
        <span class="pb-prompt-detail__tags-label">{{ t('prompt.tagsLabel') }}</span>
        <ul class="pb-prompt-detail__tag-list">
          <li
            v-for="tag in detail.tags"
            :key="tag.id"
            class="pb-prompt-detail__tag"
            :data-testid="`detail-tag-${tag.id}`"
          >
            <span class="pb-prompt-detail__tag-dot" :style="{ background: tag.color }" aria-hidden="true" />
            {{ tagText(tag) }}
          </li>
        </ul>
      </div>

      <p v-if="detail.version.description" class="pb-prompt-detail__description">
        {{ detail.version.description }}
      </p>

      <div class="pb-prompt-detail__manuscript">
        <div class="pb-prompt-detail__manuscript-head">
          <span>Prompt · {{ detail.prompt.version_no }}</span>
          <span>{{ formattedUpdatedAt }}</span>
        </div>
        <pre class="pb-prompt-detail__body" data-testid="detail-body">{{ detail.version.body }}</pre>
      </div>

      <section class="pb-ratings">
        <h2 class="pb-ratings__title">{{ t('prompt.ratingSection') }}</h2>
        <p class="pb-ratings__hint">{{ t('prompt.ratingHint') }}</p>

        <p class="pb-ratings__showcase-link">
          <RouterLink
            class="pb-ratings__showcase-anchor"
            :to="{ name: 'showcase' }"
            data-testid="showcase-entry-link"
          >
            {{ t('showcase.entryLink') }}
          </RouterLink>
          <span>{{ t('showcase.entryHint') }}</span>
        </p>

        <div v-if="ratings.length > 0" class="pb-ratings__picker">
          <el-select
            :model-value="ratingModelIDs"
            data-testid="ratings-model-select"
            class="pb-ratings__select"
            multiple
            filterable
            collapse-tags
            collapse-tags-tooltip
            clearable
            :placeholder="t('prompt.ratingSelectPlaceholder')"
            @update:model-value="setRatingModelIDs($event as number[])"
          >
            <el-option
              v-for="option in ratingOptions"
              :key="option.value"
              :value="option.value"
              :label="option.label"
            >
              <span class="pb-ratings__option">
                <span>{{ option.label }}</span>
                <span class="pb-ratings__option-tags">
                  <span v-if="option.rated" class="pb-ratings__option-tag">
                    {{ t('prompt.ratingSelectRated') }}
                  </span>
                  <span v-if="option.mine" class="pb-ratings__option-tag is-mine">
                    {{ t('prompt.ratingSelectMine') }}
                  </span>
                </span>
              </span>
            </el-option>
          </el-select>

          <div class="pb-ratings__quick">
            <el-button size="small" text data-testid="ratings-quick-all" @click="applyRatingQuick('all')">
              {{ t('prompt.ratingSelectAll') }}
            </el-button>
            <el-button size="small" text data-testid="ratings-quick-rated" @click="applyRatingQuick('rated')">
              {{ t('prompt.ratingSelectRatedOnly') }}
            </el-button>
            <el-button
              v-if="auth.isAuthenticated"
              size="small"
              text
              data-testid="ratings-quick-mine"
              @click="applyRatingQuick('mine')"
            >
              {{ t('prompt.ratingSelectMineOnly') }}
            </el-button>
            <el-button size="small" text data-testid="ratings-quick-reset" @click="resetRatingSelection">
              {{ t('prompt.ratingSelectReset') }}
            </el-button>
          </div>

          <p class="pb-ratings__selection" data-testid="ratings-selection-summary">
            {{
              t('prompt.ratingSelectSummary', {
                selected: selectedRatings.length,
                total: ratings.length,
              })
            }}
            <span v-if="ratingSelectionIsDefault">{{ t('prompt.ratingSelectDefaultHint') }}</span>
          </p>
        </div>

        <p v-if="ratings.length === 0" data-testid="ratings-empty">{{ t('prompt.ratingEmpty') }}</p>

        <p
          v-else-if="selectedRatings.length === 0"
          class="pb-ratings__selection-empty"
          data-testid="ratings-selection-empty"
        >
          {{ t('prompt.ratingSelectEmpty') }}
        </p>

        <ul v-else class="pb-ratings__list" data-testid="ratings-list">
          <li v-for="item in selectedRatings" :key="item.model_id" class="pb-ratings__item">
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
              <RouterLink
                class="pb-ratings__example-link"
                :to="{ name: 'showcase', query: { model: String(item.model_id) } }"
                :data-testid="`rating-showcase-${item.model_id}`"
              >
                {{ t('showcase.modelLink') }}
              </RouterLink>
            </div>
          </li>
        </ul>

        <el-dialog
          v-model="dialogVisible"
          data-testid="rating-dialog"
          :title="t('prompt.myRating')"
          width="560px"
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

          <div class="pb-rating-extra">
            <el-divider />
            <h3 class="pb-rating-extra__title">{{ t('showcase.ratingExtraTitle') }}</h3>
            <p class="pb-rating-extra__hint">{{ t('showcase.ratingExtraHint') }}</p>

            <el-input
              v-model="pendingOutput"
              type="textarea"
              :rows="4"
              :maxlength="MAX_OUTPUT_CHARS"
              show-word-limit
              :placeholder="t('showcase.outputPlaceholder')"
              data-testid="rating-output"
            />

            <div class="pb-rating-extra__images">
              <input
                ref="imageInput"
                class="pb-rating-extra__file"
                type="file"
                accept="image/jpeg,image/png,image/webp,image/gif,image/bmp,image/tiff"
                multiple
                data-testid="rating-image-input"
                @change="onImagePick"
              />
              <el-button
                size="small"
                :disabled="pendingImages.length >= MAX_IMAGE_COUNT"
                data-testid="rating-image-choose"
                @click="chooseImages"
              >
                {{ t('showcase.chooseImages') }}
              </el-button>
              <span class="pb-rating-extra__limit">
                {{ t('showcase.imageLimit', { count: MAX_IMAGE_COUNT }) }}
              </span>
            </div>

            <ul v-if="pendingImages.length > 0" class="pb-rating-extra__list" data-testid="rating-image-list">
              <li v-for="(file, index) in pendingImages" :key="`${file.name}-${index}`">
                <span class="pb-rating-extra__name">{{ file.name }}</span>
                <el-button
                  size="small"
                  text
                  :data-testid="`rating-image-remove-${index}`"
                  @click="removePendingImage(index)"
                >
                  {{ t('showcase.removeImage') }}
                </el-button>
              </li>
            </ul>
          </div>

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
          {{ t('prompt.statsComputedAt') }} · {{ formattedComputedAt }}
          <span v-if="timezone" class="pb-stats__timezone">{{ timezone }}</span>
        </p>

        <p v-if="statsFailure" class="pb-prompt-detail__error" data-testid="stats-error">
          {{ t('prompt.statsUnavailable') }}
        </p>

        <template v-else-if="stats">
          <ul v-if="stats.recommendation.length > 0" class="pb-stats__tags" data-testid="stats-tags">
            <li v-for="tag in stats.recommendation" :key="tag.id" class="pb-stats__tag">
              <span class="pb-stats__tag-label" :data-testid="`stats-tag-${tag.kind}`">
                {{ recommendTagText(tag) }}
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
          <div class="pb-stats__model-tools" data-testid="stats-model-tools">
            <p class="pb-stats__model-summary" data-testid="stats-model-summary">
              {{
                t('prompt.statsModelSummary', {
                  shown: statsModelSummary.shown,
                  total: statsModelSummary.total,
                })
              }}
              <span v-if="statsModelSummary.hidden > 0" class="pb-stats__model-linked">
                {{ t('prompt.statsModelLinked') }}
              </span>
            </p>
            <div class="pb-stats__model-actions">
              <el-button
                size="small"
                text
                data-testid="stats-models-all"
                @click="applyRatingQuick('all')"
              >
                {{ t('prompt.statsModelsAll') }}
              </el-button>
              <el-button
                size="small"
                text
                data-testid="stats-models-rated"
                @click="applyRatingQuick('rated')"
              >
                {{ t('prompt.statsModelsRated') }}
              </el-button>
            </div>
          </div>

          <p
            v-if="visibleStatsModels.length === 0"
            class="pb-stats__model-empty"
            data-testid="stats-models-empty"
          >
            {{ t('prompt.statsModelsEmpty') }}
          </p>

          <div v-else class="pb-stats__table-wrap">
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
                <tr v-for="model in visibleStatsModels" :key="model.model_id">
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

          <p v-if="!statsHasRatedModel" class="pb-stats__model-note" data-testid="stats-models-note">
            {{ t('prompt.statsModelsNoRating') }}
          </p>
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
            <span class="pb-versions__date">{{ formattedVersionTime(version.created_at) }}</span>
          </li>
        </ul>
      </section>
    </template>

    <div v-else class="pb-prompt-detail__failure" data-testid="detail-error">
      <p class="pb-prompt-detail__error">{{ failure || t('prompt.notFound') }}</p>
      <el-button
        v-if="retryable"
        size="small"
        data-testid="detail-retry"
        @click="loadDetail"
      >
        {{ t('prompt.retry') }}
      </el-button>
    </div>

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

/* ---------- 标签 ----------
 * 标签是提示词的分类入口，与考察目的、考察维度并列展示在元信息下方。
 */

.pb-prompt-detail__tags {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: var(--pb-space-sm);
  padding: var(--pb-space-sm) var(--pb-space-md);
  background: color-mix(in srgb, var(--pb-surface-muted) 52%, transparent);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-md);
}

.pb-prompt-detail__tags-label {
  color: var(--pb-ink-faint);
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.pb-prompt-detail__tag-list {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--pb-space-xs);
  list-style: none;
  margin: 0;
  padding: 0;
}

.pb-prompt-detail__tag {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 10px;
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-pill);
  background: var(--pb-surface);
  color: var(--pb-ink-soft);
  font-size: 12.5px;
  line-height: 1.7;
}

.pb-prompt-detail__tag-dot {
  width: 6px;
  height: 6px;
  flex-shrink: 0;
  border-radius: 50%;
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

.pb-ratings__picker {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
  margin-bottom: var(--pb-space-md);
}

.pb-ratings__select {
  max-width: 100%;
  width: 420px;
}

.pb-ratings__option {
  align-items: center;
  display: flex;
  gap: var(--pb-space-sm);
  justify-content: space-between;
  width: 100%;
}

.pb-ratings__option-tags {
  display: inline-flex;
  flex: none;
  gap: 4px;
}

.pb-ratings__option-tag {
  background: var(--pb-surface-muted);
  border-radius: var(--pb-radius-pill);
  color: var(--pb-ink-faint);
  font-size: 10.5px;
  padding: 1px 7px;
}

.pb-ratings__option-tag.is-mine {
  background: var(--pb-accent-soft);
  color: var(--pb-accent-strong);
}

.pb-ratings__quick {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-xs);
}

.pb-ratings__selection {
  color: var(--pb-ink-faint);
  font-size: 12px;
  margin: 0;
}

.pb-ratings__selection-empty {
  border: 1px dashed var(--pb-line-strong);
  border-radius: var(--pb-radius-md);
  color: var(--pb-ink-faint);
  font-size: 13px;
  margin: 0;
  padding: var(--pb-space-lg);
  text-align: center;
}

.pb-ratings__hint,
.pb-stats__hint {
  margin: 0 0 var(--pb-space-md);
  color: var(--pb-ink-faint);
  font-size: 13px;
}

.pb-ratings__showcase-link {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 4px 10px;
  margin: 0 0 var(--pb-space-md);
  padding: var(--pb-space-sm) var(--pb-space-md);
  border: 1px solid var(--pb-line);
  border-left: 2px solid var(--pb-accent);
  border-radius: 0 var(--pb-radius-md) var(--pb-radius-md) 0;
  background: color-mix(in srgb, var(--pb-accent-soft) 45%, transparent);
  color: var(--pb-ink-soft);
  font-size: 12.5px;
}

.pb-ratings__showcase-anchor {
  color: var(--pb-accent-strong);
  font-weight: 600;
  border-bottom: 1px solid var(--pb-accent-ring);
  transition: border-color var(--pb-duration) var(--pb-ease);
}

.pb-ratings__showcase-anchor:hover {
  border-color: var(--pb-accent);
}

.pb-ratings__example-link {
  margin-left: auto;
  color: var(--pb-ink-faint);
  font-size: 12px;
  border-bottom: 1px solid transparent;
  transition:
    color var(--pb-duration) var(--pb-ease),
    border-color var(--pb-duration) var(--pb-ease);
}

.pb-ratings__example-link:hover {
  color: var(--pb-accent);
  border-color: var(--pb-accent-ring);
}

.pb-rating-extra {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
}

.pb-rating-extra__title {
  margin: 0;
  color: var(--pb-ink);
  font-size: 14px;
  font-weight: 600;
}

.pb-rating-extra__hint {
  margin: 0;
  color: var(--pb-ink-faint);
  font-size: 12px;
  line-height: 1.7;
}

.pb-rating-extra__images {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--pb-space-sm);
}

.pb-rating-extra__file {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
  white-space: nowrap;
  border: 0;
}

.pb-rating-extra__limit {
  color: var(--pb-ink-faint);
  font-size: 11.5px;
}

.pb-rating-extra__list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  list-style: none;
  margin: 0;
  padding: 0;
}

.pb-rating-extra__list li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--pb-space-sm);
  padding: 3px 10px;
  border-radius: var(--pb-radius-sm);
  background: var(--pb-surface-muted);
}

.pb-rating-extra__name {
  min-width: 0;
  overflow: hidden;
  color: var(--pb-ink-soft);
  font-family: var(--pb-font-mono);
  font-size: 11.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
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

.pb-stats__timezone {
  margin-left: 8px;
  padding: 1px 7px;
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-pill);
  color: var(--pb-ink-faint);
  font-size: 10px;
  letter-spacing: 0.04em;
}

/* ---------- 模型完成度表 ----------
 * 表格与评分区的模型选择联动：工具条说明当前展示范围，并提供两个快捷入口。
 */

.pb-stats__model-tools {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: var(--pb-space-sm);
  margin-bottom: var(--pb-space-sm);
}

.pb-stats__model-summary {
  margin: 0;
  color: var(--pb-ink-faint);
  font-size: 12px;
}

.pb-stats__model-linked {
  margin-left: 6px;
  color: var(--pb-ink-faint);
  opacity: 0.85;
}

.pb-stats__model-actions {
  display: flex;
  align-items: center;
  gap: var(--pb-space-xs);
}

.pb-stats__model-empty {
  margin: 0;
  padding: var(--pb-space-lg);
  border: 1px dashed var(--pb-line-strong);
  border-radius: var(--pb-radius-md);
  color: var(--pb-ink-faint);
  font-size: 13px;
  text-align: center;
}

.pb-stats__model-note {
  margin: var(--pb-space-sm) 0 0;
  color: var(--pb-ink-faint);
  font-size: 12px;
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

.pb-prompt-detail__failure {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: var(--pb-space-sm);
}

/* ---------- 加载骨架 ----------
 * 结构对齐详情页的真实版式：标题区、元信息网格与正文块。
 * 这样数据到达时页面高度不会突变，滚动条也不会跳动。
 */

.pb-detail-skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-lg);
}

.pb-detail-skeleton__label {
  margin: 0;
  color: var(--pb-ink-faint);
  font-size: 13px;
}

.pb-detail-skeleton__head {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
  padding: var(--pb-space-xl);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-xl);
  box-shadow:
    var(--pb-shadow-sm),
    var(--pb-shadow-inset);
}

.pb-detail-skeleton__meta {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: var(--pb-space-md) var(--pb-space-xl);
  padding: var(--pb-space-lg) var(--pb-space-xl);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
}

.pb-detail-skeleton__body {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
  padding: var(--pb-space-xl);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
}

.pb-detail-skeleton__line {
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

.pb-detail-skeleton__line--chips {
  width: 280px;
  height: 16px;
}

.pb-detail-skeleton__line--title {
  width: 52%;
  height: 34px;
}

.pb-detail-skeleton__body .pb-detail-skeleton__line:nth-child(3n) {
  width: 88%;
}

.pb-detail-skeleton__body .pb-detail-skeleton__line:last-child {
  width: 62%;
}

@media (max-width: 640px) {
  .pb-detail-skeleton__head,
  .pb-detail-skeleton__meta,
  .pb-detail-skeleton__body {
    padding: var(--pb-space-lg);
  }

  .pb-detail-skeleton__line--title {
    width: 74%;
    height: 28px;
  }
}

.pb-prompt-detail__notice {
  margin: 0;
  color: var(--pb-ink-soft);
  font-size: 13.5px;
  word-break: break-all;
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

  .pb-ratings__select {
    width: 100%;
  }

  .pb-versions__item {
    flex-wrap: wrap;
  }

  .pb-versions__date {
    margin-left: 0;
  }
}
</style>