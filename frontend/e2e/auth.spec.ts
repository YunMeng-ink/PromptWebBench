import { readdirSync, readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, type Page, test } from '@playwright/test'

const here = dirname(fileURLToPath(import.meta.url))
const MAIL_DROP = resolve(here, '..', '..', 'backend', '.maildrop')

const PASSWORD = 'E2ePassw0rd!'

function uniqueEmail(prefix: string): string {
  return `${prefix}-${Date.now()}-${Math.floor(Math.random() * 10000)}@example.com`
}

/** latestCodeFor 从 SMTP 接收器落盘的邮件中取出指定收件人的验证码。 */
function latestCodeFor(email: string): string {
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

async function fillLoginAndSubmit(page: Page, email: string, password: string): Promise<void> {
  await page.goto('/login')
  await page.getByTestId('login-email').fill(email)
  await page.getByTestId('login-password').fill(password)
  await page.getByTestId('login-submit').click()
}

/** registerThroughUI 完成注册与邮箱验证，结束时处于登录态。 */
async function registerThroughUI(page: Page, email: string): Promise<void> {
  await page.goto('/register')
  await page.getByTestId('register-email').fill(email)
  await page.getByTestId('register-password').fill(PASSWORD)
  await page.getByTestId('register-confirm').fill(PASSWORD)
  await page.getByTestId('register-submit').click()

  await expect(page).toHaveURL(/\/verify-email/)
  await page.getByTestId('verify-code').fill(latestCodeFor(email))
  await page.getByTestId('verify-submit').click()
  await expect(page.getByTestId('header-user')).toBeVisible()
}

// 多个用例共用同一个已验证账户，避免触发认证类接口的限流。
let sharedEmail: string | null = null

async function loginSharedAccount(page: Page): Promise<string> {
  if (sharedEmail === null) {
    sharedEmail = uniqueEmail('shared')
    await registerThroughUI(page, sharedEmail)
    return sharedEmail
  }
  await fillLoginAndSubmit(page, sharedEmail, PASSWORD)
  await expect(page.getByTestId('header-user')).toBeVisible()
  return sharedEmail
}

test('游客访问个人中心会被引导到登录页', async ({ page }) => {
  await page.goto('/me')

  await expect(page).toHaveURL(/\/login\?redirect=/)
  await expect(page.getByTestId('login-submit')).toBeVisible()
})

test('注册与邮箱验证完成后直接处于登录态', async ({ page }) => {
  const email = uniqueEmail('signup')

  await page.goto('/register')
  await page.getByTestId('register-email').fill(email)
  await page.getByTestId('register-password').fill(PASSWORD)
  await page.getByTestId('register-confirm').fill(PASSWORD)
  await page.getByTestId('register-submit').click()

  await expect(page).toHaveURL(/\/verify-email/)
  await expect(page.getByText(email)).toBeVisible()

  await page.getByTestId('verify-code').fill(latestCodeFor(email))
  await page.getByTestId('verify-submit').click()

  await expect(page).toHaveURL('/')
  await expect(page.getByTestId('header-user')).toBeVisible()
})

test('登出后恢复游客态，可再次登录', async ({ page }) => {
  const email = await loginSharedAccount(page)

  await page.getByTestId('header-user').click()
  await page.getByTestId('header-logout').click()
  await expect(page.getByTestId('header-login')).toBeVisible()

  await page.goto('/me')
  await expect(page).toHaveURL(/\/login/)

  await fillLoginAndSubmit(page, email, PASSWORD)
  await expect(page.getByTestId('header-user')).toBeVisible()

  await page.goto('/me')
  await expect(page.getByTestId('me-username')).toBeVisible()
})

test('口令错误时给出提示且不进入登录态', async ({ page }) => {
  await loginSharedAccount(page)
  await page.getByTestId('header-user').click()
  await page.getByTestId('header-logout').click()
  await expect(page.getByTestId('header-login')).toBeVisible()

  await fillLoginAndSubmit(page, sharedEmail ?? '', 'WrongPassw0rd!')

  await expect(page.getByText('邮箱或密码错误')).toBeVisible()
  await expect(page.getByTestId('header-login')).toBeVisible()
})

test('个人中心可以修改资料', async ({ page }) => {
  await loginSharedAccount(page)

  await page.goto('/me')
  await page.getByTestId('me-username-input').fill('端到端昵称')
  await page.getByTestId('me-profile-form').getByTestId('me-save').click()

  await expect(page.getByTestId('me-notice')).toContainText('资料已更新')
  await expect(page.getByTestId('header-user')).toContainText('端到端昵称')
})

test('API Token 只展示一次并可撤销', async ({ page }) => {
  await loginSharedAccount(page)

  await page.goto('/me')
  await page.getByRole('tab', { name: 'API Token' }).click()
  await expect(page.getByTestId('me-token-absent')).toBeVisible()

  await page.getByTestId('me-token-create').click()
  const plain = page.getByTestId('me-token-plain')
  await expect(plain).toBeVisible()
  const token = await plain.inputValue()
  expect(token.startsWith('pb_')).toBe(true)

  await page.getByTestId('me-token-revoke').click()
  await expect(page.getByTestId('me-token-absent')).toBeVisible()
  await expect(page.getByTestId('me-token-plain')).toHaveCount(0)
})

test('修改口令后当前会话保持可用', async ({ page }) => {
  await loginSharedAccount(page)

  await page.goto('/me')
  await page.getByRole('tab', { name: '口令' }).click()
  await page.getByTestId('me-old-password').fill(PASSWORD)
  await page.getByTestId('me-new-password').fill('ChangedE2e1!')
  await page.getByTestId('me-confirm-password').fill('ChangedE2e1!')
  await page.getByTestId('me-password-submit').click()

  await expect(page.getByTestId('me-notice')).toContainText('口令已修改')

  await page.goto('/me')
  await expect(page.getByTestId('header-user')).toBeVisible()
  await expect(page.getByTestId('me-username')).toBeVisible()
})
