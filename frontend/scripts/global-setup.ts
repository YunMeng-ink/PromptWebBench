// Playwright 全局前置：服务就绪后准备端到端测试所需的固定数据。
//
// 两类内容：
//   1. 模型与标签。正式来源是后台与 models.dev 导入，那部分在阶段 6 交付；
//      评分的端到端用例需要一个可评分的模型，这里每次运行准备两条。
//   2. 共用账户。走真实的注册与邮箱验证流程创建，使登录校验路径与真实用户一致；
//      各规格文件只负责登录，避免重复注册耗尽认证类限流额度。
//
// 前者在后端启动之后执行，此时迁移已完成，表一定存在。

import { execFileSync } from 'node:child_process'
import { readdirSync, readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import type { FullConfig } from '@playwright/test'

const here = dirname(fileURLToPath(import.meta.url))
const frontendDir = resolve(here, '..')
const repoRoot = resolve(frontendDir, '..')
const MAIL_DROP = resolve(repoRoot, 'backend', '.maildrop')

const API = process.env.E2E_API ?? 'http://127.0.0.1:8080/api/v1'
const DB_NAME = process.env.E2E_DATABASE ?? 'prompt_benchmark_e2e'
const SHARED_EMAIL = 'e2e-shared@example.com'
const SHARED_PASSWORD = 'E2ePassw0rd!'

async function waitForBackend(attempts = 60): Promise<void> {
  for (let i = 0; i < attempts; i += 1) {
    try {
      const response = await fetch(`${API}/health`)
      if (response.ok) {
        return
      }
    } catch {
      // 服务尚未就绪，继续等待。
    }
    await new Promise((done) => setTimeout(done, 500))
  }
  throw new Error(`后端健康检查在预期时间内未就绪：${API}/health`)
}

function psql(sql: string, quiet = false): string {
  return execFileSync(
    'psql',
    ['-U', 'postgres', '-h', '127.0.0.1', '-v', 'ON_ERROR_STOP=1', '-t', '-A', '-d', DB_NAME, '-c', sql],
    { encoding: 'utf8', stdio: quiet ? 'pipe' : 'inherit' },
  )
}

/** latestCodeFor 从落盘的邮件中取出指定收件人的验证码。 */
function latestCodeFor(email: string): string {
  const files = readdirSync(MAIL_DROP)
    .filter((name) => name.endsWith('.eml'))
    .sort()

  for (let i = files.length - 1; i >= 0; i -= 1) {
    const raw = readFileSync(resolve(MAIL_DROP, files[i] as string), 'utf8')
    if (!raw.includes(email)) {
      continue
    }
    const match = raw.match(/\b(\d{6})\b/)
    if (match?.[1] !== undefined) {
      return match[1]
    }
  }
  throw new Error(`邮件目录中未找到发往 ${email} 的验证码`)
}

/** cookies 保存本文件发出的请求所累积的 Cookie，供写请求做 CSRF 双提交。 */
const cookies = new Map<string, string>()

/** absorb 记录响应头下发的 Cookie。 */
function absorb(response: Response): void {
  // Node 的 fetch 不暴露 getSetCookie，取原始头自行切分多条 Cookie。
  const raw = response.headers.get('set-cookie')
  if (raw === null) {
    return
  }
  for (const entry of raw.split(/,(?=[^;,]+=)/)) {
    const [pair] = entry.split(';')
    const index = pair?.indexOf('=') ?? -1
    if (pair !== undefined && index > 0) {
      cookies.set(pair.slice(0, index).trim(), pair.slice(index + 1).trim())
    }
  }
}

/** post 发出一次写请求，附带 Cookie 与 CSRF 头。 */
async function post(path: string, body: unknown): Promise<Response> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  const csrf = cookies.get('pb_csrf')
  if (csrf !== undefined) {
    headers['X-CSRF-Token'] = csrf
  }
  if (cookies.size > 0) {
    headers.Cookie = [...cookies].map(([k, v]) => `${k}=${v}`).join('; ')
  }

  const response = await fetch(`${API}${path}`, {
    method: 'POST',
    headers,
    body: JSON.stringify(body),
  })
  absorb(response)
  return response
}

/**
 * ensureSharedAccount 通过真实流程创建共用账户，已存在则跳过。
 *
 * 直接对后端发请求：全局前置在后端就绪之后、前端预览服务起来之前运行，
 * 此时还没有可用的页面。Cookie 由本文件自己维护，写方法的 CSRF 双提交
 * 因此与浏览器中的行为一致。
 */
async function ensureSharedAccount(): Promise<void> {
  if (psql(`SELECT count(*) FROM users WHERE email = '${SHARED_EMAIL}'`, true).trim() === '1') {
    return
  }

  // 任一业务读请求都会让中间件下发 CSRF Cookie。
  absorb(await fetch(`${API}/categories`))

  const registered = await post('/auth/register', {
    email: SHARED_EMAIL,
    password: SHARED_PASSWORD,
  })
  if (!registered.ok) {
    throw new Error(`创建共用账户失败：${await registered.text()}`)
  }

  const verified = await post('/auth/verify-email', {
    email: SHARED_EMAIL,
    code: latestCodeFor(SHARED_EMAIL),
  })
  if (!verified.ok) {
    throw new Error(`验证共用账户失败：${await verified.text()}`)
  }
}

export default async function globalSetup(_config: FullConfig): Promise<void> {
  await waitForBackend()

  psql(`INSERT INTO models (name, model_key, provider_key, provider_name)
        VALUES
          ('测试模型 A', 'e2e-model-a', 'e2e', '端到端测试厂商'),
          ('测试模型 B', 'e2e-model-b', 'e2e', '端到端测试厂商')
        ON CONFLICT (model_key, provider_key) WHERE deleted_at IS NULL DO NOTHING`)

  psql(`INSERT INTO tags (slug, name_zh, name_en, priority)
        VALUES ('e2e-tag', '端到端标签', 'E2E Tag', 10)
        ON CONFLICT (slug) DO NOTHING`)

  await ensureSharedAccount()
}