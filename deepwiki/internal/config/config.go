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
	// TrustedProxies 可信反向代理 CIDR 列表（gin.SetTrustedProxies；空 = 不信任任何代理，
	// per-IP 限流取 RemoteAddr，忽略 X-Forwarded-For，§9.1）。
	TrustedProxies []string `mapstructure:"trusted_proxies" yaml:"trusted_proxies" json:"trusted_proxies"`
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
