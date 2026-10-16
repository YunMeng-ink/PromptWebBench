// 私有题库的端到端用例：题库生命周期、协作者权限边界、
// 开发者可见引用题的越权读取专项回归、非成员访问。

import { expect, test } from '@playwright/test'
import {
  collaboratorUsername,
  createPrompt,
  createPromptWithVisibility,
  loginDeveloper,
  loginReporter,
  loginShared,
  openAsShared,
} from './helpers'

/** uniqueName 给每次运行一个独立名称，避免同名题库在断言中互相干扰。 */
function uniqueName(prefix: string): string {
  return `${prefix}-${Date.now().toString(36)}`
}

/** createLibrary 从题库列表页创建题库，返回其 ID。 */
async function createLibrary(page: import('@playwright/test').Page, name: string): Promise<number> {
  await openAsShared(page, '/libraries')
  await page.getByTestId('library-create').click()
  await page.getByTestId('library-new-name').fill(name)
  await page.getByTestId('library-create-submit').click()
  await expect(page).toHaveURL(/\/libraries\/\d+$/)
  const match = page.url().match(/\/libraries\/(\d+)$/)
  if (match?.[1] === undefined) {
    throw new Error(`创建题库后未跳转到详情页，当前地址 ${page.url()}`)
  }
  return Number.parseInt(match[1], 10)
}

/** purgeLibrary 从题库列表页彻底删除一个回收站题库。 */
async function purgeLibrary(
  page: import('@playwright/test').Page,
  libraryID: number,
): Promise<void> {
  await openAsShared(page, '/libraries')
  // 列表是异步加载的：必须等它渲染出结果（列表或空态）再判断题库是否存在，
  // 否则会在数据到达之前把「还在的题库」当成「已删除」，跳过删除步骤。
  await expect(
    page.getByTestId('libraries-list').or(page.getByTestId('libraries-empty')),
  ).toBeVisible()
  if ((await page.getByTestId(`library-${libraryID}`).count()) > 0) {
    await page.getByTestId(`library-${libraryID}`).getByRole('link').click()
    await page.getByTestId('library-delete').click()
    await page.getByTestId('library-delete-confirm').click()
    // 删除后回到列表页；同样要等列表渲染完成才能操作回收站条目。
    await expect(page).toHaveURL(/\/libraries$/)
    await expect(
      page.getByTestId('libraries-recycle').or(page.getByTestId('libraries-recycle-empty')),
    ).toBeVisible()
  }
  await page.getByTestId(`library-purge-${libraryID}`).click()
  await page.getByTestId('library-purge-confirm').click()
  await expect(page.getByTestId(`library-recycled-${libraryID}`)).toHaveCount(0)
}

/** firstQuestionID 返回题目列表首题的标识。 */
async function firstQuestionID(page: import('@playwright/test').Page): Promise<number> {
  const testID = await page
    .getByTestId('library-questions')
    .locator('li')
    .first()
    .getAttribute('data-testid')
  return Number.parseInt((testID ?? '').replace('question-', ''), 10)
}

test('题库生命周期：加题、排序、导出、回收站恢复', async ({ page }) => {
  await loginDeveloper(page)
  const promptID = await createPrompt(page, uniqueName('题库引用来源'))
  const libraryName = uniqueName('题库')
  const libraryID = await createLibrary(page, libraryName)

  // 引用题：正文取自来源提示词，标签随题目保存。
  await page.getByTestId('library-add-reference').click()
  await page.getByTestId('library-add-prompt-id').fill(String(promptID))
  await page.getByTestId('library-add-tags').fill('端到端标签甲，端到端标签乙')
  await page.getByTestId('library-add-submit').click()
  await expect(page.getByTestId('library-notice')).toContainText('引用题目已加入题库')
  await expect(page.getByTestId('library-questions')).toContainText('端到端标签甲')

  // 派生题：自带正文，不占题库容量。
  await page.getByTestId('library-add-derived').click()
  await page.getByTestId('library-add-title').fill('派生题目标题')
  await page.getByTestId('library-add-body').fill('派生题目正文')
  await page.getByTestId('library-add-description').fill('派生题目说明')
  await page.getByTestId('library-add-submit').click()
  await expect(page.getByTestId('library-notice')).toContainText('派生题目已加入题库')
  await expect(page.getByTestId('library-questions').locator('li')).toHaveCount(2)

  // 排序：首题下移后不再占据第一位。
  const firstID = await firstQuestionID(page)
  await page.getByTestId(`question-down-${firstID}`).click()
  await expect(page.getByTestId('library-notice')).toContainText('题目顺序已更新')
  expect(await firstQuestionID(page)).not.toBe(firstID)

  // 导出：浏览器收到一个以题库名称命名的 Markdown 文件。
  const [download] = await Promise.all([
    page.waitForEvent('download'),
    page.getByTestId('library-export').click(),
  ])
  expect(download.suggestedFilename()).toBe(`${libraryName}.md`)

  // 题目详情页提供两种复制方式（需求 §10.8）。
  await page.goto(`/library-questions/${firstID}`)
  await page.getByTestId('question-copy-body').click()
  await expect(page.getByTestId('question-copied')).toBeVisible()
  await page.getByTestId('question-copy-full').click()
  await expect(page.getByTestId('question-copied')).toBeVisible()

  // 删除题目 → 题库回收站 → 恢复。
  await page.goto(`/libraries/${libraryID}`)
  await page.getByTestId(`question-delete-${firstID}`).click()
  await page.getByTestId('question-delete-confirm').click()
  await expect(page.getByTestId('library-notice')).toContainText('题目已移入题库回收站')
  await expect(page.getByTestId('library-question-recycle')).toBeVisible()
  await page.getByTestId(`question-restore-${firstID}`).click()
  await expect(page.getByTestId('library-notice')).toContainText('题目已恢复')

  // 删除题库 → 进入回收站 → 恢复。
  await page.getByTestId('library-delete').click()
  await page.getByTestId('library-delete-confirm').click()
  await expect(page).toHaveURL(/\/libraries$/)
  await expect(page.getByTestId(`library-recycled-${libraryID}`)).toBeVisible()
  await page.getByTestId(`library-restore-${libraryID}`).click()
  await expect(page.getByTestId('libraries-notice')).toContainText('题库已恢复')
  await expect(page.getByTestId(`library-${libraryID}`)).toBeVisible()

  await purgeLibrary(page, libraryID)
})

test('协作者可以增删题目，但改不了名称、删不了题库、邀请不了成员', async ({ page }) => {
  const collaborator = collaboratorUsername()

  // 先发提示词再建题库：发布提示词会把页面带到提示词详情，
  // 题库接口只有在题库页上才可达。
  await loginDeveloper(page)
  const promptID = await createPrompt(page, uniqueName('协作引用来源'))
  const libraryName = uniqueName('协作题库')
  const libraryID = await createLibrary(page, libraryName)

  await page.getByTestId('library-invite').click()
  await page.getByTestId('library-invite-name').fill(collaborator)
  await page.getByTestId('library-invite-submit').click()
  await expect(page.getByTestId('library-notice')).toContainText('已邀请')

  // 协作者视角：看得到题库与角色，但没有改名、删库、邀请入口。
  await loginReporter(page)
  await page.goto(`/libraries/${libraryID}`)
  await expect(page.getByTestId('library-name')).toHaveText(libraryName)
  await expect(page.getByTestId('library-meta')).toContainText('协作者')
  await expect(page.getByTestId('library-delete')).toHaveCount(0)
  await expect(page.getByTestId('library-invite')).toHaveCount(0)

  // 修改描述：协作者可以做；名称字段不出现，避免提交被整体拒绝。
  await page.getByTestId('library-edit').click()
  await expect(page.getByTestId('library-edit-name')).toHaveCount(0)
  await page.getByTestId('library-edit-description').fill('协作者写入的描述')
  await page.getByTestId('library-edit-submit').click()
  await expect(page.getByTestId('library-notice')).toContainText('题库已更新')

  // 添加派生题、贴标签、移除标签：协作者都可以做。
  await page.getByTestId('library-add-derived').click()
  await page.getByTestId('library-add-title').fill('协作者添加的题目')
  await page.getByTestId('library-add-body').fill('协作者添加的正文')
  await page.getByTestId('library-add-submit').click()
  const derivedID = await firstQuestionID(page)
  await page.goto(`/library-questions/${derivedID}`)
  await page.getByTestId('question-tag-input').fill('协作标签')
  await page.getByTestId('question-tag-add').click()
  await expect(page.getByTestId('question-notice')).toContainText('标签已添加')
  await page.getByTestId('tag-remove-协作标签').click()
  await expect(page.getByTestId('question-notice')).toContainText('标签已移除')

  // 所有者加入一条引用题，供协作者删除。
  await loginDeveloper(page)
  await page.goto(`/libraries/${libraryID}`)
  await page.getByTestId('library-add-reference').click()
  await page.getByTestId('library-add-prompt-id').fill(String(promptID))
  await page.getByTestId('library-add-submit').click()
  await expect(page.getByTestId('library-questions')).toContainText('协作引用来源')
  const referenceID = await firstQuestionID(page)

  // 协作者删除引用题：所有者收到站内通知（需求 §10.6、C-34）。
  await loginReporter(page)
  await page.goto(`/libraries/${libraryID}`)
  await page.getByTestId(`question-delete-${referenceID}`).click()
  await page.getByTestId('question-delete-confirm').click()
  await expect(page.getByTestId('library-notice')).toContainText('题目已移入题库回收站')

  await loginDeveloper(page)
  await page.goto('/notifications')
  await expect(page.getByTestId('notifications-list')).toContainText('题库题目被协作者删除')
  await expect(page.getByTestId('notifications-list')).toContainText(collaborator)

  await purgeLibrary(page, libraryID)
})

test('普通用户协作者看不到开发者可见的引用题', async ({ page }) => {
  const collaborator = collaboratorUsername()

  await loginDeveloper(page)
  const secret = await createPromptWithVisibility(page, uniqueName('开发者专属'), 'developer')
  const libraryID = await createLibrary(page, uniqueName('开发者内容题库'))

  await page.getByTestId('library-add-reference').click()
  await page.getByTestId('library-add-prompt-id').fill(String(secret))
  await page.getByTestId('library-add-submit').click()
  await expect(page.getByTestId('library-questions')).toContainText('开发者专属')

  await page.getByTestId('library-invite').click()
  await page.getByTestId('library-invite-name').fill(collaborator)
  await page.getByTestId('library-invite-submit').click()
  await expect(page.getByTestId('library-notice')).toContainText('已邀请')

  // 协作者看不到该题目：列表为空，直接打开题目地址也拿不到正文。
  await loginReporter(page)
  await page.goto(`/libraries/${libraryID}`)
  await expect(page.getByTestId('library-questions-empty')).toBeVisible()
  await page.goto(`/library-questions/${secret}`)
  await expect(page.getByTestId('question-title')).toHaveCount(0)

  // 导出同样不含开发者专属正文。
  await page.goto(`/libraries/${libraryID}`)
  const [download] = await Promise.all([
    page.waitForEvent('download'),
    page.getByTestId('library-export').click(),
  ])
  const stream = await download.createReadStream()
  const chunks: Buffer[] = []
  for await (const chunk of stream) {
    chunks.push(chunk as Buffer)
  }
  expect(Buffer.concat(chunks).toString('utf8')).not.toContain('开发者可见的提示词正文')

  await loginDeveloper(page)
  await purgeLibrary(page, libraryID)
})

test('非成员打不开他人题库', async ({ page }) => {
  await loginDeveloper(page)
  const libraryID = await createLibrary(page, uniqueName('私有题库'))

  await loginShared(page)
  await page.goto(`/libraries/${libraryID}`)
  await expect(page.getByTestId('library-error')).toContainText('不存在')

  await loginDeveloper(page)
  await purgeLibrary(page, libraryID)
})
