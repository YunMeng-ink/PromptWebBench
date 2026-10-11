import { expect, test } from '@playwright/test'

test('首页渲染顶栏、占位内容与页脚', async ({ page }) => {
  await page.goto('/')

  await expect(page.getByRole('heading', { level: 1 })).toContainText('Prompt Benchmark')
  await expect(page.locator('footer')).toContainText('Prompt Benchmark')
  await expect(page.getByRole('navigation')).toBeVisible()
})

test('未匹配路由展示 404 页面并可返回首页', async ({ page }) => {
  await page.goto('/no-such-page')

  await expect(page.getByText('页面不存在')).toBeVisible()
  await page.getByRole('link', { name: '返回首页' }).click()
  await expect(page).toHaveURL('/')
  await expect(page.getByRole('heading', { level: 1 })).toContainText('Prompt Benchmark')
})

test('切换语言后界面文案变为英文并持久化', async ({ page }) => {
  await page.goto('/')

  // Element Plus 的下拉选择器需要点击其外层容器展开选项。
  await page.locator('.el-select').click()
  await page.getByRole('option', { name: 'English' }).click()

  await expect(page.getByText('The platform skeleton is ready', { exact: false })).toBeVisible()

  await page.reload()
  await expect(page.getByText('The platform skeleton is ready', { exact: false })).toBeVisible()
})

test('主题切换为深色后刷新仍保持', async ({ page }) => {
  await page.goto('/')

  await page.getByRole('button', { name: '主题' }).click()
  await expect(page.locator('html')).toHaveClass(/dark/)

  await page.reload()
  await expect(page.locator('html')).toHaveClass(/dark/)
})
