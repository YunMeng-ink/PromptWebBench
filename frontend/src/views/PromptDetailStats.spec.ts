import { flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { errorEnvelope, mountView, resetLocale, stubResponse } from '@/testing/mount'
import PromptDetailView from '@/views/PromptDetailView.vue'

/** memoryRouter 提供带 id 参数的内存路由。 */
function memoryRouter(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'home', component: { template: '<div />' } },
      { path: '/prompts/:id', name: 'detail', component: PromptDetailView },
      { path: '/prompts/:id/edit', name: 'edit', component: { template: '<div />' } },
    ],
  })
}

const detailFixture = {
  prompt: {
    id: 1,
    title: '统计展示提示词',
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
    title: '统计展示提示词',
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
}

/** statsFixture 是数据充足且带一条已确认推荐标签的统计响应。 */
const statsFixture = {
  prompt_id: 1,
  history: {
    sufficient: true,
    discrimination: 4.6,
    confidence: 'high',
    valid_model_count: 4,
    total_rating_count: 32,
  },
  recent: {
    sufficient: false,
    discrimination: null,
    confidence: 'low',
    valid_model_count: 2,
    total_rating_count: 6,
  },
  models: [
    {
      model_id: 11,
      model_name: '模型甲',
      history: {
        has_data: true,
        mean: 1.4,
        adjusted: 1.4,
        rating_count: 8,
        user_count: 8,
        sd: 0.5,
        ci_half: 0.35,
        confidence_score: 0.6,
        confidence: 'medium',
      },
      recent: {
        has_data: true,
        mean: 4.6,
        adjusted: 4.6,
        rating_count: 6,
        user_count: 6,
        sd: 0.4,
        ci_half: 0.32,
        confidence_score: 0.58,
        confidence: 'medium',
      },
    },
    {
      model_id: 12,
      model_name: '模型乙',
      history: {
        has_data: false,
        mean: null,
        adjusted: 3,
        rating_count: 0,
        user_count: 0,
        sd: 0,
        ci_half: 0,
        confidence_score: 0,
        confidence: 'low',
      },
      recent: {
        has_data: false,
        mean: null,
        adjusted: 3,
        rating_count: 0,
        user_count: 0,
        sd: 0,
        ci_half: 0,
        confidence_score: 0,
        confidence: 'low',
      },
    },
  ],
  recommendation: [
    {
      id: 1,
      model_id: 11,
      scope: 'prompt_model',
      kind: 'saturation',
      label_zh: '训练饱和：模型甲',
      label_en: 'Saturated: 模型甲',
      confidence: 'high',
      reason: '该模型近 N 日校准完成度已达 4.5，评分置信度中以上',
    },
  ],
  computed_at: '2026-10-12T04:00:00Z',
}

/** stubDetailEndpoints 按路径分派详情页所需的三个端点。 */
function stubDetailEndpoints(options: { stats?: unknown; statsStatus?: number } = {}): void {
  const statsBody = options.stats ?? statsFixture
  const statsStatus = options.statsStatus ?? 200
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      const path = url.replace(/^https?:\/\/[^/]+/, '').split('?')[0] ?? ''
      if (path.endsWith('/session')) {
        return stubResponse({ status: 401, body: errorEnvelope('unauthorized') })
      }
      if (path === '/api/v1/prompts/1/stats') {
        return stubResponse({ status: statsStatus, body: statsBody })
      }
      if (path === '/api/v1/prompts/1/ratings') {
        return stubResponse({ status: 200, body: { items: [] } })
      }
      if (path === '/api/v1/prompts/1') {
        return stubResponse({ status: 200, body: detailFixture })
      }
      return stubResponse({ status: 404, body: errorEnvelope('not_found') })
    }),
  )
}

async function mountDetail(): Promise<VueWrapper> {
  const router = memoryRouter()
  await router.push('/prompts/1')
  await router.isReady()
  const wrapper = mountView(PromptDetailView, { router })
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

describe('PromptDetailView 测量结果', () => {
  it('数据充足的历史区分度显示数值', async () => {
    stubDetailEndpoints()
    const wrapper = await mountDetail()

    expect(wrapper.find('[data-testid="stats-history-discrimination"]').text()).toBe('4.60')
    expect(wrapper.find('[data-testid="stats-discrimination-confidence"]').text()).toBe('低')
    expect(wrapper.find('[data-testid="stats-valid-models"]').text()).toBe('2')
    expect(wrapper.find('[data-testid="stats-total-ratings"]').text()).toBe('6')
  })

  it('数据不足时显示 C-06 文案而不是 0', async () => {
    stubDetailEndpoints()
    const wrapper = await mountDetail()

    expect(wrapper.find('[data-testid="stats-recent-discrimination"]').text()).toBe(
      '数据不足，暂不计算区分度',
    )
  })

  it('逐模型给出两个窗口的完成度、评分数与置信度', async () => {
    stubDetailEndpoints()
    const wrapper = await mountDetail()

    const row = wrapper.find('[data-testid="stats-model-11"]').element.closest('tr')
    expect(row).not.toBeNull()
    const cells = Array.from(row?.querySelectorAll('td') ?? []).map((cell) =>
      (cell.textContent ?? '').trim(),
    )
    expect(cells).toEqual(['模型甲', '1.40', '4.60', '8', '6', '4.60', '6', '中'])
  })

  it('无观测值的模型显示尚无评分而不是 0.00', async () => {
    stubDetailEndpoints()
    const wrapper = await mountDetail()

    const row = wrapper.find('[data-testid="stats-model-12"]').element.closest('tr')
    const cells = Array.from(row?.querySelectorAll('td') ?? []).map((cell) =>
      (cell.textContent ?? '').trim(),
    )
    expect(cells.slice(0, 5)).toEqual(['模型乙', '—', '—', '0', '0'])
    expect(cells[7]).toBe('尚无评分')
  })

  it('展示已确认的推荐标签与推荐理由', async () => {
    stubDetailEndpoints()
    const wrapper = await mountDetail()

    const tag = wrapper.find('[data-testid="stats-tag-saturation"]')
    expect(tag.exists()).toBe(true)
    expect(tag.text()).toBe('训练饱和：模型甲')
    expect(wrapper.find('[data-testid="stats-tags"]').text()).toContain('推荐理由')
  })

  it('统计接口失败时只把测量区标记为不可用，正文照常展示', async () => {
    stubDetailEndpoints({ statsStatus: 500, stats: errorEnvelope('internal') })
    const wrapper = await mountDetail()

    expect(wrapper.find('[data-testid="stats-error"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="detail-body"]').text()).toBe('提示词正文')
    expect(wrapper.find('[data-testid="detail-title"]').text()).toBe('统计展示提示词')
  })

  it('英文界面下推荐标签显示英文文案', async () => {
    localStorage.setItem('pb.locale', 'en')
    const { setLocale } = await import('@/i18n')
    setLocale('en')

    stubDetailEndpoints()
    const wrapper = await mountDetail()

    expect(wrapper.find('[data-testid="stats-tag-saturation"]').text()).toBe('Saturated: 模型甲')

    setLocale('zh')
  })
})
