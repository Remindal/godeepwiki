# DeepWiki(Go版) API 参考文档 · v1 正式版（企业级技术栈版）

| 项 | 内容 |
|---|---|
| 文档编号 | DW-API-REF-01 |
| API 版本 | v1（正式版） |
| 文档版本 | 0.2.0（企业级技术栈版，health `version` 字段同源） |
| 上游依据 | DW-DESIGN-BASELINE-00（设计基线冻结版）＋《03_企业级技术栈变更总纲 v2.0》（唯一权威契约） |
| Base URL | `http://{host}:8080/api/v1` |
| 内容类型 | 请求/响应主体均为 `application/json; charset=utf-8`（流式端点除外） |
| 适用范围 | 前端、CLI、集成测试与答辩演示脚本的唯一 API 契约 |

> **一致性声明**：本文档的 REST 契约（端点路径/方法/请求字段/响应字段）、统一响应信封、状态机、事件协议、限流规则数值、ID 与时间规范与 v1 冻结版**逐字符一致**；错误码在 v1 基础上**仅新增** 4 个基础设施错误码（50302/50303/50304/50203）；health 响应按总纲 §7 新契约全文替换。实现层（存储/队列/检索/配置中心/事件总线）按总纲整体升级。JSON 字段统一 `snake_case`；时间统一 UTC + RFC3339；ID 统一带类型前缀。
>
> **技术栈变更说明（v2.0）**：对外契约全部冻结不变；实现层由 v1 原方案的「SQLite（modernc 驱动）＋ bleve ＋ go-git ＋ 进程内队列/限流/事件总线」整体替换为「PostgreSQL 16 + pgvector、OpenSearch 2.x、RabbitMQ、Redis 哨兵集群、etcd、系统 git CLI（≥2.30）、各厂商官方 SDK」。本文中所有实现层措辞均以新栈为准。

---

## 目录

- [1. 文档信息](#1-文档信息)
- [2. 通用约定](#2-通用约定)
- [3. 端点详细文档](#3-端点详细文档)
- [4. 流式协议专章](#4-流式协议专章)
- [5. 完整调用流程示例](#5-完整调用流程示例)
- [6. 附录](#6-附录)

---

## 1. 文档信息

### 1.1 版本与 Base URL

| 项 | 值 | 说明 |
|---|---|---|
| API 版本 | `v1` | 路径段中的版本号，冻结后不变 |
| 实现版本 | `0.2.0` | 服务自身版本号，与 health 响应的 `version` 字段一致 |
| Base URL | `http://{host}:8080/api/v1` | 本地默认 `http://localhost:8080/api/v1` |
| 例外端点 | `GET /metrics` | Prometheus 指标**不带版本前缀**，路径为 `http://{host}:8080/metrics` |
| 默认监听 | `:8080`（`server.addr`） | 可经配置/环境变量调整 |

### 1.2 版本化策略

全部业务 API 位于 `/api/v1` 前缀下。未来若出现**不向后兼容**的变更，将升级为 `/api/v2` 并新增前缀；届时 **v1 进入冻结维护状态，至少保持向后兼容一个版本周期**，在此期间 v1 端点不删除、不修改既有字段语义，仅做缺陷修复。客户端应以路径中的版本号为准，不要自行拼接或忽略版本段。

**兼容性承诺（v1 生命周期内允许的变化）**：新增可选请求字段、新增响应字段、新增端点、新增错误码。**不允许的变化**：删除/重命名字段、改变字段类型、改变既有状态机转移、改变既有错误码语义。本次 0.2.0 的 4 个新增错误码与 health 新增字段即属「允许的变化」。

### 1.3 时间格式

全链路时间字段统一为 **UTC + RFC3339**，形如 `2026-07-05T08:30:00Z`（结尾 `Z` 表示 UTC，不带本地偏移）。存储层（Postgres `timestamptz` 列）与 API 层格式完全一致，写入/读出不做时区转换，范围查询与排序直接走索引。客户端解析时应按 RFC3339 处理；展示时由前端自行换算本地时区。

### 1.4 ID 规范

所有全局 ID 由 `oklog/ulid` 生成，带类型前缀。ULID 字典序与时间序一致，利于索引与排序。

| 类型 | 前缀 | 校验正则 | 示例 |
|---|---|---|---|
| 任务 ID | `tsk_` | `^tsk_[0-9A-HJKMNP-TV-Z]{26}$` | `tsk_01J2X9K7QZ0ABCDEFGHJKMNP` |
| 仓库 ID | `repo_` | `^repo_[0-9A-HJKMNP-TV-Z]{26}$` | `repo_01J2X9K7QZ0ABCDEFGHJKMNQ` |
| Chunk ID | `chk_` | `^chk_[0-9A-HJKMNP-TV-Z]{26}$` | `chk_01J2X9K7QZ0ABCDEFGHJKMNT` |
| 请求 ID | `req_` | `^req_[0-9A-HJKMNP-TV-Z]{26}$` | `req_01J2X9K7QZ0ABCDEFGHJKMNR` |

路径参数中的 ID 会先经过「前缀 + ULID 正则」校验再查库/拼路径，以杜绝路径穿越与非法输入。**Wiki 页面**使用人类可读 `slug`（如 `overview`、`module-api`），不是全局 ID，作用域仅限单个仓库内。

> 内部组件另用 `wk_` 前缀（Worker 节点实例 ID，遵守同一 ULID 规范），仅出现在日志与指标标签中，不出现在 API 路径与响应契约中。

### 1.5 快速开始（环境变量约定）

全文 curl 示例统一使用以下 shell 变量，复制前请先 `export`：

```bash
export HOST="http://localhost:8080"
export BASE="$HOST/api/v1"
export KEY="your-api-key"        # 普通 API key（auth.api_keys 之一）
export ADMIN="your-admin-key"    # 管理 key（auth.admin_key）
```

服务启动前还需注入基础设施坐标（v2.0 新增 7 个环境变量，仅环境变量注入、禁止 yaml 明文；下值为 `docker compose up -d` 后的本地默认值）：

```bash
export DEEPWIKI_POSTGRES_DSN="postgres://deepwiki:deepwiki@localhost:5432/deepwiki?sslmode=disable"
export DEEPWIKI_OPENSEARCH_USERNAME=""     # dev 单节点关闭安全插件，可留空
export DEEPWIKI_OPENSEARCH_PASSWORD=""
export DEEPWIKI_RABBITMQ_URL="amqp://guest:guest@localhost:5672/"
export DEEPWIKI_REDIS_SENTINEL_ADDRESSES="localhost:26379,localhost:26380,localhost:26381"
export DEEPWIKI_REDIS_PASSWORD=""          # 哨兵与 master 共用
export DEEPWIKI_ETCD_ENDPOINTS="localhost:2379"
```

> 前置：仓库根目录执行 `docker compose up -d` 拉起 postgres / opensearch / rabbitmq / redis（1 主 2 从 3 哨兵）/ etcd，待全部服务 healthy 后再启动 DeepWiki 服务（详见 §6.4）。

---

## 2. 通用约定

### 2.1 认证

除 `GET /api/v1/health` 与 `GET /metrics` 外，**所有端点都要求 `X-API-Key` 请求头命中** `auth.api_keys` 中配置的任一 key。

| 端点类别 | 请求头 | 判定规则 |
|---|---|---|
| 普通接口 | `X-API-Key: <key>` | `<key>` ∈ `auth.api_keys` |
| `PUT /api/v1/config` | `X-API-Key: <admin_key>` | `<key>` 必须 **等于** `auth.admin_key`；命中普通 key 返回 `40301 forbidden` |
| `GET /api/v1/health`、`GET /metrics` | 无需 | 免鉴权 |

> ⚠️ **开发模式行为（务必知悉）**：当 `auth.api_keys` 为空（默认即空）时，系统进入**开发模式**，**跳过全部鉴权并打印 WARN 日志**。此时任何请求都无需 `X-API-Key` 也能通过。**这仅是本地开发的便利行为，生产环境必须配置 `auth.api_keys` 与 `auth.admin_key`**，否则管理端点与配额体系形同虚设。`auth.api_keys` 与 `auth.admin_key` 只从环境变量 `DEEPWIKI_API_KEYS`（逗号分隔）与 `DEEPWIKI_ADMIN_KEY` 注入，yaml 不落明文。

**实现说明（v2.0，对外契约不变）**：API key 的明文只存在于环境变量。启动时，服务把 `DEEPWIKI_API_KEYS` / `DEEPWIKI_ADMIN_KEY` 中的明文 key 以 `SHA-256(salt‖key)` 哈希后**幂等 upsert** 进 Postgres `api_keys` 表（已存在同 hash 则跳过；admin key 置 `is_admin=true`）：

```sql
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

认证中间件走「**Redis 缓存（TTL 60s）→ Postgres**」二级查找：先查 `auth:key:<sha256(key)>`（值为 `{key_id,is_admin,revoked}` JSON），命中直接判定；未命中查 `api_keys` 表并回写缓存；key 被吊销（`revoked_at` 非空）时主动 `DEL` 缓存键，最长 60s 内全网生效。密钥落库只存哈希，禁止明文进入 Postgres / etcd / 日志。鉴权失败的统一响应见 §2.6 错误码表（`40101` / `40301`）。

### 2.2 统一响应信封

所有 JSON 端点的响应体都包裹在统一信封中。HTTP 状态码表示**传输层结果**，信封中的 `code` 表示**业务结果**，二者同时存在。

**成功信封**：

```json
{
  "code": 0,
  "message": "ok",
  "data": { },
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMNP"
}
```

**失败信封**：

```json
{
  "code": 40001,
  "message": "invalid_param: field question length must be between 1 and 4000",
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMNP",
  "details": [
    { "field": "question", "issue": "length_out_of_range" }
  ]
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `code` | int | 业务错误码；**成功恒为 0**；失败时取 §2.6 错误码表中的值 |
| `message` | string | 脱敏后的面向用户描述；**绝不回传 `err.Error()` 原文**（防路径/SQL/密钥泄漏） |
| `data` | object | 成功时的业务负载；失败信封无此字段 |
| `request_id` | string | 由 RequestID 中间件生成（`req_` + ULID），与 `X-Request-ID` 响应头、全部日志行一致 |
| `details` | array | 可选；校验类错误给出字段级明细 `[{field, issue, ...}]`，可能携带附加上下文（如 `existing_repo_id`） |

### 2.3 request_id 与全链路关联

每个请求由 RequestID 中间件生成一个 `request_id`（`req_` + ULID），同时出现在：

- 响应头的 `X-Request-ID`；
- 响应信封的 `request_id` 字段；
- 该请求相关的全部 zap 结构化日志行。

排查问题时，请把 `X-Request-ID` 提供给运维，据此可在日志中串联该请求的完整处理轨迹（含派生的 `task_id` / `repo_id`）。开启 OpenTelemetry（`observability.otel_endpoint` 非空）后，`request_id` 同时写入 trace/span 属性，可在链路追踪系统中按同一 ID 关联 API → Worker → git/LLM/DB 的完整调用链。

### 2.4 限流（两级）

系统采用**两级限流**，禁止用单一全局桶。命中限流返回 `429 + 42901 rate_limited`。

| 级别 | 键 | 算法 | 参数（默认） | 作用范围 | 目的 |
|---|---|---|---|---|---|
| L1 per-IP | 客户端 IP（`RemoteAddr`；仅在配置可信代理后采信 `X-Forwarded-For`） | 滑动窗口 | `rps=10, burst=20` | 全部 `/api/v1/*` | 兜底防滥用、防误刷 |
| L2 per-API-key | `X-API-Key` | 滑动窗口 | `ingest_per_hour=20` | `POST /ingest`、`POST /repos/{id}/refresh`、`POST /wiki/generate` | 昂贵端点（git/embedding/LLM 成本）配额 |
| L2 per-API-key | `X-API-Key` | 滑动窗口 | `ask_per_minute=30` | `POST /ask`、`POST /ask/stream` | LLM 成本配额 |

**限流规则数值与响应头契约为冻结项，与 v1 完全一致；仅存储实现由进程内桶替换为 Redis Lua 滑动窗口（哨兵集群），语义与数值不动。**

实现要点（v2.0）：限流判定全部走 **Redis 分布式滑动窗口**，Lua 脚本原子执行（客户端经 `redis.NewFailoverClient` 接哨兵，`MasterName=deepwiki-master`）：

```lua
-- KEYS[1]=窗口键  ARGV=now_ms, window_ms, limit
redis.call('ZREMRANGEBYSCORE', KEYS[1], 0, ARGV[1]-ARGV[2])
local n = redis.call('ZCARD', KEYS[1])
if n < tonumber(ARGV[3]) then
  redis.call('ZADD', KEYS[1], ARGV[1], ARGV[1]..'-'..math.random(1,1e9))
  redis.call('PEXPIRE', KEYS[1], ARGV[2])
  return {1, tonumber(ARGV[3])-n-1}
end
return {0, 0}
```

Redis 键与窗口/配额换算（与上表数值一一对应）：`rl:ip:<ip>`（窗口 60s，`limit = rps×60 + burst`，默认 `10×60+20=620`）、`rl:key:<key_hash>:ingest`（窗口 3600s，limit=20）、`rl:key:<key_hash>:ask`（窗口 60s，limit=30）。响应头三件套由 Lua 返回的剩余量与键 TTL 计算，契约不变。

**降级**：Redis 不可用时降级为**进程内 x/time/rate 兜底**并打 WARN 日志，同时 health 置 `degraded`（`redis.ratelimit_degraded=true`，指标 `deepwiki_ratelimit_degraded_total` 递增）。多副本下兜底桶只能近似限流、无法全局一致，这是「可用性优先」的有意取舍。开发模式（无 API key）下 L2 退化为按 IP 计数，与 v1 行为一致。

**限流响应契约**：命中时必带以下响应头；未命中的受限端点响应同样携带 `X-RateLimit-*` 三件套，便于客户端自我节流。

| 响应头 | 说明 |
|---|---|
| `Retry-After: <seconds>` | 桶恢复一个令牌的预计秒数 |
| `X-RateLimit-Limit` | 该桶的配额上限 |
| `X-RateLimit-Remaining` | 当前剩余令牌数 |
| `X-RateLimit-Reset` | 桶重置的 UTC epoch 秒 |

**429 响应示例**：

```http
HTTP/1.1 429 Too Many Requests
Retry-After: 37
X-RateLimit-Limit: 30
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1751727400
X-Request-ID: req_01J2X9K7QZ0ABCDEFGHJKMNU
Content-Type: application/json; charset=utf-8

{
  "code": 42901,
  "message": "rate limited",
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMNU"
}
```

> 另有 `42902 queue_full` 属「任务队列满」背压（非请求限流），仅在建任务类端点出现，同样带 `Retry-After` 头（估算规则：`clamp(queued / pool_size × avg_task_seconds, 10, 600)`），详见 §3.3。

### 2.5 分页

所有列表端点（`GET /repos`、`GET /tasks`）遵循统一分页规范。

| 项 | 约定 |
|---|---|
| 请求参数 | `page`（≥1，默认 1）、`page_size`（1~100，默认 20） |
| 排序 | 固定 `created_at DESC`（任务/仓库均按创建时间倒序） |
| 响应结构 | `data: { items: [...], pagination: { page, page_size, total, total_pages } }` |
| 越界 | `page` 超出范围时返回空 `items` 与真实 `total`，**不报错** |

`pagination` 字段说明：

| 字段 | 类型 | 说明 |
|---|---|---|
| `page` | int | 当前页码（从 1 开始） |
| `page_size` | int | 每页条数 |
| `total` | int | 符合条件的总条数 |
| `total_pages` | int | 总页数（`ceil(total / page_size)`） |

### 2.6 错误码总表

下表为系统全部 **20 个错误码**：前 16 个与 v1 完全一致（冻结，不得增删改语义），后 4 个（`50203`/`50302`/`50303`/`50304`）为 v2.0 按总纲 §6 **新增**的基础设施错误码。`50004 task_interrupted` 仅出现在任务的 `error.code` 中（重启恢复时写入），不直接作为 API 响应码。

| code | 常量名 | HTTP | 含义 | 典型触发场景 | 客户端建议动作 |
|---|---|---|---|---|---|
| 40001 | `invalid_param` | 400 | 参数校验失败 | validator 校验不通过；`repo_url` 非法；ID 格式不符；仓库体积超限；PUT /config 写入基础设施坐标类 key | 检查 `details` 字段级明细，修正请求参数后重试 |
| 40101 | `unauthorized` | 401 | 未鉴权 | 缺失/无效 `X-API-Key` | 携带有效的 `X-API-Key` 重试 |
| 40301 | `forbidden` | 403 | 无权限 | 用普通 key 调用 `PUT /api/v1/config` | 改用 admin key（`X-API-Key` 值等于 `auth.admin_key`） |
| 40401 | `task_not_found` | 404 | 任务不存在 | `task_id` 查无 | 确认 `task_id` 正确；任务可能因仓库被删而仍存历史 |
| 40402 | `repo_not_found` | 404 | 仓库不存在 | `repo_id` 查无 | 确认 `repo_id`；仓库可能已被删除 |
| 40403 | `wiki_not_found` | 404 | Wiki 未生成 | wiki 任务未跑过或被级联删除 | 先调用 `POST /wiki/generate` 生成 |
| 40901 | `repo_already_exists` | 409 | 仓库已存在且 commit 未变 | ingest 幂等命中（`details` 附 `existing_repo_id`） | 复用已有 `repo_id`；若远端已更新，改走 refresh |
| 40902 | `invalid_task_state` | 409 | 任务状态不允许该操作 | 对终态任务取消；仓库非 `ready` 时 ask/refresh/wiki；非法状态转移 | 等待任务/仓库进入合适状态后再操作 |
| 42201 | `config_validation_failed` | 422 | 配置校验失败 | `PUT /config` 校验不通过（范围/枚举/跨字段/embedding 维度） | 按 `message` 修正配置；维度不一致时需重建索引 |
| 42901 | `rate_limited` | 429 | 限流命中 | per-IP 窗口或 per-key 配额耗尽 | 按 `Retry-After` 等待后重试；降低请求频率 |
| 42902 | `queue_full` | 429 | 任务队列满 | RabbitMQ 队列深度 ≥ `worker.queue_size`（x-max-length） | 按 `Retry-After` 退避后重试；勿立即重提交 |
| 50001 | `internal_error` | 500 | 内部错误 | 未分类错误；panic recovery | 记录 `request_id` 联系运维；可稍后重试 |
| 50004 | `task_interrupted` | 500 | 任务被中断 | 重启恢复时不可安全重投的非终态任务落库（仅在 `task.error` 出现） | 按原 `request_payload` 对应端点重新提交任务 |
| 50201 | `llm_unavailable` | 502 | LLM 服务不可用 | 重试耗尽、连接失败、持续 5xx、熔断器 open | 稍后重试；检查 LLM provider 与网络 |
| 50202 | `embedding_unavailable` | 502 | Embedding 服务不可用 | 同上（embedding 链路） | 稍后重试；检查 Embedding provider 与网络 |
| 50203 | `vector_store_unavailable` | 502 | 向量检索暂不可用 | Postgres/pgvector 查询失败（ask 的 embedding 检索路径） | 稍后重试；持续出现时检查 health 的 `postgres` 字段与 pgvector 扩展状态 |
| 50301 | `service_not_ready` | 503 | 服务未就绪 | 启动未完成或优雅退出中（readiness=false） | 等待就绪（health 返回 200）后再请求 |
| 50302 | `queue_unavailable` | 503 | 任务队列暂不可用，请稍后重试 | RabbitMQ 连接失败或发布确认（publisher confirm）失败 | 指数退避重试；持续失败时查看 health 的 `rabbitmq` 字段 |
| 50303 | `search_unavailable` | 503 | 检索服务暂不可用 | OpenSearch 不可用且影响 ask 检索 | 稍后重试；检查 health 的 `opensearch` 字段与集群状态 |
| 50304 | `config_store_unavailable` | 503 | 配置中心暂不可用 | etcd 写路径不可用（`PUT /config`；GET 走本地快照缓存不报错） | 稍后重试 PUT；检查 health 的 `etcd` 字段与 etcd 集群 |

> health 的 `degraded` 映射：postgres / opensearch / rabbitmq / redis / etcd 任一异常（含限流降级中、git 不可用）→ `status=degraded`；readiness=false 时返回 503 + `50301`，语义不变。

---

## 3. 端点详细文档

端点总览（18 个，与基线 API 总表一致）：

| # | 方法 | 路径 | 成功码 | 鉴权 | 限流桶 |
|---|---|---|---|---|---|
| 1 | GET | `/api/v1/health` | 200 | 免 | per-IP |
| 2 | GET | `/metrics` | 200 | 免 | 不限 |
| 3 | POST | `/api/v1/ingest` | 202 | API key | per-IP + ingest_per_hour |
| 4 | GET | `/api/v1/repos` | 200 | API key | per-IP |
| 5 | GET | `/api/v1/repos/{repo_id}` | 200 | API key | per-IP |
| 6 | DELETE | `/api/v1/repos/{repo_id}` | 200 | API key | per-IP |
| 7 | POST | `/api/v1/repos/{repo_id}/refresh` | 202 | API key | per-IP + ingest_per_hour |
| 8 | GET | `/api/v1/tasks` | 200 | API key | per-IP |
| 9 | GET | `/api/v1/tasks/{task_id}` | 200 | API key | per-IP |
| 10 | DELETE | `/api/v1/tasks/{task_id}` | 202 | API key | per-IP |
| 11 | POST | `/api/v1/ask` | 200 | API key | per-IP + ask_per_minute |
| 12 | POST | `/api/v1/ask/stream` | 200 | API key | per-IP + ask_per_minute |
| 13 | POST | `/api/v1/wiki/generate` | 202 | API key | per-IP + ingest_per_hour |
| 14 | GET | `/api/v1/repos/{repo_id}/wiki` | 200 | API key | per-IP |
| 15 | GET | `/api/v1/config` | 200 | API key | per-IP |
| 16 | PUT | `/api/v1/config` | 200 | admin key | per-IP |
| 17 | GET | `/api/v1/events` | 200 | API key | per-IP |
| 18 | GET | `/api/v1/ws` | 101 | API key | per-IP |

---

### 3.1 GET /api/v1/health — 健康检查

**用途**：返回服务及全部依赖的运行时健康信息（LLM / Embedding / Postgres / OpenSearch / RabbitMQ / Redis / etcd / git / Worker），供负载均衡探活与运维诊断。

**设计解释**：health 返回的是**缓存的探测结果**而非实时外部调用——`llm.reachable` / `embedding.reachable` / `postgres.connected` / `opensearch.connected` / `rabbitmq.connected` / `redis.connected` / `etcd.connected` 由启动时与每 60s 的后台探测维护，`git.available` 在启动时经 `git --version` 探测并随后台心跳复测，因此本接口毫秒级返回、不会成为新的故障放大点。它同时暴露 Worker 池的 `busy/total/queued` 实时值（`queued` 即 RabbitMQ `deepwiki.task.jobs` 队列深度），让调用方一眼看出系统是否拥堵。优雅退出期间 readiness 置失败，本接口返回 `503 + 50301` 但保留 `status` 字段供诊断，负载均衡据此摘流量。

| 方法 | 路径 | 鉴权 | 限流 |
|---|---|---|---|
| GET | `/api/v1/health` | 免 | per-IP |

**请求**：无路径参数、无查询参数、无请求体、无需请求头。

**成功响应（200）**（总纲 §7 新契约）：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "status": "ok",
    "version": "0.2.0",
    "uptime_seconds": 3661,
    "started_at": "2026-07-05T08:30:00Z",
    "llm":       { "provider": "openai", "model": "gpt-4o-mini", "reachable": true, "breaker": "closed" },
    "embedding": { "provider": "openai", "model": "text-embedding-3-small", "dimensions": 1536, "reachable": true, "breaker": "closed" },
    "postgres":  { "connected": true, "pool": { "total": 10, "idle": 8 }, "migration_version": 1 },
    "opensearch":{ "connected": true, "cluster_status": "green", "indices": 3 },
    "rabbitmq":  { "connected": true, "queue_depth": 0, "consumers": 2 },
    "redis":     { "connected": true, "mode": "sentinel", "master": "redis-master:6379", "ratelimit_degraded": false },
    "etcd":      { "connected": true, "endpoints": ["localhost:2379"] },
    "git":       { "available": true, "version": "2.43.0" },
    "worker":    { "busy": 0, "total": 2, "queued": 0 }
  },
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMNV"
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `status` | string | `ok` = 全部依赖正常；`degraded` = 任一依赖异常（llm/embedding 不可达或熔断打开、postgres/opensearch/rabbitmq/redis/etcd 异常、git 不可用、限流降级中、维度不一致拒绝服务中） |
| `version` | string | 服务版本号（当前 `0.2.0`） |
| `uptime_seconds` | int | 已运行秒数 |
| `started_at` | string | 启动时间（UTC RFC3339） |
| `llm` | object | `{provider, model, reachable, breaker}`；provider 取值见附录配置；`breaker` 为 gobreaker 熔断器状态（`closed`/`open`/`half_open`） |
| `embedding` | object | `{provider, model, dimensions, reachable, breaker}`；`breaker` 同上 |
| `postgres` | object | `{connected, pool, migration_version}`；`pool` 为 pgxpool 实时值 `{total, idle}`；`migration_version` 为 golang-migrate 当前迁移版本（`schema_migrations`） |
| `opensearch` | object | `{connected, cluster_status, indices}`；`cluster_status` 为集群健康色（`green`/`yellow`/`red`）；`indices` 为 `deepwiki-chunks-*` 索引数量 |
| `rabbitmq` | object | `{connected, queue_depth, consumers}`；`queue_depth` 为 `deepwiki.task.jobs` 当前消息数；`consumers` 为已注册消费者数 |
| `redis` | object | `{connected, mode, master, ratelimit_degraded}`；`mode=sentinel` 表示经哨兵接入；`ratelimit_degraded=true` 表示限流已降级进程内兜底（§2.4） |
| `etcd` | object | `{connected, endpoints}`；配置中心连接状态与客户端端点 |
| `git` | object | `{available, version}`；启动时 `git --version` 解析；缺失或版本 < 2.30 → `available=false` 且 `status=degraded` |
| `worker` | object | 实时值：`busy`=运行中 worker 数，`total`=`pool_size`，`queued`=RabbitMQ 队列深度（与 `rabbitmq.queue_depth` 同源） |

**未就绪响应（503）**：优雅退出期间 readiness=false 时返回，`status` 保持原值供诊断。

```json
{
  "code": 50301,
  "message": "service not ready",
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMNW"
}
```

**幂等性与副作用**：幂等（GET），无副作用，可高频轮询（受 per-IP 限流约束）。

**curl 示例**：

```bash
curl -sS "$BASE/health" | jq
```

---

### 3.2 GET /metrics — Prometheus 指标

**用途**：以 Prometheus text 格式暴露运行指标，供监控系统抓取。

**设计解释**：该端点**刻意不带 `/api/v1` 版本前缀**，因为 Prometheus 抓取目标是基础设施契约而非业务 API，路径稳定便于固化到监控配置。它免鉴权、不限流，以便 Prometheus 高频抓取不被业务限流误伤。指标由中间件埋点、事件总线（Redis Streams 发布路径）与各基础设施客户端（pgxpool / OpenSearch / RabbitMQ / Redis / etcd）埋点共同产出，与业务解耦。此外，`observability.otel_endpoint` 非空时启用 OpenTelemetry Traces（OTLP gRPC，覆盖 gin middleware、worker pipeline 与 pgx/opensearch/rabbitmq 调用 span）；endpoint 为空则完全禁用、零成本。Trace 不经本端点暴露。

| 方法 | 路径 | 鉴权 | 限流 |
|---|---|---|---|
| GET | `/metrics` | 免 | 不限 |

**请求**：无参数、无请求体。

**成功响应（200）**：`Content-Type: text/plain; version=0.0.4`。下表列出全部对外承诺的指标（v1 全部保留，`deepwiki_queue_length` 语义改为 RabbitMQ 队列深度；v2.0 按总纲 §4.8 新增 9 个基础设施指标）：

```text
# HELP deepwiki_http_requests_total Total HTTP requests.
# TYPE deepwiki_http_requests_total counter
deepwiki_http_requests_total{method="POST",path="/api/v1/ask",code="200"} 128
# HELP deepwiki_tasks_total Total tasks by type and state.
# TYPE deepwiki_tasks_total counter
deepwiki_tasks_total{type="ingest",state="completed"} 7
deepwiki_tasks_total{type="ingest",state="failed"} 1
# HELP deepwiki_worker_busy Busy workers.
# TYPE deepwiki_worker_busy gauge
deepwiki_worker_busy 1
# HELP deepwiki_queue_length RabbitMQ queue depth (deepwiki.task.jobs).
# TYPE deepwiki_queue_length gauge
deepwiki_queue_length 3
# HELP deepwiki_llm_tokens_total LLM tokens.
# TYPE deepwiki_llm_tokens_total counter
deepwiki_llm_tokens_total{provider="openai",model="gpt-4o-mini",kind="prompt"} 24012
# HELP deepwiki_rabbitmq_queue_depth RabbitMQ queue depth by queue.
# TYPE deepwiki_rabbitmq_queue_depth gauge
deepwiki_rabbitmq_queue_depth{queue="deepwiki.task.jobs"} 3
# HELP deepwiki_rabbitmq_publish_confirms_total RabbitMQ publisher confirm results.
# TYPE deepwiki_rabbitmq_publish_confirms_total counter
deepwiki_rabbitmq_publish_confirms_total{result="acked"} 412
deepwiki_rabbitmq_publish_confirms_total{result="nacked"} 0
# HELP deepwiki_redis_op_duration_seconds Redis operation latency.
# TYPE deepwiki_redis_op_duration_seconds histogram
deepwiki_redis_op_duration_seconds_bucket{op="ratelimit_lua",le="0.005"} 980
# HELP deepwiki_opensearch_op_duration_seconds OpenSearch operation latency.
# TYPE deepwiki_opensearch_op_duration_seconds histogram
deepwiki_opensearch_op_duration_seconds_bucket{op="search",le="0.05"} 231
# HELP deepwiki_etcd_op_duration_seconds etcd operation latency.
# TYPE deepwiki_etcd_op_duration_seconds histogram
deepwiki_etcd_op_duration_seconds_bucket{op="txn_put",le="0.05"} 12
# HELP deepwiki_pg_pool_conns Postgres pool connections by state.
# TYPE deepwiki_pg_pool_conns gauge
deepwiki_pg_pool_conns{state="idle"} 8
deepwiki_pg_pool_conns{state="in_use"} 2
# HELP deepwiki_vector_search_duration_seconds pgvector ANN search latency.
# TYPE deepwiki_vector_search_duration_seconds histogram
deepwiki_vector_search_duration_seconds_bucket{le="0.02"} 190
# HELP deepwiki_keyword_search_duration_seconds OpenSearch BM25 search latency.
# TYPE deepwiki_keyword_search_duration_seconds histogram
deepwiki_keyword_search_duration_seconds_bucket{le="0.05"} 190
# HELP deepwiki_ratelimit_degraded_total Rate limiter fell back to in-process bucket.
# TYPE deepwiki_ratelimit_degraded_total counter
deepwiki_ratelimit_degraded_total 0
```

**错误响应**：本端点设计上不产生业务错误；仅极端情况下返回 `50001`。

**幂等性与副作用**：幂等（GET），无副作用。

**curl 示例**：

```bash
curl -sS "$HOST/metrics"
```

---

### 3.3 POST /api/v1/ingest — 提交仓库摄取任务

**用途**：提交一个 GitHub 仓库 URL，异步完成「克隆 → 解析 → 切分 → 向量化 → 持久化」全流程。

**设计解释**：返回 **202 而非同步等待**，是因为摄取是一个可能耗时数十秒到数分钟的长任务（涉及网络 clone 与多次 embedding 调用），同步等待会撑爆 HTTP 超时并阻塞连接。任务先落 Postgres `tasks` 表（`state=pending`，事务内），事务提交后向 RabbitMQ `deepwiki.task.jobs` 投递**瘦消息**（body 仅 `{"task_id":"tsk_...","type":"ingest"}`，≤4KB，禁止携带任务状态/进度；`mandatory=true` + publisher confirm），由 Worker Pool 受限并发消费执行——任务投递与执行跨进程/跨节点，状态唯一来源始终是 Postgres。投递前 `QueueDeclarePassive` 探测队列深度，深度 ≥ `x-max-length`（= `worker.queue_size`，默认 100）返回 `42902` 背压；publisher confirm 失败则把任务置 `failed`（`50302 queue_unavailable`）并返回 503。提交前用轻量的 `git ls-remote` 取远端 HEAD commit 做幂等判断——commit 未变即拒绝，避免对同一仓库重复全量摄取浪费配额与算力。

| 方法 | 路径 | 鉴权 | 限流 |
|---|---|---|---|
| POST | `/api/v1/ingest` | API key | per-IP + ingest_per_hour |

**请求头**：`X-API-Key: <key>`、`Content-Type: application/json`。

**请求体**：

```json
{
  "repo_url": "https://github.com/gin-gonic/gin",
  "branch": "master",
  "auto_wiki": true,
  "options": {
    "chunk_size": 1000,
    "chunk_overlap": 150,
    "include_ext": [".go", ".md"],
    "exclude_dirs": ["testdata"]
  }
}
```

| 字段 | 类型 | 必填 | 默认值 | 校验规则 |
|---|---|---|---|---|
| `repo_url` | string | 是 | — | 合法 git URL（`https://`、`http://`、`git@host:org/repo.git`）；拒绝 `file://` 等本地协议；长度 ≤ 512 |
| `branch` | string | 否 | 远端默认分支 | 省略时经 `git ls-remote` 解析默认分支；长度 ≤ 128；禁止 `..`、空白与 `` ~^:?*[\ `` 等 git ref 非法字符 |
| `auto_wiki` | bool | 否 | `false` | 为 `true` 时 ingest 进入 completed 后自动提交一个 wiki 任务 |
| `options` | object | 否 | — | 覆盖本次任务的摄取参数，**不改全局配置** |
| `options.chunk_size` | int | 否 | `ingest.chunk_size` | 100~4000 |
| `options.chunk_overlap` | int | 否 | `ingest.chunk_overlap` | 0 ~ chunk_size/2 |
| `options.include_ext` | []string | 否 | `ingest.include_ext` | 元素以 `.` 开头 |
| `options.exclude_dirs` | []string | 否 | `ingest.exclude_dirs` | 目录名（非路径） |

**成功响应（202）**：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "task_id": "tsk_01J2X9K7QZ0ABCDEFGHJKMNP",
    "repo_id": "repo_01J2X9K7QZ0ABCDEFGHJKMNQ",
    "type": "ingest",
    "state": "pending",
    "queue_position": 1,
    "created_at": "2026-07-05T08:30:00Z"
  },
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMNR"
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `task_id` | string | 任务 ID，用于轮询/取消/SSE 过滤 |
| `repo_id` | string | 仓库 ID；首次提交时新建，幂等命中时见 `details.existing_repo_id` |
| `type` | string | 恒为 `ingest` |
| `state` | string | 入队成功即 `pending` |
| `queue_position` | int | 队列中的位置（≥1），按投递前队列深度 +1 估算；被 Worker 取出后归 0 |
| `created_at` | string | 创建时间（UTC RFC3339） |

**幂等命中响应（409）**：commit 未变时拒绝重复摄取。

```json
{
  "code": 40901,
  "message": "repo already exists with identical commit hash",
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMNS",
  "details": [
    { "field": "repo_url", "issue": "repo_already_exists", "existing_repo_id": "repo_01J2X9K7QZ0ABCDEFGHJKMNQ" }
  ]
}
```

> commit 已变（远端有新提交）时同样返回 `40901`，但 `details[].issue` 为 `use_refresh`，提示客户端改走 `POST /repos/{repo_id}/refresh`。`ls-remote` 失败（网络/私有仓库无凭据）时放行创建任务，由 clone 阶段报错。

**队列满响应（429）**：

```json
{
  "code": 42902,
  "message": "queue full",
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMNX"
}
```

**队列不可用响应（503）**：RabbitMQ 连接/发布确认失败。

```json
{
  "code": 50302,
  "message": "queue unavailable",
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMNX"
}
```

**幂等性与副作用**：非幂等（会创建任务与数据），但具备**业务幂等**——同一 `(repo_url, branch)` 且 commit 未变时返回 409 而非重复执行。副作用：创建 tasks 记录、落库 repos、占用队列与 Worker，成功后写入 chunks/向量/OpenSearch 索引。

**curl 示例**：

```bash
curl -sS -X POST "$BASE/ingest" \
  -H "X-API-Key: $KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "repo_url": "https://github.com/gin-gonic/gin",
    "branch": "master",
    "auto_wiki": true,
    "options": { "chunk_size": 1000, "chunk_overlap": 150, "include_ext": [".go", ".md"], "exclude_dirs": ["testdata"] }
  }' | jq
```

---

### 3.4 GET /api/v1/repos — 仓库列表

**用途**：分页列出已提交摄取的仓库及其状态与统计。

**设计解释**：列表采用统一分页结构（§2.5），固定按 `created_at DESC` 排序，让最新提交的仓库排在最前，符合「最近操作优先」的使用直觉。列表项只放摘要字段（不含 `latest_task`、`wiki_available` 等详情字段），控制响应体积；需要完整信息时取 `repo_id` 调详情接口。

| 方法 | 路径 | 鉴权 | 限流 |
|---|---|---|---|
| GET | `/api/v1/repos` | API key | per-IP |

**请求头**：`X-API-Key: <key>`。

**查询参数**：

| 参数 | 类型 | 必填 | 默认 | 说明 |
|---|---|---|---|---|
| `page` | int | 否 | 1 | 页码，≥1 |
| `page_size` | int | 否 | 20 | 每页条数，1~100 |

**成功响应（200）**：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [
      {
        "repo_id": "repo_01J2X9K7QZ0ABCDEFGHJKMNQ",
        "repo_url": "https://github.com/gin-gonic/gin",
        "branch": "master",
        "commit_hash": "9b8c3f4e2a1d6c5b7e8f9a0b1c2d3e4f5a6b7c8d",
        "state": "ready",
        "stats": { "files": 88, "chunks": 412, "tokens": 98000 },
        "created_at": "2026-07-05T08:30:00Z",
        "updated_at": "2026-07-05T08:33:10Z"
      }
    ],
    "pagination": { "page": 1, "page_size": 20, "total": 1, "total_pages": 1 }
  },
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMNY"
}
```

`items[]` 字段说明：

| 字段 | 类型 | 说明 |
|---|---|---|
| `repo_id` | string | 仓库 ID |
| `repo_url` | string | 仓库 URL |
| `branch` | string | 分支 |
| `commit_hash` | string | 当前落库的 commit hash |
| `state` | string | `ingesting` \| `ready` \| `error` |
| `stats` | object | `{files, chunks, tokens}` 累计统计 |
| `created_at` | string | 创建时间 |
| `updated_at` | string | 最近更新时间 |

**错误响应（400）**：分页参数越界。

```json
{
  "code": 40001,
  "message": "invalid_param: field page_size must be between 1 and 100",
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMNZ",
  "details": [
    { "field": "page_size", "issue": "out_of_range" }
  ]
}
```

**幂等性与副作用**：幂等（GET），无副作用。

**curl 示例**：

```bash
curl -sS "$BASE/repos?page=1&page_size=20" -H "X-API-Key: $KEY" | jq
```

---

### 3.5 GET /api/v1/repos/{repo_id} — 仓库详情

**用途**：获取单个仓库的完整信息，含最近一次任务摘要与 Wiki 可用性。

**设计解释**：详情接口在列表字段基础上补了 `latest_task`（最近一次任务摘要）、`wiki_available`、`chunk_count`，让前端一次请求就能渲染仓库详情页，而不必再额外调 tasks 列表与 wiki 接口。`latest_task` 给出最近一次任务的 `state`，便于展示「是否正在刷新/生成 wiki」。

| 方法 | 路径 | 鉴权 | 限流 |
|---|---|---|---|
| GET | `/api/v1/repos/{repo_id}` | API key | per-IP |

**路径参数**：

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `repo_id` | string | 是 | `repo_` 前缀 ULID，先过正则校验 |

**请求头**：`X-API-Key: <key>`。

**成功响应（200）**：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "repo_id": "repo_01J2X9K7QZ0ABCDEFGHJKMNQ",
    "repo_url": "https://github.com/gin-gonic/gin",
    "branch": "master",
    "commit_hash": "9b8c3f4e2a1d6c5b7e8f9a0b1c2d3e4f5a6b7c8d",
    "state": "ready",
    "stats": { "files": 88, "chunks": 412, "tokens": 98000 },
    "created_at": "2026-07-05T08:30:00Z",
    "updated_at": "2026-07-05T08:33:10Z",
    "latest_task": {
      "task_id": "tsk_01J2X9K7QZ0ABCDEFGHJKMNP",
      "type": "ingest",
      "state": "completed",
      "created_at": "2026-07-05T08:30:00Z"
    },
    "wiki_available": true,
    "chunk_count": 412
  },
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMP0"
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| 列表字段 | — | 同 §3.4 `items[]` |
| `latest_task` | object\|null | 最近一次任务摘要 `{task_id, type, state, created_at}`；无任务时为 `null` |
| `wiki_available` | bool | 是否已生成 Wiki |
| `chunk_count` | int | 该仓 chunk 总数 |

**错误响应（404）**：仓库不存在。

```json
{
  "code": 40402,
  "message": "repo not found",
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMP1"
}
```

**幂等性与副作用**：幂等（GET），无副作用。

**curl 示例**：

```bash
curl -sS "$BASE/repos/repo_01J2X9K7QZ0ABCDEFGHJKMNQ" -H "X-API-Key: $KEY" | jq
```

---

### 3.6 DELETE /api/v1/repos/{repo_id} — 删除仓库（级联）

**用途**：删除一个仓库及其全部派生数据（chunks、向量、wiki、OpenSearch 索引、本地目录）。

**设计解释**：删除是**级联**的，保证不留孤儿数据。采用「先 DB 事务、后删外部资源」的顺序：DB 事务成功即逻辑删除（chunks/wiki_pages 走 `ON DELETE CASCADE`，tasks 的 `repo_id` 置 NULL 以保留任务历史），事务提交后再删 OpenSearch 索引（每仓一索引，索引名为 `deepwiki-chunks-<repo_id 小写>`；OpenSearch 索引名必须小写，repo_id 含大写 ULID，统一 `strings.ToLower`）与本地仓库目录；外部资源删除失败只记 ERROR 并后台重试清理，**不回滚 DB**——因为外部索引/文件系统操作无法纳入 SQL 事务，这种「逻辑删除优先」是工程上的一致性取舍。

| 方法 | 路径 | 鉴权 | 限流 |
|---|---|---|---|
| DELETE | `/api/v1/repos/{repo_id}` | API key | per-IP |

**路径参数**：`repo_id`（`repo_` 前缀 ULID）。

**请求头**：`X-API-Key: <key>`。无请求体。

**成功响应（200）**：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "repo_id": "repo_01J2X9K7QZ0ABCDEFGHJKMNQ",
    "deleted": {
      "chunks": 412,
      "vectors": 412,
      "wiki_pages": 13,
      "opensearch_docs": 412,
      "local_dir": true
    }
  },
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMP2"
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `repo_id` | string | 被删除的仓库 ID |
| `deleted.chunks` | int | 删除的 chunk 数 |
| `deleted.vectors` | int | 删除的向量数（pgvector 列随 chunk 行级联消失） |
| `deleted.wiki_pages` | int | 删除的 wiki 行数（toc + pages） |
| `deleted.opensearch_docs` | int | 删除的 OpenSearch 文档数（对应索引 `deepwiki-chunks-<repo_id 小写>` 整体删除） |
| `deleted.local_dir` | bool | 本地目录是否删除成功 |

**错误响应（404）**：仓库不存在（`40402`，同 §3.5）。

**幂等性与副作用**：**级联删除、不可逆**，副作用大；对已不存在的 `repo_id` 返回 404。任务历史保留（`repo_id` 置 NULL），便于追溯。

**curl 示例**：

```bash
curl -sS -X DELETE "$BASE/repos/repo_01J2X9K7QZ0ABCDEFGHJKMNQ" -H "X-API-Key: $KEY" | jq
```

---

### 3.7 POST /api/v1/repos/{repo_id}/refresh — 增量更新任务

**用途**：对已摄取仓库执行增量更新，只重处理变更文件。

**设计解释**：refresh 用 `git -C <dir> fetch --depth 1 origin <branch>` → `git -C <dir> reset --hard FETCH_HEAD` → `git -C <dir> clean -fdx`（清理未跟踪文件）而非 `git pull`——`pull` 会引入 merge，工作区一旦脏或有冲突就会卡死 pipeline；`fetch + reset --hard` 则把本地强制对齐远端，确定性更强。全部 git 操作经 `exec.CommandContext` 调用系统 git CLI（禁止 `sh -c` 字符串拼接，`GIT_TERMINAL_PROMPT=0`，单次操作挂 `git.op_timeout` 超时），ctx 取消可即时中断传输；失败回退重新 clone + 原子切换。同一仓库的 refresh/ingest 互斥由 **Redis 分布式锁**（`SET lock:refresh:<repo_id> <token> NX PX 300000`，Lua 校验 token 后 DEL 解锁）保证——多 Worker 节点下进程内互斥无效，必须分布式互斥；锁 300s 自动过期 + 任务 CAS 兜底，不引入 watchdog。随后用文件 `sha256[:16]` 做 hash diff，只对 added/modified 文件重新切分与向量化，deleted/modified 的旧 chunks 在事务内删除，从而避免对未变更的大量文件重复算向量，显著节省 embedding 成本与耗时。本接口无请求体，仓库即上下文。

| 方法 | 路径 | 鉴权 | 限流 |
|---|---|---|---|
| POST | `/api/v1/repos/{repo_id}/refresh` | API key | per-IP + ingest_per_hour |

**路径参数**：`repo_id`（`repo_` 前缀 ULID）。

**请求头**：`X-API-Key: <key>`。**无请求体**。

**成功响应（202）**：data 结构与 ingest 一致，`type` 为 `refresh`。

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "task_id": "tsk_01J2X9K7QZ0ABCDEFGHJKMP3",
    "repo_id": "repo_01J2X9K7QZ0ABCDEFGHJKMNQ",
    "type": "refresh",
    "state": "pending",
    "queue_position": 1,
    "created_at": "2026-07-05T09:00:00Z"
  },
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMP4"
}
```

**错误响应（409）**：仓库非 `ready`（如仍在 `ingesting` 或处于 `error`），或该仓已有进行中任务冲突（分布式锁被持有）。

```json
{
  "code": 40902,
  "message": "invalid task state: repo is not ready for refresh",
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMP5"
}
```

**错误响应（404）**：仓库不存在（`40402`）。

**幂等性与副作用**：非幂等（每次调用新建任务）；副作用为创建 refresh 任务。无变更时任务仍走完整状态机（fetching→diffing→…→persisting→completed）但各阶段零工作量快速通过，`stats` 全 0。

**curl 示例**：

```bash
curl -sS -X POST "$BASE/repos/repo_01J2X9K7QZ0ABCDEFGHJKMNQ/refresh" -H "X-API-Key: $KEY" | jq
```

---

### 3.8 GET /api/v1/tasks — 任务列表

**用途**：统一查询 ingest / refresh / wiki 三类任务，支持按类型、状态、仓库过滤与分页。

**设计解释**：相比按资源拆分的 `/ingest/{id}`、`/wiki/task/{id}`，统一为 `/api/v1/tasks` 是「整个系统设计就一致了」的彻底化——三类任务共用同一套查询与取消端点，前端只需对接一套任务契约，用 `type` 字段区分种类。任务状态以 Postgres `tasks` 表为唯一来源（RabbitMQ 消息只携带 task_id；Worker 崩溃后由 Reconciler 从库中重建队列视图，重启可恢复），因此列表/详情反映的是落库状态（进度落库有 500ms 或每 5% 一次的节流，可能略滞后于 SSE 实时事件，属预期取舍）。

| 方法 | 路径 | 鉴权 | 限流 |
|---|---|---|---|
| GET | `/api/v1/tasks` | API key | per-IP |

**请求头**：`X-API-Key: <key>`。

**查询参数**：

| 参数 | 类型 | 必填 | 默认 | 说明 |
|---|---|---|---|---|
| `type` | string | 否 | 全部 | `ingest` \| `refresh` \| `wiki` |
| `state` | string | 否 | 全部 | 见附录状态机（如 `pending`/`embedding`/`completed` 等） |
| `repo_id` | string | 否 | 全部 | 只看该仓任务 |
| `page` | int | 否 | 1 | 页码，≥1 |
| `page_size` | int | 否 | 20 | 每页条数，1~100 |

**成功响应（200）**：`items[]` 为 Task 的 API 投影（不含 `cancel_flag`、`request_payload`）。

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [
      {
        "task_id": "tsk_01J2X9K7QZ0ABCDEFGHJKMNP",
        "type": "ingest",
        "repo_id": "repo_01J2X9K7QZ0ABCDEFGHJKMNQ",
        "state": "embedding",
        "progress": { "current": 12, "total": 40, "percent": 30 },
        "stats": { "files": 88, "chunks": 412, "tokens": 98000 },
        "error": null,
        "queue_position": 0,
        "created_at": "2026-07-05T08:30:00Z",
        "started_at": "2026-07-05T08:30:05Z",
        "finished_at": null
      }
    ],
    "pagination": { "page": 1, "page_size": 20, "total": 1, "total_pages": 1 }
  },
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMP6"
}
```

`items[]`（Task API 投影）字段说明：

| 字段 | 类型 | 说明 |
|---|---|---|
| `task_id` | string | 任务 ID |
| `type` | string | `ingest` \| `refresh` \| `wiki` |
| `repo_id` | string\|null | 仓库 ID；仓库被删除后历史任务此字段为 `null` |
| `state` | string | 唯一状态字段，见附录状态机 |
| `progress` | object | `{current, total, percent}`；`total=0` 表示该阶段总量未知 |
| `stats` | object | `{files, chunks, tokens}` 随阶段累计；无数据时全 0 |
| `error` | object\|null | 失败时 `{code, message, stage}`；否则 `null` |
| `queue_position` | int | 排队中时 ≥1；被 Worker 取出后归 0 |
| `created_at` | string | 创建时间 |
| `started_at` | string\|null | Worker 开始执行时间 |
| `finished_at` | string\|null | 进入终态时间 |

**错误响应（400）**：过滤参数非法（如 `type` 不在枚举内）。

```json
{
  "code": 40001,
  "message": "invalid_param: field type must be one of ingest|refresh|wiki",
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMP7",
  "details": [
    { "field": "type", "issue": "invalid_enum" }
  ]
}
```

**幂等性与副作用**：幂等（GET），无副作用。

**curl 示例**：

```bash
curl -sS "$BASE/tasks?type=ingest&state=embedding&page=1&page_size=20" -H "X-API-Key: $KEY" | jq
```

---

### 3.9 GET /api/v1/tasks/{task_id} — 任务详情

**用途**：获取单个任务的全字段投影（同列表项结构），用于精确跟踪某一任务。

**设计解释**：与列表共用同一份 Task 投影结构，前端无需为「列表」与「详情」维护两套模型。它是轮询跟踪任务的轻量入口；若需实时进度，更推荐订阅 SSE（§4.2）而非高频轮询本接口。

| 方法 | 路径 | 鉴权 | 限流 |
|---|---|---|---|
| GET | `/api/v1/tasks/{task_id}` | API key | per-IP |

**路径参数**：`task_id`（`tsk_` 前缀 ULID）。

**请求头**：`X-API-Key: <key>`。

**成功响应（200）**：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "task_id": "tsk_01J2X9K7QZ0ABCDEFGHJKMNP",
    "type": "ingest",
    "repo_id": "repo_01J2X9K7QZ0ABCDEFGHJKMNQ",
    "state": "completed",
    "progress": { "current": 40, "total": 40, "percent": 100 },
    "stats": { "files": 88, "chunks": 412, "tokens": 98000 },
    "error": null,
    "queue_position": 0,
    "created_at": "2026-07-05T08:30:00Z",
    "started_at": "2026-07-05T08:30:05Z",
    "finished_at": "2026-07-05T08:33:10Z"
  },
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMP8"
}
```

字段说明同 §3.8 `items[]`。失败任务的 `error` 示例：`{"code": 50202, "message": "embedding unavailable", "stage": "embedding"}`。

**错误响应（404）**：任务不存在。

```json
{
  "code": 40401,
  "message": "task not found",
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMP9"
}
```

**幂等性与副作用**：幂等（GET），无副作用。

**curl 示例**：

```bash
curl -sS "$BASE/tasks/tsk_01J2X9K7QZ0ABCDEFGHJKMNP" -H "X-API-Key: $KEY" | jq
```

---

### 3.10 DELETE /api/v1/tasks/{task_id} — 取消任务

**用途**：取消一个排队中或运行中的任务。

**设计解释**：取消返回 **202（受理）而非立即等待终态**，因为「取消生效」依赖 Pipeline 在下一个 `ctx.Done()` 检查点退出，这需要一点时间；返回的 body 是当前 task 快照，`state` 可能尚未变为 `cancelled`，前端应通过 SSE/WS 的 `task.state_changed` 事件拿到终态。取消的粒度是「任务级」：所有外部 I/O（git、embedding、LLM、DB）都传入任务 ctx——git CLI 经 `exec.CommandContext` 在 ctx 取消时终止子进程、中断传输；LLM/embedding 官方 SDK 调用均接受 ctx 并随取消中断流式响应；Postgres 操作经 pgx 的 ctx 取消中断，保证能及时停下来。对仍在 RabbitMQ 中排队、未被消费的任务，Worker 取出时发现 `cancel_flag`（或 CAS 抢占失败）直接落 `cancelled` 并 ack，不执行任何阶段。

| 方法 | 路径 | 鉴权 | 限流 |
|---|---|---|---|
| DELETE | `/api/v1/tasks/{task_id}` | API key | per-IP |

**路径参数**：`task_id`（`tsk_` 前缀 ULID）。

**请求头**：`X-API-Key: <key>`。无请求体。

**成功响应（202）**：返回当前 task 快照。

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "task_id": "tsk_01J2X9K7QZ0ABCDEFGHJKMPA",
    "type": "ingest",
    "repo_id": "repo_01J2X9K7QZ0ABCDEFGHJKMNQ",
    "state": "embedding",
    "progress": { "current": 12, "total": 40, "percent": 30 },
    "stats": { "files": 88, "chunks": 412, "tokens": 98000 },
    "error": null,
    "queue_position": 0,
    "created_at": "2026-07-05T08:30:00Z",
    "started_at": "2026-07-05T08:30:05Z",
    "finished_at": null
  },
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMPB"
}
```

**错误响应（409）**：对已进入终态（`completed`/`failed`/`cancelled`）的任务取消。

```json
{
  "code": 40902,
  "message": "invalid task state: cannot cancel a terminal task",
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMPC"
}
```

**错误响应（404）**：任务不存在（`40401`）。

**幂等性与副作用**：非幂等；副作用为置 `cancel_flag` 并触发取消，任务最终落 `cancelled`（写 `finished_at`）。对排队中任务，Worker 取出时发现 flag 直接落 `cancelled`，不执行任何阶段。

**curl 示例**：

```bash
curl -sS -X DELETE "$BASE/tasks/tsk_01J2X9K7QZ0ABCDEFGHJKMPA" -H "X-API-Key: $KEY" | jq
```

---

### 3.11 POST /api/v1/ask — 非流式问答

**用途**：对已 `ready` 的仓库进行语义问答，返回带引用的完整回答（一次性 JSON）。

**设计解释**：`references` 必须来自**真实检索结果**——响应中的引用就是 Retriever 返回的 hits，不是从 LLM 输出中解析出来的；每个 `chunk_id` 在响应装配前逐一校验存在，system prompt 也明确约束 LLM 只能依据给定片段作答、禁止编造行号与文件路径，片段不足时回答「未在仓库中找到相关代码」。`mode` 参数（keyword/embedding/hybrid）是检索策略的 AB 测试入口：`keyword` 走 **OpenSearch BM25**（`multi_match` 于 `content^2, path`，每仓一索引 `deepwiki-chunks-<repo_id 小写>` 物理隔离）；`embedding` 走 **pgvector**（`<=>` 余弦距离算子 + HNSW 索引，`ef_search` 可调，ANN 近似检索）；`hybrid` 把两路结果做 **RRF（Reciprocal Rank Fusion）融合**。`score` 语义与 v1 完全一致（keyword=BM25 分；embedding=余弦相似度 [0,1]；hybrid=RRF 融合分），只是产生分数的引擎升级。`top_k` 与 `temperature` 可按请求覆盖全局默认。

| 方法 | 路径 | 鉴权 | 限流 |
|---|---|---|---|
| POST | `/api/v1/ask` | API key | per-IP + ask_per_minute |

**请求头**：`X-API-Key: <key>`、`Content-Type: application/json`。

**请求体**：

```json
{
  "repo_id": "repo_01J2X9K7QZ0ABCDEFGHJKMNQ",
  "question": "这个项目的中间件链是怎么组织的？",
  "mode": "hybrid",
  "top_k": 8,
  "temperature": 0.2
}
```

| 字段 | 类型 | 必填 | 默认值 | 校验规则 |
|---|---|---|---|---|
| `repo_id` | string | 是 | — | `repo_` 前缀格式校验；仓库须 `state=ready`，否则 `40902` |
| `question` | string | 是 | — | 长度 1~4000 字符 |
| `mode` | enum | 否 | `hybrid` | `keyword` \| `embedding` \| `hybrid` |
| `top_k` | int | 否 | `retriever.top_k` | 1~30 |
| `temperature` | float | 否 | `llm.temperature` | 请求值钳制到 [0, 2] |

**成功响应（200）**：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "answer": "中间件链按 RequestID → Recovery → Auth → RateLimit → CORS 的顺序注册……[internal/router/router.go:42-67]",
    "references": [
      {
        "chunk_id": "chk_01J2X9K7QZ0ABCDEFGHJKMNT",
        "path": "internal/router/router.go",
        "start_line": 42,
        "end_line": 67,
        "language": "go",
        "score": 0.83,
        "snippet": "func NewRouter(...) *gin.Engine {\n    r.Use(middleware.RequestID()) ..."
      }
    ],
    "mode": "hybrid",
    "usage": { "prompt_tokens": 1823, "completion_tokens": 256 },
    "latency_ms": 2140
  },
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMPD"
}
```

`data` 字段说明：

| 字段 | 类型 | 说明 |
|---|---|---|
| `answer` | string | LLM 生成的回答，引用格式 `[path:start-end]` |
| `references` | array | 真实检索结果；每项见下表 |
| `mode` | string | 实际生效的检索模式（请求缺省时回显默认值） |
| `usage` | object | `{prompt_tokens, completion_tokens}`；provider 不返回时按估算填充 |
| `latency_ms` | int | 端到端耗时（毫秒） |

`references[]` 字段说明（**必须含 path/start_line/end_line/language/score/chunk_id**）：

| 字段 | 类型 | 说明 |
|---|---|---|
| `chunk_id` | string | chunk ID（`chk_` 前缀），装配前逐一校验存在 |
| `path` | string | 仓库内相对路径 |
| `start_line` | int | 起始行号 |
| `end_line` | int | 结束行号 |
| `language` | string | 语言标识（如 `go`、`markdown`） |
| `score` | float | keyword=BM25 分；embedding=余弦相似度 [0,1]；hybrid=RRF 融合分 |
| `snippet` | string | 片段内容节选 |

**错误响应（409）**：仓库非 `ready`（如仍在 `ingesting`）。

```json
{
  "code": 40902,
  "message": "invalid task state: repo is not ready for ask",
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMPE"
}
```

**其他典型错误**：`40001`（question 超长/为空、mode 非法、top_k 越界）、`40402`（repo 不存在）、`50201`（LLM 不可用）、`50202`（embedding 不可用）、`50203`（pgvector 向量检索暂不可用）、`50303`（OpenSearch 检索服务暂不可用）。

**幂等性与副作用**：幂等（读操作，不落库），但受 `ask_per_minute` 配额约束；每次调用消耗 LLM token。

**curl 示例**：

```bash
curl -sS -X POST "$BASE/ask" \
  -H "X-API-Key: $KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "repo_id": "repo_01J2X9K7QZ0ABCDEFGHJKMNQ",
    "question": "这个项目的中间件链是怎么组织的？",
    "mode": "hybrid",
    "top_k": 8,
    "temperature": 0.2
  }' | jq
```

---

### 3.12 POST /api/v1/ask/stream — 流式问答（SSE）

**用途**：与 `/ask` 相同的问答能力，但以 SSE 流式返回，先推引用再逐 token 推回答，降低首字节等待。

**设计解释**：请求体与 `/ask` 完全一致，仅响应形式不同——这样前端只需切换端点就能在「一次性 JSON」与「流式」间选择，复用同一套参数校验与 RAG 流程（OpenSearch BM25 / pgvector `<=>` / RRF 融合，同 §3.11）。流式把 `references` 放在最前（检索完成立即推送），让用户先看到「依据哪些代码作答」，再边看 token 边读，体验远好于等全部生成完。帧级协议详见 §4.1。

| 方法 | 路径 | 鉴权 | 限流 | 成功码 |
|---|---|---|---|---|
| POST | `/api/v1/ask/stream` | API key | per-IP + ask_per_minute | 200（`text/event-stream`） |

**请求头**：`X-API-Key: <key>`、`Content-Type: application/json`。

**请求体**：与 §3.11 `/ask` 完全相同（`repo_id` / `question` / `mode` / `top_k` / `temperature`）。

**响应头**：`Content-Type: text/event-stream`、`Cache-Control: no-cache`、`Connection: keep-alive`、`X-Accel-Buffering: no`、`X-Request-ID: req_...`。

**事件序列**：`references`（恰好 1 个）→ `token`（0~N 个）→ `done`（恰好 1 个）；`error` 可出现在任意位置并终止流。完整帧格式、逐行样例与断开行为见 §4.1。

**错误响应**：建连前的参数/鉴权/限流错误仍走标准 JSON 信封（`40001`/`40101`/`40902`/`40402`/`42901`/`50203`/`50303`）；建连后的流内错误以 `event: error` 帧下发（§4.1）。

**幂等性与副作用**：幂等（读操作）；受 `ask_per_minute` 配额约束；客户端断开即 ctx 取消、LLM 流中断。

**curl 示例**（`-N` 关闭缓冲以实时查看流）：

```bash
curl -N -X POST "$BASE/ask/stream" \
  -H "X-API-Key: $KEY" \
  -H "Content-Type: application/json" \
  -d '{"repo_id":"repo_01J2X9K7QZ0ABCDEFGHJKMNQ","question":"中间件链怎么组织？","mode":"hybrid"}'
```

---

### 3.13 POST /api/v1/wiki/generate — 生成 Wiki 任务

**用途**：为已 `ready` 的仓库异步生成 Wiki 文档（TOC + 页面）。

**设计解释**：Wiki 生成与 ingest 共用同一套任务系统（TaskManager 落 Postgres → RabbitMQ 瘦消息投递 → Worker Pool 消费），返回 **202 + task_id** 而非同步生成——生成涉及多次 LLM 调用、耗时不确定，异步化后客户端用任务端点跟踪即可。状态机为 `pending → outlining → generating → completed`（先出大纲再逐页生成）。若该仓已有 wiki，再次生成会**覆盖重建**（事务内先删旧行再插入），保证内容最新。

| 方法 | 路径 | 鉴权 | 限流 |
|---|---|---|---|
| POST | `/api/v1/wiki/generate` | API key | per-IP + ingest_per_hour |

**请求头**：`X-API-Key: <key>`、`Content-Type: application/json`。

**请求体**：

```json
{ "repo_id": "repo_01J2X9K7QZ0ABCDEFGHJKMNQ" }
```

| 字段 | 类型 | 必填 | 校验规则 |
|---|---|---|---|
| `repo_id` | string | 是 | `repo_` 前缀格式校验；仓库须 `state=ready`，否则 `40902` |

**成功响应（202）**：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "task_id": "tsk_01J2X9K7QZ0ABCDEFGHJKMPF",
    "repo_id": "repo_01J2X9K7QZ0ABCDEFGHJKMNQ",
    "type": "wiki",
    "state": "pending",
    "queue_position": 1,
    "created_at": "2026-07-05T09:10:00Z"
  },
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMPG"
}
```

**错误响应（409）**：仓库非 `ready`。

```json
{
  "code": 40902,
  "message": "invalid task state: repo is not ready for wiki generation",
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMPH"
}
```

**其他典型错误**：`40402`（repo 不存在）、`42902`（队列满）、`50302`（队列不可用）。

**幂等性与副作用**：非幂等（每次新建 wiki 任务）；副作用为占用队列/Worker，完成后覆盖写入 wiki_pages。

**curl 示例**：

```bash
curl -sS -X POST "$BASE/wiki/generate" \
  -H "X-API-Key: $KEY" \
  -H "Content-Type: application/json" \
  -d '{"repo_id":"repo_01J2X9K7QZ0ABCDEFGHJKMNQ"}' | jq
```

---

### 3.14 GET /api/v1/repos/{repo_id}/wiki — 获取已生成的 Wiki

**用途**：获取某仓库已生成的 Wiki（TOC + 全部页面 Markdown）。

**设计解释**：TOC（目录树）与 pages（页面内容）一次返回，前端可直接渲染左侧目录 + 右侧内容，无需多次请求。`slug` 是仓库内的人类可读标识（如 `overview`、`module-api`），非全局 ID，便于做锚点与路由。Wiki 未生成时返回 `40403`，引导前端先去触发生成。

| 方法 | 路径 | 鉴权 | 限流 |
|---|---|---|---|
| GET | `/api/v1/repos/{repo_id}/wiki` | API key | per-IP |

**路径参数**：`repo_id`（`repo_` 前缀 ULID）。

**请求头**：`X-API-Key: <key>`。

**成功响应（200）**：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "repo_id": "repo_01J2X9K7QZ0ABCDEFGHJKMNQ",
    "task_id": "tsk_01J2X9K7QZ0ABCDEFGHJKMPF",
    "generated_at": "2026-07-05T09:12:30Z",
    "toc": [
      { "slug": "overview", "title": "项目概览", "parent_slug": "", "sort_order": 0 },
      { "slug": "module-api", "title": "API 层", "parent_slug": "", "sort_order": 1 }
    ],
    "pages": [
      {
        "slug": "overview",
        "title": "项目概览",
        "content_md": "# 项目概览\n\n本仓库是一个高性能 HTTP 框架……",
        "sort_order": 0,
        "updated_at": "2026-07-05T09:12:30Z"
      },
      {
        "slug": "module-api",
        "title": "API 层",
        "content_md": "# API 层\n\n负责参数绑定与响应装配……",
        "sort_order": 1,
        "updated_at": "2026-07-05T09:12:30Z"
      }
    ]
  },
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMPI"
}
```

`data` 字段说明：

| 字段 | 类型 | 说明 |
|---|---|---|
| `repo_id` | string | 仓库 ID |
| `task_id` | string | 生成本次 wiki 的任务 ID |
| `generated_at` | string | 生成时间 |
| `toc` | array | 目录项 `[{slug, title, parent_slug, sort_order}]` |
| `pages` | array | 页面 `[{slug, title, content_md, sort_order, updated_at}]` |

**错误响应（404）**：Wiki 未生成（`40403`）。

```json
{
  "code": 40403,
  "message": "wiki not found",
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMPJ"
}
```

**幂等性与副作用**：幂等（GET），无副作用。

**curl 示例**：

```bash
curl -sS "$BASE/repos/repo_01J2X9K7QZ0ABCDEFGHJKMNQ/wiki" -H "X-API-Key: $KEY" | jq
```

---

### 3.15 GET /api/v1/config — 获取当前生效配置

**用途**：返回当前生效配置（yaml 基线 + etcd `/deepwiki/config/*` 覆写合并结果），密钥字段脱敏。

**设计解释**：返回的是**多层合并后的生效值**而非 yaml 原文，让运维看到「此刻系统真正在用什么」。加载顺序：viper 读 yaml+env（仅引导基础设施坐标）→ 连接 etcd → 全量读 `/deepwiki/config/` 前缀覆盖 → `Watch(prefix)` 增量热更新 → 重建「生效配置」快照（atomic.Value）。本接口**只读本地快照缓存**，毫秒级返回、不压 etcd；etcd 短暂不可用时读路径完全不受影响（仅 PUT 报错，见 §3.16）。`api_key` 一律脱敏返回（长度 > 8 取前 3 + `***` + 后 4，否则全 `******`）；环境变量注入项（api_keys/admin_key、postgres dsn、rabbitmq url、redis password、opensearch 用户名密码）不出现在响应中，杜绝密钥经 API 泄漏。`restart_required` 列出「已写入但需重启才生效」的 key，避免误以为改了就立即生效。

| 方法 | 路径 | 鉴权 | 限流 |
|---|---|---|---|
| GET | `/api/v1/config` | API key | per-IP |

**请求头**：`X-API-Key: <key>`。

**成功响应（200）**：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "version": 7,
    "config": {
      "server": { "addr": ":8080", "read_timeout": "30s", "shutdown_timeout": "30s", "cors_allowed_origins": ["http://localhost:5173"] },
      "rate_limit": { "per_ip": { "rps": 10, "burst": 20 }, "per_key": { "ingest_per_hour": 20, "ask_per_minute": 30 } },
      "worker": { "pool_size": 2, "queue_size": 100 },
      "ingest": { "workdir": "./data/repos", "max_repo_size_mb": 500, "chunk_size": 800, "chunk_overlap": 120, "include_ext": [".go", ".md"], "exclude_dirs": [".git", "node_modules", "vendor", "dist", "build", "target"] },
      "embedding": { "provider": "openai", "model": "text-embedding-3-small", "api_key": "sk-***abcd", "base_url": "", "batch_size": 64, "timeout": "60s", "retry": { "max": 3, "backoff": "2s" } },
      "llm": { "provider": "openai", "model": "gpt-4o-mini", "api_key": "sk-***wxyz", "base_url": "", "temperature": 0.2, "max_tokens": 2048, "timeout": "120s", "retry": { "max": 2, "backoff": "1s" } },
      "retriever": { "mode": "hybrid", "top_k": 8, "rrf_k": 60 },
      "storage": { "postgres": { "max_conns": 10 }, "vector": { "dimensions": 1536, "ef_search": 64 } },
      "search": { "opensearch": { "addresses": ["http://localhost:9200"], "index_prefix": "deepwiki" } },
      "queue": { "rabbitmq": { "prefetch": 2, "max_retries": 3 } },
      "redis": { "sentinel": { "addresses": ["localhost:26379"], "master_name": "deepwiki-master" } },
      "etcd": { "endpoints": ["localhost:2379"], "prefix": "/deepwiki" },
      "git": { "op_timeout": "10m", "binary_path": "git" },
      "observability": { "otel_endpoint": "", "log": { "level": "info", "format": "json" } }
    },
    "restart_required": ["server.addr"]
  },
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMPK"
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `version` | int | 当前配置版本号（PUT 成功后 +1；对应 etcd `/deepwiki/config_version`，单调递增） |
| `config` | object | 生效配置全量（脱敏后；基础设施坐标中涉密钥字段不返回） |
| `restart_required` | array | 已写入但需重启才生效的点分 key 列表（仅 `server.addr` 可经 PUT 落入此类；基础设施坐标类 key 的 PUT 直接拒绝 `40001`，见 §3.16） |

**错误响应**：仅通用鉴权/限流错误（`40101`/`42901`）。

**幂等性与副作用**：幂等（GET），无副作用。

**curl 示例**：

```bash
curl -sS "$BASE/config" -H "X-API-Key: $KEY" | jq
```

---

### 3.16 PUT /api/v1/config — 部分更新配置

**用途**：以 JSON Merge Patch 语义部分更新运行时配置，校验通过后热生效。

**设计解释**：采用「Merge Patch → 全量校验 → 失败整体拒绝保持旧值 → 成功经 etcd `Txn` 原子写入 → watch 广播热生效」的强一致流程，杜绝「改一半生效一半失败」把运行中配置搞坏。同一 etcd 事务内完成三件事：put `/deepwiki/config/<dotted.key>` 覆写值（JSON）、`/deepwiki/config_version` +1、写 `/deepwiki/audit/<version>` 审计记录 `{changed_by, change, result, reject_reason, at}`；随后各节点的 `Watch(prefix)` 回调在毫秒级把变更推到本节点与全部其他节点，重建生效配置快照并通知订阅者（限流参数、chunk 参数、retriever 参数、LLM/embedding 运行时可选项）。每次 PUT 无论成败都留审计（result=`applied`/`rejected`），满足审计追溯。需 **admin key**——把改配置的权限从普通调用方中分离，防止误改 `pool_size`/`temperature` 等影响全局。

| 方法 | 路径 | 鉴权 | 限流 |
|---|---|---|---|
| PUT | `/api/v1/config` | **admin key**（`X-API-Key` 值须等于 `auth.admin_key`） | per-IP |

**请求头**：`X-API-Key: <admin_key>`、`Content-Type: application/json`。

**请求体**（JSON Merge Patch，只传要改的 key）：

```json
{
  "retriever": { "top_k": 12 },
  "llm": { "temperature": 0.5 }
}
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| 任意配置 key | — | 否 | 以点分嵌套对象表达，仅传入需变更的部分；合法 key 与取值域见附录配置清单 |

**成功响应（200）**：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "version": 8,
    "applied": { "retriever.top_k": 12, "llm.temperature": 0.5 },
    "restart_required": [],
    "warnings": []
  },
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMPL"
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `version` | int | 更新后的配置版本号（etcd `/deepwiki/config_version`） |
| `applied` | object | 本次实际生效的点分 key → 值 |
| `restart_required` | array | 本次写入但需重启才生效的 key（如 `server.addr`） |
| `warnings` | array | 非阻塞提示，如 `["embedding provider changed, existing index may need rebuild"]` |

**关键规则**：

- **restart_required 项**：`server.addr` 允许写入但不当场生效，响应 `restart_required` 列出，重启后生效。
- **基础设施坐标类 key 不可写**：`storage.postgres.dsn`、`storage.vector.dimensions`、`search.opensearch.*`、`queue.rabbitmq.url`、`queue.rabbitmq.prefetch`、`redis.*`、`etcd.endpoints`、`etcd.prefix`、`git.binary_path`、`observability.otel_endpoint` 只在 yaml/env 引导层，PUT 直接拒绝（`40001`），修改需改 yaml/env 后重启。其中 `storage.vector.dimensions` 建表即定型（pgvector `vector(1536)` 列），改维度 = 新迁移 + 全量重建索引，不走本接口。
- **密钥禁入 etcd**：`api_key`/密码类字段仍只走环境变量，请求体携带此类 key 一律拒绝（`40001`），禁止密钥写入 etcd。
- **embedding 变更**：provider/model/base_url 变更且库中已存在 chunks 时，系统发起一次探测性 `Embed(["dimension probe"])` 取维度并与已存 chunks 比对；**维度不一致或探测失败 → 拒绝（`42201`）并提示重建索引**；库中无数据 → 放行并返回 warning。DB 层 `vector(1536)` 列类型为第二道防线，直接拒绝维度不符的写入。
- **审计**：每次 PUT 写 `/deepwiki/audit/<version>`（result=`applied`/`rejected`，含操作者脱敏 `changed_by`、change、reject_reason、时间）。
- **etcd 不可用**：写路径返回 `50304 config_store_unavailable`（GET /config 走本地快照缓存不受影响）；health 置 `degraded`。

**校验失败响应（422）**：整体拒绝，保持旧值，审计记 `rejected`。

```json
{
  "code": 42201,
  "message": "config validation failed: chunk_overlap must be <= chunk_size/2",
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMPM",
  "details": [
    { "field": "ingest.chunk_overlap", "issue": "cross_field_constraint" }
  ]
}
```

**错误响应（403）**：用普通 key 调用本端点。

```json
{
  "code": 40301,
  "message": "forbidden: admin key required",
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMPN"
}
```

**错误响应（503）**：etcd 写路径不可用。

```json
{
  "code": 50304,
  "message": "config store unavailable",
  "request_id": "req_01J2X9K7QZ0ABCDEFGHJKMPN"
}
```

**幂等性与副作用**：非幂等（每次成功都会 `version+1` 并写审计）；副作用为热更新相关组件（限流器重建滑动窗口参数、WorkerPool 软扩缩容、Retriever/Provider 重建等），watch 回调保证全部节点一致生效。

**curl 示例**：

```bash
curl -sS -X PUT "$BASE/config" \
  -H "X-API-Key: $ADMIN" \
  -H "Content-Type: application/json" \
  -d '{"retriever":{"top_k":12},"llm":{"temperature":0.5}}' | jq
```

---

### 3.17 GET /api/v1/events — 全局事件流（SSE）

**用途**：订阅事件总线扇出的全局事件流（任务状态/进度、Wiki 完成），支持断线补发与过滤。

**设计解释**：这是「WS/SSE 不得直接订阅 Task，必须经事件总线扇出」的落地——Handler 只订阅事件总线，推送结构化字段、不拼字符串。事件总线基于 **Redis Streams + Pub/Sub**：Worker 发布事件 = `XADD events:task:<task_id> * seq <n> type <t> data <json>`（`seq` 为全局单调递增序号，跨连接有效）+ `XTRIM MAXLEN ~ 1000`（每任务事件日志保留最近约 1000 条）+ `PUBLISH events:fanout <task_id>`；各 API 节点 `SUBSCRIBE events:fanout`，命中本节点持有连接的 task 时 `XRANGE` 取增量推送给 SSE 客户端——多节点部署下，事件由 Worker 节点产生、经 Redis Pub/Sub 跨节点扇出到所有 API 节点，任意节点上的连接都能收到（v1 原方案的进程内事件总线无法跨进程广播，这是替换的核心动因）。它支持 `Last-Event-ID` 断线重连补发（经 `XRANGE events:task:<task_id> <last> +` 回放），让前端短暂断网后不错过状态；`?types=` 与 `?repo_id=` 过滤让前端只收关心的事件，降低噪音。完整协议、事件类型与重连语义见 §4.2。

| 方法 | 路径 | 鉴权 | 限流 | 成功码 |
|---|---|---|---|---|
| GET | `/api/v1/events` | API key | per-IP | 200（`text/event-stream`） |

**请求头**：`X-API-Key: <key>`；可选 `Last-Event-ID: <seq>`。

**查询参数**：

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `types` | string | 否 | 逗号分隔事件类型白名单；缺省=全部 |
| `repo_id` | string | 否 | 只推该仓库相关事件；缺省=全部 |

**事件类型**：`task.state_changed` / `task.progress` / `wiki.completed`（data 结构见 §4.2）。

**错误响应**：建连前错误走标准信封（`40101`/`42901` 等）。

**curl 示例**：

```bash
curl -N "$BASE/events?types=task.state_changed,wiki.completed&repo_id=repo_01J2X9K7QZ0ABCDEFGHJKMNQ" \
  -H "X-API-Key: $KEY"
```

---

### 3.18 GET /api/v1/ws — WebSocket 事件流

**用途**：以 WebSocket 提供与 SSE 同源（事件总线）的事件流，作为 SSE 的备选通道。

**设计解释**：WS 与 SSE 共享同一事件总线与过滤器语义（事件经 Redis Streams 持久化、Pub/Sub 跨节点扇出），但不受浏览器「每域约 6 个 HTTP/1.1 长连接」限制，适合同时需要多路实时通道的场景。升级为 101 后推送 JSON 帧，服务端每 15s 发 WS ping 帧保活。重连时可携带 `resume_from=<seq>` 查询参数，服务端经 Redis Streams 回放 `seq > resume_from` 且匹配过滤器的事件；seq 过旧（对应事件已被 `XTRIM` 修剪）时先推一帧 `gap`，客户端须回退 `GET /api/v1/tasks` 全量同步以重建状态。完整握手与消息格式见 §4.3。

| 方法 | 路径 | 鉴权 | 限流 | 成功码 |
|---|---|---|---|---|
| GET | `/api/v1/ws` | API key | per-IP | 101（协议升级） |

**请求头**：`X-API-Key: <key>` 及标准 WebSocket 升级头（`Upgrade: websocket`、`Connection: Upgrade`、`Sec-WebSocket-Key`、`Sec-WebSocket-Version: 13`）。

**查询参数**：同 `/events`（`types` / `repo_id`），另加可选 `resume_from`（断线重连时携带最后收到的 `seq`）。

**消息格式**：升级后推送 `{"seq":12,"type":"task.state_changed","data":{...}}`；详见 §4.3。

**curl 示例**（curl 不便直接演示 WS，此处示意握手请求；实际建议用 `websocat`/浏览器）：

```bash
curl -i -N "$BASE/ws?repo_id=repo_01J2X9K7QZ0ABCDEFGHJKMNQ" \
  -H "X-API-Key: $KEY" \
  -H "Upgrade: websocket" -H "Connection: Upgrade" \
  -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" -H "Sec-WebSocket-Version: 13"
```

---

## 4. 流式协议专章

本章详述三个长连接端点的帧级协议。三者数据都源于事件总线（Redis Streams 持久化 + Pub/Sub 跨节点扇出）/ RAG 流程，但语义不同：`/ask/stream` 是**一次性问答流**（无重连补发），`/events` 是**可经 `Last-Event-ID` 回放的全局事件流**，`/ws` 是**可经 `resume_from` 回放的 WebSocket 事件流**。事件名、payload 结构、帧格式与示例为冻结项，与 v1 逐字符一致。

### 4.1 POST /api/v1/ask/stream — SSE 问答流

#### 4.1.1 连接建立

建连前的参数校验、鉴权、限流与 `/ask` 一致，失败走标准 JSON 信封。校验通过后返回 `200`，响应头：

```http
HTTP/1.1 200 OK
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
X-Request-ID: req_01J2X9K7QZ0ABCDEFGHJKMPO
```

> `X-Accel-Buffering: no` 用于提示 Nginx 等反向代理不要缓冲该响应，保证 token 实时下推。

#### 4.1.2 SSE 事件序列

事件 `id` 为**连接内单调递增序号**（仅本连接内有效，不可跨连接续传）。帧序列：

```text
: heartbeat                                          ← 每 15s 一行 comment 心跳

event: references                                    ← 检索完成后立即推送（恰好 1 个）
id: 1
data: {"request_id":"req_01J2X9K7QZ0ABCDEFGHJKMPO","mode":"hybrid","references":[{"chunk_id":"chk_01J2X9K7QZ0ABCDEFGHJKMNT","path":"internal/router/router.go","start_line":42,"end_line":67,"language":"go","score":0.83,"snippet":"func NewRouter(...) *gin.Engine {...}"}]}

event: token                                         ← 增量文本（0~N 个）
id: 2
data: {"delta":"中间件链按"}

event: token
id: 3
data: {"delta":" RequestID → Recovery"}

event: done                                          ← 收尾：usage + latency（恰好 1 个）
id: 4
data: {"usage":{"prompt_tokens":1823,"completion_tokens":256},"latency_ms":2140}
```

#### 4.1.3 事件类型

| event | 次数 | data 结构 | 说明 |
|---|---|---|---|
| `references` | 恰好 1 | `{request_id, mode, references[]}` | 检索完成后最先推送；`references[]` 字段同 §3.11 |
| `token` | 0~N | `{delta}` | LLM 增量文本；客户端按序拼接 `delta` 还原完整回答 |
| `done` | 恰好 1 | `{usage, latency_ms}` | 流正常收尾；`usage` 兜底规则见下 |
| `error` | 0~1 | `{code, message, request_id}` | 任意阶段失败时下发并终止流 |

#### 4.1.4 出错时

任意阶段失败（如 LLM 流内错误、embedding 失败、向量/关键词检索不可用），此前已推送的事件**不回滚**，直接下发 `error` 帧后关闭连接：

```text
event: error
data: {"code":50201,"message":"llm unavailable","request_id":"req_01J2X9K7QZ0ABCDEFGHJKMPO"}
```

#### 4.1.5 协议规则

| 规则 | 说明 |
|---|---|
| 事件顺序 | `references`（1 个）→ `token`（0~N）→ `done`（1 个）；`error` 可出现在任意位置并终止流 |
| 心跳 | 每 15s 输出一行 `: heartbeat`（SSE comment），防代理/浏览器断连；客户端应忽略 comment 行 |
| 断开处理 | 客户端断开 → ctx 取消 → LLM 流中断、goroutine 退出；**不补偿、不重放**（问答流无 Last-Event-ID 语义） |
| usage 兜底 | provider 流式接口不返回 usage 时，`done` 中按估算填充并记日志 |

#### 4.1.6 完整 curl 示例与逐行输出

```bash
curl -N -X POST "$BASE/ask/stream" \
  -H "X-API-Key: $KEY" \
  -H "Content-Type: application/json" \
  -d '{"repo_id":"repo_01J2X9K7QZ0ABCDEFGHJKMNQ","question":"中间件链怎么组织？","mode":"hybrid"}'
```

终端逐行输出样例（`>` 为 curl 打印，实际为流式到达）：

```text
event: references
id: 1
data: {"request_id":"req_...","mode":"hybrid","references":[{"chunk_id":"chk_...","path":"internal/router/router.go","start_line":42,"end_line":67,"language":"go","score":0.83,"snippet":"..."}]}

event: token
id: 2
data: {"delta":"中间件链按"}

event: token
id: 3
data: {"delta":" RequestID → Recovery → Auth"}

event: token
id: 4
data: {"delta":" → RateLimit → CORS 注册"}

event: done
id: 5
data: {"usage":{"prompt_tokens":1823,"completion_tokens":256},"latency_ms":2140}
```

---

### 4.2 GET /api/v1/events — 全局事件流（SSE）

#### 4.2.1 连接与过滤

```http
GET /api/v1/events?types=task.state_changed,wiki.completed&repo_id=repo_01J2X9K7QZ0ABCDEFGHJKMNQ
X-API-Key: <key>
Last-Event-ID: 128          ← 可选，断线重连时携带最后收到的 id
```

| Query / 头 | 说明 |
|---|---|
| `types` | 逗号分隔事件类型白名单；缺省=全部 |
| `repo_id` | 只推该仓库相关事件；缺省=全部 |
| `Last-Event-ID` | 断线重连时携带最后收到的 `id`；服务端从 Redis Streams（每任务事件日志 `events:task:<task_id>`，`XTRIM MAXLEN ~ 1000` 保留最近约 1000 条）补发 `seq > Last-Event-ID` 且匹配过滤器的事件 |

#### 4.2.2 帧格式

`id: <seq>` + `event: <type>` + `data: <json>`；`seq` 为全局单调递增序号（跨连接有效，是 `Last-Event-ID` 的依据，由事件总线发布时统一分配）。每 15s 一行 `: heartbeat` 心跳。

```text
id: 129
event: task.state_changed
data: {"task_id":"tsk_01J2X9K7QZ0ABCDEFGHJKMNP","type":"ingest","repo_id":"repo_01J2X9K7QZ0ABCDEFGHJKMNQ","state":"embedding","previous_state":"chunking","queue_position":0,"error":null,"timestamp":"2026-07-05T08:30:10Z"}

id: 130
event: task.progress
data: {"task_id":"tsk_01J2X9K7QZ0ABCDEFGHJKMNP","type":"ingest","repo_id":"repo_01J2X9K7QZ0ABCDEFGHJKMNQ","stage":"embedding","progress":{"current":12,"total":40,"percent":30},"stats":{"files":88,"chunks":412,"tokens":98000},"timestamp":"2026-07-05T08:30:11Z"}
```

#### 4.2.3 事件类型与 data 结构

| event | data payload | 触发时机 |
|---|---|---|
| `task.state_changed` | `{"task_id":"tsk_...","type":"ingest","repo_id":"repo_...","state":"embedding","previous_state":"chunking","queue_position":0,"error":null,"timestamp":"..."}` | 任务每次状态机转移 |
| `task.progress` | `{"task_id":"tsk_...","type":"ingest","repo_id":"repo_...","stage":"embedding","progress":{"current":12,"total":40,"percent":30},"stats":{"files":88,"chunks":412,"tokens":98000},"timestamp":"..."}` | 阶段内进度推进（实时，未节流） |
| `wiki.completed` | `{"repo_id":"repo_...","task_id":"tsk_...","pages":12,"timestamp":"..."}` | wiki 任务完成、页面落库后 |

#### 4.2.4 断线重连与 gap 事件

- 重连时携带 `Last-Event-ID: <seq>`，服务端从 Redis Streams 回放（`XRANGE events:task:<task_id> <last> +`）补发 `seq > Last-Event-ID` 且匹配过滤器的事件。
- 若 `Last-Event-ID` 过旧（对应事件已被 `XTRIM` 修剪）或 Redis 数据丢失导致无法补发，服务端推送一条 `event: gap`，提示客户端**回退 `GET /api/v1/tasks` 全量同步**以重建状态：

```text
event: gap
data: {"reason":"last_event_id too old or buffer reset","hint":"resync via GET /api/v1/tasks"}
```

> **注意**：SSE 断线补发依赖 Redis Streams 中保留的事件日志（每任务约 1000 条，跨服务重启仍可回放，不再随 API 进程重启而丢失）；但 seq 过旧被修剪、或 Redis 自身数据不可用时仍无法补发。客户端收到 `gap` 后必须回退全量同步；WebSocket 通道（§4.3）以 `resume_from` 享有同样的回放与 gap 语义。

#### 4.2.5 前端 EventSource 代码片段（10 行内）

```javascript
const es = new EventSource(`${BASE}/events?types=task.state_changed,task.progress&repo_id=${repoId}`);
// 浏览器原生 EventSource 无法自定义请求头；需带 X-API-Key 时用 fetch EventSource 实现或改走 WS。
es.addEventListener('task.state_changed', e => {
  const d = JSON.parse(e.data);
  console.log('状态变更:', d.state, '任务:', d.task_id);
});
es.addEventListener('task.progress', e => updateProgressBar(JSON.parse(e.data)));
es.addEventListener('gap', () => resyncViaRest());       // 收到 gap 回退全量同步
es.onerror = () => console.warn('SSE 断开，浏览器将自动重连（自带 Last-Event-ID）');
```

> 浏览器原生 `EventSource` 不支持自定义请求头，无法直接带 `X-API-Key`。可选方案：① 使用支持自定义头的 fetch-based EventSource 库；② 改走 WebSocket（§4.3）在连接后鉴权；③ 开发模式下无需鉴权。

---

### 4.3 GET /api/v1/ws — WebSocket 事件流

#### 4.3.1 升级握手

客户端发起标准 WebSocket 升级请求（query 同 `/events`：`types` / `repo_id`，另加可选 `resume_from`）：

```http
GET /api/v1/ws?repo_id=repo_01J2X9K7QZ0ABCDEFGHJKMNQ&resume_from=128 HTTP/1.1
Host: localhost:8080
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==
Sec-WebSocket-Version: 13
X-API-Key: <key>
```

服务端校验通过返回 `101 Switching Protocols` 完成升级；鉴权失败返回 `40101`，限流返回 `42901`。携带 `resume_from` 时，服务端先回放 `seq > resume_from` 的存量事件，再转入实时推送。

#### 4.3.2 消息格式

升级后，服务端将与 SSE **同源**（事件总线、同过滤器语义）的事件以 JSON 帧推送，每帧一条：

```json
{"seq":129,"type":"task.state_changed","data":{"task_id":"tsk_...","type":"ingest","repo_id":"repo_...","state":"embedding","previous_state":"chunking","queue_position":0,"error":null,"timestamp":"2026-07-05T08:30:10Z"}}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `seq` | int | 全局单调递增序号（与 SSE 的 `id` 同源） |
| `type` | string | `task.state_changed` \| `task.progress` \| `wiki.completed` \| `gap` |
| `data` | object | 对应事件的 payload（结构同 §4.2.3；`gap` 见 §4.3.3） |

#### 4.3.3 心跳、回放与断开

- **心跳**：服务端每 15s 发送一个 **WebSocket ping 帧**；客户端应回复 pong（多数 WS 库自动处理）。长时间收不到 ping/pong 说明连接已死，应重连。
- **回放**：重连时携带 `resume_from=<seq>`（最后收到的 `seq`），服务端经 Redis Streams（`XRANGE events:task:<task_id> <last> +`）回放 `seq > resume_from` 且匹配过滤器的事件。
- **gap**：若 `resume_from` 过旧（对应事件已被 `XTRIM` 修剪）或 Redis 数据丢失导致无法回放，服务端先推一帧 gap，客户端须**回退 `GET /api/v1/tasks` 全量同步**以重建最新状态，再继续订阅增量事件：

```json
{"seq":0,"type":"gap","data":{"reason":"resume_from too old or stream trimmed","hint":"resync via GET /api/v1/tasks"}}
```

---

## 5. 完整调用流程示例

下面是一个端到端的故事化场景：**「把 gin 仓库接进来，跟踪摄取，做问答，生成 Wiki，调参，顺手取消一个排队任务，最后做一次增量刷新」**。全程用 curl 演示，关键响应只节选要点。

> 前置：
> 1. 仓库根目录执行 `docker compose up -d` 拉起基础设施（postgres / opensearch / rabbitmq / redis 1 主 2 从 3 哨兵 / etcd），待全部服务 healthy 后再启动 DeepWiki 服务（§6.4）；
> 2. `export HOST="http://localhost:8080"; export BASE="$HOST/api/v1"; export KEY=...; export ADMIN=...`（§1.5）。

### 步骤 1 · 健康检查

```bash
curl -sS "$BASE/health" | jq '.data | {status, version, worker, postgres, rabbitmq, redis, etcd, git}'
```
```json
{
  "status": "ok",
  "version": "0.2.0",
  "worker": { "busy": 0, "total": 2, "queued": 0 },
  "postgres": { "connected": true, "pool": { "total": 10, "idle": 8 }, "migration_version": 1 },
  "rabbitmq": { "connected": true, "queue_depth": 0, "consumers": 2 },
  "redis": { "connected": true, "mode": "sentinel", "master": "redis-master:6379", "ratelimit_degraded": false },
  "etcd": { "connected": true, "endpoints": ["localhost:2379"] },
  "git": { "available": true, "version": "2.43.0" }
}
```
`status=ok` 且基础设施全部 connected、git 可用、队列为空，可以开始。

### 步骤 2 · 提交 ingest（202）

```bash
curl -sS -X POST "$BASE/ingest" -H "X-API-Key: $KEY" -H "Content-Type: application/json" \
  -d '{"repo_url":"https://github.com/gin-gonic/gin","branch":"master","auto_wiki":true}' | jq .data
```
```json
{
  "task_id": "tsk_01J2X9K7QZ0ABCDEFGHJKMNP",
  "repo_id": "repo_01J2X9K7QZ0ABCDEFGHJKMNQ",
  "type": "ingest",
  "state": "pending",
  "queue_position": 1,
  "created_at": "2026-07-05T08:30:00Z"
}
```
记下 `task_id` 与 `repo_id`。`auto_wiki=true` 表示摄取完成后自动提交一个 wiki 任务。任务已落 Postgres 并向 RabbitMQ 投递瘦消息，等待 Worker 消费。

### 步骤 3 · 跟踪任务到 completed（轮询 / SSE 二选一）

**方式 A：轮询**

```bash
curl -sS "$BASE/tasks/tsk_01J2X9K7QZ0ABCDEFGHJKMNP" -H "X-API-Key: $KEY" | jq '.data | {state, progress, stats}'
```
```json
{ "state": "embedding", "progress": { "current": 12, "total": 40, "percent": 30 }, "stats": { "files": 88, "chunks": 412, "tokens": 98000 } }
```
隔几秒再查，直到 `state=completed`。

**方式 B：订阅 SSE（推荐，实时）**

```bash
curl -N "$BASE/events?types=task.state_changed&repo_id=repo_01J2X9K7QZ0ABCDEFGHJKMNQ" -H "X-API-Key: $KEY"
```
```text
id: 129
event: task.state_changed
data: {"task_id":"tsk_...","type":"ingest","state":"embedding","previous_state":"chunking",...}

id: 136
event: task.state_changed
data: {"task_id":"tsk_...","type":"ingest","state":"completed","previous_state":"persisting",...}
```
看到 `state=completed` 即摄取完成。因 `auto_wiki=true`，随后还会收到一条 `type=wiki` 任务的 `pending` 事件。

### 步骤 4 · ask 非流式

```bash
curl -sS -X POST "$BASE/ask" -H "X-API-Key: $KEY" -H "Content-Type: application/json" \
  -d '{"repo_id":"repo_01J2X9K7QZ0ABCDEFGHJKMNQ","question":"中间件链怎么组织？","mode":"hybrid","top_k":8}' | jq '.data | {answer, refs: (.references|length), latency_ms}'
```
```json
{ "answer": "中间件链按 RequestID → Recovery → Auth → RateLimit → CORS 注册……", "refs": 8, "latency_ms": 2140 }
```

### 步骤 5 · ask 流式

```bash
curl -N -X POST "$BASE/ask/stream" -H "X-API-Key: $KEY" -H "Content-Type: application/json" \
  -d '{"repo_id":"repo_01J2X9K7QZ0ABCDEFGHJKMNQ","question":"限流是怎么做的？","mode":"hybrid"}'
```
```text
event: references
id: 1
data: {"request_id":"req_...","mode":"hybrid","references":[...]}

event: token
id: 2
data: {"delta":"采用两级限流：per-IP 滑动窗口"}

event: token
id: 3
data: {"delta":" + per-API-key 配额……"}

event: done
id: 4
data: {"usage":{"prompt_tokens":1502,"completion_tokens":180},"latency_ms":1630}
```

### 步骤 6 · 生成 wiki（202）

若步骤 2 未开 `auto_wiki`，可手动触发：

```bash
curl -sS -X POST "$BASE/wiki/generate" -H "X-API-Key: $KEY" -H "Content-Type: application/json" \
  -d '{"repo_id":"repo_01J2X9K7QZ0ABCDEFGHJKMNQ"}' | jq .data
```
```json
{ "task_id": "tsk_01J2X9K7QZ0ABCDEFGHJKMPF", "repo_id": "repo_...NQ", "type": "wiki", "state": "pending", "queue_position": 1, "created_at": "2026-07-05T09:10:00Z" }
```

### 步骤 7 · 获取 wiki

等 wiki 任务 `completed`（同样用 SSE 或轮询跟踪）后：

```bash
curl -sS "$BASE/repos/repo_01J2X9K7QZ0ABCDEFGHJKMNQ/wiki" -H "X-API-Key: $KEY" | jq '.data | {generated_at, toc: (.toc|length), pages: (.pages|length)}'
```
```json
{ "generated_at": "2026-07-05T09:12:30Z", "toc": 12, "pages": 12 }
```

### 步骤 8 · 修改配置 top_k（admin）

把默认召回数从 8 调到 12：

```bash
curl -sS -X PUT "$BASE/config" -H "X-API-Key: $ADMIN" -H "Content-Type: application/json" \
  -d '{"retriever":{"top_k":12}}' | jq .data
```
```json
{ "version": 8, "applied": { "retriever.top_k": 12 }, "restart_required": [], "warnings": [] }
```
写入经 etcd Txn 原子生效（覆写值 + `config_version`+1 + 审计同一事务），watch 回调毫秒级广播到全部节点；`restart_required` 为空说明立即热生效。用普通 key 调会得 `40301`。

### 步骤 9 · 取消一个排队任务

假设又提交了一个 ingest 正在排队：

```bash
# 先找到排队中的任务
curl -sS "$BASE/tasks?state=pending" -H "X-API-Key: $KEY" | jq '.data.items[] | {task_id, type, queue_position}'

# 取消它（202 受理，state 尚未变 cancelled）
curl -sS -X DELETE "$BASE/tasks/tsk_01J2X9K7QZ0ABCDEFGHJKMPX" -H "X-API-Key: $KEY" | jq '.data.state'
```
```json
"pending"
```
随后 SSE 会推一条 `state=cancelled` 的 `task.state_changed` 事件，确认取消生效。

### 步骤 10 · refresh 增量更新

gin 仓库有了新提交，做一次增量刷新：

```bash
curl -sS -X POST "$BASE/repos/repo_01J2X9K7QZ0ABCDEFGHJKMNQ/refresh" -H "X-API-Key: $KEY" | jq .data
```
```json
{ "task_id": "tsk_01J2X9K7QZ0ABCDEFGHJKMP3", "repo_id": "repo_...NQ", "type": "refresh", "state": "pending", "queue_position": 1, "created_at": "2026-07-05T09:20:00Z" }
```
refresh 任务按 `fetching → diffing → chunking → embedding → persisting → completed` 推进，只对变更文件重算向量（git CLI `fetch --depth 1` + `reset --hard FETCH_HEAD` + `clean -fdx`，同仓互斥走 Redis 分布式锁）。

---

## 6. 附录

### 6.1 状态机速查表（三类任务）

单一状态字段 `state`，终态为 `completed` / `failed` / `cancelled`。

| type | 状态流转（非终态 → 终态） | 阶段权重（折算整体进度） |
|---|---|---|
| `ingest` | `pending → cloning → parsing → chunking → embedding → persisting → completed` | cloning 15% / parsing 10% / chunking 10% / embedding 50% / persisting 15% |
| `refresh` | `pending → fetching → diffing → chunking → embedding → persisting → completed` | fetching 20% / diffing 10% / chunking 10% / embedding 45% / persisting 15% |
| `wiki` | `pending → outlining → generating → completed` | outlining 10% / generating 90% |

**通用规则**：

- 任一非终态阶段都可 → `failed`（阶段错误）或 → `cancelled`（收到取消）。
- 进入终态必须写 `finished_at`，且不可再转移。
- 非终态 → `failed` 时必须同时写 `error{code, message, stage}`。
- 非法转移返回 `40902 invalid_task_state`。
- refresh 无变更时仍走完整状态机，各阶段零工作量快速通过，`stats` 全 0。

全部合法 `state` 取值：`pending`、`cloning`、`parsing`、`chunking`、`embedding`、`persisting`、`outlining`、`generating`、`fetching`、`diffing`、`completed`、`failed`、`cancelled`。

### 6.2 环境变量清单

密钥**只从环境变量注入**，yaml 不落明文；修改后需重启生效（属 restart_required 类）。前 4 个为 v1 保留项，后 7 个为 v2.0 新增的基础设施坐标（总纲 §5.3）。

| 环境变量 | 对应配置 | 用途 |
|---|---|---|
| `DEEPWIKI_API_KEYS` | `auth.api_keys` | 普通 API key 列表（逗号分隔）；为空=开发模式放行；启动时哈希后幂等写入 `api_keys` 表 |
| `DEEPWIKI_ADMIN_KEY` | `auth.admin_key` | 管理端点 `PUT /config` 专用 key（落库 `is_admin=true`） |
| `DEEPWIKI_EMBEDDING_API_KEY` | `embedding.api_key` | Embedding provider 密钥 |
| `DEEPWIKI_LLM_API_KEY` | `llm.api_key` | LLM provider 密钥 |
| `DEEPWIKI_POSTGRES_DSN` | `storage.postgres.dsn` | Postgres 连接串（pgxpool；唯一注入途径，禁止 yaml 明文） |
| `DEEPWIKI_OPENSEARCH_USERNAME` | `search.opensearch.username` | OpenSearch 用户名（dev 关闭安全插件可留空） |
| `DEEPWIKI_OPENSEARCH_PASSWORD` | `search.opensearch.password` | OpenSearch 密码 |
| `DEEPWIKI_RABBITMQ_URL` | `queue.rabbitmq.url` | RabbitMQ 连接 URL（含 vhost） |
| `DEEPWIKI_REDIS_SENTINEL_ADDRESSES` | `redis.sentinel.addresses` | Redis 哨兵地址列表（逗号分隔，覆盖 yaml） |
| `DEEPWIKI_REDIS_PASSWORD` | `redis.password` | Redis 密码（哨兵与 master 共用） |
| `DEEPWIKI_ETCD_ENDPOINTS` | `etcd.endpoints` | etcd 客户端端点（逗号分隔，覆盖 yaml） |

其余 `DEEPWIKI_` 前缀环境变量经 viper AutomaticEnv 覆盖对应配置 key（仅启动期引导；运行时业务配置以 etcd 为准）。

**provider 取值**：

- `embedding.provider`：`openai` \| `dashscope` \| `siliconflow` \| `ollama` \| `voyage`
- `llm.provider`：`openai` \| `gemini` \| `claude` \| `ollama` \| `deepseek`

### 6.3 主要配置 key 速查

**(A) 运行时业务配置（PUT /config 可改，热更新）**——v1 既有项冻结，另含总纲 §5.2 中标注「热更新=是」的新项：

| key | 类型 | 默认 | 热更新 | 校验 |
|---|---|---|---|---|
| `worker.pool_size` | int | 2 | 是（软扩缩容） | 1~16 |
| `worker.queue_size` | int | 100 | 是 | 1~1000 |
| `ingest.chunk_size` | int | 800 | 是 | 100~4000 |
| `ingest.chunk_overlap` | int | 120 | 是 | 0 ~ chunk_size/2 |
| `retriever.mode` | enum | hybrid | 是 | keyword\|embedding\|hybrid |
| `retriever.top_k` | int | 8 | 是 | 1~30 |
| `retriever.rrf_k` | int | 60 | 是 | 1~1000 |
| `llm.temperature` | float | 0.2 | 是 | 0~2 |
| `llm.max_tokens` | int | 2048 | 是 | 1~32768 |
| `rate_limit.per_ip.rps` | int | 10 | 是 | 1~1000 |
| `rate_limit.per_ip.burst` | int | 20 | 是 | 1~2000（且 burst ≥ rps） |
| `rate_limit.per_key.ingest_per_hour` | int | 20 | 是 | 1~1000 |
| `rate_limit.per_key.ask_per_minute` | int | 30 | 是 | 1~1000 |
| `storage.postgres.max_conns` | int | 10 | 是 | pgxpool 连接池上限 |
| `storage.vector.ef_search` | int | 64 | 是 | HNSW 查询精度/延迟权衡 |
| `queue.rabbitmq.max_retries` | int | 3 | 是 | DLX 重试链次数 |
| `git.op_timeout` | duration | 10m | 是 | 单次 git CLI 操作超时 |
| `observability.log.level` | enum | info | 是 | debug\|info\|warn\|error |
| `server.addr` | string | :8080 | 否（restart_required，可写入） | 合法 host:port |

**(B) 基础设施引导配置（yaml/env 引导层；PUT /config 拒绝 `40001`，修改需重启）**——总纲 §5.2 权威清单：

| key | 类型 | 默认 | 热更新 | 说明 |
|---|---|---|---|---|
| `storage.postgres.dsn` | string | 空 | 否（restart_required） | **禁止 yaml 明文**，仅 env `DEEPWIKI_POSTGRES_DSN` |
| `storage.vector.dimensions` | int | 1536 | 否（建表定型） | pgvector 列维度；改维度 = 新迁移 + 全量重建 |
| `search.opensearch.addresses` | []string | `["http://localhost:9200"]` | 否 | |
| `search.opensearch.username/password` | string | 空 | 否（仅 env） | `DEEPWIKI_OPENSEARCH_USERNAME/PASSWORD` |
| `search.opensearch.index_prefix` | string | `deepwiki` | 否 | 索引名前缀 |
| `queue.rabbitmq.url` | string | 空 | 否 | 仅 env `DEEPWIKI_RABBITMQ_URL` |
| `queue.rabbitmq.prefetch` | int | = worker.pool_size | 否 | |
| `redis.sentinel.addresses` | []string | `[localhost:26379]` | 否 | env `DEEPWIKI_REDIS_SENTINEL_ADDRESSES` 可覆盖 |
| `redis.sentinel.master_name` | string | `deepwiki-master` | 否 | |
| `redis.password` | string | 空 | 否（仅 env） | `DEEPWIKI_REDIS_PASSWORD` |
| `etcd.endpoints` | []string | `[localhost:2379]` | 否 | env `DEEPWIKI_ETCD_ENDPOINTS` 可覆盖 |
| `etcd.prefix` | string | `/deepwiki` | 否 | |
| `git.binary_path` | string | `git` | 否 | |
| `observability.otel_endpoint` | string | 空（禁用） | 否 | OTLP gRPC |

> 完整配置清单与校验规则见《00_设计基线》§8 与《03_企业级技术栈变更总纲》§5。

### 6.4 部署提示

**docker compose 服务清单**（开发/验收态一键拉起，总纲 §3.1 权威）：

| 服务 | 镜像 | 端口 | 关键配置 |
|---|---|---|---|
| `postgres` | `pgvector/pgvector:pg16` | 5432 | `POSTGRES_DB=deepwiki`；`shared_buffers=256MB`；健康检查 `pg_isready` |
| `opensearch` | `opensearchproject/opensearch:2.17.1` | 9200 | 单节点 dev：`discovery.type=single-node`、`plugins.security.disabled=true`、`OPENSEARCH_INITIAL_ADMIN_PASSWORD` 仅生产需要；`bootstrap.memory_lock=true`、`ES_JAVA_OPTS=-Xms512m -Xmx512m`、ulimit memlock 无限制 |
| `rabbitmq` | `rabbitmq:3.13.7-management` | 5672 / 15672 | 默认 vhost `/`；management UI 验收用 |
| `redis-master` | `redis:7.4.1-alpine` | 6379 | `appendonly yes` |
| `redis-replica-1/2` | 同上 | 6380/6381 | `replicaof redis-master 6379` |
| `redis-sentinel-1/2/3` | 同上 | 26379/26380/26381 | `sentinel monitor deepwiki-master redis-master 6379 2`；`down-after-milliseconds 5000`；`failover-timeout 15000` |
| `etcd` | `quay.io/coreos/etcd:v3.5.21` | 2379 | dev 单节点（`ETCD_LISTEN_CLIENT_URLS` 等）；生产 3 节点集群 |

**生产部署拓扑**（总纲 §3.2 权威）：

```text
                 ┌──────────────┐
   Client ──LB──▶│ API 节点 × N  │（无状态：Gin + 限流中间件 + SSE/WS 推送）
                 └──┬───┬───┬───┘
                    │   │   │
   ┌────────────────┘   │   └──────────────────┐
   ▼                    ▼                      ▼
┌─────────┐      ┌────────────┐        ┌─────────────┐
│Postgres │      │  RabbitMQ   │        │ Redis 哨兵集群│
│+pgvector│      │  队列集群   │        │(1主2从3哨兵) │
└─────────┘      └─────┬──────┘        └─────────────┘
                       │ consume          ▲
                 ┌─────▼──────┐           │
                 │Worker 节点×M│───────────┘（分布式锁/限流/事件总线）
                 └─┬────────┬─┘
                   ▼        ▼
            ┌───────────┐ ┌──────────┐
            │ OpenSearch │ │  etcd    │   + 共享卷（仓库工作目录）
            │  3 节点    │ │  3 节点  │
            └───────────┘ └──────────┘
```

| 主题 | 要点 |
|---|---|
| 系统依赖 git | 镜像/主机必须安装 **git ≥ 2.30**（Dockerfile `apk add git`，debian 系 `apt-get install -y git`）；启动时 `git --version` 探测，缺失或版本不足 → health `degraded`（`git.available=false`）且 ingest/refresh 直接失败 |
| API 节点无状态 | 不持有任务执行、不持有本地索引；SSE/WS 连接的订阅关系仅内存，事件经 Redis Pub/Sub 扇入；可自由水平扩缩 |
| Worker 节点与共享卷 | 消费 RabbitMQ（`prefetch = worker.pool_size`）；仓库工作目录 `./data/repos` 多 Worker 生产态须挂 NFS/PVC 共享卷；课程/中小企业规模可单 Worker 节点本地盘——两者择一，勿以本地盘跑多副本，否则 refresh 的 fetch/reset 会互相覆盖 |
| 配置分层 | yaml+env 只引导基础设施坐标（DSN、endpoints）；运行时业务配置全部在 etcd（`/deepwiki/config/*`），`PUT /config` 经 Txn 原子生效、watch 毫秒级广播全节点；密钥只走环境变量，禁止写入 etcd |
| 反向代理取真实 IP | per-IP 限流基于 `RemoteAddr`；经 Nginx/ngrok 时必须 `gin.SetTrustedProxies(...)` 配置可信代理后才可信 `X-Forwarded-For`，否则限流键恒为代理 IP。答辩走内网穿透时建议说明或调大 per_ip 窗口 |
| SSE 缓冲 | 反向代理需对 SSE 端点禁缓冲；服务已下发 `X-Accel-Buffering: no`，Nginx 侧确认未覆盖即可 |
| HTTP/2 与 SSE 连接数 | HTTP/1.1 下浏览器每域约 6 个长连接，`/events` + `/ask/stream` 并发演示可能耗尽；建议演示用 HTTP/2，或 curl 分终端演示，或用 WebSocket（不受此限）作备份 |
| 优雅退出 | API 节点：SIGINT/SIGTERM → readiness 置失败（health 返 `50301`）→ 停接新请求 → 等在途请求与 SSE/WS 推送排空（上限 `server.shutdown_timeout`）→ flush 日志 → 关连接。Worker 节点：readiness fail → `Channel.Cancel` 停拉新消息 → 等在途任务完成（上限 `server.shutdown_timeout`）→ 未完成者 nack requeue=true 让别的节点接走 → 关 RabbitMQ/Postgres 连接（防连接泄漏） |
| 重启恢复 | Worker 启动时 Reconciler 扫描 Postgres 非终态任务：pending 且无消费者持有 → 重新投递；running 态且 `updated_at` 超 5 分钟无心跳 → 重置 pending 重投（执行前 CAS 抢占 `UPDATE ... WHERE state='pending'` 保证幂等，at-least-once 下不重复执行）。无法安全重投的任务落 `failed`（`error.code=50004`，`message="task interrupted by service restart"`），客户端按原端点重提交 |
| Postgres | 连接池 pgxpool（`MaxConns=10, MinConns=2, MaxConnLifetime=1h, HealthCheckPeriod=30s`）；迁移 golang-migrate 只前进不回滚（`migrations/*.up.sql`，无 .down）；dirty 状态 panic 退出并提示 `migrate force`；时间列 `timestamptz`，`stats_json/progress_json/error_json` 为 JSONB |

---

> 本文档与《00_设计基线》及《03_企业级技术栈变更总纲 v2.0》保持一致：端点 18 个、错误码 20 个（16 个 v1 冻结 + 4 个 v2.0 新增）、状态名与字段名逐字符对齐、health 按总纲 §7 新契约。任何与总纲的差异以总纲为准。
