// 提示词与字典的接口封装。类型与后端 OpenAPI 契约一一对应。

import { api, type RequestOptions } from './client'

export interface Purpose {
  id: number
  slug: string
  name_zh: string
  name_en: string
  description_zh: string
  description_en: string
  is_high_risk: boolean
}

export interface Capability {
  id: number
  slug: string
  name_zh: string
  name_en: string
  group_key: 'cognitive' | 'execution' | 'reliability'
  description_zh: string
  description_en: string
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
  kind: 'theme' | 'form'
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
  purpose_id: number
  capability_id: number
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
  purpose_name_zh: string
  purpose_name_en: string
  capability_name_zh: string
  capability_name_en: string
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
  purpose_id?: string
  capability_id?: string
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
  purpose_id: number
  capability_id: number
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
  purpose: Purpose
  capability: Capability
  difficulty?: Difficulty
  tags: Tag[]
  uploader: string
  major_modification_notice?: string
}

export interface PromptPayload {
  title: string
  body: string
  description?: string
  purpose_id: number
  capability_id: number
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
      purpose_id: filters.purpose_id,
      capability_id: filters.capability_id,
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
  importMarkdown(file: File): Promise<ImportedMarkdown> {
    const form = new FormData()
    form.append('file', file)
    // 走统一封装：FormData 请求自动携带 CSRF 双提交头，错误经 ApiError
    // 保留稳定错误码与 401 语义（浏览器自动设置 multipart 边界）。
    return api.post<ImportedMarkdown>('/prompts/import-markdown', form)
  },
}

export const dictionaryApi = {
  purposes(options?: RequestOptions): Promise<{ items: Purpose[] }> {
    return api.get<{ items: Purpose[] }>('/purposes', options)
  },
  capabilities(options?: RequestOptions): Promise<{ items: Capability[] }> {
    return api.get<{ items: Capability[] }>('/capabilities', options)
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
