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
