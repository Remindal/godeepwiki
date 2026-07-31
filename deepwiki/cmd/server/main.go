// DeepWiki(Go版) 服务端入口。
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
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
	"deepwiki/internal/api/middleware"
	"deepwiki/internal/config"
	"deepwiki/internal/embed"
	"deepwiki/internal/eventbus"
	"deepwiki/internal/ingest"
	"deepwiki/internal/llm"
	"deepwiki/internal/lock"
	"deepwiki/internal/model"
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
	cm := config.NewManager(cfg, overrides, cfgVersion, etcdSrc, logger)
	go cm.StartWatch(ctx)

	// ⑤ Postgres 连接池（pgxpool：MaxConns=10, MinConns=2, MaxConnLifetime=1h, HealthCheckPeriod=30s，总纲 §4.1）。
	db, err := store.Open(ctx, cfg.Storage.Postgres.DSN, cfg.Storage.Postgres.MaxConns, logger)
	if err != nil {
		fatal(logger, "connect postgres", err)
	}
	pool := db.Pool()

	// ⑥ golang-migrate Up（embed.FS source；dirty 状态 panic 退出并提示 migrate force，只前进原则，总纲 R4）。
	if err := store.Migrate(cfg.Storage.Postgres.DSN, logger); err != nil {
		fatal(logger, "migrate", err)
	}

	// ⑦ OpenSearch 客户端 + 启动一致性校验（每仓 count(index) == chunks 表行数，总纲 §4.2）。
	searchCli, err := search.NewClient(ctx, cfg.Search.OpenSearch, logger)
	if err != nil {
		fatal(logger, "connect opensearch", err)
	}
	chunkStore := store.NewChunkStore(db, logger)
	repoStore := store.NewRepoStore(db, logger)

	// embedding 维度探测依赖（PUT /config 变更 embedding 时校验与库列维度一致，反 AI 错误 #14）。
	cm.WithDimProbe(
		func(ecfg config.EmbeddingConfig) (config.DimProber, error) { return embed.New(ecfg, logger) },
		func(ctx context.Context) (int64, error) {
			var n int64
			err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM chunks`).Scan(&n)
			return n, err
		},
	)
	go verifyIndices(ctx, repoStore, chunkStore, searchCli, pool, logger)

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
	// 周期恢复：worker 崩溃后 running 僵死任务不等重启即可被重置重投（总纲 §4.3）。
	reconciler.StartPeriodic(ctx, time.Minute)

	// ⑪ 统一任务系统：事件总线（Redis Streams + Pub/Sub 扇出）+ 消费端 + 有界 Worker Pool。
	bus := eventbus.NewRedisStreamsBus(rdb, logger)
	go bus.StartFanout(ctx)
	tm := task.NewManager(taskStore, bus, publisher, cfg.Worker, logger).
		WithLocker(lock.New(rdb, logger)).
		WithMaxRetries(cfg.Queue.RabbitMQ.MaxRetries)

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
	apiKeyStore := store.NewAPIKeyStore(db, logger)
	bootstrapAPIKeys(ctx, cfg.Auth, apiKeyStore, logger)
	wikiStore := store.NewWikiStore(db, logger)
	vectorStore := store.NewVectorStore(db, cfg.Storage.Vector.EFSearch, logger)
	cloner := ingest.NewGitCloner(cfg.Git.BinaryPath, cfg.Git.OpTimeout, logger)
	retrievers := buildRetrievers(searchCli, chunkStore, vectorStore, pool, emb, llmClient, cfg, logger)

	ingestSvc := service.NewIngestService(tm, repoStore, cloner, publisher, cm, logger)
	repoSvc := service.NewRepoService(repoStore, chunkStore, vectorStore, wikiStore, searchCli, tm, logger)
	askSvc := service.NewAskService(cm, repoStore, retrievers, llmClient, logger)
	wikiSvc := service.NewWikiService(tm, repoStore, wikiStore, logger)

	// 注册三类任务执行器后再启动消费（总纲 §4.3：CAS 抢占 → 路由 Executor）。
	onAutoWiki := func(ctx context.Context, repoID string) {
		if _, err := wikiSvc.Generate(ctx, repoID); err != nil {
			logger.Warn("auto wiki generate failed", zap.String("repo_id", repoID), zap.Error(err))
		}
	}
	tm.RegisterExecutor(service.NewIngestExecutor(taskStore, repoStore, cloner, emb, chunkStore, searchCli, bus, cm, onAutoWiki, logger))
	tm.RegisterExecutor(service.NewRefreshExecutor(taskStore, repoStore, cloner, emb, chunkStore, searchCli, bus, cm, logger))
	tm.RegisterExecutor(service.NewWikiExecutor(taskStore, repoStore, wikiStore, chunkStore, retrievers, llmClient, cm, bus, logger))
	tm.Start(ctx, consumer)
	tm.StartDLQConsumer(ctx, mq)

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
		Chunks:    chunkStore,
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
	db.Close()                              // ④ postgres
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
	// provider（LLM/Embedding）经代理访问外网，冷启动首次 TLS 握手较慢；
	// 且 thinking 模型（DeepSeek-V4 等）连 ping 也会先跑推理，20s 经常性超时误报。
	// 给 60s 宽限；仅作用于后台探测 goroutine，不会阻塞 health 请求路径。
	providerCtx, providerCancel := context.WithTimeout(ctx, 60*time.Second)
	defer providerCancel()

	degraded := false
	snap := p.snapshot.Load()

	// Postgres：Ping + 连接池状态 + 迁移版本（schema_migrations）。
	pgOK := p.pool.Ping(probeCtx) == nil
	stat := p.pool.Stat()
	snap.Postgres.Connected = pgOK
	snap.Postgres.Pool.Total = stat.TotalConns()
	snap.Postgres.Pool.Idle = stat.IdleConns()
	p.metrics.PgPoolConns.WithLabelValues("total").Set(float64(stat.TotalConns()))
	p.metrics.PgPoolConns.WithLabelValues("idle").Set(float64(stat.IdleConns()))
	p.metrics.PgPoolConns.WithLabelValues("acquired").Set(float64(stat.AcquiredConns()))
	if pgOK {
		var version uint
		if err := p.pool.QueryRow(probeCtx, `SELECT version FROM schema_migrations LIMIT 1`).Scan(&version); err == nil {
			snap.Postgres.MigrationVersion = version
		}
	} else {
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

	// RabbitMQ：主队列深度与消费者数（QueueDeclarePassive）；指标 deepwiki_queue_length / rabbitmq_queue_depth。
	depth, consumers, mqErr := p.publisher.QueueStats(probeCtx)
	snap.RabbitMQ.Connected = mqErr == nil
	snap.RabbitMQ.QueueDepth = depth
	snap.RabbitMQ.Consumers = consumers
	if mqErr == nil {
		p.metrics.QueueLength.Set(float64(depth))
		p.metrics.RabbitMQQueueDepth.WithLabelValues("deepwiki.task.jobs").Set(float64(depth))
		observability.SetQueueDepth(float64(depth))
	} else {
		degraded = true
	}

	// Redis 哨兵：Ping + 当前主地址（FailoverClient 故障转移后 Options().Addr 自动指向新主）；
	// ratelimit_degraded 反映降级兜底状态（总纲 §4.4）。
	redisOK := p.rdb.Ping(probeCtx).Err() == nil
	snap.Redis.Connected = redisOK
	snap.Redis.Mode = "sentinel"
	if cli, ok := p.rdb.(*redis.Client); ok {
		snap.Redis.Master = cli.Options().Addr
	} else {
		snap.Redis.Master = p.cfg.Get().Redis.Sentinel.MasterName
	}
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

	// LLM / Embedding：reachabilityProber（Ping + gobreaker 状态）异步探测；
	// 失败只记 WARN 不阻塞，结果写入快照（health 接口只读快照，毫秒级返回）。
	if err := probeProviderErr(providerCtx, p.llmCli); err != nil {
		snap.LLM.Reachable = false
		p.logger.Warn("llm provider probe failed", zap.Error(err))
	} else {
		snap.LLM.Reachable = true
	}
	snap.LLM.Breaker = breakerStateOf(p.llmCli)
	if err := probeProviderErr(providerCtx, p.emb); err != nil {
		snap.Embedding.Reachable = false
		p.logger.Warn("embedding provider probe failed", zap.Error(err))
	} else {
		snap.Embedding.Reachable = true
	}
	snap.Embedding.Breaker = breakerStateOf(p.emb)
	snap.Embedding.Dimensions = p.emb.Dimensions()
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

func probeProviderErr(ctx context.Context, provider any) error {
	pr, ok := provider.(reachabilityProber)
	if !ok {
		return fmt.Errorf("provider does not implement reachabilityProber")
	}
	return pr.Ping(ctx)
}

func breakerStateOf(provider any) string {
	if pr, ok := provider.(reachabilityProber); ok {
		return pr.BreakerState()
	}
	return "closed"
}

// buildRetrievers 检索三件套装配（keyword=OpenSearch BM25、embedding=pgvector HNSW、hybrid=RRF 融合）。
// 全部经 RerankRetriever 装饰：粗筛 topK*4 → LLM 重排 → 截断 topK（重排失败自动降级原序）。
func buildRetrievers(searchCli *search.Client, chunks store.ChunkStore, vectors store.VectorStore, pool *pgxpool.Pool, emb embed.Embedder, llmClient llm.LLM, cfg *config.Config, logger *zap.Logger) map[string]retriever.Retriever {
	kw := retriever.NewKeywordRetriever(searchCli, chunks, logger)
	vec := retriever.NewVectorRetriever(pool, emb, cfg.Storage.Vector.EFSearch, logger)
	hyb := retriever.NewHybridRetriever(kw, vec, cfg.Retriever.RRFK, logger)
	// hybrid 外再包 Multi-Query（改写 3 路并行召回合并）与 rerank（合并后重排一次）。
	mq := retriever.NewMultiQueryRetriever(hyb, llmClient, logger)
	reranker := retriever.NewLLMReranker(llmClient)
	return map[string]retriever.Retriever{
		"keyword":   retriever.NewRerankRetriever(kw, logger).WithReranker(reranker),
		"embedding": retriever.NewRerankRetriever(vec, logger).WithReranker(reranker),
		"hybrid":    retriever.NewRerankRetriever(mq, logger).WithReranker(reranker),
	}
}

// verifyIndices 启动一致性校验（总纲 §4.2）：每仓 count(index) == chunks 子块行数
//（父子块双层索引后 OpenSearch 只装子块；父块仅供上下文不参与对账），
// 不一致 → WARN 并后台重建该仓索引（本函数在独立 goroutine 中运行，重建随校验内联完成）。
func verifyIndices(ctx context.Context, repos store.RepoStore, chunks store.ChunkStore, searchCli *search.Client, pool *pgxpool.Pool, logger *zap.Logger) {
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
		// 对账口径 = 子块行数（parent_chunk_id IS NOT NULL），与 OpenSearch 装块口径一致。
		var want int64
		if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM chunks WHERE repo_id = $1 AND parent_chunk_id IS NOT NULL`, repoID).Scan(&want); err != nil {
			logger.Error("verify indices: count chunks failed", zap.String("repo_id", repoID), zap.Error(err))
			continue
		}
		got, err := searchCli.Count(ctx, repoID)
		if err != nil {
			logger.Error("verify indices: count index failed", zap.String("repo_id", repoID), zap.Error(err))
			continue
		}
		if got != want {
			logger.Warn("verify indices: mismatch, rebuilding",
				zap.String("repo_id", repoID), zap.Int64("postgres", want), zap.Int64("opensearch", got))
			if err := rebuildIndex(ctx, pool, searchCli, repoID); err != nil {
				logger.Error("verify indices: rebuild failed", zap.String("repo_id", repoID), zap.Error(err))
				continue
			}
			logger.Info("verify indices: rebuilt", zap.String("repo_id", repoID), zap.Int64("docs", want))
		}
	}
	logger.Info("opensearch indices verified", zap.Int("repos", len(repoIDs)))
}

// rebuildIndex 全量重建单仓索引：删索引 → 建索引 → 从 chunks 表读子块（parent_chunk_id IS NOT NULL）→ bulk 重写（幂等 _id=chunk_id）。
func rebuildIndex(ctx context.Context, pool *pgxpool.Pool, searchCli *search.Client, repoID string) error {
	rows, err := pool.Query(ctx, `
		SELECT chunk_id, repo_id, path, start_line, end_line, language, content, parent_chunk_id
		FROM chunks WHERE repo_id = $1 AND parent_chunk_id IS NOT NULL
	`, repoID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var chunks []model.Chunk
	for rows.Next() {
		var c model.Chunk
		var parentID *string
		if err := rows.Scan(&c.ChunkID, &c.RepoID, &c.Path, &c.StartLine, &c.EndLine, &c.Language, &c.Content, &parentID); err != nil {
			return err
		}
		if parentID != nil {
			c.ParentChunkID = *parentID
		}
		chunks = append(chunks, c)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := searchCli.DeleteIndex(ctx, repoID); err != nil {
		return err
	}
	if err := searchCli.CreateIndex(ctx, repoID); err != nil {
		return err
	}
	return searchCli.BulkIndex(ctx, repoID, chunks)
}

// bootstrapAPIKeys 启动引导：把 DEEPWIKI_API_KEYS / DEEPWIKI_ADMIN_KEY 中的明文 key
// 以 SHA-256(salt‖key) 哈希后幂等 upsert 进 api_keys 表（salt 每 key 随机，总纲 R14）。
// 硬约束 #2：明文 key 只存在于本函数栈帧，禁止入日志/缓存/其他存储；运行期鉴权走哈希比对。
func bootstrapAPIKeys(ctx context.Context, auth config.AuthConfig, keys store.APIKeyStore, logger *zap.Logger) {
	upsertOne := func(key string, isAdmin bool) {
		if key == "" {
			return
		}
		saltBytes := make([]byte, 16)
		if _, err := rand.Read(saltBytes); err != nil {
			logger.Error("bootstrap api key: rand salt failed", zap.Error(err))
			return
		}
		salt := hex.EncodeToString(saltBytes)
		sum := sha256.Sum256([]byte(salt + key))
		k := &store.APIKey{
			KeyID:   middleware.NewULID("key_"),
			KeyHash: hex.EncodeToString(sum[:]),
			Salt:    salt,
			IsAdmin: isAdmin,
		}
		if err := keys.Upsert(ctx, k); err != nil {
			logger.Error("bootstrap api key failed", zap.Bool("is_admin", isAdmin), zap.Error(err))
		}
	}
	for _, k := range auth.APIKeys {
		upsertOne(k, false)
	}
	upsertOne(auth.AdminKey, true)
	if len(auth.APIKeys) > 0 || auth.AdminKey != "" {
		if n, err := keys.Count(ctx); err == nil {
			logger.Info("api keys bootstrapped", zap.Int64("active_keys", n))
		}
	}
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
