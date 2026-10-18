import { flushPromises } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '@/router'
import {
  currentUserFixture,
  errorEnvelope,
  mountView,
  resetLocale,
  stubResponse,
} from '@/testing/mount'
import LoginView from '@/views/LoginView.vue'

const fetchMock = vi.fn()

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
  resetLocale()
})

async function setup(initialPath = '/login') {
  const router = createAppRouter(createMemoryHistory())
  await router.push(initialPath)
  await router.isReady()
  const wrapper = mountView(LoginView, { router })
  await flushPromises()
  return { wrapper, router }
}

describe('登录页', () => {
  it('登录成功后跳转到 redirect 指定的地址', async () => {
    fetchMock.mockResolvedValue(stubResponse({ status: 200, body: currentUserFixture() }))
    const { wrapper, router } = await setup('/login?redirect=/me')

    await wrapper.get('input[data-testid="login-email"]').setValue('alice@example.com')
    await wrapper.get('input[data-testid="login-password"]').setValue('Password1!')
    await wrapper.get('[data-testid="login-form"]').trigger('submit')
    await flushPromises()

    // 目标视图按路由拆分，跳转需要等动态模块加载完成。
    await vi.waitFor(() => expect(router.currentRoute.value.fullPath).toBe('/me'))
  })

  it('凭据错误时展示本地化提示且不跳转', async () => {
    fetchMock.mockResolvedValue(
      stubResponse({ status: 401, body: errorEnvelope('invalid_credentials', '邮箱或密码错误') }),
    )
    const { wrapper, router } = await setup()

    await wrapper.get('input[data-testid="login-email"]').setValue('alice@example.com')
    await wrapper.get('input[data-testid="login-password"]').setValue('WrongPass1!')
    await wrapper.get('[data-testid="login-form"]').trigger('submit')
    await flushPromises()

    expect(wrapper.text()).toContain('邮箱或密码错误')
    expect(router.currentRoute.value.name).toBe('login')
  })

  it('提供注册、找回口令与 GitHub 入口', async () => {
    const { wrapper } = await setup()

    expect(wrapper.text()).toContain('还没有账号？去注册')
    expect(wrapper.text()).toContain('忘记口令？')
    expect(wrapper.get('a.pb-auth__oauth').attributes('href')).toBe(
      '/api/v1/auth/oauth/github/start',
    )
  })
})
