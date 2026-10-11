import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { authApi, type User, type UserRole } from '@/api/auth'
import { setUnauthorizedHandler, type UnauthorizedEvent } from '@/api/client'

export type AuthStatus = 'unknown' | 'guest' | 'authenticated'

/**
 * 登录态。首次导航时状态为 unknown，由标记为 silent 的探测请求解析为
 * 游客或已登录；silent 请求收到 401 只置为游客，绝不跳转（开发方案 §3.6）。
 */
export const useAuthStore = defineStore('auth', () => {
  const status = ref<AuthStatus>('unknown')
  const user = ref<User | null>(null)
  const redirectTarget = ref<string | null>(null)

  const isAuthenticated = computed(() => status.value === 'authenticated')
  const isVerified = computed(() => user.value?.status === 'normal')

  function markGuest(): void {
    status.value = 'guest'
    user.value = null
  }

  function markAuthenticated(next: User): void {
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

  /** fetchSession 是登录态探测；游客收到 401 属于预期结果。 */
  async function fetchSession(): Promise<void> {
    try {
      markAuthenticated(await authApi.session())
    } catch {
      markGuest()
    }
  }

  async function login(email: string, password: string): Promise<User> {
    const loggedIn = await authApi.login({ email, password })
    markAuthenticated(loggedIn)
    return loggedIn
  }

  async function logout(): Promise<void> {
    try {
      await authApi.logout()
    } finally {
      markGuest()
      redirectTarget.value = null
    }
  }

  async function register(email: string, password: string, inviteCode: string): Promise<void> {
    await authApi.register({
      email,
      password,
      invite_code: inviteCode === '' ? undefined : inviteCode,
    })
  }

  async function verifyEmail(email: string, code: string): Promise<void> {
    await authApi.verifyEmail({ email, code })
    await fetchSession()
  }

  async function updateProfile(input: {
    username?: string
    bio?: string
    avatar_url?: string
  }): Promise<void> {
    markAuthenticated(await authApi.updateProfile(input))
  }

  /** changePassword 后后端换发新会话，这里直接采用返回的账户信息。 */
  async function changePassword(oldPassword: string, newPassword: string): Promise<void> {
    markAuthenticated(
      await authApi.changePassword({ old_password: oldPassword, new_password: newPassword }),
    )
  }

  async function clearRedirectTarget(): Promise<void> {
    redirectTarget.value = null
  }

  function hasRole(...roles: UserRole[]): boolean {
    const role = user.value?.role
    return role !== undefined && roles.includes(role)
  }

  /** connect 把全局 401 事件接到本 store；由应用入口调用一次。 */
  function connect(): void {
    setUnauthorizedHandler(handleUnauthorized)
  }

  return {
    status,
    user,
    redirectTarget,
    isAuthenticated,
    isVerified,
    markGuest,
    markAuthenticated,
    reset,
    handleUnauthorized,
    fetchSession,
    login,
    logout,
    register,
    verifyEmail,
    updateProfile,
    changePassword,
    clearRedirectTarget,
    hasRole,
    connect,
  }
})
