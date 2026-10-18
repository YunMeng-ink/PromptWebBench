// 通知中心：查看、按类型筛选、标记已读、全部已读、删除、未读计数。
// 六项能力对应需求 §12.3。

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import {
  type Announcement,
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
const announcements = ref<Announcement[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const selectedKinds = ref<NotificationKind[]>([])
const loading = ref(true)
const failure = ref('')
const notice = ref('')

/** announcementUnread 统计当前展示的公告中未读的条数。 */
const announcementUnread = computed(() => announcements.value.filter((item) => !item.read).length)

/** formatTime 把 UTC 时刻按本机时区呈现。 */
function formatTime(raw: string): string {
  if (raw === '') {
    return ''
  }
  const parsed = new Date(raw)
  return Number.isNaN(parsed.getTime()) ? raw : parsed.toLocaleString()
}

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

/**
 * load 拉取当前筛选条件下的通知与全部有效公告。
 *
 * 未读数由「站内通知 + 未读公告」两部分组成，因此公告必须出现在这里；
 * 否则首页角标会指向用户在本页看不到的内容。
 */
async function load(): Promise<void> {
  loading.value = true
  try {
    const [result, announcementPage] = await Promise.all([
      socialApi.notifications(selectedKinds.value, page.value, pageSize),
      socialApi.announcements(1, 100),
    ])
    items.value = result.items
    announcements.value = announcementPage.items
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
    await Promise.all([socialApi.markAllRead(), socialApi.markAnnouncementsRead()])
    for (const item of items.value) {
      item.read = true
    }
    for (const item of announcements.value) {
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
      <div class="pb-notifications__heading">
        <span class="pb-eyebrow" aria-hidden="true">Inbox</span>
        <h1 class="pb-notifications__title">{{ t('notifications.title') }}</h1>
      </div>
      <div class="pb-notifications__actions">
        <el-button data-testid="notifications-mark-all" @click="markAllRead">
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

    <section
      v-if="announcements.length > 0"
      class="pb-notifications__announcements"
      data-testid="notifications-announcements"
    >
      <div class="pb-notifications__section-head">
        <span class="pb-notifications__section-title">{{ t('announcements.title') }}</span>
        <span v-if="announcementUnread > 0" class="pb-notifications__section-count">
          {{ t('notifications.unread', { count: announcementUnread }) }}
        </span>
      </div>
      <article
        v-for="item in announcements"
        :key="item.id"
        class="pb-notifications__announcement"
        :class="{ 'pb-notifications__item--unread': !item.read }"
        :data-testid="`announcement-${item.id}`"
      >
        <div class="pb-notifications__item-head">
          <span class="pb-notifications__kind">{{ t('notifications.kindAnnouncement') }}</span>
          <span class="pb-notifications__item-title">{{ item.title }}</span>
          <span v-if="!item.read" class="pb-notifications__dot">
            {{ t('announcements.unread') }}
          </span>
        </div>
        <p class="pb-notifications__body">{{ item.body }}</p>
        <time v-if="item.published_at" class="pb-notifications__time">
          {{ formatTime(item.published_at) }}
        </time>
      </article>
    </section>

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

    <p v-if="loading" class="pb-notifications__hint">{{ t('notifications.loading') }}</p>
    <div
      v-else-if="items.length === 0 && announcements.length === 0"
      class="pb-empty"
      data-testid="notifications-empty"
    >
      <span class="pb-empty__title">{{ t('notifications.empty') }}</span>
    </div>

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
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-md);
  padding: var(--pb-space-xl) 0 var(--pb-space-2xl);
}

.pb-notifications__head {
  position: relative;
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--pb-space-lg);
  padding-bottom: var(--pb-space-md);
  border-bottom: 1px solid var(--pb-line);
}

.pb-notifications__head::after {
  content: "";
  position: absolute;
  left: 0;
  bottom: -1px;
  width: 140px;
  height: 2px;
  background: linear-gradient(90deg, var(--pb-accent), var(--pb-purpose), transparent);
}

.pb-notifications__heading {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-xs);
}

.pb-notifications__title {
  font-size: 34px;
  font-weight: 500;
  letter-spacing: -0.012em;
  margin: 0;
}

.pb-notifications__unread {
  align-self: flex-start;
  margin: 0;
  padding: 3px 11px;
  border-radius: var(--pb-radius-pill);
  background: var(--pb-accent-soft);
  color: var(--pb-accent-strong);
  font-size: 12.5px;
}

.pb-notifications__filter {
  max-width: 320px;
}

.pb-notifications__announcements {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
}

.pb-notifications__section-head {
  align-items: baseline;
  display: flex;
  gap: var(--pb-space-sm);
}

.pb-notifications__section-title {
  font-family: var(--pb-font-display);
  font-size: 18px;
  font-weight: 500;
}

.pb-notifications__section-count {
  background: var(--pb-surface-muted);
  border-radius: var(--pb-radius-pill);
  color: var(--pb-ink-faint);
  font-size: 11.5px;
  padding: 2px 9px;
}

.pb-notifications__announcement {
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-left: 2px solid transparent;
  border-radius: var(--pb-radius-md);
  padding: var(--pb-space-md) var(--pb-space-lg);
  transition:
    background-color var(--pb-duration) var(--pb-ease),
    border-color var(--pb-duration) var(--pb-ease);
}

.pb-notifications__time {
  color: var(--pb-ink-faint);
  display: block;
  font-size: 12px;
  margin-top: var(--pb-space-xs);
}

.pb-notifications__list {
  display: flex;
  flex-direction: column;
  gap: var(--pb-space-sm);
  list-style: none;
  margin: 0;
  padding: 0;
}

.pb-notifications__item {
  padding: var(--pb-space-md) var(--pb-space-lg);
  background: var(--pb-surface);
  border: 1px solid var(--pb-line);
  border-radius: var(--pb-radius-md);
  border-left: 2px solid transparent;
}

.pb-notifications__item--unread {
  border-left-color: var(--pb-accent);
  background: var(--pb-accent-soft);
}

.pb-notifications__item-head {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--pb-space-sm);
}

.pb-notifications__kind {
  padding: 1px 9px;
  border-radius: var(--pb-radius-pill);
  background: var(--pb-surface-muted);
  color: var(--pb-ink-soft);
  font-size: 11.5px;
}

.pb-notifications__item-title {
  color: var(--pb-ink);
  font-weight: 600;
}

.pb-notifications__dot {
  color: var(--pb-accent-strong);
  font-size: 11.5px;
  font-weight: 600;
}

.pb-notifications__body {
  margin: var(--pb-space-xs) 0 0;
  color: var(--pb-ink-soft);
  font-size: 13.5px;
  line-height: 1.7;
}

.pb-notifications__item-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pb-space-xs);
  margin-top: var(--pb-space-xs);
}

.pb-notifications__error {
  margin: 0;
  padding: var(--pb-space-sm) var(--pb-space-md);
  border-radius: var(--pb-radius-md);
  background: var(--pb-accent-soft);
  color: var(--pb-danger);
  font-size: 13.5px;
}

.pb-notifications__notice,
.pb-notifications__hint {
  margin: 0;
  color: var(--pb-ink-soft);
  font-size: 13.5px;
}

.pb-notifications__pager {
  justify-content: flex-end;
  margin-top: var(--pb-space-md);
}
</style>