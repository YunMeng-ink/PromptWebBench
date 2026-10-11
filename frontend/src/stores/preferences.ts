import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { type AppLocale, setLocale as applyLocale, currentLocale } from '@/i18n'

export type ThemeMode = 'light' | 'dark'

const THEME_STORAGE_KEY = 'pb.theme'

function detectTheme(): ThemeMode {
  if (typeof localStorage !== 'undefined') {
    const stored = localStorage.getItem(THEME_STORAGE_KEY)
    if (stored === 'light' || stored === 'dark') {
      return stored
    }
  }
  if (typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches) {
    return 'dark'
  }
  return 'light'
}

function applyTheme(theme: ThemeMode): void {
  if (typeof document === 'undefined') {
    return
  }
  document.documentElement.classList.toggle('dark', theme === 'dark')
  document.documentElement.dataset.theme = theme
}

/** 界面偏好：语言与主题。仅此两项需要全局状态（开发方案 §4.8）。 */
export const usePreferencesStore = defineStore('preferences', () => {
  const locale = ref<AppLocale>(currentLocale())
  const theme = ref<ThemeMode>(detectTheme())

  applyTheme(theme.value)

  const isDark = computed(() => theme.value === 'dark')

  function setLocale(next: AppLocale): void {
    locale.value = next
    applyLocale(next)
  }

  function setTheme(next: ThemeMode): void {
    theme.value = next
    applyTheme(next)
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(THEME_STORAGE_KEY, next)
    }
  }

  function toggleTheme(): void {
    setTheme(theme.value === 'dark' ? 'light' : 'dark')
  }

  return { locale, theme, isDark, setLocale, setTheme, toggleTheme }
})
