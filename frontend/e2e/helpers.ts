// 端到端测试共用的账户与页面操作。
//
// 注册与验证码发送都受认证类限流约束，而 Playwright 为每个规格文件单独加载本模块，
// 模块级变量无法跨文件共享。因此账户在全局前置里通过真实的注册流程创建一次，
// 各规格文件只负责登录：既保证登录走的是与真实用户相同的校验路径，
// 也不会因重复注册耗尽认证类额度。

import { readdirSync, readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import type { Page } from '@playwright/test'
import { expect } from '@playwright/test'

const here = dirname(fileURLToPath(import.meta.url))
const MAIL_DROP = resolve(here, '..', '..', 'backend', '.maildrop')

export const BASE_URL = 'http://127.0.0.1:4173'
export const SHARED_BASE_URL = BASE_URL
export const PASSWORD = 'E2ePassw0rd!'

/** SHARED_EMAIL 是整个套件共用的账户，各规格文件都用它。 */
export const SHARED_EMAIL = 'e2e-shared@example.com'

/** latestCodeFor 从 SMTP 接收器落盘的邮件中取出指定收件人的验证码。 */
export function latestCodeFor(email: string): string {
  const files = readdirSync(MAIL_DROP)
    .filter((name) => name.endsWith('.eml'))
    .sort()

  for (let i = files.length - 1; i >= 0; i -= 1) {
    const raw = readFileSync(resolve(MAIL_DROP, files[i] as string), 'utf8')
    if (!raw.includes(email)) {
      continue
    }
    const match = raw.match(/\b(\d{6})\b/)
    if (match?.[1] !== undefined) {
      return match[1]
    }
  }
  throw new Error(`邮件目录中未找到发往 ${email} 的验证码`)
}

/** loginShared 在给定页面上登录共用账户。 */
export async function loginShared(page: Page): Promise<string> {
  await page.goto('/login')

  // 等表单渲染完成再填值：页面刚导航时组件尚未挂载，直接 fill 会落空。
  const email = page.getByTestId('login-email')
  await expect(email).toBeVisible()
  await email.fill(SHARED_EMAIL)

  await page.getByTestId('login-password').fill(PASSWORD)
  await page.getByTestId('login-submit').click()

  await expect(page.getByTestId('header-user')).toBeVisible()
  return SHARED_EMAIL
}

/**
 * openAsShared 打开一个需要登录的页面。
 *
 * 登录态保存在内存中，重新导航会重建应用并重新探测会话。守卫与启动探测共享同一次
 * 请求，这里等待顶栏出现登录用户，并确认没有被守卫弹回登录页。
 */
export async function openAsShared(page: Page, path: string): Promise<void> {
  await page.goto(path)
  await expect(page).toHaveURL(new RegExp(`${path}$`))
  await expect(page.getByTestId('header-user')).toBeVisible()
}

/** createPrompt 通过上传表单发布一条公开提示词，返回其 ID。 */
export async function createPrompt(page: Page, title: string): Promise<number> {
  await openAsShared(page, '/upload')

  const titleInput = page.getByTestId('prompt-title')
  await expect(titleInput).toBeVisible()
  await titleInput.fill(title)
  await page.getByTestId('prompt-body').fill('端到端测试使用的提示词正文。')

  await page.getByTestId('prompt-category').locator('.el-select__wrapper').click()
  await page.getByRole('option').first().click()

  await page.getByTestId('prompt-submit').click()
  await expect(page).toHaveURL(/\/prompts\/\d+$/)

  const match = page.url().match(/\/prompts\/(\d+)$/)
  if (match?.[1] === undefined) {
    throw new Error(`发布后未跳转到详情页，当前地址 ${page.url()}`)
  }
  return Number.parseInt(match[1], 10)
}
