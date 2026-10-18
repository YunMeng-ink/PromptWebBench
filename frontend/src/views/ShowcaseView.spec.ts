import { flushPromises, type VueWrapper } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter, type Router } from 'vue-router'
import { errorEnvelope, mountView, resetLocale, stubResponse } from '@/testing/mount'
import ShowcaseView from '@/views/ShowcaseView.vue'

const groupsFixture = {
  items: [
    {
      model_id: 11,
      model_name: '模型甲',
      provider_key: 'acme',
      provider_name: 'Acme',
      image_count: 1,
      output_count: 1,
      last_contributed_at: '2026-10-12T00:00:00Z',
    },
    {
      model_id: 12,
      model_name: '模型乙',
      provider_key: 'acme',
      provider_name: 'Acme',
      image_count: 1,
      output_count: 0,
      last_contributed_at: '2026-10-11T00:00:00Z',
    },
    {
      model_id: 21,
      model_name: '模型丙',
      provider_key: 'globex',
      provider_name: 'Globex',
      image_count: 0,
      output_count: 2,
      last_contributed_at: '2026-10-10T00:00:00Z',
    },
  ],
}

const entriesByModel: Record<string, unknown> = {
  '11': {
    items: [
      {
        kind: 'image',
        id: 101,
        prompt_id: 1,
        prompt_title: '图片提示词',
        username: 'alice',
        url: 'https://img.example/i/101.webp',
        created_at: '2026-10-12T00:00:00Z',
      },
      {
        kind: 'output',
        id: 102,
        prompt_id: 2,
        prompt_title: '输出提示词',
        username: 'bob',
        content: '模型输出示例',
        created_at: '2026-10-11T00:00:00Z',
      },
    ],
    page: 1,
    page_size: 12,
    total: 2,
  },
  '12': {
    items: [
      {
        kind: 'image',
        id: 103,
        prompt_id: 1,
        prompt_title: '图片提示词',
        username: 'alice',
        url: 'https://img.example/i/103.webp',
        created_at: '2026-10-09T00:00:00Z',
      },
    ],
    page: 1,
    page_size: 12,
    total: 1,
  },
  '21': {
    items: [],
    page: 1,
    page_size: 12,
    total: 0,
  },
}

function memoryRouter(): Router {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', name: 'home', component: { template: '<div />' } },
      { path: '/showcase', name: 'showcase', component: ShowcaseView },
      { path: '/prompts/:id', name: 'detail', component: { template: '<div />' } },
    ],
  })
}

function stubShowcase(groups: unknown): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input)
    const path = url.replace(/^https?:\/\/[^/]+/, '')
    if (path === '/api/v1/showcase/groups') {
      return stubResponse({ status: 200, body: groups })
    }
    const match = /^\/api\/v1\/showcase\/models\/(\d+)\/entries/.exec(path)
    if (match !== null) {
      const body = entriesByModel[match[1] ?? '']
      if (body === undefined) {
        return stubResponse({ status: 404, body: errorEnvelope('not_found') })
      }
      return stubResponse({ status: 200, body })
    }
    return stubResponse({ status: 404, body: errorEnvelope('not_found') })
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

async function mountShowcase(): Promise<VueWrapper> {
  const router = memoryRouter()
  await router.push('/showcase')
  await router.isReady()
  const wrapper = mountView(ShowcaseView, { router })
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

describe('ShowcaseView 模型结果展示区', () => {
  it('按模型类别列出模型，默认展示第一个模型的图片与输出', async () => {
    stubShowcase(groupsFixture)
    const wrapper = await mountShowcase()

    expect(wrapper.find('[data-testid="showcase-provider-all"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="showcase-provider-acme"]').text()).toContain('Acme')
    expect(wrapper.find('[data-testid="showcase-provider-globex"]').text()).toContain('Globex')
    expect(wrapper.find('[data-testid="showcase-model-11"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="showcase-current-model"]').text()).toBe('模型甲')
    expect(wrapper.find('[data-testid="showcase-entry-image-101"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="showcase-entry-output-102"]').text()).toContain(
      '模型输出示例',
    )
    expect(wrapper.find('[data-testid="showcase-total"]').text()).toContain('2')
  })

  it('切换模型会拉取对应素材，切换类别会收窄模型列表', async () => {
    const fetchMock = stubShowcase(groupsFixture)
    const wrapper = await mountShowcase()

    await wrapper.find('[data-testid="showcase-model-12"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="showcase-current-model"]').text()).toBe('模型乙')
    expect(wrapper.find('[data-testid="showcase-entry-image-103"]').exists()).toBe(true)
    expect(
      fetchMock.mock.calls.some((call) => String(call[0]).includes('/showcase/models/12/entries')),
    ).toBe(true)

    await wrapper.find('[data-testid="showcase-provider-globex"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="showcase-model-11"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="showcase-model-21"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="showcase-model-empty"]').exists()).toBe(true)
  })

  it('没有任何素材时给出空状态', async () => {
    stubShowcase({ items: [] })
    const wrapper = await mountShowcase()

    expect(wrapper.find('[data-testid="showcase-empty"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="showcase-entries"]').exists()).toBe(false)
  })
})
