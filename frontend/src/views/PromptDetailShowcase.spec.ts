import { flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import {
  currentUserFixture,
  errorEnvelope,
  mountView,
  resetLocale,
  stubResponse,
} from '@/testing/mount'
import PromptDetailView from '@/views/PromptDetailView.vue'

const fetchMock = vi.fn()

interface RecordedCall {
  url: string
  method: string
  body: unknown
}

function calls(): RecordedCall[] {
  return fetchMock.mock.calls.map((call) => ({
    url: String(call[0]),
    method: ((call[1] as RequestInit | undefined)?.method ?? 'GET') as string,
    body: (call[1] as RequestInit | undefined)?.body,
  }))
}

const detailFixture = {
  prompt: {
    id: 1,
    title: '展示素材提示词',
    description: '描述',
    purpose_id: 1,
    capability_id: 1,
    visibility: 'public',
    sensitive_level: 'low',
    uploader_id: 7,
    version_no: 'v1.0',
    major_modified: false,
    comment_count: 0,
    favorite_count: 0,
    created_at: '2026-10-01T00:00:00Z',
    updated_at: '2026-10-02T00:00:00Z',
  },
  version: {
    id: 1,
    version_no: 'v1.0',
    title: '展示素材提示词',
    body: '提示词正文',
    description: '描述',
    purpose_id: 1,
    capability_id: 1,
    visibility: 'public',
    sensitive_level: 'low',
    is_major: false,
    change_note: '',
    author_id: 7,
    created_at: '2026-10-01T00:00:00Z',
  },
  versions: [],
  purpose: {
    id: 1,
    slug: 'capability_boundary',
    name_zh: '边界探测',
    name_en: 'Capability Boundary',
    description_zh: '',
    description_en: '',
    is_high_risk: false,
  },
  capability: {
    id: 1,
    slug: 'reasoning',
    name_zh: '推理',
    name_en: 'Reasoning',
    group_key: 'cognitive',
    description_zh: '',
    description_en: '',
  },
  tags: [],
  uploader: 'alice',
}

const statsFixture = {
  prompt_id: 1,
  history: {
    sufficient: false,
    discrimination: null,
    confidence: 'low',
    valid_model_count: 0,
    total_rating_count: 0,
  },
  recent: {
    sufficient: false,
    discrimination: null,
    confidence: 'low',
    valid_model_count: 0,
    total_rating_count: 0,
  },
  models: [],
  recommendation: [],
  computed_at: '2026-10-12T04:00:00Z',
}

function memoryRouter(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'home', component: { template: '<div />' } },
      { path: '/showcase', name: 'showcase', component: { template: '<div />' } },
      { path: '/prompts/:id', name: 'detail', component: PromptDetailView },
      { path: '/prompts/:id/edit', name: 'edit', component: { template: '<div />' } },
    ],
  })
}

function stubDetail(): void {
  fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const path = url.replace(/^https?:\/\/[^/]+/, '').split('?')[0] ?? ''
    const method = init?.method ?? 'GET'
    if (path.endsWith('/session')) {
      return stubResponse({ status: 200, body: currentUserFixture({ id: 7, username: 'alice' }) })
    }
    if (method === 'GET' && path === '/api/v1/prompts/1/stats') {
      return stubResponse({ status: 200, body: statsFixture })
    }
    if (method === 'GET' && path === '/api/v1/prompts/1/ratings') {
      return stubResponse({
        status: 200,
        body: {
          items: [
            {
              model_id: 11,
              model_name: '模型甲',
              rating_count: 0,
              user_count: 0,
              mean_score: 0,
            },
          ],
        },
      })
    }
    if (method === 'GET' && path === '/api/v1/prompts/1') {
      return stubResponse({ status: 200, body: detailFixture })
    }
    if (method === 'POST' && path === '/api/v1/prompts/1/ratings') {
      return stubResponse({
        status: 200,
        body: {
          id: 1,
          prompt_id: 1,
          model_id: 11,
          score: 4,
          edit_count: 0,
          max_edits: 3,
          created_at: '2026-10-12T00:00:00Z',
          updated_at: '2026-10-12T00:00:00Z',
        },
      })
    }
    if (method === 'POST' && path === '/api/v1/prompts/1/rating-outputs') {
      return stubResponse({
        status: 200,
        body: {
          output: { id: 31, content: '输出', created_at: '2026-10-12T00:00:00Z' },
        },
      })
    }
    if (method === 'POST' && path === '/api/v1/prompts/1/rating-images/11') {
      return stubResponse({
        status: 200,
        body: {
          image: {
            id: 41,
            url: 'https://img.example/i/41.webp',
            filename: '41.webp',
            created_at: '2026-10-12T00:00:00Z',
          },
        },
      })
    }
    return stubResponse({ status: 404, body: errorEnvelope('not_found') })
  })
}

async function mountDetail(): Promise<VueWrapper> {
  const pinia = createPinia()
  setActivePinia(pinia)
  const auth = useAuthStore()
  auth.markAuthenticated(currentUserFixture({ id: 7, username: 'alice' }) as never)

  const router = memoryRouter()
  await router.push('/prompts/1')
  await router.isReady()
  const wrapper = mountView(PromptDetailView, { router, pinia })
  await flushPromises()
  return wrapper
}

/** selectScore 在 jsdom 中触发 Element Plus 单选组的分值变更。 */
async function selectScore(wrapper: VueWrapper, score: number): Promise<void> {
  const radio = wrapper.find(`[data-testid="rating-score-${score}"] input`)
  const input = radio.element as HTMLInputElement
  input.checked = true
  input.dispatchEvent(new Event('change', { bubbles: true }))
  await flushPromises()
}

/** pickImage 把文件写入文件选择框并触发 change。 */
async function pickImage(wrapper: VueWrapper, file: File): Promise<void> {
  const input = wrapper.find('[data-testid="rating-image-input"]').element as HTMLInputElement
  Object.defineProperty(input, 'files', { value: [file], writable: false, configurable: true })
  input.dispatchEvent(new Event('change', { bubbles: true }))
  await flushPromises()
}

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
  setActivePinia(createPinia())
  resetLocale()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('PromptDetailView 模型结果提交', () => {
  it('评分时附带模型输出与图片，按顺序提交三个端点', async () => {
    stubDetail()
    const wrapper = await mountDetail()

    await wrapper.find('[data-testid="rating-start-11"]').trigger('click')
    await flushPromises()
    await selectScore(wrapper, 4)

    const host = wrapper.find('[data-testid="rating-output"]')
    const textarea = host.element instanceof HTMLTextAreaElement ? host : host.find('textarea')
    ;(textarea.element as HTMLTextAreaElement).value = '模型输出文本'
    await textarea.trigger('input')

    await pickImage(wrapper, new File(['png'], 'result.png', { type: 'image/png' }))
    expect(wrapper.find('[data-testid="rating-image-list"]').text()).toContain('result.png')

    await wrapper.find('[data-testid="rating-confirm"]').trigger('click')
    await flushPromises()

    const posted = calls().filter((call) => call.method === 'POST')
    const paths = posted.map((call) => call.url.replace(/^https?:\/\/[^/]+/, ''))
    expect(paths).toContain('/api/v1/prompts/1/ratings')
    expect(paths).toContain('/api/v1/prompts/1/rating-outputs')
    expect(paths).toContain('/api/v1/prompts/1/rating-images/11')

    const outputCall = posted.find((call) => call.url.endsWith('/rating-outputs'))
    expect(String(outputCall?.body)).toContain('模型输出文本')
    expect(wrapper.find('[data-testid="detail-notice"]').text()).toContain('图片已上传')
  })

  it('拒绝不支持的图片类型，且不发出上传请求', async () => {
    stubDetail()
    const wrapper = await mountDetail()

    await wrapper.find('[data-testid="rating-start-11"]').trigger('click')
    await flushPromises()
    await pickImage(wrapper, new File(['text'], 'notes.txt', { type: 'text/plain' }))

    expect(wrapper.find('[data-testid="detail-action-error"]').text()).toContain('notes.txt')
    expect(wrapper.find('[data-testid="rating-image-list"]').exists()).toBe(false)
    expect(calls().some((call) => call.url.includes('/rating-images/'))).toBe(false)
  })
})
