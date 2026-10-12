// 通知中心：查看、按类型筛选、标记已读、全部已读、删除、未读计数。
// 六项能力对应需求 §12.3。

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import {
  NOTIFICATION_KINDS,
  type Notification,
  type NotificationKind,
  socialApi,
} from '@/api/social'
import { useErrorMessage } from '@/composables/useErrorMessage'
import { useAuthStore } from '@/stores/auth'
import { useNotificationStore } from '@/stores/notifications'

const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const notifications = useNotificationStore()
const { messageOf } = useErrorMessage()

const items = ref<Notification[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const selectedKinds = ref<NotificationKind[]>([])
const loading = ref(true)
const failure = ref('')
const notice = ref('')

const kindOptions = computed(() =>
  NOTIFICATION_KINDS.map((kind) => ({
    value: kind,
    label: t(`notifications.kind${kindSuffix(kind)}`),
  })),
)

/** kindSuffix 把通知类型的英文键转为 i18n 键后缀。 */
function kindSuffix(kind: NotificationKind): string {
  switch (kind) {
    case 'comment_reply':
      return 'CommentReply'
    case 'library_question_deleted':
      return 'LibraryQuestionDeleted'
    case 'announcement':
      return 'Announcement'
    default:
      return 'System'
  }
}

/** load 拉取当前筛选条件下的通知。 */
async function load(): Promise<void> {
  loading.value = true
  try {
    const result = await socialApi.notifications(selectedKinds.value, page.value, pageSize)
    items.value = result.items
    total.value = result.total
    failure.value = ''
  } catch (error) {
    failure.value = messageOf(error)
  } finally {
    loading.value = false
  }
}

/** applyFilter 切换筛选后回到第一页并重新拉取。 */
async function applyFilter(kinds: NotificationKind[]): Promise<void> {
  selectedKinds.value = kinds
  page.value = 1
  await load()
}

async function changePage(next: number): Promise<void> {
  page.value = next
  await load()
}

async function markRead(item: Notification): Promise<void> {
  if (item.read) {
    return
  }
  try {
    await socialApi.markRead(item.id)
    item.read = true
    notifications.setUnread(notifications.unread - 1)
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function markAllRead(): Promise<void> {
  try {
    await socialApi.markAllRead()
    for (const item of items.value) {
      item.read = true
    }
    notifications.setUnread(0)
  } catch (error) {
    failure.value = messageOf(error)
  }
}

async function remove(item: Notification): Promise<void> {
  try {
    await socialApi.removeNotification(item.id)
    notice.value = t('notifications.deleted')
    if (!item.read) {
      notifications.setUnread(notifications.unread - 1)
    }
    await load()
  } catch (error) {
    failure.value = messageOf(error)
  }
}

/** openLink 打开通知指向的站内页面。 */
async function openLink(item: Notification): Promise<void> {
  await markRead(item)
  if (item.link !== '') {
    await router.push(item.link)
  }
}

/**
 * 登录跳转交给路由守卫：`requiresAuth` 已在进入本页前解析登录态。
 * 组件再判一次会与守卫的 ensureSession 竞争，未登录的跳转因此可能先于
 * 会话探测返回而把已登录用户也赶回登录页。
 */
onMounted(async () => {
  await auth.connect()
  if (!auth.isAuthenticated) {
    return
  }
  await load()
  await notifications.refresh().catch(() => undefined)
})
</script>

<template>
  <section class="pb-notifications">
    <header class="pb-notifications__head">
      <h1 class="pb-notifications__title">{{ t('notifications.title') }}</h1>
      <div class="pb-notifications__actions">
        <el-button size="small" data-testid="notifications-mark-all" @click="markAllRead">
          {{ t('notifications.markAllRead') }}
        </el-button>
      </div>
    </header>

    <p class="pb-notifications__unread" data-testid="notifications-unread">
      {{
        notifications.hasUnread
          ? t('notifications.unread', { count: notifications.unread })
          : t('notifications.noUnread')
      }}
    </p>

    <div class="pb-notifications__filter" data-testid="notifications-filter">
      <el-select
        multiple
        collapse-tags
        clearable
        :placeholder="t('notifications.filter')"
        :model-value="selectedKinds"
        @update:model-value="applyFilter($event as NotificationKind[])"
      >
        <el-option
          v-for="option in kindOptions"
          :key="option.value"
          :label="option.label"
          :value="option.value"
        />
      </el-select>
    </div>

    <p v-if="failure" class="pb-notifications__error" data-testid="notifications-error">
      {{ failure }}
    </p>
    <p v-if="notice" class="pb-notifications__notice" data-testid="notifications-notice">
      {{ notice }}
    </p>

    <p v-if="loading" class="pb-notifications__hint">{{ t('notifications.loadFailed') }}</p>
    <p v-else-if="items.length === 0" data-testid="notifications-empty">
      {{ t('notifications.empty') }}
    </p>

    <ul v-else class="pb-notifications__list" data-testid="notifications-list">
      <li
        v-for="item in items"
        :key="item.id"
        class="pb-notifications__item"
        :class="{ 'pb-notifications__item--unread': !item.read }"
        :data-testid="`notification-${item.id}`"
      >
        <div class="pb-notifications__item-head">
          <span class="pb-notifications__kind">{{ t(`notifications.kind${kindSuffix(item.kind)}`) }}</span>
          <span class="pb-notifications__item-title">{{ item.title }}</span>
          <span v-if="!item.read" class="pb-notifications__dot" data-testid="notification-unread-dot">
            {{ t('announcements.unread') }}
          </span>
        </div>
        <p class="pb-notifications__body">{{ item.body }}</p>
        <div class="pb-notifications__item-actions">
          <el-button
            v-if="!item.read"
            size="small"
            text
            :data-testid="`notification-read-${item.id}`"
            @click="markRead(item)"
          >
            {{ t('notifications.markRead') }}
          </el-button>
          <el-button
            v-if="item.link !== ''"
            size="small"
            text
            :data-testid="`notification-open-${item.id}`"
            @click="openLink(item)"
          >
            {{ t('notifications.viewLink') }}
          </el-button>
          <el-button
            size="small"
            text
            type="danger"
            :data-testid="`notification-delete-${item.id}`"
            @click="remove(item)"
          >
            {{ t('notifications.delete') }}
          </el-button>
        </div>
      </li>
    </ul>

    <el-pagination
      v-if="total > pageSize"
      class="pb-notifications__pager"
      layout="prev, pager, next"
      :current-page="page"
      :page-size="pageSize"
      :total="total"
      @current-change="changePage"
    />
  </section>
</template>

<style scoped>
.pb-notifications {
  padding: var(--pb-space-lg) 0;
}

.pb-notifications__head {
  align-items: center;
  display: flex;
  justify-content: space-between;
}

.pb-notifications__title {
  font-size: 22px;
  margin: 0;
}

.pb-notifications__unread {
  color: var(--pb-color-text-soft);
  font-size: 13px;
  margin: var(--pb-space-xs) 0 var(--pb-space-sm);
}

.pb-notifications__filter {
  margin-bottom: var(--pb-space-md);
}

.pb-notifications__list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.pb-notifications__item {
  border-top: 1px solid var(--pb-color-border);
  padding: var(--pb-space-sm) 0;
}

.pb-notifications__item--unread {
  background-color: var(--el-color-primary-light-9);
}

.pb-notifications__item-head {
  align-items: center;
  display: flex;
  gap: var(--pb-space-sm);
}

.pb-notifications__kind {
  background-color: var(--pb-color-surface-muted);
  border-radius: var(--pb-radius-sm);
  color: var(--pb-color-text-soft);
  font-size: 12px;
  padding: 1px var(--pb-space-xs);
}

.pb-notifications__item-title {
  font-weight: 600;
}

.pb-notifications__dot {
  color: var(--el-color-primary);
  font-size: 12px;
}

.pb-notifications__body {
  color: var(--pb-color-text-soft);
  font-size: 13px;
  margin: var(--pb-space-xs) 0 0;
}

.pb-notifications__item-actions {
  display: flex;
  gap: var(--pb-space-xs);
  margin-top: var(--pb-space-xs);
}

.pb-notifications__error {
  color: var(--el-color-danger);
}

.pb-notifications__notice,
.pb-notifications__hint {
  color: var(--pb-color-text-soft);
}

.pb-notifications__pager {
  margin-top: var(--pb-space-md);
}
</style>