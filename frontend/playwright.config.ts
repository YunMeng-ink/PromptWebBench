import { defineConfig, devices } from '@playwright/test'

const PORT = 4173
const BASE_URL = `http://127.0.0.1:${PORT}`

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: true,
  retries: 0,
  reporter: [['list']],
  use: {
    baseURL: BASE_URL,
    // 平台按浏览器语言自动选择默认语言，用例固定为中文以断言中文文案。
    locale: 'zh-CN',
    trace: 'retain-on-failure',
  },
  // 使用本机安装的 Chrome，避免依赖 Playwright 自带的浏览器下载。
  projects: [{ name: 'chrome', use: { ...devices['Desktop Chrome'], channel: 'chrome' } }],
  webServer: {
    command: 'pnpm preview',
    url: BASE_URL,
    reuseExistingServer: true,
    timeout: 120_000,
  },
})
