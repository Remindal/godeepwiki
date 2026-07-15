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
)

// EtcdSource etcd 配置源。
type EtcdSource struct {
	cli    *clientv3.Client
	prefix string
	logger *zap.Logger
}

// NewEtcdSource 建立 etcd client。
func NewEtcdSource(ctx context.Context, endpoints []string, prefix string, logger *zap.Logger) (*EtcdSource, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 10 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("etcd new client: %w", err)
	}
	return &EtcdSource{cli: cli, prefix: strings.TrimSuffix(prefix, "/"), logger: logger}, nil
}

// LoadAll 加载所有覆盖配置与版本号。
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

// ApplyTxn 原子写入覆盖配置并递增版本号。
func (s *EtcdSource) ApplyTxn(ctx context.Context, changes map[string]json.RawMessage) error {
	panic("TODO: EtcdSource.ApplyTxn not implemented")
}

// Watch 返回配置前缀的 watch 通道。
func (s *EtcdSource) Watch(ctx context.Context) clientv3.WatchChan {
	return s.cli.Watch(ctx, s.prefix+suffixConfig, clientv3.WithPrefix())
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
