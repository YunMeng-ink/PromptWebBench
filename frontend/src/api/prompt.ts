// 提示词与字典的接口封装。类型与后端 OpenAPI 契约一一对应。

import { API_PREFIX, api, type RequestOptions } from './client'

export interface Category {
  id: number
  slug: string
  name_zh: string
  name_en: string
  is_high_risk: boolean
}

export interface Ability {
  id: number
  slug: string
  name_zh: string
  name_en: string
}

export interface Difficulty {
  id: number
  slug: string
  name_zh: string
  name_en: string
  level: number
}

export interface Tag {
  id: number
  slug: string
  name_zh: string
  name_en: string
  color: string
}

export interface Model {
  id: number
  name: string
  model_key: string
  provider_key: string
  provider_name: string
}

export type Visibility = 'public' | 'private' | 'developer'
export type SensitiveLevel = 'low' | 'medium' | 'high'
export type SortOrder = 'latest' | 'top_rated' | 'discrimination'

export interface Prompt {
  id: number
  title: string
  description: string
  category_id: number
  ability_id?: number
  difficulty_id?: number
  visibility: Visibility
  sensitive_level: SensitiveLevel
  uploader_id: number
  version_no: string
  major_modified: boolean
  comment_count: number
  favorite_count: number
  created_at: string
  updated_at: string
}

export interface PromptListItem extends Prompt {
  category_name_zh: string
  category_name_en: string
  ability_name_zh?: string
  ability_name_en?: string
  difficulty_name_zh?: string
  difficulty_name_en?: string
  discrimination: number
  data_sufficient: boolean
  average_adjusted: number
}

/** 分页响应的固定形状（后端 C-40）：空列表为 [] 而非 null。 */
export interface Page<T> {
  items: T[]
  page: number
  page_size: number
  total: number
}

export interface PromptListQuery {
  page?: number
  page_size?: number
  category_id?: string
  ability_id?: string
  difficulty_id?: string
  keyword?: string
  model_id?: string
  tag_ids?: string[]
  sort?: SortOrder
}

export interface PromptVersion {
  id: number
  version_no: string
  title: string
  body: string
  description: string
  category_id: number
  ability_id?: number
  difficulty_id?: number
  visibility: Visibility
  sensitive_level: SensitiveLevel
  is_major: boolean
  change_note: string
  author_id: number
  created_at: string
}

export interface PromptDetail {
  prompt: Prompt
  version: PromptVersion
  versions: PromptVersion[]
  category: Category
  tags: Tag[]
  uploader: string
  major_modification_notice?: string
}

export interface PromptPayload {
  title: string
  body: string
  description?: string
  category_id: number
  ability_id?: number
  difficulty_id?: number
  visibility?: Visibility
  sensitive_level?: SensitiveLevel
  tag_ids?: number[]
  change_note?: string
  is_major_change?: boolean
}

export interface PromptWriteResult {
  prompt: Prompt
  version_no: string
  major_change: boolean
  archived_ratings: number
  notice?: string
}

export interface ImportedMarkdown {
  title: string
  body: string
  description: string
}

/** query 把筛选项拼成查询串，空值不参与拼接。 */
function query(params: Record<string, string | string[] | number | undefined>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === '') {
      continue
    }
    if (Array.isArray(value)) {
      for (const item of value) {
        search.append(key, item)
      }
      continue
    }
    search.set(key, String(value))
  }
  const text = search.toString()
  return text === '' ? '' : `?${text}`
}

export const promptApi = {
  list(filters: PromptListQuery, options?: RequestOptions): Promise<Page<PromptListItem>> {
    const search = query({
      page: filters.page,
      page_size: filters.page_size,
      category_id: filters.category_id,
      ability_id: filters.ability_id,
      difficulty_id: filters.difficulty_id,
      keyword: filters.keyword,
      model_id: filters.model_id,
      tag_ids: filters.tag_ids,
      sort: filters.sort,
    })
    return api.get<Page<PromptListItem>>(`/prompts${search}`, options)
  },

  detail(id: number, options?: RequestOptions): Promise<PromptDetail> {
    return api.get<PromptDetail>(`/prompts/${id}`, options)
  },

  mine(options?: RequestOptions): Promise<Page<PromptListItem>> {
    return api.get<Page<PromptListItem>>('/me/prompts', options)
  },

  create(payload: PromptPayload): Promise<PromptWriteResult> {
    return api.post<PromptWriteResult>('/prompts', payload)
  },

  update(id: number, payload: PromptPayload): Promise<PromptWriteResult> {
    return api.patch<PromptWriteResult>(`/prompts/${id}`, payload)
  },

  remove(id: number): Promise<void> {
    return api.delete<void>(`/prompts/${id}`)
  },

  /** importMarkdown 上传 .md 文件，返回解析出的标题、正文与描述。 */
  async importMarkdown(file: File): Promise<ImportedMarkdown> {
    const form = new FormData()
    form.append('file', file)

    const response = await fetch(`${API_PREFIX}/prompts/import-markdown`, {
      method: 'POST',
      body: form,
      credentials: 'include',
      headers: { Accept: 'application/json' },
    })
    if (!response.ok) {
      // 复用统一的错误形状：这里只处理非 2xx，读失败交由调用方处理。
      const payload = (await response.json().catch(() => null)) as {
        error: { code: string; message: string; details?: string[] }
      } | null
      throw new Error(payload?.error?.message ?? '导入失败')
    }
    return (await response.json()) as ImportedMarkdown
  },
}

export const dictionaryApi = {
  categories(options?: RequestOptions): Promise<{ items: Category[] }> {
    return api.get<{ items: Category[] }>('/categories', options)
  },
  abilities(options?: RequestOptions): Promise<{ items: Ability[] }> {
    return api.get<{ items: Ability[] }>('/abilities', options)
  },
  difficulties(options?: RequestOptions): Promise<{ items: Difficulty[] }> {
    return api.get<{ items: Difficulty[] }>('/difficulties', options)
  },
  tags(options?: RequestOptions): Promise<{ items: Tag[] }> {
    return api.get<{ items: Tag[] }>('/tags', options)
  },
  models(options?: RequestOptions): Promise<{ items: Model[] }> {
    return api.get<{ items: Model[] }>('/models', options)
  },
}

/** visibilityOptions 返回当前身份可选的可见性档位，与后端的判定一致。 */
export function visibilityOptions(canUseDeveloper: boolean): Visibility[] {
  const options: Visibility[] = ['public', 'private']
  if (canUseDeveloper) {
    options.push('developer')
  }
  return options
}
