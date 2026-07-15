# DeepWiki(Go版) 项目脚手架 · Kimi Code 创建指令

| 项 | 内容 |
|---|---|
| 文档编号 | DW-SCAFFOLD-KIMI-02 |
| 上游依据 | 《DeepWiki(Go版) 系统设计基线》v1.0（冻结版，文档编号 DW-DESIGN-BASELINE-00）＋《DeepWiki(Go版) 企业级技术栈变更总纲》v2.0（2026-07-15，唯一权威技术栈契约） |
| 使用方式 | 用户将本文档**整体复制粘贴**给你（Kimi Code）。本文档即创建项目的唯一指令书 |
| 目标产物 | Go 1.22+ 项目 `deepwiki`：可 `go build ./...`、可 `go vet ./...`、可 `make infra-up` 拉起基础设施后可 `go run ./cmd/server` 启动并正确应答 `GET /api/v1/health` 的完整工程骨架 |

> **给 Kimi Code 的元指令（必须先读）**
> 1. 本文档中所有"禁止 / 必须 / 不得"均为硬约束，不得以任何理由绕过；标注【完整代码】的文件逐字符照抄（含注释）；标注【骨架 TODO】的文件按给定包名、import、类型与函数签名创建，函数体保留文中所写 TODO 注释与占位实现。
> 2. 不得自行增删文件、不得发明新模块 / 新接口 / 新依赖；拿不准时以本文档为准；凡技术栈、配置 key、错误码、索引名、队列名、Redis 键名、etcd 键空间与本文档冲突时，以《企业级技术栈变更总纲》v2.0 逐字为准。
> 3. 全局约定（基线冻结项）：JSON 字段统一 snake_case；ID 统一带类型前缀（`tsk_` / `repo_` / `chk_` / `req_`，ULID 生成）；时间统一 UTC + RFC3339；所有业务 API 以 `/api/v1` 为前缀（`/metrics` 除外）。
> 4. 本阶段只搭骨架：handler / service / pipeline / provider 的业务实现一律是带明确 TODO 注释的占位，**但设计层内容（领域类型、接口、错误码、配置结构体、建库迁移 DDL、配置文件、docker-compose）必须完整落地**。
> 5. 全部代码（含注释中的标识符）用英文；TODO 注释可用中文。
> 6. v1 原方案（单机轻量栈：单文件库 / 进程内索引 / 纯 Go 库克隆 / 进程内队列与限流）整体作废，仅基线「冻结项」（REST 契约、状态机、事件协议、限流数值、ID/时间规范）保留；涉及基础设施的句子一律按本文档新措辞书写。

---

## 1. 你的角色与目标

你是一名资深 Go 工程师。请在目标目录创建 **DeepWiki(Go版)** 后端项目骨架：一个「Git 仓库异步摄取（git CLI 浅克隆→解析→切分→向量化→落库 PostgreSQL + pgvector）+ 语义检索问答（OpenSearch BM25 / pgvector HNSW / hybrid 三种可插拔检索 + 多 LLM 官方 SDK Provider，支持 SSE 流式）+ 异步 Wiki 生成」的企业级系统：任务队列走 RabbitMQ（瘦消息），限流 / 分布式锁 / 事件总线走 Redis 哨兵集群，运行时配置走 etcd。模块名（Go module path）为 **`deepwiki`**，语言版本 **Go 1.22+**。本阶段交付的是**工程骨架**：目录树、包声明、领域类型、冻结接口、错误码、配置结构体、建库迁移、docker-compose 基础设施、可启动的 HTTP 服务与健康检查；业务逻辑留 TODO，下一轮迭代分模块补齐。

---

## 2. 技术栈与依赖版本锁定表

依赖与《企业级技术栈变更总纲》§1 替换矩阵一致。**禁止**使用 `latest`；按下表写定版本，用 `go get 包@版本` 逐条锁定后再 `go mod tidy`（tidy 会自动补 indirect 行，禁止手工删除 indirect 行）。

| 领域 | 依赖 | 锁定版本 | 用途 |
|---|---|---|---|
| HTTP 框架 | `github.com/gin-gonic/gin` | **v1.10.0** | 路由、中间件、参数绑定 |
| WebSocket | `github.com/gorilla/websocket` | **v1.5.3** | `/api/v1/ws` |
| 日志 | `go.uber.org/zap` | **v1.27.0** | 结构化日志（JSON） |
| 配置引导 | `github.com/spf13/viper` | **v1.19.0** | 仅启动期 yaml + 环境变量引导加载，不参与运行时覆写 |
| 校验 | `github.com/go-playground/validator/v10` | **v10.22.1** | 请求与配置校验 |
| ID | `github.com/oklog/ulid/v2` | **v2.1.0** | `tsk_/repo_/chk_/req_` 主体 |
| 指标 | `github.com/prometheus/client_golang` | **v1.20.5** | `/metrics` |
| 关系/状态存储 | `github.com/jackc/pgx/v5` | **v5.7.1** | PostgreSQL 16 驱动与 `pgxpool` 连接池（全量状态存储） |
| 向量检索 | `github.com/pgvector/pgvector-go` | **v0.2.3** | pgvector 类型绑定（`vector(1536)` + HNSW + `<=>` 余弦距离） |
| 数据库迁移 | `github.com/golang-migrate/migrate/v4` | **v4.18.1** | embed.FS `iofs` source + `pgx` driver，只前进迁移 |
| 关键词检索 | `github.com/opensearch-project/opensearch-go/v4` | **v4.4.0** | OpenSearch 2.x 客户端（BM25，每仓一索引） |
| 任务队列 | `github.com/rabbitmq/amqp091-go` | **v1.10.0** | RabbitMQ 客户端（publisher confirm + manual ack + DLX） |
| Redis 客户端 | `github.com/redis/go-redis/v9` | **v9.7.0** | `FailoverClient` 接哨兵；限流 Lua / 分布式锁 / Streams 事件总线 / API key 缓存 |
| 动态配置 | `go.etcd.io/etcd/client/v3` | **v3.5.21** | etcd 读写 + watch 热更新 + Txn 原子多键写入 |
| LLM SDK | `github.com/openai/openai-go` | **v1.12.0** | OpenAI / DeepSeek / DashScope / SiliconFlow / VoyageAI（OpenAI 兼容端点统一走此 SDK，base_url 区分） |
| LLM SDK | `github.com/anthropics/anthropic-sdk-go` | **v1.5.0** | Claude 官方 SDK |
| LLM SDK | `google.golang.org/genai` | **v1.20.0** | Gemini 官方统一 GenAI SDK |
| LLM/Embedding SDK | `github.com/ollama/ollama` | **v0.11.4** | 仅用其 `api` 包（本地 Ollama） |
| 熔断 | `github.com/sony/gobreaker/v2` | **v2.3.0** | 每个 provider 实例一个熔断器（半开探测） |
| 限流兜底原语 | `golang.org/x/time` | **v0.5.0**（`rate` 包） | Redis 不可用时进程内降级兜底 |
| 链路追踪 | `go.opentelemetry.io/otel` + `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` | **v1.35.0** | OpenTelemetry Traces（OTLP gRPC，endpoint 为空则零成本禁用） |
| 忽略规则 | `github.com/sabhiram/go-gitignore` | **v1.0.0** | ingest 文件级 include/exclude 规则匹配（.gitignore 语义） |

**额外禁令（本轮）**：LLM / Embedding 的 10 个 provider **必须使用《变更总纲》§4.7 指定的官方 SDK**（openai-go / anthropic-sdk-go / google genai / ollama api），禁止退回标准库 `net/http` 手写 SSE/HTTP 客户端；且**任何 SDK 类型禁止出现在 `internal/model` 与各接口签名中**（硬约束 #17，依赖方向铁律）。禁止重新引入 v1 原方案已移除的存储 / 检索 / Git 组件依赖（含 langchaingo）。

**系统依赖**：`git ≥ 2.30`（git CLI，部署前置依赖；Docker 镜像内 `apk add git`，README 必须写明）。

---

## 3. 硬约束清单（反 AI 常见错误 18 条 · 命令式禁止句）

以下 18 条按《变更总纲》§9 转写（基线第 10 章 15 条更新措辞 + 新增 3 条），是你实现（含后续迭代）时的红线。每条给出违反后果。**你现在写下的每一个 TODO 注释，也必须复述对应约束**，使下一轮实现者无法绕过。

1. **限流粒度**：禁止实现全局单桶限流，禁止把进程内内存桶当作限流的唯一实现。必须实现 per-IP + per-API-key 两级限流：存储用 **Redis 分布式滑动窗口**（Lua 脚本原子执行；键 `rl:ip:<ip>`、`rl:key:<key_hash>:ingest|ask|wiki`）；窗口与配额数值冻结（L1：`per_ip.rps=10`、`per_ip.burst=20`，窗口 60s，`limit = rps*60 + burst`；L2：`ingest_per_hour=20`、`ask_per_minute=30`、`wiki_per_hour=10`）；**Redis 不可用时必须降级为进程内 `golang.org/x/time/rate` 兜底并记 WARN、health 置 degraded**（可用性优先的有意取舍）；命中限流必须返回 `429 + 42901 rate_limited`，响应头必带 `Retry-After` 与 `X-RateLimit-Limit / X-RateLimit-Remaining / X-RateLimit-Reset`。
   *违反后果：单用户刷接口即可拖垮全站，git/embedding/LLM 昂贵端点成本失控；多副本间限流不一致形同虚设。*
2. **密钥管理**：禁止在 `config.yaml`、Go 源码、测试夹具中硬编码任何 API key。密钥只从环境变量注入：`DEEPWIKI_API_KEYS`（逗号分隔）、`DEEPWIKI_ADMIN_KEY`、`DEEPWIKI_EMBEDDING_API_KEY`、`DEEPWIKI_LLM_API_KEY`；API key 落库（`api_keys` 表）**只存 `SHA-256(salt‖key)` 十六进制哈希**，认证走「Redis 缓存（60s）→ Postgres」二级查找；**禁止明文进入 Postgres / etcd / 日志**；日志与错误响应必须过滤 `Authorization` / `X-API-Key` 头与 `api_key` 字段；`GET /api/v1/config` 必须按"长度>8 → 前 3 字符 + `***` + 后 4 字符，否则全 `******`"脱敏返回。
   *违反后果：密钥随仓库或日志泄漏，属一票否决的安全事故。*
3. **任务状态存储**：禁止用内存 map 作为任务状态的唯一存储，禁止把任务状态 / 进度塞进 RabbitMQ 消息。**Postgres `tasks` 表是任务状态的唯一来源**：任务创建即在事务内写入 `pending`，之后每次状态 / 进度推进都更新该表；RabbitMQ 消息只携带 `task_id + type`（瘦消息，见第 16 条）；内存中只允许保存运行中任务的 `context.Context` 与 cancel 函数。
   *违反后果：进程 / 节点重启任务全部丢失且无法恢复，"重启可恢复"设计目标破产。*
4. **并发安全**：禁止裸 goroutine（无 `recover()`、无 ctx）。Worker、RabbitMQ consumer 回调与一切派生 goroutine 必须 `defer recover()`；git CLI / embedding / LLM / DB / MQ 全部 I/O 必须传入任务 ctx；Pipeline 每阶段入口与循环内必须 `select ctx.Done()`。
   *违反后果：一次 panic 拖垮整个进程；任务无法取消；外部调用句柄泄漏。*
5. **Git 更新**：禁止在 refresh 中使用 `git pull`，禁止用 `sh -c` 拼接 git 命令。必须用系统 git CLI（`exec.CommandContext`，参数以独立数组元素传递，env 强制 `GIT_TERMINAL_PROMPT=0`，单次操作挂 `context.WithTimeout`，`git.op_timeout` 默认 10m）：`git -C <dir> fetch --depth 1 origin <branch>` → `git -C <dir> reset --hard FETCH_HEAD` → `git -C <dir> clean -fdx`；任一步失败回退为重新 `git clone --depth 1` 到 `./data/repos/.tmp/<task_id>/` 后 `os.Rename` 原子切换。
   *违反后果：工作区脏状态与 merge 冲突直接卡死 pipeline；shell 拼接引入命令注入面。*
6. **并发上限**：禁止无限制 `go func()` 起后台任务。所有后台任务必须经 `TaskManager.Submit` → RabbitMQ 有界队列（`x-max-length = worker.queue_size`，默认 100）→ 每 worker 节点有界 Worker Pool（`worker.pool_size`，默认 2，`prefetch = pool_size`）；投递前必须 `QueueDeclarePassive` 探测队列深度，深度 ≥ `x-max-length` 必须返回 `429 + 42902 queue_full + Retry-After`；publisher confirm 失败必须把任务标记 `failed`（`50302 queue_unavailable`）并返回 503。
   *违反后果：大仓库并发打爆 broker / 内存 / 文件句柄 / 上游配额。*
7. **外部调用韧性**：禁止无超时无重试的外部调用，禁止固定间隔重试。每次调用由调用方统一挂 `context.WithTimeout`；**重试优先用官方 SDK 内置机制**（openai-go 默认对 429/5xx 指数退避并尊重 `Retry-After`）；SDK 无内置重试者（ollama）必须外包一层手写指数退避 `backoff × 2^n` + ±20% 抖动，仅对网络错误 / 429 / 5xx 重试，4xx（非 429）不重试；**每个 provider 实例必须套 `gobreaker` 熔断器**：连续失败 ≥5 → open 60s → half-open 单探测 → 关闭，状态反映到 health（degraded）。
   *违反后果：上游故障被重试风暴放大，级联雪崩。*
8. **错误响应**：禁止把 `err.Error()` 原文回传给客户端。必须统一错误信封 `{code, message, request_id, details}`；原始错误只进 zap 日志；message 用脱敏后的固定文案。
   *违反后果：泄漏服务器路径、SQL 语句、密钥片段。*
9. **配置热更新原子性**：禁止"改一半生效一半失败"。`PUT /api/v1/config` 必须：Merge Patch 合并 → 全量校验（validator tag + 跨字段 `chunk_overlap ≤ chunk_size/2`、`per_ip.burst ≥ per_ip.rps` + embedding 维度探测）→ 校验失败整体拒绝（42201）保持旧值 → 成功则以**单条 etcd `Txn`** 原子写入（`/deepwiki/config/*` overrides + `/deepwiki/config_version` +1 + `/deepwiki/audit/<version>` 审计记录，三者同一事务）→ watch 回调保证本节点与其他节点同步热生效；etcd 写路径不可用返回 `50304 config_store_unavailable`。
   *违反后果：运行中配置被改坏、多节点配置不一致，系统进入不可预知状态。*
10. **优雅退出**：禁止收到 SIGTERM 直接 `os.Exit`。必须：readiness 置失败（health 返回 `503 + 50301`）→ 停接新请求 → `Channel.Cancel` 停拉新消息 → 等 worker 排空（上限 `server.shutdown_timeout`）→ 在途未完成任务 **nack `requeue=true`** 让其他节点接走，无法重投者落库 `failed / 50004 task_interrupted` → flush 日志 → 关 EventBus（Redis 订阅）→ 关 RabbitMQ 通道与连接 → 关 Redis / Postgres 连接池。
    *违反后果：任务半截、消息丢失、连接泄漏。*
11. **输入安全**：禁止字符串拼接 SQL，禁止用用户传入的 id 直接拼文件路径。SQL 必须全部参数化（**Postgres `$n` 占位**）；`repo_id` / `task_id` 必须先过"前缀 + ULID 正则"（`^(tsk_|repo_|chk_)[0-9A-HJKMNP-TV-Z]{26}$`）再查库 / 拼路径；chunk 路径必须 `filepath.Clean` 后校验仍在仓库根内、禁止 `..` 与绝对路径。
    *违反后果：SQL 注入与路径穿越，一票否决。*
12. **CORS**：禁止写死 `Access-Control-Allow-Origin: *`。必须仅放行 `server.cors_allowed_origins` 白名单（配置校验必须拒绝 `*`）；预检 `OPTIONS` 由 CORS 中间件直接应答（204），不进入 Auth。
    *违反后果：任意站点可携带凭据跨域调用本服务。*
13. **时间格式**：禁止本地时区与混合时间格式。全链路必须 UTC + RFC3339（`time.Now().UTC().Format(time.RFC3339)`）；落库为 **Postgres `timestamptz` 列**，API 响应同格式，不做时区转换。
    *违反后果：前后端时间解析错乱、排序与分页错误。*
14. **向量维度一致性**：禁止更换 embedding provider/model 后静默用新维度查旧索引。启动时与 `PUT /config` 时必须做**应用层维度探测**（provider/model/base_url 变更且库中有 chunks → `Embed(["dimension probe"])` 比对）：不一致拒绝配置变更（42201）/ health 置 degraded 并提示重建索引；**DB 层 `chunks.embedding vector(1536)` 列类型是第二道防线**，直接拒绝维度不符写入；`chunks.embedding_model` 列必须记录向量来源；改维度 = 新迁移 + 全量重建。
    *违反后果：检索结果全是垃圾且无任何报错。*
15. **引用真实性**：禁止从 LLM 输出中解析或编造 references。references 必须只取 Retriever 真实返回的 hits，响应装配前逐一校验 `chunk_id` 存在；system prompt 必须约束 LLM 只能引用给定片段、禁止编造行号，片段不足时回答"未在仓库中找到相关代码"。
    *违反后果：回答引用不存在的文件与行号，核心功能失去可信度。*
16. **瘦消息（新增）**：禁止把任务 payload、文件内容、chunks、向量等大对象塞进 RabbitMQ 消息。消息体必须 ≤ 4KB，只含 `{"task_id":"tsk_...","type":"ingest|refresh|wiki"}`；任务状态、进度、错误一律读写 Postgres `tasks` 表。
    *违反后果：消息堆积拖垮 broker，投递延迟雪崩，队列失去削峰意义。*
17. **SDK 隔离（新增）**：禁止任何官方 SDK 类型（openai-go / anthropic-sdk-go / google genai / ollama api / pgvector-go / opensearch-go / amqp091-go 等）出现在 `internal/model` 与 `LLM / Embedder / Retriever / Cloner / Store` 等接口签名中；adapter 内部必须完成 SDK 类型 → 领域类型的转换。
    *违反后果：更换 provider 要改动业务层，分层依赖铁律破产。*
18. **幂等消费（新增）**：禁止假设 RabbitMQ 消息恰好一次到达（at-least-once 语义）。消费路径必须先 CAS 抢占任务：`UPDATE tasks SET state='cloning' ... WHERE task_id=$1 AND state='pending'`，CAS 失败（已被其他节点或重投执行）直接 ack 丢弃；终态落库后才 ack；panic/recover 或瞬时错误 nack `requeue=false` 进 DLX 重试链（最多 3 次），重试耗尽落库 `failed（50003）`。
    *违反后果：多节点重复执行同一任务，写放大、状态错乱、上游费用翻倍。*

---

## 4. 完整目录树（必须包含且仅包含以下约 90 个文件，精确计数 96 个）

与《变更总纲》§8 新包结构逐字对齐。新增包：`queue / search / ratelimit / lock / observability`；`store` 包调整为 pgx 实现并新增 `apikey_store.go`（原 `config_store.go` 移除，配置覆写迁往 etcd，见 `internal/config/etcd_source.go`）；`ingest` 新增 `ignore.go`；`config` 拆分为 `loader.go / etcd_source.go`；`eventbus` 拆分为 `redis_stream.go / redis_fanout.go`；根目录新增 `docker-compose.yml / Dockerfile / deploy/`。

```text
deepwiki/
├── go.mod
├── README.md
├── Makefile
├── .gitignore
├── .env.example
├── docker-compose.yml           # 一键基础设施：postgres(pgvector)/opensearch/rabbitmq/redis 哨兵×3+主从/etcd
├── Dockerfile                   # 多阶段构建；运行镜像必须 apk add git（git CLI 系统依赖）
├── cmd/
│   └── server/
│       └── main.go
├── configs/
│   └── config.yaml
├── migrations/
│   ├── 000001_init.up.sql       # golang-migrate 命名（只有 .up 没有 .down，只前进）；含 CREATE EXTENSION vector
│   └── migrations.go            # embed.FS 导出（go:embed 不允许跨目录引用，必须放在此）
├── deploy/
│   └── redis-sentinel/
│       └── sentinel.conf        # 哨兵配置模板（compose 三哨兵挂载用）
├── data/
│   └── .gitkeep
├── docs/
│   └── README.md                # API 契约占位说明
└── internal/
    ├── model/                   # 领域类型与错误码（最内层，只依赖标准库；20 个错误码）
    │   ├── task.go
    │   ├── repo.go
    │   ├── chunk.go
    │   ├── chat.go
    │   ├── event.go
    │   └── errors.go
    ├── embed/                   # Embedder 接口 + 5 provider 官方 SDK 骨架 + 工厂
    │   ├── embedder.go
    │   ├── openai.go            # openai-go（base_url 可配）
    │   ├── dashscope.go         # openai-go + 百炼兼容端点 base_url
    │   ├── siliconflow.go       # openai-go + SiliconFlow base_url
    │   ├── voyage.go            # openai-go + VoyageAI base_url
    │   ├── ollama.go            # ollama api 包
    │   └── factory.go
    ├── llm/                     # LLM 接口 + 5 provider 官方 SDK 骨架（各内嵌 gobreaker）+ 工厂
    │   ├── llm.go
    │   ├── openai.go            # openai-go
    │   ├── deepseek.go          # openai-go + DeepSeek base_url
    │   ├── claude.go            # anthropic-sdk-go
    │   ├── gemini.go            # google.golang.org/genai
    │   ├── ollama.go            # ollama api 包
    │   └── factory.go
    ├── retriever/               # Retriever 接口 + keyword/vector/hybrid/rerank 骨架
    │   ├── retriever.go
    │   ├── keyword.go           # OpenSearch BM25（每仓一索引）
    │   ├── vector.go            # pgvector HNSW（检索 SQL 唯一实现处）
    │   ├── hybrid.go
    │   └── rerank.go
    ├── ingest/                  # Cloner 接口 + git CLI 实现、Pipeline 类型、Parser、Chunker、忽略规则
    │   ├── pipeline.go
    │   ├── cloner.go            # git CLI：exec.CommandContext，禁止 sh -c / git pull
    │   ├── parser.go
    │   ├── chunker.go
    │   └── ignore.go            # go-gitignore 规则匹配 include/exclude + 默认跳过清单
    ├── store/                   # PostgreSQL 存储层（pgx 实现骨架；config_store.go 移除，迁往 etcd）
    │   ├── postgres.go          # pgxpool 连接封装（MaxConns/MinConns/HealthCheckPeriod/Ping/WithTx）
    │   ├── migrate.go           # golang-migrate Up() 封装（dirty panic）
    │   ├── repo_store.go
    │   ├── chunk_store.go
    │   ├── vector_store.go      # pgvector 实现（InsertBatch/pgvector 类型/检索 SQL）
    │   ├── wiki_store.go
    │   └── apikey_store.go      # API key 哈希存储（SHA-256(salt‖key)）
    ├── queue/                   # RabbitMQ（新增包）
    │   ├── rabbitmq.go          # 连接 + 拓扑声明（exchange/队列/DLX/重试队列）
    │   ├── publisher.go         # mandatory + publisher confirm + 深度探测
    │   ├── consumer.go          # prefetch + manual ack/nack + CAS 抢占
    │   └── reconciler.go        # 启动恢复：非终态任务重投
    ├── search/                  # OpenSearch（新增包）
    │   └── opensearch.go        # 客户端 + 索引生命周期（建索引/删索引/count 校验/bulk）
    ├── ratelimit/               # 分布式限流（新增包）
    │   ├── limiter.go           # Limiter 接口
    │   ├── redis_lua.go         # Lua 滑动窗口脚本（内嵌）
    │   └── fallback.go          # x/time/rate 进程内降级兜底
    ├── lock/                    # 分布式锁（新增包）
    │   └── redis_lock.go        # SET NX PX + Lua 校验 token 解锁
    ├── task/                    # 统一任务系统（ingest/refresh/wiki 共用）
    │   ├── manager.go           # Submit → 投递 RabbitMQ（瘦消息）
    │   ├── pool.go
    │   ├── executor.go
    │   └── store.go             # tasks 表读写（Postgres，状态唯一来源）
    ├── eventbus/
    │   ├── bus.go               # EventBus 接口（不变）
    │   ├── redis_stream.go      # XADD + XTRIM ~1000 + XRANGE 回放
    │   └── redis_fanout.go      # Pub/Sub events:fanout 跨节点扇出
    ├── config/
    │   ├── config.go            # 配置结构体全集
    │   ├── loader.go            # viper 引导（yaml+env，仅基础设施坐标）
    │   ├── etcd_source.go       # etcd 读写 + watch 热更新 + Txn
    │   └── manager.go
    ├── observability/           # 可观测性（新增包）
    │   ├── metrics.go           # prometheus 指标注册（含 MQ/Redis/OpenSearch/etcd/pgx 池指标）
    │   └── tracing.go           # OpenTelemetry 初始化（OTLP，endpoint 空则禁用）
    ├── service/                 # 业务编排层（面向接口）
    │   ├── ask_service.go
    │   ├── wiki_service.go
    │   ├── ingest_service.go
    │   └── repo_service.go
    └── api/                     # 接入层 + Handler 层
        ├── router.go
        ├── dto/
        │   ├── ingest.go
        │   ├── ask.go
        │   ├── config.go
        │   ├── pagination.go
        │   └── health.go
        ├── middleware/
        │   ├── requestid.go
        │   ├── recovery.go
        │   ├── auth.go          # apikey_store + Redis 缓存二级查找
        │   ├── ratelimit.go     # 调用 ratelimit 包（Redis Lua + 降级）
        │   └── cors.go
        └── handler/
            ├── health.go
            ├── ingest.go
            ├── repo.go
            ├── task.go
            ├── ask.go
            ├── wiki.go
            ├── config.go
            └── events.go
```

分层依赖铁律（基线 §2.2 + 《变更总纲》§8 末段，禁止违反）：`model` 在最内层，不 import 任何 internal 包；`embed / llm / retriever / ingest / store / queue / search / ratelimit / lock / task / eventbus / config / observability` 只 import `model` 与其下方包；`service` 只 import 上述接口包与 `model`；`api` 在最外层装配一切。禁止出现 import 环。

---

## 5. 逐文件创建指令

以下按包分组。每个文件给出：路径、包名与职责、必须包含的内容。【完整代码】逐字符照抄；【骨架 TODO】按签名创建、函数体保留 TODO。本文件 §5.1 ~ §5.13 连续完整，§5.7 起（task / queue / search / ratelimit / lock / eventbus / config / observability / service / api / cmd / 根目录文件）见下文。

### 5.1 internal/model —— 领域类型与错误码（全部【完整代码】）

包内所有文件 `package model`，只允许 import 标准库。与基线 §4.2、§4.3、§5、§7 逐字符一致，禁止改字段名、改 json tag；错误码在 v1 冻结 16 个基础上按《变更总纲》§6 **追加 4 个基础设施错误码**（共 20 个）。时间字段落库映射 Postgres `timestamptz` 列，API 输出仍为 UTC + RFC3339（硬约束 #13）。

**① `internal/model/task.go`** — 任务领域类型、状态机常量与合法转移表。

```go
// Package model 领域类型与错误码。最内层，禁止 import 任何 internal 包与 provider SDK。
package model

import (
	"encoding/json"
	"time"
)

// TaskType 任务类型（基线 §4.2，冻结）。
type TaskType string

const (
	TaskTypeIngest  TaskType = "ingest"
	TaskTypeRefresh TaskType = "refresh"
	TaskTypeWiki    TaskType = "wiki"
)

// TaskState 任务状态（基线 §4.3，冻结）。任务生命周期只由 state 一个字段表达，
// 禁止另设 status/phase 等二义字段。
type TaskState string

const (
	TaskStatePending    TaskState = "pending"
	TaskStateCloning    TaskState = "cloning"
	TaskStateParsing    TaskState = "parsing"
	TaskStateChunking   TaskState = "chunking"
	TaskStateEmbedding  TaskState = "embedding"
	TaskStatePersisting TaskState = "persisting"
	TaskStateOutlining  TaskState = "outlining"  // wiki 专用
	TaskStateGenerating TaskState = "generating" // wiki 专用
	TaskStateFetching   TaskState = "fetching"   // refresh 专用
	TaskStateDiffing    TaskState = "diffing"    // refresh 专用
	TaskStateCompleted  TaskState = "completed"
	TaskStateFailed     TaskState = "failed"
	TaskStateCancelled  TaskState = "cancelled"
)

// IsTerminal 终态：completed / failed / cancelled。进入终态必须写 finished_at 且不可再转移。
func (s TaskState) IsTerminal() bool {
	return s == TaskStateCompleted || s == TaskStateFailed || s == TaskStateCancelled
}

// validTransitions 合法状态转移表（基线 §4.3 三类状态机的并集，冻结）。
var validTransitions = map[TaskState][]TaskState{
	TaskStatePending:    {TaskStateCloning, TaskStateOutlining, TaskStateFetching, TaskStateCancelled},
	TaskStateCloning:    {TaskStateParsing, TaskStateCancelled, TaskStateFailed},
	TaskStateParsing:    {TaskStateChunking, TaskStateCancelled, TaskStateFailed},
	TaskStateChunking:   {TaskStateEmbedding, TaskStateCancelled, TaskStateFailed},
	TaskStateEmbedding:  {TaskStatePersisting, TaskStateCancelled, TaskStateFailed},
	TaskStatePersisting: {TaskStateCompleted, TaskStateCancelled, TaskStateFailed},
	TaskStateOutlining:  {TaskStateGenerating, TaskStateCancelled, TaskStateFailed},
	TaskStateGenerating: {TaskStateCompleted, TaskStateCancelled, TaskStateFailed},
	TaskStateFetching:   {TaskStateDiffing, TaskStateCancelled, TaskStateFailed},
	TaskStateDiffing:    {TaskStateChunking, TaskStateCancelled, TaskStateFailed},
	TaskStateCompleted:  {},
	TaskStateFailed:     {},
	TaskStateCancelled:  {},
}

// CanTransition 状态机转移校验；TaskStore.UpdateState 必须调用它，非法转移返回 40902。
func CanTransition(from, to TaskState) bool {
	for _, s := range validTransitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// Progress 阶段内进度；Total=0 表示阶段总量未知（不确定进度条）。
type Progress struct {
	Current int `json:"current"`
	Total   int `json:"total"`
	Percent int `json:"percent"` // 0~100，阶段内
}

// Stats 随阶段累计；无数据时全 0。
type Stats struct {
	Files  int `json:"files"`
	Chunks int `json:"chunks"`
	Tokens int `json:"tokens"`
}

// TaskError 失败时写入；正常/运行中为 null。
type TaskError struct {
	Code    int    `json:"code"`    // 复用统一错误码空间（如 50004）
	Message string `json:"message"` // 脱敏后的面向用户描述
	Stage   string `json:"stage"`   // 失败时所处的 state
}

// Task 任务全字段（基线 §4.2）。CancelFlag / RequestPayload 不落 API 响应。
// 时间字段（CreatedAt/StartedAt/FinishedAt）落库为 Postgres timestamptz 列，
// API 输出统一 UTC + RFC3339（硬约束 #13）。
type Task struct {
	TaskID         string          `json:"task_id"`
	Type           TaskType        `json:"type"`
	RepoID         string          `json:"repo_id"`
	State          TaskState       `json:"state"`
	Progress       Progress        `json:"progress"`
	Stats          Stats           `json:"stats"`
	Err            *TaskError      `json:"error"`
	QueuePosition  int             `json:"queue_position"`
	CancelFlag     bool            `json:"-"`
	RequestPayload json.RawMessage `json:"-"`
	CreatedAt      time.Time       `json:"created_at"`
	StartedAt      *time.Time      `json:"started_at"`
	FinishedAt     *time.Time      `json:"finished_at"`
}

// TaskFilter 任务列表过滤（§6.7：type/state/repo_id + 分页）。
type TaskFilter struct {
	Type     *TaskType
	State    *TaskState
	RepoID   string
	Page     int
	PageSize int
}

// TaskPatch 增量更新补丁；指针字段为 nil 表示不更新。Err 与 ClearErr 互斥。
type TaskPatch struct {
	State         *TaskState
	Progress      *Progress
	Stats         *Stats
	Err           *TaskError
	ClearErr      bool
	QueuePosition *int
	StartedAt     *time.Time
	FinishedAt    *time.Time
}
```

**② `internal/model/repo.go`** — 仓库领域类型。

```go
package model

import "time"

// 仓库状态（基线 §12.2 CHECK 约束，冻结）。
const (
	RepoStateIngesting = "ingesting"
	RepoStateReady     = "ready"
	RepoStateError     = "error"
)

// Repo 仓库。LocalPath 不落 API 响应。
// CreatedAt/UpdatedAt 落库为 Postgres timestamptz 列，API 输出 UTC + RFC3339（硬约束 #13）。
type Repo struct {
	RepoID     string    `json:"repo_id"`
	RepoURL    string    `json:"repo_url"`
	Branch     string    `json:"branch"`
	CommitHash string    `json:"commit_hash"`
	LocalPath  string    `json:"-"`
	State      string    `json:"state"` // ingesting|ready|error
	Stats      RepoStats `json:"stats"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type RepoStats struct {
	Files  int `json:"files"`
	Chunks int `json:"chunks"`
	Tokens int `json:"tokens"`
}
```

**③ `internal/model/chunk.go`** — 分块与检索命中。

```go
package model

// Chunk 代码/文档分块领域模型（基线 §7）。
// Path 为仓库内相对路径，禁止 .. 与绝对路径（反 AI 错误 #11）。
// Vector 在检索路径中可不填充（按 chunk_id 懒加载）；落库为 chunks.embedding vector(1536) 列，
// 维度不符会被列类型直接拒绝（反 AI 错误 #14 第二道防线）。
type Chunk struct {
	ChunkID        string
	RepoID         string
	Path           string
	StartLine      int
	EndLine        int
	Language       string
	Content        string
	Vector         []float32
	FileHash       string // 所属文件 sha256 前 16 位，refresh diff 用
	EmbeddingModel string // 产出向量的 provider/model，维度一致性校验用（反 AI 错误 #14）
}

// ChunkHit 检索命中。Score 语义：keyword=BM25 分；embedding=余弦相似度 [0,1]；hybrid=RRF 融合分。
type ChunkHit struct {
	Chunk Chunk
	Score float64
}
```

**④ `internal/model/chat.go`** — LLM 对话类型（流式与非流式共用 ChatRequest，基线 §7）。

```go
package model

// ChatMessage 角色：system|user|assistant。
type ChatMessage struct {
	Role    string
	Content string
}

// ChatRequest 流式与非流式共用（建议⑧）；是否流式由调用 LLM 的哪个方法决定，结构体不含 stream 字段。
type ChatRequest struct {
	Model       string
	Messages    []ChatMessage
	Temperature float64
	MaxTokens   int
}

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type ChatResponse struct {
	Content      string
	Model        string
	Usage        Usage
	FinishReason string // stop|length|content_filter|...
}

// StreamChunk 流式输出元素；消费方必须先检查 Err。
type StreamChunk struct {
	Delta        string  // 增量文本
	FinishReason string  // 非空表示结束原因
	Usage        *Usage  // 仅结束 chunk 可能携带（provider 支持时）
	Err          error   // 非 nil 表示流内错误，此后 channel 将被关闭
}
```

**⑤ `internal/model/event.go`** — EventBus 事件与过滤器（基线 §6.4、§7）。

```go
package model

import (
	"encoding/json"
	"time"
)

// 事件类型（基线 §6.4，冻结；gap 为断线补发空洞提示）。
const (
	EventTypeTaskStateChanged = "task.state_changed"
	EventTypeTaskProgress     = "task.progress"
	EventTypeWikiCompleted    = "wiki.completed"
	EventTypeGap              = "gap"
)

// Event 统一事件。Seq 单调递增，是 SSE id / Last-Event-ID 的依据；
// 物理载体为 Redis Streams（events:task:<task_id>，XTRIM MAXLEN ~ 1000）。
// Timestamp 落库/落流均为 UTC + RFC3339（硬约束 #13）。
type Event struct {
	Seq       uint64          `json:"seq"`
	Type      string          `json:"type"`
	RepoID    string          `json:"repo_id"`
	TaskID    string          `json:"task_id"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// EventFilter 订阅过滤；空 = 全部。
type EventFilter struct {
	Types  []string
	RepoID string
}
```

**⑥ `internal/model/errors.go`** — 统一信封、20 个错误码常量、哨兵错误（v1 冻结 16 个与基线 §5.2、§5.3 逐字符一致；v2 按《变更总纲》§6 追加 4 个基础设施错误码，禁止再增删）。

```go
package model

import (
	"errors"
	"fmt"
)

// ---------------- 错误码（基线 §5.3 冻结 16 个 + 变更总纲 §6 新增 4 个 = 固定 20 个，不得增删） ----------------

const (
	CodeInvalidParam           = 40001 // invalid_param
	CodeUnauthorized           = 40101 // unauthorized
	CodeForbidden              = 40301 // forbidden
	CodeTaskNotFound           = 40401 // task_not_found
	CodeRepoNotFound           = 40402 // repo_not_found
	CodeWikiNotFound           = 40403 // wiki_not_found
	CodeRepoAlreadyExists      = 40901 // repo_already_exists
	CodeInvalidTaskState       = 40902 // invalid_task_state
	CodeConfigValidationFailed = 42201 // config_validation_failed
	CodeRateLimited            = 42901 // rate_limited
	CodeQueueFull              = 42902 // queue_full
	CodeInternalError          = 50001 // internal_error
	CodeTaskInterrupted        = 50004 // task_interrupted（仅出现在 task.error，不直接作为 API 响应码）
	CodeLLMUnavailable         = 50201 // llm_unavailable
	CodeEmbeddingUnavailable   = 50202 // embedding_unavailable
	CodeServiceNotReady        = 50301 // service_not_ready
	// ---- v2 新增（变更总纲 §6，基础设施错误码）----
	CodeVectorStoreUnavailable  = 50203 // vector_store_unavailable：Postgres/pgvector 查询失败（ask embedding 路径）
	CodeQueueUnavailable        = 50302 // queue_unavailable：RabbitMQ 连接/发布确认失败
	CodeSearchUnavailable       = 50303 // search_unavailable：OpenSearch 不可用且影响 ask
	CodeConfigStoreUnavailable  = 50304 // config_store_unavailable：etcd 写路径不可用（GET 走缓存不报错）
)

// 错误码名称（message 前缀用，如 "invalid_param: field question length must be between 1 and 4000"）。
const (
	ErrNameInvalidParam           = "invalid_param"
	ErrNameUnauthorized           = "unauthorized"
	ErrNameForbidden              = "forbidden"
	ErrNameTaskNotFound           = "task_not_found"
	ErrNameRepoNotFound           = "repo_not_found"
	ErrNameWikiNotFound           = "wiki_not_found"
	ErrNameRepoAlreadyExists      = "repo_already_exists"
	ErrNameInvalidTaskState       = "invalid_task_state"
	ErrNameConfigValidationFailed = "config_validation_failed"
	ErrNameRateLimited            = "rate_limited"
	ErrNameQueueFull              = "queue_full"
	ErrNameInternalError          = "internal_error"
	ErrNameTaskInterrupted        = "task_interrupted"
	ErrNameLLMUnavailable         = "llm_unavailable"
	ErrNameEmbeddingUnavailable   = "embedding_unavailable"
	ErrNameServiceNotReady        = "service_not_ready"
	// ---- v2 新增 ----
	ErrNameVectorStoreUnavailable = "vector_store_unavailable"
	ErrNameQueueUnavailable       = "queue_unavailable"
	ErrNameSearchUnavailable      = "search_unavailable"
	ErrNameConfigStoreUnavailable = "config_store_unavailable"
)

// HTTPStatusOf 错误码 → HTTP 状态码映射（基线 §5.3 + 变更总纲 §6）。
func HTTPStatusOf(code int) int {
	switch code {
	case CodeInvalidParam:
		return 400
	case CodeUnauthorized:
		return 401
	case CodeForbidden:
		return 403
	case CodeTaskNotFound, CodeRepoNotFound, CodeWikiNotFound:
		return 404
	case CodeRepoAlreadyExists, CodeInvalidTaskState:
		return 409
	case CodeConfigValidationFailed:
		return 422
	case CodeRateLimited, CodeQueueFull:
		return 429
	case CodeLLMUnavailable, CodeEmbeddingUnavailable, CodeVectorStoreUnavailable:
		return 502
	case CodeServiceNotReady, CodeQueueUnavailable, CodeSearchUnavailable, CodeConfigStoreUnavailable:
		return 503
	default:
		return 500
	}
}

// MessageOf 错误码默认文案（脱敏固定文案；禁止向客户端回传 err.Error() 原文，反 AI 错误 #8）。
// v1 冻结 16 个保持英文固定文案；v2 新增 4 个按变更总纲 §6 message 列逐字使用中文文案。
func MessageOf(code int) string {
	switch code {
	case CodeInvalidParam:
		return "invalid param"
	case CodeUnauthorized:
		return "unauthorized"
	case CodeForbidden:
		return "forbidden"
	case CodeTaskNotFound:
		return "task not found"
	case CodeRepoNotFound:
		return "repo not found"
	case CodeWikiNotFound:
		return "wiki not found"
	case CodeRepoAlreadyExists:
		return "repo already exists"
	case CodeInvalidTaskState:
		return "invalid task state"
	case CodeConfigValidationFailed:
		return "config validation failed"
	case CodeRateLimited:
		return "rate limited"
	case CodeQueueFull:
		return "queue full"
	case CodeTaskInterrupted:
		return "task interrupted"
	case CodeLLMUnavailable:
		return "llm unavailable"
	case CodeEmbeddingUnavailable:
		return "embedding unavailable"
	case CodeServiceNotReady:
		return "service not ready"
	case CodeQueueUnavailable:
		return "任务队列暂不可用，请稍后重试"
	case CodeSearchUnavailable:
		return "检索服务暂不可用"
	case CodeConfigStoreUnavailable:
		return "配置中心暂不可用"
	case CodeVectorStoreUnavailable:
		return "向量检索暂不可用"
	default:
		return "internal error"
	}
}

// ---------------- 统一响应信封（基线 §5.2，冻结） ----------------

// ErrorDetail 字段级明细；ExistingRepoID 仅 40901 幂等命中时使用。
type ErrorDetail struct {
	Field          string `json:"field"`
	Issue          string `json:"issue"`
	ExistingRepoID string `json:"existing_repo_id,omitempty"`
}

// Envelope 统一响应信封。成功：code=0,message="ok",data 非空；失败：code!=0,details 可选。
type Envelope struct {
	Code      int           `json:"code"`
	Message   string        `json:"message"`
	Data      any           `json:"data,omitempty"`
	RequestID string        `json:"request_id"`
	Details   []ErrorDetail `json:"details,omitempty"`
}

// APIError 业务错误；Handler 层据此装配信封。Err 为原始错误，只进 zap 日志，禁止回传。
type APIError struct {
	Code    int
	Message string
	Details []ErrorDetail
	Err     error
}

func (e *APIError) Error() string { return fmt.Sprintf("api error %d: %s", e.Code, e.Message) }
func (e *APIError) Unwrap() error { return e.Err }

// NewAPIError 用默认文案构造（message 可传空串取默认）。
func NewAPIError(code int, message string) *APIError {
	if message == "" {
		message = MessageOf(code)
	}
	return &APIError{Code: code, Message: message}
}

// ---------------- 哨兵错误（跨层传递，Handler 映射为错误码） ----------------

var (
	ErrQueueFull         = errors.New("queue full")          // → 42902
	ErrInvalidTaskState  = errors.New("invalid task state")  // → 40902
	ErrTaskNotFound      = errors.New("task not found")      // → 40401
	ErrRepoNotFound      = errors.New("repo not found")      // → 40402
	ErrWikiNotFound      = errors.New("wiki not found")      // → 40403
	ErrRepoAlreadyExists = errors.New("repo already exists") // → 40901
	// ---- v2 新增（与新错误码配套）----
	ErrVectorStoreUnavailable = errors.New("vector store unavailable")  // → 50203
	ErrQueueUnavailable       = errors.New("queue unavailable")         // → 50302
	ErrSearchUnavailable      = errors.New("search unavailable")        // → 50303
	ErrConfigStoreUnavailable = errors.New("config store unavailable")  // → 50304
)
```

---

### 5.2 internal/embed —— Embedder 接口、5 个 provider 官方 SDK 骨架与工厂

10 个 provider 的 SDK 选型以《变更总纲》§4.7「provider → SDK 映射（权威）」为准：embedding 侧 openai / dashscope / siliconflow / voyage 四家均为 OpenAI 兼容端点，统一走 **openai-go**（仅 base_url 不同）；ollama 走 **ollama `api` 包**。所有 adapter 内嵌 `gobreaker` 熔断器字段（构造时注入，硬约束 #7）；SDK 类型禁止外泄到接口签名（硬约束 #17）。

**① `internal/embed/embedder.go`**【完整代码】— 向量模型抽象（基线 §7，冻结签名）。实现：OpenAI / DashScope / SiliconFlow / VoyageAI / Ollama。

```go
// Package embed 向量模型 Provider 抽象与实现。
package embed

import "context"

// Embedder 向量模型抽象（基线 §7，冻结签名）。
// 任何官方 SDK 类型禁止出现在本签名与返回值中（硬约束 #17）。
type Embedder interface {
	// Embed 批量向量化；实现方内部按配置 batch_size 分批、带超时与 SDK 内置/外包指数退避重试（硬约束 #7）。
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int      // 向量维度（配置探测/构造时确定，运行期不可变）
	ProviderName() string // openai|dashscope|siliconflow|ollama|voyage
	ModelName() string
}
```

**② `internal/embed/openai.go`**【骨架 TODO】— OpenAI 实现骨架（openai-go 官方客户端，base_url 可配）。

```go
package embed

import (
	"context"

	"deepwiki/internal/config"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
)

// openAIDims 已知模型维度表；未知模型 dims=0，下一轮实现时探测（基线 §8.3）。
var openAIDims = map[string]int{
	"text-embedding-3-small": 1536,
	"text-embedding-3-large": 3072,
	"text-embedding-ada-002": 1536,
}

// OpenAIEmbedder 基于官方 SDK openai-go 的 OpenAI 向量实现骨架（变更总纲 §4.7）。
type OpenAIEmbedder struct {
	cfg     config.EmbeddingConfig
	dims    int
	client  openai.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

// NewOpenAIEmbedder 构造 OpenAI 向量 adapter；breaker 由工厂注入（每 provider 实例一个，硬约束 #7）。
func NewOpenAIEmbedder(cfg config.EmbeddingConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *OpenAIEmbedder {
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	return &OpenAIEmbedder{
		cfg:     cfg,
		dims:    openAIDims[cfg.Model],
		client:  openai.NewClient(opts...),
		breaker: breaker,
		logger:  logger,
	}
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	// TODO: 用 openai-go 调用 client.Embeddings.New(ctx, openai.EmbeddingNewParams{...})，要求：
	// ① 整个调用必须经 e.breaker.Execute 包裹（硬约束 #7：连续失败 ≥5 → open 60s → half-open 单探测，状态反映到 health）；
	// ② 按 cfg.BatchSize 分批；ctx 超时由调用方统一 context.WithTimeout 包裹（硬约束 #7）；
	// ③ 重试优先用 SDK 内置机制（openai-go 默认对 429/5xx 指数退避并尊重 Retry-After），禁止再手写固定间隔重试；
	// ④ API key 仅经 option.WithAPIKey 注入，禁止打印密钥到日志（硬约束 #2）；
	// ⑤ SDK 返回类型必须在本方法内转换为 [][]float32，禁止外泄到签名（硬约束 #17）；
	// ⑥ 重试耗尽 / breaker open 映射哨兵错误，由上层转 50202 embedding_unavailable。
	panic("TODO: OpenAIEmbedder.Embed not implemented")
}

func (e *OpenAIEmbedder) Dimensions() int      { return e.dims }
func (e *OpenAIEmbedder) ProviderName() string { return "openai" }
func (e *OpenAIEmbedder) ModelName() string    { return e.cfg.Model }

var _ Embedder = (*OpenAIEmbedder)(nil)
```

**③ `internal/embed/dashscope.go`**【骨架 TODO】— DashScope（阿里云百炼）实现骨架：复用 openai-go 客户端，base_url 默认 `https://dashscope.aliyuncs.com/compatible-mode/v1`（变更总纲 §4.7 映射表）。

```go
package embed

import (
	"context"

	"deepwiki/internal/config"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
)

// dashscopeDefaultBaseURL 阿里云百炼 DashScope OpenAI 兼容端点（变更总纲 §4.7，逐字）。
const dashscopeDefaultBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"

// dashscopeDims 已知模型维度表；下一轮按官方文档补全，未知模型 dims=0 时探测。
var dashscopeDims = map[string]int{
	"text-embedding-v3": 1024,
	"text-embedding-v2": 1536,
}

// DashScopeEmbedder 复用 openai-go 客户端的 DashScope 向量实现骨架（OpenAI 兼容端点）。
type DashScopeEmbedder struct {
	cfg     config.EmbeddingConfig
	dims    int
	client  openai.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewDashScopeEmbedder(cfg config.EmbeddingConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *DashScopeEmbedder {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = dashscopeDefaultBaseURL
	}
	return &DashScopeEmbedder{
		cfg:     cfg,
		dims:    dashscopeDims[cfg.Model],
		client:  openai.NewClient(option.WithAPIKey(cfg.APIKey), option.WithBaseURL(baseURL)),
		breaker: breaker,
		logger:  logger,
	}
}

func (e *DashScopeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	// TODO: 经 openai-go 调用百炼兼容端点 embeddings，要求同 OpenAIEmbedder.Embed ①~⑥
	//（breaker 包裹、batch 分批、SDK 内置重试、密钥禁打印、SDK 类型不外泄、失败映射 50202）；
	// dims 未知时取首个返回向量长度回填并缓存（维度一致性探测，硬约束 #14）。
	panic("TODO: DashScopeEmbedder.Embed not implemented")
}

func (e *DashScopeEmbedder) Dimensions() int      { return e.dims }
func (e *DashScopeEmbedder) ProviderName() string { return "dashscope" }
func (e *DashScopeEmbedder) ModelName() string    { return e.cfg.Model }

var _ Embedder = (*DashScopeEmbedder)(nil)
```

**④ `internal/embed/siliconflow.go`**【骨架 TODO】— SiliconFlow 实现骨架：复用 openai-go，base_url 默认 `https://api.siliconflow.cn/v1`。

```go
package embed

import (
	"context"

	"deepwiki/internal/config"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
)

// siliconflowDefaultBaseURL SiliconFlow OpenAI 兼容端点（变更总纲 §4.7，逐字）。
const siliconflowDefaultBaseURL = "https://api.siliconflow.cn/v1"

// siliconflowDims 已知模型维度表；下一轮按官方文档补全。
var siliconflowDims = map[string]int{
	"BAAI/bge-large-zh-v1.5": 1024,
	"BAAI/bge-m3":            1024,
}

// SiliconFlowEmbedder 复用 openai-go 客户端的 SiliconFlow 向量实现骨架（OpenAI 兼容端点）。
type SiliconFlowEmbedder struct {
	cfg     config.EmbeddingConfig
	dims    int
	client  openai.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewSiliconFlowEmbedder(cfg config.EmbeddingConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *SiliconFlowEmbedder {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = siliconflowDefaultBaseURL
	}
	return &SiliconFlowEmbedder{
		cfg:     cfg,
		dims:    siliconflowDims[cfg.Model],
		client:  openai.NewClient(option.WithAPIKey(cfg.APIKey), option.WithBaseURL(baseURL)),
		breaker: breaker,
		logger:  logger,
	}
}

func (e *SiliconFlowEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	// TODO: 经 openai-go 调用 SiliconFlow /v1/embeddings，要求同 OpenAIEmbedder.Embed ①~⑥；
	// dims 未知时按首个返回向量长度探测（硬约束 #14）。
	panic("TODO: SiliconFlowEmbedder.Embed not implemented")
}

func (e *SiliconFlowEmbedder) Dimensions() int      { return e.dims }
func (e *SiliconFlowEmbedder) ProviderName() string { return "siliconflow" }
func (e *SiliconFlowEmbedder) ModelName() string    { return e.cfg.Model }

var _ Embedder = (*SiliconFlowEmbedder)(nil)
```

**⑤ `internal/embed/voyage.go`**【骨架 TODO】— VoyageAI 实现骨架：复用 openai-go，base_url 默认 `https://api.voyageai.com/v1`（VoyageAI 官方提供 OpenAI 兼容 embeddings 端点）。

```go
package embed

import (
	"context"

	"deepwiki/internal/config"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
)

// voyageDefaultBaseURL VoyageAI OpenAI 兼容 embeddings 端点（变更总纲 §4.7，逐字）。
const voyageDefaultBaseURL = "https://api.voyageai.com/v1"

// voyageDims 已知模型维度表；下一轮按官方文档补全。
var voyageDims = map[string]int{
	"voyage-code-3": 1024,
	"voyage-3":      1024,
}

// VoyageEmbedder 复用 openai-go 客户端的 VoyageAI 向量实现骨架（OpenAI 兼容端点，embedding only）。
type VoyageEmbedder struct {
	cfg     config.EmbeddingConfig
	dims    int
	client  openai.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewVoyageEmbedder(cfg config.EmbeddingConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *VoyageEmbedder {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = voyageDefaultBaseURL
	}
	return &VoyageEmbedder{
		cfg:     cfg,
		dims:    voyageDims[cfg.Model],
		client:  openai.NewClient(option.WithAPIKey(cfg.APIKey), option.WithBaseURL(baseURL)),
		breaker: breaker,
		logger:  logger,
	}
}

func (e *VoyageEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	// TODO: 经 openai-go 调用 VoyageAI /v1/embeddings，要求同 OpenAIEmbedder.Embed ①~⑥；
	// dims 未知时按首个返回向量长度探测（硬约束 #14）。
	panic("TODO: VoyageEmbedder.Embed not implemented")
}

func (e *VoyageEmbedder) Dimensions() int      { return e.dims }
func (e *VoyageEmbedder) ProviderName() string { return "voyage" }
func (e *VoyageEmbedder) ModelName() string    { return e.cfg.Model }

var _ Embedder = (*VoyageEmbedder)(nil)
```

**⑥ `internal/embed/ollama.go`**【骨架 TODO】— Ollama 实现骨架：走官方 `github.com/ollama/ollama` 的 `api` 包，base_url 指向本地。

```go
package embed

import (
	"context"
	"net/http"
	"net/url"

	"deepwiki/internal/config"

	"github.com/ollama/ollama/api"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
)

// ollamaDefaultBaseURL 本地 Ollama 默认地址（变更总纲 §4.7：base_url 指向本地）。
const ollamaDefaultBaseURL = "http://127.0.0.1:11434"

// OllamaEmbedder 基于 ollama api 包的本地向量实现骨架。
type OllamaEmbedder struct {
	cfg     config.EmbeddingConfig
	dims    int
	client  *api.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewOllamaEmbedder(cfg config.EmbeddingConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *OllamaEmbedder {
	raw := cfg.BaseURL
	if raw == "" {
		raw = ollamaDefaultBaseURL
	}
	base, err := url.Parse(raw)
	if err != nil {
		base, _ = url.Parse(ollamaDefaultBaseURL) // base_url 格式校验在 config 层兜底
	}
	return &OllamaEmbedder{
		cfg:     cfg,
		client:  api.NewClient(base, &http.Client{Timeout: cfg.Timeout}),
		breaker: breaker,
		logger:  logger,
	}
}

func (e *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	// TODO: 用 ollama api 包 client.Embed(ctx, &api.EmbedRequest{Model: e.cfg.Model, Input: texts})，要求：
	// ① ollama SDK 无内置重试 → 必须外包一层手写指数退避 backoff×2^n + ±20% 抖动，
	//    仅对网络错 / 429 / 5xx 重试（变更总纲 §4.7，硬约束 #7）；
	// ② 整个调用经 e.breaker.Execute 包裹（连续失败 ≥5 → open 60s → half-open 单探测）；
	// ③ dims 用首次成功返回的 len(embeddings[0]) 探测并缓存（硬约束 #14 维度探测用）；
	// ④ SDK 类型（api.EmbedResponse 等）禁止外泄，方法内转换为 [][]float32（硬约束 #17）；
	// ⑤ 重试耗尽 / breaker open 映射 50202 embedding_unavailable。
	panic("TODO: OllamaEmbedder.Embed not implemented")
}

func (e *OllamaEmbedder) Dimensions() int      { return e.dims }
func (e *OllamaEmbedder) ProviderName() string { return "ollama" }
func (e *OllamaEmbedder) ModelName() string    { return e.cfg.Model }

var _ Embedder = (*OllamaEmbedder)(nil)
```

**⑦ `internal/embed/factory.go`**【完整代码】— provider 枚举工厂（基线 §8.1 冻结 5 值；SDK 分支按变更总纲 §4.7 映射表）。熔断器设置与变更总纲 §4.7 韧性映射逐字一致：连续失败 ≥5 → open 60s → half-open 单探测。

```go
package embed

import (
	"fmt"
	"time"

	"deepwiki/internal/config"

	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
)

// newBreaker 每个 provider 实例一个熔断器（变更总纲 §4.7 / 硬约束 #7）：
// 连续失败 ≥5 → open 60s → half-open 单探测（MaxRequests=1）→ 关闭；状态反映到 health。
func newBreaker(name string) *gobreaker.CircuitBreaker[any] {
	return gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
		Name:        name,
		MaxRequests: 1,
		Timeout:     60 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool { return counts.ConsecutiveFailures >= 5 },
	})
}

// New 按配置构造 Embedder。provider 取值冻结：openai|dashscope|siliconflow|ollama|voyage。
// SDK 分支按变更总纲 §4.7：openai/dashscope/siliconflow/voyage → openai-go（不同 base_url）；ollama → ollama api 包。
func New(cfg config.EmbeddingConfig, logger *zap.Logger) (Embedder, error) {
	switch cfg.Provider {
	case "openai":
		return NewOpenAIEmbedder(cfg, newBreaker("embed-openai"), logger), nil
	case "dashscope":
		return NewDashScopeEmbedder(cfg, newBreaker("embed-dashscope"), logger), nil
	case "siliconflow":
		return NewSiliconFlowEmbedder(cfg, newBreaker("embed-siliconflow"), logger), nil
	case "ollama":
		return NewOllamaEmbedder(cfg, newBreaker("embed-ollama"), logger), nil
	case "voyage":
		return NewVoyageEmbedder(cfg, newBreaker("embed-voyage"), logger), nil
	default:
		return nil, fmt.Errorf("unknown embedding provider: %s", cfg.Provider)
	}
}
```

---

### 5.3 internal/llm —— LLM 接口、5 个 provider 官方 SDK 骨架与工厂

SDK 选型按《变更总纲》§4.7 映射表：openai / deepseek 走 **openai-go**（不同 base_url）；claude 走 **anthropic-sdk-go**；gemini 走 **google.golang.org/genai**；ollama 走 **ollama `api` 包**。每个 adapter 内嵌 `gobreaker` 熔断器字段（构造时注入，硬约束 #7）。`GenerateStream` 流式签名冻结不变。

**① `internal/llm/llm.go`**【完整代码】— 对话模型抽象（基线 §7，冻结签名）。实现：OpenAI / DeepSeek / Claude / Gemini / Ollama。

```go
// Package llm 对话模型 Provider 抽象与实现。
package llm

import (
	"context"

	"deepwiki/internal/model"
)

// LLM 对话模型抽象（基线 §7，冻结签名）。
// 任何官方 SDK 类型禁止出现在本签名与返回值中（硬约束 #17）。
type LLM interface {
	// Generate 非流式。
	Generate(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error)
	// GenerateStream 流式；返回 channel 由实现方关闭；流内错误通过 StreamChunk.Err 传递。
	GenerateStream(ctx context.Context, req model.ChatRequest) (<-chan model.StreamChunk, error)
	ProviderName() string // openai|gemini|claude|ollama|deepseek
	ModelName() string
}
```

**② `internal/llm/openai.go`**【骨架 TODO】— OpenAI 实现骨架（openai-go 官方客户端）。

```go
package llm

import (
	"context"

	"deepwiki/internal/config"
	"deepwiki/internal/model"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
)

// OpenAILLM 基于官方 SDK openai-go 的 OpenAI 对话实现骨架（变更总纲 §4.7）。
type OpenAILLM struct {
	cfg     config.LLMConfig
	client  openai.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

// NewOpenAILLM 构造 OpenAI 对话 adapter；breaker 由工厂注入（每 provider 实例一个，硬约束 #7）。
func NewOpenAILLM(cfg config.LLMConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *OpenAILLM {
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	return &OpenAILLM{
		cfg:     cfg,
		client:  openai.NewClient(opts...),
		breaker: breaker,
		logger:  logger,
	}
}

func (l *OpenAILLM) Generate(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	// TODO: 用 openai-go 调用 client.Chat.Completions.New(...)，要求：
	// ① 整个调用经 l.breaker.Execute 包裹（硬约束 #7：连续失败 ≥5 → open 60s → half-open 单探测）；
	// ② ctx 超时由调用方统一 context.WithTimeout 包裹；重试优先用 SDK 内置（429/5xx 指数退避，尊重 Retry-After）；
	// ③ API key 仅经 option.WithAPIKey 注入，禁止打印（硬约束 #2）；
	// ④ provider 不返回 usage 时按 tokens≈ceil(len([]rune(content))/4) 估算兜底并记日志（基线 §12.4）；
	// ⑤ SDK 类型在本方法内转换为 model.ChatResponse，禁止外泄（硬约束 #17）；
	// ⑥ 重试耗尽 / 持续 5xx / breaker open 映射 50201 llm_unavailable。
	panic("TODO: OpenAILLM.Generate not implemented")
}

func (l *OpenAILLM) GenerateStream(ctx context.Context, req model.ChatRequest) (<-chan model.StreamChunk, error) {
	// TODO: 用 openai-go 流式 API（stream := client.Chat.Completions.NewStreaming(...)；stream.Next() 迭代，
	// SDK 负责 SSE 解析，禁止手写 SSE 解析），要求：
	// ① 返回 channel 由实现方 goroutine 关闭，goroutine 必须 defer recover()（硬约束 #4）；
	// ② ctx 取消即中断流并退出 goroutine（硬约束 #4）；建立调用经 breaker 包裹，流内错误计入 breaker 失败；
	// ③ 结束 chunk 携带 FinishReason / Usage（provider 支持时）；流内错误经 StreamChunk.Err 传递后关闭 channel；
	// ④ SDK 流式类型转换为 model.StreamChunk，禁止外泄（硬约束 #17）。
	panic("TODO: OpenAILLM.GenerateStream not implemented")
}

func (l *OpenAILLM) ProviderName() string { return "openai" }
func (l *OpenAILLM) ModelName() string    { return l.cfg.Model }

var _ LLM = (*OpenAILLM)(nil)
```

**③ `internal/llm/deepseek.go`**【骨架 TODO】— DeepSeek 实现骨架：复用 openai-go 客户端，base_url 默认 `https://api.deepseek.com`（变更总纲 §4.7，OpenAI 兼容）。

```go
package llm

import (
	"context"

	"deepwiki/internal/config"
	"deepwiki/internal/model"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
)

// deepseekDefaultBaseURL DeepSeek OpenAI 兼容端点（变更总纲 §4.7，逐字）。
const deepseekDefaultBaseURL = "https://api.deepseek.com"

// DeepSeekLLM 复用 openai-go 客户端的 DeepSeek 对话实现骨架（OpenAI 兼容端点）。
type DeepSeekLLM struct {
	cfg     config.LLMConfig
	client  openai.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewDeepSeekLLM(cfg config.LLMConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *DeepSeekLLM {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = deepseekDefaultBaseURL
	}
	return &DeepSeekLLM{
		cfg:     cfg,
		client:  openai.NewClient(option.WithAPIKey(cfg.APIKey), option.WithBaseURL(baseURL)),
		breaker: breaker,
		logger:  logger,
	}
}

func (l *DeepSeekLLM) Generate(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	// TODO: 经 openai-go 调用 DeepSeek /chat/completions，要求同 OpenAILLM.Generate ①~⑥
	//（breaker 包裹、SDK 内置重试、密钥禁打印、usage 兜底、SDK 类型不外泄、失败映射 50201）。
	panic("TODO: DeepSeekLLM.Generate not implemented")
}

func (l *DeepSeekLLM) GenerateStream(ctx context.Context, req model.ChatRequest) (<-chan model.StreamChunk, error) {
	// TODO: openai-go 流式（stream.Next() 迭代），要求同 OpenAILLM.GenerateStream ①~④。
	panic("TODO: DeepSeekLLM.GenerateStream not implemented")
}

func (l *DeepSeekLLM) ProviderName() string { return "deepseek" }
func (l *DeepSeekLLM) ModelName() string    { return l.cfg.Model }

var _ LLM = (*DeepSeekLLM)(nil)
```

**④ `internal/llm/claude.go`**【骨架 TODO】— Claude 实现骨架：走官方 anthropic-sdk-go。

```go
package llm

import (
	"context"

	"deepwiki/internal/config"
	"deepwiki/internal/model"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
)

// ClaudeLLM 基于官方 anthropic-sdk-go 的 Claude 对话实现骨架（变更总纲 §4.7）。
type ClaudeLLM struct {
	cfg     config.LLMConfig
	client  anthropic.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewClaudeLLM(cfg config.LLMConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *ClaudeLLM {
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	return &ClaudeLLM{
		cfg:     cfg,
		client:  anthropic.NewClient(opts...),
		breaker: breaker,
		logger:  logger,
	}
}

func (l *ClaudeLLM) Generate(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	// TODO: 用 anthropic-sdk-go 调用 client.Messages.New(...)，要求：
	// ① 整个调用经 l.breaker.Execute 包裹（硬约束 #7）；
	// ② 模型领域角色映射：model.ChatMessage{system|user|assistant} → SDK 消息参数，
	//    system 消息按 SDK 要求拆到 system 参数；转换只允许发生在本 adapter 内（硬约束 #17）；
	// ③ 重试优先 SDK 内置；密钥禁打印（硬约束 #2）；usage 缺失按 tokens≈ceil(len([]rune)/4) 兜底；
	// ④ 失败映射 50201 llm_unavailable。
	panic("TODO: ClaudeLLM.Generate not implemented")
}

func (l *ClaudeLLM) GenerateStream(ctx context.Context, req model.ChatRequest) (<-chan model.StreamChunk, error) {
	// TODO: anthropic 流式 API（Messages.NewStreaming，事件迭代）→ model.StreamChunk channel，要求：
	// ① goroutine defer recover() 且传 ctx（硬约束 #4）；② 流内错误经 StreamChunk.Err 传递后关闭 channel；
	// ③ 结束 chunk 携带 FinishReason / Usage；④ SDK 事件类型不外泄（硬约束 #17）。
	panic("TODO: ClaudeLLM.GenerateStream not implemented")
}

func (l *ClaudeLLM) ProviderName() string { return "claude" }
func (l *ClaudeLLM) ModelName() string    { return l.cfg.Model }

var _ LLM = (*ClaudeLLM)(nil)
```

**⑤ `internal/llm/gemini.go`**【骨架 TODO】— Gemini 实现骨架：走官方统一 GenAI SDK（google.golang.org/genai）。

```go
package llm

import (
	"context"
	"fmt"

	"deepwiki/internal/config"
	"deepwiki/internal/model"

	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
	"google.golang.org/genai"
)

// GeminiLLM 基于 google.golang.org/genai 的 Gemini 对话实现骨架（变更总纲 §4.7）。
type GeminiLLM struct {
	cfg     config.LLMConfig
	client  *genai.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewGeminiLLM(cfg config.LLMConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *GeminiLLM {
	// genai.NewClient 需要 ctx；工厂无 ctx 入参，这里用 Background，装配层必须在启动阶段完成构造（启动失败优于带病运行）。
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		panic(fmt.Sprintf("genai client init: %v", err))
	}
	return &GeminiLLM{
		cfg:     cfg,
		client:  client,
		breaker: breaker,
		logger:  logger,
	}
}

func (l *GeminiLLM) Generate(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	// TODO: 用 genai 调用 client.Models.GenerateContent(...)，要求：
	// ① 整个调用经 l.breaker.Execute 包裹（硬约束 #7）；
	// ② model.ChatMessage → genai.Content 转换只允许发生在本 adapter 内（硬约束 #17）；
	// ③ 密钥仅经 ClientConfig.APIKey 注入，禁止打印（硬约束 #2）；usage 缺失按估算兜底并记日志；
	// ④ 失败映射 50201 llm_unavailable。
	panic("TODO: GeminiLLM.Generate not implemented")
}

func (l *GeminiLLM) GenerateStream(ctx context.Context, req model.ChatRequest) (<-chan model.StreamChunk, error) {
	// TODO: genai 流式 GenerateContentStream（迭代器）→ model.StreamChunk channel，要求：
	// ① goroutine defer recover() 且传 ctx（硬约束 #4）；② 流内错误经 StreamChunk.Err 传递后关闭 channel；
	// ③ 结束 chunk 携带 FinishReason / Usage；④ SDK 类型不外泄（硬约束 #17）。
	panic("TODO: GeminiLLM.GenerateStream not implemented")
}

func (l *GeminiLLM) ProviderName() string { return "gemini" }
func (l *GeminiLLM) ModelName() string    { return l.cfg.Model }

var _ LLM = (*GeminiLLM)(nil)
```

**⑥ `internal/llm/ollama.go`**【骨架 TODO】— Ollama 实现骨架：走官方 ollama `api` 包，base_url 指向本地。

```go
package llm

import (
	"context"
	"net/http"
	"net/url"

	"deepwiki/internal/config"
	"deepwiki/internal/model"

	"github.com/ollama/ollama/api"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
)

// ollamaDefaultBaseURL 本地 Ollama 默认地址（变更总纲 §4.7：base_url 指向本地）。
const ollamaDefaultBaseURL = "http://127.0.0.1:11434"

// OllamaLLM 基于 ollama api 包的本地对话实现骨架。
type OllamaLLM struct {
	cfg     config.LLMConfig
	client  *api.Client
	breaker *gobreaker.CircuitBreaker[any]
	logger  *zap.Logger
}

func NewOllamaLLM(cfg config.LLMConfig, breaker *gobreaker.CircuitBreaker[any], logger *zap.Logger) *OllamaLLM {
	raw := cfg.BaseURL
	if raw == "" {
		raw = ollamaDefaultBaseURL
	}
	base, err := url.Parse(raw)
	if err != nil {
		base, _ = url.Parse(ollamaDefaultBaseURL) // base_url 格式校验在 config 层兜底
	}
	return &OllamaLLM{
		cfg:     cfg,
		client:  api.NewClient(base, &http.Client{Timeout: cfg.Timeout}),
		breaker: breaker,
		logger:  logger,
	}
}

func (l *OllamaLLM) Generate(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	// TODO: 用 ollama api 包 client.Chat(ctx, &api.ChatRequest{Model: l.cfg.Model, Messages: ..., Stream: &false}, fn)，要求：
	// ① ollama SDK 无内置重试 → 外包一层手写指数退避 backoff×2^n + ±20% 抖动，仅网络错 / 429 / 5xx（硬约束 #7）；
	// ② 整个调用经 l.breaker.Execute 包裹；③ model.ChatMessage → api.Message 转换只允许在本 adapter 内（硬约束 #17）；
	// ④ usage 缺失按 tokens≈ceil(len([]rune(content))/4) 兜底；⑤ 失败映射 50201 llm_unavailable。
	panic("TODO: OllamaLLM.Generate not implemented")
}

func (l *OllamaLLM) GenerateStream(ctx context.Context, req model.ChatRequest) (<-chan model.StreamChunk, error) {
	// TODO: ollama 流式（ChatRequest.Stream=true，回调内逐条接收 api.ChatResponse）→ model.StreamChunk channel，要求：
	// ① goroutine defer recover() 且传 ctx（硬约束 #4）；② 流内错误经 StreamChunk.Err 传递后关闭 channel；
	// ③ Done 时结束 chunk 携带 FinishReason / Usage（EvalCount 等）；④ SDK 类型不外泄（硬约束 #17）。
	panic("TODO: OllamaLLM.GenerateStream not implemented")
}

func (l *OllamaLLM) ProviderName() string { return "ollama" }
func (l *OllamaLLM) ModelName() string    { return l.cfg.Model }

var _ LLM = (*OllamaLLM)(nil)
```

**⑦ `internal/llm/factory.go`**【完整代码】— provider 枚举工厂（基线 §8.1 冻结 5 值；SDK 分支按变更总纲 §4.7 映射表）。

```go
package llm

import (
	"fmt"
	"time"

	"deepwiki/internal/config"

	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
)

// newBreaker 每个 provider 实例一个熔断器（变更总纲 §4.7 / 硬约束 #7）：
// 连续失败 ≥5 → open 60s → half-open 单探测（MaxRequests=1）→ 关闭；状态反映到 health。
func newBreaker(name string) *gobreaker.CircuitBreaker[any] {
	return gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
		Name:        name,
		MaxRequests: 1,
		Timeout:     60 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool { return counts.ConsecutiveFailures >= 5 },
	})
}

// New 按配置构造 LLM。provider 取值冻结：openai|gemini|claude|ollama|deepseek。
// SDK 分支按变更总纲 §4.7：openai/deepseek → openai-go（不同 base_url）；claude → anthropic-sdk-go；
// gemini → google.golang.org/genai；ollama → ollama api 包。
func New(cfg config.LLMConfig, logger *zap.Logger) (LLM, error) {
	switch cfg.Provider {
	case "openai":
		return NewOpenAILLM(cfg, newBreaker("llm-openai"), logger), nil
	case "gemini":
		return NewGeminiLLM(cfg, newBreaker("llm-gemini"), logger), nil
	case "claude":
		return NewClaudeLLM(cfg, newBreaker("llm-claude"), logger), nil
	case "ollama":
		return NewOllamaLLM(cfg, newBreaker("llm-ollama"), logger), nil
	case "deepseek":
		return NewDeepSeekLLM(cfg, newBreaker("llm-deepseek"), logger), nil
	default:
		return nil, fmt.Errorf("unknown llm provider: %s", cfg.Provider)
	}
}
```

---

### 5.4 internal/retriever —— Retriever 接口与 3+1 实现骨架

keyword 走 OpenSearch（BM25，每仓一索引）；vector 走 pgvector（HNSW + `<=>` 余弦距离）；hybrid（RRF 融合）与 rerank（装饰器）逻辑骨架与基线一致。

**① `internal/retriever/retriever.go`**【完整代码】— 检索抽象（基线 §7，冻结签名；AskService 只面向本接口，禁止直接访问 pgvector / OpenSearch 客户端）。

```go
// Package retriever 可插拔检索抽象与实现（keyword/embedding/hybrid + rerank 装饰器）。
package retriever

import (
	"context"

	"deepwiki/internal/model"
)

// Retriever 检索抽象（基线 §7，冻结签名）。
type Retriever interface {
	Search(ctx context.Context, repoID string, query string, topK int) ([]model.ChunkHit, error)
	Mode() string // keyword|embedding|hybrid
}
```

**② `internal/retriever/keyword.go`**【骨架 TODO】— OpenSearch BM25 检索。

```go
package retriever

import (
	"context"

	"go.uber.org/zap"

	"deepwiki/internal/model"
	"deepwiki/internal/search"
	"deepwiki/internal/store"
)

// KeywordRetriever OpenSearch BM25 实现；每仓一索引 deepwiki-chunks-<repo_id 全小写>（OpenSearch
// 索引名必须小写，repo_id 含大写 ULID，统一 strings.ToLower），文档 _id = chunk_id。
type KeywordRetriever struct {
	client     *search.Client   // OpenSearch 客户端封装（internal/search，见 §5.11.5）
	chunkStore store.ChunkStore // 命中后按 chunk_id 回填 Chunk（references 校验依赖，硬约束 #15）
	logger     *zap.Logger
}

func NewKeywordRetriever(client *search.Client, chunkStore store.ChunkStore, logger *zap.Logger) *KeywordRetriever {
	return &KeywordRetriever{client: client, chunkStore: chunkStore, logger: logger}
}

func (r *KeywordRetriever) Mode() string { return "keyword" }

func (r *KeywordRetriever) Search(ctx context.Context, repoID string, query string, topK int) ([]model.ChunkHit, error) {
	// TODO: 实现 OpenSearch BM25 检索，要求：
	// ① repoID 必须先过 ULID 正则（硬约束 #11），索引名经 internal/search 导出的构造函数生成
	//    （deepwiki-chunks-<repo_id 全小写>），禁止字符串拼接用户输入进查询体；
	// ② 查询体：multi_match 于 content^2, path；filter: term repo_id（索引内天然隔离可省略，保留 filter 防御）；
	//    BM25 默认排序（mapping 已声明 index.similarity.default.type=BM25）；
	//    进阶 path_filter 用 prefix 查询匹配 path.raw；
	// ③ 取 topK 命中，用 _id（chunk_id）经 chunkStore.GetByIDs 回填 Chunk；
	// ④ 尊重 ctx：OpenSearch 请求必须带 ctx，每步检查 ctx.Err()（硬约束 #4）；
	// ⑤ OpenSearch 不可用映射 50303 search_unavailable；score 为 BM25 分。
	panic("TODO: KeywordRetriever.Search not implemented")
}

var _ Retriever = (*KeywordRetriever)(nil)
```

**③ `internal/retriever/vector.go`**【骨架 TODO】— pgvector HNSW 余弦检索（《变更总纲》§4.1：本类型是向量检索 SQL 的唯一实现处）。

```go
package retriever

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"deepwiki/internal/embed"
	"deepwiki/internal/model"
)

// VectorRetriever 向量检索实现：query 向量化 → pgvector HNSW 检索。
// 持有 pgxpool 直连 chunks 表（变更总纲 §4.1：检索 SQL 唯一实现处）。
type VectorRetriever struct {
	pool     *pgxpool.Pool
	emb      embed.Embedder
	efSearch int // storage.vector.ef_search，默认 64（HNSW 查询精度/延迟权衡，可热更新）
	logger   *zap.Logger
}

func NewVectorRetriever(pool *pgxpool.Pool, emb embed.Embedder, efSearch int, logger *zap.Logger) *VectorRetriever {
	if efSearch <= 0 {
		efSearch = 64
	}
	return &VectorRetriever{pool: pool, emb: emb, efSearch: efSearch, logger: logger}
}

func (r *VectorRetriever) Mode() string { return "embedding" }

func (r *VectorRetriever) Search(ctx context.Context, repoID string, query string, topK int) ([]model.ChunkHit, error) {
	// TODO: 实现 pgvector 检索，要求：
	// ① repoID 必须先过 ULID 正则（硬约束 #11）；
	// ② r.emb.Embed(ctx, []string{query}) 取第一条向量，失败映射 50202 embedding_unavailable；
	// ③ 必须在事务内执行下列 SQL（SET LOCAL 仅事务内有效；efSearch 为整数配置值拼接，非用户输入），
	//    变更总纲 §4.1 检索 SQL 全文：
	//      SET LOCAL hnsw.ef_search = 64;
	//      SELECT chunk_id, path, start_line, end_line, language, content,
	//             1 - (embedding <=> $2) AS score
	//      FROM chunks
	//      WHERE repo_id = $1
	//        AND ($3::text IS NULL OR path LIKE $3)   -- 按文件路径过滤（进阶要求）
	//      ORDER BY embedding <=> $2
	//      LIMIT $4;
	//    $3 传 nil 表示不按路径过滤；
	// ④ 查询向量绑定用 pgvector.NewVector(vec)；维度不符会被 vector(1536) 列类型直接拒绝（硬约束 #14 第二道防线）；
	// ⑤ score 为余弦相似度 [0,1]；DB 查询失败映射 50203 vector_store_unavailable。
	panic("TODO: VectorRetriever.Search not implemented")
}

var _ Retriever = (*VectorRetriever)(nil)
```

**④ `internal/retriever/hybrid.go`**【骨架 TODO】— RRF 融合。

```go
package retriever

import (
	"context"

	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// HybridRetriever RRF 融合（基线 §2.1，rrf_k 默认 60）。
type HybridRetriever struct {
	keyword *KeywordRetriever
	vector  *VectorRetriever
	rrfK    int
	logger  *zap.Logger
}

func NewHybridRetriever(keyword *KeywordRetriever, vector *VectorRetriever, rrfK int, logger *zap.Logger) *HybridRetriever {
	return &HybridRetriever{keyword: keyword, vector: vector, rrfK: rrfK, logger: logger}
}

func (r *HybridRetriever) Mode() string { return "hybrid" }

func (r *HybridRetriever) Search(ctx context.Context, repoID string, query string, topK int) ([]model.ChunkHit, error) {
	// TODO: 实现 RRF 融合，要求：
	// ① 两路检索（OpenSearch BM25 + pgvector HNSW）可并行，但 goroutine 必须 defer recover() 且传 ctx（硬约束 #4）；
	// ② 融合分 score = Σ 1/(rrfK + rank)，按 chunk_id 合并，降序取 topK；
	// ③ 任一路失败降级为另一路结果并记 WARN，不整体失败；两路均失败才返回错误。
	panic("TODO: HybridRetriever.Search not implemented")
}

var _ Retriever = (*HybridRetriever)(nil)
```

**⑤ `internal/retriever/rerank.go`**【骨架 TODO】— 重排装饰器（可选；`Mode()` 返回内层值；是否启用由配置内部开关决定，不进入 API 契约，基线 §7 扩展点说明）。

```go
package retriever

import (
	"context"

	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// RerankRetriever 装饰器：内嵌基础 Retriever，Search 中对候选结果重排后截断到 topK。
type RerankRetriever struct {
	inner  Retriever
	logger *zap.Logger
}

func NewRerankRetriever(inner Retriever, logger *zap.Logger) *RerankRetriever {
	return &RerankRetriever{inner: inner, logger: logger}
}

func (r *RerankRetriever) Mode() string { return r.inner.Mode() }

func (r *RerankRetriever) Search(ctx context.Context, repoID string, query string, topK int) ([]model.ChunkHit, error) {
	// TODO: 实现重排装饰，要求：
	// ① inner.Search(ctx, repoID, query, topK*4) 取候选；② 交叉编码或 LLM 重排打分；③ 截断到 topK；
	// ④ 重排失败必须降级为 inner 原序结果并记 WARN，不得整体失败。
	panic("TODO: RerankRetriever.Search not implemented")
}

var _ Retriever = (*RerankRetriever)(nil)
```

---

### 5.5 internal/ingest —— Cloner 接口与 git CLI 实现、Pipeline 类型、Parser、Chunker、忽略规则

**① `internal/ingest/cloner.go`**【接口完整 + 实现骨架 TODO】— Git 抽象（基线 §7，冻结签名）与系统 git CLI 实现（系统依赖 `git ≥ 2.30`）。本文件是硬约束 #5 的落点，**禁止出现任何 `git pull` 语义、禁止 `sh -c` 拼接命令**。

```go
// Package ingest 仓库摄取：克隆、解析、切分与 Pipeline 编排。
package ingest

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// Cloner Git 抽象（基线 §7，冻结签名）。
type Cloner interface {
	// Clone 克隆到 destDir（调用方负责传入 .tmp 临时目录，成功后原子 rename）。
	Clone(ctx context.Context, url, branch, destDir string) error
	// FetchAndReset = git fetch --depth 1 origin <branch> → git reset --hard FETCH_HEAD → git clean -fdx。
	// 禁止 git pull（硬约束 #5）；返回新的 commit hash。
	FetchAndReset(ctx context.Context, repoDir, branch string) (newCommitHash string, err error)
}

// GitCloner 基于系统 git CLI 的实现（exec.CommandContext，无 shell；CommandContext 天然支持 ctx 取消）。
type GitCloner struct {
	binaryPath string        // git.binary_path，默认 "git"
	opTimeout  time.Duration // git.op_timeout，默认 10m
	logger     *zap.Logger
}

// NewGitCloner binaryPath 为空取 "git"；opTimeout ≤ 0 取 10 分钟。
func NewGitCloner(binaryPath string, opTimeout time.Duration, logger *zap.Logger) *GitCloner {
	if binaryPath == "" {
		binaryPath = "git"
	}
	if opTimeout <= 0 {
		opTimeout = 10 * time.Minute
	}
	return &GitCloner{binaryPath: binaryPath, opTimeout: opTimeout, logger: logger}
}

var _ Cloner = (*GitCloner)(nil)

// Clone 浅克隆指定分支到 destDir；branch 为空时取远端默认分支。
func (c *GitCloner) Clone(ctx context.Context, url, branch, destDir string) error {
	// TODO: 实现浅克隆，要求（硬约束 #5，禁止 git pull / 禁止 sh -c）：
	// ① exec.CommandContext(ctx, c.binaryPath, "clone", "--depth", "1", "--single-branch",
	//    "--branch", branch, url, destDir)：参数必须以独立数组元素传递，禁止拼成单个字符串经 sh -c 执行；
	// ② cmd.Env 在 os.Environ() 上追加 GIT_TERMINAL_PROMPT=0，杜绝交互式凭据提示卡死 worker；
	// ③ 单次操作挂 context.WithTimeout(ctx, c.opTimeout)（git.op_timeout，默认 10m）；
	// ④ branch 为空时省略 --branch（远端默认分支）；
	// ⑤ 失败返回 fmt.Errorf 包装；stderr 截断进 zap 日志，禁止向客户端回传原始输出（硬约束 #8）。
	panic("TODO: GitCloner.Clone not implemented")
}

// FetchAndReset 等价于 git fetch → reset --hard FETCH_HEAD → clean -fdx。
// 禁止 git pull：工作区脏状态与 merge 冲突会直接卡死 pipeline（硬约束 #5）。
func (c *GitCloner) FetchAndReset(ctx context.Context, repoDir, branch string) (string, error) {
	// TODO: 依次执行三条命令（均为 exec.CommandContext 独立调用，禁止 git pull，禁止 sh -c）：
	// ① git -C <repoDir> fetch --depth 1 origin <branch>
	// ② git -C <repoDir> reset --hard FETCH_HEAD（reset 目标必须是 FETCH_HEAD，fetch 后即取，避免并发漂移）
	// ③ git -C <repoDir> clean -fdx（清理未跟踪文件）
	// 每步 env 带 GIT_TERMINAL_PROMPT=0，各挂 context.WithTimeout(c.opTimeout)；
	// 任一步失败返回错误，由调用方回退为「重新 clone 到 ./data/repos/.tmp/<task_id>/ 后 os.Rename 原子切换」；
	// 返回新 commit hash：git -C <repoDir> rev-parse HEAD 输出 strings.TrimSpace。
	panic("TODO: GitCloner.FetchAndReset not implemented")
}

// LsRemote 取远端分支 HEAD commit（ingest 幂等判断用，基线 §6.1；轻量、不落盘）。
// 失败（网络/私有仓库无凭据）时由调用方放行创建任务，clone 阶段再报错。
func (c *GitCloner) LsRemote(ctx context.Context, url, branch string) (string, error) {
	// TODO: exec.CommandContext(ctx, c.binaryPath, "ls-remote", url, "refs/heads/"+branch)，
	// 解析首列 hash；branch 为空时改查 "HEAD"；env GIT_TERMINAL_PROMPT=0 + context.WithTimeout(c.opTimeout)；
	// 未匹配到分支返回 fmt.Errorf("branch %q not found on remote %s", branch, url)。
	panic("TODO: GitCloner.LsRemote not implemented")
}
```

**② `internal/ingest/pipeline.go`**【类型完整 + Run 骨架 TODO】— Pipeline 上下文与阶段类型（基线 §7，冻结字段）。

```go
package ingest

import (
	"context"

	"go.uber.org/zap"

	"deepwiki/internal/eventbus"
	"deepwiki/internal/model"
)

// StageFunc Pipeline 阶段函数；每阶段入口与循环内必须 select ctx.Done()（反 AI 错误 #4）。
type StageFunc func(ctx context.Context, pc *PipelineContext) error

// PipelineContext 阶段间上下文（基线 §7，冻结字段）。
// WorkDir 为本任务临时工作目录 ./data/repos/.tmp/<task_id>；
// Files / Chunks 为阶段间中间产物（内存传递，落库以 Persist 阶段为准）。
type PipelineContext struct {
	Task    *model.Task
	Repo    *model.Repo
	Options IngestOptions // 本次任务生效的摄取参数（请求 options 覆盖配置后的结果）
	WorkDir string
	Files   []SourceFile
	Chunks  []model.Chunk
}

// IngestOptions 本次任务生效的摄取参数（基线 §7）。
type IngestOptions struct {
	ChunkSize    int
	ChunkOverlap int
	IncludeExt   []string
	ExcludeDirs  []string
}

// SourceFile 解析阶段的文件中间产物（基线 §7）。
type SourceFile struct {
	Path     string
	Language string
	Content  string
	Hash     string // sha256(content)[:16]
}

// Stage 命名阶段（Name 为阶段对应的 TaskState，用于状态推进与事件发布）。
type Stage struct {
	Name model.TaskState
	Fn   StageFunc
}

// Pipeline 顺序阶段执行器。
type Pipeline struct {
	stages []Stage
	bus    eventbus.EventBus
	logger *zap.Logger
}

func NewPipeline(stages []Stage, bus eventbus.EventBus, logger *zap.Logger) *Pipeline {
	return &Pipeline{stages: stages, bus: bus, logger: logger}
}

// report 进度回调：由调用方（Worker）实现 UpdateState + 落库节流（每 500ms 或每推进 5% 一次，基线 §4.4）。
type ProgressReport func(state model.TaskState, progress model.Progress, stats model.Stats) error

func (p *Pipeline) Run(ctx context.Context, pc *PipelineContext, report ProgressReport) error {
	// TODO: 顺序执行 p.stages，要求：
	// ① 每阶段入口必须 select ctx.Done()，取消时返回 ctx.Err()（反 AI 错误 #4，Worker 据此落 cancelled）；
	// ② 每阶段开始调用 report(stage.Name, ...) 并向 p.bus.Publish task.state_changed 事件（结构化字段，禁止拼字符串）；
	// ③ 阶段错误原样返回（错误须带 stage 信息，供 Worker 落 failed 时填 error.stage）；
	// ④ 进度落库节流在 Worker 侧实现，本函数只负责实时回调。
	panic("TODO: Pipeline.Run not implemented")
}
```

**③ `internal/ingest/parser.go`**【骨架 TODO】— 文件解析（遍历、过滤、hash、语言识别）。

```go
package ingest

import (
	"context"
)

// ParseFiles 遍历 workDir，按 include_ext/exclude_dirs 过滤，产出 SourceFile 列表（基线 §12.4）。
func ParseFiles(ctx context.Context, workDir string, opts IngestOptions) ([]SourceFile, error) {
	// TODO: 实现解析，要求：
	// ① filepath.WalkDir 遍历，跳过 opts.ExcludeDirs 目录名（非路径匹配）；
	// ② 仅保留 opts.IncludeExt 中的扩展名（小写比较）；③ 每文件算 Hash = sha256(content)[:16]；
	// ④ 扩展名→语言映射（.go→go、.py→python、.md→markdown……），未知扩展名 Language="" 仍入库；
	// ⑤ 相对路径禁止 .. 与绝对路径（反 AI 错误 #11）；⑥ 文件循环内必须 select ctx.Done()（反 AI 错误 #4）；
	// ⑦ 文件级 include/exclude 与默认跳过清单统一走 FileFilter（ignore.go，go-gitignore 语义）。
	panic("TODO: ParseFiles not implemented")
}
```

**④ `internal/ingest/chunker.go`**【骨架 TODO】— 行对齐切分。

```go
package ingest

import (
	"context"

	"deepwiki/internal/model"
)

// ChunkFiles 把 SourceFile 切分为 Chunk（基线 §12.4 切分策略）。
func ChunkFiles(ctx context.Context, repoID string, files []SourceFile, opts IngestOptions) ([]model.Chunk, error) {
	// TODO: 实现切分，要求：
	// ① 行对齐固定窗口：按行累加至 chunk_size（目标 token 数，tokens≈ceil(len([]rune(content))/4)）切块，
	//    块间回退 chunk_overlap 对应行数重叠；② Markdown 优先按标题层级切，再按窗口兜底；
	// ③ 一块一文件，保 path/start_line/end_line 精确；④ ChunkID = "chk_" + ULID；FileHash/EmbeddingModel 必填；
	// ⑤ 文件循环内必须 select ctx.Done()（反 AI 错误 #4）。
	panic("TODO: ChunkFiles not implemented")
}
```

**⑤ `internal/ingest/ignore.go`**【骨架 TODO】— 文件级过滤规则（go-gitignore，.gitignore 语义；《变更总纲》§4.6：include/exclude、跳过 .git/vendor/node_modules/二进制/超大文件的逻辑不变，规则匹配用 go-gitignore）。

```go
package ingest

import (
	ignore "github.com/sabhiram/go-gitignore"
)

// DefaultSkipDirs 默认跳过的目录名（非路径匹配；无论 include/exclude 如何配置都生效）。
var DefaultSkipDirs = []string{".git", "vendor", "node_modules"}

// DefaultMaxFileSize 默认跳过的单文件大小上限（1 MiB；超出按超大文件跳过，避免内存与分块失真）。
const DefaultMaxFileSize int64 = 1 << 20

// FileFilter include/exclude 规则匹配器（go-gitignore，.gitignore 语义）。
// include 为空表示全部允许；exclude 命中即拒绝；优先级：DefaultSkipDirs > exclude > include。
type FileFilter struct {
	include     *ignore.GitIgnore // nil 表示未配置 include（全部允许）
	exclude     *ignore.GitIgnore // nil 表示未配置 exclude
	skipDirs    []string
	maxFileSize int64
}

// NewFileFilter includePatterns / excludePatterns 为空切片时对应规则不生效；
// maxFileSize ≤ 0 取 DefaultMaxFileSize。
func NewFileFilter(includePatterns, excludePatterns []string, maxFileSize int64) *FileFilter {
	// TODO: 用 ignore.CompileIgnoreLines(lines...) 分别编译 include/exclude 规则；
	// 空规则列表时对应字段保持 nil；maxFileSize ≤ 0 取 DefaultMaxFileSize。
	panic("TODO: NewFileFilter not implemented")
}

// SkipDir 判断目录名（非路径）是否应整体跳过：DefaultSkipDirs ∪ 用户 ExcludeDirs。
func (f *FileFilter) SkipDir(dirName string) bool {
	// TODO: 目录名命中 f.skipDirs 即返回 true（硬约束 #11：只按目录名比较，不做路径匹配，避免误伤同名深层目录）。
	panic("TODO: FileFilter.SkipDir not implemented")
}

// SkipFile 判断仓库内相对路径是否应跳过，要求：
// TODO: ① relPath 先 filepath.Clean，含 ".." 或绝对路径一律跳过（硬约束 #11）；
// ② exclude 命中（MatchesPath）→ true；include 非 nil 且未命中 → true；
// ③ size > f.maxFileSize → true（超大文件）；④ 二进制文件（调用方探测后传 isBinary=true）→ true。
func (f *FileFilter) SkipFile(relPath string, size int64, isBinary bool) bool {
	panic("TODO: FileFilter.SkipFile not implemented")
}
```

---

### 5.6 internal/store + migrations —— PostgreSQL + pgvector 存储层

按《变更总纲》§4.1 落地：驱动 `pgx/v5`（`pgxpool` 连接池）；迁移 `golang-migrate/v4`（embed.FS `iofs` source，只前进）；向量 `pgvector`（`vector(1536)` + HNSW）；时间列全部 `timestamptz`；`stats_json / progress_json / error_json / toc_json` 升级为 **JSONB**；v1 的配置覆写/审计两表移除（配置覆写与审计迁往 etcd，见 `internal/config/etcd_source.go`）；v1 的 schema 版本记录表移除（golang-migrate 自建 `schema_migrations`）；`chunks.vector BLOB` 列移除（由 `embedding vector(1536)` 取代）；**新增 `api_keys` 表**（API key 只存哈希，硬约束 #2）。

**① `migrations/migrations.go`**【完整代码】— embed.FS 导出（`go:embed` 不允许 `..` 跨目录，故本文件必须位于 `migrations/` 根）。

```go
// Package migrations 内嵌 PostgreSQL 迁移脚本（变更总纲 §4.1：golang-migrate iofs source，只前进不回滚）。
// 命名从 000001_init.up.sql 起，只有 .up 没有 .down；迁移文件一旦合入不得修改，
// 变更只能新增更高序号的 .up.sql 文件。
package migrations

import "embed"

//go:embed *.up.sql
var FS embed.FS
```

**② `migrations/000001_init.up.sql`**【完整代码】— 建库 DDL 全文（与《变更总纲》§4.1 逐字对齐；要求数据库镜像 `pgvector/pgvector:pg16`，否则 CREATE EXTENSION 失败）。

```sql
-- 000001_init.up.sql —— DeepWiki 初始 schema（PostgreSQL 16 + pgvector）。
-- 只前进迁移：本文件合入后禁止修改；schema 变更只能新增更高序号的 .up.sql。
-- schema_migrations 版本表由 golang-migrate 自建自管，本文件不创建。

-- pgvector 扩展（向量列与 HNSW 索引依赖；镜像必须为 pgvector/pgvector:pg16）
CREATE EXTENSION IF NOT EXISTS vector;

-- 仓库
CREATE TABLE IF NOT EXISTS repos (
    repo_id     TEXT PRIMARY KEY,                 -- repo_ + ULID
    repo_url    TEXT NOT NULL,
    branch      TEXT NOT NULL,
    commit_hash TEXT NOT NULL DEFAULT '',
    local_path  TEXT NOT NULL DEFAULT '',
    state       TEXT NOT NULL DEFAULT 'ingesting' -- ingesting|ready|error
                CHECK (state IN ('ingesting','ready','error')),
    stats_json  JSONB NOT NULL DEFAULT '{"files":0,"chunks":0,"tokens":0}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (repo_url, branch)                     -- ingest 幂等的存储层兜底
);

-- 任务（三类任务共用；状态单一字段 state；任务状态唯一来源，硬约束 #3）
CREATE TABLE IF NOT EXISTS tasks (
    task_id              TEXT PRIMARY KEY,        -- tsk_ + ULID
    type                 TEXT NOT NULL
                         CHECK (type IN ('ingest','refresh','wiki')),
    repo_id              TEXT REFERENCES repos(repo_id) ON DELETE SET NULL,
    state                TEXT NOT NULL
                         CHECK (state IN ('pending','cloning','parsing','chunking',
                                          'embedding','persisting','outlining',
                                          'generating','fetching','diffing',
                                          'completed','failed','cancelled')),
    progress_json        JSONB NOT NULL DEFAULT '{"current":0,"total":0,"percent":0}'::jsonb,
    stats_json           JSONB NOT NULL DEFAULT '{"files":0,"chunks":0,"tokens":0}'::jsonb,
    error_json           JSONB,                   -- {code,message,stage} 或 NULL
    queue_position       INTEGER NOT NULL DEFAULT 0,
    cancel_flag          INTEGER NOT NULL DEFAULT 0,
    request_payload_json JSONB,                   -- 原始请求快照（重试/审计）
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at           TIMESTAMPTZ,
    finished_at          TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_tasks_state   ON tasks(state);
CREATE INDEX IF NOT EXISTS idx_tasks_repo    ON tasks(repo_id);
CREATE INDEX IF NOT EXISTS idx_tasks_created ON tasks(created_at);

-- 代码/文档分块（向量内联于 chunks.embedding，pgvector 列；VectorStore 与 VectorRetriever 的物理载体）
CREATE TABLE IF NOT EXISTS chunks (
    chunk_id        TEXT PRIMARY KEY,             -- chk_ + ULID
    repo_id         TEXT NOT NULL
                    REFERENCES repos(repo_id) ON DELETE CASCADE,
    path            TEXT NOT NULL,                -- 仓库内相对路径
    start_line      INTEGER NOT NULL,
    end_line        INTEGER NOT NULL,
    language        TEXT NOT NULL,
    content         TEXT NOT NULL,
    file_hash       TEXT NOT NULL DEFAULT '',     -- 文件 sha256[:16]，refresh diff 用
    embedding_model TEXT NOT NULL DEFAULT '',     -- 维度一致性校验用（硬约束 #14）
    embedding       vector(1536),                 -- pgvector 列；维度建表定型（storage.vector.dimensions），
                                                  -- 改维度 = 新迁移 + 全量重建；维度不符写入直接拒绝（#14 第二道防线）
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_chunks_repo      ON chunks(repo_id);
CREATE INDEX IF NOT EXISTS idx_chunks_repo_path ON chunks(repo_id, path);
-- HNSW 近似最近邻索引（余弦距离；查询时 SET LOCAL hnsw.ef_search = 64）
CREATE INDEX IF NOT EXISTS idx_chunks_embedding_hnsw ON chunks
    USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 128);

-- Wiki（TOC 与页面同表，kind 区分；一仓一行 kind='toc' + N 行 kind='page'）
CREATE TABLE IF NOT EXISTS wiki_pages (
    repo_id     TEXT NOT NULL
                REFERENCES repos(repo_id) ON DELETE CASCADE,
    slug        TEXT NOT NULL,                    -- 仓内标识，如 overview / module-api
    kind        TEXT NOT NULL CHECK (kind IN ('toc','page')),
    title       TEXT NOT NULL DEFAULT '',
    parent_slug TEXT NOT NULL DEFAULT '',
    sort_order  INTEGER NOT NULL DEFAULT 0,
    content_md  TEXT NOT NULL DEFAULT '',
    toc_json    JSONB,                            -- 仅 kind='toc' 行存放 TOC JSON
    task_id     TEXT REFERENCES tasks(task_id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (repo_id, slug)
);

-- API 密钥（只存哈希，硬约束 #2：SHA-256(salt‖key)；环境变量引导 → 本表 → Redis 60s 缓存二级查找）
CREATE TABLE IF NOT EXISTS api_keys (
    key_id      TEXT PRIMARY KEY,                 -- key_ + ULID
    name        TEXT NOT NULL DEFAULT '',
    key_hash    TEXT NOT NULL UNIQUE,             -- SHA-256(salt ‖ key) 十六进制
    salt        TEXT NOT NULL,
    is_admin    BOOLEAN NOT NULL DEFAULT FALSE,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

**③ `internal/store/postgres.go`**【完整代码】— pgxpool 连接封装（池参数、Ping、事务辅助）。

```go
// Package store PostgreSQL 存储层：全部状态持久化（硬约束 #3：Postgres tasks 表为任务状态唯一来源）。
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// DB 连接封装（pgxpool 连接池）。
type DB struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// Open 建立 pgxpool 连接池（变更总纲 §4.1：MaxConns=10, MinConns=2, MaxConnLifetime=1h,
// HealthCheckPeriod=30s；DSN 仅由环境变量 DEEPWIKI_POSTGRES_DSN 注入，禁止 yaml 明文）。
// maxConns 来自 storage.postgres.max_conns（默认 10，可热更新后重建池）。
func Open(ctx context.Context, dsn string, maxConns int32, logger *zap.Logger) (*DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("postgres dsn empty: set DEEPWIKI_POSTGRES_DSN")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	} else {
		cfg.MaxConns = 10
	}
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour
	cfg.HealthCheckPeriod = 30 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &DB{pool: pool, logger: logger}, nil
}

// Pool 暴露底层 *pgxpool.Pool（各 store 实现、VectorRetriever、health 探测用）。
func (d *DB) Pool() *pgxpool.Pool { return d.pool }

// Ping 健康检查用（health 的 postgres.connected 字段）。
func (d *DB) Ping(ctx context.Context) error { return d.pool.Ping(ctx) }

// Stat 连接池统计（health 的 postgres.pool.total/idle 字段）。
func (d *DB) Stat() *pgxpool.Stat { return d.pool.Stat() }

// Close 优雅退出最后一步调用（硬约束 #10）。
func (d *DB) Close() { d.pool.Close() }

// WithTx 事务辅助：fn 返回错误即回滚，否则提交；panic 向上传播（回滚后由上层 recover，硬约束 #4）。
func (d *DB) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := d.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		// Commit 成功后 Rollback 为 no-op；未提交则回滚，保证连接不泄漏（硬约束 #10）。
		_ = tx.Rollback(ctx)
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}
```

**④ `internal/store/migrate.go`**【完整代码】— golang-migrate 封装（`Up()` 只前进；dirty 状态 panic 退出并提示 `migrate force`）。

```go
package store

import (
	"errors"
	"fmt"
	"strings"

	"deepwiki/migrations"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx" // pgx driver（连接串 scheme: pgx://）
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"go.uber.org/zap"
)

// Migrate 启动时执行全部未应用的迁移（变更总纲 §4.1：golang-migrate + iofs source，只前进原则不变）。
// 任一失败返回错误，启动方必须 panic 退出（启动失败优于带病运行）；
// dirty 状态直接 panic 并提示用 `migrate force <version>` 修复后重启。
func Migrate(dsn string, logger *zap.Logger) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("migrate iofs source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, toPgxURL(dsn))
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			logger.Info("schema up to date")
			return nil
		}
		var dirty migrate.ErrDirty
		if errors.As(err, &dirty) {
			// 库处于 dirty 状态：人工核对后执行 `migrate force <version>` 再重启；禁止自动 force。
			panic(fmt.Sprintf("database schema dirty at version %d; run `migrate force %d` after manual verification, then restart", dirty.Version, dirty.Version))
		}
		return fmt.Errorf("migrate up: %w", err)
	}
	version, dirtyFlag, err := m.Version()
	if err != nil {
		return fmt.Errorf("migrate version: %w", err)
	}
	logger.Info("migrations applied", zap.Uint("version", version), zap.Bool("dirty", dirtyFlag))
	return nil
}

// toPgxURL 把 postgres:// 或 postgresql:// 连接串转为 golang-migrate pgx driver 的 pgx:// scheme；
// 已是 pgx:// 或其他 scheme（如含查询参数的标准串）原样返回。
func toPgxURL(dsn string) string {
	for _, p := range []string{"postgres://", "postgresql://"} {
		if strings.HasPrefix(dsn, p) {
			return "pgx://" + strings.TrimPrefix(dsn, p)
		}
	}
	return dsn
}
```

**⑤ `internal/store/repo_store.go`**【接口完整 + 实现骨架 TODO】— RepoStore（基线 §7，冻结签名）。

```go
package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// RepoStore 仓库仓储（基线 §7，冻结签名）。
type RepoStore interface {
	Create(ctx context.Context, r *model.Repo) error
	Get(ctx context.Context, repoID string) (*model.Repo, error)
	GetByURLBranch(ctx context.Context, url, branch string) (*model.Repo, error)
	Update(ctx context.Context, r *model.Repo) error
	List(ctx context.Context, page, pageSize int) (repos []*model.Repo, total int64, err error)
	// Delete 事务级联：chunks/wiki_pages 随 ON DELETE CASCADE 删除；tasks.repo_id 置 NULL；
	// 事务提交后再删 OpenSearch 索引与本地目录（基线 §12.3 顺序约定）。
	Delete(ctx context.Context, repoID string) error
}

type pgRepoStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewRepoStore 返回 RepoStore 的 PostgreSQL 实现。
func NewRepoStore(db *DB, logger *zap.Logger) RepoStore {
	return &pgRepoStore{pool: db.Pool(), logger: logger}
}

var _ RepoStore = (*pgRepoStore)(nil)

func (s *pgRepoStore) Create(ctx context.Context, r *model.Repo) error {
	// TODO: INSERT INTO repos (repo_id, repo_url, branch, commit_hash, local_path, state, stats_json, created_at, updated_at)
	// VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)；stats_json 用 json.Marshal(r.Stats) 绑定（JSONB 列）；
	// 全部参数化 $n 占位（硬约束 #11）；时间写 time.Now().UTC()，列类型 timestamptz，API 输出 UTC+RFC3339（#13）；
	// UNIQUE(repo_url,branch) 冲突（pg 错误码 23505）映射 model.ErrRepoAlreadyExists。
	panic("TODO: pgRepoStore.Create not implemented")
}

func (s *pgRepoStore) Get(ctx context.Context, repoID string) (*model.Repo, error) {
	// TODO: 按主键查询；pgx.ErrNoRows 映射 model.ErrRepoNotFound；stats_json 反序列化为 model.RepoStats；
	// repoID 入参必须先过 ULID 正则（硬约束 #11）。
	panic("TODO: pgRepoStore.Get not implemented")
}

func (s *pgRepoStore) GetByURLBranch(ctx context.Context, url, branch string) (*model.Repo, error) {
	// TODO: 同 Get；未命中返回 model.ErrRepoNotFound（ingest 幂等判断用，基线 §6.1）。
	panic("TODO: pgRepoStore.GetByURLBranch not implemented")
}

func (s *pgRepoStore) Update(ctx context.Context, r *model.Repo) error {
	// TODO: 全列更新（state/commit_hash/local_path/stats_json/updated_at）；updated_at 由本方法刷新为当前 UTC。
	panic("TODO: pgRepoStore.Update not implemented")
}

func (s *pgRepoStore) List(ctx context.Context, page, pageSize int) ([]*model.Repo, int64, error) {
	// TODO: created_at DESC 排序 + LIMIT $1 OFFSET $2；返回 total（COUNT(*)）；越界返回空 items 与真实 total（基线 §5.4）。
	panic("TODO: pgRepoStore.List not implemented")
}

func (s *pgRepoStore) Delete(ctx context.Context, repoID string) error {
	// TODO: 级联删除（基线 §12.3 顺序约定，变更总纲 §4.1 级联删除矩阵不变）：
	// ① 单事务内 DELETE repos 行（chunks/wiki_pages 靠 ON DELETE CASCADE，tasks.repo_id 靠 ON DELETE SET NULL）；
	// ② 事务提交后再删 OpenSearch 索引（deepwiki-chunks-<repo_id 全小写>）与本地仓库目录，
	//    外部资源失败只记 ERROR 日志并后台重试清理，不回滚 DB。
	panic("TODO: pgRepoStore.Delete not implemented")
}
```

**⑥ `internal/store/chunk_store.go`**【接口完整 + 实现骨架 TODO】— ChunkStore（基线 §7，冻结签名）。

```go
package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// ChunkStore 分块仓储（基线 §7，冻结签名）。
type ChunkStore interface {
	InsertBatch(ctx context.Context, chunks []model.Chunk) error
	GetByID(ctx context.Context, chunkID string) (*model.Chunk, error)
	GetByIDs(ctx context.Context, chunkIDs []string) ([]*model.Chunk, error)
	DeleteByRepo(ctx context.Context, repoID string) error
	// DeleteByPaths refresh 增量删除（modified ∪ deleted 文件对应的 chunks）。
	DeleteByPaths(ctx context.Context, repoID string, paths []string) error
	// FileHashes 按 path 聚合 file_hash，refresh diffing 阶段用（基线 §4.7）。
	FileHashes(ctx context.Context, repoID string) (map[string]string, error)
	Count(ctx context.Context, repoID string) (int64, error)
}

type pgChunkStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

func NewChunkStore(db *DB, logger *zap.Logger) ChunkStore {
	return &pgChunkStore{pool: db.Pool(), logger: logger}
}

var _ ChunkStore = (*pgChunkStore)(nil)

func (s *pgChunkStore) InsertBatch(ctx context.Context, chunks []model.Chunk) error {
	// TODO: 单事务批量 INSERT（pgx.Batch 或 CopyFrom），要求：
	// ① embedding 列绑定 pgvector.NewVector(c.Vector)；Vector 为 nil 时列写 NULL
	//    （解析切分阶段先插文本行，embedding 阶段再经 VectorStore.Upsert 回补向量）；
	// ② 维度不符会被 vector(1536) 列类型直接拒绝（硬约束 #14 第二道防线）；
	// ③ 全部参数化 $n 占位（硬约束 #11）；批次过大时按 500 条分批；时间 UTC（#13）。
	panic("TODO: pgChunkStore.InsertBatch not implemented")
}

func (s *pgChunkStore) GetByID(ctx context.Context, chunkID string) (*model.Chunk, error) {
	// TODO: 主键查询；pgx.ErrNoRows 透传由上层映射（references 校验 chunk_id 存在，硬约束 #15）。
	panic("TODO: pgChunkStore.GetByID not implemented")
}

func (s *pgChunkStore) GetByIDs(ctx context.Context, chunkIDs []string) ([]*model.Chunk, error) {
	// TODO: WHERE chunk_id = ANY($1)（[]string 直接绑定，禁止循环拼接 IN 列表，硬约束 #11）；
	// 检索回填用：OpenSearch 命中 _id 后批量取 Chunk。
	panic("TODO: pgChunkStore.GetByIDs not implemented")
}

func (s *pgChunkStore) DeleteByRepo(ctx context.Context, repoID string) error {
	// TODO: DELETE FROM chunks WHERE repo_id=$1。
	panic("TODO: pgChunkStore.DeleteByRepo not implemented")
}

func (s *pgChunkStore) DeleteByPaths(ctx context.Context, repoID string, paths []string) error {
	// TODO: DELETE FROM chunks WHERE repo_id=$1 AND path = ANY($2)；refresh persisting 事务内调用（基线 §4.7）。
	panic("TODO: pgChunkStore.DeleteByPaths not implemented")
}

func (s *pgChunkStore) FileHashes(ctx context.Context, repoID string) (map[string]string, error) {
	// TODO: SELECT path, file_hash FROM chunks WHERE repo_id=$1 GROUP BY path（同文件多 chunk 取任一）；
	// 返回 map[path]file_hash 供 diffing 比对（基线 §4.7）。
	panic("TODO: pgChunkStore.FileHashes not implemented")
}

func (s *pgChunkStore) Count(ctx context.Context, repoID string) (int64, error) {
	// TODO: SELECT COUNT(*) FROM chunks WHERE repo_id=$1；repo 详情 chunk_count 与启动时
	// OpenSearch 索引文档数对账用（count(index) == chunks 表行数，不一致 WARN 并后台重建，变更总纲 §4.2）。
	panic("TODO: pgChunkStore.Count not implemented")
}
```

**⑦ `internal/store/vector_store.go`**【接口完整 + 实现骨架 TODO】— VectorStore（基线 §7，冻结签名；实现替换为 pgvector：HNSW 索引 + `<=>` 余弦距离算子，业务层零改动）。

```go
package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// VectorStore 向量存储抽象（基线 §7，冻结签名）。
type VectorStore interface {
	Upsert(ctx context.Context, chunks []model.Chunk) error
	Search(ctx context.Context, repoID string, vector []float32, topK int) ([]model.ChunkHit, error)
	DeleteByRepo(ctx context.Context, repoID string) error
}

// pgVectorStore pgvector 实现：embedding vector(1536) 列 + HNSW 索引（变更总纲 §4.1）。
type pgVectorStore struct {
	pool     *pgxpool.Pool
	efSearch int // storage.vector.ef_search，默认 64（可热更新）
	logger   *zap.Logger
}

func NewVectorStore(db *DB, efSearch int, logger *zap.Logger) VectorStore {
	if efSearch <= 0 {
		efSearch = 64
	}
	return &pgVectorStore{pool: db.Pool(), efSearch: efSearch, logger: logger}
}

var _ VectorStore = (*pgVectorStore)(nil)

func (s *pgVectorStore) Upsert(ctx context.Context, chunks []model.Chunk) error {
	// TODO: 与 chunk 行同事务的批量 UPSERT，要求：
	// ① INSERT INTO chunks (...) VALUES (...) ON CONFLICT (chunk_id) DO UPDATE SET
	//    embedding = EXCLUDED.embedding, embedding_model = EXCLUDED.embedding_model；
	// ② 向量绑定用 pgvector.NewVector(c.Vector)；Vector 为 nil 的行跳过 embedding 更新；
	// ③ 维度不符由 vector(1536) 列类型直接拒绝（硬约束 #14 第二道防线），禁止应用层静默截断/补零；
	// ④ 批次按 500 条分批，全部参数化 $n 占位（硬约束 #11）。
	_ = pgvector.NewVector // 提示：本文件统一用 pgvector-go 类型绑定向量
	panic("TODO: pgVectorStore.Upsert not implemented")
}

func (s *pgVectorStore) Search(ctx context.Context, repoID string, vector []float32, topK int) ([]model.ChunkHit, error) {
	// TODO: 在事务内执行变更总纲 §4.1 检索 SQL（SET LOCAL 仅事务内有效；efSearch 为整数配置值拼接，非用户输入），SQL 全文：
	//   SET LOCAL hnsw.ef_search = 64;
	//   SELECT chunk_id, path, start_line, end_line, language, content,
	//          1 - (embedding <=> $2) AS score
	//   FROM chunks
	//   WHERE repo_id = $1
	//     AND ($3::text IS NULL OR path LIKE $3)   -- 按文件路径过滤（进阶要求）
	//   ORDER BY embedding <=> $2
	//   LIMIT $4;
	// 要求：① repoID 先过 ULID 正则（硬约束 #11）；② $2 绑定 pgvector.NewVector(vector)；
	// ③ $3 传 nil 表示不按路径过滤；④ score 为余弦相似度 [0,1]；
	// ⑤ 查询失败映射 model.ErrVectorStoreUnavailable（→ 50203）；
	// ⑥ 注意：ask 默认路径走 internal/retriever.VectorRetriever（检索 SQL 唯一实现处），
	//    本方法供 service 层不经 retriever 的直连场景使用，两处 SQL 必须保持逐字一致。
	panic("TODO: pgVectorStore.Search not implemented")
}

func (s *pgVectorStore) DeleteByRepo(ctx context.Context, repoID string) error {
	// TODO: DELETE FROM chunks WHERE repo_id=$1（向量内联于 chunks 表，删除即生效）；
	// 仓级删除常规路径走 RepoStore.Delete 的 ON DELETE CASCADE，本方法供 refresh 全量重建用。
	panic("TODO: pgVectorStore.DeleteByRepo not implemented")
}
```

**⑧ `internal/store/wiki_store.go`**【接口完整 + 实现骨架 TODO】— WikiStore 与 Wiki 领域类型（基线 §7，冻结签名）。

```go
package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// WikiTOCItem 目录项（slug 为仓内标识，非全局 ID，基线 §5.6）。
type WikiTOCItem struct {
	Slug       string `json:"slug"`
	Title      string `json:"title"`
	ParentSlug string `json:"parent_slug"`
	SortOrder  int    `json:"sort_order"`
}

// WikiPage 页面。
type WikiPage struct {
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	ContentMD string    `json:"content_md"`
	SortOrder int       `json:"sort_order"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Wiki 一仓的完整 Wiki（TOC + 页面）。
type Wiki struct {
	RepoID      string        `json:"repo_id"`
	TOC         []WikiTOCItem `json:"toc"`
	Pages       []WikiPage    `json:"pages"`
	TaskID      string        `json:"task_id"`
	GeneratedAt time.Time     `json:"generated_at"`
}

// WikiStore Wiki 仓储（基线 §7，冻结签名）。
type WikiStore interface {
	// Save 事务内整体覆盖写（先删该 repo 旧 wiki_pages 再插入 toc 行与 page 行）。
	Save(ctx context.Context, w *Wiki) error
	Get(ctx context.Context, repoID string) (*Wiki, error)
	DeleteByRepo(ctx context.Context, repoID string) error
}

type pgWikiStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

func NewWikiStore(db *DB, logger *zap.Logger) WikiStore {
	return &pgWikiStore{pool: db.Pool(), logger: logger}
}

var _ WikiStore = (*pgWikiStore)(nil)

func (s *pgWikiStore) Save(ctx context.Context, w *Wiki) error {
	// TODO: 单事务覆盖写（基线 §12.3 wiki 重建）：
	// ① DELETE FROM wiki_pages WHERE repo_id=$1；② 插入 1 行 kind='toc'（toc_json = json.Marshal(w.TOC)，JSONB 列）
	// + N 行 kind='page'；③ 时间写 time.Now().UTC()，timestamptz 列，API 输出 UTC+RFC3339（硬约束 #13）；
	// ④ 全部参数化 $n 占位（硬约束 #11）。
	_ = json.Marshal // 提示：toc_json 用 encoding/json 序列化
	panic("TODO: pgWikiStore.Save not implemented")
}

func (s *pgWikiStore) Get(ctx context.Context, repoID string) (*Wiki, error) {
	// TODO: 读出 toc 行（解析 toc_json）与全部 page 行（按 sort_order 升序）；
	// 无任何行返回 model.ErrWikiNotFound（→ 40403，基线 §6.7）。
	panic("TODO: pgWikiStore.Get not implemented")
}

func (s *pgWikiStore) DeleteByRepo(ctx context.Context, repoID string) error {
	// TODO: DELETE FROM wiki_pages WHERE repo_id=$1。
	panic("TODO: pgWikiStore.DeleteByRepo not implemented")
}
```

**⑨ `internal/store/apikey_store.go`**【接口完整 + 实现骨架 TODO】— APIKeyStore（《变更总纲》§4.1 R14 新增：环境变量引导 → Postgres 存哈希 → 认证走「Redis 缓存 60s → Postgres」二级查找）。

```go
package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// APIKey API 密钥记录（只存哈希；硬约束 #2：禁止明文进入 Postgres / etcd / 日志）。
type APIKey struct {
	KeyID     string     // key_ + ULID
	Name      string
	KeyHash   string     // SHA-256(salt ‖ key) 十六进制
	Salt      string
	IsAdmin   bool
	RevokedAt *time.Time // nil = 未吊销
	CreatedAt time.Time
}

// APIKeyStore API 密钥仓储（变更总纲 §4.1）。
type APIKeyStore interface {
	// Upsert 启动引导：把 DEEPWIKI_API_KEYS / DEEPWIKI_ADMIN_KEY 中的明文 key 哈希后幂等写入
	//（已存在同 key_hash 则跳过；salt 每 key 随机生成）。
	Upsert(ctx context.Context, k *APIKey) error
	// GetByHash 认证二级查找的 Postgres 端（Redis 缓存 auth:key:<sha256(key)> TTL 60s 在前）。
	GetByHash(ctx context.Context, keyHash string) (*APIKey, error)
	// Revoke 吊销：revoked_at = now()；调用方负责 DEL Redis 缓存 auth:key:<sha256(key)>。
	Revoke(ctx context.Context, keyID string) error
	// Count 启动时判断是否开发模式（0 = 跳过鉴权并 WARN，语义与基线一致）。
	Count(ctx context.Context) (int64, error)
}

type pgAPIKeyStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

func NewAPIKeyStore(db *DB, logger *zap.Logger) APIKeyStore {
	return &pgAPIKeyStore{pool: db.Pool(), logger: logger}
}

var _ APIKeyStore = (*pgAPIKeyStore)(nil)

func (s *pgAPIKeyStore) Upsert(ctx context.Context, k *APIKey) error {
	// TODO: INSERT INTO api_keys (key_id, name, key_hash, salt, is_admin) VALUES ($1,$2,$3,$4,$5)
	// ON CONFLICT (key_hash) DO NOTHING（幂等）；全部参数化（硬约束 #11）；
	// 禁止记录明文 key 到日志（硬约束 #2）。
	panic("TODO: pgAPIKeyStore.Upsert not implemented")
}

func (s *pgAPIKeyStore) GetByHash(ctx context.Context, keyHash string) (*APIKey, error) {
	// TODO: SELECT ... WHERE key_hash=$1 AND revoked_at IS NULL；pgx.ErrNoRows 由上层映射 40101 unauthorized；
	// keyHash 本身是哈希值不是明文，允许作为查询参数，但仍禁止明文 key 入日志（硬约束 #2）。
	panic("TODO: pgAPIKeyStore.GetByHash not implemented")
}

func (s *pgAPIKeyStore) Revoke(ctx context.Context, keyID string) error {
	// TODO: UPDATE api_keys SET revoked_at=now() WHERE key_id=$1 AND revoked_at IS NULL；
	// keyID 必须先过 key_ 前缀 + ULID 正则（硬约束 #11）。
	panic("TODO: pgAPIKeyStore.Revoke not implemented")
}

func (s *pgAPIKeyStore) Count(ctx context.Context) (int64, error) {
	// TODO: SELECT COUNT(*) FROM api_keys WHERE revoked_at IS NULL。
	panic("TODO: pgAPIKeyStore.Count not implemented")
}
```

### 5.7 internal/task —— 统一任务系统（ingest / refresh / wiki 共用）

> **架构变更说明（总纲 R9 / §4.3）**：任务调度链路整体升级为「**Postgres 落任务（pending）→ RabbitMQ 投递瘦消息（publisher confirm）→ 每节点有界 Worker Pool 消费（prefetch = pool_size）→ CAS 抢占执行 → 终态落库 → ack**」。任务状态的**唯一权威来源是 Postgres `tasks` 表**（硬约束 #3）；RabbitMQ 消息为瘦消息（body 只含 `task_id` + `type`，≤ 4KB，**禁止携带任务状态/进度/大对象**，硬约束 #16）；投递失败（confirm 失败/连接断开）必须把任务标记 `failed`（`error.code=50302 queue_unavailable`）。消费必须幂等（at-least-once 语义，硬约束 #18）。
>
> 任务模型、状态机（ingest `pending→cloning→parsing→chunking→embedding→persisting→completed`；refresh `pending→fetching→diffing→chunking→embedding→persisting→completed`；wiki `pending→outlining→generating→completed`；终态 `failed/cancelled`）、进度权重（15/10/10/50/15、20/10/10/45/15、10/90）、取消机制与查询端点契约**全部冻结不变**，仅替换队列与存储实现。

本节先给出新包 `internal/queue`（RabbitMQ 连接与拓扑、投递、消费、启动恢复）的接口与骨架，再给出 `internal/task` 的 4 个文件。

**① `internal/queue/rabbitmq.go`**【接口完整 + 实现骨架 TODO】— 连接管理与拓扑声明（拓扑名称与总纲 §4.3 逐字一致，禁止改名）。

```go
// Package queue RabbitMQ 任务队列：连接管理、拓扑声明、瘦消息投递与消费（总纲 §4.3）。
// 设计要点：RabbitMQ 只承担「任务投递与执行跨进程/跨节点」的传输职责，
// 任务状态唯一来源 = Postgres tasks 表（硬约束 #3）；消息为瘦消息（硬约束 #16）。
package queue

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// 拓扑常量（总纲 §4.3，逐字一致，禁止改名）。
const (
	// ExchangeTasks direct exchange：任务消息经路由键 deepwiki.task.jobs 进入主队列。
	ExchangeTasks = "deepwiki.tasks"
	// QueueJobs 主队列：x-max-length = worker.queue_size（默认 100，硬约束 #6），
	// x-dead-letter-exchange = deepwiki.tasks.dlx（nack requeue=false 的消息进 DLX 重试链）。
	QueueJobs = "deepwiki.task.jobs"
	// ExchangeDLX 死信 exchange（direct）：重试队列与死信队列的汇聚点。
	ExchangeDLX = "deepwiki.tasks.dlx"
	// QueueDLQ 死信队列：重试耗尽（默认 3 次，queue.rabbitmq.max_retries）后的最终归宿；
	// DLQ 消息由 Reconciler/运维巡检消费并落库 failed/50003。
	QueueDLQ = "deepwiki.task.dlq"
	// 重试队列 TTL 链：TTL 到期后经 DLX 回流主队列，实现延迟重试（最多 3 次）。
	QueueRetry5s  = "deepwiki.task.retry.5s"  // x-message-ttl=5000
	QueueRetry30s = "deepwiki.task.retry.30s" // x-message-ttl=30000
	QueueRetry5m  = "deepwiki.task.retry.5m"  // x-message-ttl=300000
)

// TaskMessage 瘦消息协议（硬约束 #16：body ≤ 4KB，只携带 task_id+type；
// 禁止把任务状态/进度/请求大对象塞进消息——状态唯一来源 = Postgres tasks 表，硬约束 #3）。
type TaskMessage struct {
	TaskID string `json:"task_id"` // tsk_ + ULID(26)
	Type   string `json:"type"`    // ingest|refresh|wiki
}

// Conn RabbitMQ 连接封装（拓扑声明、通道工厂、优雅关闭）。
type Conn struct {
	conn       *amqp.Connection
	logger     *zap.Logger
	queueMaxLen int // x-max-length = worker.queue_size（默认 100）
}

// Dial 建立 RabbitMQ 连接（url 仅由环境变量 DEEPWIKI_RABBITMQ_URL 注入，禁止 yaml 明文，硬约束 #2）。
func Dial(ctx context.Context, url string, queueMaxLen int, logger *zap.Logger) (*Conn, error) {
	// TODO: amqp.Dial(url) → 包装为 *Conn；失败返回 error（启动失败优于带病运行，基线 §12.1）。
	// 下一轮可补充：连接断开自动重连 + NotifyClose 监听 + 拓扑重声明。
	panic("TODO: queue.Dial not implemented")
}

// DeclareTopology 声明全部拓扑（幂等，可重复调用；总纲 §4.3）：
//  1. direct exchange deepwiki.tasks 与 deepwiki.tasks.dlx（durable=true）；
//  2. 主队列 deepwiki.task.jobs：durable=true，args{x-max-length=c.queueMaxLen,
//     x-dead-letter-exchange=deepwiki.tasks.dlx}，绑定路由键 deepwiki.task.jobs；
//  3. 重试队列 deepwiki.task.retry.{5s,30s,5m}：durable=true，args{x-message-ttl=5000/30000/300000,
//     x-dead-letter-exchange=deepwiki.tasks, x-dead-letter-routing-key=deepwiki.task.jobs}（TTL 到期回流主队列）；
//  4. 死信队列 deepwiki.task.dlq：durable=true，绑定 deepwiki.tasks.dlx。
//
// 验收口径：RabbitMQ management（http://localhost:15672）可见 deepwiki.task.jobs 且 x-max-length=100。
func (c *Conn) DeclareTopology(ctx context.Context) error {
	// TODO: 开临时 channel 按上述顺序 ExchangeDeclare/QueueDeclare/QueueBind 后关闭；
	// 全部声明完成后 logger.Info("rabbitmq topology declared", zap.String("exchange", ExchangeTasks), zap.String("queue", QueueJobs))。
	panic("TODO: Conn.DeclareTopology not implemented")
}

// Channel 新建 channel（publisher/consumer 各自持独立 channel，禁止跨 goroutine 共享，amqp091-go 线程安全约束）。
func (c *Conn) Channel() (*amqp.Channel, error) {
	return c.conn.Channel()
}

// QueueMaxLen 主队列 x-max-length 配置值（背压预检阈值，Manager.Submit 用）。
func (c *Conn) QueueMaxLen() int { return c.queueMaxLen }

// Close 优雅关闭连接（硬约束 #10：在 consumer 停止、在途消息 nack 处理完毕后最后调用）。
func (c *Conn) Close() error {
	if c.conn != nil && !c.conn.IsClosed() {
		return c.conn.Close()
	}
	return nil
}
```

**② `internal/queue/publisher.go`**【接口完整 + 实现骨架 TODO】— 瘦消息投递（`mandatory=true` + publisher confirm）。

```go
package queue

import (
	"context"
	"errors"

	"go.uber.org/zap"
)

// ErrPublishFailed 投递失败（confirm 未被确认 / mandatory 路由失败 / 连接断开）；
// Manager.Submit 捕获后把任务标记 failed（50302 queue_unavailable，总纲 §6）。
var ErrPublishFailed = errors.New("rabbitmq publish not confirmed")

// Publisher 瘦消息投递器（总纲 §4.3）。实现持有独立 channel 并开启 confirm 模式。
type Publisher interface {
	// Publish 投递瘦消息到 ExchangeTasks（routing key = deepwiki.task.jobs）。
	// mandatory=true：无法路由时 broker 返回 basic.return，必须视为失败；
	// publisher confirm：等待 broker 确认，未确认返回 ErrPublishFailed。
	Publish(ctx context.Context, msg TaskMessage) error
	// QueueDepth 背压预检：QueueDeclarePassive 读主队列 Messages 深度
	//（≥ x-max-length 时 Manager.Submit 拒绝投递 → 42902 + Retry-After，硬约束 #6）。
	QueueDepth(ctx context.Context) (int, error)
	// Close 关闭 channel（不断开共享 Conn）。
	Close() error
}

type amqpPublisher struct {
	conn   *Conn
	logger *zap.Logger
	// TODO（下一轮）：confirm channel、NotifyConfirm/NotifyReturn 监听、序列化缓冲。
}

func NewPublisher(conn *Conn, logger *zap.Logger) Publisher {
	return &amqpPublisher{conn: conn, logger: logger}
}

func (p *amqpPublisher) Publish(ctx context.Context, msg TaskMessage) error {
	// TODO: 实现投递，要求（总纲 §4.3，硬约束 #16）：
	// ① json.Marshal(msg) 为 body（≤ 4KB，天然满足；DeliveryMode=Persistent 持久化，ContentType=application/json）；
	// ② channel.PublishWithContext(ctx, ExchangeTasks, QueueJobs, mandatory=true, immediate=false, ...)；
	// ③ 等待 confirm：ack → 指标 deepwiki_rabbitmq_publish_confirms_total{result="ok"}++；
	//    nack/超时/return → result="fail"++ 并返回 ErrPublishFailed（由 Manager 落库 failed/50302）。
	panic("TODO: amqpPublisher.Publish not implemented")
}

func (p *amqpPublisher) QueueDepth(ctx context.Context) (int, error) {
	// TODO: 独立 channel 上 QueueDeclarePassive(QueueJobs) 读 Messages；队列为声明失败（不存在）按 0 处理并触发拓扑重声明。
	panic("TODO: amqpPublisher.QueueDepth not implemented")
}

func (p *amqpPublisher) Close() error {
	// TODO: 关闭 channel（幂等）。
	return nil
}
```

**③ `internal/queue/consumer.go`**【接口完整 + 实现骨架 TODO】— 消费端（`prefetch = worker.pool_size`，manual ack）。

```go
package queue

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// Consumer RabbitMQ 消费端封装（总纲 §4.3）：
// 每 worker 节点 prefetch = worker.pool_size（默认 2，硬约束 #6 并发上限）；
// manual ack——终态落库成功后才 ack；瞬时错误/panic → nack requeue=false 进 DLX 重试链；
// 优雅退出时未完成消息 nack requeue=true 让别的节点接走（硬约束 #10）。
type Consumer struct {
	conn     *Conn
	prefetch int
	logger   *zap.Logger
	// TODO（下一轮）：消费 channel、consumer tag（Channel.Cancel 停拉新消息用）。
}

func NewConsumer(conn *Conn, prefetch int, logger *zap.Logger) *Consumer {
	return &Consumer{conn: conn, prefetch: prefetch, logger: logger}
}

// Deliveries 启动消费并返回消息通道（autoAck=false，禁止自动确认，硬约束 #18）。
func (c *Consumer) Deliveries(ctx context.Context) (<-chan amqp.Delivery, error) {
	// TODO: 实现消费，要求：
	// ① 独立 channel → Qos(prefetch=c.prefetch, 0, false)（prefetch = pool_size，与 Worker Pool 容量一致）；
	// ② Consume(QueueJobs, consumerTag, autoAck=false, exclusive=false, ...) 返回 <-chan amqp.Delivery；
	// ③ ctx 取消或 Stop 调用 → Channel.Cancel(consumerTag) 停拉新消息（不丢弃在途消息）。
	panic("TODO: Consumer.Deliveries not implemented")
}

// Stop 停拉新消息（优雅退出第一步，硬约束 #10）；在途消息由 Worker Pool 排空或 nack requeue=true。
func (c *Consumer) Stop(ctx context.Context) error {
	// TODO: Channel.Cancel(consumerTag)；幂等。
	return nil
}
```

**④ `internal/queue/reconciler.go`**【接口完整 + 实现骨架 TODO】— 启动恢复（Reconciler，总纲 §4.3）。

```go
package queue

import (
	"context"
	"time"

	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// RecoveryStore Reconciler 所需的 tasks 表最小访问面（由 task 包的具体实现满足；
// 不污染冻结的 TaskStore 接口，见 §5.7 ⑤）。
type RecoveryStore interface {
	// FindInterrupted 查出全部非终态任务（state NOT IN ('completed','failed','cancelled')）。
	FindInterrupted(ctx context.Context) ([]*model.Task, error)
	// ResetStaleRunning running 态且 updated_at 早于 staleBefore（默认 now-5min，无心跳）
	// → 重置为 pending（幂等 CAS：WHERE state NOT IN 终态，硬约束 #18），返回重置行数。
	ResetStaleRunning(ctx context.Context, staleBefore time.Time) (int64, error)
}

// Reconciler 启动恢复器（总纲 §4.3）：worker 启动时扫描 Postgres 中非终态任务——
// pending 且无消费者持有 → 重新投递；running 态且 updated_at 超 5 分钟无心跳 → 重置 pending 重投。
// 幂等 CAS 保证多节点并发恢复安全（硬约束 #18）；worker 崩溃后 Reconciler 从 DB 重建队列视图，
// 消息系统挂一分钟任务一个都不丢（总纲 §10 答辩话术 1）。
type Reconciler struct {
	store RecoveryStore
	pub   Publisher
	logger *zap.Logger
	staleAfter time.Duration // 默认 5min
}

func NewReconciler(store RecoveryStore, pub Publisher, logger *zap.Logger) *Reconciler {
	return &Reconciler{store: store, pub: pub, logger: logger, staleAfter: 5 * time.Minute}
}

// Recover 执行一次启动恢复（main 在 worker pool 启动前调用）。
func (r *Reconciler) Recover(ctx context.Context) error {
	// TODO: 实现启动恢复，要求：
	// ① ResetStaleRunning(time.Now().UTC().Add(-r.staleAfter))：僵死 running 任务重置 pending；
	// ② FindInterrupted 逐任务 publisher.Publish(TaskMessage{TaskID, Type}) 重投瘦消息；
	//    单条投递失败只记 ERROR 并继续（下一轮周期补偿或人工巡检 DLQ），禁止中断整个恢复流程；
	// ③ 恢复完成 logger.Info("reconciler recovered", zap.Int("republished", n))。
	panic("TODO: Reconciler.Recover not implemented")
}
```

**⑤ `internal/task/store.go`**【接口完整 + 实现骨架 TODO】— TaskStore（基线 §7，冻结签名）的 Postgres 实现。注意：`FindInterrupted` 骨架必须返回 `(nil, nil)`（启动恢复路径占位，禁止 panic，否则 main 无法启动）；其余方法 panic TODO。

```go
// Package task 统一任务系统：ingest / refresh / wiki 三类任务共用同一套
// 任务模型、状态机、Worker Pool、查询/取消端点（基线 §4）。
// 存储实现为 Postgres tasks 表（总纲 R1：pgx/v5 + pgxpool，参数化 $n 占位，硬约束 #11）。
package task

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// TaskStore 任务持久化（基线 §7，冻结签名；硬约束 #3：Postgres tasks 表为任务状态唯一来源，
// 禁止用内存 map 作为唯一存储；内存中只允许保存运行中任务的 context.Context 与 cancel 函数）。
type TaskStore interface {
	Create(ctx context.Context, t *model.Task) error
	Get(ctx context.Context, taskID string) (*model.Task, error)
	// UpdateState 内置状态机转移校验（model.CanTransition）；非法转移返回 40902。
	UpdateState(ctx context.Context, taskID string, patch model.TaskPatch) error
	List(ctx context.Context, f model.TaskFilter) (tasks []*model.Task, total int64, err error)
	SetCancelFlag(ctx context.Context, taskID string) error
	// FindInterrupted 启动恢复用：查出全部非终态任务（§4.6，Reconciler 数据源）。
	FindInterrupted(ctx context.Context) ([]*model.Task, error)
}

// postgresTaskStore tasks 表 Postgres 访问实现（pgxpool 连接池；$n 占位参数化，硬约束 #11）。
type postgresTaskStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewTaskStore 返回具体类型指针：同时满足 TaskStore（冻结接口）与 queue.RecoveryStore
//（ResetStaleRunning 为启动恢复扩展方法，不进入冻结接口）。
func NewTaskStore(pool *pgxpool.Pool, logger *zap.Logger) *postgresTaskStore {
	return &postgresTaskStore{pool: pool, logger: logger}
}

var _ TaskStore = (*postgresTaskStore)(nil)

func (s *postgresTaskStore) Create(ctx context.Context, t *model.Task) error {
	// TODO: INSERT INTO tasks 全字段（progress/stats/error/request_payload 序列化为 JSONB）；
	// 参数化 $1..$n（硬约束 #11）；created_at 由调用方给 UTC 时间写入 timestamptz 列，
	// 禁止数据库本地时区（硬约束 #13：全链路 UTC + RFC3339）。
	panic("TODO: postgresTaskStore.Create not implemented")
}

func (s *postgresTaskStore) Get(ctx context.Context, taskID string) (*model.Task, error) {
	// TODO: 主键查询（QueryRow + Scan JSONB 反序列化）；pgx.ErrNoRows 映射 model.ErrTaskNotFound。
	panic("TODO: postgresTaskStore.Get not implemented")
}

func (s *postgresTaskStore) UpdateState(ctx context.Context, taskID string, patch model.TaskPatch) error {
	// TODO: 按 patch 非 nil 字段动态 UPDATE（参数化 $n），要求（§4.3 状态转移规则，冻结）：
	// ① patch.State != nil 时先读当前 state，用 model.CanTransition 校验，非法返回 model.ErrInvalidTaskState；
	// ② 进入终态必须同时写 finished_at；③ 转入 failed 必须带 patch.Err（code/message/stage）；
	// ④ ClearErr=true 时 error_json 置 NULL；⑤ updated_at = now()（timestamptz，心跳语义：Reconciler 据此判僵死）。
	panic("TODO: postgresTaskStore.UpdateState not implemented")
}

func (s *postgresTaskStore) List(ctx context.Context, f model.TaskFilter) ([]*model.Task, int64, error) {
	// TODO: 按 type/state/repo_id 过滤（非空才加条件，参数化 $n）+ created_at DESC + 分页（§5.4）；
	// API 投影不含 cancel_flag/request_payload（model.Task 的 json tag 已保证）。
	panic("TODO: postgresTaskStore.List not implemented")
}

func (s *postgresTaskStore) SetCancelFlag(ctx context.Context, taskID string) error {
	// TODO: UPDATE tasks SET cancel_flag=true WHERE task_id=$1（§4.5 取消机制第一步）。
	panic("TODO: postgresTaskStore.SetCancelFlag not implemented")
}

func (s *postgresTaskStore) FindInterrupted(ctx context.Context) ([]*model.Task, error) {
	// TODO: 查出全部非终态任务（state NOT IN ('completed','failed','cancelled')），
	// 供 queue.Reconciler 重投（pending）或重置（running 僵死）（总纲 §4.3）。
	// 本骨架阶段返回 (nil, nil) 占位，下一轮改为真实查询。
	return nil, nil
}

// ResetStaleRunning 启动恢复扩展（queue.RecoveryStore 契约）：running 态且 updated_at 早于
// staleBefore（默认 5 分钟无心跳）→ 重置 pending（幂等 CAS：WHERE state NOT IN 终态，硬约束 #18）。
func (s *postgresTaskStore) ResetStaleRunning(ctx context.Context, staleBefore time.Time) (int64, error) {
	// TODO: UPDATE tasks SET state='pending', updated_at=now()
	//       WHERE state NOT IN ('completed','failed','cancelled','pending') AND updated_at < $1；
	// 返回 RowsAffected。骨架阶段返回 (0, nil) 占位。
	return 0, nil
}
```

**⑥ `internal/task/executor.go`**【完整代码】— 执行器接口（Worker 按任务类型路由到对应 Pipeline 的内部约定）与消费侧执行路径约定。

```go
package task

import (
	"context"

	"deepwiki/internal/model"
)

// Executor 任务执行器：按 TaskType 注册到 TaskManager，Worker 取出任务后路由执行。
// 实现方（下一轮）：ingestExecutor（五阶段 Pipeline）/ refreshExecutor / wikiExecutor，
// 每阶段入口与循环内必须 select ctx.Done()（硬约束 #4）。
//
// 消费侧执行路径（总纲 §4.3，硬约束 #18 幂等消费）：
//  1. consumer 收到瘦消息 → 读 Postgres 校验任务仍为 pending
//     （CAS：UPDATE tasks SET state='cloning' ... WHERE task_id=$1 AND state='pending'；
//     CAS 失败 = 别的节点已抢占或任务已取消 → 直接 ack 丢弃，禁止重复执行）；
//  2. 路由 Executor.Execute 执行 pipeline，逐阶段 UpdateState + EventBus.Publish；
//  3. 终态落库成功 → ack；panic/recover 或瞬时错误 → nack requeue=false 进 DLX 重试链
//     （deepwiki.task.retry.{5s,30s,5m}，最多 queue.rabbitmq.max_retries=3 次）；
//  4. 重试耗尽 → 任务落库 failed（error.code=50003）。
type Executor interface {
	Type() model.TaskType
	Execute(ctx context.Context, t *model.Task) error
}
```

**⑦ `internal/task/manager.go`**【接口完整 + 实现骨架 TODO】— TaskManager 门面（Handler/Service 只依赖它，不直接碰队列与 Worker，基线 §7 冻结签名）。Submit 新语义：**Postgres 落任务（pending）→ RabbitMQ 投递瘦消息（publisher confirm）→ 投递失败标记 failed/50302**。注意：`Stats` 骨架返回真实 Total 以便 health 验收；`Start/Stop` 骨架为 no-op（禁止 panic）；`Submit/Cancel/Get/List` panic TODO。

```go
package task

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"deepwiki/internal/config"
	"deepwiki/internal/eventbus"
	"deepwiki/internal/model"
	"deepwiki/internal/queue"
)

// TaskManager 任务门面（基线 §7，冻结签名）。
type TaskManager interface {
	// Submit 落库 + 投递；队列满返回 model.ErrQueueFull（映射 42902）；
	// 投递 confirm 失败返回 model.ErrQueueUnavailable（映射 50302，总纲 §6 新增码）。
	Submit(ctx context.Context, t *model.Task) error
	// Cancel 置 cancel_flag + cancel context；终态任务返回 model.ErrInvalidTaskState（映射 40902）。
	Cancel(ctx context.Context, taskID string) error
	Get(ctx context.Context, taskID string) (*model.Task, error)
	List(ctx context.Context, f model.TaskFilter) ([]*model.Task, int64, error)
	Stats() WorkerStats
}

// WorkerStats Worker 池实时状态（health 的 worker 字段，总纲 §7）：
// Queued 语义 = RabbitMQ 主队列深度（QueueDeclarePassive 读 Messages）。
type WorkerStats struct {
	Busy   int `json:"busy"`
	Total  int `json:"total"`
	Queued int `json:"queued"`
}

// Manager TaskManager 实现骨架。
type Manager struct {
	store     TaskStore
	bus       eventbus.EventBus
	publisher queue.Publisher // RabbitMQ 瘦消息投递（confirm+mandatory）
	executors map[model.TaskType]Executor
	pool      *workerPool
	logger    *zap.Logger
	poolSize  int
	queueSize int // x-max-length = worker.queue_size（默认 100，背压预检阈值）
	mu        sync.Mutex
	cancels   map[string]context.CancelFunc // 运行中任务的取消函数（内存仅持 ctx 句柄，状态以 Postgres 为准，硬约束 #3）
}

func NewManager(store TaskStore, bus eventbus.EventBus, publisher queue.Publisher, cfg config.WorkerConfig, logger *zap.Logger) *Manager {
	return &Manager{
		store:     store,
		bus:       bus,
		publisher: publisher,
		executors: make(map[model.TaskType]Executor),
		logger:    logger,
		poolSize:  cfg.PoolSize,
		queueSize: cfg.QueueSize,
		cancels:   make(map[string]context.CancelFunc),
	}
}

var _ TaskManager = (*Manager)(nil)

// RegisterExecutor 注册任务类型执行器（main 装配时调用；下一轮注册 ingest/refresh/wiki 三个实现）。
func (m *Manager) RegisterExecutor(e Executor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executors[e.Type()] = e
}

// Start 启动消费与 Worker Pool（main 在 Reconciler 恢复完成后调用）。骨架阶段 no-op，下一轮实现消费循环。
func (m *Manager) Start(ctx context.Context, consumer *queue.Consumer) {
	// TODO: 启动消费循环（总纲 §4.3）：
	// ① consumer.Deliveries(ctx)（prefetch=pool_size）→ m.pool.Run(ctx, deliveries, m.dispatch)；
	// ② dispatch：解析 TaskMessage → CAS 抢占任务（硬约束 #18，见 executor.go 注释）→ 先查 cancel_flag
	//    （已取消直接落 cancelled 并写 finished_at）→ 写 started_at → 按 type 路由 Executor；
	// ③ 逐阶段 UpdateState + EventBus.Publish（结构化字段，禁止拼字符串）；
	// ④ worker 内全部逻辑包裹 defer recover()：panic → nack requeue=false 进 DLX（硬约束 #4）；
	// ⑤ 进度落库节流：每 500ms 或每推进 5% 一次（二者先到为准，§4.4）；
	// ⑥ 禁止绕过本池 go func 起任务（硬约束 #6）。
}

// Stop 软缩容等待（硬约束 #10 优雅退出）：consumer.Stop 停拉新消息 → 等在途任务完成
//（上限 server.shutdown_timeout）→ 未完成者 nack requeue=true 让别的节点接走。骨架阶段 no-op。
func (m *Manager) Stop(ctx context.Context) {
	// TODO: ① 停拉新消息；② 等待 worker 排空或 ctx 超时；③ 在途未完成任务 nack requeue=true
	//（Reconciler/其他节点接走继续执行，任务不丢）；④ 禁止直接杀 goroutine（硬约束 #4/#10）。
}

func (m *Manager) Submit(ctx context.Context, t *model.Task) error {
	// TODO: 投递路径（总纲 §4.3，硬约束 #3/#6/#16）：
	// ① Postgres 事务内 INSERT tasks（state=pending，queue_position=当前队列深度+1）——状态唯一来源是 Postgres；
	// ② 事务提交后 m.publisher.QueueDepth 预检：深度 ≥ m.queueSize（x-max-length，默认 100）
	//    → 任务落库 failed（error.code=42902）→ 返回 model.ErrQueueFull（映射 429 + 42902 + Retry-After）；
	// ③ m.publisher.Publish 瘦消息（body={"task_id","type"}，mandatory=true + publisher confirm）；
	// ④ queue.ErrPublishFailed → 任务标记 failed（error.code=50302）→ 返回 model.ErrQueueUnavailable
	//    （映射 503 + 50302 queue_unavailable，总纲 §6 新增码）；
	// ⑤ 成功后经 EventBus 发布 task.state_changed（state=pending，含 queue_position；payload 字段冻结）。
	panic("TODO: Manager.Submit not implemented")
}

func (m *Manager) Cancel(ctx context.Context, taskID string) error {
	// TODO: §4.5 取消机制（冻结）：
	// ① 终态任务返回 model.ErrInvalidTaskState（→ 40902）；
	// ② 非终态：SetCancelFlag + 若运行中调 m.cancels[taskID]()；Worker 捕获 ctx.Err() 后落 cancelled；
	//    尚未被消费的 pending 消息被消费时 CAS 失败直接 ack 丢弃（硬约束 #18，天然安全）；
	// ③ API 层返回 202 + 当前 task 快照（state 可能尚未变 cancelled，前端经 SSE/WS 收终态事件）。
	panic("TODO: Manager.Cancel not implemented")
}

func (m *Manager) Get(ctx context.Context, taskID string) (*model.Task, error) {
	// TODO: 委托 m.store.Get。
	panic("TODO: Manager.Get not implemented")
}

func (m *Manager) List(ctx context.Context, f model.TaskFilter) ([]*model.Task, int64, error) {
	// TODO: 委托 m.store.List。
	panic("TODO: Manager.List not implemented")
}

// Stats 骨架：Total 取配置 pool_size；Busy/Queued 下一轮取 workerPool 实时值与
// RabbitMQ 主队列深度（publisher.QueueDepth；health 验收依赖本方法，总纲 §7 worker 字段）。
func (m *Manager) Stats() WorkerStats {
	// TODO: Busy=m.pool.Busy()，Queued=m.publisher.QueueDepth（失败记 WARN 按 0 返回，health 不因此 500）。
	return WorkerStats{Busy: 0, Total: m.poolSize, Queued: 0}
}
```

**⑧ `internal/task/pool.go`**【骨架 TODO】— 有界 Worker Pool（每节点 goroutine 数 = `worker.pool_size`，与 RabbitMQ 消费端 `prefetch = pool_size` 一一对应，总纲 §4.3）。

```go
package task

import (
	"context"
	"sync"
	"sync/atomic"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

// workerPool 有界 Worker 池（硬约束 #6 并发上限：禁止无限制 go func 起后台任务；
// 池容量 = worker.pool_size，默认 2；RabbitMQ prefetch 与之相等，背压由 broker 侧 x-max-length 承担）。
// 原进程内有界队列（互斥锁+条件变量+slice）已整体移除，队列语义由 RabbitMQ 主队列承担。
type workerPool struct {
	size   int
	busy   atomic.Int64
	wg     sync.WaitGroup
	logger *zap.Logger
	// TODO（下一轮）：软扩缩容控制结构（worker 退出信号、resize 通道）。
}

func newWorkerPool(size int, logger *zap.Logger) *workerPool {
	return &workerPool{size: size, logger: logger}
}

// Run 启动 size 个 worker goroutine 消费 deliveries 并把每条消息交给 handle。
// handle 返回 queue 包约定的处理结果语义：ack（终态落库）/ nack requeue=false（进 DLX 重试链）/
// nack requeue=true（优雅退出让渡）。要求（硬约束 #4 并发安全）：
//  ① 每个 worker goroutine 必须 defer recover()：panic → 当前消息 nack requeue=false + 堆栈入日志，worker 继续存活；
//  ② handle 的 ctx 由 pool 派生（ctx 取消 → worker 退出循环，wg.Done）；
//  ③ busy 计数原子增减（health 的 worker.busy 字段）；
//  ④ 禁止在 handle 内再 go func 派生无约束 goroutine。
func (p *workerPool) Run(ctx context.Context, deliveries <-chan amqp.Delivery, handle func(ctx context.Context, d amqp.Delivery)) {
	// TODO: 按上述要求实现 worker 循环。骨架阶段不启动任何 goroutine（no-op）。
}

// Busy 运行中 worker 数（health 的 busy 字段）。
func (p *workerPool) Busy() int {
	// TODO: 返回 int(p.busy.Load())。骨架阶段返回 0。
	return 0
}

// Resize 热扩缩容（config 热更新订阅者调用，§8.2）：
// 扩容即起新 worker（注意与 consumer prefetch 联动调大）；缩容为软缩容——多余 worker 停止取新消息，
// 手头任务完成后自然退出（瞬态超调属预期取舍，§4.4 备注）。
func (p *workerPool) Resize(n int) {
	// TODO: 实现软扩缩容。骨架阶段 no-op。
}
```

### 5.8 internal/eventbus —— 事件总线

> **架构变更说明（总纲 R10 / §4.4）**：事件总线升级为「**Redis Streams（每任务事件日志，可回放）+ Redis Pub/Sub（跨节点扇出）**」——worker 发布事件 = `XADD events:task:<task_id>` + `XTRIM MAXLEN ~ 1000` + `PUBLISH events:fanout <task_id>`；API 节点 `SUBSCRIBE events:fanout` → 命中本节点有 SSE/WS 连接的 task → `XRANGE` 取增量推送；SSE `Last-Event-ID` / WS `resume_from` → `XRANGE events:task:<id> <last> +` 回放。Redis Streams 替代了 v1 原方案的进程内事件缓冲队列，多节点部署下事件可跨进程广播与回放。**事件名与 payload 结构全部冻结**（`task.state_changed` / `task.progress` / `wiki.completed` 等，总纲 §2.5）。

**① `internal/eventbus/bus.go`**【接口完整】— EventBus（基线 §7，冻结签名；WS/SSE Handler 不得直接订阅 Task，只订阅 EventBus，建议⑪）。

```go
// Package eventbus 统一事件总线：任务与系统事件的发布订阅，扇出到 SSE / WebSocket / Logger / Metrics（基线 §2.1）。
// 实现为 Redis Streams + Pub/Sub（总纲 §4.4）：每任务一条事件日志流，跨节点扇出到各 API 节点的本地连接。
package eventbus

import (
	"context"

	"deepwiki/internal/model"
)

// EventBus 事件总线抽象（基线 §7，冻结签名）。
type EventBus interface {
	Publish(ctx context.Context, ev model.Event) error
	// Subscribe 返回事件 channel 与取消订阅函数；channel 容量有界（默认 256），
	// 消费者过慢时丢最旧事件并记 WARN（背压保护，禁止阻塞发布者）。
	Subscribe(filter model.EventFilter) (<-chan model.Event, func() /*取消订阅*/)
}

// Replayer 断线补发（冻结语义平移：SSE Last-Event-ID / WS resume_from → XRANGE 回放）；
// 事件 payload 字段冻结（总纲 §2.5），回放仅改变传输来源（Redis Streams），不改变事件结构。
type Replayer interface {
	// ReplaySince 返回 seq > lastSeq 且匹配 filter 的事件；
	// lastSeq 过旧（对应流已被 XTRIM 截断）无法补发时返回 ok=false
	//（调用方推 event: gap，提示回退 GET /api/v1/tasks 全量同步，§6.4）。
	ReplaySince(ctx context.Context, lastSeq uint64, filter model.EventFilter) (events []model.Event, ok bool)
}
```

**② `internal/eventbus/redis_stream.go`**【骨架 TODO】— 发布与回放（XADD + XTRIM + XRANGE）。

```go
package eventbus

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// Redis 键名与频道（总纲 §4.4，逐字一致，禁止改名）。
const (
	// streamKeyPrefix 每任务事件日志流：events:task:<task_id>（XADD 追加 + XTRIM MAXLEN ~ 1000 截断）。
	streamKeyPrefix = "events:task:"
	// fanoutChannel 跨节点扇出频道：events:fanout（PUBLISH <task_id>，见 redis_fanout.go）。
	fanoutChannel = "events:fanout"
	// streamMaxLen 每任务事件日志近似上限（XTRIM MAXLEN ~ 1000，等价原 1000 条回放窗口语义）。
	streamMaxLen = 1000
	// subChanCap 订阅者 channel 容量（背压保护：满时丢最旧 + WARN）。
	subChanCap = 256
)

type subscriber struct {
	filter model.EventFilter
	ch     chan model.Event
	lastSeq uint64 // 本订阅已扇出到的最大 seq（fanout 增量 XRANGE 起点）
}

// RedisStreamsBus Redis Streams 事件总线实现（总纲 §4.4）。
type RedisStreamsBus struct {
	rdb    redis.UniversalClient
	seq    atomic.Uint64
	mu     sync.Mutex
	subs   map[int]*subscriber
	nextID int
	logger *zap.Logger
}

func NewRedisStreamsBus(rdb redis.UniversalClient, logger *zap.Logger) *RedisStreamsBus {
	return &RedisStreamsBus{
		rdb:    rdb,
		subs:   make(map[int]*subscriber),
		logger: logger,
	}
}

var (
	_ EventBus = (*RedisStreamsBus)(nil)
	_ Replayer = (*RedisStreamsBus)(nil)
)

func (b *RedisStreamsBus) Publish(ctx context.Context, ev model.Event) error {
	// TODO: 实现发布，要求（总纲 §4.4，事件 payload 冻结）：
	// ① b.seq.Add(1) 赋给 ev.Seq、补 ev.Timestamp（UTC，硬约束 #13）；
	// ② XADD events:task:<task_id> * seq <n> type <t> data <json>（结构化 JSON 序列化，禁止拼字符串）；
	// ③ XTRIM events:task:<task_id> MAXLEN ~ 1000（近似截断，保留最近 1000 条供回放）；
	// ④ PUBLISH events:fanout <task_id>（跨节点扇出通知，body 仅 task_id）；
	// ⑤ 本节点本地扇出：命中 filter 的 subscriber 直接投递（本进程内事件免一次网络往返）；
	// ⑥ 指标 deepwiki_redis_op_duration_seconds{op="xadd"} 计时（总纲 §4.8）。
	b.seq.Add(1)
	return nil
}

func (b *RedisStreamsBus) Subscribe(filter model.EventFilter) (<-chan model.Event, func()) {
	// TODO: 注册 subscriber（ch 容量 subChanCap=256），返回 ch 与取消订阅函数
	//（从 map 删除并关闭 ch，防 goroutine 泄漏，硬约束 #4）。
	// 骨架阶段先返回空 channel 占位（events/ws handler 同样为未实现占位，不会订阅）。
	ch := make(chan model.Event, subChanCap)
	return ch, func() {}
}

func (b *RedisStreamsBus) ReplaySince(ctx context.Context, lastSeq uint64, filter model.EventFilter) ([]model.Event, bool) {
	// TODO: 实现回放，要求（总纲 §4.4）：
	// ① filter 必须能定位到 task_id（SSE/WS 连接均绑定任务上下文）；XRANGE events:task:<task_id> <last> + 全量取出；
	// ② 反序列化后按 filter.Types / filter.RepoID 过滤，按 seq 升序返回；
	// ③ lastSeq != 0 且流中最旧 seq > lastSeq+1（流已被 XTRIM 截断）→ 返回 ok=false（调用方推 event: gap，
	//    提示回退 GET /api/v1/tasks 全量同步，§6.4 冻结语义）。
	panic("TODO: RedisStreamsBus.ReplaySince not implemented")
}

// Close 优雅退出时调用（硬约束 #10：关 EventBus 先于关基础设施连接）。
func (b *RedisStreamsBus) Close() {
	// TODO: 关闭全部 subscriber channel（§4.6 / §10.10 优雅退出顺序）。
}
```

**③ `internal/eventbus/redis_fanout.go`**【骨架 TODO】— 跨节点扇出（SUBSCRIBE `events:fanout` → XRANGE 增量 → 本地连接）。

```go
package eventbus

import (
	"context"

	"go.uber.org/zap"
)

// StartFanout 启动跨节点扇出循环（main 以 goroutine 启动；API 节点把任意 worker 节点产生的事件
// 扇入本节点的 SSE/WS 连接，总纲 §4.4）。
// 流程：SUBSCRIBE events:fanout → 收到 task_id → 检查本节点是否有订阅该任务的 subscriber
// → 有则 XRANGE events:task:<task_id> <lastSeq> + 取增量 → 按 filter 匹配投递到 subscriber.ch。
func (b *RedisStreamsBus) StartFanout(ctx context.Context) {
	// TODO: 实现扇出循环，要求：
	// ① rdb.Subscribe(ctx, fanoutChannel) 接收 task_id 通知；断线自动重订阅（go-redis PubSub 重连语义）；
	// ② 命中本节点 subscriber 后按各 subscriber.lastSeq 增量 XRANGE，避免重复推送；
	// ③ subscriber.ch 满（256）时丢最旧并记 WARN（背压保护，禁止阻塞扇出 goroutine）；
	// ④ ctx 取消 → 退订并退出 goroutine；全程 defer recover()（硬约束 #4）；
	// ⑤ 指标 deepwiki_redis_op_duration_seconds{op="fanout"} 计时。
	b.logger.Info("eventbus fanout started", zap.String("channel", fanoutChannel))
}
```

### 5.9 internal/config —— 配置结构体全集与 ConfigManager

> **架构变更说明（总纲 R12 / §4.5）**：运行时配置覆写从数据库表迁往 **etcd v3**——键空间 `/deepwiki/config/<dotted.key>`（覆写值 JSON）、`/deepwiki/config_version`（单调递增）、`/deepwiki/audit/<version>`（审计记录）；加载顺序：viper 读 yaml+env（引导基础设施坐标）→ etcd 全量读前缀覆盖 → `Watch` 增量热更新 → 重建生效快照（atomic.Value）→ 通知订阅者。PUT /config 走 **Merge Patch → 全量校验 → etcd Txn（overrides + version+1 + audit 同一事务原子生效）→ watch 广播全节点**（硬约束 #9）。**密钥仍只走环境变量，禁止写入 yaml/etcd/日志**（硬约束 #2）。

**① `internal/config/config.go`**【完整代码】— Config 结构体全集（与基线 §8.1 及总纲 §5.2 逐 key 一致；json tag 决定 `GET /config` 响应形状，禁止改动；含基础设施凭据的字段 `json:"-"` 不进入响应）。

```go
// Package config 配置结构体、加载与热更新管理（基线 §8；总纲 §5 配置 Schema）。
package config

import "time"

// Config 生效配置全集（yaml 引导 ← 环境变量 ← etcd /deepwiki/config/* 三层深合并结果，总纲 §4.5）。
// Auth 节与基础设施凭据仅由环境变量注入，不进入 GET /config 响应（json:"-"，硬约束 #2）。
type Config struct {
	Server        ServerConfig        `mapstructure:"server" yaml:"server" json:"server"`
	Auth          AuthConfig          `mapstructure:"auth" yaml:"auth" json:"-"`
	RateLimit     RateLimitConfig     `mapstructure:"rate_limit" yaml:"rate_limit" json:"rate_limit"`
	Worker        WorkerConfig        `mapstructure:"worker" yaml:"worker" json:"worker"`
	Ingest        IngestConfig        `mapstructure:"ingest" yaml:"ingest" json:"ingest"`
	Embedding     EmbeddingConfig     `mapstructure:"embedding" yaml:"embedding" json:"embedding"`
	LLM           LLMConfig           `mapstructure:"llm" yaml:"llm" json:"llm"`
	Retriever     RetrieverConfig     `mapstructure:"retriever" yaml:"retriever" json:"retriever"`
	Storage       StorageConfig       `mapstructure:"storage" yaml:"storage" json:"storage"`
	Search        SearchConfig        `mapstructure:"search" yaml:"search" json:"search"`
	Queue         QueueConfig         `mapstructure:"queue" yaml:"queue" json:"queue"`
	Redis         RedisConfig         `mapstructure:"redis" yaml:"redis" json:"redis"`
	Etcd          EtcdConfig          `mapstructure:"etcd" yaml:"etcd" json:"etcd"`
	Git           GitConfig           `mapstructure:"git" yaml:"git" json:"git"`
	Observability ObservabilityConfig `mapstructure:"observability" yaml:"observability" json:"observability"`
}

type ServerConfig struct {
	Addr               string        `mapstructure:"addr" yaml:"addr" json:"addr" validate:"required"` // restart_required
	ReadTimeout        time.Duration `mapstructure:"read_timeout" yaml:"read_timeout" json:"read_timeout" validate:"min=1000000000"`
	ShutdownTimeout    time.Duration `mapstructure:"shutdown_timeout" yaml:"shutdown_timeout" json:"shutdown_timeout" validate:"min=1000000000"`
	CORSAllowedOrigins []string      `mapstructure:"cors_allowed_origins" yaml:"cors_allowed_origins" json:"cors_allowed_origins" validate:"min=1,dive,url"` // 校验禁止 "*"
}

// AuthConfig 仅环境变量注入（DEEPWIKI_API_KEYS / DEEPWIKI_ADMIN_KEY），yaml 不落明文（硬约束 #2）。
// 启动时明文 key 哈希（SHA-256(salt‖key)）后 upsert 进 Postgres api_keys 表（幂等），运行期不持明文（总纲 R14）。
type AuthConfig struct {
	APIKeys  []string `mapstructure:"api_keys" yaml:"api_keys" json:"-"`
	AdminKey string   `mapstructure:"admin_key" yaml:"admin_key" json:"-"`
}

type RateLimitConfig struct {
	PerIP  PerIPConfig  `mapstructure:"per_ip" yaml:"per_ip" json:"per_ip"`
	PerKey PerKeyConfig `mapstructure:"per_key" yaml:"per_key" json:"per_key"`
}

// PerIPConfig L1 per-IP 限流（冻结数值：默认 rps=10 burst=20，作用于全部端点，总纲 §2.8）；
// 跨字段校验 per_ip.burst ≥ per_ip.rps（硬约束 #9 全量校验规则）。
type PerIPConfig struct {
	RPS   int `mapstructure:"rps" yaml:"rps" json:"rps" validate:"min=1"`
	Burst int `mapstructure:"burst" yaml:"burst" json:"burst" validate:"min=1"`
}

// PerKeyConfig L2 per-API-key 配额（冻结数值：ingest_per_hour=20、ask_per_minute=30、wiki_per_hour=10，总纲 §2.8）。
type PerKeyConfig struct {
	IngestPerHour int `mapstructure:"ingest_per_hour" yaml:"ingest_per_hour" json:"ingest_per_hour" validate:"min=1"`
	AskPerMinute  int `mapstructure:"ask_per_minute" yaml:"ask_per_minute" json:"ask_per_minute" validate:"min=1"`
	WikiPerHour   int `mapstructure:"wiki_per_hour" yaml:"wiki_per_hour" json:"wiki_per_hour" validate:"min=1"`
}

// WorkerConfig PoolSize 同时决定 RabbitMQ 消费端 prefetch（queue.rabbitmq.prefetch 缺省取本值，总纲 §5.2）。
type WorkerConfig struct {
	PoolSize  int `mapstructure:"pool_size" yaml:"pool_size" json:"pool_size" validate:"min=1"`
	QueueSize int `mapstructure:"queue_size" yaml:"queue_size" json:"queue_size" validate:"min=1"` // = RabbitMQ 主队列 x-max-length
}

type IngestConfig struct {
	Workdir       string   `mapstructure:"workdir" yaml:"workdir" json:"workdir" validate:"required"`
	MaxRepoSizeMB int      `mapstructure:"max_repo_size_mb" yaml:"max_repo_size_mb" json:"max_repo_size_mb" validate:"min=1"`
	ChunkSize     int      `mapstructure:"chunk_size" yaml:"chunk_size" json:"chunk_size" validate:"min=100"`
	ChunkOverlap  int      `mapstructure:"chunk_overlap" yaml:"chunk_overlap" json:"chunk_overlap" validate:"min=0"` // 跨字段 ≤ chunk_size/2
	IncludeExt    []string `mapstructure:"include_ext" yaml:"include_ext" json:"include_ext" validate:"min=1"`
	ExcludeDirs   []string `mapstructure:"exclude_dirs" yaml:"exclude_dirs" json:"exclude_dirs" validate:"min=1"`
}

type RetryConfig struct {
	Max     int           `mapstructure:"max" yaml:"max" json:"max" validate:"min=0"`
	Backoff time.Duration `mapstructure:"backoff" yaml:"backoff" json:"backoff" validate:"min=100000000"`
}

type EmbeddingConfig struct {
	Provider  string        `mapstructure:"provider" yaml:"provider" json:"provider" validate:"oneof=openai dashscope siliconflow ollama voyage"` // openai|dashscope|siliconflow|ollama|voyage
	Model     string        `mapstructure:"model" yaml:"model" json:"model" validate:"required"`
	APIKey    string        `mapstructure:"api_key" yaml:"api_key" json:"api_key"` // 仅环境变量注入；GET /config 脱敏返回
	BaseURL   string        `mapstructure:"base_url" yaml:"base_url" json:"base_url" validate:"omitempty,url"`
	BatchSize int           `mapstructure:"batch_size" yaml:"batch_size" json:"batch_size" validate:"min=1"`
	Timeout   time.Duration `mapstructure:"timeout" yaml:"timeout" json:"timeout" validate:"min=1000000000"`
	Retry     RetryConfig   `mapstructure:"retry" yaml:"retry" json:"retry"`
}

type LLMConfig struct {
	Provider    string        `mapstructure:"provider" yaml:"provider" json:"provider" validate:"oneof=openai gemini claude ollama deepseek"` // openai|gemini|claude|ollama|deepseek
	Model       string        `mapstructure:"model" yaml:"model" json:"model" validate:"required"`
	APIKey      string        `mapstructure:"api_key" yaml:"api_key" json:"api_key"` // 仅环境变量注入；脱敏返回
	BaseURL     string        `mapstructure:"base_url" yaml:"base_url" json:"base_url" validate:"omitempty,url"`
	Temperature float64       `mapstructure:"temperature" yaml:"temperature" json:"temperature" validate:"min=0,max=2"`
	MaxTokens   int           `mapstructure:"max_tokens" yaml:"max_tokens" json:"max_tokens" validate:"min=1"`
	Timeout     time.Duration `mapstructure:"timeout" yaml:"timeout" json:"timeout" validate:"min=1000000000"`
	Retry       RetryConfig   `mapstructure:"retry" yaml:"retry" json:"retry"`
}

type RetrieverConfig struct {
	Mode string `mapstructure:"mode" yaml:"mode" json:"mode" validate:"oneof=keyword embedding hybrid"` // keyword|embedding|hybrid
	TopK int    `mapstructure:"top_k" yaml:"top_k" json:"top_k" validate:"min=1,max=30"`
	RRFK int    `mapstructure:"rrf_k" yaml:"rrf_k" json:"rrf_k" validate:"min=1"`
}

// StorageConfig Postgres + pgvector（总纲 R1/R2；v1 原方案的 sqlite_path 项已删除）。
type StorageConfig struct {
	Postgres PostgresConfig `mapstructure:"postgres" yaml:"postgres" json:"postgres"`
	Vector   VectorConfig   `mapstructure:"vector" yaml:"vector" json:"vector"`
}

type PostgresConfig struct {
	// DSN 禁止 yaml 明文，仅由环境变量 DEEPWIKI_POSTGRES_DSN 注入（总纲 §5.2；restart_required；json:"-" 不进入 GET /config）。
	DSN      string `mapstructure:"dsn" yaml:"dsn" json:"-"`
	MaxConns int32  `mapstructure:"max_conns" yaml:"max_conns" json:"max_conns" validate:"min=1,max=100"` // pgxpool.MaxConns=10，热更新
}

type VectorConfig struct {
	Dimensions int `mapstructure:"dimensions" yaml:"dimensions" json:"dimensions" validate:"min=1"` // 默认 1536；建表定型 restart_required（改维度 = 新迁移 + 全量重建）
	EFSearch   int `mapstructure:"ef_search" yaml:"ef_search" json:"ef_search" validate:"min=1"`    // HNSW SET LOCAL hnsw.ef_search，默认 64，热更新
}

type SearchConfig struct {
	OpenSearch OpenSearchConfig `mapstructure:"opensearch" yaml:"opensearch" json:"opensearch"`
}

type OpenSearchConfig struct {
	Addresses   []string `mapstructure:"addresses" yaml:"addresses" json:"addresses" validate:"min=1,dive,url"` // restart_required
	Username    string   `mapstructure:"username" yaml:"username" json:"-"`                                     // 仅 env DEEPWIKI_OPENSEARCH_USERNAME
	Password    string   `mapstructure:"password" yaml:"password" json:"-"`                                     // 仅 env DEEPWIKI_OPENSEARCH_PASSWORD
	IndexPrefix string   `mapstructure:"index_prefix" yaml:"index_prefix" json:"index_prefix" validate:"required"` // 默认 deepwiki；索引名 <prefix>-chunks-<repo_id 小写>，restart_required
}

type QueueConfig struct {
	RabbitMQ RabbitMQConfig `mapstructure:"rabbitmq" yaml:"rabbitmq" json:"rabbitmq"`
}

type RabbitMQConfig struct {
	URL        string `mapstructure:"url" yaml:"url" json:"-"`                                            // 仅 env DEEPWIKI_RABBITMQ_URL；restart_required
	Prefetch   int    `mapstructure:"prefetch" yaml:"prefetch" json:"prefetch" validate:"min=1"`          // 缺省 = worker.pool_size；restart_required
	MaxRetries int    `mapstructure:"max_retries" yaml:"max_retries" json:"max_retries" validate:"min=0,max=10"` // DLX 重试链次数，默认 3；热更新
}

type RedisConfig struct {
	Sentinel SentinelConfig `mapstructure:"sentinel" yaml:"sentinel" json:"sentinel"`
	Password string         `mapstructure:"password" yaml:"password" json:"-"` // 仅 env DEEPWIKI_REDIS_PASSWORD
}

type SentinelConfig struct {
	Addresses  []string `mapstructure:"addresses" yaml:"addresses" json:"addresses" validate:"min=1"`     // env DEEPWIKI_REDIS_SENTINEL_ADDRESSES 可覆盖；restart_required
	MasterName string   `mapstructure:"master_name" yaml:"master_name" json:"master_name" validate:"required"` // 默认 deepwiki-master
}

type EtcdConfig struct {
	Endpoints []string `mapstructure:"endpoints" yaml:"endpoints" json:"endpoints" validate:"min=1"` // env DEEPWIKI_ETCD_ENDPOINTS 可覆盖；restart_required
	Prefix    string   `mapstructure:"prefix" yaml:"prefix" json:"prefix" validate:"required,startswith=/"` // 默认 /deepwiki
}

type GitConfig struct {
	OpTimeout  time.Duration `mapstructure:"op_timeout" yaml:"op_timeout" json:"op_timeout" validate:"min=1000000000"` // 单次 git CLI 操作超时，默认 10m；热更新
	BinaryPath string        `mapstructure:"binary_path" yaml:"binary_path" json:"binary_path" validate:"required"`    // 默认 git；restart_required
}

type ObservabilityConfig struct {
	OTelEndpoint string    `mapstructure:"otel_endpoint" yaml:"otel_endpoint" json:"otel_endpoint" validate:"omitempty"` // OTLP gRPC；空 = 禁用（零成本）
	Log          LogConfig `mapstructure:"log" yaml:"log" json:"log"`
}

type LogConfig struct {
	Level  string `mapstructure:"level" yaml:"level" json:"level" validate:"oneof=debug info warn error"`   // 热更新
	Format string `mapstructure:"format" yaml:"format" json:"format" validate:"oneof=json console"`         // json|console
}

// RestartRequiredKeys PUT /config 接受但不当场生效的键清单（总纲 §4.5；响应 restart_required 字段回显）：
// server.addr 及全部基础设施坐标（storage.postgres.dsn、search.opensearch.*、queue.rabbitmq.url、
// queue.rabbitmq.prefetch、redis.*、etcd.*、storage.vector.dimensions、git.binary_path、observability.otel_endpoint）。
var RestartRequiredKeys = []string{
	"server.addr",
	"storage.postgres.dsn",
	"storage.vector.dimensions",
	"search.opensearch.addresses",
	"search.opensearch.username",
	"search.opensearch.password",
	"search.opensearch.index_prefix",
	"queue.rabbitmq.url",
	"queue.rabbitmq.prefetch",
	"redis.sentinel.addresses",
	"redis.sentinel.master_name",
	"redis.password",
	"etcd.endpoints",
	"etcd.prefix",
	"git.binary_path",
	"observability.otel_endpoint",
}
```

**② `internal/config/loader.go`**【完整代码】— viper 引导加载（yaml + 环境变量；环境变量清单与总纲 §5.3 逐字一致）。

```go
package config

import (
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Load 读取 yaml 引导配置 + 环境变量注入（总纲 §4.5 加载顺序第一层；etcd 覆写由 EtcdSource 叠加）。
// 密钥与基础设施凭据只从环境变量读取（硬约束 #2），yaml 中出现明文密钥/凭据时校验应拒绝启动。
// 环境变量清单（总纲 §5.3，逐字一致）：
//
//	DEEPWIKI_API_KEYS / DEEPWIKI_ADMIN_KEY / DEEPWIKI_EMBEDDING_API_KEY / DEEPWIKI_LLM_API_KEY（保留）
//	DEEPWIKI_POSTGRES_DSN / DEEPWIKI_OPENSEARCH_USERNAME / DEEPWIKI_OPENSEARCH_PASSWORD /
//	DEEPWIKI_RABBITMQ_URL / DEEPWIKI_REDIS_SENTINEL_ADDRESSES / DEEPWIKI_REDIS_PASSWORD /
//	DEEPWIKI_ETCD_ENDPOINTS（新增）
func Load(configPath string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(configPath)
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	// 环境变量注入（yaml 不落明文，硬约束 #2）
	cfg.Auth.APIKeys = splitCSV(os.Getenv("DEEPWIKI_API_KEYS"))
	cfg.Auth.AdminKey = os.Getenv("DEEPWIKI_ADMIN_KEY")
	cfg.Embedding.APIKey = os.Getenv("DEEPWIKI_EMBEDDING_API_KEY")
	cfg.LLM.APIKey = os.Getenv("DEEPWIKI_LLM_API_KEY")
	cfg.Storage.Postgres.DSN = os.Getenv("DEEPWIKI_POSTGRES_DSN")
	cfg.Search.OpenSearch.Username = os.Getenv("DEEPWIKI_OPENSEARCH_USERNAME")
	cfg.Search.OpenSearch.Password = os.Getenv("DEEPWIKI_OPENSEARCH_PASSWORD")
	cfg.Queue.RabbitMQ.URL = os.Getenv("DEEPWIKI_RABBITMQ_URL")
	if addrs := os.Getenv("DEEPWIKI_REDIS_SENTINEL_ADDRESSES"); addrs != "" {
		cfg.Redis.Sentinel.Addresses = splitCSV(addrs)
	}
	cfg.Redis.Password = os.Getenv("DEEPWIKI_REDIS_PASSWORD")
	if endpoints := os.Getenv("DEEPWIKI_ETCD_ENDPOINTS"); endpoints != "" {
		cfg.Etcd.Endpoints = splitCSV(endpoints)
	}
	// queue.rabbitmq.prefetch 缺省取 worker.pool_size（总纲 §5.2）
	if cfg.Queue.RabbitMQ.Prefetch <= 0 {
		cfg.Queue.RabbitMQ.Prefetch = cfg.Worker.PoolSize
	}
	return &cfg, nil
}

func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// MaskAPIKey 脱敏规则（§6.5，冻结）：长度 > 8 → 前 3 字符 + "***" + 后 4 字符；否则全 "******"。
func MaskAPIKey(key string) string {
	if len(key) > 8 {
		return key[:3] + "***" + key[len(key)-4:]
	}
	return "******"
}
```

**③ `internal/config/etcd_source.go`**【接口完整 + 实现骨架 TODO】— etcd 配置源（clientv3 连接、全量读前缀、Txn 原子写、Watch 热更新、审计）。

```go
package config

import (
	"context"
	"encoding/json"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

// etcd 键空间（总纲 §4.5，逐字一致；prefix 默认为 /deepwiki，下列为相对 suffix）：
//
//	<prefix>/config/<dotted.key>   # 运行时覆写值（JSON），如 /deepwiki/config/retriever.top_k
//	<prefix>/config_version        # 单调递增整数（与 v1 config_version 语义一致）
//	<prefix>/audit/<version>       # 每次 PUT 的审计记录 {changed_by, change, result, reject_reason, at}
const (
	suffixConfig  = "/config/"
	suffixVersion = "/config_version"
	suffixAudit   = "/audit/"
)

// AuditRecord 配置变更审计记录（写入 <prefix>/audit/<version>，JSON 序列化）。
type AuditRecord struct {
	ChangedBy    string         `json:"changed_by"`    // 脱敏后的 key 标识（硬约束 #2，禁止明文）
	Change       map[string]any `json:"change"`        // 本次 Merge Patch 内容
	Result       string         `json:"result"`        // applied|rejected
	RejectReason string         `json:"reject_reason"` // rejected 时填校验失败摘要
	At           time.Time      `json:"at"`            // UTC（硬约束 #13）
}

// EtcdSource etcd 配置源（总纲 §4.5 / R12）：配置即集群状态，watch 为标准热更新机制，
// Txn 保证「全量校验后原子生效」，revision 天然审计。
type EtcdSource struct {
	cli    *clientv3.Client
	prefix string // 默认 /deepwiki
	logger *zap.Logger
}

// NewEtcdSource 建立 etcd 连接（endpoints 来自 yaml/env 引导层；DialTimeout 5s）。
func NewEtcdSource(ctx context.Context, endpoints []string, prefix string, logger *zap.Logger) (*EtcdSource, error) {
	// TODO: clientv3.New(clientv3.Config{Endpoints: endpoints, DialTimeout: 5*time.Second})；
	// 启动失败优于带病运行（基线 §12.1）。骨架阶段直接 panic 由上层 fatal。
	panic("TODO: NewEtcdSource not implemented")
}

// LoadAll 全量读 <prefix>/config/ 前缀（WithPrefix）+ <prefix>/config_version：
// 返回 dotted.key → 覆写值 JSON 的映射与当前版本号（键不存在时版本号按 1 起）。
// etcd 不可用 → 返回 error，调用方退化为本地快照缓存（总纲 §4.5：读路径可用性优先）。
func (s *EtcdSource) LoadAll(ctx context.Context) (overrides map[string]json.RawMessage, version int64, err error) {
	// TODO: 实现全量读，要求：
	// ① Get(prefix+suffixConfig, clientv3.WithPrefix()) 逐 KV 解析：键后缀为 dotted.key，值为 JSON；
	// ② Get(prefix+suffixVersion) 读版本号（不存在 → version=1）；
	// ③ 指标 deepwiki_etcd_op_duration_seconds{op="load_all"} 计时。
	panic("TODO: EtcdSource.LoadAll not implemented")
}

// ApplyTxn 原子写入（硬约束 #9 全量原子性）：同一 etcd Txn 内
// put 全部 overrides（<prefix>/config/<dotted.key>）+ version+1（<prefix>/config_version）+
// audit（<prefix>/audit/<version>）→ 任一失败整体回滚，不存在「改一半」。
func (s *EtcdSource) ApplyTxn(ctx context.Context, overrides map[string]json.RawMessage, newVersion int64, audit AuditRecord) error {
	// TODO: 实现原子写入，要求：
	// ① cli.Txn(ctx).Then(OpPut×N...).Commit()；Op 列表 = overrides 各键 + config_version(newVersion) + audit 记录；
	// ② audit.At 由调用方给 UTC 时间；ChangedBy 必须已脱敏；
	// ③ etcd 不可用 → 返回 error，Manager 映射 50304 config_store_unavailable（总纲 §6 新增码）；
	// ④ 指标 deepwiki_etcd_op_duration_seconds{op="txn"} 计时。
	panic("TODO: EtcdSource.ApplyTxn not implemented")
}

// Watch 监听 <prefix>/config/ 前缀增量变化（clientv3.WithPrefix）；
// Manager 的 watch 回调据此重建生效快照并通知订阅者（多节点一致可见）。
func (s *EtcdSource) Watch(ctx context.Context) clientv3.WatchChan {
	// TODO: return s.cli.Watch(ctx, s.prefix+suffixConfig, clientv3.WithPrefix())
	panic("TODO: EtcdSource.Watch not implemented")
}

// Healthy 健康探测（后台 60s 探测循环用；etcd 不可用 → health degraded，总纲 §7）。
func (s *EtcdSource) Healthy(ctx context.Context) bool {
	// TODO: ctx 2s 超时 Get(prefix+suffixVersion)（或 Status 任一端点）；成功 true。
	return true
}

// Endpoints 返回当前端点列表（health 的 etcd.endpoints 字段，总纲 §7）。
func (s *EtcdSource) Endpoints() []string {
	// TODO: return s.cli.Endpoints()
	return nil
}

// Close 关闭客户端（优雅退出顺序：etcd 在 Postgres 之后、日志 flush 之前关闭）。
func (s *EtcdSource) Close() error {
	if s.cli != nil {
		return s.cli.Close()
	}
	return nil
}
```

**④ `internal/config/manager.go`**【Load 相关类型完整 + Manager 骨架 TODO】— 生效快照、热更新、脱敏。

```go
package config

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"

	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"
)

// ApplyResult PUT /config 成功结果（§6.5 响应 data 的来源，冻结）。
type ApplyResult struct {
	Version         int64
	Applied         map[string]any
	RestartRequired []string
	Warnings        []string
}

// Manager 配置热更新管理器（§8.2：atomic.Value 持快照；订阅者热生效；
// 覆写持久化在 etcd（总纲 §4.5），v1 原方案的配置覆写表已废弃）。
type Manager struct {
	snapshot atomic.Value // *Config
	version  atomic.Int64
	src      *EtcdSource
	validate *validator.Validate
	mu       sync.Mutex
	subs     []func(*Config)
	logger   *zap.Logger
}

func NewManager(cfg *Config, version int64, src *EtcdSource, logger *zap.Logger) *Manager {
	m := &Manager{
		src:      src,
		validate: validator.New(),
		logger:   logger,
	}
	m.version.Store(version)
	m.snapshot.Store(cfg)
	return m
}

// Get 当前生效配置快照。
func (m *Manager) Get() *Config { return m.snapshot.Load().(*Config) }

// Version 当前配置版本号（与 etcd /deepwiki/config_version 一致）。
func (m *Manager) Version() int64 { return m.version.Load() }

// Subscribe 注册热更新订阅者（RateLimiter / WorkerPool / Retriever 工厂 / Provider 注册表 / Logger，§8.2）。
func (m *Manager) Subscribe(fn func(*Config)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subs = append(m.subs, fn)
}

// Masked 返回脱敏副本（GET /config 用；Auth 节与环境变量注入项不出现，§6.5，硬约束 #2）。
func (m *Manager) Masked() *Config {
	src := *m.Get()
	src.Embedding.APIKey = MaskAPIKey(src.Embedding.APIKey)
	src.LLM.APIKey = MaskAPIKey(src.LLM.APIKey)
	src.Auth = AuthConfig{}
	return &src
}

// StartWatch 启动 etcd watch 循环（main 以 goroutine 启动）：
// <prefix>/config/ 前缀任一变化 → 全量重读 → 深合并重建生效快照（atomic.Value 原子替换）
// → 通知全部订阅者；watch 断流自动重建（etcd 不可用期间读走本地快照缓存，GET 路径不报错）。
func (m *Manager) StartWatch(ctx context.Context) {
	// TODO: 实现 watch 循环，要求（总纲 §4.5，硬约束 #9）：
	// ① m.src.Watch(ctx) 消费事件流；任一 Put/Delete → m.src.LoadAll 全量重读 + 深合并到引导配置 →
	//    全量校验通过后原子替换 snapshot（version 同步）、依次调用订阅者；
	// ② watch channel 关闭（断流）→ 退避 1s 重建 watch；ctx 取消 → 退出；
	// ③ 全程 defer recover()（硬约束 #4）；启动日志 etcd watch established。
	m.logger.Info("etcd watch established", zap.String("prefix", m.src.prefix+suffixConfig))
}

// Apply JSON Merge Patch 语义部分更新（§6.5）。
func (m *Manager) Apply(ctx context.Context, patch json.RawMessage, changedBy string) (*ApplyResult, error) {
	// TODO: 实现动态配置更新，要求（硬约束 #9 全量原子性）：
	// ① Merge Patch 合并到当前快照得到候选配置；
	// ② 全量校验（§8.3）：validator tag（范围/枚举/格式，见 config.go 各 validate 列）+ 跨字段
	//    chunk_overlap ≤ chunk_size/2、per_ip.burst ≥ per_ip.rps + embedding 维度探测（provider/model/base_url
	//    变更且库中有 chunks 时，以新配置 Embed(["dimension probe"]) 比对维度，不一致或探测失败 → 拒绝并提示
	//    重建索引，硬约束 #14）；
	// ③ 任一失败 → 整体拒绝保持旧值，返回 42201 + details 字段级明细，写审计 result=rejected
	//    （审计写 etcd /deepwiki/audit/<version>，changedBy 已脱敏）；
	// ④ 成功 → m.src.ApplyTxn 原子写入（overrides + version+1 + audit 同一事务）→ watch 回调驱动本节点
	//    与其他节点同步生效（快照替换 + 通知订阅者），写审计 result=applied；
	// ⑤ restart_required 项（RestartRequiredKeys 清单：server.addr 与全部基础设施坐标）允许写入不当场生效，
	//    列入响应 restart_required；
	// ⑥ embedding 变更且库中无数据 → 放行并附 warnings ["embedding provider changed, existing index may need rebuild"]；
	// ⑦ etcd 不可用 → 返回 50304 config_store_unavailable（GET /config 走快照缓存不报错，总纲 §4.5/§6）；
	// ⑧ 密钥字段（embedding.api_key / llm.api_key）只允许环境变量注入，PUT 中携带一律 40001 拒绝（硬约束 #2 扩展）。
	panic("TODO: Manager.Apply not implemented")
}
```

### 5.10 internal/service —— 业务编排层（全部【骨架 TODO】）

> 说明：`internal/api/dto` 为纯数据结构包（只依赖 `model` / `config`，无任何行为），service 层与 handler 层共享它，不违反依赖方向铁律。编排层只依赖领域接口与基础设施接口（`queue.Publisher` / `search.Client`），不依赖任何 provider SDK 具体类型（硬约束 #17）。

**① `internal/service/ingest_service.go`** — 摄取/刷新编排。

```go
// Package service 业务编排层：只依赖领域接口，不依赖任何 provider SDK 具体类型（基线 §2.2，硬约束 #17）。
package service

import (
	"context"

	"go.uber.org/zap"

	"deepwiki/internal/api/dto"
	"deepwiki/internal/config"
	"deepwiki/internal/ingest"
	"deepwiki/internal/model"
	"deepwiki/internal/queue"
	"deepwiki/internal/store"
	"deepwiki/internal/task"
)

// IngestService 摄取与刷新编排。
type IngestService struct {
	tm        task.TaskManager
	repos     store.RepoStore
	cloner    ingest.Cloner
	publisher queue.Publisher // RabbitMQ 背压预检（QueueDepth），避免白做 LsRemote
	cfg       *config.Manager
	logger    *zap.Logger
}

func NewIngestService(tm task.TaskManager, repos store.RepoStore, cloner ingest.Cloner, publisher queue.Publisher, cfg *config.Manager, logger *zap.Logger) *IngestService {
	return &IngestService{tm: tm, repos: repos, cloner: cloner, publisher: publisher, cfg: cfg, logger: logger}
}

// SubmitIngest POST /api/v1/ingest 的业务实现（§6.1）。
func (s *IngestService) SubmitIngest(ctx context.Context, req dto.IngestRequest) (*model.Task, *model.Repo, error) {
	// TODO: 实现摄取提交，要求：
	// ① s.publisher.QueueDepth 背压预检：≥ x-max-length（worker.queue_size，默认 100）→ 直接返回
	//    model.ErrQueueFull（42902），避免白做 LsRemote（总纲 §4.3 背压契约）；
	// ② cloner.LsRemote 取远端 HEAD commit（git ls-remote，失败放行并记 WARN，§6.1）；
	// ③ 与 repos.GetByURLBranch 比对：commit 未变 → 40901（details 附 existing_repo_id）；
	//    commit 已变 → 40901（details.issue=use_refresh）；
	// ④ 生成 repo_id（"repo_"+ULID）与 task_id（"tsk_"+ULID），Repo 预创建 state=ingesting；
	// ⑤ Task{Type:ingest, State:pending, RequestPayload: 原始请求快照} 经 tm.Submit 提交
	//    （内部：Postgres 落 pending → RabbitMQ 瘦消息 confirm 投递；confirm 失败 → 50302）；
	// ⑥ options 缺省值取 ingest.* 配置（请求 options 覆盖配置，§6.1 校验规则表）。
	panic("TODO: IngestService.SubmitIngest not implemented")
}

// SubmitRefresh POST /api/v1/repos/{repo_id}/refresh 的业务实现（§4.7、§6.7）。
func (s *IngestService) SubmitRefresh(ctx context.Context, repoID string) (*model.Task, error) {
	// TODO: 实现刷新提交，要求：
	// ① repoID 先过 ULID 正则（硬约束 #11）；② 仓库不存在 → 40402；非 ready 或有进行中任务冲突 → 40902；
	// ③ 同仓 refresh 互斥经 Redis 分布式锁 lock:refresh:<repo_id>（SET NX PX 300000，总纲 R13；
	//    多 worker 节点下 v1 原方案的进程内去重机制已失效，必须分布式互斥；持锁失败 → 40902）；
	// ④ 构造 Task{Type:refresh} 经 tm.Submit 提交（Pipeline：fetching→diffing→chunking→embedding→persisting；
	//    git CLI fetch --depth 1 + reset --hard FETCH_HEAD + clean -fdx，禁止 git pull，硬约束 #5）。
	panic("TODO: IngestService.SubmitRefresh not implemented")
}
```

**② `internal/service/repo_service.go`** — 仓库资源族。

```go
package service

import (
	"context"

	"go.uber.org/zap"

	"deepwiki/internal/model"
	"deepwiki/internal/search"
	"deepwiki/internal/store"
	"deepwiki/internal/task"
)

// RepoService 仓库资源族（列表/详情/删除）。
type RepoService struct {
	repos     store.RepoStore
	chunks    store.ChunkStore
	vectors   store.VectorStore
	wikis     store.WikiStore
	searchCli search.Client // OpenSearch 索引生命周期（删除仓库时删索引，总纲 §4.2）
	tm        task.TaskManager
	logger    *zap.Logger
}

func NewRepoService(repos store.RepoStore, chunks store.ChunkStore, vectors store.VectorStore, wikis store.WikiStore, searchCli search.Client, tm task.TaskManager, logger *zap.Logger) *RepoService {
	return &RepoService{repos: repos, chunks: chunks, vectors: vectors, wikis: wikis, searchCli: searchCli, tm: tm, logger: logger}
}

// ListRepos GET /api/v1/repos（§5.4 分页，created_at DESC）。
func (s *RepoService) ListRepos(ctx context.Context, page, pageSize int) ([]*model.Repo, int64, error) {
	// TODO: 委托 repos.List；page<1 按 1 处理，pageSize 钳制 1~100 默认 20（§5.4）。
	panic("TODO: RepoService.ListRepos not implemented")
}

// GetRepo GET /api/v1/repos/{repo_id}（§6.7：详情 + latest_task + wiki_available + chunk_count）。
func (s *RepoService) GetRepo(ctx context.Context, repoID string) (*model.Repo, error) {
	// TODO: repos.Get；未命中 → 40402。（latest_task/wiki_available/chunk_count 由 handler 装配或本方法扩展结构，下一轮定）
	panic("TODO: RepoService.GetRepo not implemented")
}

// DeleteRepo DELETE /api/v1/repos/{repo_id}（§12.3 级联矩阵与顺序约定，总纲 §4.1 不变）。
func (s *RepoService) DeleteRepo(ctx context.Context, repoID string) error {
	// TODO: ① repoID ULID 正则校验（硬约束 #11）；
	// ② repos.Delete（DB 事务级联：chunks/wiki_pages CASCADE、tasks.repo_id 置 NULL）；
	// ③ 事务提交后 s.searchCli.DeleteIndex("deepwiki-chunks-"+strings.ToLower(repoID))
	//    （OpenSearch 索引，替代 v1 原方案的 bleve 索引目录）+ 删本地仓库目录；
	//    外部资源失败只记 ERROR 并后台重试清理，不回滚 DB（§12.3，总纲 §4.1）。
	panic("TODO: RepoService.DeleteRepo not implemented")
}
```

**③ `internal/service/ask_service.go`** — 问答编排（RAG 链路）。

```go
package service

import (
	"context"

	"go.uber.org/zap"

	"deepwiki/internal/api/dto"
	"deepwiki/internal/config"
	"deepwiki/internal/llm"
	"deepwiki/internal/model"
	"deepwiki/internal/retriever"
)

// AskService 问答编排（§6.2、§6.3；Ask 不得直接访问 pgvector/OpenSearch，只面向 Retriever 接口，建议⑥——
// keyword 实现走 OpenSearch BM25（总纲 §4.2），embedding 实现走 pgvector HNSW（总纲 §4.1），hybrid 为 RRF 融合）。
type AskService struct {
	cfg        *config.Manager
	retrievers map[string]retriever.Retriever // key = mode：keyword|embedding|hybrid
	llm        llm.LLM
	logger     *zap.Logger
}

func NewAskService(cfg *config.Manager, retrievers map[string]retriever.Retriever, l llm.LLM, logger *zap.Logger) *AskService {
	return &AskService{cfg: cfg, retrievers: retrievers, llm: l, logger: logger}
}

// Ask POST /api/v1/ask（§6.2）。
func (s *AskService) Ask(ctx context.Context, req dto.AskRequest) (*dto.AskResponse, error) {
	// TODO: 实现非流式问答，要求：
	// ① 仓库须 state=ready，否则 40902；② mode 缺省取 retriever.mode 配置，top_k 缺省取 retriever.top_k（1~30），
	//    temperature 省略取 llm.temperature 且钳制 [0,2]；
	// ③ 按 mode 选 Retriever.Search 取 hits（topK）；vector 路径先 Embed 查询向量再走 pgvector <=> 余弦距离
	//    （SET LOCAL hnsw.ef_search 取 storage.vector.ef_search，默认 64）；
	// ④ 组装 prompt：system 明确"只能依据给定片段作答，引用格式 [path:start-end]，禁止编造行号与文件路径，
	//    片段不足时回答'未在仓库中找到相关代码'"（硬约束 #15）；
	// ⑤ llm.Generate 生成回答；references 就是 Retriever 返回的 hits（禁止从 LLM 输出解析），
	//    响应装配前逐一校验 chunk_id 存在（硬约束 #15）；
	// ⑥ usage 来自 provider；缺失时按 tokens≈ceil(len([]rune(content))/4) 估算兜底（§12.4）；
	// ⑦ 错误映射：LLM 不可用 → 50201；Embedding 不可用 → 50202；OpenSearch 不可用 → 50303 search_unavailable；
	//    pgvector 查询失败 → 50203 vector_store_unavailable（总纲 §6 新增码）；mode 回显实际生效值。
	panic("TODO: AskService.Ask not implemented")
}

// AskStream POST /api/v1/ask/stream 的业务实现（§6.3；sink 由 handler 提供，负责 SSE 帧写出）。
func (s *AskService) AskStream(ctx context.Context, req dto.AskRequest, sink func(event string, payload any) error) error {
	// TODO: 实现流式问答，要求：
	// ① 事件顺序：references（恰好 1 个）→ token（0~N 个）→ done（恰好 1 个）；error 可出现在任意位置并终止流；
	// ② 检索完成立即 sink("references", ...)；llm.GenerateStream 的每个 delta → sink("token", {delta})；
	// ③ 结束 sink("done", {usage, latency_ms})；provider 不返回 usage 时估算兜底；
	// ④ 客户端断开 → ctx 取消 → LLM 流中断、goroutine 退出，不补偿不重放（§6.3）；
	// ⑤ 其余约束与错误映射同 Ask ③~⑦。
	panic("TODO: AskService.AskStream not implemented")
}
```

**④ `internal/service/wiki_service.go`** — Wiki 生成与获取。

```go
package service

import (
	"context"

	"go.uber.org/zap"

	"deepwiki/internal/model"
	"deepwiki/internal/store"
	"deepwiki/internal/task"
)

// WikiService Wiki 编排（与 ingest 共用同一套任务系统，建议⑩）。
type WikiService struct {
	tm     task.TaskManager
	wikis  store.WikiStore
	logger *zap.Logger
}

func NewWikiService(tm task.TaskManager, wikis store.WikiStore, logger *zap.Logger) *WikiService {
	return &WikiService{tm: tm, wikis: wikis, logger: logger}
}

// Generate POST /api/v1/wiki/generate（§6.7）：返回 202 + task_id；已有 wiki 则覆盖重建。
func (s *WikiService) Generate(ctx context.Context, repoID string) (*model.Task, error) {
	// TODO: ① repoID ULID 正则校验；仓库须 ready，否则 40902；② 构造 Task{Type:wiki, State:pending} 经 tm.Submit 提交
	// （wiki 状态机 pending→outlining→generating→completed，§4.3；L2 配额走 wiki_per_hour=10，总纲 §2.8）；
	// ③ auto_wiki=true 的 ingest 进入 completed 后由 TaskManager 自动调用本方法（§4.3 级联联动）。
	panic("TODO: WikiService.Generate not implemented")
}

// GetWiki GET /api/v1/repos/{repo_id}/wiki（§6.7）：未生成 → 40403。
func (s *WikiService) GetWiki(ctx context.Context, repoID string) (*store.Wiki, error) {
	// TODO: 委托 wikis.Get；model.ErrWikiNotFound → 40403。
	panic("TODO: WikiService.GetWiki not implemented")
}
```

### 5.11 internal/api —— DTO、中间件、Handler、路由

#### 5.11.1 internal/api/dto（全部【完整代码】，与基线 §5、§6 API 契约逐字段一致；health 响应按总纲 §7 新契约）

**① `internal/api/dto/ingest.go`** — 摄取请求与 202 响应。

```go
// Package dto API 请求/响应数据结构（纯数据，无行为；与基线 §5、§6 契约一致）。
package dto

// IngestRequest POST /api/v1/ingest 请求体（§6.1）。
type IngestRequest struct {
	RepoURL  string            `json:"repo_url" binding:"required"`
	Branch   string            `json:"branch"`
	AutoWiki bool              `json:"auto_wiki"`
	Options  *IngestOptionsDTO `json:"options"`
}

// IngestOptionsDTO 本次任务的摄取参数覆盖（缺省取 ingest.* 配置）。
type IngestOptionsDTO struct {
	ChunkSize    *int     `json:"chunk_size"`
	ChunkOverlap *int     `json:"chunk_overlap"`
	IncludeExt   []string `json:"include_ext"`
	ExcludeDirs  []string `json:"exclude_dirs"`
}

// TaskSubmittedResponse 建任务类端点（ingest/refresh/wiki）202 响应 data（§6.1、§6.7）。
type TaskSubmittedResponse struct {
	TaskID        string `json:"task_id"`
	RepoID        string `json:"repo_id"`
	Type          string `json:"type"`
	State         string `json:"state"`
	QueuePosition int    `json:"queue_position"`
	CreatedAt     string `json:"created_at"`
}
```

**② `internal/api/dto/ask.go`** — 问答请求/响应与 SSE 事件 payload（§6.2、§6.3）。

```go
package dto

// AskRequest POST /api/v1/ask 与 /ask/stream 共用请求体（§6.2）。
type AskRequest struct {
	RepoID      string   `json:"repo_id" binding:"required"`
	Question    string   `json:"question" binding:"required"`
	Mode        string   `json:"mode"` // keyword|embedding|hybrid，缺省取配置
	TopK        *int     `json:"top_k"`
	Temperature *float64 `json:"temperature"`
}

// ReferenceDTO 引用片段（必须来自真实检索结果，硬约束 #15）。
type ReferenceDTO struct {
	ChunkID   string  `json:"chunk_id"`
	Path      string  `json:"path"`
	StartLine int     `json:"start_line"`
	EndLine   int     `json:"end_line"`
	Language  string  `json:"language"`
	Score     float64 `json:"score"`
	Snippet   string  `json:"snippet"`
}

// UsageDTO token 用量。
type UsageDTO struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// AskResponse POST /api/v1/ask 200 响应 data（§6.2）。
type AskResponse struct {
	Answer     string         `json:"answer"`
	References []ReferenceDTO `json:"references"`
	Mode       string         `json:"mode"`
	Usage      UsageDTO       `json:"usage"`
	LatencyMs  int64          `json:"latency_ms"`
}

// StreamReferencesEvent SSE references 事件 payload（§6.3）。
type StreamReferencesEvent struct {
	RequestID  string         `json:"request_id"`
	Mode       string         `json:"mode"`
	References []ReferenceDTO `json:"references"`
}

// StreamTokenEvent SSE token 事件 payload。
type StreamTokenEvent struct {
	Delta string `json:"delta"`
}

// StreamDoneEvent SSE done 事件 payload。
type StreamDoneEvent struct {
	Usage     UsageDTO `json:"usage"`
	LatencyMs int64    `json:"latency_ms"`
}

// StreamErrorEvent SSE error 事件 payload（任意阶段失败，此前已推送的事件不回滚）。
type StreamErrorEvent struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}
```

**③ `internal/api/dto/config.go`** — 动态配置响应（§6.5）。

```go
package dto

import "deepwiki/internal/config"

// ConfigResponse GET /api/v1/config 200 响应 data（Config 为脱敏副本，§6.5）。
type ConfigResponse struct {
	Version         int64         `json:"version"`
	Config          config.Config `json:"config"`
	RestartRequired []string      `json:"restart_required"`
}

// ConfigUpdateResponse PUT /api/v1/config 200 响应 data。
type ConfigUpdateResponse struct {
	Version         int64          `json:"version"`
	Applied         map[string]any `json:"applied"`
	RestartRequired []string       `json:"restart_required"`
	Warnings        []string       `json:"warnings"`
}
```

**④ `internal/api/dto/pagination.go`** — 分页结构（§5.4）。

```go
package dto

// Pagination 分页元信息（排序固定 created_at DESC）。
type Pagination struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}

// PageResult 分页响应 data：{ items, pagination }；越界返回空 items 与真实 total，不报错。
type PageResult[T any] struct {
	Items      []T        `json:"items"`
	Pagination Pagination `json:"pagination"`
}
```

**⑤ `internal/api/dto/health.go`** — 健康检查响应（**总纲 §7 新契约**，逐字段一致）。

```go
package dto

// LLMHealth llm 依赖状态（breaker 为 gobreaker 状态：closed|open|half-open，总纲 R8）。
type LLMHealth struct {
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Reachable bool   `json:"reachable"`
	Breaker   string `json:"breaker"`
}

// EmbeddingHealth embedding 依赖状态（dimensions 未知时省略）。
type EmbeddingHealth struct {
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Dimensions int    `json:"dimensions,omitempty"`
	Reachable  bool   `json:"reachable"`
	Breaker    string `json:"breaker"`
}

// PgPoolHealth pgxpool 连接池实时状态。
type PgPoolHealth struct {
	Total int32 `json:"total"`
	Idle  int32 `json:"idle"`
}

// PostgresHealth Postgres 依赖状态（migration_version 来自 golang-migrate schema_migrations）。
type PostgresHealth struct {
	Connected        bool          `json:"connected"`
	Pool             PgPoolHealth  `json:"pool"`
	MigrationVersion uint          `json:"migration_version"`
}

// OpenSearchHealth OpenSearch 依赖状态（cluster_status：green|yellow|red；indices 为 deepwiki-* 索引数）。
type OpenSearchHealth struct {
	Connected     bool   `json:"connected"`
	ClusterStatus string `json:"cluster_status"`
	Indices       int    `json:"indices"`
}

// RabbitMQHealth RabbitMQ 依赖状态（queue_depth = 主队列 deepwiki.task.jobs 深度；consumers = 消费者数）。
type RabbitMQHealth struct {
	Connected  bool `json:"connected"`
	QueueDepth int  `json:"queue_depth"`
	Consumers  int  `json:"consumers"`
}

// RedisHealth Redis 依赖状态（mode=sentinel；master 为哨兵发现的当前主地址；
// ratelimit_degraded = Redis 不可用时限流已降级进程内兜底，总纲 §4.4）。
type RedisHealth struct {
	Connected         bool   `json:"connected"`
	Mode              string `json:"mode"`
	Master            string `json:"master"`
	RatelimitDegraded bool   `json:"ratelimit_degraded"`
}

// EtcdHealth etcd 依赖状态。
type EtcdHealth struct {
	Connected bool     `json:"connected"`
	Endpoints []string `json:"endpoints"`
}

// GitHealth git CLI 可用性（启动时 git --version 解析，缺失 → degraded，总纲 §4.6）。
type GitHealth struct {
	Available bool   `json:"available"`
	Version   string `json:"version"`
}

// WorkerHealth worker 池实时状态（queued = RabbitMQ 主队列深度）。
type WorkerHealth struct {
	Busy   int `json:"busy"`
	Total  int `json:"total"`
	Queued int `json:"queued"`
}

// HealthResponse GET /api/v1/health 200 响应 data（总纲 §7 新契约；v1 原方案的 sqlite 字段已整体移除）。
type HealthResponse struct {
	Status        string          `json:"status"` // ok|degraded
	Version       string          `json:"version"`
	UptimeSeconds int64           `json:"uptime_seconds"`
	StartedAt     string          `json:"started_at"`
	LLM           LLMHealth       `json:"llm"`
	Embedding     EmbeddingHealth `json:"embedding"`
	Postgres      PostgresHealth  `json:"postgres"`
	OpenSearch    OpenSearchHealth `json:"opensearch"`
	RabbitMQ      RabbitMQHealth  `json:"rabbitmq"`
	Redis         RedisHealth     `json:"redis"`
	Etcd          EtcdHealth      `json:"etcd"`
	Git           GitHealth       `json:"git"`
	Worker        WorkerHealth    `json:"worker"`
}
```

#### 5.11.2 internal/api/middleware

**① `internal/api/middleware/requestid.go`**【完整代码】— RequestID 中间件（`req_` + ULID；写 `X-Request-ID` 响应头与 gin.Context，§5.2）。

```go
// Package middleware Gin 中间件链：RequestID → Recovery → CORS → Auth → RateLimit。
package middleware

import (
	crand "crypto/rand"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oklog/ulid/v2"
)

// ContextKeyRequestID gin.Context 中 request_id 的键。
const ContextKeyRequestID = "request_id"

// RequestID 生成或透传请求 ID（req_ + ULID；ULID 字典序与时间序一致，利索引与排序，§5.6）。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if rid == "" {
			rid = "req_" + ulid.MustNew(ulid.Timestamp(time.Now().UTC()), ulid.Monotonic(crand.Reader, 0)).String()
		}
		c.Set(ContextKeyRequestID, rid)
		c.Header("X-Request-ID", rid)
		c.Next()
	}
}

// GetRequestID 从 gin.Context 取 request_id（handler 装配信封用）。
func GetRequestID(c *gin.Context) string {
	return c.GetString(ContextKeyRequestID)
}

// NewULID 生成带类型前缀的 ID（tsk_/repo_/chk_；§5.6）。各模块实现阶段统一使用本函数。
func NewULID(prefix string) string {
	return prefix + ulid.MustNew(ulid.Timestamp(time.Now().UTC()), ulid.Monotonic(crand.Reader, 0)).String()
}
```

**② `internal/api/middleware/recovery.go`**【完整代码】— Recovery 中间件（panic → zap 堆栈 + 50001 信封；硬约束 #4、#8）。

```go
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// Recovery panic 恢复：原始错误（含堆栈）只进 zap 日志，响应为脱敏固定文案（硬约束 #8）。
func Recovery(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic recovered",
					zap.Any("panic", r),
					zap.String("request_id", GetRequestID(c)),
					zap.String("path", c.FullPath()),
					zap.Stack("stack"),
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, model.Envelope{
					Code:      model.CodeInternalError,
					Message:   model.MessageOf(model.CodeInternalError),
					RequestID: GetRequestID(c),
				})
			}
		}()
		c.Next()
	}
}
```

**③ `internal/api/middleware/cors.go`**【完整代码】— CORS 白名单中间件（禁止 `*`；预检 OPTIONS 直接 204 应答，不进 Auth，§5.7、§8.1，硬约束 #12）。

```go
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CORS 仅放行 server.cors_allowed_origins 白名单（配置校验已拒绝 "*"，
// 此处再过滤一次作双保险，硬约束 #12）。
func CORS(allowedOrigins []string) gin.HandlerFunc {
	allow := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o == "*" {
			continue // 禁止通配
		}
		allow[o] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			if _, ok := allow[origin]; ok {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Vary", "Origin")
				c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				c.Header("Access-Control-Allow-Headers", "Content-Type, X-API-Key, X-Request-ID, Last-Event-ID")
				c.Header("Access-Control-Max-Age", "86400")
			}
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent) // 预检直接应答，不进 Auth（§5.7）
			return
		}
		c.Next()
	}
}
```

**④ `internal/api/middleware/auth.go`**【骨架 TODO】— API key 鉴权（总纲 R14：**SHA-256(key) 查询 Redis 缓存（TTL 60s）→ 未命中查 Postgres `api_keys` 表** 二级查找；`auth.api_keys` 为空 = 开发模式跳过并 WARN；`/api/v1/health` 与 `/metrics` 免鉴权；`PUT /config` 额外要求 admin）。

```go
package middleware

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"deepwiki/internal/model"
	"deepwiki/internal/store"
)

// ContextKeyAPIKey gin.Context 中已鉴权 API key 记录的键（*keyRecord；日志/审计使用时必须脱敏，硬约束 #2）。
const ContextKeyAPIKey = "api_key"

// keyRecord 缓存值结构（Redis 键 auth:key:<sha256(key)> → 本结构 JSON，TTL 60s，总纲 §4.4）。
type keyRecord struct {
	KeyID   string `json:"key_id"`
	IsAdmin bool   `json:"is_admin"`
	Revoked bool   `json:"revoked"`
}

// Auth X-API-Key 鉴权（总纲 R14/§4.4；硬约束 #2：密钥只存 SHA-256(salt‖key) 哈希，
// 禁止明文入 Postgres/etcd/日志）。devMode=true（auth.api_keys 为空）时跳过鉴权并打一次 WARN。
func Auth(cache redis.UniversalClient, keys store.APIKeyStore, devMode bool, logger *zap.Logger) gin.HandlerFunc {
	var warnOnce sync.Once
	return func(c *gin.Context) {
		if devMode {
			warnOnce.Do(func() { logger.Warn("auth disabled: auth.api_keys is empty (dev mode)") })
			c.Next()
			return
		}
		if c.FullPath() == "/api/v1/health" { // health 与 /metrics 免鉴权（§5.7）
			c.Next()
			return
		}
		// TODO: 实现二级查找（总纲 R14）：
		// ① key := c.GetHeader("X-API-Key")；空 → 40101；
		// ② sum := sha256(key) → GET auth:key:<hex(sum)（Redis 缓存，TTL 60s）；
		// ③ 缓存未命中 → keys.FindByHash(ctx, sum) 查 Postgres api_keys 表
		//    （逐条比对 SHA-256(salt‖key)；命中且未吊销 → 回写缓存 {key_id,is_admin,revoked:false}）；
		// ④ 未命中或已吊销 → 401 + 40101；命中 → c.Set(ContextKeyAPIKey, &rec) 放行；
		// ⑤ 吊销路径（管理端）必须同时 DEL auth:key:<sha256> 主动失效缓存；
		// ⑥ Redis 不可用 → 降级直查 Postgres 并记 WARN（可用性优先，不因此拒绝合法请求）。
		key := c.GetHeader("X-API-Key")
		if key == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, model.Envelope{
				Code:      model.CodeUnauthorized,
				Message:   model.MessageOf(model.CodeUnauthorized),
				RequestID: GetRequestID(c),
			})
			return
		}
		c.Next()
	}
}

// AdminOnly PUT /api/v1/config 的 admin 鉴权（已鉴权 key 的 is_admin=false → 40301，§5.7）；
// 开发模式（无 key 记录）放行。
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		rec, _ := c.Get(ContextKeyAPIKey)
		if rec == nil { // 开发模式（无 key 配置）：放行
			c.Next()
			return
		}
		// TODO: kr := rec.(*keyRecord)；kr.IsAdmin → 放行；否则 403 + 40301。
		c.Next()
	}
}
```

**⑤ `internal/api/middleware/ratelimit.go`**【骨架 TODO】— 两级限流中间件（硬约束 #1；存储实现 = **Redis Lua 滑动窗口**，Redis 不可用自动降级进程内 `x/time/rate` 兜底并 WARN + health degraded，总纲 §4.4；限流语义与数值冻结）。骨架阶段直通放行，**下一轮必须最先补齐**，补齐前不得宣称限流完成。

```go
package middleware

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/config"
	"deepwiki/internal/ratelimit"
)

// RateLimiter 两级限流中间件（§9.1，硬约束 #1：禁止全局单桶）：
// L1 per-IP 滑动窗口（per_ip.rps/per_ip.burst，默认 10/20，作用于全部 /api/v1/*）；
// L2 per-API-key 配额（ingest_per_hour=20 / ask_per_minute=30 / wiki_per_hour=10，
// 作用于建任务类与问答类昂贵端点）。数值冻结（总纲 §2.8），仅替换存储实现。
type RateLimiter struct {
	cfg     *config.Manager
	limiter ratelimit.Limiter // Redis Lua 滑动窗口 + 进程内 x/time/rate 降级兜底（§5.11.5）
	logger  *zap.Logger
}

func NewRateLimiter(cfg *config.Manager, limiter ratelimit.Limiter, logger *zap.Logger) *RateLimiter {
	return &RateLimiter{cfg: cfg, limiter: limiter, logger: logger}
}

// Middleware 限流中间件。【骨架阶段直通】下一轮实现，要求（硬约束 #1、§9.1~9.2、总纲 §4.4）：
//  1. L1 per-IP：limiter.Allow(ctx, "rl:ip:"+ip, 60s, rps*60+burst)（窗口换算见总纲 §4.4：limit = rps*60 + burst；
//     仅在 gin.SetTrustedProxies 配置后才采信 X-Forwarded-For）；
//  2. L2 per-API-key：ingest/refresh 走 "rl:key:<key_hash>:ingest"（3600s/20）、ask/ask-stream 走
//     "rl:key:<key_hash>:ask"（60s/30）、wiki generate 走 "rl:key:<key_hash>:wiki"（3600s/10）；
//     无 API key（开发模式）时 L2 退化为按 IP 计数；
//  3. 命中 → 429 + 42901 rate_limited + Retry-After 头 + X-RateLimit-Limit/Remaining/Reset
//     （Reset 为 UTC epoch 秒，响应头契约冻结）；
//  4. 未命中的受限端点响应同样携带 X-RateLimit-* 三件套；
//  5. Redis 不可用时 ratelimit 包内部自动降级进程内 x/time/rate 兜底 + WARN +
//     指标 deepwiki_ratelimit_degraded_total++ + health redis.ratelimit_degraded=true（可用性优先的有意取舍）；
//  6. 配置热更新：订阅 ConfigManager 重建窗口参数（§8.2）。
func (l *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: 按上述要求实现两级限流（当前为骨架直通）。
		c.Next()
	}
}
```

#### 5.11.3 internal/api/handler

**① `internal/api/handler/health.go`**【完整代码】— 健康检查（验收用，必须真实可用；按总纲 §7 新契约装配）+ 包内共享响应辅助函数 + 探测快照容器。

```go
// Package handler Handler 层：只做参数绑定/校验、统一信封装配、错误码映射；
// 不直接访问 DB / Provider，只调 Service（基线 §2.2）。
package handler

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/api/dto"
	"deepwiki/internal/api/middleware"
	"deepwiki/internal/config"
	"deepwiki/internal/model"
	"deepwiki/internal/task"
)

// ---------- 包内共享响应辅助（统一信封，硬约束 #8） ----------

func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, model.Envelope{
		Code:      0,
		Message:   "ok",
		Data:      data,
		RequestID: middleware.GetRequestID(c),
	})
}

func respondError(c *gin.Context, code int, message string, details []model.ErrorDetail) {
	c.JSON(model.HTTPStatusOf(code), model.Envelope{
		Code:      code,
		Message:   message,
		RequestID: middleware.GetRequestID(c),
		Details:   details,
	})
}

// respondNotImplemented 骨架阶段占位：未实现端点统一返回 50001 信封（下一轮逐端点替换为真实实现）。
func respondNotImplemented(c *gin.Context) {
	respondError(c, model.CodeInternalError, "internal error: endpoint not implemented yet (scaffold)", nil)
}

// ---------- HealthSnapshot（60s 后台探测循环写，health 接口只读，毫秒级返回） ----------

// HealthSnapshot 依赖状态快照容器（atomic.Value 持有 dto.HealthResponse 的依赖字段部分；
// 总纲 §7：探测仍走 60s 后台缓存，health 接口本身禁止发起外部调用）。
type HealthSnapshot struct {
	mu   sync.RWMutex
	data dto.HealthResponse
}

func NewHealthSnapshot() *HealthSnapshot {
	return &HealthSnapshot{data: dto.HealthResponse{
		Status: "degraded",
		Redis:  dto.RedisHealth{Mode: "sentinel"},
	}}
}

// Store 覆盖依赖字段（Version/UptimeSeconds/StartedAt/Worker 由 handler 每次请求时现填）。
func (s *HealthSnapshot) Store(d dto.HealthResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = d
}

func (s *HealthSnapshot) Load() dto.HealthResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data
}

// ---------- HealthHandler ----------

// HealthHandler GET /api/v1/health（总纲 §7，建议⑬）。
type HealthHandler struct {
	version string
	start   time.Time
	ready   *atomic.Bool
	cfg     *config.Manager
	snap    *HealthSnapshot
	worker  func() task.WorkerStats
	logger  *zap.Logger
}

func NewHealthHandler(version string, start time.Time, ready *atomic.Bool, cfg *config.Manager, snap *HealthSnapshot, worker func() task.WorkerStats, logger *zap.Logger) *HealthHandler {
	return &HealthHandler{version: version, start: start, ready: ready, cfg: cfg, snap: snap, worker: worker, logger: logger}
}

func (h *HealthHandler) Health(c *gin.Context) {
	cfg := h.cfg.Get()
	// 依赖字段全部来自 60s 后台探测缓存（毫秒级返回；接口内禁止发起外部调用）。
	data := h.snap.Load()
	data.Version = h.version
	data.UptimeSeconds = int64(time.Since(h.start).Seconds())
	data.StartedAt = h.start.UTC().Format(time.RFC3339)
	data.LLM.Provider = cfg.LLM.Provider
	data.LLM.Model = cfg.LLM.Model
	data.Embedding.Provider = cfg.Embedding.Provider
	data.Embedding.Model = cfg.Embedding.Model
	ws := h.worker()
	data.Worker = dto.WorkerHealth{Busy: ws.Busy, Total: ws.Total, Queued: ws.Queued}
	if !h.ready.Load() { // 优雅退出中：503 + 50301，status 保持原值供诊断（§6.6）
		c.JSON(http.StatusServiceUnavailable, model.Envelope{
			Code:      model.CodeServiceNotReady,
			Message:   model.MessageOf(model.CodeServiceNotReady),
			Data:      data,
			RequestID: middleware.GetRequestID(c),
		})
		return
	}
	respondOK(c, data)
}
```

**②~⑧ 其余 7 个 handler 文件**【骨架 TODO】：构造函数为完整代码，端点方法统一先 `respondNotImplemented(c)` 占位，TODO 注释写清该端点必须遵守的全部契约。逐一创建如下：

**② `internal/api/handler/ingest.go`** — IngestHandler。

```go
package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/service"
)

// IngestHandler POST /api/v1/ingest。
type IngestHandler struct {
	svc    *service.IngestService
	logger *zap.Logger
}

func NewIngestHandler(svc *service.IngestService, logger *zap.Logger) *IngestHandler {
	return &IngestHandler{svc: svc, logger: logger}
}

func (h *IngestHandler) Ingest(c *gin.Context) {
	// TODO: 实现 POST /api/v1/ingest（§6.1），要求：
	// ① ShouldBindJSON dto.IngestRequest + 校验（repo_url 合法 git URL、拒绝 file:// 等本地协议、≤512；
	//    branch ≤128 且禁止 ..、空白与 ~^:?*[\ 等 git ref 非法字符；options 字段按 §6.1 表校验）→ 失败 40001 + details；
	// ② 调 h.svc.SubmitIngest；幂等命中 40901（details 附 existing_repo_id / use_refresh）；
	// ③ 成功 202 + dto.TaskSubmittedResponse；
	// ④ model.ErrQueueFull → 429 + 42902 + Retry-After（估算：clamp(queued/pool_size×avg_task_seconds, 10, 600)，
	//    queued 为 RabbitMQ 主队列深度，§9.4）；
	// ⑤ model.ErrQueueUnavailable → 503 + 50302 queue_unavailable（RabbitMQ 投递 confirm 失败，总纲 §6）；
	// ⑥ 禁止回传 err.Error() 原文（硬约束 #8）。
	respondNotImplemented(c)
}
```

**③ `internal/api/handler/repo.go`** — RepoHandler（列表/详情/删除/刷新）。

```go
package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/service"
)

// RepoHandler /api/v1/repos 资源族。
type RepoHandler struct {
	repos  *service.RepoService
	ingest *service.IngestService
	logger *zap.Logger
}

func NewRepoHandler(repos *service.RepoService, ingest *service.IngestService, logger *zap.Logger) *RepoHandler {
	return &RepoHandler{repos: repos, ingest: ingest, logger: logger}
}

func (h *RepoHandler) List(c *gin.Context) {
	// TODO: GET /api/v1/repos（§6.7）：page/page_size 解析（§5.4 默认 1/20、page_size 钳制 1~100）；
	// 响应 dto.PageResult[Repo 摘要]；items 字段：repo_id, repo_url, branch, commit_hash, state, stats, created_at, updated_at。
	respondNotImplemented(c)
}

func (h *RepoHandler) Get(c *gin.Context) {
	// TODO: GET /api/v1/repos/{repo_id}（§6.7）：repo_id 先过 ^repo_[0-9A-HJKMNP-TV-Z]{26}$ 正则（硬约束 #11）；
	// 详情 = 列表字段 + latest_task + wiki_available + chunk_count；未命中 40402。
	respondNotImplemented(c)
}

func (h *RepoHandler) Delete(c *gin.Context) {
	// TODO: DELETE /api/v1/repos/{repo_id}（§6.7、§12.3）：ULID 正则校验；
	// 响应 {repo_id, deleted:{chunks, vectors, wiki_pages, opensearch_docs, local_dir:true}}
	//（关键词索引文档数字段随 OpenSearch 平移更名，语义不变）；任务历史保留（repo_id 置 NULL）。
	respondNotImplemented(c)
}

func (h *RepoHandler) Refresh(c *gin.Context) {
	// TODO: POST /api/v1/repos/{repo_id}/refresh（§6.7）：无 body；202 同 ingest 的 data 结构（type=refresh）；
	// 仓库非 ready / 进行中任务冲突 → 40902；同仓互斥锁 lock:refresh:<repo_id> 持锁失败 → 40902；
	// 限流桶 L1 per-IP + L2 ingest_per_hour。
	respondNotImplemented(c)
}
```

**④ `internal/api/handler/task.go`** — TaskHandler（统一任务端点，建议⑩）。

```go
package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/task"
)

// TaskHandler /api/v1/tasks 统一任务端点（ingest/refresh/wiki 共用，type 字段区分）。
type TaskHandler struct {
	tm     task.TaskManager
	logger *zap.Logger
}

func NewTaskHandler(tm task.TaskManager, logger *zap.Logger) *TaskHandler {
	return &TaskHandler{tm: tm, logger: logger}
}

func (h *TaskHandler) List(c *gin.Context) {
	// TODO: GET /api/v1/tasks（§6.7）：?type=&state=&repo_id=&page=&page_size= 过滤分页；
	// items 为 Task 全字段投影（不含 cancel_flag/request_payload，model.Task json tag 已保证）+ pagination。
	respondNotImplemented(c)
}

func (h *TaskHandler) Get(c *gin.Context) {
	// TODO: GET /api/v1/tasks/{task_id}：task_id 先过 ^tsk_[0-9A-HJKMNP-TV-Z]{26}$ 正则；未命中 40401。
	// 对应题目 GET /api/ingest/:id/status 的映射端点（路径以本契约为准，总纲 §2.11）。
	respondNotImplemented(c)
}

func (h *TaskHandler) Cancel(c *gin.Context) {
	// TODO: DELETE /api/v1/tasks/{task_id}（§4.5）：ULID 正则校验；
	// 成功 202 + 当前 task 快照（state 可能尚未变 cancelled）；终态 → 40902；
	// model.ErrTaskNotFound → 40401；model.ErrInvalidTaskState → 40902。
	respondNotImplemented(c)
}
```

**⑤ `internal/api/handler/ask.go`** — AskHandler（同步 + SSE 流式）。

```go
package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/service"
)

// AskHandler POST /api/v1/ask 与 /ask/stream。
type AskHandler struct {
	svc    *service.AskService
	logger *zap.Logger
}

func NewAskHandler(svc *service.AskService, logger *zap.Logger) *AskHandler {
	return &AskHandler{svc: svc, logger: logger}
}

func (h *AskHandler) Ask(c *gin.Context) {
	// TODO: POST /api/v1/ask（§6.2）：ShouldBindJSON dto.AskRequest + 校验
	// （repo_id 格式；question 长度 1~4000；mode ∈ keyword|embedding|hybrid；top_k 1~30）→ 40001 + details；
	// 成功 200 + dto.AskResponse；仓库非 ready → 40902；LLM 不可用 → 50201；Embedding 不可用 → 50202；
	// OpenSearch 不可用 → 50303；pgvector 查询失败 → 50203；限流桶 L1 per-IP + L2 ask_per_minute。
	respondNotImplemented(c)
}

func (h *AskHandler) AskStream(c *gin.Context) {
	// TODO: POST /api/v1/ask/stream（§6.3 SSE）：
	// ① 响应头 Content-Type: text/event-stream、Cache-Control: no-cache、Connection: keep-alive、X-Accel-Buffering: no；
	// ② 事件 id 为连接内单调递增序号；每 15s 一行 ": heartbeat" 心跳；
	// ③ 事件顺序 references（恰好 1）→ token（0~N）→ done（恰好 1）；error 任意位置终止流；
	// ④ 客户端断开 → ctx 取消 → 中断 LLM 流并退出 goroutine（硬约束 #4）；
	// ⑤ 校验失败在升级 SSE 前返回 40001 信封。
	respondNotImplemented(c)
}
```

**⑥ `internal/api/handler/wiki.go`** — WikiHandler。

```go
package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/service"
)

// WikiHandler POST /api/v1/wiki/generate 与 GET /api/v1/repos/{repo_id}/wiki。
type WikiHandler struct {
	svc    *service.WikiService
	logger *zap.Logger
}

func NewWikiHandler(svc *service.WikiService, logger *zap.Logger) *WikiHandler {
	return &WikiHandler{svc: svc, logger: logger}
}

func (h *WikiHandler) Generate(c *gin.Context) {
	// TODO: POST /api/v1/wiki/generate（§6.7）：body {"repo_id":"repo_..."} 必填且仓库须 ready（否则 40902）；
	// 202 + dto.TaskSubmittedResponse（type=wiki）；已有 wiki 则覆盖重建；
	// 限流桶 L1 per-IP + L2 wiki_per_hour（总纲 §2.8）。
	respondNotImplemented(c)
}

func (h *WikiHandler) GetWiki(c *gin.Context) {
	// TODO: GET /api/v1/repos/{repo_id}/wiki（§6.7）：
	// {repo_id, task_id, generated_at, toc:[{slug,title,parent_slug,sort_order}], pages:[{slug,title,content_md,sort_order,updated_at}]}；
	// 未生成 → 40403 wiki_not_found。
	respondNotImplemented(c)
}
```

**⑦ `internal/api/handler/config.go`** — ConfigHandler（GET 脱敏 / PUT admin+校验+etcd 审计）。

```go
package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/config"
)

// ConfigHandler GET/PUT /api/v1/config（§6.5，建议⑭）。
type ConfigHandler struct {
	cm     *config.Manager
	logger *zap.Logger
}

func NewConfigHandler(cm *config.Manager, logger *zap.Logger) *ConfigHandler {
	return &ConfigHandler{cm: cm, logger: logger}
}

func (h *ConfigHandler) GetConfig(c *gin.Context) {
	// TODO: GET /api/v1/config：返回 dto.ConfigResponse{Version, Config: h.cm.Masked(), RestartRequired}；
	// 密钥字段必须脱敏（config.MaskAPIKey 规则），Auth 节与基础设施凭据不出现（json:"-"，硬约束 #2）；
	// etcd 不可用时读本地快照缓存，GET 路径不报错（总纲 §4.5）。
	respondNotImplemented(c)
}

func (h *ConfigHandler) UpdateConfig(c *gin.Context) {
	// TODO: PUT /api/v1/config（路由层已挂 AdminOnly）：
	// ① 读取 JSON Merge Patch 原文 → h.cm.Apply(ctx, patch, 脱敏后的 key 标识)；
	// ② 校验失败 → 42201 + details 字段级明细（整体拒绝保持旧值，审计写 etcd /deepwiki/audit/<version>
	//    result=rejected，硬约束 #9）；
	// ③ 成功 → dto.ConfigUpdateResponse{version, applied, restart_required, warnings}（审计 result=applied）；
	// ④ etcd 写路径不可用 → 503 + 50304 config_store_unavailable（总纲 §6 新增码）。
	respondNotImplemented(c)
}
```

**⑧ `internal/api/handler/events.go`** — EventHandler（SSE 全局事件流 + WebSocket，建议⑪：只订阅 EventBus，禁止直接订阅 Task）。

```go
package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"deepwiki/internal/eventbus"
)

// EventHandler GET /api/v1/events（SSE）与 GET /api/v1/ws（WebSocket）。
type EventHandler struct {
	bus      eventbus.EventBus
	replayer eventbus.Replayer // Redis Streams XRANGE 回放（与 bus 同实例）
	upgrader websocket.Upgrader
	logger   *zap.Logger
}

func NewEventHandler(bus eventbus.EventBus, replayer eventbus.Replayer, logger *zap.Logger) *EventHandler {
	return &EventHandler{
		bus:      bus,
		replayer: replayer,
		logger:   logger,
		upgrader: websocket.Upgrader{
			// TODO（下一轮）：CheckOrigin 必须按 server.cors_allowed_origins 白名单校验（硬约束 #12）。
		},
	}
}

func (h *EventHandler) Events(c *gin.Context) {
	// TODO: GET /api/v1/events?types=task.state_changed,wiki.completed&repo_id=repo_...（§6.4）：
	// ① 仅订阅 EventBus（model.EventFilter{Types, RepoID}），禁止直接订阅 Task（建议⑪）；
	// ② Last-Event-ID 头 → h.replayer.ReplaySince 补发（Redis Streams：XRANGE events:task:<task_id> <last> +，
	//    每任务流 XTRIM MAXLEN ~ 1000）；过旧（流已截断）推 event: gap 提示回退 GET /api/v1/tasks；
	// ③ 帧格式：id: <seq> + event: <type> + data: <json>；每 15s 一行 ": heartbeat"；
	// ④ payload 为结构化字段直推（事件名与 payload 冻结，总纲 §2.5），禁止拼字符串（建议②）；
	// ⑤ 客户端断开即取消订阅并退出。
	respondNotImplemented(c)
}

func (h *EventHandler) WebSocket(c *gin.Context) {
	// TODO: GET /api/v1/ws（§6.7）：
	// ① upgrader.Upgrade 101；Query 过滤语义同 /events（types/repo_id）；
	// ② 推送 JSON 帧 {"seq":12,"type":"task.state_changed","data":{...}}（seq 单调递增 + resume_from 回放，
	//    回放源同为 Redis Streams）；
	// ③ 服务端每 15s 发 WS ping 帧；写超时与读超时必须设置，goroutine 带 recover（硬约束 #4）；
	// ④ 无 WS 内断线补发时，重连后客户端回退 GET /api/v1/tasks（§6.4）。
	respondNotImplemented(c)
}
```

#### 5.11.4 internal/api/router.go【完整代码】— 路由装配（18 个端点与基线 §5.1 一一对应，不得增减）

```go
// Package api 接入层装配：路由、中间件链、/metrics。
package api

import (
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"deepwiki/internal/api/handler"
	"deepwiki/internal/api/middleware"
	"deepwiki/internal/config"
	"deepwiki/internal/eventbus"
	"deepwiki/internal/ratelimit"
	"deepwiki/internal/service"
	"deepwiki/internal/store"
	"deepwiki/internal/task"
)

// Deps 装配依赖（main 注入）。
type Deps struct {
	Logger    *zap.Logger
	Cfg       *config.Manager
	Version   string
	StartTime time.Time
	Ready     *atomic.Bool
	Snapshot  *handler.HealthSnapshot // 60s 后台探测快照（health 毫秒级返回的数据源）
	Tasks     task.TaskManager
	Bus       eventbus.EventBus
	Replayer  eventbus.Replayer     // 与 Bus 同实例（Redis Streams 回放）
	Limiter   ratelimit.Limiter     // Redis Lua 滑动窗口 + 降级兜底（硬约束 #1）
	KeyCache  redis.UniversalClient // API key 二级查找的 L1 缓存（auth:key:*，总纲 R14）
	APIKeys   store.APIKeyStore     // Postgres api_keys 表（L2，总纲 R14）
	IngestSvc *service.IngestService
	RepoSvc   *service.RepoService
	AskSvc    *service.AskService
	WikiSvc   *service.WikiService
}

// NewRouter 装配全部路由与中间件（基线 §5.1 端点总表，冻结）。
// 中间件注册顺序说明：RequestID → Recovery 全局；/api/v1 组内 CORS 必须先于 Auth 注册，
// 保证预检 OPTIONS 由 CORS 直接应答不进 Auth（§5.7），随后 Auth → RateLimit。
func NewRouter(d Deps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	// TODO（下一轮）：生产经反向代理时 gin.SetTrustedProxies 后才可信 X-Forwarded-For（per-IP 限流取数，§9.1）。
	r.Use(middleware.RequestID())
	r.Use(middleware.Recovery(d.Logger))

	cfg := d.Cfg.Get()

	healthH := handler.NewHealthHandler(d.Version, d.StartTime, d.Ready, d.Cfg, d.Snapshot, d.Tasks.Stats, d.Logger)
	ingestH := handler.NewIngestHandler(d.IngestSvc, d.Logger)
	repoH := handler.NewRepoHandler(d.RepoSvc, d.IngestSvc, d.Logger)
	taskH := handler.NewTaskHandler(d.Tasks, d.Logger)
	askH := handler.NewAskHandler(d.AskSvc, d.Logger)
	wikiH := handler.NewWikiHandler(d.WikiSvc, d.Logger)
	configH := handler.NewConfigHandler(d.Cfg, d.Logger)
	eventH := handler.NewEventHandler(d.Bus, d.Replayer, d.Logger)
	rateLimiter := middleware.NewRateLimiter(d.Cfg, d.Limiter, d.Logger)

	v1 := r.Group("/api/v1")
	v1.Use(middleware.CORS(cfg.Server.CORSAllowedOrigins))
	v1.Use(middleware.Auth(d.KeyCache, d.APIKeys, len(cfg.Auth.APIKeys) == 0, d.Logger))
	v1.Use(rateLimiter.Middleware())
	{
		v1.GET("/health", healthH.Health)                          // 1
		v1.POST("/ingest", ingestH.Ingest)                         // 3
		v1.GET("/repos", repoH.List)                               // 4
		v1.GET("/repos/:repo_id", repoH.Get)                       // 5
		v1.DELETE("/repos/:repo_id", repoH.Delete)                 // 6
		v1.POST("/repos/:repo_id/refresh", repoH.Refresh)          // 7
		v1.GET("/tasks", taskH.List)                               // 8
		v1.GET("/tasks/:task_id", taskH.Get)                       // 9
		v1.DELETE("/tasks/:task_id", taskH.Cancel)                 // 10
		v1.POST("/ask", askH.Ask)                                  // 11
		v1.POST("/ask/stream", askH.AskStream)                     // 12
		v1.POST("/wiki/generate", wikiH.Generate)                  // 13
		v1.GET("/repos/:repo_id/wiki", wikiH.GetWiki)              // 14
		v1.GET("/config", configH.GetConfig)                       // 15
		v1.PUT("/config", middleware.AdminOnly(), configH.UpdateConfig) // 16
		v1.GET("/events", eventH.Events)                           // 17
		v1.GET("/ws", eventH.WebSocket)                            // 18
	}

	// 2. Prometheus 指标（不带版本前缀，免鉴权，§5.1）。
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	return r
}
```

#### 5.11.5 新基础设施包：internal/ratelimit + internal/lock + internal/search + internal/observability

**① `internal/ratelimit/limiter.go`**【接口完整】— 限流器抽象与判定结果。

```go
// Package ratelimit 分布式限流：Redis Lua 滑动窗口（主实现）+ 进程内 x/time/rate（降级兜底）
//（总纲 §4.4 / R11，硬约束 #1：per-IP + per-API-key 两级，禁止全局单桶；语义与数值冻结）。
package ratelimit

import (
	"context"
	"time"
)

// Decision 一次限流判定结果（响应头 X-RateLimit-* 与 Retry-After 的数据源，契约冻结）。
type Decision struct {
	Allowed    bool          // 是否放行
	Limit      int           // 窗口配额（X-RateLimit-Limit）
	Remaining  int           // 剩余额度（X-RateLimit-Remaining）
	ResetUnix  int64         // 窗口重置时刻（UTC epoch 秒，X-RateLimit-Reset）
	RetryAfter time.Duration // 命中限流时的 Retry-After 秒数
	Degraded   bool          // true = Redis 不可用已降级进程内兜底（health redis.ratelimit_degraded）
}

// Limiter 限流器接口（中间件只依赖本接口，不感知 Redis）。
type Limiter interface {
	// Allow 在窗口 window 内对 key 计数，超过 limit 返回 Allowed=false。
	// key 形态（总纲 §4.4，逐字一致）：
	//   rl:ip:<ip>                    —— L1 per-IP（window 60s，limit = rps*60 + burst）
	//   rl:key:<key_hash>:ingest      —— L2 摄取/刷新（3600s/20）
	//   rl:key:<key_hash>:ask         —— L2 问答（60s/30）
	//   rl:key:<key_hash>:wiki        —— L2 wiki 生成（3600s/10）
	Allow(ctx context.Context, key string, window time.Duration, limit int) (Decision, error)
	// Degraded 当前是否处于降级态（health 探测循环读取）。
	Degraded() bool
	// Close 释放资源（进程内兜底表的清理 goroutine 退出）。
	Close() error
}
```

**② `internal/ratelimit/redis_lua.go`**【骨架 TODO】— Redis 滑动窗口实现（Lua 原子执行，脚本全文内嵌，与总纲 §4.4 逐字一致）。

```go
package ratelimit

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// slidingWindowLua 滑动窗口限流脚本（总纲 §4.4 权威脚本，逐字一致，禁止改动）：
// ZSET 成员为请求时间戳，窗口外成员先剔除再计数，原子完成「判定+记账+过期」。
const slidingWindowLua = `
-- KEYS[1]=窗口键  ARGV=now_ms, window_ms, limit
redis.call('ZREMRANGEBYSCORE', KEYS[1], 0, ARGV[1]-ARGV[2])
local n = redis.call('ZCARD', KEYS[1])
if n < tonumber(ARGV[3]) then
  redis.call('ZADD', KEYS[1], ARGV[1], ARGV[1]..'-'..math.random(1,1e9))
  redis.call('PEXPIRE', KEYS[1], ARGV[2])
  return {1, tonumber(ARGV[3])-n-1}
end
return {0, 0}
`

// redisLimiter Redis 滑动窗口限流器（Lua 原子执行；哨兵高可用经 go-redis FailoverClient，总纲 R11）。
type redisLimiter struct {
	rdb      redis.UniversalClient
	script   *redis.Script
	fallback *fallbackLimiter
	degraded atomic.Bool
	logger   *zap.Logger
}

func NewRedisLimiter(rdb redis.UniversalClient, logger *zap.Logger) Limiter {
	return &redisLimiter{
		rdb:      rdb,
		script:   redis.NewScript(slidingWindowLua),
		fallback: newFallbackLimiter(),
		logger:   logger,
	}
}

func (l *redisLimiter) Allow(ctx context.Context, key string, window time.Duration, limit int) (Decision, error) {
	// TODO: 实现判定，要求（总纲 §4.4）：
	// ① script.Run(ctx, l.rdb, []string{key}, now_ms, window.Milliseconds(), limit) 原子执行；
	// ② 成功 → 解析 {allowed, remaining} 装配 Decision（ResetUnix=now+window，命中时 RetryAfter≈窗口剩余，
	//    可由 PTTL 精确化）；degraded 置 false；
	// ③ Redis 错误（网络/超时/哨兵切换中）→ 降级 l.fallback.allow(key, window, limit) 放行判定 +
	//    WARN 日志 + degraded 置 true + 指标 deepwiki_ratelimit_degraded_total++
	//    （可用性优先的有意取舍，总纲 §4.4；恢复成功后 degraded 自动回落 false）；
	// ④ 指标 deepwiki_redis_op_duration_seconds{op="ratelimit_lua"} 计时。
	panic("TODO: redisLimiter.Allow not implemented")
}

func (l *redisLimiter) Degraded() bool { return l.degraded.Load() }

func (l *redisLimiter) Close() error {
	if l.fallback != nil {
		l.fallback.close()
	}
	return nil
}
```

**③ `internal/ratelimit/fallback.go`**【骨架 TODO】— 进程内 `x/time/rate` 降级兜底（仅 Redis 不可用时启用）。

```go
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// fallbackLimiter 进程内 x/time/rate 降级兜底（总纲 §4.4：Redis 不可用时启用 + WARN +
// health degraded；多副本下各节点独立计数是已知近似，可用性优先的有意取舍）。
type fallbackLimiter struct {
	mu   sync.Mutex
	lims map[string]*entry
	stop chan struct{}
}

type entry struct {
	lim      *rate.Limiter
	lastUsed time.Time
}

func newFallbackLimiter() *fallbackLimiter {
	return &fallbackLimiter{
		lims: make(map[string]*entry),
		stop: make(chan struct{}),
	}
}

// allow 按窗口语义近似换算：速率 = limit/window，突发 = limit（滑动窗口上限的保守近似）。
func (f *fallbackLimiter) allow(key string, window time.Duration, limit int) Decision {
	// TODO: 实现兜底判定，要求：
	// ① 不存在则 rate.NewLimiter(rate.Every(window/time.Duration(limit)), limit) 建桶；
	// ② Allow() 判定并更新 lastUsed；Remaining 用 Tokens() 取整近似；
	// ③ 后台 goroutine 每 5 分钟清理 lastUsed 超 10 分钟的 idle 桶（LRU 淘汰，防内存膨胀，硬约束 #4：
	//    goroutine 必须可退出——监听 f.stop）。
	panic("TODO: fallbackLimiter.allow not implemented")
}

func (f *fallbackLimiter) close() { close(f.stop) }
```

**④ `internal/lock/redis_lock.go`**【骨架 TODO】— Redis 分布式锁（`SET NX PX` + owner token + Lua 解锁，总纲 §4.4 / R13）。

```go
// Package lock Redis 分布式锁（总纲 R13：替代多 worker 场景下失效的 v1 原方案进程内去重机制）。
// 锁键：lock:refresh:<repo_id>；TTL 300s（持锁方 pipeline 正常短于 5 分钟，
// 不引入 watchdog：超时自动释放 + 任务 CAS 兜底，硬约束 #18）。
package lock

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// refreshLockTTL refresh 互斥锁 TTL（300s，总纲 §4.4）。
const refreshLockTTL = 300 * time.Second

// unlockLua 解锁脚本（校验 owner token 后 DEL，防止误删他人锁；与总纲 §4.4 语义一致）：
const unlockLua = `
-- KEYS[1]=锁键  ARGV[1]=owner token
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0
`

// Locker refresh 互斥分布式锁。
type Locker struct {
	rdb     redis.UniversalClient
	unlock  *redis.Script
	logger  *zap.Logger
}

func New(rdb redis.UniversalClient, logger *zap.Logger) *Locker {
	return &Locker{rdb: rdb, unlock: redis.NewScript(unlockLua), logger: logger}
}

// AcquireRefresh 获取同仓 refresh 互斥锁：SET lock:refresh:<repo_id> <token> NX PX 300000；
// ok=false 表示他节点持锁（调用方映射 40902）；token 为 ULID（解锁时校验）。
func (l *Locker) AcquireRefresh(ctx context.Context, repoID string) (token string, ok bool, err error) {
	// TODO: 实现加锁，要求：
	// ① repoID 先过 ULID 正则（硬约束 #11，禁止拼路径/键名注入）；
	// ② token = ulid 随机串；SET lock:refresh:<repoID> token NX PX 300000；
	// ③ 指标 deepwiki_redis_op_duration_seconds{op="lock_acquire"} 计时。
	panic("TODO: Locker.AcquireRefresh not implemented")
}

// ReleaseRefresh 释放锁：Lua 校验 token 后 DEL（仅持锁本人可解）。
func (l *Locker) ReleaseRefresh(ctx context.Context, repoID, token string) error {
	// TODO: 实现解锁（unlockLua 脚本见上，脚本全文禁止改动）；token 不匹配按成功返回（幂等）。
	panic("TODO: Locker.ReleaseRefresh not implemented")
}
```

**⑤ `internal/search/opensearch.go`**【接口完整 + 实现骨架 TODO】— OpenSearch 客户端与索引生命周期（总纲 §4.2：每仓一索引 `deepwiki-chunks-<repo_id 全小写>`，BM25）。

```go
// Package search OpenSearch 客户端与索引生命周期（总纲 R3/§4.2）：
// 每仓一索引物理隔离（删仓 = 删索引）；BM25 默认排序；opensearch-go/v4 官方客户端。
package search

import (
	"context"
	"strings"

	opensearch "github.com/opensearch-project/opensearch-go/v4"
	"go.uber.org/zap"

	"deepwiki/internal/config"
	"deepwiki/internal/model"
)

// chunksIndexMapping chunk 索引 mapping（总纲 §4.2 权威 mapping，逐字一致，禁止改动）：
// code_analyzer 按非字母数字（保留 _ 和 .）切分并小写化；BM25 similarity；
// dev 单节点 0 副本，生产 3 节点 number_of_replicas=1（compose/部署参数化）。
const chunksIndexMapping = `{
  "settings": { "number_of_shards": 1, "number_of_replicas": 0, "analysis": {
      "analyzer": { "code_analyzer": { "type": "pattern", "pattern": "[^\\p{L}\\p{N}_.]+", "lowercase": true } } },
    "index.similarity.default.type": "BM25" },
  "mappings": { "properties": {
    "chunk_id":   { "type": "keyword" },
    "repo_id":    { "type": "keyword" },
    "path":       { "type": "text", "analyzer": "code_analyzer", "fields": { "raw": { "type": "keyword" } } },
    "content":    { "type": "text", "analyzer": "code_analyzer" },
    "language":   { "type": "keyword" },
    "start_line": { "type": "integer" },
    "end_line":   { "type": "integer" } } }
}`

// IndexName 每仓索引名：<index_prefix>-chunks-<repo_id 全小写>
//（OpenSearch 索引名必须小写；repo_id 含大写 ULID，统一 strings.ToLower，总纲 §4.2）。
func IndexName(prefix, repoID string) string {
	return prefix + "-chunks-" + strings.ToLower(repoID)
}

// Hit 关键词检索命中（KeywordRetriever 的适配输入；chunk 正文由 Postgres chunks 表回填）。
type Hit struct {
	ChunkID string
	Score   float64
}

// Client OpenSearch 客户端（索引生命周期 + 检索）。
type Client struct {
	oscli  *opensearch.Client
	prefix string // config.search.opensearch.index_prefix，默认 deepwiki
	logger *zap.Logger
}

// NewClient 建立 OpenSearch 客户端（addresses/username/password 来自配置与环境变量；
// 启动 Ping 失败即返回 error——启动失败优于带病运行，基线 §12.1）。
func NewClient(ctx context.Context, cfg config.OpenSearchConfig, logger *zap.Logger) (*Client, error) {
	// TODO: opensearch.NewClient(opensearch.Config{Addresses: cfg.Addresses, Username: cfg.Username,
	// Password: cfg.Password}) → Ping；成功日志 opensearch connected。
	panic("TODO: search.NewClient not implemented")
}

// CreateIndex 建索引（幂等：已存在跳过；mapping 用 chunksIndexMapping 常量）。
func (c *Client) CreateIndex(ctx context.Context, repoID string) error {
	// TODO: PUT /<index> body=chunksIndexMapping；400 resource_already_exists 视为成功。
	panic("TODO: Client.CreateIndex not implemented")
}

// DeleteIndex 删索引（删仓级联的外部资源步骤，总纲 §4.1：不存在视为成功；失败由调用方记 ERROR + 后台重试）。
func (c *Client) DeleteIndex(ctx context.Context, repoID string) error {
	// TODO: DELETE /<index>；404 视为成功。
	panic("TODO: Client.DeleteIndex not implemented")
}

// BulkIndex 批量写入 chunk（_id = chunk_id；Persist 阶段 Postgres 事务提交成功后再调用，顺序约定不变）。
func (c *Client) BulkIndex(ctx context.Context, repoID string, chunks []model.Chunk) error {
	// TODO: bulk API 逐条 {"index":{"_index":<index>,"_id":chunk_id}} + 文档 JSON；
	// 指标 deepwiki_opensearch_op_duration_seconds{op="bulk"} 计时。
	panic("TODO: Client.BulkIndex not implemented")
}

// Count 索引文档数（启动一致性校验：count(index) == chunks 表行数，
// 不一致 → WARN 并后台重建该仓索引，总纲 §4.2）。
func (c *Client) Count(ctx context.Context, repoID string) (int64, error) {
	// TODO: POST /<index>/_count；索引不存在返回 0（不视为错误）。
	panic("TODO: Client.Count not implemented")
}

// Search 关键词检索（KeywordRetriever 唯一实现路径，总纲 §4.2）：
// multi_match 于 content^2, path；filter: term repo_id（索引内天然隔离可省略，保留 filter 防御）；
// BM25 默认排序；pathFilter 非空时用 prefix 查询匹配 path.raw。
func (c *Client) Search(ctx context.Context, repoID, query string, topK int, pathFilter string) ([]Hit, error) {
	// TODO: 实现检索，要求：
	// ① bool.must = multi_match{query, fields:["content^2","path"]}；bool.filter = term{repo_id: repoID}；
	// ② pathFilter != "" → bool.filter 追加 prefix{"path.raw": pathFilter}；
	// ③ size=topK；解析 hits[].{_id, _score} 为 []Hit（正文由 KeywordRetriever 按 chunk_id 回 Postgres 取）；
	// ④ OpenSearch 不可用 → error 由上层映射 50303 search_unavailable（总纲 §6）；
	// ⑤ 指标 deepwiki_opensearch_op_duration_seconds{op="search"} 与 deepwiki_keyword_search_duration_seconds 计时。
	panic("TODO: Client.Search not implemented")
}

// Ping 健康探测（60s 后台探测循环用；顺带读 cluster health 与 deepwiki-* 索引数）。
func (c *Client) Ping(ctx context.Context) (clusterStatus string, indices int, err error) {
	// TODO: GET _cluster/health → status；GET _cat/indices/<prefix>-*?format=json → 计数。
	panic("TODO: Client.Ping not implemented")
}

// Close 释放资源（opensearch-go 无显式 Close，保留对称接口供优雅退出调用）。
func (c *Client) Close() error { return nil }
```

**⑥ `internal/observability/metrics.go`**【完整代码】— Prometheus 指标注册清单（v1 既有指标保留 + 总纲 §4.8 新增 9 项；`deepwiki_queue_length` 语义改为 RabbitMQ 队列深度）。

```go
// Package observability 可观测性：Prometheus 指标注册与 OpenTelemetry Traces 初始化（总纲 §4.8 / R16）。
package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics 全局指标集合（main 启动时 Register 一次；v1 既有指标保留，新增项见总纲 §4.8）。
type Metrics struct {
	// ---- v1 保留 ----
	WorkerBusy  prometheus.Gauge   // deepwiki_worker_busy
	QueueLength prometheus.Gauge   // deepwiki_queue_length（语义 = RabbitMQ 主队列深度，总纲 §4.8）
	// ---- v2 新增（总纲 §4.8） ----
	RabbitMQQueueDepth      *prometheus.GaugeVec     // deepwiki_rabbitmq_queue_depth{queue}
	RabbitMQPublishConfirms *prometheus.CounterVec   // deepwiki_rabbitmq_publish_confirms_total{result}
	RedisOpDuration         *prometheus.HistogramVec // deepwiki_redis_op_duration_seconds{op}
	OpenSearchOpDuration    *prometheus.HistogramVec // deepwiki_opensearch_op_duration_seconds{op}
	EtcdOpDuration          *prometheus.HistogramVec // deepwiki_etcd_op_duration_seconds{op}
	PgPoolConns             *prometheus.GaugeVec     // deepwiki_pg_pool_conns{state}
	VectorSearchDuration    prometheus.Histogram     // deepwiki_vector_search_duration_seconds
	KeywordSearchDuration   prometheus.Histogram     // deepwiki_keyword_search_duration_seconds
	RatelimitDegraded       prometheus.Counter       // deepwiki_ratelimit_degraded_total
}

// Register 注册全部指标（promauto 默认注册表；重复注册会 panic，只允许 main 调用一次）。
func Register() *Metrics {
	return &Metrics{
		WorkerBusy: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "deepwiki_worker_busy", Help: "运行中 worker 数"}),
		QueueLength: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "deepwiki_queue_length", Help: "RabbitMQ 主队列 deepwiki.task.jobs 深度"}),
		RabbitMQQueueDepth: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "deepwiki_rabbitmq_queue_depth", Help: "RabbitMQ 各队列深度"}, []string{"queue"}),
		RabbitMQPublishConfirms: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "deepwiki_rabbitmq_publish_confirms_total", Help: "publisher confirm 结果计数"}, []string{"result"}),
		RedisOpDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name: "deepwiki_redis_op_duration_seconds", Help: "Redis 操作耗时",
			Buckets: prometheus.DefBuckets}, []string{"op"}),
		OpenSearchOpDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name: "deepwiki_opensearch_op_duration_seconds", Help: "OpenSearch 操作耗时",
			Buckets: prometheus.DefBuckets}, []string{"op"}),
		EtcdOpDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name: "deepwiki_etcd_op_duration_seconds", Help: "etcd 操作耗时",
			Buckets: prometheus.DefBuckets}, []string{"op"}),
		PgPoolConns: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "deepwiki_pg_pool_conns", Help: "pgxpool 连接数（state=total|idle|acquired）"}, []string{"state"}),
		VectorSearchDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name: "deepwiki_vector_search_duration_seconds", Help: "pgvector HNSW 检索耗时",
			Buckets: prometheus.DefBuckets}),
		KeywordSearchDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Name: "deepwiki_keyword_search_duration_seconds", Help: "OpenSearch BM25 检索耗时",
			Buckets: prometheus.DefBuckets}),
		RatelimitDegraded: promauto.NewCounter(prometheus.CounterOpts{
			Name: "deepwiki_ratelimit_degraded_total", Help: "限流降级进程内兜底次数"}),
	}
}
```

**⑦ `internal/observability/tracing.go`**【骨架 TODO】— OpenTelemetry Traces 初始化（OTLP gRPC；endpoint 空则禁用，零成本，总纲 R16）。

```go
package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.uber.org/zap"
)

// InitTracer 初始化 OTel TracerProvider（总纲 R16：gin middleware + worker pipeline span +
// pgx/opensearch/rabbitmq 调用 span；OTLP endpoint 空则禁用，零成本）。
// 返回 shutdown 函数（优雅退出时在 flush 日志前调用，强制导出残余 span）。
func InitTracer(ctx context.Context, endpoint, serviceName string, logger *zap.Logger) (shutdown func(context.Context) error, err error) {
	if endpoint == "" {
		logger.Info("otel tracing disabled (observability.otel_endpoint empty)")
		return func(context.Context) error { return nil }, nil
	}
	// TODO: 实现初始化，要求：
	// ① otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(endpoint), otlptracegrpc.WithInsecure()) 建 exporter；
	// ② sdktrace.NewTracerProvider(WithBatcher(exporter), WithResource(resource.NewWithAttributes(
	//    semconv.SchemaURL, semconv.ServiceName(serviceName))))；
	// ③ otel.SetTracerProvider(tp)；返回 tp.Shutdown；
	// ④ gin 侧用 otelgin 中间件、worker pipeline 手动 span、pgx/opensearch/rabbitmq 调用点包 span（下一轮接入）。
	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(endpoint), otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName(serviceName))),
	)
	otel.SetTracerProvider(tp)
	logger.Info("otel tracing enabled", zap.String("endpoint", endpoint))
	return tp.Shutdown, nil
}
```

### 5.12 cmd/server/main.go【完整代码】— 启动装配与优雅退出

> 装配顺序（严格按序，任一失败即退出——启动失败优于带病运行，基线 §12.1）：加载引导配置（viper）→ 初始化 zap → OTel → 连接 etcd 并加载运行时配置 + watch → 连接 Postgres（pgxpool）→ golang-migrate Up → 连接 OpenSearch（启动 count 校验）→ 连接 Redis 哨兵（FailoverClient）→ 连接 RabbitMQ（声明拓扑）→ 启动 Reconciler → 启动 worker pool/consumer → git 可用性探测 → 装配 gin（中间件链）→ 启动后台 health 探测循环（60s）→ 优雅退出（硬约束 #10）。`store` 包构造函数签名以 §5.6 为准（pgxpool 注入）。

```go
// DeepWiki(Go版) 服务端入口。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"deepwiki/internal/api"
	"deepwiki/internal/api/handler"
	"deepwiki/internal/config"
	"deepwiki/internal/embed"
	"deepwiki/internal/eventbus"
	"deepwiki/internal/ingest"
	"deepwiki/internal/llm"
	"deepwiki/internal/observability"
	"deepwiki/internal/queue"
	"deepwiki/internal/ratelimit"
	"deepwiki/internal/retriever"
	"deepwiki/internal/search"
	"deepwiki/internal/service"
	"deepwiki/internal/store"
	"deepwiki/internal/task"
)

const version = "0.2.0"

func main() {
	configPath := flag.String("config", "configs/config.yaml", "config file path")
	flag.Parse()

	// ① 引导配置（viper：yaml + 环境变量；密钥与基础设施凭据仅 env 注入，硬约束 #2）。
	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal(nil, "load config", err)
	}

	// ② 结构化日志（zap，生产 JSON）。
	logger := newLogger(cfg.Observability.Log)
	defer func() { _ = logger.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ③ OpenTelemetry Traces（OTLP endpoint 空则禁用，零成本，总纲 R16）。
	shutdownTracer, err := observability.InitTracer(ctx, cfg.Observability.OTelEndpoint, "deepwiki", logger)
	if err != nil {
		fatal(logger, "init otel tracer", err)
	}
	metrics := observability.Register()

	// ④ etcd：连接 → 全量读 /deepwiki/config/ 前缀叠加运行时覆写 → Watch 热更新（硬约束 #9，总纲 §4.5）。
	etcdSrc, err := config.NewEtcdSource(ctx, cfg.Etcd.Endpoints, cfg.Etcd.Prefix, logger)
	if err != nil {
		fatal(logger, "connect etcd", err)
	}
	overrides, cfgVersion, err := etcdSrc.LoadAll(ctx)
	if err != nil {
		fatal(logger, "load config overrides from etcd", err)
	}
	cm := config.NewManager(config.MergeOverrides(cfg, overrides), cfgVersion, etcdSrc, logger)
	go cm.StartWatch(ctx)

	// ⑤ Postgres 连接池（pgxpool：MaxConns=10, MinConns=2, MaxConnLifetime=1h, HealthCheckPeriod=30s，总纲 §4.1）。
	pool, err := store.NewPool(ctx, cfg.Storage.Postgres, logger)
	if err != nil {
		fatal(logger, "connect postgres", err)
	}

	// ⑥ golang-migrate Up（embed.FS source；dirty 状态 panic 退出并提示 migrate force，只前进原则，总纲 R4）。
	if err := store.MigrateUp(ctx, cfg.Storage.Postgres.DSN, logger); err != nil {
		fatal(logger, "migrate", err)
	}

	// ⑦ OpenSearch 客户端 + 启动一致性校验（每仓 count(index) == chunks 表行数，总纲 §4.2）。
	searchCli, err := search.NewClient(ctx, cfg.Search.OpenSearch, logger)
	if err != nil {
		fatal(logger, "connect opensearch", err)
	}
	chunkStore := store.NewChunkStore(pool, logger)
	repoStore := store.NewRepoStore(pool, logger)
	go verifyIndices(ctx, repoStore, chunkStore, searchCli, logger)

	// ⑧ Redis 哨兵 FailoverClient（总纲 §4.4；地址与密码仅环境变量/引导层注入）。
	rdb := redis.NewFailoverClient(&redis.FailoverOptions{
		MasterName:    cfg.Redis.Sentinel.MasterName,
		SentinelAddrs: cfg.Redis.Sentinel.Addresses,
		Password:      cfg.Redis.Password,
	})
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		pingCancel()
		fatal(logger, "connect redis via sentinel", err)
	}
	pingCancel()
	logger.Info("redis sentinel master discovered", zap.String("master_name", cfg.Redis.Sentinel.MasterName))

	// ⑨ RabbitMQ 连接 + 拓扑声明（exchange deepwiki.tasks / 主队列 x-max-length=100 / DLX / 重试 TTL 链，总纲 §4.3）。
	mq, err := queue.Dial(ctx, cfg.Queue.RabbitMQ.URL, cfg.Worker.QueueSize, logger)
	if err != nil {
		fatal(logger, "connect rabbitmq", err)
	}
	if err := mq.DeclareTopology(ctx); err != nil {
		fatal(logger, "declare rabbitmq topology", err)
	}
	publisher := queue.NewPublisher(mq, logger)
	consumer := queue.NewConsumer(mq, cfg.Queue.RabbitMQ.Prefetch, logger)

	// ⑩ Reconciler 启动恢复（总纲 §4.3：pending 重投；running 超 5 分钟无心跳重置 pending 重投，幂等 CAS）。
	taskStore := task.NewTaskStore(pool, logger)
	reconciler := queue.NewReconciler(taskStore, publisher, logger)
	if err := reconciler.Recover(ctx); err != nil {
		// 恢复失败不阻断启动（DLQ/周期巡检兜底），但必须 ERROR 留痕。
		logger.Error("reconciler recover failed", zap.Error(err))
	}

	// ⑪ 统一任务系统：事件总线（Redis Streams + Pub/Sub 扇出）+ 消费端 + 有界 Worker Pool。
	bus := eventbus.NewRedisStreamsBus(rdb, logger)
	go bus.StartFanout(ctx)
	tm := task.NewManager(taskStore, bus, publisher, cfg.Worker, logger)
	// ingest/refresh/wiki 三个 Executor 在各自模块迭代落地后注册（tm.RegisterExecutor(...)）；
	// 未注册类型的任务被消费时落 failed/50001，不会阻塞其他类型。
	tm.Start(ctx, consumer)

	// ⑫ git 可用性探测（git --version；缺失 → health degraded，总纲 §4.6）。
	gitVersion, gitOK := probeGit(ctx, cfg.Git.BinaryPath)
	if !gitOK {
		logger.Error("git binary not available", zap.String("binary_path", cfg.Git.BinaryPath))
	}

	// Provider 与检索装配（构造函数均为纯装配，不发起网络调用；官方 SDK adapter，硬约束 #17）。
	emb, err := embed.New(cfg.Embedding, logger)
	if err != nil {
		fatal(logger, "build embedder", err)
	}
	llmClient, err := llm.New(cfg.LLM, logger)
	if err != nil {
		fatal(logger, "build llm", err)
	}

	// ⑬ 装配 gin（中间件链：RequestID → Recovery → CORS → Auth → RateLimit，硬约束 #1/#2）。
	limiter := ratelimit.NewRedisLimiter(rdb, logger)
	snapshot := handler.NewHealthSnapshot()
	apiKeyStore := store.NewAPIKeyStore(pool, logger)
	wikiStore := store.NewWikiStore(pool, logger)
	vectorStore := store.NewVectorStore(pool, logger)
	cloner := ingest.NewGitCloner(cfg.Git.BinaryPath, logger)
	retrievers := buildRetrievers(searchCli, chunkStore, vectorStore, emb, cfg, logger)

	ingestSvc := service.NewIngestService(tm, repoStore, cloner, publisher, cm, logger)
	repoSvc := service.NewRepoService(repoStore, chunkStore, vectorStore, wikiStore, searchCli, tm, logger)
	askSvc := service.NewAskService(cm, retrievers, llmClient, logger)
	wikiSvc := service.NewWikiService(tm, wikiStore, logger)

	ready := &atomic.Bool{}
	ready.Store(true)
	router := api.NewRouter(api.Deps{
		Logger:    logger,
		Cfg:       cm,
		Version:   version,
		StartTime: time.Now().UTC(),
		Ready:     ready,
		Snapshot:  snapshot,
		Tasks:     tm,
		Bus:       bus,
		Replayer:  bus,
		Limiter:   limiter,
		KeyCache:  rdb,
		APIKeys:   apiKeyStore,
		IngestSvc: ingestSvc,
		RepoSvc:   repoSvc,
		AskSvc:    askSvc,
		WikiSvc:   wikiSvc,
	})

	// ⑭ 后台 health 探测循环（60s；health 接口只读快照，毫秒级返回，总纲 §7）。
	probe := &healthProber{
		cfg: cm, pool: pool, searchCli: searchCli, publisher: publisher, rdb: rdb,
		etcdSrc: etcdSrc, emb: emb, llmCli: llmClient, gitBinary: cfg.Git.BinaryPath,
		gitVersion: gitVersion, gitOK: gitOK, limiter: limiter, metrics: metrics,
		snapshot: snapshot, logger: logger,
	}
	go probe.loop(ctx, 60*time.Second)

	srv := &http.Server{
		Addr:        cfg.Server.Addr,
		Handler:     router,
		ReadTimeout: cfg.Server.ReadTimeout,
	}
	go func() {
		logger.Info("deepwiki listening", zap.String("addr", cfg.Server.Addr), zap.String("version", version))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fatal(logger, "http server", err)
		}
	}()

	<-ctx.Done()
	// ⑮ 优雅退出（硬约束 #10，总纲 §4.3 语义平移）：
	// readiness 置失败（health 返回 503 + 50301）→ 停接新请求 → consumer 停拉新消息 →
	// 等在途任务完成（上限 server.shutdown_timeout）→ 未完成者 nack requeue=true 让别的节点接走 →
	// 按序关连接：rabbitmq → redis → opensearch → postgres → etcd → flush 日志（禁止连接泄漏）。
	logger.Info("shutting down")
	ready.Store(false)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown error", zap.Error(err))
	}
	if err := consumer.Stop(context.Background()); err != nil {
		logger.Error("stop consumer error", zap.Error(err))
	}
	tm.Stop(shutdownCtx) // 等在途 → 未完成 nack requeue=true（硬约束 #10）
	bus.Close()
	if err := publisher.Close(); err != nil {
		logger.Error("close rabbitmq publisher error", zap.Error(err))
	}
	if err := mq.Close(); err != nil { // ① rabbitmq
		logger.Error("close rabbitmq error", zap.Error(err))
	}
	if err := rdb.Close(); err != nil { // ② redis
		logger.Error("close redis error", zap.Error(err))
	}
	if err := searchCli.Close(); err != nil { // ③ opensearch
		logger.Error("close opensearch error", zap.Error(err))
	}
	pool.Close() // ④ postgres
	if err := etcdSrc.Close(); err != nil { // ⑤ etcd
		logger.Error("close etcd error", zap.Error(err))
	}
	if err := shutdownTracer(context.Background()); err != nil {
		logger.Error("shutdown tracer error", zap.Error(err))
	}
	logger.Info("bye") // ⑥ flush 日志由 defer logger.Sync() 完成
}

// healthProber 60s 后台探测循环：聚合全部依赖状态写 HealthSnapshot（health 接口毫秒级返回的数据源）。
type healthProber struct {
	cfg        *config.Manager
	pool       *pgxpool.Pool
	searchCli  *search.Client
	publisher  queue.Publisher
	rdb        redis.UniversalClient
	etcdSrc    *config.EtcdSource
	emb        embed.Embedder
	llmCli     llm.LLM
	gitBinary  string
	gitVersion string
	gitOK      bool
	limiter    ratelimit.Limiter
	metrics    *observability.Metrics
	snapshot   *handler.HealthSnapshot
	logger     *zap.Logger
}

// loop 每 interval 探测一轮并刷新快照；goroutine 随 ctx 取消退出（硬约束 #4）。
func (p *healthProber) loop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	p.probeOnce(ctx) // 启动即探测一轮，避免 health 长时间 degraded
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.probeOnce(ctx)
		}
	}
}

// probeOnce 单轮探测：任一依赖异常 → status=degraded（postgres/opensearch/rabbitmq/redis/etcd
// 任一异常 → degraded，readiness 503 + 50301 语义不变，总纲 §6/§7）。
func (p *healthProber) probeOnce(ctx context.Context) {
	defer func() { // 单轮探测 panic 不得拖垮进程（硬约束 #4）
		if r := recover(); r != nil {
			p.logger.Error("health probe panic", zap.Any("panic", r))
		}
	}()
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	degraded := false
	snap := p.snapshot.Load()

	// Postgres：Ping + 连接池状态 + 迁移版本（schema_migrations）。
	pgOK := p.pool.Ping(probeCtx) == nil
	stat := p.pool.Stat()
	snap.Postgres.Connected = pgOK
	snap.Postgres.Pool.Total = stat.TotalConns()
	snap.Postgres.Pool.Idle = stat.IdleConns()
	if !pgOK {
		degraded = true
	}

	// OpenSearch：cluster health + 索引计数。
	clusterStatus, indices, osErr := p.searchCli.Ping(probeCtx)
	snap.OpenSearch.Connected = osErr == nil
	snap.OpenSearch.ClusterStatus = clusterStatus
	snap.OpenSearch.Indices = indices
	if osErr != nil {
		degraded = true
	}

	// RabbitMQ：主队列深度（QueueDeclarePassive）；指标 deepwiki_queue_length / rabbitmq_queue_depth。
	depth, mqErr := p.publisher.QueueDepth(probeCtx)
	snap.RabbitMQ.Connected = mqErr == nil
	snap.RabbitMQ.QueueDepth = depth
	if mqErr == nil {
		p.metrics.QueueLength.Set(float64(depth))
		p.metrics.RabbitMQQueueDepth.WithLabelValues("deepwiki.task.jobs").Set(float64(depth))
	} else {
		degraded = true
	}

	// Redis 哨兵：Ping + 当前主地址（FailoverClient 故障转移后 Options().Addr 自动指向新主）；
	// ratelimit_degraded 反映降级兜底状态（总纲 §4.4）。
	redisOK := p.rdb.Ping(probeCtx).Err() == nil
	snap.Redis.Connected = redisOK
	snap.Redis.Mode = "sentinel"
	snap.Redis.Master = p.rdb.Options().Addr
	snap.Redis.RatelimitDegraded = p.limiter.Degraded()
	if !redisOK {
		degraded = true
	}

	// etcd：Healthy + endpoints。
	etcdOK := p.etcdSrc.Healthy(probeCtx)
	snap.Etcd.Connected = etcdOK
	snap.Etcd.Endpoints = p.etcdSrc.Endpoints()
	if !etcdOK {
		degraded = true
	}

	// git CLI：缓存启动探测结果，周期复验。
	if v, ok := probeGit(probeCtx, p.gitBinary); ok {
		p.gitVersion, p.gitOK = v, true
	} else {
		p.gitOK = false
	}
	snap.Git.Available = p.gitOK
	snap.Git.Version = p.gitVersion
	if !p.gitOK {
		degraded = true
	}

	// LLM / Embedding：启动探测 + gobreaker 状态（下一轮 adapter 落地 reachabilityProber 后生效；
	// 骨架阶段固定 reachable=false → degraded，验收标准允许）。
	snap.LLM.Reachable = probeProvider(probeCtx, p.llmCli)
	snap.LLM.Breaker = breakerStateOf(p.llmCli)
	snap.Embedding.Reachable = probeProvider(probeCtx, p.emb)
	snap.Embedding.Breaker = breakerStateOf(p.emb)
	if !snap.LLM.Reachable || !snap.Embedding.Reachable {
		degraded = true
	}

	if degraded {
		snap.Status = "degraded"
	} else {
		snap.Status = "ok"
	}
	p.snapshot.Store(snap)
}

// reachabilityProber provider 可达性探测契约（embed/llm adapter 下一轮实现：轻量 Ping + gobreaker 状态；
// 硬约束 #7 外部调用韧性：SDK 内置重试优先 + gobreaker 熔断，连续失败 ≥5 → open → health degraded）。
type reachabilityProber interface {
	Ping(ctx context.Context) error
	BreakerState() string // closed|open|half-open（gobreaker，总纲 R8）
}

func probeProvider(ctx context.Context, provider any) bool {
	pr, ok := provider.(reachabilityProber)
	if !ok {
		return false // 骨架阶段：未实现探测契约按不可达处理（degraded）
	}
	return pr.Ping(ctx) == nil
}

func breakerStateOf(provider any) string {
	if pr, ok := provider.(reachabilityProber); ok {
		return pr.BreakerState()
	}
	return "closed"
}

// buildRetrievers 检索三件套装配（keyword=OpenSearch BM25、embedding=pgvector HNSW、hybrid=RRF 融合）。
func buildRetrievers(searchCli *search.Client, chunks store.ChunkStore, vectors store.VectorStore, emb embed.Embedder, cfg *config.Config, logger *zap.Logger) map[string]retriever.Retriever {
	kw := retriever.NewKeywordRetriever(searchCli, chunks, logger)
	vec := retriever.NewVectorRetriever(vectors, emb)
	hyb := retriever.NewHybridRetriever(kw, vec, cfg.Retriever.RRFK, logger)
	return map[string]retriever.Retriever{"keyword": kw, "embedding": vec, "hybrid": hyb}
}

// verifyIndices 启动一致性校验（总纲 §4.2）：每仓 count(index) == chunks 表行数，
// 不一致 → WARN 并后台重建该仓索引。store 方法签名以 §5.6 为准。
func verifyIndices(ctx context.Context, repos store.RepoStore, chunks store.ChunkStore, searchCli *search.Client, logger *zap.Logger) {
	defer func() { // 校验失败不得拖垮启动（硬约束 #4）
		if r := recover(); r != nil {
			logger.Error("verify indices panic", zap.Any("panic", r))
		}
	}()
	repoIDs, err := repos.ListRepoIDs(ctx)
	if err != nil {
		logger.Error("verify indices: list repos failed", zap.Error(err))
		return
	}
	for _, repoID := range repoIDs {
		want, err := chunks.CountByRepo(ctx, repoID)
		if err != nil {
			logger.Error("verify indices: count chunks failed", zap.String("repo_id", repoID), zap.Error(err))
			continue
		}
		got, err := searchCli.Count(ctx, repoID)
		if err != nil {
			logger.Error("verify indices: count index failed", zap.String("repo_id", repoID), zap.Error(err))
			continue
		}
		if got != want {
			logger.Warn("verify indices: mismatch, schedule rebuild",
				zap.String("repo_id", repoID), zap.Int64("postgres", want), zap.Int64("opensearch", got))
			// 下一轮：投递该仓索引重建任务（bulk 全量重写，幂等 _id=chunk_id）。
		}
	}
	logger.Info("opensearch indices verified", zap.Int("repos", len(repoIDs)))
}

// probeGit git CLI 可用性探测（git --version 解析版本号，总纲 §4.6；
// exec.CommandContext 直调，禁止 sh -c 拼接，硬约束 #5）。
func probeGit(ctx context.Context, binary string) (string, bool) {
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(probeCtx, binary, "--version").Output()
	if err != nil {
		return "", false
	}
	fields := strings.Fields(string(out)) // 形如 "git version 2.43.0"
	if len(fields) >= 3 {
		return fields[2], true
	}
	return strings.TrimSpace(string(out)), true
}

func newLogger(cfg config.LogConfig) *zap.Logger {
	var zc zap.Config
	if cfg.Format == "console" {
		zc = zap.NewDevelopmentConfig()
	} else {
		zc = zap.NewProductionConfig()
	}
	if lvl, err := zap.ParseAtomicLevel(cfg.Level); err == nil {
		zc.Level = lvl
	}
	logger, err := zc.Build()
	if err != nil {
		fatal(nil, "build logger", err)
	}
	return logger
}

func fatal(logger *zap.Logger, msg string, err error) {
	if logger != nil {
		logger.Fatal(msg, zap.Error(err))
	}
	fmt.Fprintf(os.Stderr, "fatal: %s: %v\n", msg, err)
	os.Exit(1)
}
```

> 注：`config.MergeOverrides`（引导配置 ← etcd 覆写深合并）为下一轮实现的纯函数，与 `Manager.Apply` 共用同一套合并语义；`retriever.NewKeywordRetriever` 首参已改为 `search.Client`（§5.4 同步更新），`ingest.NewGitCloner` 首参为 git 二进制路径（§5.5 同步更新），`store.NewPool` / `store.MigrateUp` / `store.NewAPIKeyStore` 等构造函数签名以 §5.6 为准。

### 5.13 非 Go 文件（全部【完整代码】）

> `go.mod` 不在本节重复：模块声明 `module deepwiki` + `go 1.22`，直接依赖与锁定版本以第 2 章《技术栈与依赖版本锁定表》为准（含 pgx/v5、pgvector-go、opensearch-go/v4、golang-migrate/v4、amqp091-go、go-redis/v9、etcd client/v3、otel、gobreaker、各 LLM 官方 SDK 等新增依赖与 gin/zap/viper 等保留项），`go mod tidy` 生成 indirect 行，禁止手工删除。

**① `configs/config.yaml`** — 默认配置（与基线 §8.1 默认值及总纲 §5.2 新配置树逐 key 一致；禁止写入任何明文密钥与基础设施凭据）。

```yaml
# DeepWiki(Go版) 默认配置（基线 §8.1 + 总纲 §5.2 新配置树）。
# 密钥禁止落 yaml：仅由环境变量注入（硬约束 #2）——
#   DEEPWIKI_API_KEYS / DEEPWIKI_ADMIN_KEY / DEEPWIKI_EMBEDDING_API_KEY / DEEPWIKI_LLM_API_KEY
# 基础设施凭据同样禁止落 yaml：
#   DEEPWIKI_POSTGRES_DSN / DEEPWIKI_OPENSEARCH_USERNAME / DEEPWIKI_OPENSEARCH_PASSWORD /
#   DEEPWIKI_RABBITMQ_URL / DEEPWIKI_REDIS_PASSWORD
server:
  addr: ":8080"                       # restart_required
  read_timeout: 30s
  shutdown_timeout: 30s
  cors_allowed_origins:               # 白名单，禁止 "*"
    - "http://localhost:3000"
    - "http://localhost:5173"

rate_limit:                           # 限流数值冻结（总纲 §2.8）；存储为 Redis Lua 滑动窗口
  per_ip:
    rps: 10
    burst: 20
  per_key:
    ingest_per_hour: 20
    ask_per_minute: 30
    wiki_per_hour: 10

worker:
  pool_size: 2                        # 同时决定 RabbitMQ 消费端 prefetch（缺省）
  queue_size: 100                     # = RabbitMQ 主队列 deepwiki.task.jobs 的 x-max-length

ingest:
  workdir: "./data/repos"
  max_repo_size_mb: 500
  chunk_size: 800
  chunk_overlap: 120
  include_ext: [.go, .py, .js, .ts, .tsx, .jsx, .java, .md, .txt, .json, .yaml, .yml, .toml, .rs, .cpp, .c, .h, .rb, .php, .sh, .sql, .html, .css]
  exclude_dirs: [.git, node_modules, vendor, dist, build, target]

embedding:
  provider: openai                    # openai|dashscope|siliconflow|ollama|voyage
  model: text-embedding-3-small
  api_key: ""                         # 禁止明文；仅 DEEPWIKI_EMBEDDING_API_KEY 注入
  base_url: ""
  batch_size: 64
  timeout: 60s
  retry:
    max: 3
    backoff: 2s

llm:
  provider: openai                    # openai|gemini|claude|ollama|deepseek
  model: gpt-4o-mini
  api_key: ""                         # 禁止明文；仅 DEEPWIKI_LLM_API_KEY 注入
  base_url: ""
  temperature: 0.2
  max_tokens: 2048
  timeout: 120s
  retry:
    max: 2
    backoff: 1s

retriever:
  mode: hybrid                        # keyword|embedding|hybrid
  top_k: 8
  rrf_k: 60

storage:
  postgres:
    dsn: ""                           # 禁止 yaml 明文；仅 DEEPWIKI_POSTGRES_DSN 注入；restart_required
    max_conns: 10                     # pgxpool MaxConns；热更新
  vector:
    dimensions: 1536                  # pgvector 列维度；建表定型 restart_required（改维度 = 新迁移 + 全量重建）
    ef_search: 64                     # HNSW SET LOCAL hnsw.ef_search；热更新

search:
  opensearch:
    addresses: ["http://localhost:9200"]  # restart_required
    username: ""                      # 仅 DEEPWIKI_OPENSEARCH_USERNAME 注入
    password: ""                      # 仅 DEEPWIKI_OPENSEARCH_PASSWORD 注入
    index_prefix: deepwiki            # 索引名 <prefix>-chunks-<repo_id 小写>；restart_required

queue:
  rabbitmq:
    url: ""                           # 仅 DEEPWIKI_RABBITMQ_URL 注入；restart_required
    prefetch: 2                       # 缺省 = worker.pool_size；restart_required
    max_retries: 3                    # DLX 重试链次数；热更新

redis:
  sentinel:
    addresses: ["localhost:26379"]    # DEEPWIKI_REDIS_SENTINEL_ADDRESSES 可覆盖；restart_required
    master_name: deepwiki-master
  password: ""                        # 仅 DEEPWIKI_REDIS_PASSWORD 注入

etcd:
  endpoints: ["localhost:2379"]       # DEEPWIKI_ETCD_ENDPOINTS 可覆盖；restart_required
  prefix: /deepwiki                   # 键空间 /deepwiki/config/*、/deepwiki/config_version、/deepwiki/audit/*

git:
  op_timeout: 10m                     # 单次 git CLI 操作超时；热更新
  binary_path: git                    # restart_required

observability:
  otel_endpoint: ""                   # OTLP gRPC；空 = 禁用（零成本）
  log:
    level: info                       # debug|info|warn|error；热更新
    format: json                      # json|console
```

**② `.env.example`** — 环境变量样例（与总纲 §5.3 清单逐字一致）。

```bash
# ---- 保留项（密钥，禁止落 yaml） ----
# 普通 API key 列表，逗号分隔；留空 = 开发模式（跳过鉴权并 WARN）
DEEPWIKI_API_KEYS=
# admin key（PUT /api/v1/config 用）
DEEPWIKI_ADMIN_KEY=
# Embedding provider 密钥（yaml 不落明文）
DEEPWIKI_EMBEDDING_API_KEY=
# LLM provider 密钥（yaml 不落明文）
DEEPWIKI_LLM_API_KEY=

# ---- 新增项（基础设施坐标与凭据，禁止落 yaml） ----
# Postgres DSN（pgxpool；开发态指向 docker-compose 的 postgres 服务）
DEEPWIKI_POSTGRES_DSN=postgres://deepwiki:deepwiki@localhost:5432/deepwiki?sslmode=disable
# OpenSearch 认证（开发态 plugins.security.disabled=true，可留空）
DEEPWIKI_OPENSEARCH_USERNAME=
DEEPWIKI_OPENSEARCH_PASSWORD=
# RabbitMQ 连接 URL（开发态指向 docker-compose 的 rabbitmq 服务）
DEEPWIKI_RABBITMQ_URL=amqp://guest:guest@localhost:5672/
# Redis 哨兵地址列表，逗号分隔（覆盖 yaml redis.sentinel.addresses）
DEEPWIKI_REDIS_SENTINEL_ADDRESSES=localhost:26379,localhost:26380,localhost:26381
# Redis 密码（开发态无密码，留空）
DEEPWIKI_REDIS_PASSWORD=
# etcd 端点列表，逗号分隔（覆盖 yaml etcd.endpoints）
DEEPWIKI_ETCD_ENDPOINTS=localhost:2379
```

**③ `.gitignore`**

```gitignore
# 构建产物
bin/

# 运行数据（保留 data/.gitkeep）
data/*
!data/.gitkeep

# 环境变量与密钥（硬约束 #2）
.env

# 日志与系统文件
*.log
.DS_Store
.idea/
.vscode/
```

**④ `data/.gitkeep`** — 空文件（使 `data/` 目录纳入 git；运行时内容由 `.gitignore` 忽略）。

**⑤ `docker-compose.yml`** — 一键基础设施（总纲 §3.1 权威清单：postgres(pgvector) / opensearch 单节点 / rabbitmq-management / redis 1主2从3哨兵 / etcd；含 healthcheck、卷、ulimits；默认口令仅开发态占位，生产必须经环境覆盖）。

```yaml
# DeepWiki(Go版) 开发/验收态基础设施（总纲 §3.1）。
# 用法：make infra-up（= docker compose up -d）→ docker compose ps 确认全部 healthy。
# 注意：本文件中的 deepwiki/deepwiki 与 guest/guest 仅为开发态默认口令占位，禁止用于生产。
services:
  postgres:
    image: pgvector/pgvector:pg16            # 含 pgvector 扩展（CREATE EXTENSION vector 由迁移完成）
    container_name: deepwiki-postgres
    environment:
      POSTGRES_DB: deepwiki
      POSTGRES_USER: deepwiki
      POSTGRES_PASSWORD: deepwiki            # 开发态占位
    command: postgres -c shared_buffers=256MB
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U deepwiki -d deepwiki"]
      interval: 5s
      timeout: 3s
      retries: 20

  opensearch:
    image: opensearchproject/opensearch:2.17.1
    container_name: deepwiki-opensearch
    environment:
      discovery.type: single-node            # 开发态单节点；生产 3 节点 number_of_replicas=1
      plugins.security.disabled: "true"      # 开发态关安全插件；OPENSEARCH_INITIAL_ADMIN_PASSWORD 仅生产需要
      bootstrap.memory_lock: "true"
      ES_JAVA_OPTS: "-Xms512m -Xmx512m"
    ports:
      - "9200:9200"
    volumes:
      - osdata:/usr/share/opensearch/data
    ulimits:
      memlock:
        soft: -1
        hard: -1
      nofile:
        soft: 65536
        hard: 65536
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS http://localhost:9200/_cluster/health || exit 1"]
      interval: 10s
      timeout: 5s
      retries: 30
      start_period: 30s

  rabbitmq:
    image: rabbitmq:3.13.7-management        # management UI：http://localhost:15672（验收拓扑用）
    container_name: deepwiki-rabbitmq
    environment:
      RABBITMQ_DEFAULT_USER: guest           # 开发态占位
      RABBITMQ_DEFAULT_PASS: guest
    ports:
      - "5672:5672"                          # AMQP
      - "15672:15672"                        # management UI
    volumes:
      - rmqdata:/var/lib/rabbitmq
    healthcheck:
      test: ["CMD", "rabbitmq-diagnostics", "-q", "ping"]
      interval: 5s
      timeout: 5s
      retries: 20
      start_period: 20s

  redis-master:
    image: redis:7.4.1-alpine
    container_name: deepwiki-redis-master
    command: redis-server --appendonly yes
    ports:
      - "6379:6379"
    volumes:
      - rdmaster:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 20

  redis-replica-1:
    image: redis:7.4.1-alpine
    container_name: deepwiki-redis-replica-1
    command: redis-server --appendonly yes --replicaof redis-master 6379
    ports:
      - "6380:6379"
    volumes:
      - rdreplica1:/data
    depends_on:
      redis-master:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 20

  redis-replica-2:
    image: redis:7.4.1-alpine
    container_name: deepwiki-redis-replica-2
    command: redis-server --appendonly yes --replicaof redis-master 6379
    ports:
      - "6381:6379"
    volumes:
      - rdreplica2:/data
    depends_on:
      redis-master:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 20

  redis-sentinel-1:
    image: redis:7.4.1-alpine
    container_name: deepwiki-redis-sentinel-1
    # sentinel 配置内联生成：monitor deepwiki-master / down-after 5000ms / failover-timeout 15000ms / quorum 2
    command: >
      sh -c "printf 'port 26379\nsentinel monitor deepwiki-master redis-master 6379 2\nsentinel down-after-milliseconds deepwiki-master 5000\nsentinel failover-timeout deepwiki-master 15000\nsentinel parallel-syncs deepwiki-master 1\n' > /tmp/sentinel.conf
      && redis-server /tmp/sentinel.conf --sentinel"
    ports:
      - "26379:26379"
    depends_on:
      redis-master:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "redis-cli", "-p", "26379", "ping"]
      interval: 5s
      timeout: 3s
      retries: 20

  redis-sentinel-2:
    image: redis:7.4.1-alpine
    container_name: deepwiki-redis-sentinel-2
    command: >
      sh -c "printf 'port 26379\nsentinel monitor deepwiki-master redis-master 6379 2\nsentinel down-after-milliseconds deepwiki-master 5000\nsentinel failover-timeout deepwiki-master 15000\nsentinel parallel-syncs deepwiki-master 1\n' > /tmp/sentinel.conf
      && redis-server /tmp/sentinel.conf --sentinel"
    ports:
      - "26380:26379"
    depends_on:
      redis-master:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "redis-cli", "-p", "26379", "ping"]
      interval: 5s
      timeout: 3s
      retries: 20

  redis-sentinel-3:
    image: redis:7.4.1-alpine
    container_name: deepwiki-redis-sentinel-3
    command: >
      sh -c "printf 'port 26379\nsentinel monitor deepwiki-master redis-master 6379 2\nsentinel down-after-milliseconds deepwiki-master 5000\nsentinel failover-timeout deepwiki-master 15000\nsentinel parallel-syncs deepwiki-master 1\n' > /tmp/sentinel.conf
      && redis-server /tmp/sentinel.conf --sentinel"
    ports:
      - "26381:26379"
    depends_on:
      redis-master:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "redis-cli", "-p", "26379", "ping"]
      interval: 5s
      timeout: 3s
      retries: 20

  etcd:
    image: quay.io/coreos/etcd:v3.5.21       # 开发态单节点；生产 3 节点集群（总纲 §3.2）
    container_name: deepwiki-etcd
    environment:
      ETCD_NAME: deepwiki-etcd-0
      ETCD_DATA_DIR: /etcd-data
      ETCD_LISTEN_CLIENT_URLS: http://0.0.0.0:2379
      ETCD_ADVERTISE_CLIENT_URLS: http://etcd:2379
      ETCD_LISTEN_PEER_URLS: http://0.0.0.0:2380
      ETCD_INITIAL_ADVERTISE_PEER_URLS: http://etcd:2380
      ETCD_INITIAL_CLUSTER: deepwiki-etcd-0=http://etcd:2380
      ETCD_INITIAL_CLUSTER_STATE: new
    command: ["etcd"]
    ports:
      - "2379:2379"
    volumes:
      - etcddata:/etcd-data
    healthcheck:
      test: ["CMD", "etcdctl", "--endpoints=http://localhost:2379", "endpoint", "health"]
      interval: 5s
      timeout: 3s
      retries: 20

volumes:
  pgdata:
  osdata:
  rmqdata:
  rdmaster:
  rdreplica1:
  rdreplica2:
  etcddata:
```

**⑥ `Dockerfile`** — 多阶段构建（golang:1.22-alpine build → alpine 运行镜像；**必须 `apk add --no-cache git ca-certificates`**：git CLI 为系统依赖，总纲 R5/§4.6）。

```dockerfile
# syntax=docker/dockerfile:1
# DeepWiki(Go版) 多阶段构建（Go 1.22+；纯静态二进制，CGO 关闭——全部依赖均为纯 Go 客户端）。

# ---- 构建阶段 ----
FROM golang:1.22-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/deepwiki ./cmd/server

# ---- 运行阶段 ----
FROM alpine:3.20
# git ≥ 2.30（硬约束 R5：git CLI 为部署前置依赖；验收红线：容器内 git --version 可用）
# ca-certificates：HTTPS 克隆与 provider API 调用所需根证书
RUN apk add --no-cache git ca-certificates \
    && adduser -D -u 10001 deepwiki
WORKDIR /app
COPY --from=build /out/deepwiki /app/deepwiki
COPY configs/ /app/configs/
COPY migrations/ /app/migrations/
RUN mkdir -p /app/data/repos && chown -R deepwiki:deepwiki /app/data
USER deepwiki
EXPOSE 8080
# 密钥与基础设施凭据经环境变量注入（DEEPWIKI_*，见 .env.example），禁止写入镜像。
ENTRYPOINT ["/app/deepwiki", "-config", "configs/config.yaml"]
```

**⑦ `Makefile`**

```makefile
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
```

**⑧ `docs/README.md`** — API 契约占位。

```markdown
# DeepWiki(Go版) API 文档（占位）

API 契约以《DeepWiki(Go版) 系统设计基线》§5、§6 与《01_API_正式版》为唯一权威：
- 端点总表：§5.1（18 个端点，/api/v1 前缀，/metrics 除外）
- 统一信封与错误码：§5.2、§5.3（v2 新增 50302/50303/50304/50203 四个基础设施错误码）
- 关键端点 Schema：§6.1~§6.7（ingest / ask / ask-stream SSE / events SSE / config / health / 其余端点）
- health 响应新契约：见《03_企业级技术栈变更总纲》§7（postgres/opensearch/rabbitmq/redis/etcd/git 六个依赖字段）

本目录在后续迭代中补充 OpenAPI / 示例集合。
```

**⑨ `README.md`** — 项目说明。

```markdown
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
```

---

## 6. 创建顺序（6 步，每步完成必须自检通过再进下一步）

**第①步：拉起基础设施 + 初始化模块与依赖**
1. `mkdir -p deepwiki && cd deepwiki`，先创建 `docker-compose.yml`（§5.13 ⑤）与 `.env`（由 `.env.example` 复制），执行 `make infra-up`（等价 `docker compose up -d`），`docker compose ps` 确认 **postgres / opensearch / rabbitmq / redis-master / redis-replica-1 / redis-replica-2 / redis-sentinel-1/2/3 / etcd 全部 healthy**（未 healthy 不得进入下一步）。
2. `go mod init deepwiki`，创建最小占位 `cmd/server/main.go`（`package main` + 空 `func main(){}`，第⑥步用完整代码覆盖）。
3. 逐条锁定依赖（禁止 latest；完整清单以第 2 章依赖版本锁定表为准）：
   ```bash
   go get github.com/gin-gonic/gin@v1.10.0
   go get github.com/gorilla/websocket@v1.5.3
   go get go.uber.org/zap@v1.27.0
   go get github.com/spf13/viper@v1.19.0
   go get github.com/go-playground/validator/v10@v10.22.1
   go get golang.org/x/time@v0.5.0
   go get github.com/oklog/ulid/v2@v2.1.0
   go get github.com/prometheus/client_golang@v1.20.5
   go get github.com/jackc/pgx/v5@v5.7.1
   go get github.com/pgvector/pgvector-go@v0.2.3
   go get github.com/opensearch-project/opensearch-go/v4@v4.4.0
   go get github.com/golang-migrate/migrate/v4@v4.18.1
   go get github.com/rabbitmq/amqp091-go@v1.10.0
   go get github.com/redis/go-redis/v9@v9.7.0
   go get go.etcd.io/etcd/client/v3@v3.5.21
   go get github.com/sony/gobreaker/v2@v2.3.0
   go get go.opentelemetry.io/otel@v1.35.0
   go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc@v1.35.0
   go get github.com/openai/openai-go@v1.12.0
   go get github.com/anthropics/anthropic-sdk-go@v1.5.0
   go get google.golang.org/genai@v1.20.0
   go get github.com/ollama/ollama@v0.11.4
   # 另含 include/exclude 规则匹配小工具（.gitignore 语义），见第 2 章依赖表
   ```
4. 自检：`go build ./...` 通过；`docker compose ps` 全部 healthy。

**第②步：model 层**（§5.1 全部 6 个文件，完整代码；含 v2 新增错误码 50302/50303/50304/50203 与 `ErrQueueUnavailable` 等错误变量）
自检：`go build ./internal/model/` 通过。

**第③步：migrations 与 store 层**（`migrations/migrations.go`（embed.FS 导出）、`migrations/000001_init.up.sql`（含 `CREATE EXTENSION IF NOT EXISTS vector`、api_keys 表、timestamptz 列、JSONB 列、HNSW 索引，只有 .up 没有 .down）；`internal/store/` 全部 8 个文件：postgres.go(pgxpool) / migrate.go(golang-migrate) / repo_store.go / chunk_store.go / vector_store.go(pgvector) / wiki_store.go / apikey_store.go / doc.go）
自检：`go build ./migrations/ ./internal/store/` 通过；`make infra-up` 已就绪时人工预检 `make migrate-status` 命令可连库（第⑥步后正式验收 version=1）。

**第④步：基础设施客户端包**（`internal/queue/` 4 个、`internal/search/` 1 个、`internal/ratelimit/` 3 个、`internal/lock/` 1 个、`internal/eventbus/` 3 个、`internal/config/` 4 个、`internal/observability/` 2 个）
自检：`go build ./internal/queue/ ./internal/search/ ./internal/ratelimit/ ./internal/lock/ ./internal/eventbus/ ./internal/config/ ./internal/observability/` 通过。

**第⑤步：provider 接口与 pipeline + task**（`internal/embed/` 7 个、`internal/llm/` 7 个、`internal/retriever/` 5 个、`internal/ingest/` 5 个（cloner 为 git CLI 实现，`git clone --depth 1` + `os.Rename` 原子就位；refresh 为 `fetch --depth 1` + `reset --hard FETCH_HEAD` + `clean -fdx`，git CLI 替代 v1 原方案的 go-git 库）、`internal/task/` 4 个）
自检：`go build ./internal/embed/ ./internal/llm/ ./internal/retriever/ ./internal/ingest/ ./internal/task/` 通过。

**第⑥步：service + api 装配 + main + 非 Go 文件**（`internal/service/` 4 个、`internal/api/` 全部 19 个、`cmd/server/main.go` 完整覆盖、`configs/config.yaml`、`.env.example`、`.gitignore`、`docker-compose.yml`、`Dockerfile`、`Makefile`、`data/.gitkeep`、`docs/README.md`、`README.md`）
1. `go mod tidy`
2. 自检：`go build ./...` 通过；`go vet ./...` 通过；`make tidy` 无 diff；`go run ./cmd/server` 可启动（见第 7 章验收）。

---

## 7. 验收标准

### 7.1 编译与静态检查（必须全绿）

```bash
go build ./...     # 必须通过
go vet ./...       # 必须通过
go mod tidy        # 执行后 git diff --exit-code go.mod go.sum 必须无 diff
                   # （全部新依赖已锁定版本，禁止 latest / 伪版本漂移）
docker compose ps  # 全部服务 STATUS 为 (healthy)
```

### 7.2 启动

```bash
set -a; source .env; set +a
go run ./cmd/server        # 或 make run
# 期望日志（JSON，按序出现）：
# {"level":"info","msg":"migration applied","version":1,"file":"000001_init.up.sql"}
# {"level":"info","msg":"opensearch connected","addresses":["http://localhost:9200"]}
# {"level":"info","msg":"rabbitmq topology declared","exchange":"deepwiki.tasks","queue":"deepwiki.task.jobs"}
# {"level":"info","msg":"redis sentinel master discovered","master_name":"deepwiki-master"}
# {"level":"info","msg":"etcd watch established","prefix":"/deepwiki/config/"}
# {"level":"info","msg":"deepwiki listening","addr":":8080","version":"0.2.0"}
```

### 7.3 验收 curl 清单

```bash
# ① 健康检查（骨架阶段允许 status=degraded、llm/embedding reachable=false）
curl -s http://localhost:8080/api/v1/health | jq .
# 期望：HTTP 200；响应头含 X-Request-ID: req_<26位ULID>；body 形如（总纲 §7 新契约）：
# {
#   "code": 0, "message": "ok",
#   "data": {
#     "status": "degraded", "version": "0.2.0",
#     "uptime_seconds": <int>, "started_at": "<UTC RFC3339>",
#     "llm":       {"provider":"openai","model":"gpt-4o-mini","reachable":false,"breaker":"closed"},
#     "embedding": {"provider":"openai","model":"text-embedding-3-small","dimensions":1536,"reachable":false,"breaker":"closed"},
#     "postgres":  {"connected":true,"pool":{"total":10,"idle":8},"migration_version":1},
#     "opensearch":{"connected":true,"cluster_status":"green","indices":0},
#     "rabbitmq":  {"connected":true,"queue_depth":0,"consumers":0},
#     "redis":     {"connected":true,"mode":"sentinel","master":"redis-master:6379","ratelimit_degraded":false},
#     "etcd":      {"connected":true,"endpoints":["localhost:2379"]},
#     "git":       {"available":true,"version":"2.43.0"},
#     "worker":    {"busy":0,"total":2,"queued":0}
#   },
#   "request_id": "req_..."
# }

# ② Prometheus 指标
curl -s http://localhost:8080/metrics | head -5
# 期望：HTTP 200，Prometheus text 格式（# HELP / # TYPE 开头）；
# 且包含 deepwiki_worker_busy / deepwiki_queue_length 等已注册指标

# ③ 未实现端点的信封形状（骨架阶段返回 50001 占位，信封格式必须正确）
curl -s -X POST http://localhost:8080/api/v1/ingest \
  -H 'Content-Type: application/json' \
  -d '{"repo_url":"https://github.com/gin-gonic/gin"}' | jq .
# 期望：HTTP 500；{"code":50001,"message":"internal error: endpoint not implemented yet (scaffold)","request_id":"req_..."}

# ④ 任务列表（骨架阶段同样返回 50001 占位信封，信封格式必须正确）
curl -s 'http://localhost:8080/api/v1/tasks?page=1&page_size=20' | jq .

# ⑤ 未注册路径
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8080/api/v1/nope
# 期望：404（gin 默认）

# ⑥ 优雅退出：运行中按 Ctrl-C
# 期望日志依次出现：shutting down → bye；进程退出码 0；无 panic；
# 且关闭顺序为 rabbitmq → redis → opensearch → postgres → etcd（硬约束 #10）
```

### 7.4 验收红线（任何一条不满足即返工）

- `go build ./...` 与 `go vet ./...` 必须零错误零警告；`go mod tidy` 后 `go.mod`/`go.sum` 无 diff（全部新依赖锁定版本）。
- 目录树与第 4 章完全一致（约 90 个文件），无多余文件（`go.sum` 除外，由工具链生成）。
- 全部 20 个错误码（v1 16 个 + v2 新增 50302/50303/50304/50203）、全部接口签名、Config 全部字段、`000001_init.up.sql` 全文与本文档逐字符一致。
- `docker compose ps` 全部服务 healthy（postgres / opensearch / rabbitmq / redis-master / redis-replica-1/2 / redis-sentinel-1/2/3 / etcd）。
- `docker build` 产物容器内 `git --version` 可用且 ≥ 2.30（git CLI 为系统依赖，Dockerfile `apk add --no-cache git ca-certificates`）。
- Postgres 中 `schema_migrations` 版本 = 1 且 dirty=false；`\dx` 可见 `vector` 扩展已创建（`CREATE EXTENSION IF NOT EXISTS vector` 在 000001_init.up.sql 内）。
- RabbitMQ management（http://localhost:15672）可见队列 `deepwiki.task.jobs`（x-max-length=100、x-dead-letter-exchange=deepwiki.tasks.dlx）、`deepwiki.task.retry.{5s,30s,5m}` 与 `deepwiki.task.dlq`。
- health 响应必须包含 `postgres` / `opensearch` / `rabbitmq` / `redis` / `etcd` / `git` 六个依赖字段，且字段名与总纲 §7 逐字一致。
- `git pull` 只允许出现在 cloner.go 的禁止性注释中，任何代码路径不得实现 pull 语义（refresh 必须是 `fetch --depth 1` + `reset --hard FETCH_HEAD` + `clean -fdx`）；禁止 `sh -c` 拼接 git 命令；`grep -rn "TODO" internal/` 结果只能出现在本文档指定的骨架函数体内。
- `configs/config.yaml` 与全部 Go 源码中不得出现任何明文密钥与基础设施凭据（docker-compose.yml 中的开发态默认口令占位除外，且必须带注释说明）。

---

## 8. 后续迭代建议（给用户）

1. **先跑通骨架再动手**：本文档执行完、验收全绿后，把骨架（含 docker-compose.yml 与 Dockerfile）提交一个 commit 作为基线，后续每轮迭代一个 commit，便于回滚与 review。
2. **按依赖顺序分模块让 Kimi Code 实现**：ingest pipeline（git CLI cloner 已有 → parser → chunker → persist：Postgres 事务落 chunks+pgvector，事务提交后 bulk 写 OpenSearch，顺序约定不变）→ task 系统（consumer/pool/manager 真实化：CAS 抢占、DLX 重试链、Reconciler 周期补偿、取消与恢复）→ ask 链路（retriever 三件套：keyword=OpenSearch、embedding=pgvector、hybrid=RRF → LLM 官方 SDK adapter + gobreaker → AskService → SSE）→ wiki 生成 → eventbus 扇出（Redis Streams 回放 + Pub/Sub 跨节点）→ config 热更新（etcd Txn + watch + 审计）。不要一次性全量生成，质量会崩塌。
3. **每模块实现后人工 review + 红线扫描**：`grep` 检查 `git pull`、`sh -c`、裸 `go func`（无 recover/ctx）、`err.Error()` 回传、明文密钥、字符串拼接 SQL（必须 `$n` 占位）、RabbitMQ 消息体超 4KB（硬约束 #16）；再跑 `go build ./... && go vet ./...` 与模块单测。
4. **下一轮最先补限流中间件**（ratelimit 包 Lua 判定 + middleware 接入，硬约束 #1 当前是直通骨架）与 auth 二级查找真实化（Redis 缓存 → Postgres api_keys）；安全相关不要留到答辩前。
5. **provider 一律官方 SDK adapter**（openai-go / anthropic-sdk-go / genai / ollama api）+ 每实例 gobreaker；SDK 类型不得泄漏到 model 与接口签名（硬约束 #17），base_url 覆盖使 OpenAI 兼容端点统一走 openai-go。
6. **横切逻辑单独成轮**：embedding 维度一致性双保险（应用层探测 + `vector(1536)` 列类型兜底，硬约束 #14）、优雅退出与 Reconciler（硬约束 #10）、进度落库节流、OTel span 覆盖（gin → worker pipeline → pgx/opensearch/rabbitmq 调用点）；并做故障演练——`kill -TERM` 看排空与 nack requeue、`docker compose stop rabbitmq` 看投递 50302、`docker compose stop redis-master` 看哨兵切换与限流降级兜底（`ratelimit_degraded=true`）、`docker compose stop etcd` 看 PUT /config 返回 50304 而 GET 走快照缓存。
7. **每次会话只给 Kimi Code 一个模块的上下文**：贴基线对应章节 + 本文档对应小节 + 相关已有代码，避免上下文污染导致它"自由发挥"。
8. **演示数据提前准备**：选 1~2 个中小型公开仓库（如 gin-gonic/gin 的某个 tag 子集）预跑 ingest，答辩时直接演示 ask / wiki / events，避免现场等 embedding。
9. **生产化路线（答辩加分项，按总纲 §3.2 展开）**：OpenSearch 3 节点集群并 `number_of_replicas=1`；pgvector 调优（`ef_search` 召回率/延迟权衡、`m`/`ef_construction` 重建策略）；RabbitMQ 集群 + quorum 队列替代单节点；etcd 3 节点集群；Redis 哨兵生产参数调优；worker 仓库工作目录换共享卷（NFS/PVC）；API 无状态多副本 + LB。
