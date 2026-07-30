# GoWiki API 接口文档

版本：v0.2.0 · 基础路径：`/api/v1` · 默认端口：`8080`

> 本文档与代码逐一核对生成（router/handler/dto/middleware/model），字段名、错误码、限流数值均为代码事实。

---

## 1. 通用约定

### 1.1 统一响应信封

除特别标注外，所有端点返回统一信封：

```json
{
  "code": 0,
  "message": "ok",
  "data": {},
  "request_id": "req_01KYSAVMWDJKBCWQZS0ZBTF9NS",
  "details": [{ "field": "repo_url", "issue": "repository already ingested", "existing_repo_id": "repo_..." }]
}
```

- 成功：`code=0, message="ok"`；查询类 HTTP 200，**建任务类（ingest/refresh/wiki generate）与取消任务返回 HTTP 202**。
- 失败：`code≠0`，HTTP 状态码按错误码映射（见 §1.3）；`details` 仅部分错误携带。
- 例外（不走信封）：
  - `GET /repos/{id}/wiki/export` 直出 `text/markdown` 字节流；
  - `GET /metrics` 直出 Prometheus 文本；
  - `POST /ask/stream`、`GET /events` 进入流式后改用事件帧（见 §6.2、§8）。

### 1.2 鉴权

- 请求头：`X-API-Key: <key>`。
- 二级查找：Redis 缓存（TTL 60s）→ Postgres `api_keys` 表；库中只存 `SHA-256(salt‖key)` 哈希，不存明文。
- **开发模式**：`auth.api_keys` 为空（env `DEEPWIKI_API_KEYS=`）时全部接口免鉴权并 WARN 一次，admin 校验同样放行。
- 免鉴权端点（任何模式）：`GET /api/v1/health`、`GET /metrics`。
- admin key（env `DEEPWIKI_ADMIN_KEY`）仅用于 `PUT /api/v1/config`；非 admin → `40301`。

### 1.3 错误码全表

| code | 常量 | HTTP | 说明 |
|---|---|---|---|
| 40001 | invalid_param | 400 | 参数/JSON 校验失败（details 含字段级原因） |
| 40101 | unauthorized | 401 | 缺失或非法 API key |
| 40301 | forbidden | 403 | 非 admin 调用管理端点 |
| 40401 | task_not_found | 404 | 任务不存在 |
| 40402 | repo_not_found | 404 | 仓库不存在（chunk 不存在也复用此码） |
| 40403 | wiki_not_found | 404 | wiki 尚未生成 |
| 40901 | repo_already_exists | 409 | 仓库已摄取（details 含 `existing_repo_id`） |
| 40902 | invalid_task_state | 409 | 状态冲突（仓库未就绪/已有运行中任务/wiki 生成中/终态不可取消） |
| 42201 | config_validation_failed | 422 | 配置补丁校验失败 |
| 42901 | rate_limited | 429 | 触发限流（带 `Retry-After`） |
| 42902 | queue_full | 429 | 任务队列满（带 `Retry-After: 60`） |
| 50001 | internal_error | 500 | 内部错误 |
| 50201 | llm_unavailable | 502 | LLM 服务不可用（熔断/超时/余额不足） |
| 50202 | embedding_unavailable | 502 | Embedding 服务不可用 |
| 50203 | vector_store_unavailable | 502 | 向量检索暂不可用 |
| 50301 | service_not_ready | 503 | 优雅退出中（仅 /health） |
| 50302 | queue_unavailable | 503 | 任务队列暂不可用 |
| 50303 | search_unavailable | 503 | 检索服务暂不可用 |
| 50304 | config_store_unavailable | 503 | 配置中心暂不可用 |

### 1.4 限流

| 层级 | 规则 | 默认值 |
|---|---|---|
| L1 per-IP | 滑动窗口 60s，`rps*60 + burst` | 10 rps + 20 burst = 620 次/分 |
| L2 ingest 类 | POST /ingest、/refresh | 20 次/小时 |
| L2 ask 类 | POST /ask、/ask/stream | 30 次/分钟 |
| L2 wiki 类 | POST /wiki/generate | 10 次/小时 |

- L2 按 API key 计数，无 key（dev 模式）退化为 per-IP。
- 响应头：`X-RateLimit-Limit` / `X-RateLimit-Remaining` / `X-RateLimit-Reset`。

### 1.5 ID 格式

所有 ID 为前缀 + 26 位 Crockford Base32 ULID：`repo_…` / `tsk_…` / `chk_…`，正则 `^(repo|tsk|chk)_[0-9A-HJKMNP-TV-Z]{26}$`。

### 1.6 任务状态机

`pending → cloning → parsing → chunking → embedding → persisting → completed`
wiki 任务：`pending → outlining → generating → completed`；refresh 任务：`pending → fetching → diffing → …`；终态：`completed / failed / cancelled`。

---

## 2. 系统

### 2.1 GET /api/v1/health — 健康检查（免鉴权）

数据来自 60s 后台探测快照，接口内不发外部调用。

```json
{
  "status": "ok | degraded",
  "version": "0.2.0",
  "uptime_seconds": 1290,
  "started_at": "2026-07-30T11:33:45Z",
  "llm":       { "provider": "openai", "model": "deepseek-ai/DeepSeek-V4-Flash", "reachable": true, "breaker": "closed" },
  "embedding": { "provider": "siliconflow", "model": "BAAI/bge-large-zh-v1.5", "dimensions": 1024, "reachable": true, "breaker": "closed" },
  "postgres":  { "connected": true, "pool": { "total": 3, "idle": 3 }, "migration_version": 4 },
  "opensearch":{ "connected": true, "cluster_status": "green", "indices": 2 },
  "rabbitmq":  { "connected": true, "queue_depth": 0, "consumers": 1 },
  "redis":     { "connected": true, "mode": "sentinel", "master": "FailoverClient", "ratelimit_degraded": false },
  "etcd":      { "connected": true, "endpoints": ["etcd:2379"] },
  "git":       { "available": true, "version": "2.45.4" },
  "worker":    { "busy": 0, "total": 2, "queued": 0 }
}
```

优雅退出中返回 503 + `50301`（data 照常返回）。

### 2.2 GET /metrics — Prometheus 指标（免鉴权，无版本前缀）

---

## 3. 仓库

### 3.1 POST /api/v1/ingest — 摄取仓库 → 202

```json
{
  "repo_url": "https://github.com/sirupsen/logrus",
  "branch": "",
  "auto_wiki": false,
  "options": { "chunk_size": 800, "chunk_overlap": 60, "include_ext": [".go"], "exclude_dirs": ["vendor"] }
}
```

校验：`repo_url` ≤512 且为 http(s)/git@ 形式；`branch` ≤128 且禁 `..` 等特殊字符；`chunk_size ≥100`；`chunk_overlap ≤ chunk_size/2`。

```json
{ "task_id": "tsk_...", "repo_id": "repo_...", "type": "ingest", "state": "pending", "queue_position": 1, "created_at": "..." }
```

错误：`40001` / `40901`（已摄取，details 带 `existing_repo_id`）/ `42902` / `50302`。

### 3.2 GET /api/v1/repos — 仓库列表

Query：`page`（默认 1）、`page_size`（默认 20，上限 100）。按 created_at 倒序。

```json
{
  "items": [{ "repo_id": "...", "repo_url": "...", "branch": "main", "commit_hash": "...",
              "state": "ingesting | ready | error",
              "stats": { "files": 0, "chunks": 0, "tokens": 0 },
              "created_at": "...", "updated_at": "..." }],
  "pagination": { "page": 1, "page_size": 20, "total": 2, "total_pages": 1 }
}
```

### 3.3 GET /api/v1/repos/{repo_id} — 仓库详情

Repo 全字段 + `latest_task`（Task，可空）+ `wiki_available`(bool) + `chunk_count`(int64)。错误：`40001` / `40402`。

### 3.4 DELETE /api/v1/repos/{repo_id} — 删除仓库（级联）

```json
{ "repo_id": "...", "deleted": { "chunks": 460, "vectors": 296, "wiki_pages": 8, "opensearch_docs": 296, "local_dir": true } }
```

错误：`40001` / `40402`。

### 3.5 POST /api/v1/repos/{repo_id}/refresh — 增量刷新 → 202

无 body。git diff 驱动的增量重摄取（modified ∪ deleted 文件对应 chunk 删除重做）。返回结构同 ingest（`type:"refresh"`）。

错误：`40902`（repo 未 ready / 已有运行中任务 / refresh 进行中）。

### 3.6 GET /api/v1/repos/{repo_id}/paths/exists?prefix=… — 路径前缀存在性校验

供前端 path_filter 输入去抖校验用。**格式非法也返回 200**，以 `valid` 字段区分：

```json
{ "prefix": "router/", "valid": true, "exists": true, "reason": "" }
```

规则：≤256、禁 `..`、禁 `\`、禁前导 `/`；空前缀恒 `exists=true`。

---

## 4. 任务

Task 结构：

```json
{ "task_id": "tsk_...", "type": "ingest | refresh | wiki", "repo_id": "repo_...",
  "state": "embedding",
  "progress": { "current": 35, "total": 100, "percent": 35 },
  "stats": { "files": 0, "chunks": 0, "tokens": 0 },
  "error": null,
  "queue_position": 0, "created_at": "...", "started_at": "...", "finished_at": null }
```

### 4.1 GET /api/v1/tasks — 任务列表

Query：`type`、`state`、`repo_id`、`page`、`page_size`。返回 PageResult[Task]（结构同 §3.2 分页）。

### 4.2 GET /api/v1/tasks/{task_id} — 任务详情

错误：`40001` / `40401`。

### 4.3 DELETE /api/v1/tasks/{task_id} — 取消任务 → 202

立即写入终态 `cancelled`（含 running 任务；执行器后续写入被幂等忽略）。错误：`40902`（已终态）。

---

## 5. 问答（RAG）

### 5.1 POST /api/v1/ask — 同步问答

```json
{
  "repo_id": "repo_...",
  "question": "logrus 的 Entry 是如何输出日志的？",
  "mode": "hybrid",
  "top_k": 5,
  "temperature": 0.7,
  "path_filter": "internal/",
  "history": [{ "role": "user", "content": "..." }, { "role": "assistant", "content": "..." }]
}
```

- `repo_id` 与 `repo_url` 二选一必填（`repo_url` 直传时按 URL+branch 查仓，可带 `branch`）。
- `question` 1~4000 字；`mode ∈ keyword | embedding | hybrid`（缺省取配置）；`top_k ∈ [1,30]`。
- `path_filter`：仓库相对路径前缀，规则同 §3.6。

```json
{
  "answer": "……（含 [path:start-end] 引用标注）",
  "references": [{ "chunk_id": "chk_...", "path": "entry.go", "start_line": 186, "end_line": 311,
                   "language": "go", "score": 3.0, "snippet": "…" }],
  "mode": "hybrid",
  "usage": { "prompt_tokens": 4606, "completion_tokens": 2751 },
  "latency_ms": 149735
}
```

> 引用为**父块**（完整函数级上下文）；检索入口是子块向量/BM25，命中后回父块。`references` 必须来自真实检索结果（硬约束 #15），不允许 LLM 杜撰。

错误：`40001` / `40402` / `40902`（repo 未 ready）/ `50201` / `50202` / `50203` / `50303`。

### 5.2 POST /api/v1/ask/stream — 流式问答（SSE）

请求体与校验同 §5.1。响应 `Content-Type: text/event-stream`，15s 注释心跳（`: heartbeat`）。

事件序列：

| event | data | 说明 |
|---|---|---|
| `references` | `{request_id, mode, references:[…]}` | 检索完成后首帧 |
| `thinking` | `{delta}` | 思维链增量（可折叠展示） |
| `token` | `{delta}` | 答案增量 |
| `done` | `{usage, latency_ms}` | 结束 |
| `error` | `{code, message, request_id}` | 流式中途失败时以此终止 |

进入流式前失败仍返回标准错误信封。

---

## 6. Wiki

### 6.1 POST /api/v1/wiki/generate — 生成 wiki → 202

```json
{ "repo_id": "repo_..." }
```

返回 TaskSubmittedResponse（`type:"wiki"`）。重复生成去重：已有非终态 wiki 任务时返回 `40902`（"wiki 正在生成中，请等待完成"）。支持断点续跑：中途失败重投后按 slug 对齐跳过已完成页。

### 6.2 GET /api/v1/repos/{repo_id}/wiki — 获取 wiki

```json
{
  "repo_id": "repo_...",
  "toc": [{ "slug": "overview", "title": "项目概览", "parent_slug": "", "sort_order": 1 }],
  "pages": [{ "slug": "overview", "title": "项目概览", "content_md": "# …（含 mermaid 图）", "sort_order": 1, "updated_at": "..." }],
  "task_id": "tsk_...", "generated_at": "..."
}
```

错误：`40001` / `40402` / `40403`（未生成）。

### 6.3 GET /api/v1/repos/{repo_id}/wiki/export — 导出 markdown（不走信封）

200 `text/markdown; charset=utf-8`，`Content-Disposition: attachment; filename="wiki-<repo_id>.md"`；全部页面按目录序拼接。错误时走信封。

---

## 7. Chunk 与配置

### 7.1 GET /api/v1/chunks/{chunk_id} — 查看 chunk 原文

供前端「引用看代码」面板用。**注意：此端点响应为 PascalCase 字段**（`model.Chunk` 无 json tag，代码事实）：

```json
{ "ChunkID": "chk_...", "RepoID": "repo_...", "Path": "entry.go", "StartLine": 186, "EndLine": 311,
  "Language": "go", "Content": "…", "Vector": null, "FileHash": "…", "EmbeddingModel": "bge-large-zh-v1.5" }
```

chunk 不存在返回 `40402`，message 为 "chunk not found"。

### 7.2 GET /api/v1/config — 读取运行配置（脱敏）

```json
{ "version": 3, "config": { "server": {…}, "rate_limit": {…}, "worker": {…}, "ingest": {…},
  "embedding": {…}, "llm": {…}, "retriever": {…}, "storage": {…}, "search": {…}, "queue": {…},
  "redis": {…}, "etcd": {…}, "git": {…}, "observability": {…} },
  "restart_required": [] }
```

`auth` 整节与 DSN/密码类字段不返回；`api_key` 经掩码返回。

### 7.3 PUT /api/v1/config — 热更新配置（admin only）

Body 为任意非空 JSON patch（原文应用，非 DTO 绑定）：

```json
{ "retriever": { "mode": "hybrid", "top_k": 8 }, "llm": { "temperature": 0.5 } }
```

```json
{ "version": 4, "applied": {…}, "restart_required": ["storage.vector.dimensions"], "warnings": [] }
```

错误：`40001`（空 body）/ `40301`（非 admin）/ `42201`（校验失败）/ `50304`。

---

## 8. 实时事件

### 8.1 GET /api/v1/events — SSE 事件流

Query：`types`（逗号分隔）、`repo_id`、`task_id`；Header `Last-Event-ID: <seq>` 触发 Redis Streams 回放（历史截断时先推 `gap` 事件）。

帧格式：`id: <seq>\nevent: <type>\ndata: <Event JSON>\n\n`，15s 注释心跳。

```json
{ "seq": 1024, "type": "task.state_changed", "repo_id": "repo_...", "task_id": "tsk_...",
  "timestamp": "2026-07-30T11:00:00Z", "payload": { "state": "embedding", "progress": {…}, "stats": {…} } }
```

事件类型（冻结）：`task.state_changed`（pending 事件 payload 另含 `queue_position`）、`task.progress`、`wiki.completed`、`gap`。

### 8.2 GET /api/v1/ws — WebSocket 事件流

握手：标准 Upgrade；Origin 校验放行规则：无 Origin 头 / 白名单为空 / 白名单命中 / 同源。

Query：`types` / `repo_id` / `task_id` 同 SSE；`resume_from=<seq>` 替代 Last-Event-ID。

下行 JSON 帧：

```json
{ "seq": 1024, "type": "task.state_changed", "data": { "state": "embedding", "progress": {…}, "stats": {…} } }
```

上行无协议（读侧仅感知断开）；服务端每 15s 发 WS Ping，写超时 10s。

---

## 附：Postman 使用

仓库内附 `docs/gowiki.postman_collection.json`，导入后设置集合变量：

| 变量 | 说明 |
|---|---|
| `base_url` | 默认 `http://localhost:8080` |
| `api_key` | dev 模式留空即可；开启鉴权后填明文 key |

SSE/WebSocket 端点 Postman 支持原生调试（New → WebSocket / Server-Sent Events）。
