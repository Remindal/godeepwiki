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
