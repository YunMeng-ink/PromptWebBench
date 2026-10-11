import { flushPromises } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory } from 'vue-router'
import { API_PREFIX } from '@/api/client'
import { createAppRouter } from '@/router'
import { useAuthStore } from '@/stores/auth'
import { currentUserFixture, mountView, resetLocale, stubResponse } from '@/testing/mount'
import MeView from '@/views/MeView.vue'

const fetchMock = vi.fn()

interface StubCall {
  url: string
  method: string
}

function calls(): StubCall[] {
  return fetchMock.mock.calls.map((call) => ({
    url: String(call[0]),
    method: ((call[1] as RequestInit | undefined)?.method ?? 'GET') as string,
  }))
}

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
  resetLocale()
})

async function setup(body: unknown = currentUserFixture()) {
  // 受保护页面的路由守卫会读取登录态，因此先建好 Pinia 并置为已登录。
  const pinia = createPinia()
  setActivePinia(pinia)

  const auth = useAuthStore()
  auth.markAuthenticated(body as never)

  const router = createAppRouter(createMemoryHistory())
  await router.push('/me')
  await router.isReady()

  const wrapper = mountView(MeView, { router, pinia })
  await flushPromises()
  return { wrapper, auth, router }
}

function routeByFetch(handlers: Record<string, () => Response>): void {
  fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const method = init?.method ?? 'GET'
    const key = `${method} ${url.replace(API_PREFIX, '')}`
    const handler = handlers[key]
    if (handler === undefined) {
      throw new Error(`未预期的请求: ${key}`)
    }
    return handler()
  })
}

describe('个人中心', () => {
  it('未验证账户显示限制提示', async () => {
    routeByFetch({
      'GET /me/api-token': () => stubResponse({ status: 200, body: { present: false } }),
    })
    const { wrapper } = await setup(
      currentUserFixture({ status: 'unverified', email_verified: false }),
    )

    expect(wrapper.find('[data-testid="me-unverified"]').exists()).toBe(true)
  })

  it('保存资料后提示成功并刷新本地账户信息', async () => {
    routeByFetch({
      'GET /me/api-token': () => stubResponse({ status: 200, body: { present: false } }),
      'PATCH /me': () =>
        stubResponse({
          status: 200,
          body: currentUserFixture({ username: '新名字', bio: '简介' }),
        }),
    })
    const { wrapper, auth } = await setup()

    await wrapper.get('input[data-testid="me-username-input"]').setValue('新名字')
    await wrapper.get('[data-testid="me-profile-form"]').trigger('submit')
    await flushPromises()

    expect(auth.user?.username).toBe('新名字')
    expect(wrapper.text()).toContain('资料已更新')
    expect(calls().some((call) => call.method === 'PATCH' && call.url.endsWith('/me'))).toBe(true)
  })

  it('生成令牌后仅展示一次明文，撤销后清除', async () => {
    let tokenPresent = false
    routeByFetch({
      'GET /me/api-token': () =>
        stubResponse({
          status: 200,
          body: tokenPresent ? { present: true, prefix: 'pb_secret' } : { present: false },
        }),
      'POST /me/api-token': () => {
        tokenPresent = true
        return stubResponse({
          status: 200,
          body: { token: 'pb_secret_token', prefix: 'pb_secret' },
        })
      },
      'DELETE /me/api-token': () => {
        tokenPresent = false
        return stubResponse({ status: 204 })
      },
    })
    const { wrapper } = await setup()

    expect(wrapper.find('[data-testid="me-token-absent"]').exists()).toBe(true)

    await wrapper.get('[data-testid="me-token-create"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('input[data-testid="me-token-plain"]').element).toHaveProperty(
      'value',
      'pb_secret_token',
    )
    expect(wrapper.find('[data-testid="me-token-present"]').exists()).toBe(true)

    await wrapper.get('[data-testid="me-token-revoke"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('input[data-testid="me-token-plain"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="me-token-absent"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('令牌已撤销')
  })

  it('已存在令牌时展示前缀而非明文', async () => {
    routeByFetch({
      'GET /me/api-token': () =>
        stubResponse({ status: 200, body: { present: true, prefix: 'pb_abcdef12' } }),
    })
    const { wrapper } = await setup()

    expect(wrapper.get('[data-testid="me-token-present"]').text()).toContain('pb_abcdef12')
    expect(wrapper.find('input[data-testid="me-token-plain"]').exists()).toBe(false)
  })

  it('两次口令不一致时本地拦截，不发起请求', async () => {
    routeByFetch({
      'GET /me/api-token': () => stubResponse({ status: 200, body: { present: false } }),
    })
    const { wrapper } = await setup()
    const before = calls().length

    await wrapper.get('input[data-testid="me-old-password"]').setValue('OldPassw0rd!')
    await wrapper.get('input[data-testid="me-new-password"]').setValue('NewPassw0rd!')
    await wrapper.get('input[data-testid="me-confirm-password"]').setValue('Different1!')
    await wrapper.get('[data-testid="me-password-form"]').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('两次输入的口令不一致')
    expect(calls().length).toBe(before)
  })
})
