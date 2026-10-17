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

/**
 * ADMIN_EMAIL 与 ADMIN_PASSWORD 是后端启动时按 ADMIN_INIT_* 建立的初始管理员。
 * 它的身份不由注册流程产生，因此走独立的口令常量。
 */
export const ADMIN_EMAIL = 'e2e-admin@prompt-benchmark.local'
export const ADMIN_PASSWORD = 'E2eAdminPassw0rd!'

/** SHARED_EMAIL 是整个套件共用的账户，各规格文件都用它。 */
export const SHARED_EMAIL = 'e2e-shared@example.com'

/**
 * REPORTER_EMAIL 是第二个账户。
 *
 * 举报与「查看他人公开收藏夹」要求操作者不是内容所有者：作者看不到举报自己的
 * 入口，公开收藏夹也需要一个非所有者的观察者。两个账户都在全局前置里创建。
 */
export const REPORTER_EMAIL = 'e2e-reporter@example.com'

/**
 * DEVELOPER_EMAIL 是第三个账户，身份组为开发者。
 *
 * 「开发者可见提示词」的用例需要这个身份：普通用户看不到这些内容，
 * 因此无法构造「普通用户协作者看不到开发者专属引用题」的场景。
 */
export const DEVELOPER_EMAIL = 'e2e-developer@example.com'

/**
 * loginAs 用给定账户登录，并确认顶栏显示的就是该账户。
 *
 * 只等「顶栏出现用户」是不够的：在已登录状态下重新登录时，顶栏本来就显示着上一个
 * 账户，等待会立刻通过，随后的导航会把尚未完成的登录请求取消掉，用例就在
 * 上一个身份下继续执行。因此这里按昵称比对，等的是登录真正完成。
 */
export async function loginAs(
  page: Page,
  email: string,
  password: string = PASSWORD,
): Promise<void> {
  await page.goto('/login')

  // 等表单渲染完成再填值：页面刚导航时组件尚未挂载，直接 fill 会落空。
  const emailInput = page.getByTestId('login-email')
  await expect(emailInput).toBeVisible()
  await emailInput.fill(email)

  await page.getByTestId('login-password').fill(password)
  await page.getByTestId('login-submit').click()

  const hero = page.getByTestId('header-user')
  await expect(hero).toBeVisible()
  const expected = knownUsername(email)
  if (expected !== null) {
    await expect(hero).toHaveText(expected)
  }
}

/** knownUsername 返回全局前置记录的该邮箱昵称；未记录时返回 null。 */
function knownUsername(email: string): string | null {
  try {
    return accountUsernames()[email] ?? null
  } catch {
    return null
  }
}

/** loginReporter 用第二账户登录，供举报与旁观他人收藏夹的用例使用。 */
export async function loginReporter(page: Page): Promise<void> {
  await loginAs(page, REPORTER_EMAIL)
}

/** loginDeveloper 用开发者账户登录，供开发者可见内容的用例使用。 */
export async function loginDeveloper(page: Page): Promise<void> {
  await loginAs(page, DEVELOPER_EMAIL)
}

/** loginAdmin 用初始管理员登录，供管理后台用例使用。 */
export async function loginAdmin(page: Page): Promise<void> {
  await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD)
}

/** openAdmin 以管理员身份打开后台的某个模块。 */
export async function openAdmin(page: Page, path: string): Promise<void> {
  await loginAdmin(page)
  await page.goto(path)
  await expect(page.getByTestId('admin-nav')).toBeVisible()
}

/**
 * accountUsernames 读取全局前置写出的账户昵称。
 *
 * 昵称由注册流程从邮箱推导，用例邀请协作者时需要填写它，登录后也要据此确认
 * 身份确实是目标账户。让前置把结果写出来，规格文件就不必为了读昵称而多登录一次。
 */
export function accountUsernames(): Record<string, string> {
  const raw = readFileSync(resolve(here, '..', '.e2e-accounts.json'), 'utf8')
  return JSON.parse(raw) as Record<string, string>
}

/** collaboratorUsername 返回第二账户的昵称，供邀请协作者的用例填写。 */
export function collaboratorUsername(): string {
  const name = accountUsernames()[REPORTER_EMAIL]
  if (name === undefined) {
    throw new Error(`未找到 ${REPORTER_EMAIL} 的昵称，全局前置未写出账户信息`)
  }
  return name
}

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
  await loginAs(page, SHARED_EMAIL)
  return SHARED_EMAIL
}

/**
 * openSharedPrompt 用共用账户发布一条提示词，再用第二账户打开它。
 *
 * 举报与旁观收藏夹都需要「不是作者」的身份，而发布提示词只应由共用账户做，
 * 使提示词的所有权稳定。
 */
export async function openAsReporter(page: Page, ownerPage: Page, title: string): Promise<number> {
  await loginShared(ownerPage)
  const promptID = await createPrompt(ownerPage, title)
  await loginReporter(page)
  await page.goto(`/prompts/${promptID}`)
  await expect(page.getByTestId('detail-title')).toBeVisible()
  return promptID
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

  await page.getByTestId('prompt-purpose').locator('.el-select__wrapper').click()
  await page.getByRole('option', { name: '边界探测', exact: true }).click()
  await page.getByTestId('prompt-capability').locator('.el-select__wrapper').click()
  await page.getByRole('option', { name: '推理', exact: true }).click()

  await page.getByTestId('prompt-submit').click()
  await expect(page).toHaveURL(/\/prompts\/\d+$/)

  const match = page.url().match(/\/prompts\/(\d+)$/)
  if (match?.[1] === undefined) {
    throw new Error(`发布后未跳转到详情页，当前地址 ${page.url()}`)
  }
  return Number.parseInt(match[1], 10)
}

/**
 * createPromptWithVisibility 以给定可见性发布一条提示词。
 *
 * 可见性选择器只对开发者及以上身份出现（需求 §8.4），因此调用方必须已经用
 * 开发者账户登录；普通身份下该选择器不存在，这里会直接失败而不是静默降级。
 */
export async function createPromptWithVisibility(
  page: Page,
  title: string,
  visibility: 'public' | 'private' | 'developer',
): Promise<number> {
  await openAsShared(page, '/upload')

  const titleInput = page.getByTestId('prompt-title')
  await expect(titleInput).toBeVisible()
  await titleInput.fill(title)
  await page.getByTestId('prompt-body').fill('开发者可见的提示词正文。')

  await page.getByTestId('prompt-purpose').locator('.el-select__wrapper').click()
  await page.getByRole('option', { name: '边界探测', exact: true }).click()
  await page.getByTestId('prompt-capability').locator('.el-select__wrapper').click()
  await page.getByRole('option', { name: '推理', exact: true }).click()

  await page.getByTestId('prompt-visibility').locator('.el-select__wrapper').click()
  await page.getByRole('option', { name: visibilityLabel(visibility) }).click()

  await page.getByTestId('prompt-submit').click()
  await expect(page).toHaveURL(/\/prompts\/\d+$/)

  const match = page.url().match(/\/prompts\/(\d+)$/)
  if (match?.[1] === undefined) {
    throw new Error(`发布后未跳转到详情页，当前地址 ${page.url()}`)
  }
  return Number.parseInt(match[1], 10)
}

/** visibilityLabel 返回可见性选项在中文界面下的文案。 */
function visibilityLabel(visibility: 'public' | 'private' | 'developer'): string {
  switch (visibility) {
    case 'public':
      return '公开'
    case 'private':
      return '私有'
    default:
      return '开发者可见'
  }
}
