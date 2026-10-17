import { flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { visibilityOptions } from '@/api/prompt'
import { mountView, resetLocale, stubResponse } from '@/testing/mount'
import HomeView from '@/views/HomeView.vue'
import PromptFormView from '@/views/PromptFormView.vue'
import { normalizeVisibility, visiblePurposes } from '@/views/promptFormRules'

/** memoryRouter 构造内存历史路由，供需要路由参数的视图挂载。 */
function memoryRouter(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'home', component: HomeView },
      { path: '/upload', name: 'upload', component: PromptFormView },
      { path: '/prompts/:id', name: 'detail', component: PromptFormView },
      { path: '/prompts/:id/edit', name: 'edit', component: PromptFormView },
    ],
  })
}

/**
 * purposeOptionsFor 返回给定身份在表单中可见的考察目的。
 */
const purposeFixtures = [
  {
    id: 1,
    slug: 'capability_boundary',
    name_zh: '边界探测',
    name_en: 'Capability Boundary',
    description_zh: '模型是否具备该能力，边界在哪里',
    description_en: 'Does the model have the capability?',
    is_high_risk: false,
  },
  {
    id: 9,
    slug: 'adversarial',
    name_zh: '对抗探测',
    name_en: 'Adversarial Probing',
    description_zh: '是否抵抗刻意操纵',
    description_en: 'Does the model resist manipulation?',
    is_high_risk: true,
  },
]

const capabilityFixtures = [
  {
    id: 1,
    slug: 'reasoning',
    name_zh: '推理',
    name_en: 'Reasoning',
    group_key: 'cognitive',
    description_zh: '多步演绎与约束求解',
    description_en: 'Multi-step deduction',
  },
]

interface DictionaryStub {
  purposes?: unknown[]
  capabilities?: unknown[]
  difficulties?: unknown[]
  tags?: unknown[]
  models?: unknown[]
}

const prompts: unknown[] = []

/**
 * 安装接口桩。首页与表单共用一套字典端点，评分与提示词各自独立，
 * 因此按路径前缀分派而不是逐个枚举。
 */
function stubEndpoints({
  purposes = [],
  capabilities = capabilityFixtures,
}: DictionaryStub = {}): void {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      const path = url.replace(/^https?:\/\/[^/]+/, '').split('?')[0] ?? ''

      if (path.endsWith('/purposes')) {
        return stubResponse({ status: 200, body: { items: purposes } })
      }
      if (path.endsWith('/capabilities')) {
        return stubResponse({ status: 200, body: { items: capabilities } })
      }
      if (path.endsWith('/difficulties')) {
        return stubResponse({ status: 200, body: { items: [] } })
      }
      if (path.endsWith('/tags')) {
        return stubResponse({ status: 200, body: { items: [] } })
      }
      if (path.endsWith('/models')) {
        return stubResponse({ status: 200, body: { items: [] } })
      }
      if (path.endsWith('/session')) {
        return stubResponse({ status: 401, body: { error: { code: 'unauthorized', message: '' } } })
      }
      if (path.endsWith('/prompts')) {
        return stubResponse({
          status: 200,
          body: { items: prompts, page: 1, page_size: 20, total: prompts.length },
        })
      }
      return stubResponse({ status: 404, body: { error: { code: 'not_found', message: '' } } })
    }),
  )
}

async function mountHome(): Promise<VueWrapper> {
  const wrapper = mountView(HomeView)
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  prompts.length = 0
  setActivePinia(createPinia())
  resetLocale()
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('HomeView 列表与筛选', () => {
  it('空列表展示空态而不是报错', async () => {
    stubEndpoints()
    const wrapper = await mountHome()

    expect(wrapper.find('[data-testid="home-empty"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="home-list"]').exists()).toBe(false)
  })

  it('列表项展示标题、版本号与区分度不足提示', async () => {
    prompts.push({
      id: 11,
      title: '推理类提示词',
      description: '描述说明',
      purpose_id: 1,
      capability_id: 1,
      visibility: 'public',
      sensitive_level: 'low',
      uploader_id: 1,
      version_no: 'v1.2',
      major_modified: false,
      comment_count: 0,
      favorite_count: 0,
      created_at: '2026-10-01T00:00:00Z',
      updated_at: '2026-10-02T00:00:00Z',
      purpose_name_zh: '边界探测',
      purpose_name_en: 'Capability Boundary',
      capability_name_zh: '推理',
      capability_name_en: 'Reasoning',
      discrimination: 0,
      data_sufficient: false,
      average_adjusted: 0,
    })
    stubEndpoints()
    const wrapper = await mountHome()

    expect(wrapper.find('[data-testid="prompt-link-11"]').text()).toBe('推理类提示词')
    expect(wrapper.find('[data-testid="prompt-version-11"]').text()).toBe('v1.2')
    expect(wrapper.find('[data-testid="prompt-discrimination-11"]').text()).toContain('数据不足')
  })

  it('数据充足时展示区分度数值', async () => {
    prompts.push({
      id: 12,
      title: '有数据的提示词',
      description: '',
      purpose_id: 1,
      capability_id: 1,
      visibility: 'public',
      sensitive_level: 'low',
      uploader_id: 1,
      version_no: 'v1.0',
      major_modified: false,
      comment_count: 0,
      favorite_count: 0,
      created_at: '2026-10-01T00:00:00Z',
      updated_at: '2026-10-02T00:00:00Z',
      purpose_name_zh: '边界探测',
      purpose_name_en: 'Capability Boundary',
      capability_name_zh: '推理',
      capability_name_en: 'Reasoning',
      discrimination: 4.6,
      data_sufficient: true,
      average_adjusted: 3.25,
    })
    stubEndpoints()
    const wrapper = await mountHome()

    expect(wrapper.find('[data-testid="prompt-discrimination-12"]').text()).toContain('4.60')
  })

  it('切换排序会带 sort 参数重新查询', async () => {
    stubEndpoints()
    const wrapper = await mountHome()

    // 排序选择框带 data-testid，取它的组件实例后触发两个事件：
    // update:modelValue 更新绑定值，change 触发重新查询，与用户点选等价。
    // CSS 选择器的返回值是 WrapperLike，取 vm 前先收敛为 VueWrapper。
    const sort = wrapper.findComponent('[data-testid="filter-sort"]') as VueWrapper
    expect(sort.exists()).toBe(true)
    await sort.vm.$emit('update:modelValue', 'discrimination')
    await sort.vm.$emit('change', 'discrimination')
    await flushPromises()

    const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls
    const last = String(calls[calls.length - 1]?.[0] ?? '')
    expect(last).toContain('sort=discrimination')
  })

  it('切换考察目的与考察维度会带对应参数重新查询', async () => {
    stubEndpoints({ purposes: purposeFixtures })
    const wrapper = await mountHome()

    const purpose = wrapper.findComponent('[data-testid="filter-purpose"]') as VueWrapper
    await purpose.vm.$emit('update:modelValue', '1')
    await purpose.vm.$emit('change', '1')
    await flushPromises()

    const capability = wrapper.findComponent('[data-testid="filter-capability"]') as VueWrapper
    await capability.vm.$emit('update:modelValue', '1')
    await capability.vm.$emit('change', '1')
    await flushPromises()

    const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls
    const urls = calls.map((call) => String(call[0] ?? ''))
    expect(urls.some((url) => url.includes('purpose_id=1'))).toBe(true)
    expect(urls.some((url) => url.includes('capability_id=1'))).toBe(true)
  })

  it('未设置任何筛选时重置按钮不可用', async () => {
    stubEndpoints()
    const wrapper = await mountHome()

    expect(wrapper.find('[data-testid="filter-reset"]').attributes('disabled')).toBeDefined()
  })

  it('游客看不到上传入口', async () => {
    stubEndpoints()
    const wrapper = await mountHome()

    expect(wrapper.find('[data-testid="home-upload"]').exists()).toBe(false)
  })
})

describe('PromptFormView 权限相关的选项', () => {
  it('普通用户的考察目的选项不含高风险条目', () => {
    const visible = visiblePurposes(purposeFixtures, false).map((item) => item.slug)
    expect(visible).toEqual(['capability_boundary'])
  })

  it('开发者的考察目的选项包含高风险条目', () => {
    const visible = visiblePurposes(purposeFixtures, true).map((item) => item.slug)
    expect(visible).toEqual(['capability_boundary', 'adversarial'])
  })

  it('普通用户的可见性只有公开与私有两档', () => {
    expect(visibilityOptions(false)).toEqual(['public', 'private'])
  })

  it('开发者的可见性多出开发者可见一档', () => {
    expect(visibilityOptions(true)).toEqual(['public', 'private', 'developer'])
  })

  it('普通用户提交的开发者可见被回退为公开', () => {
    expect(normalizeVisibility('developer', false)).toBe('public')
    expect(normalizeVisibility('developer', true)).toBe('developer')
    expect(normalizeVisibility('private', false)).toBe('private')
  })

  it('上传页在字典就绪后渲染标题输入框', async () => {
    stubEndpoints({ purposes: purposeFixtures })
    const wrapper = mountView(PromptFormView, { router: memoryRouter() })
    await flushPromises()

    expect(wrapper.find('input[data-testid="prompt-title"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="prompt-submit"]').exists()).toBe(true)
  })
})

describe('PromptFormView 编辑表单的加载门禁', () => {
  it('详情返回前禁用提交，返回后才可提交', async () => {
    let release: (() => void) | undefined
    const gate = new Promise<void>((resolve) => {
      release = resolve
    })

    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        const path = url.replace(/^https?:\/\/[^/]+/, '').split('?')[0] ?? ''
        if (path.endsWith('/purposes')) {
          return stubResponse({ status: 200, body: { items: purposeFixtures } })
        }
        if (path.endsWith('/capabilities')) {
          return stubResponse({ status: 200, body: { items: capabilityFixtures } })
        }
        if (path.endsWith('/difficulties')) {
          return stubResponse({ status: 200, body: { items: [] } })
        }
        if (path.endsWith('/tags') || path.endsWith('/models')) {
          return stubResponse({ status: 200, body: { items: [] } })
        }
        if (path.endsWith('/session')) {
          return stubResponse({
            status: 401,
            body: { error: { code: 'unauthorized', message: '' } },
          })
        }
        if (/\/prompts\/7$/.test(path)) {
          // 详情返回前挂起，模拟表单已渲染但数据尚未到达的时刻。
          await gate
          return stubResponse({
            status: 200,
            body: {
              prompt: {
                id: 7,
                title: '待编辑提示词',
                description: '',
                purpose_id: 1,
                capability_id: 1,
                visibility: 'public',
                sensitive_level: 'low',
                uploader_id: 1,
                version_no: 'v1.0',
                major_modified: false,
                comment_count: 0,
                favorite_count: 0,
                created_at: '2026-01-01T00:00:00Z',
                updated_at: '2026-01-01T00:00:00Z',
              },
              version: {
                id: 70,
                version_no: 'v1.0',
                title: '待编辑提示词',
                body: '正文',
                description: '',
                purpose_id: 1,
                capability_id: 1,
                visibility: 'public',
                sensitive_level: 'low',
                is_major: false,
                change_note: '',
                author_id: 1,
                created_at: '2026-01-01T00:00:00Z',
              },
              versions: [],
              purpose: {
                id: 1,
                slug: 'capability_boundary',
                name_zh: '边界探测',
                name_en: 'Capability Boundary',
                description_zh: '模型是否具备该能力，边界在哪里',
                description_en: 'Does the model have the capability?',
                is_high_risk: false,
              },
              capability: {
                id: 1,
                slug: 'reasoning',
                name_zh: '推理',
                name_en: 'Reasoning',
                group_key: 'cognitive',
                description_zh: '多步演绎与约束求解',
                description_en: 'Multi-step deduction',
              },
              tags: [],
              uploader: 'alice',
            },
          })
        }
        return stubResponse({ status: 404, body: { error: { code: 'not_found', message: '' } } })
      }),
    )

    const router = memoryRouter()
    await router.push('/prompts/7/edit')
    await router.isReady()
    const wrapper = mountView(PromptFormView, { router })
    await flushPromises()

    // 详情未到达：提交入口被禁用，避免把占位取值（空正文、考察目的 0）发给后端。
    expect(wrapper.find('[data-testid="prompt-submit"]').attributes('disabled')).toBeDefined()

    release?.()
    await flushPromises()

    expect(wrapper.find('[data-testid="prompt-submit"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.find('input[data-testid="prompt-title"]').element).toHaveProperty(
      'value',
      '待编辑提示词',
    )
  })
})
