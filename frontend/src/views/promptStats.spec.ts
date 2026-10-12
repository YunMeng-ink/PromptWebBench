import { describe, expect, it } from 'vitest'
import type { ModelStatsWindow, StatsWindow } from '@/api/stats'
import {
  confidenceKey,
  discriminationOf,
  formatScore,
  meanOf,
  modelLabel,
  PLACEHOLDER,
} from '@/views/promptStats'

function windowWith(overrides: Partial<StatsWindow> = {}): StatsWindow {
  return {
    sufficient: true,
    discrimination: 2.6,
    confidence: 'medium',
    valid_model_count: 4,
    total_rating_count: 32,
    ...overrides,
  }
}

function modelWindowWith(overrides: Partial<ModelStatsWindow> = {}): ModelStatsWindow {
  return {
    has_data: true,
    mean: 3.75,
    adjusted: 3.5,
    rating_count: 8,
    user_count: 8,
    sd: 0.7,
    ci_half: 0.48,
    confidence_score: 0.6,
    confidence: 'medium',
    ...overrides,
  }
}

describe('区分度展示', () => {
  it('数据充足时给出数值', () => {
    expect(discriminationOf(windowWith())).toBe(2.6)
  })

  it('数据不足时给出 null，调用方显示 C-06 文案', () => {
    // 后端在数据不足时不给出数值，前端必须区分「暂不计算」与「区分度为零」。
    expect(discriminationOf(windowWith({ sufficient: false, discrimination: null }))).toBeNull()
  })

  it('数据不足时即便后端给出数值也不展示', () => {
    expect(discriminationOf(windowWith({ sufficient: false, discrimination: 0 }))).toBeNull()
  })

  it('区分度为零与数据不足是两件事', () => {
    // 极差为 0 时区分度恰为 1，此时数据充足，必须展示 1 而不是占位文案。
    expect(discriminationOf(windowWith({ discrimination: 1 }))).toBe(1)
  })
})

describe('完成度展示', () => {
  it('有观测值时给出原始平均分', () => {
    expect(meanOf(modelWindowWith())).toBe(3.75)
  })

  it('置信度为低不影响是否给出数值', () => {
    // 样本量不足由置信度表达，不构成「无数据」。
    const low = modelWindowWith({ confidence: 'low', confidence_score: 0.2 })
    expect(meanOf(low)).toBe(3.75)
  })

  it('无观测值时给出 null', () => {
    const empty = modelWindowWith({ has_data: false, mean: null, rating_count: 0, user_count: 0 })
    expect(meanOf(empty)).toBeNull()
  })

  it('formatScore 把无值渲染为占位符而不是 0', () => {
    expect(formatScore(null)).toBe(PLACEHOLDER)
    expect(formatScore(3.75)).toBe('3.75')
    expect(formatScore(4)).toBe('4.00')
  })
})

describe('置信度与模型名', () => {
  it('置信度等级映射到文案键后缀', () => {
    expect(confidenceKey('high')).toBe('High')
    expect(confidenceKey('medium')).toBe('Medium')
    expect(confidenceKey('low')).toBe('Low')
  })

  it('模型名为空时回落到 ID', () => {
    expect(modelLabel('', 42)).toBe('#42')
    expect(modelLabel('模型甲', 42)).toBe('模型甲')
  })
})
