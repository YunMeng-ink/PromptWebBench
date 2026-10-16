// 复制到剪贴板。写入失败时把失败反馈交给调用方，不静默吞掉。

import { ref } from 'vue'

/** useClipboard 返回复制动作与最近一次复制成功的提示键。 */
export function useClipboard(): {
  copied: ReturnType<typeof ref<string>>
  copy: (text: string, notice: string) => Promise<boolean>
} {
  const copied = ref('')

  async function copy(text: string, notice: string): Promise<boolean> {
    if (navigator.clipboard === undefined) {
      copied.value = ''
      return false
    }
    try {
      await navigator.clipboard.writeText(text)
      copied.value = notice
      return true
    } catch {
      copied.value = ''
      return false
    }
  }

  return { copied, copy }
}
