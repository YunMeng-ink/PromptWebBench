SHELL := /bin/sh
BACKEND := .

.PHONY: all schema generate build vet fmt test test-unit lint clean

all: fmt vet test build

# 生成 db/schema.sql（sqlc 的输入）
schema:
	cd $(BACKEND) && go run ./cmd/schemagen

# 生成 sqlc 类型安全查询
generate: schema
	cd $(BACKEND) && sqlc generate

fmt:
	cd $(BACKEND) && gofmt -l -w ./cmd ./internal ./migrations

vet:
	cd $(BACKEND) && go vet ./...

build:
	cd $(BACKEND) && go build ./...

test:
	cd $(BACKEND) && go test ./...

test-unit:
	cd $(BACKEND) && go test ./internal/...

clean:
	cd $(BACKEND) && rm -rf bin
