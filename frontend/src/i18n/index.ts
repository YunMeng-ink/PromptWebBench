import { createI18n } from 'vue-i18n'
import en from './en'
import zh from './zh'

export const SUPPORTED_LOCALES = ['zh', 'en'] as const
export type AppLocale = (typeof SUPPORTED_LOCALES)[number]

export const DEFAULT_LOCALE: AppLocale = 'zh'
const LOCALE_STORAGE_KEY = 'pb.locale'

function isSupported(value: string): value is AppLocale {
  return (SUPPORTED_LOCALES as readonly string[]).includes(value)
}

/** 优先取用户上次选择，其次按浏览器语言，最后回退到默认语言。 */
export function detectLocale(): AppLocale {
  if (typeof localStorage !== 'undefined') {
    const stored = localStorage.getItem(LOCALE_STORAGE_KEY)
    if (stored !== null && isSupported(stored)) {
      return stored
    }
  }
  if (typeof navigator !== 'undefined') {
    for (const candidate of navigator.languages ?? [navigator.language]) {
      const primary = candidate.split('-')[0] ?? ''
      if (isSupported(primary)) {
        return primary
      }
    }
  }
  return DEFAULT_LOCALE
}

export const i18n = createI18n({
  legacy: false,
  locale: detectLocale(),
  fallbackLocale: DEFAULT_LOCALE,
  messages: { zh, en },
})

export function currentLocale(): AppLocale {
  return i18n.global.locale.value as AppLocale
}

export function setLocale(locale: AppLocale): void {
  i18n.global.locale.value = locale
  if (typeof localStorage !== 'undefined') {
    localStorage.setItem(LOCALE_STORAGE_KEY, locale)
  }
  if (typeof document !== 'undefined') {
    document.documentElement.lang = locale === 'zh' ? 'zh-CN' : 'en'
  }
}

/** 把后端错误码翻译为当前语言的文案，未收录的码回退到服务端消息。 */
export function translateErrorCode(code: string, fallback: string): string {
  const key = `error.${code}`
  return i18n.global.te(key) ? i18n.global.t(key) : fallback
}
