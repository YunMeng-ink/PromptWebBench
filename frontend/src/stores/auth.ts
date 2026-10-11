import { defineStore } from 'pinia'
import { ref } from 'vue'
import { setUnauthorizedHandler, type UnauthorizedEvent } from '@/api/client'

export type AuthStatus = 'unknown' | 'guest' | 'authenticated'
export type UserRole = 'user' | 'developer' | 'admin'

export interface SessionUser {
  id: number
  username: string
  role: UserRole
  status: string
}

/**
 * 登录态。首次导航时状态为 unknown，由标记为 silent 的探测请求解析为
 * 游客或已登录；silent 请求收到 401 只置为游客，绝不跳转（开发方案 §3.6）。
 */
export const useAuthStore = defineStore('auth', () => {
  const status = ref<AuthStatus>('unknown')
  const user = ref<SessionUser | null>(null)
  const redirectTarget = ref<string | null>(null)

  function markGuest(): void {
    status.value = 'guest'
    user.value = null
  }

  function markAuthenticated(next: SessionUser): void {
    status.value = 'authenticated'
    user.value = next
    redirectTarget.value = null
  }

  function reset(): void {
    status.value = 'unknown'
    user.value = null
    redirectTarget.value = null
  }

  function handleUnauthorized(event: UnauthorizedEvent): void {
    if (event.silent) {
      markGuest()
      return
    }
    status.value = 'guest'
    user.value = null
    redirectTarget.value = event.path
  }

  /** 把全局 401 事件接到本 store；由应用入口调用一次。 */
  function connect(): void {
    setUnauthorizedHandler(handleUnauthorized)
  }

  return {
    status,
    user,
    redirectTarget,
    markGuest,
    markAuthenticated,
    reset,
    handleUnauthorized,
    connect,
  }
})
