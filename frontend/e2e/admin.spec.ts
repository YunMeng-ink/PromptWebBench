// 管理后台的端到端用例：11 个模块的入口可用、非管理员被拦截、
// 模型导入的重复处理、提示词生命周期、配置项保存与操作日志留痕。

import { expect, test } from '@playwright/test'
import { ADMIN_EMAIL, BASE_URL, createPrompt, loginAdmin, loginShared, openAdmin } from './helpers'

/**
 * MODULES 是 11 个模块的导航标识、路径与页面标题。
 * 断言标题而不是表格：空数据时视图只显示空态文案，不渲染表格。
 */
const MODULES: { nav: string; path: string; title: string }[] = [
  { nav: 'admin-nav-admin-dashboard', path: '/admin/dashboard', title: '统计仪表盘' },
  { nav: 'admin-nav-admin-prompts', path: '/admin/prompts', title: '提示词管理' },
  { nav: 'admin-nav-admin-models', path: '/admin/models', title: '模型管理' },
  { nav: 'admin-nav-admin-users', path: '/admin/users', title: '用户管理' },
  { nav: 'admin-nav-admin-tags', path: '/admin/tags', title: '标签管理' },
  { nav: 'admin-nav-admin-reports', path: '/admin/reports', title: '举报管理' },
  { nav: 'admin-nav-admin-invites', path: '/admin/invites', title: '注册与邀请码' },
  { nav: 'admin-nav-admin-logs', path: '/admin/logs', title: '操作日志' },
  { nav: 'admin-nav-admin-announcements', path: '/admin/announcements', title: '公告管理' },
  { nav: 'admin-nav-admin-recycle', path: '/admin/recycle', title: '回收站管理' },
  { nav: 'admin-nav-admin-settings', path: '/admin/settings', title: '系统设置' },
]

/** confirmButton 是 Element Plus 确认对话框的确认按钮。 */
function confirmButton(page: import('@playwright/test').Page) {
  return page.getByRole('button', { name: '确定' }).last()
}

test('非管理员访问后台被拦截', async ({ page }) => {
  await loginShared(page)

  // 前端守卫：被弹回首页，且不渲染后台导航。
  await page.goto('/admin/dashboard')
  await expect(page).toHaveURL(`${BASE_URL}/`)
  await expect(page.getByTestId('admin-nav')).toHaveCount(0)

  // 后端中间件：直接请求后台接口得到 403。
  const response = await page.request.get('/api/v1/admin/dashboard')
  expect(response.status()).toBe(403)
  const body = (await response.json()) as { error: { code: string } }
  expect(body.error.code).toBe('permission_denied')
})

test('管理员可以进入 11 个模块', async ({ page }) => {
  await openAdmin(page, '/admin/dashboard')
  await expect(page.getByTestId('admin-dashboard-count-user_total')).toBeVisible()
  await expect(page.getByTestId('admin-dashboard-hot')).toBeVisible()

  for (const module of MODULES) {
    await page.getByTestId(module.nav).click()
    await expect(page).toHaveURL(`${BASE_URL}${module.path}`)
    await expect(page.getByRole('heading', { level: 2 })).toHaveText(module.title)
  }
})

test('模型操作收纳在更多菜单中', async ({ page }) => {
  await openAdmin(page, '/admin/models')

  await page.getByTestId('admin-models-more').first().click()
  await expect(
    page.locator('.el-dropdown-menu__item:visible', { hasText: '关联提示词' }),
  ).toBeVisible()
  await expect(
    page.locator('.el-dropdown-menu__item:visible', { hasText: '原始字段快照' }),
  ).toBeVisible()
})

test('分类字典的考察目的标签页不显示文案键', async ({ page }) => {
  await openAdmin(page, '/admin/dictionary')
  await page.getByRole('tab', { name: '考察目的' }).click()

  const table = page.getByTestId('admin-dictionary-purpose-table')
  await expect(table).toBeVisible()
  await expect(table).toContainText('删除')
  await expect(table).not.toContainText('admin.common.')
})

test('模型导入的重复处理走二次确认', async ({ page }) => {
  await openAdmin(page, '/admin/models')

  // 首次导入：替身目录里的三条条目被导入。库中可能已有同名模型（重复运行套件时），
  // 因此这里只断言导入完成，具体计数留给下一次预览确认。
  await page.getByTestId('admin-models-import').click()
  await expect(page.getByRole('dialog')).toContainText('从 models.dev 导入')
  await expect(page.getByTestId('admin-models-import-figure-matched')).toContainText('3')

  // 先全选当前筛选结果，再执行导入。
  await page.getByTestId('admin-models-import-select-page').click()
  await expect(page.getByTestId('admin-models-import-selected')).toContainText('已选 3 条')
  await page.getByTestId('admin-models-import-run').click()
  await expect(page.getByText(/导入完成：新建/)).toBeVisible()
  await expect(page.getByTestId('admin-models-table')).toContainText('E2E Import One')
  // 等提示气泡自行消失再进入下一次导入：两次的文案相同，
  // 前一次的气泡还在场时，后一次的断言会同时匹配到两个元素。
  await expect(page.getByText(/导入完成：新建/)).toBeHidden({ timeout: 15000 })

  // 再次预览：三条都标为重复，管理员据此决定是否覆盖。
  await page.getByTestId('admin-models-import').click()
  await expect(page.getByTestId('admin-models-import-figure-duplicate')).toContainText('3')

  // 不勾选覆盖时重复条目被跳过。
  await page.getByTestId('admin-models-import-select-page').click()
  await page.getByTestId('admin-models-import-run').click()
  await expect(page.getByText(/跳过 3 条/)).toBeVisible()
})

test('强制标记重大修改生成新版本并重置评分', async ({ page }) => {
  const title = `后台强制标记 ${Date.now()}`
  await loginShared(page)
  const promptID = await createPrompt(page, title)

  await loginAdmin(page)
  await page.goto(`/admin/prompts?keyword=${encodeURIComponent(title)}`)
  await page.getByTestId('admin-prompts-detail').first().click()
  await expect(page.getByTestId('admin-prompts-detail-title')).toContainText(title)

  await page.getByTestId('admin-prompts-major-mark').click()
  await confirmButton(page).click()
  await expect(page.getByText(/已标记重大修改：版本 v2\.0/)).toBeVisible()

  // 前台详情页显示重大修改提示。
  await page.goto(`/prompts/${promptID}`)
  await expect(page.getByTestId('detail-title')).toBeVisible()
  await expect(page.getByText('该提示词已发生重大修改，当前评分基于最新版本。')).toBeVisible()
})

test('提示词删除后进入回收站并可恢复', async ({ page }) => {
  const title = `后台回收站 ${Date.now()}`
  await loginShared(page)
  await createPrompt(page, title)

  await loginAdmin(page)
  await page.goto(`/admin/prompts?keyword=${encodeURIComponent(title)}`)
  await page.getByTestId('admin-prompts-delete').first().click()
  await confirmButton(page).click()
  await expect(page.getByText('提示词已移入回收站。')).toBeVisible()

  await page.goto('/admin/recycle?object_type=prompt')
  const row = page.getByRole('row', { name: new RegExp(title) })
  await expect(row).toBeVisible()
  await row.getByTestId('admin-recycle-restore').click()
  await expect(page.getByText('对象已恢复。')).toBeVisible()
  await expect(page.getByRole('row', { name: new RegExp(title) })).toHaveCount(0)
})

test('系统设置展示密钥状态并发送测试邮件', async ({ page }) => {
  await openAdmin(page, '/admin/settings')

  // 密钥项以口令输入框呈现，不回显任何取值。
  await expect(page.getByTestId('admin-settings-value-smtp_password')).toHaveAttribute(
    'type',
    'password',
  )

  await page.getByTestId('admin-settings-test-to').fill(ADMIN_EMAIL)
  await page.getByTestId('admin-settings-test-send').click()
  await expect(page.getByText('测试邮件已发送。')).toBeVisible()

  // 保存一个数值型业务参数，验证取值按类型写入。
  await page.getByTestId('admin-settings-save-recycle_retention_days').click()
  await expect(page.getByText('配置项已保存。')).toBeVisible()
})

test('生成邀请码并禁用', async ({ page }) => {
  await openAdmin(page, '/admin/invites')

  await page.getByTestId('admin-invites-generate').click()
  await page.getByTestId('admin-invites-form-count').locator('input').fill('2')
  await page.getByTestId('admin-invites-form-submit').click()
  await expect(page.getByText('已生成 2 个邀请码。')).toBeVisible()

  const firstRow = page.getByTestId('admin-invites-table').getByRole('row').nth(1)
  await expect(firstRow).toContainText('PB-')
  await firstRow.getByTestId('admin-invites-disable').click()
  await expect(page.getByText('邀请码状态已更新。')).toBeVisible()
})

test('发布公告后撤回，用户侧立即消失', async ({ page }) => {
  await openAdmin(page, '/admin/announcements')

  const title = `端到端公告 ${Date.now()}`
  await page.getByTestId('admin-announcements-create').click()
  await page.getByTestId('admin-announcements-form-title').fill(title)
  await page.getByTestId('admin-announcements-form-body').fill('公告正文')
  await page.getByTestId('admin-announcements-form-submit').click()
  await expect(page.getByText('公告已发布。')).toBeVisible()

  // 用户侧的公告接口可以看到这条公告。
  const published = await page.request.get('/api/v1/announcements?page_size=50')
  expect(JSON.stringify(await published.json())).toContain(title)

  // 撤回后立即从用户侧消失（C-33）。
  const row = page.getByRole('row', { name: new RegExp(title) })
  await row.getByTestId('admin-announcements-withdraw').click()
  await confirmButton(page).click()
  await expect(page.getByText('公告已撤回。')).toBeVisible()

  const withdrawn = await page.request.get('/api/v1/announcements?page_size=50')
  expect(JSON.stringify(await withdrawn.json())).not.toContain(title)
})

test('操作日志记录管理动作且不可编辑不可删除', async ({ page }) => {
  await openAdmin(page, '/admin/logs')

  await expect(page.getByTestId('admin-logs-readonly')).toBeVisible()
  await expect(page.getByTestId('admin-logs-action-count')).toContainText('18')
  await expect(page.getByTestId('admin-logs-table')).toContainText('admin')

  // 日志模块只注册读端点：写方法一律不被接受。
  for (const method of ['put', 'post', 'patch', 'delete'] as const) {
    const response = await page.request[method]('/api/v1/admin/operation-logs', { data: {} })
    expect([403, 404, 405]).toContain(response.status())
  }
})

test('后台界面在中英两种语言下都可呈现', async ({ page }) => {
  await openAdmin(page, '/admin/dashboard')

  // 语言选择器在顶栏：点击 el-select 的包装节点展开下拉。
  await page.locator('.pb-header__locale .el-select__wrapper').click()
  await page.getByRole('option', { name: 'English' }).click()
  await expect(page.getByTestId('admin-nav-admin-dashboard')).toHaveText('Dashboard')

  await page.locator('.pb-header__locale .el-select__wrapper').click()
  await page.getByRole('option', { name: '中文' }).click()
  await expect(page.getByTestId('admin-nav-admin-dashboard')).toHaveText('统计仪表盘')
})
