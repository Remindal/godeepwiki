package handler

import (
	"regexp"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/model"
	"deepwiki/internal/store"
)

var chunkIDPattern = regexp.MustCompile(`^chk_[0-9A-HJKMNP-TV-Z]{26}$`)

// ChunkHandler GET /api/v1/chunks/{chunk_id}：按 ID 取代码块全文（前端引用查看器数据源）。
type ChunkHandler struct {
	chunks store.ChunkStore
	logger *zap.Logger
}

func NewChunkHandler(chunks store.ChunkStore, logger *zap.Logger) *ChunkHandler {
	return &ChunkHandler{chunks: chunks, logger: logger}
}

func (h *ChunkHandler) Get(c *gin.Context) {
	chunkID := c.Param("chunk_id")
	if !chunkIDPattern.MatchString(chunkID) {
		respondError(c, model.CodeInvalidParam, model.MessageOf(model.CodeInvalidParam),
			[]model.ErrorDetail{{Field: "chunk_id", Issue: "must match ^chk_[0-9A-HJKMNP-TV-Z]{26}$"}})
		return
	}
	chunk, err := h.chunks.GetByID(c.Request.Context(), chunkID)
	if err != nil {
		h.logger.Error("get chunk failed", zap.String("chunk_id", chunkID), zap.Error(err))
		respondError(c, model.CodeRepoNotFound, "chunk not found", nil)
		return
	}
	respondOK(c, chunk)
}