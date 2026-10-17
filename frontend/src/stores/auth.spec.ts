import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory } from 'vue-router'
import { setUnauthorizedHandler } from '@/api/client'
import { createAppRouter } from '@/router'
import { useAuthStore } from '@/stores/auth'
import { currentUserFixture, errorEnvelope, stubResponse } from '@/testing/mount'

const fetchMock = vi.fn()

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
  setUnauthorizedHandler(null)
  setActivePinia(createPinia())
})

afterEach(() => {
  setUnauthorizedHandler(null)
})

describe('登录守卫', () => {
  it('游客访问需要登录的页面时跳转登录页并带上原始地址', async () => {
    fetchMock.mockResolvedValue(
      stubResponse({ status: 401, body: errorEnvelope('unauthorized', '未登录') }),
    )
    const router = createAppRouter(createMemoryHistory())
    useAuthStore().connect()

    await router.push('/me')
    await router.isReady()

    expect(router.currentRoute.value.name).toBe('login')
    expect(router.currentRoute.value.query.redirect).toBe('/me')
  })

  it('已登录用户可以进入受保护页面', async () => {
    fetchMock.mockResolvedValue(stubResponse({ status: 200, body: currentUserFixture() }))
    const router = createAppRouter(createMemoryHistory())
    useAuthStore().connect()

    await router.push('/me')
    await router.isReady()

    expect(router.currentRoute.value.name).toBe('me')
  })

  it('公开页面不触发登录态探测', async () => {
    const router = createAppRouter(createMemoryHistory())

    await router.push('/login')
    await router.isReady()

    expect(router.currentRoute.value.name).toBe('login')
    expect(fetchMock).not.toHaveBeenCalled()
  })
})

describe('登录态 store', () => {
  it('silent 探测收到 401 时置为游客且不跳转', async () => {
    fetchMock.mockResolvedValue(
      stubResponse({ status: 401, body: errorEnvelope('unauthorized', '未登录') }),
    )
    const auth = useAuthStore()
    auth.connect()

    await auth.fetchSession()

    expect(auth.status).toBe('guest')
    expect(auth.user).toBeNull()
  })

  it('探测成功时记录账户信息', async () => {
    fetchMock.mockResolvedValue(stubResponse({ status: 200, body: currentUserFixture() }))
    const auth = useAuthStore()

    await auth.fetchSession()

    expect(auth.status).toBe('authenticated')
    expect(auth.isAuthenticated).toBe(true)
    expect(auth.isVerified).toBe(true)
    expect(auth.user?.email).toBe('alice@example.com')
  })

  it('非 silent 请求收到 401 时落到游客态', async () => {
    const auth = useAuthStore()
    auth.connect()
    auth.markAuthenticated(currentUserFixture() as never)
    fetchMock.mockResolvedValue(
      stubResponse({ status: 401, body: errorEnvelope('unauthorized', '未登录') }),
    )

    await auth.updateProfile({ bio: 'x' }).catch(() => undefined)

    expect(auth.status).toBe('guest')
    expect(auth.user).toBeNull()
  })

  it('登出后回到游客态', async () => {
    fetchMock.mockResolvedValue(stubResponse({ status: 200, body: currentUserFixture() }))
    const auth = useAuthStore()
    await auth.fetchSession()

    fetchMock.mockResolvedValue(stubResponse({ status: 200, body: currentUserFixture() }))
    await auth.logout()

    expect(auth.status).toBe('guest')
    expect(auth.user).toBeNull()
  })

  it('未验证账户的 isVerified 为假', async () => {
    fetchMock.mockResolvedValue(
      stubResponse({
        status: 200,
        body: currentUserFixture({ status: 'unverified', email_verified: false }),
      }),
    )
    const auth = useAuthStore()

    await auth.fetchSession()

    expect(auth.isAuthenticated).toBe(true)
    expect(auth.isVerified).toBe(false)
  })

  it('hasRole 按身份组判定', async () => {
    fetchMock.mockResolvedValue(
      stubResponse({ status: 200, body: currentUserFixture({ role: 'developer' }) }),
    )
    const auth = useAuthStore()
    await auth.fetchSession()

    expect(auth.hasRole('developer')).toBe(true)
    expect(auth.hasRole('admin')).toBe(false)
    expect(auth.hasRole('user', 'developer')).toBe(true)
  })
})
