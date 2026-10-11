import { flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { visibilityOptions } from '@/api/prompt'
import { mountView, resetLocale, stubResponse } from '@/testing/mount'
import HomeView from '@/views/HomeView.vue'
import PromptFormView from '@/views/PromptFormView.vue'
import { normalizeVisibility, visibleCategories } from '@/views/promptFormRules'

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
 * categoryOptionsFor 返回给定身份在表单中可见的类别。
 */
const categoryFixtures = [
  { id: 1, slug: 'reasoning', name_zh: '逻辑推理', name_en: 'Reasoning', is_high_risk: false },
  { id: 9, slug: 'jailbreak', name_zh: '越狱对抗', name_en: 'Jailbreak', is_high_risk: true },
]

interface DictionaryStub {
  categories?: unknown[]
  abilities?: unknown[]
  difficulties?: unknown[]
  tags?: unknown[]
  models?: unknown[]
}

const prompts: unknown[] = []

/**
 * 安装接口桩。首页与表单共用一套字典端点，评分与提示词各自独立，
 * 因此按路径前缀分派而不是逐个枚举。
 */
function stubEndpoints({ categories = [] }: DictionaryStub = {}): void {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      const path = url.replace(/^https?:\/\/[^/]+/, '').split('?')[0] ?? ''

      if (path.endsWith('/categories')) {
        return stubResponse({ status: 200, body: { items: categories } })
      }
      if (path.endsWith('/abilities')) {
        return stubResponse({ status: 200, body: { items: [] } })
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
      category_id: 1,
      visibility: 'public',
      sensitive_level: 'low',
      uploader_id: 1,
      version_no: 'v1.2',
      major_modified: false,
      comment_count: 0,
      favorite_count: 0,
      created_at: '2026-10-01T00:00:00Z',
      updated_at: '2026-10-02T00:00:00Z',
      category_name_zh: '逻辑推理',
      category_name_en: 'Logical Reasoning',
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
      category_id: 1,
      visibility: 'public',
      sensitive_level: 'low',
      uploader_id: 1,
      version_no: 'v1.0',
      major_modified: false,
      comment_count: 0,
      favorite_count: 0,
      created_at: '2026-10-01T00:00:00Z',
      updated_at: '2026-10-02T00:00:00Z',
      category_name_zh: '逻辑推理',
      category_name_en: 'Logical Reasoning',
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
  it('普通用户的类别选项不含高风险类别', () => {
    const visible = visibleCategories(categoryFixtures, false).map((item) => item.slug)
    expect(visible).toEqual(['reasoning'])
  })

  it('开发者的类别选项包含高风险类别', () => {
    const visible = visibleCategories(categoryFixtures, true).map((item) => item.slug)
    expect(visible).toEqual(['reasoning', 'jailbreak'])
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
    stubEndpoints({ categories: categoryFixtures })
    const wrapper = mountView(PromptFormView, { router: memoryRouter() })
    await flushPromises()

    expect(wrapper.find('input[data-testid="prompt-title"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="prompt-submit"]').exists()).toBe(true)
  })
})
