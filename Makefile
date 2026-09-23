# 本地与 CI 共用的验收命令。
# CI 里执行 make ci，本地手动验收也用同样的目标，保证两边跑的是同一套命令。

GO      ?= go
BIN_DIR := bin

.PHONY: help fmt fmt-check vet test test-race build ci clean

help: ## 列出所有可用目标
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  make %-12s %s\n", $$1, $$2}'

fmt: ## 自动格式化所有 Go 源码（会改写文件）
	$(GO) fmt ./...

fmt-check: ## 只检查格式不改文件，存在未格式化文件时失败（CI 使用）
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "以下文件未格式化，请先执行 make fmt："; \
		echo "$$unformatted"; \
		exit 1; \
	fi

vet: ## go vet 静态检查
	$(GO) vet ./...

test: ## 运行单元测试
	$(GO) test ./...

test-race: ## 竞态检测模式下运行测试（需要 CGO；CI 的 Linux runner 自带 gcc）
	CGO_ENABLED=1 $(GO) test -race ./...

build: ## 编译可执行文件到 bin/ 目录
	$(GO) build -o $(BIN_DIR)/dianping ./cmd/service

ci: fmt-check vet test build ## CI 入口：格式检查 → vet → 测试 → 编译

clean: ## 删除构建产物
	rm -rf $(BIN_DIR)
