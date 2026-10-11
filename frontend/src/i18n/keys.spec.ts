import { describe, expect, it } from 'vitest'
import en from '@/i18n/en'
import zh from '@/i18n/zh'

function collectKeys(value: unknown, prefix = ''): string[] {
  if (typeof value !== 'object' || value === null) {
    return [prefix]
  }
  return Object.entries(value).flatMap(([key, child]) =>
    collectKeys(child, prefix === '' ? key : `${prefix}.${key}`),
  )
}

describe('语言包键', () => {
  it('中英键完全一一对应', () => {
    const zhKeys = collectKeys(zh).sort()
    const enKeys = collectKeys(en).sort()
    expect(enKeys).toEqual(zhKeys)
  })

  it('每个键都有非空文案', () => {
    for (const key of collectKeys(zh)) {
      const value = key.split('.').reduce<unknown>((node, part) => {
        if (typeof node !== 'object' || node === null) {
          return undefined
        }
        return (node as Record<string, unknown>)[part]
      }, zh)
      expect(typeof value, `键 ${key} 的值不是字符串`).toBe('string')
      expect((value as string).trim(), `键 ${key} 的文案为空`).not.toBe('')
    }
  })
})
