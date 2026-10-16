import { expect, test } from '@playwright/test'
import { createPrompt, loginReporter, loginShared, SHARED_BASE_URL } from './helpers'

/** firstModelID 取出字典中的第一个可用模型。 */
async function firstModelID(page: import('@playwright/test').Page): Promise<number> {
  const id = await page.evaluate(async () => {
    const response = await fetch('/api/v1/models')
    const body = (await response.json()) as { items: { id: number }[] }
    return body.items[0]?.id ?? 0
  })
  if (id === 0) {
    throw new Error('模型字典为空')
  }
  return id
}

test('收藏提示词后出现在我的收藏夹里', async ({ page }) => {
  await loginShared(page)
  const promptID = await createPrompt(page, `收藏验证-${Date.now()}`)

  await page.getByTestId('detail-favorite').click()
  const name = `收藏夹-${Date.now()}`
  await page.getByTestId('favorite-new-name').fill(name)
  await page.getByTestId('favorite-dialog-create').click()

  await expect(page.getByTestId('detail-notice')).toContainText('已收藏')

  await page.goto('/favorites')
  await expect(page.getByTestId('favorites-list')).toContainText(name)
  await expect(page.getByTestId(`favorite-item-${promptID}`)).toContainText('收藏验证')
})

test('游客可查看公开收藏夹但看不到私有收藏夹', async ({ browser }) => {
  const ownerContext = await browser.newContext({ locale: 'zh-CN', baseURL: SHARED_BASE_URL })
  const ownerPage = await ownerContext.newPage()
  await loginShared(ownerPage)
  const title = `公开收藏-${Date.now()}`
  const promptID = await createPrompt(ownerPage, title)

  const ownerID = await ownerPage.evaluate(async () => {
    const response = await fetch('/api/v1/me')
    const body = (await response.json()) as { id: number }
    return body.id
  })

  // 建一个公开收藏夹并放入提示词。可见性必须显式选择：默认档位是私有。
  // 完成信号是对话框关闭：它只在两个请求都成功之后才发生。
  // 不能断言提示文案，因为「已收藏」这个文案在第二次创建时本来就还在屏幕上，
  // 断言会立刻通过，随后的导航会把尚未完成的创建请求取消掉。
  const publicName = `公开夹-${Date.now()}`
  await ownerPage.getByTestId('detail-favorite').click()
  await ownerPage.getByTestId('favorite-new-name').fill(publicName)
  await ownerPage.locator('[data-testid="favorite-visibility"]').getByText('公开').click()
  await ownerPage.getByTestId('favorite-dialog-create').click()
  await expect(ownerPage.getByTestId('favorite-new-name')).toBeHidden()

  // 再建一个私有收藏夹并放入同一条提示词：重新打开后档位回到默认的私有。
  const privateName = `私有夹-${Date.now()}`
  await ownerPage.getByTestId('detail-favorite').click()
  await ownerPage.getByTestId('favorite-new-name').fill(privateName)
  await ownerPage.getByTestId('favorite-dialog-create').click()
  await expect(ownerPage.getByTestId('favorite-new-name')).toBeHidden()

  // 所有者本人两个都看得到。
  await ownerPage.goto('/favorites')
  await expect(ownerPage.getByTestId('favorites-list')).toContainText(publicName)
  await expect(ownerPage.getByTestId('favorites-list')).toContainText(privateName)
  await ownerContext.close()

  // 游客访问该用户的收藏夹页面：只应看到公开的那一个。
  const guestContext = await browser.newContext({ locale: 'zh-CN', baseURL: SHARED_BASE_URL })
  const guestPage = await guestContext.newPage()
  await guestPage.goto(`/users/${ownerID}/favorites`)

  await expect(guestPage.getByTestId('favorites-list')).toContainText(publicName)
  await expect(guestPage.getByTestId('favorites-list')).not.toContainText(privateName)
  await expect(guestPage.getByTestId(`favorite-item-${promptID}`)).toContainText('公开收藏')
  await guestContext.close()
})

test('评论发表、回复与点赞可用', async ({ page }) => {
  await loginShared(page)
  await createPrompt(page, `评论验证-${Date.now()}`)

  await page.getByTestId('comment-input').fill('端到端评论正文')
  await page.getByTestId('comment-submit').click()
  await expect(page.getByTestId('comments-notice')).toContainText('评论已发表')

  const comment = page
    .locator('[data-testid^="comment-"]')
    .filter({ hasText: '端到端评论正文' })
    .first()
  await expect(comment).toBeVisible()

  const commentID = await comment.getAttribute('data-testid')
  const id = commentID?.replace('comment-', '') ?? ''

  // 点赞后计数为 1。
  await page.getByTestId(`comment-like-${id}`).click()
  await expect(page.getByTestId(`comment-like-count-${id}`)).toHaveText('1')

  // 回复自己的评论：回复本身要出现，且不产生通知。
  await page.getByTestId(`comment-reply-${id}`).click()
  await page.getByTestId(`comment-reply-input-${id}`).fill('端到端回复正文')
  await page.getByTestId(`comment-reply-submit-${id}`).click()
  await expect(page.getByTestId('comments-notice')).toContainText('回复已发表')

  // 发表回复后该条评论的回复列表自动展开，回复正文立即可见。
  await expect(page.getByTestId('replies-list')).toContainText('端到端回复正文')

  // 收起后再展开仍然可见。
  await page.getByTestId(`comment-replies-toggle-${id}`).click()
  await expect(page.getByTestId('replies-list')).toHaveCount(0)
  await page.getByTestId(`comment-replies-toggle-${id}`).click()
  await expect(page.getByTestId('replies-list')).toContainText('端到端回复正文')
})

test('举报提交成功且原因以英文枚举键往返', async ({ page, browser }) => {
  // 举报需要非作者身份：作者看不到举报自己的入口。
  const ownerContext = await browser.newContext({ locale: 'zh-CN', baseURL: SHARED_BASE_URL })
  const ownerPage = await ownerContext.newPage()
  await loginShared(ownerPage)
  const promptID = await createPrompt(ownerPage, `举报验证-${Date.now()}`)
  await ownerContext.close()

  await loginReporter(page)
  await page.goto(`/prompts/${promptID}`)

  await page.getByTestId('detail-report').click()
  await page.getByTestId('report-reason-dangerous').click()
  await page.getByTestId('report-submit').click()

  // 提交成功后详情页给出反馈；el-dialog 关闭后仍留在 DOM 中（只是隐藏），
  // 因此断言反馈文案而不是对话框元素是否消失。
  await expect(page.getByTestId('detail-notice')).toContainText('举报已提交')
})

test('举报对话框在英文界面下显示英文原因文案', async ({ page, browser }) => {
  const ownerContext = await browser.newContext({ locale: 'zh-CN', baseURL: SHARED_BASE_URL })
  const ownerPage = await ownerContext.newPage()
  await loginShared(ownerPage)
  const promptID = await createPrompt(ownerPage, `举报文案-${Date.now()}`)
  await ownerContext.close()

  await loginReporter(page)
  await page.goto(`/prompts/${promptID}`)

  await page.getByTestId('detail-report').click()
  await expect(page.getByTestId('report-reason-spam')).toContainText('垃圾广告')
  await page.getByTestId('report-cancel').click()

  // 切换到英文后重开，文案随语言包变化。
  await page.goto('/')
  await page.locator('.pb-header__locale .el-select__wrapper').click()
  await page.getByRole('option', { name: 'English' }).click()

  await page.goto(`/prompts/${promptID}`)
  await page.getByTestId('detail-report').click()
  await expect(page.getByTestId('report-reason-spam')).toContainText('Spam')
})

test('评论回复产生通知，通知中心六项能力可用', async ({ browser }) => {
  // 共用账户发布提示词并评论，第二账户回复：共用账户因此收到一条 comment_reply。
  const ownerContext = await browser.newContext({ locale: 'zh-CN', baseURL: SHARED_BASE_URL })
  const ownerPage = await ownerContext.newPage()
  await loginShared(ownerPage)
  const promptID = await createPrompt(ownerPage, `通知验证-${Date.now()}`)

  await ownerPage.getByTestId('comment-input').fill('等待回复的评论')
  await ownerPage.getByTestId('comment-submit').click()
  await expect(ownerPage.getByTestId('comments-notice')).toContainText('评论已发表')

  const commentID = await ownerPage.evaluate(async (id) => {
    const response = await fetch(`/api/v1/prompts/${id}/comments`)
    const body = (await response.json()) as { items: { id: number }[] }
    return body.items[0]?.id ?? 0
  }, promptID)
  expect(commentID).toBeGreaterThan(0)

  // 第二账户回复该评论。
  const replierContext = await browser.newContext({ locale: 'zh-CN', baseURL: SHARED_BASE_URL })
  const replierPage = await replierContext.newPage()
  await loginReporter(replierPage)
  await replierPage.goto(`/prompts/${promptID}`)
  await replierPage.getByTestId(`comment-reply-${commentID}`).click()
  await replierPage.getByTestId(`comment-reply-input-${commentID}`).fill('来自第二账户的回复')
  await replierPage.getByTestId(`comment-reply-submit-${commentID}`).click()
  await expect(replierPage.getByTestId('comments-notice')).toContainText('回复已发表')
  await replierContext.close()

  // 共用账户收到通知：顶栏角标出现。
  await ownerPage.reload()
  await expect(ownerPage.getByTestId('header-unread')).toHaveText('1')

  // 1. 查看通知与未读数
  await ownerPage.getByTestId('header-notifications').click()
  await expect(ownerPage).toHaveURL(/\/notifications$/)
  await expect(ownerPage.getByTestId('notifications-list')).toContainText('你的评论收到了回复')
  await expect(ownerPage.getByTestId('notifications-unread')).toContainText('未读 1 条')

  // 2. 按类型筛选
  const select = ownerPage.getByTestId('notifications-filter').locator('.el-select__wrapper')
  await select.click()
  await ownerPage.getByRole('option', { name: '评论回复' }).click()
  await ownerPage.keyboard.press('Escape')
  await expect(ownerPage.getByTestId('notifications-list')).toContainText('你的评论收到了回复')

  // 3. 标记单条已读
  const notificationID = await ownerPage
    .locator('[data-testid^="notification-read-"]')
    .first()
    .getAttribute('data-testid')
  const id = notificationID?.replace('notification-read-', '') ?? ''
  await ownerPage.getByTestId(`notification-read-${id}`).click()
  await expect(ownerPage.getByTestId('notifications-unread')).toContainText('没有未读通知')

  // 4. 全部标记已读
  await ownerPage.getByTestId('notifications-mark-all').click()
  await expect(ownerPage.getByTestId('notifications-unread')).toContainText('没有未读通知')

  // 5. 删除通知
  await ownerPage.getByTestId(`notification-delete-${id}`).click()
  await expect(ownerPage.getByTestId('notifications-notice')).toContainText('通知已删除')
  await expect(ownerPage.getByTestId('notifications-empty')).toBeVisible()

  // 6. 未读角标随已读全部归零
  await ownerPage.goto('/')
  await expect(ownerPage.getByTestId('header-unread')).toHaveCount(0)
  await ownerContext.close()
})

test('公告列表接口对游客可用', async ({ page }) => {
  await page.goto('/')

  const result = await page.evaluate(async () => {
    const response = await fetch('/api/v1/announcements')
    const body = (await response.json()) as { items: unknown[]; total: number }
    return { status: response.status, hasItems: Array.isArray(body.items), total: body.total }
  })
  expect(result.status).toBe(200)
  expect(result.hasItems).toBe(true)
  expect(result.total).toBeGreaterThanOrEqual(0)
})

test('未登录访问收藏夹页被引导到登录页', async ({ page }) => {
  await page.goto('/favorites')
  await expect(page).toHaveURL(/\/login/)
})

test('详情页无评分时评论与测量结果区同时可用', async ({ page }) => {
  await loginShared(page)
  const modelID = await firstModelID(page)
  await createPrompt(page, `并存验证-${Date.now()}`)

  await expect(page.getByTestId('stats-section')).toBeVisible()
  await expect(page.getByTestId('comments-section')).toBeVisible()
  await expect(page.getByTestId('comments-empty')).toBeVisible()

  await page.getByTestId(`rating-start-${modelID}`).click()
  await page.locator('[data-testid="rating-score-4"]').click()
  await page.getByTestId('rating-confirm').click()
  await expect(page.getByTestId('detail-notice')).toContainText('评分已提交')

  // 评分后测量结果更新，评论区的空态不受影响。
  await expect(page.getByTestId('stats-total-ratings')).toHaveText('1')
  await expect(page.getByTestId('comments-empty')).toBeVisible()
})
