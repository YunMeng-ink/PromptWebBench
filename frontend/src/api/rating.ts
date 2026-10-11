// 评分的接口封装。

import { api } from './client'
import type { Page } from './prompt'

export interface Rating {
  id: number
  prompt_id: number
  model_id: number
  score: number
  edit_count: number
  max_edits: number
  created_at: string
  updated_at: string
}

/** PromptModelRating 是详情页按模型聚合的评分，以及当前用户在该模型上的取值。 */
export interface PromptModelRating {
  model_id: number
  model_name: string
  rating_count: number
  user_count: number
  mean_score: number
  my_score?: number
  my_edit_count?: number
}

export interface MyRating {
  id: number
  prompt_id: number
  prompt_title: string
  model_id: number
  model_name: string
  score: number
  edit_count: number
  created_at: string
}

export interface RatingPayload {
  model_id: number
  score: number
}

export const ratingApi = {
  submit(promptId: number, payload: RatingPayload): Promise<Rating> {
    return api.post<Rating>(`/prompts/${promptId}/ratings`, payload)
  },

  update(promptId: number, payload: RatingPayload): Promise<Rating> {
    return api.put<Rating>(`/prompts/${promptId}/ratings`, payload)
  },

  remove(promptId: number, modelId: number): Promise<void> {
    return api.delete<void>(`/prompts/${promptId}/ratings/${modelId}`)
  },

  byPrompt(promptId: number): Promise<{ items: PromptModelRating[] }> {
    return api.get<{ items: PromptModelRating[] }>(`/prompts/${promptId}/ratings`)
  },

  mine(page = 1, pageSize = 20): Promise<Page<MyRating>> {
    return api.get<Page<MyRating>>(`/ratings/mine?page=${page}&page_size=${pageSize}`)
  },
}

/** scoreLabels 是 1-5 锚定量表的文案键，与需求 §3.2 的含义一一对应。 */
export const SCORE_KEYS = ['score1', 'score2', 'score3', 'score4', 'score5'] as const
export type ScoreKey = (typeof SCORE_KEYS)[number]
