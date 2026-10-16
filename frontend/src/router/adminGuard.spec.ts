// 后台路由守卫：非管理员访问 /admin 下的任何页面都会被弹回首页。
// 这是后端中间件之外的第二道防线（阶段 6 验收标准：前端守卫 + 后端中间件双重拦截）。

import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory } from 'vue-router'
import { createAppRouter } from '@/router'
import { resetLocale, stubResponse } from '@/testing/mount'

/** stubSession 让登录态探测返回给定身份的账户。 */
function stubSession(role: 'guest' | 'user' | 'admin'): void {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      const path = url.replace(/^https?:\/\/[^/]+/, '').split('?')[0] ?? ''
      if (path.endsWith('/session')) {
        if (role === 'guest') {
          return stubResponse({
            status: 401,
            body: { error: { code: 'unauthorized', message: '' } },
          })
        }
        return stubResponse({
          status: 200,
          body: {
            id: 1,
            username: 'alice',
            email: 'alice@example.com',
            role,
            status: 'normal',
            bio: '',
            avatar_url: '',
            email_verified: true,
          },
        })
      }
      // 后台页面若被真正加载，其请求会走到这里；测试只关心是否发生了跳转。
      return stubResponse({ status: 200, body: { items: [], page: 1, page_size: 20, total: 0 } })
    }),
  )
}

beforeEach(() => {
  resetLocale()
  // 守卫通过 useAuthStore 读取登录态，因此必须先有活动的 Pinia 实例。
  setActivePinia(createPinia())
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('后台路由守卫', () => {
  it('普通用户被弹回首页', async () => {
    stubSession('user')
    const router = createAppRouter(createMemoryHistory())

    await router.push('/admin/settings')
    expect(router.currentRoute.value.name).toBe('home')
  })

  it('游客被引导到登录页并带上来源地址', async () => {
    stubSession('guest')
    const router = createAppRouter(createMemoryHistory())

    await router.push('/admin/prompts')
    expect(router.currentRoute.value.name).toBe('login')
    expect(router.currentRoute.value.query.redirect).toBe('/admin/prompts')
  })

  it('管理员可以进入后台', async () => {
    stubSession('admin')
    const router = createAppRouter(createMemoryHistory())

    await router.push('/admin/dashboard')
    expect(router.currentRoute.value.name).toBe('admin-dashboard')
  })

  it('/admin 默认重定向到统计仪表盘', async () => {
    stubSession('admin')
    const router = createAppRouter(createMemoryHistory())

    await router.push('/admin')
    expect(router.currentRoute.value.name).toBe('admin-dashboard')
  })
})
