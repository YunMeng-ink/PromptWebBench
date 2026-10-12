// 统计接口的封装。类型与后端 OpenAPI 契约一一对应。
//
// 区分度在数据不足时为 null：后端按 C-06 不给出数值，前端据此显示
// 「数据不足，暂不计算区分度」，因此这里的类型是可空数字。

import { api } from './client'

export type ConfidenceLevel = 'high' | 'medium' | 'low'

export type RecommendTagKind =
  | 'obsolete'
  | 'suspected_obsolete'
  | 'saturation'
  | 'suspected_saturation'

/** Window 是一个时间窗口在一条提示词上的统计结果。 */
export interface StatsWindow {
  sufficient: boolean
  discrimination: number | null
  confidence: ConfidenceLevel
  valid_model_count: number
  total_rating_count: number
}

/** ModelWindow 是单个模型在一个窗口内的完成度统计。 */
export interface ModelStatsWindow {
  has_data: boolean
  mean: number | null
  adjusted: number
  rating_count: number
  user_count: number
  sd: number
  ci_half: number
  confidence_score: number
  confidence: ConfidenceLevel
}

export interface ModelStats {
  model_id: number
  model_name: string
  history: ModelStatsWindow
  recent: ModelStatsWindow
}

/** RecommendTag 是已确认的推荐标签（需求 §6.3：未确认的不进前台）。 */
export interface RecommendTag {
  id: number
  model_id: number
  scope: 'prompt' | 'prompt_model'
  kind: RecommendTagKind
  label_zh: string
  label_en: string
  confidence: ConfidenceLevel
  reason: string
}

export interface PromptStats {
  prompt_id: number
  history: StatsWindow
  recent: StatsWindow
  models: ModelStats[]
  recommendation: RecommendTag[]
  computed_at: string
}

export const statsApi = {
  byPrompt(promptId: number): Promise<PromptStats> {
    return api.get<PromptStats>(`/prompts/${promptId}/stats`)
  },
}
