import { flushPromises } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
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

/** openMoreMenu 打开宽屏「更多」菜单并等待浮层渲染。 */
async function openMoreMenu(wrapper: ReturnType<typeof setup>['wrapper']): Promise<void> {
  await wrapper.find('[data-testid="header-more"]').trigger('click')
  await flushPromises()
}

beforeEach(() => {
  resetLocale()
})

afterEach(() => {
  for (const node of document.querySelectorAll('.el-popper')) {
    node.remove()
  }
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

  it('普通用户的更多菜单中没有后台入口', async () => {
    const { wrapper, auth } = setup()
    auth.markAuthenticated({ ...(currentUserFixture() as object), role: 'user' } as never)
    await nextTick()
    await openMoreMenu(wrapper)

    expect(document.body.querySelector('[data-testid="header-admin"]')).toBeNull()
    expect(document.body.querySelector('[data-testid="header-libraries"]')).not.toBeNull()
  })

  it('管理员的更多菜单包含后台入口', async () => {
    const { wrapper, auth } = setup()
    auth.markAuthenticated({ ...(currentUserFixture() as object), role: 'admin' } as never)
    await nextTick()
    await openMoreMenu(wrapper)

    const item = document.body.querySelector('[data-testid="header-admin"]')
    expect(item).not.toBeNull()
    expect(item?.textContent).toContain('管理后台')
  })

  it('通知入口以铃铛形式固定在工具区', async () => {
    const { wrapper, auth } = setup()
    auth.markAuthenticated(currentUserFixture() as never)
    await nextTick()

    const bell = wrapper.find('[data-testid="header-notifications"]')
    expect(bell.exists()).toBe(true)
    expect(bell.attributes('href')).toBe('/notifications')
    expect(bell.attributes('aria-label')).toBe('通知中心')
  })
})
