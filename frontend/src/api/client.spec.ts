import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ApiError,
  api,
  CSRF_COOKIE_NAME,
  CSRF_HEADER_NAME,
  setUnauthorizedHandler,
} from '@/api/client'

interface StubResponseInit {
  status: number
  body?: unknown
  contentType?: string
}

function stubResponse({
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

const fetchMock = vi.fn()

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
  document.cookie = `${CSRF_COOKIE_NAME}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/`
  setUnauthorizedHandler(null)
})

afterEach(() => {
  setUnauthorizedHandler(null)
})

describe('接口调用封装', () => {
  it('成功响应返回解析后的内容', async () => {
    fetchMock.mockResolvedValue(stubResponse({ status: 200, body: { status: 'ok' } }))

    const result = await api.get<{ status: string }>('/health')

    expect(result).toEqual({ status: 'ok' })
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/health',
      expect.objectContaining({ method: 'GET', credentials: 'include' }),
    )
  })

  it('204 响应返回 undefined', async () => {
    fetchMock.mockResolvedValue(stubResponse({ status: 204 }))

    await expect(api.delete('/anything')).resolves.toBeUndefined()
  })

  it('错误信封被解析为带稳定错误码的 ApiError', async () => {
    fetchMock.mockResolvedValue(
      stubResponse({
        status: 422,
        body: {
          error: {
            code: 'validation_failed',
            message: '请求字段校验未通过',
            details: ['body.score: 必须介于 1 与 5 之间'],
          },
        },
      }),
    )

    const error = await api.post('/prompts', {}).catch((thrown: unknown) => thrown)

    expect(error).toBeInstanceOf(ApiError)
    const apiError = error as ApiError
    expect(apiError.code).toBe('validation_failed')
    expect(apiError.status).toBe(422)
    expect(apiError.details).toHaveLength(1)
  })

  it('非 JSON 错误体回退为 internal 码并保留原文', async () => {
    fetchMock.mockResolvedValue(
      stubResponse({ status: 502, body: '<html>bad gateway</html>', contentType: 'text/html' }),
    )

    const error = (await api.get('/probe').catch((thrown: unknown) => thrown)) as ApiError

    expect(error.code).toBe('internal')
    expect(error.message).toContain('bad gateway')
  })

  it('写方法携带 CSRF 双提交头', async () => {
    document.cookie = `${CSRF_COOKIE_NAME}=token-123; path=/`
    fetchMock.mockResolvedValue(stubResponse({ status: 200, body: {} }))

    await api.post('/probe', { value: 1 })

    const call = fetchMock.mock.calls[0]
    const headers = (call?.[1] as RequestInit | undefined)?.headers as Record<string, string>
    expect(headers[CSRF_HEADER_NAME]).toBe('token-123')
    expect(headers['Content-Type']).toBe('application/json')
  })

  it('无 CSRF Cookie 时不发送该校验头', async () => {
    fetchMock.mockResolvedValue(stubResponse({ status: 200, body: {} }))

    await api.post('/probe', {})

    const call = fetchMock.mock.calls[0]
    const headers = (call?.[1] as RequestInit | undefined)?.headers as Record<string, string>
    expect(headers[CSRF_HEADER_NAME]).toBeUndefined()
  })

  it('读方法不发送 CSRF 头', async () => {
    document.cookie = `${CSRF_COOKIE_NAME}=token-123; path=/`
    fetchMock.mockResolvedValue(stubResponse({ status: 200, body: {} }))

    await api.get('/probe')

    const call = fetchMock.mock.calls[0]
    const headers = (call?.[1] as RequestInit | undefined)?.headers as Record<string, string>
    expect(headers[CSRF_HEADER_NAME]).toBeUndefined()
  })
})

describe('登录态探测的 401 语义', () => {
  it('silent 请求收到 401 时以 silent 标记通知调用方', async () => {
    const handler = vi.fn()
    setUnauthorizedHandler(handler)
    fetchMock.mockResolvedValue(
      stubResponse({ status: 401, body: { error: { code: 'unauthorized', message: '未登录' } } }),
    )

    await api.get('/me', { silent: true }).catch(() => undefined)

    expect(handler).toHaveBeenCalledWith({ path: '/me', silent: true })
  })

  it('非 silent 请求收到 401 时标记为需要跳转登录', async () => {
    const handler = vi.fn()
    setUnauthorizedHandler(handler)
    fetchMock.mockResolvedValue(
      stubResponse({ status: 401, body: { error: { code: 'unauthorized', message: '未登录' } } }),
    )

    await api.post('/ratings', { score: 5 }).catch(() => undefined)

    expect(handler).toHaveBeenCalledWith({ path: '/ratings', silent: false })
  })

  it('非 401 错误不触发 401 处理', async () => {
    const handler = vi.fn()
    setUnauthorizedHandler(handler)
    fetchMock.mockResolvedValue(
      stubResponse({ status: 403, body: { error: { code: 'forbidden', message: '权限不足' } } }),
    )

    await api.get('/me').catch(() => undefined)

    expect(handler).not.toHaveBeenCalled()
  })

  it('401 业务错误（invalid_credentials）不触发 401 处理', async () => {
    const handler = vi.fn()
    setUnauthorizedHandler(handler)
    fetchMock.mockResolvedValue(
      stubResponse({
        status: 401,
        body: { error: { code: 'invalid_credentials', message: '邮箱或密码错误' } },
      }),
    )

    const error = (await api
      .patch('/me/password', { old: 'x' })
      .catch((thrown: unknown) => thrown)) as ApiError

    expect(handler).not.toHaveBeenCalled()
    expect(error.code).toBe('invalid_credentials')
  })

  it('FormData 写请求不设置 JSON 头且携带 CSRF 头', async () => {
    document.cookie = `${CSRF_COOKIE_NAME}=token-123; path=/`
    fetchMock.mockResolvedValue(stubResponse({ status: 200, body: {} }))

    const form = new FormData()
    form.append('file', new File(['x'], 'a.md'))
    await api.post('/prompts/import-markdown', form)

    const call = fetchMock.mock.calls[0]
    const headers = (call?.[1] as RequestInit | undefined)?.headers as Record<string, string>
    expect(headers[CSRF_HEADER_NAME]).toBe('token-123')
    expect(headers['Content-Type']).toBeUndefined()
    expect((call?.[1] as RequestInit | undefined)?.body).toBe(form)
  })
})
