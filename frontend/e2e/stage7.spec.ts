// 阶段 7 的验收用例：隐私政策与条款、账号生命周期（导出与注销）、375px 窄屏适配。
//
// 375px 用例覆盖全部路由：在真实浏览器中以 375×667 访问每个路由，
// 断言没有任何元素的矩形越过视口边界。容器自身被撑宽但被外层裁剪时根元素
// 不一定溢出，因此逐元素比较边界而非只看 scrollWidth。

import { expect, test } from '@playwright/test'
import {
  ADMIN_EMAIL,
  ADMIN_PASSWORD,
  createPrompt,
  latestCodeFor,
  loginAs,
  loginShared,
  openAsShared,
  PASSWORD,
} from './helpers'

const MOBILE_VIEWPORT = { width: 375, height: 667 }

/** 游客可直接访问的路由。 */
const GUEST_ROUTES = [
  '/',
  '/login',
  '/register',
  '/reset-password',
  '/methodology',
  '/privacy',
  '/terms',
  '/no-such-page',
]

/** 需要登录的路由。 */
const AUTH_ROUTES = ['/me', '/favorites', '/libraries', '/notifications', '/upload']

/** 管理后台的全部模块路由。 */
const ADMIN_ROUTES = [
  '/admin/dashboard',
  '/admin/prompts',
  '/admin/models',
  '/admin/users',
  '/admin/tags',
  '/admin/reports',
  '/admin/invites',
  '/admin/logs',
  '/admin/announcements',
  '/admin/recycle',
  '/admin/settings',
]

/**
 * expectNoHorizontalOverflow 断言页面在窄屏下没有横向溢出。
 *
 * 判定到元素的最近裁剪祖先为止：带 overflow 隐藏或可滚动的祖先会自行处理
 * 内部宽内容（Element Plus 的页签条就是这么做的），这类元素不会把页面撑宽，
 * 因而不算溢出。宽高为零的隐藏元素同样不参与判定；允许 1px 的舍入误差。
 */
async function expectNoHorizontalOverflow(page: import('@playwright/test').Page): Promise<void> {
  const offenders = await page.evaluate(() => {
    const viewport = document.documentElement.clientWidth

    /** isClippedByAncestor 判断元素是否被某个祖先裁剪。 */
    const isClippedByAncestor = (element: Element): boolean => {
      let parent = element.parentElement
      while (parent !== null && parent !== document.documentElement) {
        const style = getComputedStyle(parent)
        const overflowX = style.overflowX
        if (overflowX !== 'visible') {
          return true
        }
        parent = parent.parentElement
      }
      return false
    }

    const out: string[] = []
    for (const element of Array.from(document.querySelectorAll('*'))) {
      const rect = element.getBoundingClientRect()
      if (rect.width === 0 || rect.height === 0) {
        continue
      }
      if (rect.right <= viewport + 1 && rect.left >= -1) {
        continue
      }
      if (isClippedByAncestor(element)) {
        continue
      }
      const tag = element.tagName.toLowerCase()
      const cls = typeof element.className === 'string' ? element.className.split(' ')[0] : ''
      out.push(`${tag}.${cls}`)
    }
    return out.slice(0, 5)
  })
  expect(offenders, `以下元素在 375px 下横向溢出：${offenders.join('、')}`).toEqual([])
}

/** uniqueEmail 给每次运行一个独立邮箱，避免用例之间互相影响。 */
function uniqueEmail(prefix: string): string {
  return `${prefix}-${Date.now()}-${Math.floor(Math.random() * 10000)}@example.com`
}

/**
 * registerAccount 通过真实注册与验证流程创建账户，返回注册后的昵称。
 * 昵称由后端从邮箱推导，注册响应带不回昵称，因此登录后从顶栏读取。
 */
async function registerAccount(
  page: import('@playwright/test').Page,
  email: string,
): Promise<string> {
  await page.goto('/register')
  await page.getByTestId('register-email').fill(email)
  await page.getByTestId('register-password').fill(PASSWORD)
  await page.getByTestId('register-confirm').fill(PASSWORD)
  await page.getByTestId('register-submit').click()

  await expect(page).toHaveURL(/\/verify-email/)
  await page.getByTestId('verify-code').fill(latestCodeFor(email))
  await page.getByTestId('verify-submit').click()

  await expect(page.getByTestId('header-user')).toBeVisible()
  return (await page.getByTestId('header-user').innerText()).trim()
}

test.describe('375px 窄屏适配', () => {
  test.use({ viewport: MOBILE_VIEWPORT })

  test('游客路由无横向溢出', async ({ page }) => {
    for (const path of GUEST_ROUTES) {
      await page.goto(path)
      await expectNoHorizontalOverflow(page)
    }
  })

  test('登录用户路由无横向溢出', async ({ page }) => {
    // 先登录一次，后续每个路由重新导航都会重建应用并重新探测会话。
    await loginShared(page)
    for (const path of AUTH_ROUTES) {
      await openAsShared(page, path)
      await expectNoHorizontalOverflow(page)
    }
  })

  test('提示词详情页无横向溢出', async ({ page }) => {
    // 详情页需要一个确实存在的提示词：公开列表在空库时是空列表。
    await loginShared(page)
    const promptID = await createPrompt(page, `窄屏详情-${Date.now()}`)
    await page.goto(`/prompts/${promptID}`)
    await expect(page.getByTestId('detail-title')).toBeVisible()
    await expectNoHorizontalOverflow(page)
  })

  test('后台全部模块无横向溢出', async ({ page }) => {
    await loginAs(page, ADMIN_EMAIL, ADMIN_PASSWORD)
    for (const path of ADMIN_ROUTES) {
      await page.goto(path)
      await expect(page.getByTestId('admin-nav')).toBeVisible()
      await expectNoHorizontalOverflow(page)
    }
  })
})

test.describe('隐私政策与用户条款', () => {
  test('页脚链接可进入两份文本', async ({ page }) => {
    await page.goto('/')

    await page.getByTestId('footer-privacy').click()
    await expect(page.getByTestId('legal-page')).toContainText('隐私政策')
    await expect(page.getByTestId('legal-page')).toContainText('你的权利')

    await page.getByTestId('legal-back').click()
    await expect(page).toHaveURL('/')

    await page.getByTestId('footer-terms').click()
    await expect(page.getByTestId('legal-page')).toContainText('用户条款')
    await expect(page.getByTestId('legal-page')).toContainText('行为规范')
  })

  test('英文界面下条款显示英文文案', async ({ page }) => {
    await page.goto('/terms')

    await page.locator('.el-select').first().click()
    await page.getByRole('option', { name: 'English' }).click()

    await expect(page.getByTestId('legal-page')).toContainText('Terms of service')
    await expect(page.getByTestId('legal-page')).toContainText('Conduct')
  })

  test('页脚入口可查看公开的计算方法', async ({ page }) => {
    await page.goto('/')
    await page.getByTestId('footer-methodology').click()

    await expect(page).toHaveURL(/\/methodology$/)
    await expect(page.getByTestId('methodology-page')).toContainText('计算方法')
    await expect(page.getByTestId('methodology-page')).toContainText('贝叶斯校准平均分')
    await expect(page.getByTestId('methodology-page')).toContainText('discrimination = clamp')
  })
})

test.describe('账号生命周期', () => {
  test('导出我的数据下载 JSON 文件', async ({ page }) => {
    await loginShared(page)
    await openAsShared(page, '/me')

    await page.getByRole('tab', { name: '我的数据' }).click()
    const downloadPromise = page.waitForEvent('download')
    await page.getByTestId('me-export').click()
    const download = await downloadPromise

    expect(download.suggestedFilename()).toMatch(/^prompt-benchmark-.*\.json$/)
    await expect(page.getByTestId('me-notice')).toContainText('导出文件已开始下载')
  })

  test('注销确认字符串不一致时被拦截', async ({ page }) => {
    const email = uniqueEmail('lifecycle')
    await registerAccount(page, email)

    // 注册验证完成后停在首页，先进入个人中心。
    await openAsShared(page, '/me')
    await page.getByRole('tab', { name: '注销账户' }).click()
    await page.getByTestId('me-delete-confirm-input').fill('错误的昵称')
    await page.getByTestId('me-delete-submit').click()

    // 页面内提示，且账户仍然可用：刷新后仍是登录态。
    await expect(page.locator('.el-alert--error')).toContainText('确认字符串与当前昵称不一致')
    await page.reload()
    await expect(page.getByTestId('header-user')).toBeVisible()
  })

  test('注销账户后会话失效且邮箱可重新注册', async ({ page, browser }) => {
    const email = uniqueEmail('delete')
    const username = await registerAccount(page, email)

    await openAsShared(page, '/me')
    await page.getByRole('tab', { name: '注销账户' }).click()
    await page.getByTestId('me-delete-confirm-input').fill(username)
    await page.getByTestId('me-delete-submit').click()
    await page.getByTestId('me-delete-confirm').click()

    // 注销后回到首页并处于游客态。
    await expect(page).toHaveURL('/')
    await expect(page.getByTestId('header-login')).toBeVisible()

    // 旧会话彻底失效：直接访问个人中心被引导到登录页。
    await page.goto('/me')
    await expect(page).toHaveURL(/\/login/)

    // 原邮箱可以重新注册（昵称由邮箱推导，因此重新注册后与之前相同）。
    const fresh = await browser.newPage()
    const reused = await registerAccount(fresh, email)
    expect(reused).toBe(username)
    await fresh.close()
  })
})
