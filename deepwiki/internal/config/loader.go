package config

import (
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Load 读取 yaml 引导配置 + 环境变量注入（总纲 §4.5 加载顺序第一层；etcd 覆写由 EtcdSource 叠加）。
// 密钥与基础设施凭据只从环境变量读取（硬约束 #2），yaml 中出现明文密钥/凭据时校验应拒绝启动。
// 环境变量清单（总纲 §5.3，逐字一致）：
//
//	DEEPWIKI_API_KEYS / DEEPWIKI_ADMIN_KEY / DEEPWIKI_EMBEDDING_API_KEY / DEEPWIKI_LLM_API_KEY（保留）
//	DEEPWIKI_POSTGRES_DSN / DEEPWIKI_OPENSEARCH_USERNAME / DEEPWIKI_OPENSEARCH_PASSWORD /
//	DEEPWIKI_RABBITMQ_URL / DEEPWIKI_REDIS_SENTINEL_ADDRESSES / DEEPWIKI_REDIS_PASSWORD /
//	DEEPWIKI_ETCD_ENDPOINTS（新增）
func Load(configPath string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(configPath)
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	// 环境变量注入（yaml 不落明文，硬约束 #2）
	cfg.Auth.APIKeys = SplitCSV(os.Getenv("DEEPWIKI_API_KEYS"))
	cfg.Auth.AdminKey = os.Getenv("DEEPWIKI_ADMIN_KEY")
	cfg.Embedding.APIKey = os.Getenv("DEEPWIKI_EMBEDDING_API_KEY")
	cfg.LLM.APIKey = os.Getenv("DEEPWIKI_LLM_API_KEY")
	cfg.Storage.Postgres.DSN = os.Getenv("DEEPWIKI_POSTGRES_DSN")
	cfg.Search.OpenSearch.Username = os.Getenv("DEEPWIKI_OPENSEARCH_USERNAME")
	cfg.Search.OpenSearch.Password = os.Getenv("DEEPWIKI_OPENSEARCH_PASSWORD")
	if addrs := os.Getenv("OPENSEARCH_ADDRESSES"); addrs != "" {
		cfg.Search.OpenSearch.Addresses = SplitCSV(addrs)
	}
	cfg.Queue.RabbitMQ.URL = os.Getenv("DEEPWIKI_RABBITMQ_URL")
	cfg.Redis.Sentinel.Addresses = SplitCSV(os.Getenv("DEEPWIKI_REDIS_SENTINEL_ADDRESSES"))
	cfg.Redis.Password = os.Getenv("DEEPWIKI_REDIS_PASSWORD")
	if endpoints := os.Getenv("ETCD_ENDPOINTS"); endpoints != "" {
		cfg.Etcd.Endpoints = SplitCSV(endpoints)
	}
	// queue.rabbitmq.prefetch 缺省取 worker.pool_size（总纲 §5.2）
	if cfg.Queue.RabbitMQ.Prefetch <= 0 {
		cfg.Queue.RabbitMQ.Prefetch = cfg.Worker.PoolSize
	}
	return &cfg, nil
}

func SplitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// MaskAPIKey 脱敏规则（§6.5，冻结）：长度 > 8 → 前 3 字符 + "***" + 后 4 字符；否则全 "******"。
func MaskAPIKey(key string) string {
	if len(key) > 8 {
		return key[:3] + "***" + key[len(key)-4:]
	}
	return "******"
}
