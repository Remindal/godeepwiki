# GoWiki

给任意 GitHub 仓库生成**可检索的代码索引**，然后对代码提问——回答带真实文件与行号引用，还能自动生成结构化 Wiki。

```text
摄取 GitHub 仓库 → 切分代码块 → Embedding + BM25 双路索引
→ 问答（流式，引用 [path:start-end]）→ 自动生成仓库 Wiki（可下载）
```

## 功能一览

- **仓库摄取**：git clone（失败自动降级 tarball 镜像下载），自定义 include/exclude 规则，进度按阶段权重推进
- **代码问答**：五层检索栈（Multi-Query 查询改写 → LLM rerank → BM25+向量 RRF 混合 → 代码权重），多轮上下文，流式输出（打字机节奏），`path_filter` 目录级过滤（含存在性校验），引用可点开看完整代码
- **多会话**：每仓库可开多个并列对话，历史侧栏直达，离开页面后台续跑，可中途停止生成
- **增量刷新**：FileHashes diff，只重新索引变更文件
- **Wiki 生成**：全量文件树驱动目录 + Mermaid 架构图渲染，断点续跑，单文件导出下载
- **前端**：Vue 3 + Element Plus，黑白 iOS 圆角风格；设置页改模型/检索参数（密钥仅环境变量注入）
- **工程化**：RabbitMQ 任务队列（重试链 + 死信 + 崩溃恢复 + 消费看门狗）、Redis Streams 事件总线（SSE/WS 推送 + 断线回放）、两级限流、API Key 鉴权（可开关，空配置为 dev 模式）、etcd 配置热更新、Prometheus 指标、OTel 链路

## 技术栈

| 层 | 选型 |
|---|---|
| 后端 | Go 1.24，Gin，pgx/v5 |
| 存储 | Postgres + pgvector（chunks + HNSW）、OpenSearch（BM25）、RabbitMQ（任务队列）、Redis 哨兵（锁/限流/事件）、etcd（配置） |
| LLM | OpenAI 兼容协议：openai / dashscope / siliconflow / voyage / ollama（embedding）；openai / gemini / claude / ollama / deepseek（chat） |
| 前端 | Vue 3 + Vite + TS + Element Plus |

## 快速开始

### 1. 启动基础设施 + 后端

```bash
cd deepwiki
cp .env.example .env   # 复制模板，按下表填入你自己的 key（.env 永不提交）
docker compose up -d --build
docker compose ps      # 10 个容器全部 healthy 即就绪
```

> 密钥安全约定：仓库只提交 `.env.example` 空占位模板，真实 key 一律填在你自己的 `.env`
> （已 gitignore）。`configs/config.yaml` 不落任何明文密钥，全部经环境变量注入。

### 2. 必填环境变量（`.env`）

```bash
DEEPWIKI_POSTGRES_DSN=postgres://deepwiki:deepwiki@postgres:5432/deepwiki?sslmode=disable
DEEPWIKI_RABBITMQ_URL=amqp://deepwiki:deepwiki@rabbitmq:5672/
DEEPWIKI_EMBEDDING_API_KEY=sk-xxx     # embedding provider 密钥
DEEPWIKI_LLM_API_KEY=sk-xxx           # chat provider 密钥
```

默认配置指向 SiliconFlow（`api.siliconflow.cn`）：embedding 用 `BAAI/bge-large-zh-v1.5`（1024 维），chat 用 `deepseek-ai/DeepSeek-V4-Flash`。换 provider 改 `configs/config.yaml` 或 `PUT /api/v1/config`。

### 3. 启动前端（开发）

```bash
cd web
npm install
npm run dev        # http://localhost:5173，/api 自动代理到 8080
```

生产形态：`npm run build` 后由后端直接托管（`web/dist` 已挂载进容器），访问 **http://localhost:8080** 即可，无需单独前端进程。

## API 速览

统一信封 `{code, message, data, request_id}`；dev 模式免鉴权，正式模式 header 带 `X-API-Key`。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/v1/health` | 依赖健康 + worker 状态（60s 探测缓存，毫秒返回） |
| POST | `/api/v1/ingest` | `{repo_url, branch?, options?}` → 202 + task_id |
| GET | `/api/v1/tasks/{id}` | 任务进度（状态机 + 百分比 + stats） |
| POST | `/api/v1/ask` | `{repo_id 或 repo_url, question, mode?, top_k?, path_filter?, history?}` → 答案 + 引用 |
| POST | `/api/v1/ask/stream` | SSE 流式：references → thinking/token → done |
| POST | `/api/v1/wiki/generate` | `{repo_id}` → 202（生成中去重提示 40902） |
| GET | `/api/v1/repos/{id}/wiki` | 目录 + 页面 markdown（含 Mermaid 图） |
| GET | `/api/v1/repos/{id}/wiki/export` | wiki 单文件下载 |
| GET | `/api/v1/chunks/{id}` | 按 chunk_id 取代码块全文（引用查看器） |
| GET | `/api/v1/repos/{id}/paths/exists` | 路径前缀存在性校验（path_filter 辅助） |
| POST | `/api/v1/repos/{id}/refresh` | 增量刷新（同仓分布式锁互斥） |
| GET | `/api/v1/events` | SSE 事件推送（`Last-Event-ID` 断线回放） |
| GET | `/api/v1/ws` | WebSocket 推送（`resume_from` 回放） |
| GET | `/metrics` | Prometheus |

## 目录结构

```text
deepwiki/               # Go 后端
  cmd/server/           # 入口与装配
  internal/
    api/                # 路由/handler/中间件（auth/限流/SSE/WS）
    service/            # 业务编排（ingest/ask/wiki/repo + 三类任务执行器）
    task/               # 任务系统（Manager/Store/WorkerPool/DLQ 消费）
    queue/              # RabbitMQ（拓扑/投递/消费/重连/Reconciler）
    retriever/          # keyword/vector/hybrid(RRF)/rerank
    ingest/             # clone(含 tarball 降级)/parse/chunk/pipeline
    store/              # PG 仓储（repos/chunks/wiki/api_keys）
    embed, llm/         # provider 适配器（gobreaker 熔断）
    eventbus/           # Redis Streams + Pub/Sub 扇出
    config/             # yaml + etcd 热更新（维度探测）
    observability/      # Prometheus 指标 + OTel
  migrations/           # SQL 迁移（只前进）
  docker-compose.yml    # 10 容器编排（全部 restart: unless-stopped）
web/                    # Vue 3 前端（Vite，dev 代理 /api）
```

## 常见问题

**Q: github 拉不下来？**
A: 自动降级 tarball（codeload + 3 个国内镜像轮询），一般不依赖代理。tarball 拉的仓库没有 `.git`，refresh 会失败——网络恢复后删仓重拉即可走真 git。

**Q: 健康检查里 llm/embedding reachable=false？**
A: 看 `docker logs deepwiki-app | grep "probe failed"` 的真实错误（key 无效 403、欠费 402、网络不通超时）。修复后 60s 内自愈。

**Q: 任务卡 pending？**
A: 多半是 RabbitMQ 连接抖动，`docker compose restart app` 即可（Reconciler 会自动重投，wiki 有断点续跑不丢进度）。

**Q: 改 LLM/embedding 模型？**
A: `PUT /api/v1/config` 写入后 `docker compose restart app`（provider 是启动时构造的）。embedding 换模型会触发维度探测，维度与库列不符会被拒绝并提示重建。

## 开发

```bash
cd deepwiki
go build ./... && go vet ./...
go test ./internal/...        # 需要基础设施在线（PG/Redis/RabbitMQ/OpenSearch）
```

代码规范：冻结接口不改动签名；每 goroutine `defer recover()` + `select ctx.Done()`；任务状态唯一来源是 Postgres；禁止回传 `err.Error()` 原文（统一错误码信封）。
