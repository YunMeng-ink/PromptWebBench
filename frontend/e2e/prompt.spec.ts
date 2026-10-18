import { expect, test } from '@playwright/test'
import { createPrompt, loginAdmin, loginShared, openAsShared, SHARED_BASE_URL } from './helpers'

test('游客可浏览公开列表且看不到上传入口', async ({ page }) => {
  await page.goto('/')

  await expect(page.getByTestId('home-filters')).toBeVisible()
  await expect(page.getByTestId('header-upload')).toHaveCount(0)

  // 登录态在应用启动后异步探测，等它落到游客态再断言登录入口。
  await expect(page.getByTestId('header-login')).toBeVisible()
})

test('发布提示词后出现在公开列表并可进入详情', async ({ page }) => {
  await loginShared(page)
  const title = `端到端提示词-${Date.now()}`
  const promptID = await createPrompt(page, title)

  await expect(page.getByTestId('detail-title')).toContainText(title)
  await expect(page.getByTestId('detail-version')).toHaveText('v1.0')

  await page.goto('/')
  await expect(page.getByTestId(`prompt-link-${promptID}`)).toContainText(title)
})

test('首页按关键词筛选命中目标提示词', async ({ page }) => {
  await loginShared(page)
  const title = `筛选验证-${Date.now()}`
  await createPrompt(page, title)

  await page.goto('/')
  // el-input 把 data-testid 放在外层容器上，实际可输入的是它内部的 input。
  await page.locator('input[data-testid="filter-keyword"]').fill(title)
  await page.locator('input[data-testid="filter-keyword"]').press('Enter')

  await expect(page.getByTestId('home-list')).toContainText(title)
  await expect(page.getByTestId('home-list').getByRole('link')).toHaveCount(1)
})

test('三种排序方式均可选择并触发查询', async ({ page }) => {
  await loginShared(page)

  await page.goto('/')
  const sort = page.getByTestId('filter-sort')
  for (const label of ['最新更新', '评分最高', '区分度最高']) {
    await sort.locator('.el-select__wrapper').click()
    await page.getByRole('option', { name: label }).click()
    await expect(sort).toContainText(label)
  }

  // 下拉层在选中后仍保持挂载，会盖住后续用例的点击，这里显式收起。
  await page.keyboard.press('Escape')
})

test('上传页展示提示词写作指南', async ({ page }) => {
  await loginShared(page)
  await openAsShared(page, '/upload')

  await expect(page.getByTestId('prompt-guide-banner')).toBeVisible()
  await page.getByTestId('prompt-guide-open').click()
  await expect(page.getByRole('dialog')).toContainText('提示词写作指南')
  await expect(page.getByTestId('prompt-guide-dialog')).toContainText('参考来源')
})

test('考察维度支持多选且最多 5 个', async ({ page }) => {
  await loginShared(page)
  await openAsShared(page, '/upload')

  await page.getByTestId('prompt-capability').locator('.el-select__wrapper').click()
  for (const name of ['推理', '知识', '数学', '语言', '创造性']) {
    await page.getByRole('option', { name, exact: true }).click()
  }

  await expect(page.getByTestId('prompt-capability-count')).toContainText('已选 5 / 5 个')
  await expect(page.getByRole('option', { name: '代码', exact: true })).toHaveClass(/is-disabled/)
})

test('多选的考察维度在详情页完整展示', async ({ page }) => {
  await loginShared(page)
  await openAsShared(page, '/upload')

  await page.getByTestId('prompt-title').fill(`多维度提示词 ${Date.now()}`)
  await page.getByTestId('prompt-body').fill('同时考察推理、代码与工具调用。')
  await page.getByTestId('prompt-purpose').locator('.el-select__wrapper').click()
  await page.getByRole('option', { name: '边界探测', exact: true }).click()

  await page.getByTestId('prompt-capability').locator('.el-select__wrapper').click()
  for (const name of ['推理', '代码', '工具调用']) {
    await page.getByRole('option', { name, exact: true }).click()
  }
  await page.keyboard.press('Escape')

  await page.getByTestId('prompt-submit').click()
  await expect(page).toHaveURL(/\/prompts\/\d+$/)

  const capability = page.getByTestId('detail-capability')
  await expect(capability).toContainText('推理')
  await expect(capability).toContainText('代码')
  await expect(capability).toContainText('工具调用')
})

test('普通用户的可见性选项不含开发者可见', async ({ page }) => {
  await loginShared(page)

  await openAsShared(page, '/upload')
  await page.getByTestId('prompt-visibility').locator('.el-select__wrapper').click()
  await expect(page.getByRole('option', { name: '公开' })).toBeVisible()
  await expect(page.getByRole('option', { name: '私有' })).toBeVisible()
  await expect(page.getByRole('option', { name: '开发者可见' })).toHaveCount(0)
})

test('普通用户的考察目的列表不含高风险条目', async ({ page }) => {
  await loginShared(page)

  await openAsShared(page, '/upload')
  await page.getByTestId('prompt-purpose').locator('.el-select__wrapper').click()
  await expect(page.getByRole('option', { name: '对抗探测' })).toHaveCount(0)
})

test('普通修改生成次版本并保留历史版本', async ({ page }) => {
  await loginShared(page)
  const title = `普通修改-${Date.now()}`
  const promptID = await createPrompt(page, title)

  await page.getByTestId('detail-edit').click()
  await expect(page).toHaveURL(new RegExp(`/prompts/${promptID}/edit$`))

  // 编辑表单在详情返回前就会渲染，先等原标题回填再输入，避免异步回填覆盖输入。
  const titleInput = page.getByTestId('prompt-title')
  await expect(titleInput).toHaveValue(title)
  await titleInput.fill(`${title}-已修改`)
  await page.getByTestId('prompt-change-note').fill('调整措辞')
  await page.getByTestId('prompt-submit').click()

  await expect(page.getByTestId('detail-version')).toHaveText('v1.1')
  await expect(page.getByTestId('detail-title')).toContainText(`${title}-已修改`)
})

test('重大修改进到主版本并展示提示文案', async ({ page }) => {
  await loginShared(page)
  const title = `重大修改-${Date.now()}`
  const promptID = await createPrompt(page, title)

  await page.goto(`/prompts/${promptID}/edit`)

  // 表单在详情返回前就渲染了，此时输入框可见但仍是占位取值。
  // 等原标题回填后再填写，避免异步回填把输入与重大修改标记一起覆盖。
  const titleInput = page.getByTestId('prompt-title')
  await expect(titleInput).toHaveValue(title)
  await titleInput.fill(`${title}-重大`)

  const major = page.locator('[data-testid="prompt-major"]')
  await expect(major).toBeVisible()
  await major.click()
  // 勾选后提示文案出现，确认标记已被表单接收，再提交。
  await expect(page.getByText('重大修改会生成新版本')).toBeVisible()
  await page.getByTestId('prompt-submit').click()

  await expect(page.getByTestId('detail-version')).toHaveText('v2.0')
  await expect(page.getByTestId('detail-major-notice')).toContainText('已发生重大修改')
})

test('私有提示词对游客展示未找到', async ({ browser }) => {
  // 先用已登录的上下文建一条私有提示词，再换一个未登录的上下文访问。
  const authorContext = await browser.newContext({ locale: 'zh-CN', baseURL: SHARED_BASE_URL })
  const authorPage = await authorContext.newPage()
  await loginShared(authorPage)

  const created = await authorPage.evaluate(async () => {
    // evaluate 的函数体会被序列化到页面执行，外部函数的引用不会带过去，
    // 因此取 Cookie 的逻辑直接写在页面脚本内部。
    const csrf = document.cookie
      .split(';')
      .map((part) => part.trim())
      .find((part) => part.startsWith('pb_csrf='))
      ?.slice('pb_csrf='.length)

    const purposes = await (await fetch('/api/v1/purposes')).json()
    const capabilities = await (await fetch('/api/v1/capabilities')).json()
    const response = await fetch('/api/v1/prompts', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': csrf ?? '',
      },
      body: JSON.stringify({
        title: `私有提示词-${Date.now()}`,
        body: '正文',
        purpose_id: purposes.items[0].id,
        capability_id: capabilities.items[0].id,
        visibility: 'private',
      }),
    })
    return { status: response.status, body: await response.json() }
  })
  // 创建成功返回 200 与提示词体，不依赖状态码的具体取值。
  expect(created.status).toBe(200)
  await authorContext.close()

  // 独立的游客上下文，不影响共用账户后续用例的登录态。
  const guestContext = await browser.newContext({ locale: 'zh-CN', baseURL: SHARED_BASE_URL })
  const guestPage = await guestContext.newPage()
  await guestPage.goto(`/prompts/${created.body.prompt.id}`)

  await expect(guestPage.getByTestId('detail-error')).toBeVisible()
  await guestContext.close()
})

test('后台新建的标签立即出现在提示词表单', async ({ page }) => {
  await loginAdmin(page)
  // 先进入首页，让字典在会话内完成一次缓存。
  await page.goto('/')
  await expect(page.getByTestId('home-filters')).toBeVisible()

  const suffix = Date.now()
  const tag = {
    slug: `e2e-fresh-tag-${suffix}`,
    name_zh: `新标签${suffix}`,
    name_en: `Fresh Tag ${suffix}`,
  }
  await page.evaluate(async (payload) => {
    const csrf =
      document.cookie
        .split('; ')
        .find((part) => part.startsWith('pb_csrf='))
        ?.split('=')[1] ?? ''
    const response = await fetch('/api/v1/admin/tags', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': decodeURIComponent(csrf),
      },
      credentials: 'include',
      body: JSON.stringify({
        ...payload,
        description: '',
        color: '#336699',
        kind: 'theme',
        priority: 0,
        is_enabled: true,
      }),
    })
    if (!response.ok) {
      throw new Error(await response.text())
    }
  }, tag)

  // 站内跳转不会重建 Pinia，表单必须主动刷新标签才能看到刚创建的数据。
  await page.getByTestId('header-upload').click()
  await expect(page.getByTestId('prompt-title')).toBeVisible()
  await page.getByTestId('prompt-tags').locator('.el-select__wrapper').click()
  await expect(page.getByRole('option', { name: tag.name_zh, exact: true })).toBeVisible()
})
