BINARY := deepwiki

.PHONY: infra-up infra-down infra-ps migrate-status run build test lint vet tidy clean docker-build

# ---- 基础设施（docker compose，总纲 §3.1） ----
infra-up:        ## 拉起全部基础设施并等待 healthy
	docker compose up -d
	@echo "waiting for services to become healthy..."
	@docker compose ps

infra-down:      ## 停止并移除基础设施（数据卷保留；加 -v 可清空）
	docker compose down

infra-ps:        ## 查看基础设施健康状态
	docker compose ps

# ---- 迁移（golang-migrate 只前进；状态核对走 schema_migrations 表） ----
migrate-status:  ## 查看 Postgres schema_migrations 当前版本（期望 version=1 且无 dirty）
	docker compose exec postgres psql -U deepwiki -d deepwiki -c 'SELECT version, dirty FROM schema_migrations;'

# ---- 应用 ----
run:             ## 本地运行（依赖 .env 中的 DEEPWIKI_* 环境变量）
	go run ./cmd/server -config configs/config.yaml

build:           ## 编译二进制到 bin/
	go build -o bin/$(BINARY) ./cmd/server

test:            ## 运行全部测试
	go test ./...

lint:            ## 静态检查（未安装 golangci-lint 时退化为 go vet）
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || go vet ./...

vet:             ## go vet
	go vet ./...

tidy:            ## 整理依赖并断言 go.mod/go.sum 无 diff（验收红线）
	go mod tidy
	git diff --exit-code go.mod go.sum

clean:           ## 清理构建产物
	rm -rf bin

docker-build:    ## 构建运行镜像（含 git CLI，Dockerfile 多阶段）
	docker build -t deepwiki:$(shell git rev-parse --short HEAD 2>/dev/null || echo dev) .
