import { execFileSync } from 'node:child_process'
import { mkdirSync, rmSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

// 端到端测试前重建后端数据库并清空邮件目录，使每次运行从确定的状态开始。
// 迁移由后端启动时执行，因此此处只建空库；模型与标签的种子数据在服务就绪后
// 由 seed-e2e.mjs 写入。
const here = dirname(fileURLToPath(import.meta.url))
const frontendDir = resolve(here, '..')
const repoRoot = resolve(frontendDir, '..')

const DB_NAME = 'prompt_benchmark_e2e'
const MAIL_DROP = resolve(repoRoot, 'backend', '.maildrop')

function psql(args) {
  execFileSync('psql', ['-U', 'postgres', '-h', '127.0.0.1', '-v', 'ON_ERROR_STOP=1', ...args], {
    stdio: 'inherit',
  })
}

psql(['-c', `DROP DATABASE IF EXISTS ${DB_NAME}`])
psql(['-c', `CREATE DATABASE ${DB_NAME}`])

rmSync(MAIL_DROP, { recursive: true, force: true })
mkdirSync(MAIL_DROP, { recursive: true })

console.log(`已重建数据库 ${DB_NAME} 并清空 ${MAIL_DROP}`)