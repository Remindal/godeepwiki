# DeepWiki(Go版)

Git 仓库智能问答系统：仓库异步摄取（git CLI 克隆→解析→切分→向量化→落库）→ 语义检索问答
（keyword=OpenSearch BM25 / embedding=pgvector HNSW / hybrid RRF 可插拔检索 + 多 LLM Provider 官方 SDK，
支持 SSE 流式）→ 异步 Wiki 生成。企业级基础设施：PostgreSQL 16 + pgvector、OpenSearch、RabbitMQ、
Redis 哨兵集群、etcd、OpenTelemetry。

## 系统依赖

- Go 1.22+（本地开发）
- Docker / Docker Compose（基础设施）
- git ≥ 2.30（git CLI 为硬依赖：Docker 镜像已内置 `apk add git`；本地开发需自行安装）

## 快速开始（三步）

```bash
# 1. 准备环境变量并拉起基础设施（postgres/opensearch/rabbitmq/redis 1主2从3哨兵/etcd）
cp .env.example .env            # 按需填入密钥（也可留空走开发模式）
make infra-up                   # = docker compose up -d；docker compose ps 确认全部 healthy

# 2. 启动应用（首次启动自动执行 golang-migrate：000001_init.up.sql）
set -a; source .env; set +a
make run                        # 监听 :8080

# 3. 验收健康检查
curl -s http://localhost:8080/api/v1/health | jq .
```

## 设计文档

- 《00_设计基线.md》：系统设计唯一权威（API 契约、任务状态机、接口签名、错误码、配置表、SQL 均为冻结项）
- 《02_KimiCode_脚手架指令.md》：工程骨架创建指令（硬约束 18 条 + 创建顺序 + 验收标准）
- 《03_企业级技术栈变更总纲.md》：v2 技术栈唯一权威契约（基础设施、配置 Schema、health 契约）

## 当前状态

工程骨架阶段：/api/v1/health 与 /metrics 可用；任务调度链路（Postgres + RabbitMQ）与基础设施
客户端包已装配；其余端点返回 50001 占位信封，按迭代计划分模块补齐。
