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
	// ---- 业务指标（本轮新增） ----
	IngestTotal     *prometheus.CounterVec // deepwiki_ingest_total{status=success|failure}
	AskTotal        *prometheus.CounterVec // deepwiki_ask_total{status=success|failure}
	ChunkIndexTotal prometheus.Counter     // deepwiki_chunk_index_total
	QueueDepth      prometheus.Gauge       // deepwiki_queue_depth（RabbitMQ 主队列深度，定时探测写入）
}

// Register 注册全部指标（promauto 默认注册表；重复注册会 panic，只允许 main 调用一次）。
// 同时写入包级 Default（业务埋点 helper 的数据源）。
func Register() *Metrics {
	m := &Metrics{
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
		IngestTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "deepwiki_ingest_total", Help: "ingest 任务计数（status=success|failure）"}, []string{"status"}),
		AskTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "deepwiki_ask_total", Help: "ask 请求计数（status=success|failure）"}, []string{"status"}),
		ChunkIndexTotal: promauto.NewCounter(prometheus.CounterOpts{
			Name: "deepwiki_chunk_index_total", Help: "已索引 chunk 总数"}),
		QueueDepth: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "deepwiki_queue_depth", Help: "RabbitMQ 主队列 deepwiki.task.jobs 深度（定时探测）"}),
	}
	Default = m
	return m
}

// Default 全局指标集合（main 调用 Register 后写入；未注册时为 nil，helper 全部 no-op，便于单测）。
var Default *Metrics

// IncIngest ingest 任务计数（status=success|failure）。
func IncIngest(status string) {
	if Default != nil {
		Default.IngestTotal.WithLabelValues(status).Inc()
	}
}

// IncAsk ask 请求计数（status=success|failure）。
func IncAsk(status string) {
	if Default != nil {
		Default.AskTotal.WithLabelValues(status).Inc()
	}
}

// AddChunkIndex 累计已索引 chunk 数。
func AddChunkIndex(n int) {
	if Default != nil && n > 0 {
		Default.ChunkIndexTotal.Add(float64(n))
	}
}

// SetQueueDepth 写入 RabbitMQ 主队列深度（healthProber 定时探测调用）。
func SetQueueDepth(v float64) {
	if Default != nil {
		Default.QueueDepth.Set(v)
	}
}

// SetWorkerBusy 写入当前忙碌 worker 数。
func SetWorkerBusy(v float64) {
	if Default != nil {
		Default.WorkerBusy.Set(v)
	}
}
