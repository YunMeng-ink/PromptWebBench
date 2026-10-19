import { describe, expect, it } from 'vitest'
import { formatDate, formatDateTime, formatTime, timeZoneLabel } from '@/utils/datetime'

// 用例固定使用带时区偏移的输入，断言不依赖运行机器的时区：
// 只要输出与本机时区换算结果一致即可，比较对象由同一 Date 计算得出。
const sample = '2026-10-19T04:30:00Z'

describe('时间格式化', () => {
  it('按本地时区输出日期与时分', () => {
    const expected = new Intl.DateTimeFormat('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hourCycle: 'h23',
    }).format(new Date(sample))

    expect(formatDateTime(sample, 'zh')).toBe(expected)
  })

  it('英文界面使用英文区域格式', () => {
    const expected = new Intl.DateTimeFormat('en-US', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hourCycle: 'h23',
    }).format(new Date(sample))

    expect(formatDateTime(sample, 'en')).toBe(expected)
  })

  it('日期与时分使用同一本地时区', () => {
    const parsed = new Date(sample)
    expect(formatDate(sample, 'zh')).toBe(
      new Intl.DateTimeFormat('zh-CN', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
      }).format(parsed),
    )
    expect(formatTime(sample, 'zh')).toBe(
      new Intl.DateTimeFormat('zh-CN', {
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hourCycle: 'h23',
      }).format(parsed),
    )
  })

  it('输出不再包含原始 UTC 后缀', () => {
    expect(formatDateTime(sample, 'zh')).not.toContain('Z')
    expect(formatDate(sample, 'zh')).not.toContain('T')
  })

  it('无法解析的取值原样返回', () => {
    expect(formatDateTime('not-a-date', 'zh')).toBe('not-a-date')
    expect(formatDateTime('', 'zh')).toBe('')
    expect(formatDateTime(undefined, 'zh')).toBe('')
  })

  it('时区名称可用于界面标注', () => {
    expect(typeof timeZoneLabel()).toBe('string')
  })
})
