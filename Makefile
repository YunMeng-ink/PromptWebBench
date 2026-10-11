SHELL := /bin/sh

# 集成测试所需：指向一个可建库的维护库。用例各自建库并在结束时销毁。
TEST_DATABASE_URL ?= postgres://postgres@127.0.0.1:5432/postgres?sslmode=disable

.PHONY: gate backend-gate frontend-gate \
        backend-fmt backend-build backend-vet backend-test backend-generate \
        frontend-lint frontend-typecheck frontend-test frontend-build frontend-e2e

gate: backend-gate frontend-gate

# ---------- 后端 ----------

backend-gate: backend-fmt backend-build backend-vet backend-test

# gofmt -l 无输出即通过；有输出则列出未格式化文件并以非零状态退出。
backend-fmt:
	@out=$$(cd backend && gofmt -l ./cmd ./internal ./migrations); \
	if [ -n "$$out" ]; then echo "以下文件未格式化："; echo "$$out"; exit 1; fi; \
	echo "后端格式检查通过"

backend-build:
	cd backend && go build ./...

backend-vet:
	cd backend && go vet ./...

backend-test:
	cd backend && TEST_DATABASE_URL='$(TEST_DATABASE_URL)' go test ./...

backend-generate:
	cd backend && go run ./cmd/schemagen && sqlc generate

# ---------- 前端 ----------

frontend-gate: frontend-lint frontend-typecheck frontend-test frontend-build frontend-e2e

frontend-lint:
	cd frontend && biome check src e2e

frontend-typecheck:
	cd frontend && pnpm typecheck

frontend-test:
	cd frontend && pnpm test

frontend-build:
	cd frontend && pnpm build

frontend-e2e:
	cd frontend && pnpm e2e
