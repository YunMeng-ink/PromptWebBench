import { api, type RequestOptions } from './client'

export type UserRole = 'user' | 'developer' | 'admin'
export type UserStatus = 'normal' | 'unverified' | 'banned'

/** User 与后端 userDTO 对应。 */
export interface User {
  id: number
  username: string
  email: string
  role: UserRole
  status: UserStatus
  bio: string
  avatar_url: string
  email_verified: boolean
}

export interface APITokenStatus {
  present: boolean
  prefix?: string
  created_at?: string
  expires_at?: string
  expired?: boolean
}

export interface APIToken {
  token: string
  prefix: string
}

/** MeExport 是「我的数据」导出的结构（G-08）。字段与后端 Export 对应。 */
export interface MeExport {
  username: string
  filename: string
  prompts: {
    id: number
    title: string
    body: string
    description: string
    visibility: string
    sensitive_level: string
    version_no: string
    category_name_zh: string
    category_name_en: string
    created_at: string
    updated_at: string
  }[]
  ratings: {
    id: number
    prompt_id: number
    prompt_title: string
    model_name: string
    score: number
    edit_count: number
    created_at: string
    updated_at: string
  }[]
  comments: {
    id: number
    prompt_id: number
    prompt_title: string
    parent_id?: number
    body: string
    created_at: string
  }[]
}

/** authApi 覆盖认证、个人资料与 API Token 的全部端点。 */
export const authApi = {
  register: (body: { email: string; password: string; invite_code?: string }) =>
    api.post<User>('/auth/register', body),

  verifyEmail: (body: { email: string; code: string }) =>
    api.post<User>('/auth/verify-email', body),

  resendVerification: (body: { email: string }) =>
    api.post<void>('/auth/resend-verification', body),

  login: (body: { email: string; password: string }) => api.post<User>('/auth/login', body),

  logout: () => api.post<User>('/auth/logout'),

  // 登录态探测必须标记 silent：游客必然收到 401，此时只置为游客态，不跳转。
  session: (options: Omit<RequestOptions, 'method' | 'body'> = {}) =>
    api.get<User>('/auth/session', { ...options, silent: true }),

  requestPasswordReset: (body: { email: string }) =>
    api.post<void>('/auth/password/reset-request', body),

  resetPassword: (body: { email: string; code: string; new_password: string }) =>
    api.post<void>('/auth/password/reset', body),

  updateProfile: (body: { username?: string; bio?: string; avatar_url?: string }) =>
    api.patch<User>('/me', body),

  changePassword: (body: { old_password: string; new_password: string }) =>
    api.post<User>('/me/password', body),

  apiTokenStatus: () => api.get<APITokenStatus>('/me/api-token'),

  createApiToken: () => api.post<APIToken>('/me/api-token'),

  revokeApiToken: () => api.delete<void>('/me/api-token'),

  exportMyData: () => api.get<MeExport>('/me/export'),

  deleteAccount: (confirm: string) => api.delete<void>('/me', { confirm }),
}

/** gitHubAuthorizePath 是后端授权跳转入口，浏览器直接导航到它。 */
export const gitHubAuthorizePath = '/api/v1/auth/oauth/github/start'
