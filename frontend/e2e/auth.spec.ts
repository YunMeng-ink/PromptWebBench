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

  // 等表单渲染完成再填值：页面刚导航时组件尚未挂载，直接 fill 会落空。
  const emailInput = page.getByTestId('login-email')
  await expect(emailInput).toBeVisible()
  await emailInput.fill(email)
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

// 认证用例各自使用独立账户：本文件最后一个用例会修改口令，
// 若与提示词、评分用例共用同一账户，那两个套件之后的登录就会失败。
let authEmail: string | null = null

async function loginAuthAccount(page: Page): Promise<string> {
  if (authEmail === null) {
    authEmail = uniqueEmail('auth')
    await registerThroughUI(page, authEmail)
    return authEmail
  }

  await fillLoginAndSubmit(page, authEmail, PASSWORD)

  // 登录失败时页面上会给出错误提示，先断言登录成功再继续，
  // 否则后续断言会指向一个与登录无关的页面，报错难以定位。
  await expect(page.getByTestId('header-user')).toBeVisible()
  return authEmail
}

test('游客访问个人中心会被引导到登录页', async ({ page }) => {
  await page.goto('/me')

  await expect(page).toHaveURL(/\/login\?redirect=/)
  await expect(page.getByTestId('login-submit')).toBeVisible()
})

test('登录、注册、验证邮箱与重置口令卡片保持同一高度', async ({ page }) => {
  await page.goto('/login')

  for (const locale of ['zh', 'en']) {
    await page.evaluate((value) => localStorage.setItem('pb.locale', value), locale)

    for (const viewport of [
      { width: 1280, height: 900 },
      { width: 375, height: 667 },
    ]) {
      await page.setViewportSize(viewport)
      const heights: number[] = []
      for (const path of ['/login', '/register', '/reset-password', '/verify-email']) {
        await page.goto(path)
        const card = page.locator('.pb-auth__card')
        await expect(card).toBeVisible()
        heights.push(
          await card.evaluate((element) => Math.round(element.getBoundingClientRect().height)),
        )
      }
      expect(
        new Set(heights).size,
        `${locale} / ${viewport.width}px 下卡片高度不一致：${heights.join('、')}`,
      ).toBe(1)
    }
  }
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
  const email = await loginAuthAccount(page)

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
  await loginAuthAccount(page)
  await page.getByTestId('header-user').click()
  await page.getByTestId('header-logout').click()
  await expect(page.getByTestId('header-login')).toBeVisible()

  await fillLoginAndSubmit(page, authEmail ?? '', 'WrongPassw0rd!')

  await expect(page.getByText('邮箱或密码错误')).toBeVisible()
  await expect(page.getByTestId('header-login')).toBeVisible()
})

test('个人中心可以修改资料', async ({ page }) => {
  await loginAuthAccount(page)

  // 昵称在库上是唯一键，固定取值会让第二次运行的用例撞上已存在的记录。
  const nickname = `端到端昵称-${Date.now()}`

  await page.goto('/me')
  await page.getByTestId('me-username-input').fill(nickname)
  await page.getByTestId('me-profile-form').getByTestId('me-save').click()

  await expect(page.getByTestId('me-notice')).toContainText('资料已更新')
  await expect(page.getByTestId('header-user')).toContainText(nickname)
})

test('API Token 只展示一次并可撤销', async ({ page }) => {
  await loginAuthAccount(page)

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
  await loginAuthAccount(page)

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
