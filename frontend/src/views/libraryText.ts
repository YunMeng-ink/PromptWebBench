// 题库界面的纯展示规则与剪贴板文本拼装。这里不依赖 Vue 与网络，便于逐条单测。

import type { LibraryQuestion } from '@/api/library'

/** PLACEHOLDER 是缺失取值的占位符。 */
export const PLACEHOLDER = '—'

/** roleKey 返回题库角色的文案键。 */
export function roleKey(role: string): string {
  return role === 'owner' ? 'library.roleOwner' : 'library.roleCollaborator'
}

/** kindKey 返回题目类型的文案键。 */
export function kindKey(kind: string): string {
  return kind === 'derived' ? 'library.kindDerived' : 'library.kindReference'
}

/** localizedPurpose 按当前语言选择题目的考察目的名称。 */
export function localizedPurpose(question: LibraryQuestion, locale: string): string {
  const name = locale === 'en' ? question.purpose_name_en : question.purpose_name_zh
  return name === '' ? PLACEHOLDER : name
}

/** localizedCapability 按当前语言选择题目的考察维度名称。 */
export function localizedCapability(question: LibraryQuestion, locale: string): string {
  const name = locale === 'en' ? question.capability_name_en : question.capability_name_zh
  return name === '' ? PLACEHOLDER : name
}

/** sourceMissing 判定题目的来源提示词是否已不可用（需求 §10.7 引用题）。 */
export function sourceMissing(question: LibraryQuestion): boolean {
  return question.kind === 'reference' && !question.source_available
}

/** sourceLabel 返回来源提示词的标识与版本，供界面与复制文本共用。 */
export function sourceLabel(question: LibraryQuestion): string {
  if (question.source_prompt_id === undefined) {
    return PLACEHOLDER
  }
  const version = question.source_version_no ?? ''
  return version === ''
    ? `#${question.source_prompt_id}`
    : `#${question.source_prompt_id} (${version})`
}

/**
 * remainingDays 计算回收站条目的剩余保留天数。
 * 已到期返回 0，避免界面出现负数天数。
 */
export function remainingDays(expiresAt: string, now: Date = new Date()): number {
  const expires = Date.parse(expiresAt)
  if (Number.isNaN(expires)) {
    return 0
  }
  const diff = expires - now.getTime()
  if (diff <= 0) {
    return 0
  }
  return Math.ceil(diff / (24 * 60 * 60 * 1000))
}

/** CopyLabels 是「复制完整信息」里各字段的字段名，由语言包提供。 */
export interface CopyLabels {
  title: string
  purpose: string
  capability: string
  version: string
  tags: string
  description: string
  kind: string
  source: string
  body: string
}

/** PromptCopyInput 是提示词详情页可用于复制的内容。 */
export interface PromptCopyInput {
  title: string
  purpose: string
  capability: string
  versionNo: string
  tags: string[]
  description: string
  body: string
}

/** promptBodyText 返回「复制提示词」的内容：只有正文（需求 §10.8）。 */
export function promptBodyText(body: string): string {
  return body
}

/** promptFullText 返回「复制完整信息」的内容：带字段名的完整条目。 */
export function promptFullText(input: PromptCopyInput, labels: CopyLabels): string {
  return joinFields(
    [
      [labels.title, input.title],
      [labels.purpose, input.purpose],
      [labels.capability, input.capability],
      [labels.version, input.versionNo],
      [labels.tags, input.tags.join('、')],
      [labels.description, input.description],
    ],
    labels.body,
    input.body,
  )
}

/** QuestionCopyInput 是题库题目可用于复制的内容。取值由界面按当前语言解析后传入，
 * 使本模块不持有任何界面文案。 */
export interface QuestionCopyInput {
  title: string
  kindName: string
  purpose: string
  capability: string
  tags: string[]
  description: string
  source: string
  body: string
}

/** questionFullText 返回题库题目的「复制完整信息」内容。 */
export function questionFullText(input: QuestionCopyInput, labels: CopyLabels): string {
  return joinFields(
    [
      [labels.title, input.title],
      [labels.kind, input.kindName],
      [labels.purpose, input.purpose],
      [labels.capability, input.capability],
      [labels.tags, input.tags.join('、')],
      [labels.description, input.description],
      [labels.source, input.source === PLACEHOLDER ? '' : input.source],
    ],
    labels.body,
    input.body,
  )
}

/**
 * joinFields 把字段名与取值拼成文本：空取值的字段整行省略，
 * 正文单独成段落便于粘贴后直接使用。
 */
function joinFields(pairs: [string, string][], bodyLabel: string, body: string): string {
  const lines: string[] = []
  for (const [label, value] of pairs) {
    if (value.trim() === '') {
      continue
    }
    lines.push(`${label}：${value}`)
  }
  const head = lines.join('\n')
  const tail = body.trim() === '' ? `${bodyLabel}\n${PLACEHOLDER}` : `${bodyLabel}\n${body}`
  return head === '' ? tail : `${head}\n\n${tail}`
}

/**
 * downloadText 触发浏览器下载一段文本。
 * 用 Blob 与临时链接完成，不依赖任何后端下载端点。
 */
export function downloadText(filename: string, content: string): void {
  const blob = new Blob([content], { type: 'text/markdown;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  document.body.append(anchor)
  anchor.click()
  anchor.remove()
  // 延迟撤销：Firefox 在下载尚未启动时撤销会中止保存动作。
  window.setTimeout(() => URL.revokeObjectURL(url), 1000)
}
