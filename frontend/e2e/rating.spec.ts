import { expect, test } from '@playwright/test'
import { createPrompt, loginShared, SHARED_BASE_URL } from './helpers'

/**
 * firstModelID 取出字典中的第一个可用模型。
 * 字典为空时抛出明确错误，而不是让后续断言在无意义的选择器上超时。
 */
async function firstModelID(page: import('@playwright/test').Page): Promise<number> {
  const id = await page.evaluate(async () => {
    const response = await fetch('/api/v1/models')
    const body = (await response.json()) as { items: { id: number }[] }
    return body.items[0]?.id ?? 0
  })
  if (id === 0) {
    throw new Error('模型字典为空，评分用例无法执行')
  }
  return id
}

test('游客在详情页看不到评分入口', async ({ browser }) => {
  // 先用已登录的上下文发布一条提示词，游客用例才有可访问的对象。
  const authorContext = await browser.newContext({ locale: 'zh-CN', baseURL: SHARED_BASE_URL })
  const authorPage = await authorContext.newPage()
  await loginShared(authorPage)
  const promptID = await createPrompt(authorPage, `游客评分-${Date.now()}`)
  await authorContext.close()

  const context = await browser.newContext({ locale: 'zh-CN', baseURL: SHARED_BASE_URL })
  const page = await context.newPage()

  // 评分入口只对登录身份出现。未登录的上下文不继承共用账户的会话。
  await page.goto(`/prompts/${promptID}`)
  await expect(page.getByTestId('detail-title')).toBeVisible()
  await expect(page.getByTestId('header-login')).toBeVisible()
  await expect(page.locator('[data-testid^="rating-start-"]')).toHaveCount(0)

  await context.close()
})

test('评分提交后展示我的评分与剩余修改次数', async ({ page }) => {
  await loginShared(page)
  const promptID = await createPrompt(page, `评分提交-${Date.now()}`)
  const modelID = await firstModelID(page)

  await page.goto(`/prompts/${promptID}`)
  await page.getByTestId(`rating-start-${modelID}`).click()
  await page.locator('[data-testid="rating-score-4"]').click()
  await page.getByTestId('rating-confirm').click()

  await expect(page.getByTestId('detail-notice')).toContainText('评分已提交')
  await expect(page.getByTestId(`rating-mine-${modelID}`)).toHaveText('4')
  await expect(page.getByTestId(`rating-edits-${modelID}`)).toContainText('还可修改 3 次')
})

/** readSelectionSummary 读取「显示 N / M 个模型」中的两个数字。 */
async function readSelectionSummary(
  page: import('@playwright/test').Page,
): Promise<{ selected: number; total: number }> {
  const text = await page.getByTestId('ratings-selection-summary').innerText()
  const match = text.match(/显示 (\d+) \/ (\d+) 个模型/)
  if (match?.[1] === undefined || match[2] === undefined) {
    throw new Error(`无法解析模型选择摘要：${text}`)
  }
  return { selected: Number.parseInt(match[1], 10), total: Number.parseInt(match[2], 10) }
}

test('模型选择器可以收窄与恢复展示范围', async ({ page }) => {
  await loginShared(page)
  const promptID = await createPrompt(page, `模型选择-${Date.now()}`)

  await page.goto(`/prompts/${promptID}`)
  await expect(page.getByTestId('ratings-model-select')).toBeVisible()

  // 默认集合是「有评分的模型 + 按顺序补足到 8 个」，因此不超过 min(8, 总数)。
  const initial = await readSelectionSummary(page)
  expect(initial.total).toBeGreaterThan(0)
  expect(initial.selected).toBe(Math.min(8, initial.total))
  await expect(page.getByTestId('ratings-list').locator('li')).toHaveCount(initial.selected)

  await page.getByTestId('ratings-quick-all').click()
  await expect(page.getByTestId('ratings-list').locator('li')).toHaveCount(initial.total)

  // 新建的提示词没有任何评分，「仅有评分」因此是空集合。
  await page.getByTestId('ratings-quick-rated').click()
  await expect(page.getByTestId('ratings-selection-empty')).toBeVisible()
  await expect(page.getByTestId('ratings-list')).toHaveCount(0)

  await page.getByTestId('ratings-quick-reset').click()
  await expect(page.getByTestId('ratings-list').locator('li')).toHaveCount(initial.selected)
})

test('量表只有 1-5 五档', async ({ page }) => {
  await loginShared(page)
  const promptID = await createPrompt(page, `量表校验-${Date.now()}`)
  const modelID = await firstModelID(page)

  await page.goto(`/prompts/${promptID}`)
  await page.getByTestId(`rating-start-${modelID}`).click()

  await expect(page.getByTestId('rating-score-1')).toBeVisible()
  await expect(page.getByTestId('rating-score-5')).toBeVisible()
  await expect(page.getByTestId('rating-score-6')).toHaveCount(0)
})
