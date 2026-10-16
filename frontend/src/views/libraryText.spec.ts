import { describe, expect, it } from 'vitest'
import type { LibraryQuestion } from '@/api/library'
import {
  type CopyLabels,
  kindKey,
  localizedCategory,
  PLACEHOLDER,
  promptBodyText,
  promptFullText,
  questionFullText,
  remainingDays,
  roleKey,
  sourceLabel,
  sourceMissing,
} from '@/views/libraryText'

const labels: CopyLabels = {
  title: '标题',
  category: '类别',
  version: '版本',
  tags: '标签',
  description: '说明',
  kind: '类型',
  source: '来源提示词',
  body: '正文',
}

function questionFixture(overrides: Partial<LibraryQuestion> = {}): LibraryQuestion {
  return {
    id: 7,
    library_id: 3,
    kind: 'reference',
    title: '引用题标题',
    body: '引用题正文',
    description: '',
    category_name_zh: '逻辑推理',
    category_name_en: 'Logical Reasoning',
    sensitive_level: 'low',
    sort_order: 1,
    tags: [],
    source_prompt_id: 12,
    source_version_no: 'v2.0',
    source_available: true,
    source_major_modified: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('题库展示规则', () => {
  it('角色与题目类型映射到语言包键', () => {
    expect(roleKey('owner')).toBe('library.roleOwner')
    expect(roleKey('collaborator')).toBe('library.roleCollaborator')
    expect(roleKey('unknown')).toBe('library.roleCollaborator')
    expect(kindKey('reference')).toBe('library.kindReference')
    expect(kindKey('derived')).toBe('library.kindDerived')
  })

  it('类别名称按语言选择，缺失时给占位符', () => {
    const question = questionFixture()
    expect(localizedCategory(question, 'zh')).toBe('逻辑推理')
    expect(localizedCategory(question, 'en')).toBe('Logical Reasoning')
    expect(
      localizedCategory(questionFixture({ category_name_zh: '', category_name_en: '' }), 'zh'),
    ).toBe(PLACEHOLDER)
  })

  it('引用题在来源不可用时标注，派生题不受影响', () => {
    expect(sourceMissing(questionFixture({ source_available: false }))).toBe(true)
    expect(sourceMissing(questionFixture())).toBe(false)
    expect(sourceMissing(questionFixture({ kind: 'derived', source_available: false }))).toBe(false)
  })

  it('来源标识带出提示词编号与版本', () => {
    expect(sourceLabel(questionFixture())).toBe('#12 (v2.0)')
    expect(sourceLabel(questionFixture({ source_version_no: '' }))).toBe('#12')
    expect(sourceLabel(questionFixture({ source_prompt_id: undefined }))).toBe(PLACEHOLDER)
  })

  it('剩余天数向上取整，已到期为 0', () => {
    const now = new Date('2026-01-01T00:00:00Z')
    expect(remainingDays('2026-01-02T00:00:00Z', now)).toBe(1)
    expect(remainingDays('2026-01-01T12:00:00Z', now)).toBe(1)
    expect(remainingDays('2025-12-31T00:00:00Z', now)).toBe(0)
    expect(remainingDays('不是时间', now)).toBe(0)
  })
})

describe('复制文本', () => {
  it('复制提示词只给出正文', () => {
    expect(promptBodyText('正文内容')).toBe('正文内容')
  })

  it('复制完整信息带上字段名，空字段整行省略', () => {
    const text = promptFullText(
      {
        title: '标题一',
        category: '逻辑推理',
        versionNo: 'v1.0',
        tags: ['标签甲', '标签乙'],
        description: '',
        body: '正文内容',
      },
      labels,
    )
    expect(text).toBe(
      '标题：标题一\n类别：逻辑推理\n版本：v1.0\n标签：标签甲、标签乙\n\n正文\n正文内容',
    )
    expect(text).not.toContain('说明：')
  })

  it('题库题目的完整信息含类型与来源', () => {
    const text = questionFullText(
      {
        title: '引用题标题',
        kindName: '引用题',
        category: '逻辑推理',
        tags: [],
        description: '题目说明',
        source: '#12 (v2.0)',
        body: '引用题正文',
      },
      labels,
    )
    expect(text).toContain('类型：引用题')
    expect(text).toContain('来源提示词：#12 (v2.0)')
    expect(text).toContain('说明：题目说明')
    expect(text).toContain('正文\n引用题正文')
  })

  it('来源缺失时完整信息省略来源行，正文给占位符', () => {
    const text = questionFullText(
      {
        title: '（来源提示词已删除）',
        kindName: '引用题',
        category: PLACEHOLDER,
        tags: [],
        description: '',
        source: PLACEHOLDER,
        body: '',
      },
      labels,
    )
    expect(text).toBe(
      `标题：（来源提示词已删除）\n类型：引用题\n类别：${PLACEHOLDER}\n\n正文\n${PLACEHOLDER}`,
    )
  })
})
