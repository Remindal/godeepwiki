// Package config 配置系统：支持 file 与 etcd 配置源。
// 总纲 §4.2：etcd 配置源用于运行时覆盖与动态刷新。
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

const (
	suffixConfig       = "/config/"
	suffixConfigVersion = "/config_version"
	suffixAudit        = "/audit/"
)

// EtcdSource etcd 配置源。
type EtcdSource struct {
	cli    *clientv3.Client
	prefix string
	logger *zap.Logger
}

// NewEtcdSource 建立 etcd client；连接失败直接 panic（启动失败优于带病运行，总纲 §4.5）。
func NewEtcdSource(ctx context.Context, endpoints []string, prefix string, logger *zap.Logger) (*EtcdSource, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 10 * time.Second,
	})
	if err != nil {
		panic(fmt.Sprintf("etcd new client failed: %v", err))
	}
	return &EtcdSource{cli: cli, prefix: strings.TrimSuffix(prefix, "/"), logger: logger}, nil
}

// LoadAll 加载所有覆盖配置与版本号；键不存在返回空 map（GET /config 走 yaml 默认值）。
func (s *EtcdSource) LoadAll(ctx context.Context) (map[string]json.RawMessage, int64, error) {
	resp, err := s.cli.Get(ctx, s.prefix+suffixConfig, clientv3.WithPrefix())
	if err != nil {
		return nil, 0, fmt.Errorf("etcd load config: %w", err)
	}
	overrides := make(map[string]json.RawMessage)
	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		if strings.HasPrefix(key, s.prefix+suffixConfig) {
			overrides[strings.TrimPrefix(key, s.prefix+suffixConfig)] = kv.Value
		}
	}
	ver, err := s.getVersion(ctx)
	if err != nil {
		return nil, 0, err
	}
	return overrides, ver, nil
}

func (s *EtcdSource) getVersion(ctx context.Context) (int64, error) {
	resp, err := s.cli.Get(ctx, s.prefix+suffixConfigVersion)
	if err != nil {
		return 0, fmt.Errorf("etcd load version: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return 1, nil
	}
	v, err := strconv.ParseInt(string(resp.Kvs[0].Value), 10, 64)
	if err != nil {
		return 1, nil
	}
	return v, nil
}

// auditEntry etcd 审计记录（/deepwiki/audit/<version>）。
type auditEntry struct {
	ChangedBy    string                 `json:"changedBy"`
	Change       map[string]any         `json:"change"`
	Result       string                 `json:"result"`
	RejectReason string                 `json:"rejectReason,omitempty"`
	At           string                 `json:"at"`
}

// ApplyTxn 原子写入覆盖配置、递增版本号并记录审计。
func (s *EtcdSource) ApplyTxn(ctx context.Context, changes map[string]json.RawMessage, audit auditEntry) (int64, error) {
	verResp, err := s.cli.Get(ctx, s.prefix+suffixConfigVersion)
	if err != nil {
		return 0, fmt.Errorf("etcd get version: %w", err)
	}
	var currentVer int64
	var currentModRev int64
	if len(verResp.Kvs) > 0 {
		currentModRev = verResp.Kvs[0].ModRevision
		currentVer, _ = strconv.ParseInt(string(verResp.Kvs[0].Value), 10, 64)
	}
	newVersion := currentVer + 1

	audit.At = time.Now().UTC().Format(time.RFC3339)
	auditJSON, err := json.Marshal(audit)
	if err != nil {
		return 0, fmt.Errorf("marshal audit: %w", err)
	}

	ops := make([]clientv3.Op, 0, len(changes)+2)
	for k, v := range changes {
		ops = append(ops, clientv3.OpPut(s.prefix+suffixConfig+k, string(v)))
	}
	ops = append(ops,
		clientv3.OpPut(s.prefix+suffixConfigVersion, strconv.FormatInt(newVersion, 10)),
		clientv3.OpPut(s.prefix+suffixAudit+strconv.FormatInt(newVersion, 10), string(auditJSON)),
	)

	txnResp, err := s.cli.Txn(ctx).
		If(clientv3.Compare(clientv3.ModRevision(s.prefix+suffixConfigVersion), "=", currentModRev)).
		Then(ops...).
		Commit()
	if err != nil {
		return 0, fmt.Errorf("etcd apply txn: %w", err)
	}
	if !txnResp.Succeeded {
		return 0, fmt.Errorf("etcd apply txn: version conflict")
	}
	return newVersion, nil
}

// Watch 监听 /deepwiki/config/ 前缀；断流或健康检查失败后 5s 重连，通过 onChange 回调通知订阅者。
func (s *EtcdSource) Watch(ctx context.Context, onChange func()) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		watchCtx, cancel := context.WithCancel(ctx)
		ch := s.cli.Watch(watchCtx, s.prefix+suffixConfig, clientv3.WithPrefix(), clientv3.WithProgressNotify())
		s.logger.Info("etcd watch established", zap.String("prefix", s.prefix+suffixConfig))
		active := true
		for active {
			select {
			case <-ctx.Done():
				active = false
			case <-ticker.C:
				if !s.Healthy(ctx) {
					s.logger.Warn("etcd unhealthy, recreating watch")
					active = false
				}
			case wr, ok := <-ch:
				if !ok {
					active = false
					break
				}
				if wr.Err() != nil {
					s.logger.Error("etcd watch error", zap.Error(wr.Err()))
					active = false
					break
				}
				changed := false
				for _, ev := range wr.Events {
					if ev.Type == clientv3.EventTypePut || ev.Type == clientv3.EventTypeDelete {
						changed = true
						break
					}
				}
				if changed {
					onChange()
				}
			}
		}
		cancel()
		if ctx.Err() != nil {
			return
		}
		s.logger.Warn("etcd watch disconnected, reconnecting", zap.Duration("backoff", 5*time.Second))
		time.Sleep(5 * time.Second)
	}
}

// Healthy 检查 etcd 连通性。
func (s *EtcdSource) Healthy(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := s.cli.Get(ctx, s.prefix+suffixConfigVersion)
	return err == nil
}

// Endpoints 返回 etcd 端点。
func (s *EtcdSource) Endpoints() []string { return s.cli.Endpoints() }

// Close 关闭 etcd 连接。
func (s *EtcdSource) Close() error { return s.cli.Close() }
