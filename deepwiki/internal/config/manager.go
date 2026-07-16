package config

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// MergeOverrides 将 etcd 覆写值深合并到引导配置（总纲 §4.5）。
func MergeOverrides(base *Config, overrides map[string]json.RawMessage) *Config {
	if len(overrides) == 0 {
		return base
	}
	baseMap := configToMap(base)
	for dottedKey, raw := range overrides {
		var value interface{}
		if err := json.Unmarshal(raw, &value); err != nil {
			continue
		}
		setDotted(baseMap, dottedKey, value)
	}
	return mapToConfig(baseMap)
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
	base     *Config
	snapshot atomic.Value // *Config
	version  atomic.Int64
	src      *EtcdSource
	validate *validator.Validate
	mu       sync.Mutex
	subs     []func(*Config)
	logger   *zap.Logger
}

func NewManager(base *Config, overrides map[string]json.RawMessage, version int64, src *EtcdSource, logger *zap.Logger) *Manager {
	m := &Manager{
		base:     base,
		src:      src,
		validate: validator.New(),
		logger:   logger,
	}
	m.version.Store(version)
	m.snapshot.Store(MergeOverrides(base, overrides))
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

func (m *Manager) notify(cfg *Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, fn := range m.subs {
		func(f func(*Config)) {
			defer func() {
				if r := recover(); r != nil {
					m.logger.Error("config subscriber panic", zap.Any("panic", r))
				}
			}()
			f(cfg)
		}(fn)
	}
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
	m.logger.Info("etcd watch established", zap.String("prefix", m.src.prefix+suffixConfig))
	m.src.Watch(ctx, func() {
		if err := m.reload(ctx); err != nil {
			m.logger.Error("config reload from etcd failed", zap.Error(err))
		}
	})
}

func (m *Manager) reload(ctx context.Context) error {
	overrides, ver, err := m.src.LoadAll(ctx)
	if err != nil {
		return err
	}
	cfg := MergeOverrides(m.base, overrides)
	if err := m.validateStruct(cfg); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	m.version.Store(ver)
	m.snapshot.Store(cfg)
	m.notify(cfg)
	m.logger.Info("config reloaded from etcd", zap.Int64("version", ver))
	return nil
}

// Apply JSON Merge Patch 语义部分更新（§6.5）。
func (m *Manager) Apply(ctx context.Context, patch json.RawMessage, changedBy string) (*ApplyResult, error) {
	// etcd 不可用则直接拒绝写路径（GET 仍走快照缓存）。
	if m.src == nil || !m.src.Healthy(ctx) {
		return nil, model.NewAPIError(model.CodeConfigStoreUnavailable, "")
	}

	var patchMap map[string]interface{}
	if err := json.Unmarshal(patch, &patchMap); err != nil {
		return nil, model.NewAPIError(model.CodeInvalidParam, "patch must be a JSON object")
	}
	if len(patchMap) == 0 {
		return nil, model.NewAPIError(model.CodeInvalidParam, "patch is empty")
	}

	// 禁止明文密钥通过 PUT 写入（硬约束 #2 扩展）。
	if secrets := findSecretKeys(patchMap, ""); len(secrets) > 0 {
		details := make([]model.ErrorDetail, 0, len(secrets))
		for _, field := range secrets {
			details = append(details, model.ErrorDetail{Field: field, Issue: "api_key can only be set via environment variable"})
		}
		_ = m.writeAudit(ctx, changedBy, nil, "rejected", "secret key in patch", 0)
		return nil, &model.APIError{Code: model.CodeInvalidParam, Message: model.MessageOf(model.CodeInvalidParam), Details: details}
	}

	// 1) 深合并当前快照与 Merge Patch。
	currentMap := configToMap(m.Get())
	mergePatch(currentMap, patchMap)
	candidate := mapToConfig(currentMap)

	// 2) 全量校验：validator tag + 跨字段规则。
	details := m.validateWithDetails(candidate)
	restartRequired := m.restartRequiredKeys(patchMap)
	if len(details) > 0 {
		_ = m.writeAudit(ctx, changedBy, patchMap, "rejected", "validation failed", 0)
		return nil, &model.APIError{Code: model.CodeConfigValidationFailed, Message: model.MessageOf(model.CodeConfigValidationFailed), Details: details}
	}

	// TODO(轮次 3): embedding 维度探测。provider/model/base_url 变更且库中有 chunks 时，
	// 以新配置 Embed(["dimension probe"]) 比对 dimensions；不一致或探测失败 → 拒绝并提示重建索引。
	warnings := []string{}
	if embeddingChanged(m.Get().Embedding, candidate.Embedding) {
		warnings = append(warnings, "embedding provider changed, existing index may need rebuild")
	}

	// 3) 持久化到 etcd（覆盖项按 dotted key 扁平化存储）。
	changes := flattenPatch(patchMap)
	applied := make(map[string]any, len(changes))
	for k, raw := range changes {
		var v any
		_ = json.Unmarshal(raw, &v)
		applied[k] = v
	}
	newVersion, err := m.src.ApplyTxn(ctx, changes, auditEntry{
		ChangedBy: changedBy,
		Change:    applied,
		Result:    "applied",
	})
	if err != nil {
		m.logger.Error("etcd apply txn failed", zap.Error(err))
		return nil, model.NewAPIError(model.CodeConfigStoreUnavailable, "")
	}

	// 4) 原子替换快照并通知订阅者。
	m.version.Store(newVersion)
	m.snapshot.Store(candidate)
	m.notify(candidate)

	if restartRequired == nil {
		restartRequired = []string{}
	}
	if warnings == nil {
		warnings = []string{}
	}
	return &ApplyResult{
		Version:         newVersion,
		Applied:         applied,
		RestartRequired: restartRequired,
		Warnings:        warnings,
	}, nil
}

func (m *Manager) validateStruct(cfg *Config) error {
	return m.validate.Struct(cfg)
}

func (m *Manager) validateWithDetails(cfg *Config) []model.ErrorDetail {
	var details []model.ErrorDetail
	if err := m.validate.Struct(cfg); err != nil {
		if verr, ok := err.(validator.ValidationErrors); ok {
			for _, fe := range verr {
				details = append(details, model.ErrorDetail{
					Field: fe.Namespace(),
					Issue: fmt.Sprintf("failed %s (%s)", fe.Tag(), fe.Param()),
				})
			}
		} else {
			details = append(details, model.ErrorDetail{Field: "", Issue: err.Error()})
		}
	}
	if cfg.Ingest.ChunkOverlap > cfg.Ingest.ChunkSize/2 {
		details = append(details, model.ErrorDetail{
			Field: "ingest.chunk_overlap",
			Issue: fmt.Sprintf("chunk_overlap(%d) must be <= chunk_size/2(%d)", cfg.Ingest.ChunkOverlap, cfg.Ingest.ChunkSize/2),
		})
	}
	if cfg.RateLimit.PerIP.Burst < cfg.RateLimit.PerIP.RPS {
		details = append(details, model.ErrorDetail{
			Field: "rate_limit.per_ip",
			Issue: "burst must be >= rps",
		})
	}
	return details
}

func (m *Manager) restartRequiredKeys(patch map[string]interface{}) []string {
	set := make(map[string]struct{}, len(RestartRequiredKeys))
	for _, k := range RestartRequiredKeys {
		set[k] = struct{}{}
	}
	var out []string
	for k := range flattenPatch(patch) {
		if _, ok := set[k]; ok {
			out = append(out, k)
		}
	}
	return out
}

func (m *Manager) writeAudit(ctx context.Context, changedBy string, change map[string]interface{}, result, reason string, version int64) error {
	var changes map[string]any
	if change != nil {
		changes = make(map[string]any, len(change))
		for k, v := range change {
			changes[k] = v
		}
	}
	_, err := m.src.ApplyTxn(ctx, nil, auditEntry{
		ChangedBy:    changedBy,
		Change:       changes,
		Result:       result,
		RejectReason: reason,
	})
	return err
}

// ---------- 深合并与键处理工具 ----------

func configToMap(cfg *Config) map[string]interface{} {
	b, _ := json.Marshal(cfg)
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	return m
}

func mapToConfig(m map[string]interface{}) *Config {
	b, _ := json.Marshal(m)
	var cfg Config
	_ = json.Unmarshal(b, &cfg)
	return &cfg
}

func mergePatch(dst, src map[string]interface{}) {
	for k, sv := range src {
		if sv == nil {
			delete(dst, k)
			continue
		}
		if smap, ok := sv.(map[string]interface{}); ok {
			dmap, ok := dst[k].(map[string]interface{})
			if !ok {
				dmap = make(map[string]interface{})
				dst[k] = dmap
			}
			mergePatch(dmap, smap)
		} else {
			dst[k] = sv
		}
	}
}

func setDotted(m map[string]interface{}, key string, value interface{}) {
	parts := strings.Split(key, ".")
	cur := m
	for i, p := range parts {
		if i == len(parts)-1 {
			cur[p] = value
			return
		}
		next, ok := cur[p].(map[string]interface{})
		if !ok {
			next = make(map[string]interface{})
			cur[p] = next
		}
		cur = next
	}
}

func flattenPatch(m map[string]interface{}) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage)
	flattenPatchRec(m, "", out)
	return out
}

func flattenPatchRec(m map[string]interface{}, prefix string, out map[string]json.RawMessage) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if vm, ok := v.(map[string]interface{}); ok && len(vm) > 0 {
			flattenPatchRec(vm, key, out)
		} else {
			raw, _ := json.Marshal(v)
			out[key] = raw
		}
	}
}

func findSecretKeys(m map[string]interface{}, prefix string) []string {
	var out []string
	for k, v := range m {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		if path == "embedding.api_key" || path == "llm.api_key" {
			out = append(out, path)
			continue
		}
		if vm, ok := v.(map[string]interface{}); ok {
			out = append(out, findSecretKeys(vm, path)...)
		}
	}
	return out
}

func embeddingChanged(old, new EmbeddingConfig) bool {
	return old.Provider != new.Provider || old.Model != new.Model || old.BaseURL != new.BaseURL
}

// 抑制 "imported and not used"：reflect 目前未使用，但保留便于后续订阅者反射更新；如不需要可删。
var _ = reflect.DeepEqual
