import { mount, type VueWrapper } from '@vue/test-utils'
import ElementPlus from 'element-plus'
import { createPinia, type Pinia, setActivePinia } from 'pinia'
import type { Component } from 'vue'
import type { Router } from 'vue-router'
import { i18n, setLocale } from '@/i18n'

export interface MountOptions {
  router?: Router
  pinia?: Pinia
}

/** mountView 统一装配界面依赖：Pinia、i18n、Element Plus，以及可选的真实路由。 */
export function mountView(component: Component, options: MountOptions = {}): VueWrapper {
  const pinia = options.pinia ?? createPinia()
  setActivePinia(pinia)

  const plugins: unknown[] = [pinia, i18n, ElementPlus]
  if (options.router !== undefined) {
    plugins.push(options.router)
  }

  return mount(component, {
    global: {
      plugins: plugins as never,
      stubs: options.router === undefined ? { RouterLink: { template: '<a><slot /></a>' } } : {},
    },
  })
}

export interface StubResponseInit {
  status: number
  body?: unknown
  contentType?: string
}

/** jsonResponse 构造与真实响应同形状的对象，供 fetch 桩返回。 */
export function stubResponse({
  status,
  body,
  contentType = 'application/json',
}: StubResponseInit): Response {
  const text = body === undefined ? '' : typeof body === 'string' ? body : JSON.stringify(body)
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: new Headers({ 'Content-Type': contentType }),
    json: async () => JSON.parse(text),
    text: async () => text,
  } as unknown as Response
}

/** errorEnvelope 构造后端统一的错误响应体。 */
export function errorEnvelope(code: string, message = ''): unknown {
  return { error: { code, message } }
}

export function currentUserFixture(overrides: Record<string, unknown> = {}): unknown {
  return {
    id: 1,
    username: 'alice',
    email: 'alice@example.com',
    role: 'user',
    status: 'normal',
    bio: '',
    avatar_url: '',
    email_verified: true,
    ...overrides,
  }
}

export function resetLocale(): void {
  localStorage.clear()
  setLocale('zh')
  document.documentElement.classList.remove('dark')
}
