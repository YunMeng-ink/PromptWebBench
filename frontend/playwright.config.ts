import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { defineConfig, devices } from '@playwright/test'

const here = dirname(fileURLToPath(import.meta.url))
const backendDir = resolve(here, '..', 'backend')

const PORT = 4173
const BASE_URL = `http://127.0.0.1:${PORT}`
const SMTP_PORT = 2525
const API_HEALTH = 'http://127.0.0.1:8080/api/v1/health'

export default defineConfig({
  testDir: './e2e',
  // 服务就绪后写入评分用例所需的模型与标签。迁移由后端启动时执行，
  // 因此种子数据必须在服务起来之后补，不能并入重建数据库的脚本。
  globalSetup: resolve(here, 'scripts', 'global-setup.ts'),
  // 用例共用同一份后端数据与认证类限流额度，串行执行保证结果确定。
  fullyParallel: false,
  workers: 1,
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
  // 三个服务都由本用例集独占启动：端口被占用时立即报错，
  // 避免误用一个版本不符的常驻服务而得出错误的通过结论。
  webServer: [
    {
      // 本地 SMTP 接收器：用例从落盘的邮件中读取验证码。
      command: 'go run ./cmd/smtpsink -addr 127.0.0.1:2525 -dir .maildrop',
      cwd: backendDir,
      port: SMTP_PORT,
      reuseExistingServer: false,
      timeout: 180_000,
    },
    {
      command: 'go run ./cmd/server -env-file .env.e2e',
      cwd: backendDir,
      url: API_HEALTH,
      reuseExistingServer: false,
      timeout: 180_000,
    },
    {
      command: 'pnpm preview',
      cwd: here,
      url: BASE_URL,
      reuseExistingServer: false,
      timeout: 120_000,
    },
  ],
})
