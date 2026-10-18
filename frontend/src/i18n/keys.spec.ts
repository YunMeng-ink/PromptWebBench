import { readdirSync, readFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
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

/** sourceFiles 列出 src 下参与界面渲染的源码文件。 */
function sourceFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name)
    if (entry.isDirectory()) {
      return entry.name === 'testing' ? [] : sourceFiles(path)
    }
    if (entry.name.endsWith('.spec.ts') || !/\.(ts|vue)$/.test(entry.name)) {
      return []
    }
    return [path]
  })
}

/** 直接写在 t('...') 中的文案键。带插值的动态键由各自的用例覆盖。 */
const STATIC_KEY = /\bt\(\s*['"]([a-zA-Z0-9_.-]+)['"]/g

function hasKey(root: unknown, key: string): boolean {
  return key.split('.').every((part) => {
    if (typeof root !== 'object' || root === null) {
      return false
    }
    const next = (root as Record<string, unknown>)[part]
    if (next === undefined) {
      return false
    }
    root = next
    return true
  })
}

describe('语言包键', () => {
  it('中英键完全一一对应', () => {
    const zhKeys = collectKeys(zh).sort()
    const enKeys = collectKeys(en).sort()
    expect(enKeys).toEqual(zhKeys)
  })

  it('源码中直接引用的文案键都存在', () => {
    const here = dirname(fileURLToPath(import.meta.url))
    const sourceRoot = resolve(here, '..')
    const missing: string[] = []
    for (const file of sourceFiles(sourceRoot)) {
      const source = readFileSync(file, 'utf8')
      for (const match of source.matchAll(STATIC_KEY)) {
        const key = match[1]
        if (key === undefined || hasKey(zh, key)) {
          continue
        }
        missing.push(`${file.slice(sourceRoot.length + 1)}: ${key}`)
      }
    }
    expect(missing).toEqual([])
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
