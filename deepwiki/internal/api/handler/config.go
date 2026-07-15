package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/config"
)

// ConfigHandler GET/PUT /api/v1/config（§6.5，建议⑭）。
type ConfigHandler struct {
	cm     *config.Manager
	logger *zap.Logger
}

func NewConfigHandler(cm *config.Manager, logger *zap.Logger) *ConfigHandler {
	return &ConfigHandler{cm: cm, logger: logger}
}

func (h *ConfigHandler) GetConfig(c *gin.Context) {
	// TODO: GET /api/v1/config：返回 dto.ConfigResponse{Version, Config: h.cm.Masked(), RestartRequired}；
	// 密钥字段必须脱敏（config.MaskAPIKey 规则），Auth 节与基础设施凭据不出现（json:"-"，硬约束 #2）；
	// etcd 不可用时读本地快照缓存，GET 路径不报错（总纲 §4.5）。
	respondNotImplemented(c)
}

func (h *ConfigHandler) UpdateConfig(c *gin.Context) {
	// TODO: PUT /api/v1/config（路由层已挂 AdminOnly）：
	// ① 读取 JSON Merge Patch 原文 → h.cm.Apply(ctx, patch, 脱敏后的 key 标识)；
	// ② 校验失败 → 42201 + details 字段级明细（整体拒绝保持旧值，审计写 etcd /deepwiki/audit/<version>
	//    result=rejected，硬约束 #9）；
	// ③ 成功 → dto.ConfigUpdateResponse{version, applied, restart_required, warnings}（审计 result=applied）；
	// ④ etcd 写路径不可用 → 503 + 50304 config_store_unavailable（总纲 §6 新增码）。
	respondNotImplemented(c)
}
