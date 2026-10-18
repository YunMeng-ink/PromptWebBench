// 模型结果展示区的接口封装：评分附图上传、模型输出提交、公开聚合与素材列表。

import { api } from './client'

/** ShowcaseGroup 是展示区中一个模型的素材聚合，按提供厂商归类。 */
export interface ShowcaseGroup {
  model_id: number
  model_name: string
  provider_key: string
  provider_name: string
  image_count: number
  output_count: number
  last_contributed_at: string
}

/** ShowcaseEntry 是展示区中的一条素材：图片或模型输出文本。 */
export interface ShowcaseEntry {
  kind: 'image' | 'output'
  id: number
  prompt_id: number
  prompt_title: string
  username: string
  url?: string
  content?: string
  created_at: string
}

export interface ShowcaseEntryPage {
  items: ShowcaseEntry[]
  page: number
  page_size: number
  total: number
}

export interface RatingImage {
  id: number
  url: string
  filename: string
  created_at: string
}

export interface RatingOutput {
  id: number
  content: string
  created_at: string
}

/** MAX_IMAGE_BYTES 与后端展示区服务的大小上限一致。 */
export const MAX_IMAGE_BYTES = 8 * 1024 * 1024

/** MAX_IMAGE_COUNT 是单条评分可上传的图片张数，与后端保持一致。 */
export const MAX_IMAGE_COUNT = 4

/** MAX_OUTPUT_CHARS 是模型输出文本的字符上限。 */
export const MAX_OUTPUT_CHARS = 8000

export const showcaseApi = {
  groups(): Promise<{ items: ShowcaseGroup[] }> {
    return api.get<{ items: ShowcaseGroup[] }>('/showcase/groups')
  },

  entries(modelId: number, page = 1, pageSize = 20): Promise<ShowcaseEntryPage> {
    return api.get<ShowcaseEntryPage>(
      `/showcase/models/${modelId}/entries?page=${page}&page_size=${pageSize}`,
    )
  },

  uploadImage(promptId: number, modelId: number, file: File): Promise<{ image: RatingImage }> {
    const form = new FormData()
    form.append('image', file)
    return api.post<{ image: RatingImage }>(`/prompts/${promptId}/rating-images/${modelId}`, form)
  },

  submitOutput(
    promptId: number,
    modelId: number,
    content: string,
  ): Promise<{ output: RatingOutput }> {
    return api.post<{ output: RatingOutput }>(`/prompts/${promptId}/rating-outputs`, {
      model_id: modelId,
      content,
    })
  },
}
