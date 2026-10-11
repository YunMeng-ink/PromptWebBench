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
