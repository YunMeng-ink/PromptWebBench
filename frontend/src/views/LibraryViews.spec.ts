import { flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { errorEnvelope, mountView, resetLocale, stubResponse } from '@/testing/mount'
import LibraryDetailView from '@/views/LibraryDetailView.vue'
import LibraryListView from '@/views/LibraryListView.vue'
import LibraryQuestionView from '@/views/LibraryQuestionView.vue'

interface RecordedRequest {
  method: string
  path: string
  body: unknown
}

interface StubOptions {
  libraries?: unknown[]
  recycledLibraries?: unknown[]
  library?: Record<string, unknown>
  questions?: unknown[]
  members?: unknown[]
  recycledQuestions?: unknown[]
  question?: Record<string, unknown>
  revisions?: unknown[]
}

function libraryFixture(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 3,
    owner_id: 1,
    owner_name: 'alice',
    name: '测试题库',
    description: '题库描述',
    role: 'owner',
    member_count: 1,
    question_count: 2,
    live_question_count: 2,
    question_limit: 150,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function questionFixture(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 7,
    library_id: 3,
    kind: 'reference',
    title: '引用题标题',
    body: '引用题正文',
    description: '',
    purpose_name_zh: '边界探测',
    purpose_name_en: 'Capability Boundary',
    capability_name_zh: '推理',
    capability_name_en: 'Reasoning',
    sensitive_level: 'low',
    sort_order: 1,
    tags: [],
    source_prompt_id: 12,
    source_version_no: 'v1.0',
    source_available: true,
    source_major_modified: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

/** stubLibraryEndpoints 按路径分派题库端点，并记录请求。 */
function stubLibraryEndpoints(options: StubOptions = {}): RecordedRequest[] {
  const recorded: RecordedRequest[] = []
  const libraries = options.libraries ?? [libraryFixture()]
  const library = options.library ?? libraryFixture()
  const questions = options.questions ?? [questionFixture()]
  const members = options.members ?? [
    { user_id: 1, username: 'alice', avatar_url: '', role: 'owner', created_at: '' },
    { user_id: 2, username: 'bob', avatar_url: '', role: 'collaborator', created_at: '' },
  ]
  const question = options.question ?? questionFixture()

  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      const withoutHost = url.replace(/^https?:\/\/[^/]+/, '')
      const [path = ''] = withoutHost.split('?')
      const method = init?.method ?? 'GET'
      const body = init?.body === undefined ? undefined : JSON.parse(String(init.body))
      recorded.push({ method, path, body })

      if (path.endsWith('/session')) {
        return stubResponse({
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
      }

      if (path.endsWith('/me/libraries/recycle-bin')) {
        return stubResponse({
          status: 200,
          body: { items: options.recycledLibraries ?? [] },
        })
      }
      if (path.endsWith('/me/libraries')) {
        return stubResponse({
          status: 200,
          body: { items: libraries, page: 1, page_size: 20, total: libraries.length },
        })
      }
      if (path === '/api/v1/libraries' && method === 'POST') {
        return stubResponse({ status: 200, body: { library: libraryFixture({ id: 9 }) } })
      }
      if (path.endsWith('/export')) {
        return stubResponse({
          status: 200,
          body: { filename: '测试题库.md', content: '# 测试题库' },
        })
      }
      if (path.endsWith('/members') && method === 'POST') {
        return stubResponse({
          status: 200,
          body: { member: { user_id: 5, username: 'carol', role: 'collaborator' } },
        })
      }
      if (/\/members\/\d+$/.test(path) && method === 'DELETE') {
        return stubResponse({ status: 204 })
      }
      if (path.endsWith('/members')) {
        return stubResponse({ status: 200, body: { items: members } })
      }
      if (path.endsWith('/questions/order')) {
        return stubResponse({ status: 204 })
      }
      if (path.endsWith('/recycle-bin')) {
        return stubResponse({ status: 200, body: { items: options.recycledQuestions ?? [] } })
      }
      if (path.endsWith('/questions') && method === 'POST') {
        return stubResponse({ status: 200, body: { question: questionFixture({ id: 11 }) } })
      }
      if (path.endsWith('/questions')) {
        return stubResponse({
          status: 200,
          body: { items: questions, page: 1, page_size: 100, total: questions.length },
        })
      }
      if (path.endsWith('/revisions')) {
        return stubResponse({
          status: 200,
          body: { items: options.revisions ?? [] },
        })
      }
      if (/\/library-questions\/\d+$/.test(path) && method === 'PATCH') {
        return stubResponse({ status: 200, body: { question: questionFixture() } })
      }
      if (/\/library-questions\/\d+$/.test(path) && method === 'DELETE') {
        return stubResponse({ status: 204 })
      }
      if (/\/library-questions\/\d+\/restore$/.test(path)) {
        return stubResponse({ status: 200, body: { question: questionFixture() } })
      }
      if (/\/library-questions\/\d+\/purge$/.test(path)) {
        return stubResponse({ status: 204 })
      }
      if (/\/library-questions\/\d+\/tags$/.test(path)) {
        return stubResponse({ status: 204 })
      }
      if (/\/library-questions\/\d+$/.test(path)) {
        return stubResponse({ status: 200, body: { question } })
      }
      if (/\/libraries\/\d+\/purge$/.test(path)) {
        return stubResponse({ status: 204 })
      }
      if (/\/libraries\/\d+\/restore$/.test(path)) {
        return stubResponse({ status: 200, body: { library: library } })
      }
      if (/\/libraries\/\d+$/.test(path) && method === 'DELETE') {
        return stubResponse({ status: 204 })
      }
      if (/\/libraries\/\d+$/.test(path) && method === 'PATCH') {
        return stubResponse({ status: 200, body: { library } })
      }
      if (/\/libraries\/\d+$/.test(path)) {
        return stubResponse({ status: 200, body: { library } })
      }
      return stubResponse({ status: 404, body: errorEnvelope('not_found') })
    }),
  )
  return recorded
}

function memoryRouter(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'home', component: { template: '<div />' } },
      { path: '/libraries', name: 'libraries', component: LibraryListView },
      { path: '/libraries/:id', name: 'library-detail', component: LibraryDetailView },
      { path: '/library-questions/:id', name: 'library-question', component: LibraryQuestionView },
      { path: '/prompts/:id', name: 'prompt-detail', component: { template: '<div />' } },
    ],
  })
}

/** mountAt 在给定路径上挂载组件，返回包装器与实际路由。 */
async function mountAt(
  component: unknown,
  path: string,
): Promise<{ wrapper: VueWrapper; router: Router }> {
  const router = memoryRouter()
  await router.push(path)
  await router.isReady()
  const wrapper = mountView(component as never, { router })
  await flushPromises()
  return { wrapper, router }
}

beforeEach(() => {
  setActivePinia(createPinia())
  resetLocale()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('题库列表', () => {
  it('展示题库、角色与容量', async () => {
    stubLibraryEndpoints()
    const { wrapper } = await mountAt(LibraryListView, '/libraries')

    expect(wrapper.find('[data-testid="libraries-list"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="library-role-3"]').text()).toContain('所有者')
    expect(wrapper.find('[data-testid="library-role-3"]').text()).toContain('题目 2 / 150')
  })

  it('没有题库时展示空态', async () => {
    stubLibraryEndpoints({ libraries: [] })
    const { wrapper } = await mountAt(LibraryListView, '/libraries')

    expect(wrapper.find('[data-testid="libraries-empty"]').exists()).toBe(true)
  })

  it('创建题库后跳转到详情', async () => {
    const recorded = stubLibraryEndpoints()
    const { wrapper, router } = await mountAt(LibraryListView, '/libraries')

    await wrapper.find('[data-testid="library-create"]').trigger('click')
    await flushPromises()
    const nameInput = wrapper.find('[data-testid="library-new-name"]')
    await nameInput.setValue('新题库')
    await wrapper.find('[data-testid="library-create-submit"]').trigger('click')
    await flushPromises()

    const created = recorded.find((r) => r.method === 'POST' && r.path === '/api/v1/libraries')
    expect(created?.body).toEqual({ name: '新题库', description: '' })
    expect(router.currentRoute.value.path).toBe('/libraries/9')
  })

  it('彻底删除回收站题库需要二次确认', async () => {
    const recorded = stubLibraryEndpoints({
      recycledLibraries: [
        {
          library_id: 42,
          name: '已删题库',
          description: '',
          deleted_by: 1,
          deleted_at: '2026-01-01T00:00:00Z',
          expires_at: '2026-01-15T00:00:00Z',
        },
      ],
    })
    const { wrapper } = await mountAt(LibraryListView, '/libraries')

    await wrapper.find('[data-testid="library-purge-42"]').trigger('click')
    await flushPromises()
    expect(recorded.some((r) => r.path.endsWith('/libraries/42/purge'))).toBe(false)

    await wrapper.find('[data-testid="library-purge-confirm"]').trigger('click')
    await flushPromises()
    expect(
      recorded.some((r) => r.method === 'DELETE' && r.path === '/api/v1/libraries/42/purge'),
    ).toBe(true)
  })

  it('取消二次确认时不发出删除请求', async () => {
    const recorded = stubLibraryEndpoints({
      recycledLibraries: [
        {
          library_id: 42,
          name: '已删题库',
          description: '',
          deleted_by: 1,
          deleted_at: '2026-01-01T00:00:00Z',
          expires_at: '2026-01-15T00:00:00Z',
        },
      ],
    })
    const { wrapper } = await mountAt(LibraryListView, '/libraries')

    await wrapper.find('[data-testid="library-purge-42"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-testid="library-purge-cancel"]').trigger('click')
    await flushPromises()
    expect(recorded.some((r) => r.method === 'DELETE')).toBe(false)
  })
})

describe('题库详情', () => {
  it('所有者可见邀请、改名与删除题库', async () => {
    stubLibraryEndpoints()
    const { wrapper } = await mountAt(LibraryDetailView, '/libraries/3')

    expect(wrapper.find('[data-testid="library-name"]').text()).toBe('测试题库')
    expect(wrapper.find('[data-testid="library-invite"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="library-delete"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="library-add-reference"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="question-purge-7"]').exists()).toBe(false)
  })

  it('协作者看不到邀请、改名与删除题库', async () => {
    stubLibraryEndpoints({ library: libraryFixture({ role: 'collaborator' }) })
    const { wrapper } = await mountAt(LibraryDetailView, '/libraries/3')

    expect(wrapper.find('[data-testid="library-invite"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="library-delete"]').exists()).toBe(false)
    // 描述仍可修改，因此编辑入口保留。
    expect(wrapper.find('[data-testid="library-edit"]').exists()).toBe(true)
    // 协作者可以添加、删除与重排题目。
    expect(wrapper.find('[data-testid="library-add-derived"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="question-delete-7"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="question-down-7"]').exists()).toBe(true)
  })

  it('添加引用题提交来源提示词标识', async () => {
    const recorded = stubLibraryEndpoints()
    const { wrapper } = await mountAt(LibraryDetailView, '/libraries/3')

    await wrapper.find('[data-testid="library-add-reference"]').trigger('click')
    await flushPromises()
    await wrapper.find('[data-testid="library-add-prompt-id"]').setValue('12')
    await wrapper.find('[data-testid="library-add-tags"]').setValue('标签甲，标签乙')
    await wrapper.find('[data-testid="library-add-submit"]').trigger('click')
    await flushPromises()

    const added = recorded.find((r) => r.method === 'POST' && r.path.endsWith('/questions'))
    expect(added?.body).toEqual({ prompt_id: 12, tags: ['标签甲', '标签乙'] })
  })

  it('调整顺序时按新序列提交', async () => {
    const recorded = stubLibraryEndpoints({
      questions: [
        questionFixture({ id: 7, sort_order: 1 }),
        questionFixture({ id: 8, sort_order: 2, title: '第二题' }),
      ],
    })
    const { wrapper } = await mountAt(LibraryDetailView, '/libraries/3')

    await wrapper.find('[data-testid="question-down-7"]').trigger('click')
    await flushPromises()

    const ordered = recorded.find((r) => r.path.endsWith('/questions/order'))
    expect(ordered?.body).toEqual({ ids: [8, 7] })
  })

  it('删除题目在确认后才提交', async () => {
    const recorded = stubLibraryEndpoints()
    const { wrapper } = await mountAt(LibraryDetailView, '/libraries/3')

    await wrapper.find('[data-testid="question-delete-7"]').trigger('click')
    await flushPromises()
    expect(recorded.some((r) => r.method === 'DELETE')).toBe(false)

    await wrapper.find('[data-testid="question-delete-confirm"]').trigger('click')
    await flushPromises()
    expect(
      recorded.some((r) => r.method === 'DELETE' && r.path.endsWith('/library-questions/7')),
    ).toBe(true)
  })

  it('来源提示词缺失时给出提示', async () => {
    stubLibraryEndpoints({
      questions: [questionFixture({ source_available: false, title: '（来源提示词已删除）' })],
    })
    const { wrapper } = await mountAt(LibraryDetailView, '/libraries/3')

    expect(wrapper.find('[data-testid="question-source-missing-7"]').exists()).toBe(true)
  })

  it('导出下载 Markdown 文件', async () => {
    stubLibraryEndpoints()
    const createObjectURL = vi.fn(() => 'blob:test')
    const revokeObjectURL = vi.fn()
    vi.stubGlobal('URL', { ...URL, createObjectURL, revokeObjectURL })
    const clicks: string[] = []
    const originalClick = HTMLAnchorElement.prototype.click
    HTMLAnchorElement.prototype.click = function click(this: HTMLAnchorElement) {
      clicks.push(this.download)
    }

    const { wrapper } = await mountAt(LibraryDetailView, '/libraries/3')
    await wrapper.find('[data-testid="library-export"]').trigger('click')
    await flushPromises()

    expect(createObjectURL).toHaveBeenCalled()
    expect(clicks).toContain('测试题库.md')
    HTMLAnchorElement.prototype.click = originalClick
  })
})

describe('题库题目详情', () => {
  it('复制提示词与复制完整信息分别写入剪贴板', async () => {
    const written: string[] = []
    Object.defineProperty(window.navigator, 'clipboard', {
      configurable: true,
      value: {
        writeText: (text: string) => {
          written.push(text)
          return Promise.resolve()
        },
      },
    })
    stubLibraryEndpoints()
    const { wrapper } = await mountAt(LibraryQuestionView, '/library-questions/7')

    await wrapper.find('[data-testid="question-copy-body"]').trigger('click')
    await flushPromises()
    expect(written[0]).toBe('引用题正文')

    await wrapper.find('[data-testid="question-copy-full"]').trigger('click')
    await flushPromises()
    expect(written[1]).toContain('标题：引用题标题')
    expect(written[1]).toContain('类型：引用题')
    expect(written[1]).toContain('来源提示词：#12 (v1.0)')
    expect(written[1]).toContain('正文\n引用题正文')
    expect(wrapper.find('[data-testid="question-copied"]').exists()).toBe(true)
  })

  it('添加与移除标签调用对应端点', async () => {
    const recorded = stubLibraryEndpoints({ question: questionFixture({ tags: ['已有标签'] }) })
    const { wrapper } = await mountAt(LibraryQuestionView, '/library-questions/7')

    await wrapper.find('[data-testid="question-tag-input"]').setValue('新标签')
    await wrapper.find('[data-testid="question-tag-add"]').trigger('click')
    await flushPromises()
    const added = recorded.find((r) => r.method === 'PUT' && r.path.endsWith('/tags'))
    expect(added?.body).toEqual({ tag: '新标签' })

    await wrapper.find('[data-testid="tag-remove-已有标签"]').trigger('click')
    await flushPromises()
    const removed = recorded.find((r) => r.method === 'DELETE' && r.path.endsWith('/tags'))
    expect(removed?.path).toBe('/api/v1/library-questions/7/tags')
  })

  it('来源被删除时给出提示且不展示正文', async () => {
    stubLibraryEndpoints({
      question: questionFixture({
        source_available: false,
        body: '',
        title: '（来源提示词已删除）',
      }),
    })
    const { wrapper } = await mountAt(LibraryQuestionView, '/library-questions/7')

    expect(wrapper.find('[data-testid="question-source-missing"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="question-body"]').text()).toBe('')
  })

  it('展示修改记录', async () => {
    stubLibraryEndpoints({
      revisions: [
        {
          id: 1,
          editor_id: 1,
          editor_name: 'alice',
          note: '调整用词',
          before: '{}',
          after: '{}',
          created_at: '2026-01-02T00:00:00Z',
        },
      ],
    })
    const { wrapper } = await mountAt(LibraryQuestionView, '/library-questions/7')

    expect(wrapper.find('[data-testid="revision-1"]').text()).toContain('调整用词')
    expect(wrapper.find('[data-testid="revision-1"]').text()).toContain('alice')
  })
})
