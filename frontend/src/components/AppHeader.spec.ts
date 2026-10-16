import { beforeEach, describe, expect, it } from 'vitest'
import { nextTick } from 'vue'
import { createMemoryHistory } from 'vue-router'
import AppHeader from '@/components/AppHeader.vue'
import { createAppRouter } from '@/router'
import { useAuthStore } from '@/stores/auth'
import { usePreferencesStore } from '@/stores/preferences'
import { currentUserFixture, mountView, resetLocale } from '@/testing/mount'

function setup() {
  const router = createAppRouter(createMemoryHistory())
  const wrapper = mountView(AppHeader, { router })
  return { wrapper, preferences: usePreferencesStore(), auth: useAuthStore() }
}

beforeEach(() => {
  resetLocale()
})

describe('顶栏', () => {
  it('渲染平台名称与标语', () => {
    const { wrapper } = setup()
    expect(wrapper.text()).toContain('Prompt Benchmark')
    expect(wrapper.text()).toContain('提示词能力边界评测平台')
  })

  it('切换语言后文案随之改变', async () => {
    const { wrapper, preferences } = setup()
    preferences.setLocale('en')
    await nextTick()

    expect(wrapper.text()).toContain('Benchmarking the capability boundaries of language models')
    expect(wrapper.text()).toContain('Home')
    expect(wrapper.text()).not.toContain('提示词能力边界评测平台')
  })

  it('主题切换写入根元素类名与本地存储', async () => {
    const { wrapper } = setup()
    expect(document.documentElement.classList.contains('dark')).toBe(false)

    await wrapper.get('button').trigger('click')

    expect(document.documentElement.classList.contains('dark')).toBe(true)
    expect(localStorage.getItem('pb.theme')).toBe('dark')
  })

  it('主题按钮的可访问名称随语言切换', () => {
    const { wrapper } = setup()
    expect(wrapper.get('button').attributes('aria-label')).toBe('主题')
  })

  it('登录态未知时不渲染登录或账户入口，避免首屏显示错误状态', () => {
    const { wrapper } = setup()
    expect(wrapper.find('[data-testid="header-login"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="header-user"]').exists()).toBe(false)
  })

  it('游客显示登录入口', async () => {
    const { wrapper, auth } = setup()
    auth.markGuest()
    await nextTick()

    expect(wrapper.find('[data-testid="header-login"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="header-user"]').exists()).toBe(false)
  })

  it('已登录显示账户入口与登出项', async () => {
    const { wrapper, auth } = setup()
    auth.markAuthenticated(currentUserFixture() as never)
    await nextTick()

    expect(wrapper.find('[data-testid="header-user"]').text()).toBe('alice')
    expect(wrapper.find('[data-testid="header-login"]').exists()).toBe(false)
  })

  it('后台入口只对管理员出现', async () => {
    const { wrapper, auth } = setup()
    auth.markAuthenticated({ ...(currentUserFixture() as object), role: 'user' } as never)
    await nextTick()
    expect(wrapper.find('[data-testid="header-admin"]').exists()).toBe(false)

    auth.markAuthenticated({ ...(currentUserFixture() as object), role: 'admin' } as never)
    await nextTick()
    const link = wrapper.find('[data-testid="header-admin"]')
    expect(link.exists()).toBe(true)
    expect(link.attributes('href')).toBe('/admin')
  })
})
