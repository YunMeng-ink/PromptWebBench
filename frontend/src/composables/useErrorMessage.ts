import { useI18n } from 'vue-i18n'
import { ApiError } from '@/api/client'
import { translateErrorCode } from '@/i18n'

/** 把接口错误转换为当前语言的提示文案。 */
export function useErrorMessage() {
  const { t } = useI18n()

  function messageOf(error: unknown): string {
    if (error instanceof ApiError) {
      return translateErrorCode(error.code, error.message)
    }
    return t('error.internal')
  }

  return { messageOf }
}
