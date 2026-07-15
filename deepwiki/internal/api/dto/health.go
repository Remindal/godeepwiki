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
