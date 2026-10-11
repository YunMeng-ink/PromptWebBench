import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import en from '@/i18n/en'
import zh from '@/i18n/zh'

// vitest 以 frontend 为工作目录运行，据此定位后端错误码表。
const backendErrorsFile = resolve(process.cwd(), '../backend/internal/http/errors.go')

/** 从后端错误码表提取全部码值，使前后端错误码不会各自漂移。 */
function backendErrorCodes(): string[] {
  const source = readFileSync(backendErrorsFile, 'utf8')
  const matches = source.matchAll(/Code\w+\s+Code\s*=\s*"([a-z_]+)"/g)
  return [...matches].map((match) => match[1]).filter((code): code is string => code !== undefined)
}

describe('错误码语言包覆盖', () => {
  it('后端错误码表非空且可解析', () => {
    expect(backendErrorCodes().length).toBeGreaterThan(30)
  })

  it('每个后端错误码在中英语言包中都有对应文案', () => {
    for (const code of backendErrorCodes()) {
      expect(zh.error, `中文语言包缺少 error.${code}`).toHaveProperty(code)
      expect(en.error, `英文语言包缺少 error.${code}`).toHaveProperty(code)
    }
  })

  it('语言包中没有后端未定义的错误码', () => {
    const known = new Set(backendErrorCodes())
    for (const code of Object.keys(zh.error)) {
      expect(known.has(code), `error.${code} 在后端错误码表中不存在`).toBe(true)
    }
  })
})
