import { flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { REPORT_REASONS } from '@/api/social'
import CommentSection from '@/components/CommentSection.vue'
import ReportDialog from '@/components/ReportDialog.vue'
import { errorEnvelope, mountView, resetLocale, stubResponse } from '@/testing/mount'

/** 记录一次请求，便于断言提交了哪些字段。 */
interface RecordedRequest {
  method: string
  path: string
  body: unknown
}

interface StubOptions {
  comments?: unknown[]
  replies?: Record<number, unknown[]>
  authenticated?: boolean
  reportStatus?: number
}

/** stubSocialEndpoints 按路径分派评论与举报端点。 */
function stubSocialEndpoints(options: StubOptions = {}): RecordedRequest[] {
  const recorded: RecordedRequest[] = []
  const comments = options.comments ?? []
  const replies = options.replies ?? {}

  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      const path = url.replace(/^https?:\/\/[^/]+/, '').split('?')[0] ?? ''
      const method = init?.method ?? 'GET'
      const body = init?.body === undefined ? undefined : JSON.parse(String(init.body))
      recorded.push({ method, path, body })

      if (path.endsWith('/session')) {
        return options.authenticated === true
          ? stubResponse({
              status: 200,
              body: {
                id: 1,
                username: 'alice',
                email: 'alice@example.com',
                role: 'user',
                status: 'normal',
                bio: '',
                avatar_url: '',
                email_verified: true,
              },
            })
          : stubResponse({ status: 401, body: errorEnvelope('unauthorized') })
      }

      const replyMatch = path.match(/^\/api\/v1\/comments\/(\d+)\/replies$/)
      if (replyMatch?.[1] !== undefined) {
        const id = Number.parseInt(replyMatch[1], 10)
        return stubResponse({
          status: 200,
          body: {
            items: replies[id] ?? [],
            page: 1,
            page_size: 20,
            total: (replies[id] ?? []).length,
          },
        })
      }

      if (path.endsWith('/comments')) {
        if (method === 'POST') {
          return stubResponse({ status: 200, body: { comment: { id: 99, body: '新评论' } } })
        }
        return stubResponse({
          status: 200,
          body: { items: comments, page: 1, page_size: 20, total: comments.length },
        })
      }

      if (path === '/api/v1/reports') {
        if (options.reportStatus !== undefined && options.reportStatus >= 400) {
          return stubResponse({
            status: options.reportStatus,
            body: errorEnvelope('invalid_state'),
          })
        }
        return stubResponse({
          status: 200,
          body: { id: 1, reason: String((body as { reason: string }).reason) },
        })
      }

      return stubResponse({ status: 404, body: errorEnvelope('not_found') })
    }),
  )
  return recorded
}

function commentFixture(overrides: Record<string, unknown> = {}): unknown {
  return {
    id: 10,
    prompt_id: 1,
    user_id: 7,
    author_name: 'bob',
    body: '一条评论',
    like_count: 0,
    liked: false,
    reply_count: 0,
    created_at: '2026-10-12T00:00:00Z',
    ...overrides,
  }
}

/** mountComments 挂载评论组件。 */
async function mountComments(): Promise<VueWrapper> {
  const wrapper = mountView(CommentSection, { props: { promptId: 1 } })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  setActivePinia(createPinia())
  resetLocale()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('评论列表', () => {
  it('游客可读评论但看不到输入框', async () => {
    stubSocialEndpoints({ authenticated: false, comments: [commentFixture()] })
    const wrapper = await mountComments()

    expect(wrapper.find('[data-testid="comments-list"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="comment-input"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="comments-login-hint"]').exists()).toBe(true)
  })

  it('没有评论时展示空态', async () => {
    stubSocialEndpoints({ authenticated: false })
    const wrapper = await mountComments()

    expect(wrapper.find('[data-testid="comments-empty"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="comments-list"]').exists()).toBe(false)
  })

  it('登录用户可发表评论', async () => {
    const recorded = stubSocialEndpoints({ authenticated: true })
    const wrapper = await mountComments()

    await wrapper.find('textarea[data-testid="comment-input"]').setValue('我的评论')
    await wrapper.find('[data-testid="comment-submit"]').trigger('click')
    await flushPromises()

    const posted = recorded.find((r) => r.method === 'POST' && r.path.endsWith('/comments'))
    expect(posted).toBeDefined()
    expect(posted?.body).toEqual({ body: '我的评论', parent_id: undefined })
  })

  it('发表回复时带上父评论 ID', async () => {
    const recorded = stubSocialEndpoints({ authenticated: true, comments: [commentFixture()] })
    const wrapper = await mountComments()

    await wrapper.find('[data-testid="comment-reply-10"]').trigger('click')
    await wrapper.find('textarea[data-testid="comment-reply-input-10"]').setValue('我的回复')
    await wrapper.find('[data-testid="comment-reply-submit-10"]').trigger('click')
    await flushPromises()

    const posted = recorded.find((r) => r.method === 'POST' && r.path.endsWith('/comments'))
    expect(posted?.body).toEqual({ body: '我的回复', parent_id: 10 })
  })

  it('展开回复时拉取该评论的回复', async () => {
    stubSocialEndpoints({
      authenticated: false,
      comments: [commentFixture({ reply_count: 1 })],
      replies: { 10: [commentFixture({ id: 11, body: '一条回复' })] },
    })
    const wrapper = await mountComments()

    expect(wrapper.find('[data-testid="replies-list"]').exists()).toBe(false)
    await wrapper.find('[data-testid="comment-replies-toggle-10"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="replies-list"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="replies-list"]').text()).toContain('一条回复')
  })

  it('点赞与取消点赞走不同方法', async () => {
    const recorded = stubSocialEndpoints({
      authenticated: true,
      comments: [commentFixture({ liked: true, like_count: 1 })],
    })
    const wrapper = await mountComments()

    await wrapper.find('[data-testid="comment-like-10"]').trigger('click')
    await flushPromises()

    const unliked = recorded.find((r) => r.method === 'DELETE' && r.path.endsWith('/like'))
    expect(unliked).toBeDefined()
  })

  it('只有自己的评论显示删除按钮', async () => {
    stubSocialEndpoints({
      authenticated: true,
      comments: [commentFixture({ user_id: 7 }), commentFixture({ id: 12, user_id: 1 })],
    })
    const wrapper = await mountComments()

    expect(wrapper.find('[data-testid="comment-delete-10"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="comment-delete-12"]').exists()).toBe(true)
  })
})

describe('举报对话框', () => {
  it('八种原因都以英文枚举键提交', async () => {
    const recorded = stubSocialEndpoints({ authenticated: true })
    const wrapper = mountView(ReportDialog, {
      props: { visible: true, objectType: 'comment', objectId: 10 },
    })
    await flushPromises()

    for (const reason of REPORT_REASONS) {
      const radio = wrapper.find(`[data-testid="report-reason-${reason}"]`)
      expect(radio.exists()).toBe(true)
    }

    await wrapper.find('[data-testid="report-reason-spam"] input').setValue(true)
    await wrapper.find('[data-testid="report-submit"]').trigger('click')
    await flushPromises()

    const posted = recorded.find((r) => r.path === '/api/v1/reports')
    expect(posted?.body).toMatchObject({
      object_type: 'comment',
      object_id: 10,
      reason: 'spam',
    })
  })

  it('英文界面下举报原因显示英文文案', async () => {
    stubSocialEndpoints({ authenticated: true })
    const { setLocale } = await import('@/i18n')
    setLocale('en')

    const wrapper = mountView(ReportDialog, {
      props: { visible: true, objectType: 'prompt', objectId: 1 },
    })
    await flushPromises()

    const spam = wrapper.find('[data-testid="report-reason-spam"]')
    expect(spam.text()).toBe('Spam')

    setLocale('zh')
  })

  it('提交失败时展示错误文案且不关闭对话框', async () => {
    stubSocialEndpoints({ authenticated: true, reportStatus: 422 })
    const wrapper = mountView(ReportDialog, {
      props: { visible: true, objectType: 'prompt', objectId: 1 },
    })
    await flushPromises()

    await wrapper.find('[data-testid="report-submit"]').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-testid="report-error"]').exists()).toBe(true)
  })
})
