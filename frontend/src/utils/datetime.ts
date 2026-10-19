/**
 * 服务端统一以 UTC 的 RFC 3339 文本返回时刻。界面按浏览器所在时区呈现，
 * 避免用户把 UTC 时间误读成本地时间。
 */

const LOCALES: Record<string, string> = { zh: 'zh-CN', en: 'en-US' }

function localeTag(locale: string): string {
  return LOCALES[locale] ?? 'zh-CN'
}

function parse(raw: string | undefined): Date | null {
  if (raw === undefined || raw === '') {
    return null
  }
  const parsed = new Date(raw)
  return Number.isNaN(parsed.getTime()) ? null : parsed
}

/** formatDateTime 返回本地时区的日期与时分；无法解析时原样返回。 */
export function formatDateTime(raw: string | undefined, locale: string): string {
  const parsed = parse(raw)
  if (parsed === null) {
    return raw ?? ''
  }
  return new Intl.DateTimeFormat(localeTag(locale), {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
  }).format(parsed)
}

/** formatDate 返回本地时区的日期，用于只需要日期的位置。 */
export function formatDate(raw: string | undefined, locale: string): string {
  const parsed = parse(raw)
  if (parsed === null) {
    return raw ?? ''
  }
  return new Intl.DateTimeFormat(localeTag(locale), {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).format(parsed)
}

/** formatTime 返回本地时区的时分，用于同一天内的刷新时刻。 */
export function formatTime(raw: string | undefined, locale: string): string {
  const parsed = parse(raw)
  if (parsed === null) {
    return raw ?? ''
  }
  return new Intl.DateTimeFormat(localeTag(locale), {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hourCycle: 'h23',
  }).format(parsed)
}

/** timeZoneLabel 返回当前浏览器的时区名称，用于在界面上标注时间口径。 */
export function timeZoneLabel(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone
  } catch {
    return ''
  }
}
