# Prompt Benchmark

提示词能力边界评测平台。产品需求见 `需求.md`，实施方案见 `开发方案.md`，技术选型与需求裁决见 `技术可行性报告.md`。

## 使用的技术

| 层 | 选型 |
|---|---|
| 语言与框架 | Go 1.27、Echo v5.4.0、Huma v2.39.1 |
| 数据访问 | sqlc 1.31.1 生成查询 + pgx v5.11.0，动态筛选手写 |
| 数据库 | PostgreSQL 18 |
| 迁移 | 内置 runner，脚本 `//go:embed` 进二进制，启动时幂等应用 |
| 前端 | Vue 3.5、Vite 8、TypeScript 6.0、vue-router 5、Element Plus 2.14、Pinia 4、TanStack Query |
| 单元与集成测试 | `go test` + 真实 PostgreSQL；Vitest 5 |
| 端到端测试 | Playwright 1.64（Chrome、Edge、Firefox、WebKit 四浏览器） |
| 静态检查 | gofmt、go vet、biome 2.5、shellcheck、shfmt |

## 文档

| 文档 | 内容 |
|---|---|
| `docs/架构说明.md` | 按最终实现描述系统结构与结构性约束 |
| `docs/需求追溯表.md` | 需求条目 → 实现位置 → 验收方式 的逐条映射 |
| `docs/部署手册.md` | Ubuntu 单实例部署的完整命令 |
| `docs/开发与测试指南.md` | 构建、检查、测试、冒烟的完整命令 |
| `docs/验收记录-阶段7.md` | 加固与验收的实际结果与证据 |
| API 文档 | 由 Huma 生成，不手工维护：`/api/v1/docs` |

## 目录结构

| 路径 | 内容 |
|---|---|
| `backend/cmd/server` | 服务装配与启动 |
| `backend/cmd/loadtest` | 人工压测工具，不进流水线 |
| `backend/cmd/schemagen` | 由迁移生成 `db/schema.sql`（sqlc 的输入） |
| `backend/migrations` | `001` 起顺序编号的正向迁移与种子数据；`down/` 为逆向脚本 |
| `backend/db/query` | sqlc 命名查询 |
| `backend/internal/domain` | 实体与业务规则，不依赖上层 |
| `backend/internal/formula` | 纯数值计算，零依赖、无 I/O |
| `backend/internal/service` | 用例编排与事务边界 |
| `backend/internal/http` | Echo 与 Huma 装配、统一错误模型、限流、中间件 |
| `backend/internal/repo` | sqlc 生成代码与手写转换 |
| `backend/internal/platform` | 连接池、日志、迁移 runner、集成测试环境 |
| `frontend/src` | 视图、组件、状态、语言包、接口封装 |
| `frontend/e2e` | Playwright 用例 |
| `deploy/backup.sh` | 每日逻辑备份（保留 7 天） |
| `docs/` | 架构、追溯、部署、测试与验收文档 |

## 运行前提

| 组件 | 要求 |
|---|---|
| Go | 1.27 及以上 |
| Node | 22.12 及以上 |
| PostgreSQL | 18，需要一个可建库的账号 |
| sqlc | 1.31（仅在改动 `db/query` 后重新生成时需要） |
| biome | 2.5（前端检查与格式化） |
| Chrome | 本机安装的稳定版，供端到端测试驱动 |

## 后端

```sh
cd backend
cp .env.example .env.local          # 至少填写 JWT_SECRET
psql -U postgres -c 'CREATE DATABASE prompt_benchmark'

go run ./cmd/server -env-file .env.local   # 迁移随启动幂等应用
```

`-env-file` 可选：从文件加载 KEY=VALUE，已经存在的环境变量优先，便于临时覆盖。

接口前缀为 `/api/v1`；OpenAPI 3.1 规范位于 `/api/v1/openapi.json`，浏览器可读的接口文档位于 `/api/v1/docs`，健康检查为 `/api/v1/health`，指标抓取为 `/api/v1/metrics`（仅回环地址可达）。

`ADMIN_INIT_PASSWORD` 非空时，首次启动会创建管理员账号 `admin`；未设置则跳过创建，不生成默认口令。

本地开发若需要接收验证码邮件，可另开一个进程运行内置的 SMTP 接收器，把邮件落盘到目录后直接查看：

```sh
cd backend
go run ./cmd/smtpsink -addr 127.0.0.1:2525 -dir .maildrop
```

改动 `db/query/*.sql` 或迁移后重新生成查询代码：

```sh
cd backend && make generate
```

## 前端

```sh
cd frontend
pnpm install
pnpm dev                            # http://localhost:5173，/api 反向代理到后端 8080
```

语言与主题的选择保存在浏览器本地存储；默认语言按浏览器语言选择，中文浏览器得到中文界面。

## 测试

集成测试需要一个真实 PostgreSQL 实例；端到端测试还需要四个浏览器：Chrome 与 Edge 用本机安装的稳定版，Firefox 与 WebKit 由 Playwright 提供。

```sh
# 集成测试：TEST_DATABASE_URL 指向一个可建库的维护库，用例各自建库并在结束时销毁
cd backend
TEST_DATABASE_URL='postgres://postgres@127.0.0.1:5432/postgres?sslmode=disable' go test ./...

# 前端单元测试
cd frontend && pnpm test

# 端到端测试：脚本先重建 e2e 数据库并清空邮件目录，
# 再由 Playwright 拉起 SMTP 接收器、models.dev 替身、后端（backend/.env.e2e）与前端预览服务
cd frontend && npx playwright install firefox webkit
cd frontend && pnpm build && pnpm e2e

# 指定浏览器（默认集为 Chrome）
pnpm e2e:only --project=firefox
pnpm e2e:only --project=chrome --project=edge --project=firefox --project=webkit
```

## 备份

```sh
BACKUP_DIR=/var/backups/prompt-benchmark deploy/backup.sh
```

每日逻辑备份、保留 7 天（G-01），部署方式见 `docs/部署手册.md`。

## 门禁

`make gate` 依次执行全部阶段门禁：后端格式、编译、静态检查、测试，前端检查、类型、单元测试、构建、端到端测试。

```sh
make gate
TEST_DATABASE_URL='postgres://postgres@127.0.0.1:5432/postgres?sslmode=disable' make gate
```

后端与前端也可分别执行 `make backend-gate`、`make frontend-gate`。
