// 统计展示的取值规则。
//
// 这些规则决定「什么时候显示数值、什么时候显示占位文案」，
// 是整个统计展示里唯一需要判断的地方，因此独立成模块供单元测试覆盖。

import type { ConfidenceLevel, ModelStatsWindow, StatsWindow } from '@/api/stats'

/** 数据不足时的占位符。C-06 统一为「数据不足，暂不计算区分度」，数值位显示「—」。 */
export const PLACEHOLDER = '—'

/**
 * discriminationOf 返回区分度该显示的内容。
 *
 * 数据不足时返回 null，调用方显示 C-06 规定的文案；而不是显示 0。
 * 0 会被读成「区分度为零」，与「暂不计算」是两件事。
 */
export function discriminationOf(window: StatsWindow): number | null {
  if (!window.sufficient) {
    return null
  }
  return window.discrimination
}

/**
 * meanOf 返回原始平均分该显示的内容。
 *
 * 该窗口内没有观测值时返回 null，调用方显示占位符。
 * 置信度为低不构成「无数据」：有观测值就给出数值，样本量不足由置信度表达。
 */
export function meanOf(window: ModelStatsWindow): number | null {
  if (!window.has_data) {
    return null
  }
  return window.mean
}

/** confidenceKey 返回置信度等级对应的文案键后缀。 */
export function confidenceKey(level: ConfidenceLevel): 'High' | 'Medium' | 'Low' {
  switch (level) {
    case 'high':
      return 'High'
    case 'medium':
      return 'Medium'
    default:
      return 'Low'
  }
}

/** formatScore 把完成度按两位小数展示；无值时给出占位符。 */
export function formatScore(value: number | null): string {
  return value === null ? PLACEHOLDER : value.toFixed(2)
}

/** modelLabel 返回模型在统计行里的展示名；模型已下线时回落到 ID。 */
export function modelLabel(name: string, modelId: number): string {
  return name === '' ? `#${modelId}` : name
}
