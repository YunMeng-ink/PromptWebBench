import { type BrowserContext, expect, type Page, test } from '@playwright/test'
import { BASE_URL, loginAdmin, loginShared } from './helpers'

/** firstModelID 取出字典中的第一个可用模型。 */
async function firstModelID(page: Page): Promise<number> {
  const id = await page.evaluate(async () => {
    const response = await fetch('/api/v1/models')
    const body = (await response.json()) as { items: { id: number }[] }
    return body.items[0]?.id ?? 0
  })
  if (id === 0) {
    throw new Error('模型字典为空，展示区用例无法执行')
  }
  return id
}

/** PNG_1X1 是一张 1×1 的 PNG，用于真实走通 multipart 上传路径。 */
const PNG_1X1 = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
  'base64',
)

/** submitResult 在提示词详情页提交带输出文本与图片的评分。 */
async function submitResult(
  page: Page,
  promptID: number,
  modelID: number,
  output: string,
): Promise<void> {
  await page.goto(`/prompts/${promptID}`)
  await page.getByTestId(`rating-start-${modelID}`).click()
  await page.locator('[data-testid="rating-score-4"]').click()
  await page
    .locator('textarea[data-testid="rating-output"], [data-testid="rating-output"] textarea')
    .fill(output)
  await page.getByTestId('rating-image-input').setInputFiles({
    name: 'result.png',
    mimeType: 'image/png',
    buffer: PNG_1X1,
  })
  await expect(page.getByTestId('rating-image-list')).toContainText('result.png')
  await page.getByTestId('rating-confirm').click()
  await expect(page.getByTestId('detail-notice')).toContainText('评分已提交')
  await expect(page.getByTestId('detail-notice')).toContainText('图片已上传')
}

/** openModelEntries 打开展示区并选中指定模型。 */
async function openModelEntries(page: Page, modelID: number): Promise<void> {
  await page.goto('/showcase')
  await expect(page.getByTestId('showcase-page')).toBeVisible()
  await page.getByTestId(`showcase-model-${modelID}`).click()
  await expect(page.getByTestId('showcase-current-model')).toBeVisible()
}

test('评分时提交的输出与图片进入模型展示区', async ({ page }) => {
  await loginShared(page)
  const modelID = await firstModelID(page)
  const title = `展示区结果-${Date.now()}`
  const output = `端到端模型输出-${Date.now()}`

  await page.goto('/upload')
  await expect(page.getByTestId('prompt-title')).toBeVisible()
  await page.getByTestId('prompt-title').fill(title)
  await page.getByTestId('prompt-body').fill('展示区用例使用的提示词正文。')
  await page.getByTestId('prompt-purpose').locator('.el-select__wrapper').click()
  await page.getByRole('option', { name: '边界探测', exact: true }).click()
  await page.getByTestId('prompt-capability').locator('.el-select__wrapper').click()
  await page.getByRole('option', { name: '推理', exact: true }).click()
  await page.keyboard.press('Escape')
  await page.getByTestId('prompt-submit').click()
  await expect(page).toHaveURL(/\/prompts\/\d+$/)
  const promptID = Number.parseInt(page.url().match(/\/prompts\/(\d+)$/)?.[1] ?? '0', 10)

  await submitResult(page, promptID, modelID, output)

  // 详情页的跳转指引：入口链接与逐模型的示例链接都能到达展示区。
  await expect(page.getByTestId('showcase-entry-link')).toBeVisible()
  await expect(page.getByTestId(`rating-showcase-${modelID}`)).toBeVisible()

  await openModelEntries(page, modelID)
  const entries = page.getByTestId('showcase-entries')
  await expect(entries.getByText(title).first()).toBeVisible()
  await expect(entries.getByText(output)).toBeVisible()
  await expect(
    page.locator('[data-testid^="showcase-entry-image-"]').filter({ hasText: title }),
  ).toHaveCount(1)

  // 逐模型链接带上模型参数，直接定位到同一模型。
  await page.goto(`/prompts/${promptID}`)
  await page.getByTestId(`rating-showcase-${modelID}`).click()
  await expect(page).toHaveURL(new RegExp(`/showcase\\?model=${modelID}$`))
  await expect(page.getByTestId('showcase-current-model')).toHaveText(/./)
})

test('管理员可以在结果展示区治理中删除违规图片', async ({ browser }) => {
  const userContext: BrowserContext = await browser.newContext({
    locale: 'zh-CN',
    baseURL: BASE_URL,
  })
  const userPage = await userContext.newPage()
  await loginShared(userPage)
  const modelID = await firstModelID(userPage)
  const title = `违规图片-${Date.now()}`

  await userPage.goto('/upload')
  await expect(userPage.getByTestId('prompt-title')).toBeVisible()
  await userPage.getByTestId('prompt-title').fill(title)
  await userPage.getByTestId('prompt-body').fill('管理员删除用例的提示词正文。')
  await userPage.getByTestId('prompt-purpose').locator('.el-select__wrapper').click()
  await userPage.getByRole('option', { name: '边界探测', exact: true }).click()
  await userPage.getByTestId('prompt-capability').locator('.el-select__wrapper').click()
  await userPage.getByRole('option', { name: '推理', exact: true }).click()
  await userPage.keyboard.press('Escape')
  await userPage.getByTestId('prompt-submit').click()
  await expect(userPage).toHaveURL(/\/prompts\/\d+$/)
  const promptID = Number.parseInt(userPage.url().match(/\/prompts\/(\d+)$/)?.[1] ?? '0', 10)

  await submitResult(userPage, promptID, modelID, `待删除输出-${Date.now()}`)
  await userContext.close()

  const adminContext: BrowserContext = await browser.newContext({
    locale: 'zh-CN',
    baseURL: BASE_URL,
  })
  const adminPage = await adminContext.newPage()
  await loginAdmin(adminPage)
  await adminPage.goto('/admin/showcase')
  await expect(adminPage.getByTestId('admin-showcase-image-table')).toBeVisible()

  const row = adminPage.getByRole('row').filter({ hasText: title })
  await expect(row).toHaveCount(1)
  await row.getByRole('button', { name: '删除' }).click()
  await adminPage.locator('.el-message-box__btns .el-button--primary').click()
  await expect(adminPage.getByText('素材已删除。')).toBeVisible()

  // 删除后公开展示区不再展示该图片，管理员列表也不再返回它。
  await adminPage.goto('/showcase')
  await adminPage.getByTestId(`showcase-model-${modelID}`).click()
  await expect(adminPage.getByTestId('showcase-entries')).toBeVisible()
  await expect(
    adminPage.locator('[data-testid^="showcase-entry-image-"]').filter({ hasText: title }),
  ).toHaveCount(0)

  await adminPage.goto('/admin/showcase')
  await expect(adminPage.getByRole('row').filter({ hasText: title })).toHaveCount(0)
  await adminContext.close()
})
