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

  const isAuthenticated = computed(() => status.value === 'authenticated')
  const isVerified = computed(() => user.value?.status === 'normal')

  function markGuest(): void {
    status.value = 'guest'
    user.value = null
  }

  function markAuthenticated(next: User): void {
    status.value = 'authenticated'
    user.value = next
  }

  function reset(): void {
    status.value = 'unknown'
    user.value = null
  }

  /**
   * handleUnauthorized 响应全局 401。
   *
   * silent 请求是登录态探测，它收到的 401 只说明「此刻没有会话」。
   * 若用户已经通过登录或邮箱验证确立了状态，一次迟到的探测响应不应把它
   * 打回游客态，因此这里只在状态尚未解析时置为游客。
   * 非 silent 的 401 表示真正的操作被拒，一律落到游客态。
   */
  function handleUnauthorized(event: UnauthorizedEvent): void {
    if (event.silent) {
      if (status.value === 'unknown') {
        markGuest()
      }
      return
    }
    markGuest()
  }

  /**
   * inFlight 是正在进行的探测请求。路由守卫与应用启动都会发起探测，
   * 两者并发时共享同一次请求：迟到的响应不会覆盖已经解析出的状态，
   * 也不会让守卫在探测完成之前就判定为未登录。
   */
  let inFlight: Promise<void> | null = null

  /**
   * ensureSession 在状态未解析时探测一次登录态。
   *
   * 已解析时不再重复探测：邮箱验证、登录等操作会自行刷新状态，
   * 若这里再发一次探测，迟到的响应可能把刚登录的状态覆盖回游客。
   */
  async function ensureSession(): Promise<void> {
    if (status.value !== 'unknown') {
      return
    }
    if (inFlight !== null) {
      await inFlight
      return
    }

    inFlight = (async () => {
      try {
        markAuthenticated(await authApi.session())
      } catch {
        markGuest()
      } finally {
        inFlight = null
      }
    })()
    await inFlight
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

  function hasRole(...roles: UserRole[]): boolean {
    const role = user.value?.role
    return role !== undefined && roles.includes(role)
  }

  /**
   * connect 接入全局 401 事件，并解析一次登录态。
   *
   * 返回的 Promise 在探测完成后兑现：调用方 await 它就能拿到已解析的登录态，
   * 不必自己再发一次请求或轮询状态。状态已解析时立即兑现。
   */
  function connect(): Promise<void> {
    setUnauthorizedHandler(handleUnauthorized)
    return ensureSession()
  }

  return {
    status,
    user,
    isAuthenticated,
    isVerified,
    markGuest,
    markAuthenticated,
    reset,
    handleUnauthorized,
    fetchSession,
    ensureSession,
    login,
    logout,
    register,
    verifyEmail,
    updateProfile,
    changePassword,
    hasRole,
    connect,
  }
})
