// 评论列表与发表框。游客可读（需求 §12.1），登录用户可发表、回复、点赞、删除。

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { type Comment, socialApi } from '@/api/social'
import ReportDialog from '@/components/ReportDialog.vue'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { useAuthStore } from '@/stores/auth'

const props = defineProps<{ promptId: number }>()

const { t } = useI18n()
const auth = useAuthStore()
const { messageOf } = useErrorMessage()

const comments = ref<Comment[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const loading = ref(true)
const failure = ref('')
const notice = ref('')

const draft = ref('')
const submitting = ref(false)

/** 回复目标：非空时输入框用于回复该条评论。 */
const replyTarget = ref<Comment | null>(null)
const replyDraft = ref('')
const replySubmitting = ref(false)

/** 展开的回复列表，按父评论 ID 存放。 */
const replies = ref<Record<number, Comment[]>>({})

const reportVisible = ref(false)
const reportTarget = ref<Comment | null>(null)

async function load(): Promise<void> {
  loading.value = true
  try {
    const result = await socialApi.comments(props.promptId, page.value, pageSize)
    comments.value = result.items
    total.value = result.total
    failure.value = ''
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    loading.value = false
  }
}

async function changePage(next: number): Promise<void> {
  page.value = next
  await load()
}

async function post(): Promise<void> {
  if (draft.value.trim() === '') {
    return
  }
  submitting.value = true
  try {
    await socialApi.createComment(props.promptId, draft.value)
    draft.value = ''
    notice.value = t('prompt.commentPosted')
    await load()
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    submitting.value = false
  }
}

function startReply(comment: Comment): void {
  replyTarget.value = comment
  replyDraft.value = ''
}

function cancelReply(): void {
  replyTarget.value = null
  replyDraft.value = ''
}

async function submitReply(): Promise<void> {
  if (replyTarget.value === null || replyDraft.value.trim() === '') {
    return
  }
  replySubmitting.value = true
  try {
    await socialApi.createComment(props.promptId, replyDraft.value, replyTarget.value.id)
    notice.value = t('prompt.commentReplyPosted')
    const parentID = replyTarget.value.id
    cancelReply()
    await load()
    await loadReplies(parentID)
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    replySubmitting.value = false
  }
}

async function loadReplies(commentID: number): Promise<void> {
  try {
    const result = await socialApi.replies(commentID, 1, 50)
    replies.value = { ...replies.value, [commentID]: result.items }
  } catch (error) {
    failure.value = messageOf(error)
  }
}

/** toggleReplies 展开或收起回复。 */
async function toggleReplies(comment: Comment): Promise<void> {
  if (replies.value[comment.id] !== undefined) {
    const next = { ...replies.value }
    delete next[comment.id]
    replies.value = next
    return
  }
  await loadReplies(comment.id)
}

async function toggleLike(comment: Comment): Promise<void> {
  try {
    if (comment.liked) {
      await socialApi.unlikeComment(comment.id)
    } else {
      await socialApi.likeComment(comment.id)
    }
    await load()
    // 已展开的回复也要刷新，否则同一屏内的计数会不一致。
    if (replies.value[comment.id] !== undefined) {
      await loadReplies(comment.id)
    }
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function remove(comment: Comment): Promise<void> {
  try {
    await socialApi.removeComment(comment.id)
    notice.value = t('prompt.commentDeleted')
    await load()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

/** canDelete 判定当前身份是否能删除该评论。 */
function canDelete(comment: Comment): boolean {
  return auth.isAuthenticated && auth.user?.id === comment.user_id
}

function openReport(comment: Comment): void {
  reportTarget.value = comment
  reportVisible.value = true
}

/** 提示词切换时重新拉取，避免组件复用导致显示上一条的评论。 */
watch(
  () => props.promptId,
  async () => {
    replies.value = {}
    page.value = 1
    await load()
  },
)

/**
 * connect 先解析登录态再拉评论。
 *
 * 评论组件可能被挂在详情页之外（例如未来的题库题目页），因此自己解析登录态，
 * 不依赖父组件已经调过 auth.connect()。
 */
onMounted(async () => {
  await auth.connect()
  await load()
})
</script>

<template>
  <section class="pb-comments" data-testid="comments-section">
    <h2 class="pb-comments__title">{{ t('prompt.commentsSection') }}</h2>

    <div v-if="auth.isAuthenticated" class="pb-comments__editor">
      <el-input
        v-model="draft"
        type="textarea"
        :rows="3"
        maxlength="2000"
        :placeholder="t('prompt.commentPlaceholder')"
        data-testid="comment-input"
      />
      <el-button
        type="primary"
        size="small"
        :loading="submitting"
        class="pb-comments__submit"
        data-testid="comment-submit"
        @click="post"
      >
        {{ t('prompt.commentSubmit') }}
      </el-button>
    </div>
    <p v-else class="pb-comments__hint" data-testid="comments-login-hint">
      {{ t('prompt.commentNeedLogin') }}
    </p>

    <p v-if="failure" class="pb-comments__error" data-testid="comments-error">{{ failure }}</p>
    <p v-if="notice" class="pb-comments__notice" data-testid="comments-notice">{{ notice }}</p>

    <p v-if="loading" class="pb-comments__hint">{{ t('prompt.commentEmpty') }}</p>
    <p v-else-if="comments.length === 0" data-testid="comments-empty">
      {{ t('prompt.commentEmpty') }}
    </p>

    <ul v-else class="pb-comments__list" data-testid="comments-list">
      <li
        v-for="comment in comments"
        :key="comment.id"
        class="pb-comments__item"
        :data-testid="`comment-${comment.id}`"
      >
        <div class="pb-comments__head">
          <span class="pb-comments__author">{{ comment.author_name }}</span>
          <span class="pb-comments__time">{{ comment.created_at }}</span>
        </div>
        <p class="pb-comments__body">{{ comment.body }}</p>

        <div class="pb-comments__actions">
          <el-button
            v-if="auth.isAuthenticated"
            size="small"
            text
            :data-testid="`comment-like-${comment.id}`"
            @click="toggleLike(comment)"
          >
            {{ comment.liked ? t('prompt.commentUnlike') : t('prompt.commentLike') }}
            <span :data-testid="`comment-like-count-${comment.id}`">{{ comment.like_count }}</span>
          </el-button>
          <el-button
            v-if="auth.isAuthenticated"
            size="small"
            text
            :data-testid="`comment-reply-${comment.id}`"
            @click="startReply(comment)"
          >
            {{ t('prompt.commentReply') }}
          </el-button>
          <el-button
            v-if="canDelete(comment)"
            size="small"
            text
            type="danger"
            :data-testid="`comment-delete-${comment.id}`"
            @click="remove(comment)"
          >
            {{ t('prompt.commentDelete') }}
          </el-button>
          <el-button
            v-if="auth.isAuthenticated"
            size="small"
            text
            :data-testid="`comment-report-${comment.id}`"
            @click="openReport(comment)"
          >
            {{ t('prompt.reportAction') }}
          </el-button>
          <el-button
            v-if="comment.reply_count > 0"
            size="small"
            text
            :data-testid="`comment-replies-toggle-${comment.id}`"
            @click="toggleReplies(comment)"
          >
            {{
              replies[comment.id] === undefined
                ? t('prompt.commentViewReplies')
                : t('prompt.commentHideReplies')
            }}
            <span :data-testid="`comment-reply-count-${comment.id}`">{{ comment.reply_count }}</span>
          </el-button>
        </div>

        <div v-if="replyTarget?.id === comment.id" class="pb-comments__reply-editor">
          <el-input
            v-model="replyDraft"
            type="textarea"
            :rows="2"
            maxlength="2000"
            :placeholder="t('prompt.commentReplyPlaceholder')"
            :data-testid="`comment-reply-input-${comment.id}`"
          />
          <div class="pb-comments__reply-actions">
            <el-button size="small" :data-testid="`comment-reply-cancel-${comment.id}`" @click="cancelReply">
              {{ t('auth.cancel') }}
            </el-button>
            <el-button
              size="small"
              type="primary"
              :loading="replySubmitting"
              :data-testid="`comment-reply-submit-${comment.id}`"
              @click="submitReply"
            >
              {{ t('prompt.commentReply') }}
            </el-button>
          </div>
        </div>

        <ul v-if="replies[comment.id] !== undefined" class="pb-comments__replies" data-testid="replies-list">
          <li
            v-for="reply in replies[comment.id]"
            :key="reply.id"
            class="pb-comments__reply"
            :data-testid="`reply-${reply.id}`"
          >
            <span class="pb-comments__author">{{ reply.author_name }}</span>
            <p class="pb-comments__body">{{ reply.body }}</p>
          </li>
        </ul>
      </li>
    </ul>

    <el-pagination
      v-if="total > pageSize"
      layout="prev, pager, next"
      :current-page="page"
      :page-size="pageSize"
      :total="total"
      @current-change="changePage"
    />

    <ReportDialog
      v-if="reportTarget !== null"
      v-model:visible="reportVisible"
      object-type="comment"
      :object-id="reportTarget.id"
      @submitted="load"
    />
  </section>
</template>

<style scoped>
.pb-comments {
  margin: 0;
  padding: var(--pb-space-lg) var(--pb-space-xl) var(--pb-space-xl);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-lg);
  box-shadow: var(--pb-shadow-sm);
}

.pb-comments__title {
  margin: 0 0 var(--pb-space-md);
  font-size: 18px;
  font-weight: 600;
  letter-spacing: 0.01em;
}

.pb-comments__editor {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
  padding-bottom: var(--pb-space-md);
  border-bottom: 1px solid var(--pb-line);
}

.pb-comments__submit {
  align-self: flex-end;
}

.pb-comments__list,
.pb-comments__replies {
  list-style: none;
  margin: 0;
  padding: 0;
}

.pb-comments__item {
  padding: var(--pb-space-md) 0;
  border-top: 1px solid var(--pb-line);
}

.pb-comments__item:first-child {
  border-top: 0;
}

.pb-comments__head {
  display: flex;
  align-items: baseline;
  gap: var(--pb-space-sm);
}

.pb-comments__author {
  color: var(--pb-ink);
  font-weight: 600;
}

.pb-comments__time {
  color: var(--pb-ink-faint);
  font-size: 12px;
}

.pb-comments__body {
  margin: 6px 0;
  color: var(--pb-ink-soft);
  font-size: 14px;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
}

.pb-comments__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-xs);
}

.pb-comments__reply-editor {
  margin-top: var(--pb-space-sm);
}

.pb-comments__reply-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--pb-space-xs);
  margin-top: var(--pb-space-xs);
}

.pb-comments__reply {
  margin-top: var(--pb-space-xs);
  padding-left: var(--pb-space-md);
  border-left: 2px solid var(--pb-line-strong);
}

.pb-comments__error {
  margin: 0;
  padding: var(--pb-space-sm) var(--pb-space-md);
  border-radius: var(--pb-radius-md);
  background: var(--pb-accent-soft);
  color: var(--pb-danger);
  font-size: 13.5px;
}

.pb-comments__notice,
.pb-comments__hint {
  margin: var(--pb-space-sm) 0 0;
  color: var(--pb-ink-faint);
  font-size: 13.5px;
}
</style>