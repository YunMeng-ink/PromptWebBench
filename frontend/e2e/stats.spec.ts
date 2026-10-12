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
    throw new Error('模型字典为空，统计用例无法执行')
  }
  return id
}

test('游客在详情页可以看到测量结果区', async ({ browser }) => {
  const authorContext = await browser.newContext({ locale: 'zh-CN', baseURL: SHARED_BASE_URL })
  const authorPage = await authorContext.newPage()
  await loginShared(authorPage)
  const promptID = await createPrompt(authorPage, `游客测量-${Date.now()}`)
  await authorContext.close()

  const context = await browser.newContext({ locale: 'zh-CN', baseURL: SHARED_BASE_URL })
  const page = await context.newPage()
  await page.goto(`/prompts/${promptID}`)

  await expect(page.getByTestId('stats-section')).toBeVisible()
  // 没有任何评分，区分度按 C-06 显示原因文案而不是 0。
  await expect(page.getByTestId('stats-history-discrimination')).toHaveText(
    '数据不足，暂不计算区分度',
  )
  await expect(page.getByTestId('stats-recent-discrimination')).toHaveText(
    '数据不足，暂不计算区分度',
  )

  await context.close()
})

test('提交评分后该模型的完成度出现在测量结果里', async ({ page }) => {
  await loginShared(page)
  const promptID = await createPrompt(page, `测量更新-${Date.now()}`)
  const modelID = await firstModelID(page)

  await page.goto(`/prompts/${promptID}`)

  // 评分前该模型没有任何观测值，平均完成度显示占位符而不是 0.00。
  const rowBefore = page.getByTestId(`stats-model-${modelID}`).locator('xpath=ancestor::tr')
  await expect(rowBefore.locator('td').nth(1)).toHaveText('—')
  await expect(rowBefore.locator('td').nth(3)).toHaveText('0')
  await expect(rowBefore.locator('td').nth(7)).toHaveText('尚无评分')

  await page.getByTestId(`rating-start-${modelID}`).click()
  await page.locator('[data-testid="rating-score-5"]').click()
  await page.getByTestId('rating-confirm').click()
  await expect(page.getByTestId('detail-notice')).toContainText('评分已提交')

  // 评分后测算结果随之刷新：1 条 5 分评分的原始平均完成度为 5.00。
  const rowAfter = page.getByTestId(`stats-model-${modelID}`).locator('xpath=ancestor::tr')
  await expect(rowAfter.locator('td').nth(1)).toHaveText('5.00')
  await expect(rowAfter.locator('td').nth(3)).toHaveText('1')
  await expect(page.getByTestId('stats-total-ratings')).toHaveText('1')
})

test('单条评分远不足以计算区分度，仍显示数据不足', async ({ page }) => {
  await loginShared(page)
  const promptID = await createPrompt(page, `测量门槛-${Date.now()}`)
  const modelID = await firstModelID(page)

  await page.goto(`/prompts/${promptID}`)
  await page.getByTestId(`rating-start-${modelID}`).click()
  await page.locator('[data-testid="rating-score-4"]').click()
  await page.getByTestId('rating-confirm').click()
  await expect(page.getByTestId('detail-notice')).toContainText('评分已提交')

  // 需求 §4.3 要求至少 3 个有效模型、每模型 3 条、窗口内 10 条；
  // 1 个模型 1 条评分远未达到，区分度一律不计算。
  await expect(page.getByTestId('stats-recent-discrimination')).toHaveText(
    '数据不足，暂不计算区分度',
  )
  await expect(page.getByTestId('stats-valid-models')).toHaveText('0')
})
