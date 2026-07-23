package handler

import (
	"io"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/api/dto"
	"deepwiki/internal/config"
	"deepwiki/internal/model"
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
	res := dto.ConfigResponse{
		Version:         h.cm.Version(),
		Config:          *h.cm.Masked(),
		RestartRequired: []string{},
	}
	respondOK(c, res)
}

func (h *ConfigHandler) UpdateConfig(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		respondError(c, model.CodeInvalidParam, model.MessageOf(model.CodeInvalidParam), nil)
		return
	}
	if len(body) == 0 {
		respondError(c, model.CodeInvalidParam, "patch body is empty", nil)
		return
	}
	// changedBy 取请求标识；生产环境可进一步从 auth key 取脱敏 ID。
	changedBy := c.Request.RemoteAddr
	result, err := h.cm.Apply(c.Request.Context(), body, changedBy)
	if err != nil {
		if apiErr, ok := err.(*model.APIError); ok {
			respondError(c, apiErr.Code, apiErr.Message, apiErr.Details)
			return
		}
		respondError(c, model.CodeInternalError, model.MessageOf(model.CodeInternalError), nil)
		return
	}
	respondOK(c, dto.ConfigUpdateResponse{
		Version:         result.Version,
		Applied:         result.Applied,
		RestartRequired: result.RestartRequired,
		Warnings:        result.Warnings,
	})
}
