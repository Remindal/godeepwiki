# GoWiki 后端

Git 仓库智能问答系统：仓库异步摄取（git 克隆→解析→切分→向量化→落库）→ 语义检索问答
（keyword=OpenSearch BM25 / embedding=pgvector HNSW / hybrid RRF + Multi-Query 改写 + Rerank，
父子块双层索引，SSE 流式 + thinking 思维链）→ 异步 Wiki 生成（文件树目录 + Mermaid 图 + 断点续跑）。

企业级基础设施：PostgreSQL 16 + pgvector、OpenSearch 2.17、RabbitMQ（DLX 重试）、
Redis 哨兵集群（1 主 2 从 3 哨兵）、etcd 配置中心、OpenTelemetry + Prometheus。

## 系统依赖

- Go 1.25+（本地开发）；Docker 构建使用 golang:1.24-alpine
- Docker / Docker Compose（基础设施 + 应用一体部署）
- git ≥ 2.30（git CLI 为硬依赖：Docker 镜像已内置；本地开发需自行安装）
- 前端源码在仓库根 `web/`（Vue3 + Element Plus），`npm run build` 产物由后端托管

## 快速开始

```bash
# 1. 准备环境变量并拉起全部容器（postgres/opensearch/rabbitmq/redis 1主2从3哨兵/etcd/app）
cp .env.example .env            # 按需填入 LLM/Embedding 密钥（也可留空走开发模式）
docker compose up -d --build    # docker compose ps 确认全部 healthy

# 2. 首次启动自动执行数据库迁移（migrations/000001~000004）并引导 API key

# 3. 验收健康检查
curl http://localhost:8080/api/v1/health
```

浏览器访问 `http://localhost:8080` 使用前端（单端口托管，SPA 回退）。

本地开发（不容器化应用）：

```bash
docker compose up -d postgres opensearch rabbitmq redis-master redis-replica-1 redis-replica-2 \
  redis-sentinel-1 redis-sentinel-2 redis-sentinel-3 etcd
set -a; source .env; set +a     # Windows PowerShell 请改用 $env:VAR 逐条设置
make run                        # 监听 :8080
```

## 测试

```bash
go build ./... && go vet ./... && go test ./...
# 集成测试自动探测 localhost 基础设施，不可达则 Skip
```

## 文档

- [docs/API.md](docs/API.md)：API v1 完整接口文档（21 个端点、错误码全表、SSE/WebSocket 事件格式）
- [docs/gowiki.postman_collection.json](docs/gowiki.postman_collection.json)：Postman 调试集合
