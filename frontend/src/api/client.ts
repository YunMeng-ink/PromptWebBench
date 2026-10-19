// 接口调用封装：统一前缀、错误信封解析、CSRF 双提交、以及登录态探测的 401 语义。

export const API_PREFIX = '/api/v1'
export const CSRF_COOKIE_NAME = 'pb_csrf'
export const CSRF_HEADER_NAME = 'X-CSRF-Token'

export interface ErrorBody {
  code: string
  message: string
  details?: string[]
}

export interface ErrorEnvelope {
  error: ErrorBody
}

/** ApiError 保留稳定的错误码，界面按 error.<code> 呈现本地化文案。 */
export class ApiError extends Error {
  readonly code: string
  readonly status: number
  readonly details: string[]

  constructor(status: number, body: ErrorBody) {
    super(body.message)
    this.name = 'ApiError'
    this.code = body.code
    this.status = status
    this.details = body.details ?? []
  }
}

export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'

export interface RequestOptions {
  method?: HttpMethod
  body?: unknown
  /**
   * 登录态探测请求必须标记 silent：首次导航时游客必然收到 401，
   * 若拦截器对 401 一律跳转登录页，游客将无法浏览任何内容（开发方案 §3.6）。
   */
  silent?: boolean
  /**
   * 覆盖浏览器的缓存行为。字典等可被 CDN 缓存的只读接口在需要立即看到
   * 后台改动时传 no-store，使这次请求绕过浏览器与 CDN 的已有副本。
   */
  cache?: RequestCache
  signal?: AbortSignal
}

export interface UnauthorizedEvent {
  path: string
  silent: boolean
}

export type UnauthorizedHandler = (event: UnauthorizedEvent) => void

let unauthorizedHandler: UnauthorizedHandler | null = null

export function setUnauthorizedHandler(handler: UnauthorizedHandler | null): void {
  unauthorizedHandler = handler
}

function isWriteMethod(method: HttpMethod): boolean {
  return method === 'POST' || method === 'PUT' || method === 'PATCH' || method === 'DELETE'
}

function readCookie(name: string): string | null {
  if (typeof document === 'undefined') {
    return null
  }
  const target = `${name}=`
  for (const part of document.cookie.split(';')) {
    const trimmed = part.trim()
    if (trimmed.startsWith(target)) {
      return decodeURIComponent(trimmed.slice(target.length))
    }
  }
  return null
}

async function readPayload(response: Response): Promise<unknown> {
  const contentType = response.headers.get('Content-Type') ?? ''
  if (!contentType.includes('json')) {
    const text = await response.text()
    return text === '' ? null : text
  }
  try {
    return await response.json()
  } catch {
    return null
  }
}

function toApiError(status: number, payload: unknown): ApiError {
  if (typeof payload === 'object' && payload !== null && 'error' in payload) {
    const body = (payload as ErrorEnvelope).error
    if (typeof body?.code === 'string' && typeof body.message === 'string') {
      return new ApiError(status, body)
    }
  }
  return new ApiError(status, {
    code: 'internal',
    message: typeof payload === 'string' && payload !== '' ? payload : 'Internal server error',
  })
}

export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const method = options.method ?? 'GET'
  const headers: Record<string, string> = { Accept: 'application/json' }
  let body: BodyInit | null = null
  if (options.body !== undefined) {
    if (options.body instanceof FormData) {
      // multipart 边界由浏览器按 FormData 自动生成，不设置 Content-Type。
      body = options.body
    } else {
      headers['Content-Type'] = 'application/json'
      body = JSON.stringify(options.body)
    }
  }
  if (isWriteMethod(method)) {
    const token = readCookie(CSRF_COOKIE_NAME)
    if (token !== null) {
      headers[CSRF_HEADER_NAME] = token
    }
  }

  const response = await fetch(API_PREFIX + path, {
    method,
    headers,
    credentials: 'include',
    body,
    cache: options.cache,
    signal: options.signal ?? null,
  })

  if (response.status === 204) {
    return undefined as T
  }

  const payload = await readPayload(response)
  if (!response.ok) {
    const error = toApiError(response.status, payload)
    // 只有会话失效（unauthorized）才把界面打回游客态；401 下的业务错误
    // （如修改密码输错旧口令的 invalid_credentials）只作为普通错误抛出。
    if (response.status === 401 && error.code === 'unauthorized') {
      unauthorizedHandler?.({ path, silent: options.silent === true })
    }
    throw error
  }
  return payload as T
}

export const api = {
  get: <T>(path: string, options: Omit<RequestOptions, 'method' | 'body'> = {}) =>
    request<T>(path, { ...options, method: 'GET' }),
  post: <T>(path: string, body?: unknown, options: Omit<RequestOptions, 'method' | 'body'> = {}) =>
    request<T>(path, { ...options, method: 'POST', body }),
  put: <T>(path: string, body?: unknown, options: Omit<RequestOptions, 'method' | 'body'> = {}) =>
    request<T>(path, { ...options, method: 'PUT', body }),
  patch: <T>(path: string, body?: unknown, options: Omit<RequestOptions, 'method' | 'body'> = {}) =>
    request<T>(path, { ...options, method: 'PATCH', body }),
  delete: <T>(
    path: string,
    body?: unknown,
    options: Omit<RequestOptions, 'method' | 'body'> = {},
  ) => request<T>(path, { ...options, method: 'DELETE', body }),
}
