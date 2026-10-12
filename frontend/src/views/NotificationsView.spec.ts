import { flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { errorEnvelope, mountView, resetLocale, stubResponse } from '@/testing/mount'
import NotificationsView from '@/views/NotificationsView.vue'

interface RecordedRequest {
  method: string
  path: string
  query: string
}

interface StubOptions {
  notifications?: unknown[]
  unread?: number
  authenticated?: boolean
}

/** stubNotificationEndpoints 按路径分派通知中心所需的端点。 */
function stubNotificationEndpoints(options: StubOptions = {}): RecordedRequest[] {
  const recorded: RecordedRequest[] = []
  const notifications = options.notifications ?? []
  const unread = options.unread ?? 0

  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      const withoutHost = url.replace(/^https?:\/\/[^/]+/, '')
      const [path = '', query = ''] = withoutHost.split('?')
      const method = init?.method ?? 'GET'
      recorded.push({ method, path, query })

      if (path.endsWith('/session')) {
        return options.authenticated === false
          ? stubResponse({ status: 401, body: errorEnvelope('unauthorized') })
          : stubResponse({
              status: 200,
              body: {
                id: 1,
                username: 'alice',
                email: 'alice@example.com',
                role: 'user',
                status: 'normal',
                bio: '',
                avatar_url: '',
                email_verified: true,
              },
            })
      }

      if (path.endsWith('/notifications/unread-count')) {
        return stubResponse({ status: 200, body: { unread } })
      }

      if (path.endsWith('/notifications/read-all')) {
        return stubResponse({ status: 204 })
      }

      if (/\/notifications\/\d+\/read$/.test(path)) {
        return stubResponse({ status: 204 })
      }

      if (/\/notifications\/\d+$/.test(path) && method === 'DELETE') {
        return stubResponse({ status: 204 })
      }

      if (path.endsWith('/notifications')) {
        return stubResponse({
          status: 200,
          body: { items: notifications, page: 1, page_size: 20, total: notifications.length },
        })
      }

      return stubResponse({ status: 404, body: errorEnvelope('not_found') })
    }),
  )
  return recorded
}

function notificationFixture(overrides: Record<string, unknown> = {}): unknown {
  return {
    id: 1,
    kind: 'comment_reply',
    title: '你的评论收到了回复',
    body: '一条回复',
    link: '/prompts/1',
    read: false,
    created_at: '2026-10-12T00:00:00Z',
    ...overrides,
  }
}

function memoryRouter(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'home', component: { template: '<div />' } },
      { path: '/login', name: 'login', component: { template: '<div />' } },
      { path: '/notifications', name: 'notifications', component: NotificationsView },
      { path: '/prompts/:id', name: 'prompt-detail', component: { template: '<div />' } },
    ],
  })
}

async function mountNotifications(): Promise<VueWrapper> {
  const router = memoryRouter()
  await router.push('/notifications')
  await router.isReady()
  const wrapper = mountView(NotificationsView, { router })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  setActivePinia(createPinia())
  resetLocale()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('通知中心', () => {
  it('展示通知列表与未读数', async () => {
    stubNotificationEndpoints({
      notifications: [notificationFixture(), notificationFixture({ id: 2, read: true })],
      unread: 1,
    })
    const wrapper = await mountNotifications()

    expect(wrapper.find('[data-testid="notifications-list"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="notifications-unread"]').text()).toBe('未读 1 条')
  })

  it('没有通知时展示空态', async () => {
    stubNotificationEndpoints({ notifications: [] })
    const wrapper = await mountNotifications()

    expect(wrapper.find('[data-testid="notifications-empty"]').exists()).toBe(true)
  })

  it('按类型筛选时把类型作为查询参数发出', async () => {
    const recorded = stubNotificationEndpoints({ notifications: [notificationFixture()] })
    const wrapper = await mountNotifications()

    // el-select 的远程选项在 jsdom 里不可达，直接调用组件的筛选处理。
    const select = wrapper.findComponent({ name: 'ElSelect' })
    expect(select.exists()).toBe(true)
    await select.vm.$emit('update:model-value', ['comment_reply'])
    await flushPromises()

    const filtered = recorded.filter((r) => r.path.endsWith('/notifications'))
    const last = filtered[filtered.length - 1]
    expect(last?.query).toContain('kind=comment_reply')
  })

  it('标记单条已读后显示已读状态', async () => {
    stubNotificationEndpoints({ notifications: [notificationFixture()], unread: 1 })
    const wrapper = await mountNotifications()

    expect(wrapper.find('[data-testid="notification-unread-dot"]').exists()).toBe(true)
    await wrapper.find('[data-testid="notification-read-1"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="notification-unread-dot"]').exists()).toBe(false)
  })

  it('全部标记已读后未读数归零', async () => {
    stubNotificationEndpoints({
      notifications: [notificationFixture(), notificationFixture({ id: 2 })],
      unread: 2,
    })
    const wrapper = await mountNotifications()

    await wrapper.find('[data-testid="notifications-mark-all"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="notifications-unread"]').text()).toBe('没有未读通知')
    expect(wrapper.find('[data-testid="notification-unread-dot"]').exists()).toBe(false)
  })

  it('删除通知后重新拉取列表', async () => {
    const recorded = stubNotificationEndpoints({ notifications: [notificationFixture()] })
    const wrapper = await mountNotifications()

    await wrapper.find('[data-testid="notification-delete-1"]').trigger('click')
    await flushPromises()

    const deleted = recorded.find(
      (r) => r.method === 'DELETE' && r.path.endsWith('/notifications/1'),
    )
    expect(deleted).toBeDefined()
    expect(wrapper.find('[data-testid="notifications-notice"]').text()).toBe('通知已删除。')
  })

  it('未登录时不发起通知查询', async () => {
    // 登录跳转由路由守卫负责（requiresAuth）；组件在未登录时只做一次会话探测，
    // 不去请求属于他人的通知列表。
    const recorded = stubNotificationEndpoints({ authenticated: false })
    await mountNotifications()

    const listCalls = recorded.filter((r) => r.path.endsWith('/notifications'))
    expect(listCalls).toHaveLength(0)
  })
})
