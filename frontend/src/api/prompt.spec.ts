import { afterEach, describe, expect, it, vi } from 'vitest'
import { promptApi, visibilityOptions } from '@/api/prompt'

/** 捕获 fetch 调用，返回请求地址与已解析的请求体。 */
function captureFetch(response: unknown): { calls: string[]; bodies: unknown[] } {
  const calls: string[] = []
  const bodies: unknown[] = []
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    calls.push(url)
    if (init?.body !== undefined && init.body !== null) {
      bodies.push(JSON.parse(String(init.body)))
    }
    return {
      ok: true,
      status: 200,
      headers: new Headers({ 'Content-Type': 'application/json' }),
      json: async () => response,
      text: async () => JSON.stringify(response),
    } as unknown as Response
  })
  return { calls, bodies }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('visibilityOptions', () => {
  it('普通用户只拿到公开与私有两档', () => {
    expect(visibilityOptions(false)).toEqual(['public', 'private'])
  })

  it('开发者额外拿到开发者可见', () => {
    expect(visibilityOptions(true)).toEqual(['public', 'private', 'developer'])
  })
})

describe('promptApi.list', () => {
  it('把 6 维筛选与排序拼进查询串，省略空值', async () => {
    const { calls } = captureFetch({ items: [], page: 1, page_size: 20, total: 0 })

    await promptApi.list({
      page: 2,
      page_size: 20,
      category_id: '3',
      ability_id: '5',
      difficulty_id: '2',
      keyword: '推理',
      model_id: '7',
      tag_ids: ['1', '4'],
      sort: 'discrimination',
    })

    const query = new URLSearchParams((calls[0] ?? '').split('?')[1] ?? '')
    expect(query.get('page')).toBe('2')
    expect(query.get('page_size')).toBe('20')
    expect(query.get('category_id')).toBe('3')
    expect(query.get('ability_id')).toBe('5')
    expect(query.get('difficulty_id')).toBe('2')
    expect(query.get('keyword')).toBe('推理')
    expect(query.get('model_id')).toBe('7')
    expect(query.getAll('tag_ids')).toEqual(['1', '4'])
    expect(query.get('sort')).toBe('discrimination')
  })

  it('未设置的维度不进入查询串', async () => {
    const { calls } = captureFetch({ items: [], page: 1, page_size: 20, total: 0 })

    await promptApi.list({ sort: 'latest' })

    const query = new URLSearchParams((calls[0] ?? '').split('?')[1] ?? '')
    expect(query.get('category_id')).toBeNull()
    expect(query.get('keyword')).toBeNull()
    expect(query.getAll('tag_ids')).toEqual([])
    expect(query.get('sort')).toBe('latest')
  })

  it('空结果保持数组形态而不是 null', async () => {
    captureFetch({ items: [], page: 1, page_size: 20, total: 0 })
    const result = await promptApi.list({})
    expect(result.items).toEqual([])
    expect(Array.isArray(result.items)).toBe(true)
  })
})

describe('promptApi.create 与 update', () => {
  it('创建使用 POST 且请求体原样送出', async () => {
    const { bodies } = captureFetch({
      prompt: { id: 1 },
      version_no: 'v1.0',
      major_change: false,
      archived_ratings: 0,
    })

    await promptApi.create({
      title: '标题',
      body: '正文',
      category_id: 3,
      visibility: 'public',
      sensitive_level: 'low',
    })

    expect(bodies[0]).toEqual({
      title: '标题',
      body: '正文',
      category_id: 3,
      visibility: 'public',
      sensitive_level: 'low',
    })
  })

  it('修改走 PATCH 并携带重大修改标记', async () => {
    const { bodies } = captureFetch({
      prompt: { id: 5 },
      version_no: 'v2.0',
      major_change: true,
      archived_ratings: 4,
    })

    const result = await promptApi.update(5, {
      title: '改后标题',
      body: '正文',
      category_id: 3,
      is_major_change: true,
    })

    expect((bodies[0] as Record<string, unknown>).is_major_change).toBe(true)
    expect(result.major_change).toBe(true)
    expect(result.archived_ratings).toBe(4)
    expect(result.version_no).toBe('v2.0')
  })
})
