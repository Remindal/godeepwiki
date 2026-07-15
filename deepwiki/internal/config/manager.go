package config

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"

	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"
)

// MergeOverrides 将 etcd 覆写值深合并到引导配置（总纲 §4.5）。
// 骨架阶段：仅当 overrides 为空时返回 base；非空时暂不做合并（下一轮与 Apply 共用同一套合并语义）。
func MergeOverrides(base *Config, overrides map[string]json.RawMessage) *Config {
	if len(overrides) == 0 {
		return base
	}
	// TODO: 实现 dotted.key → 嵌套 map 的深合并，与 Manager.Apply 共用合并语义（硬约束 #9）。
	return base
}

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
