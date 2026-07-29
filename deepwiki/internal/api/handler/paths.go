package handler

import (
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"deepwiki/internal/model"
)

// PathExists GET /api/v1/repos/{repo_id}/paths/exists?prefix=<p>：
// 校验路径前缀在仓库中是否存在文件（前端 path_filter 存在性提示用）。
// 返回 {prefix, valid, exists, reason}：valid=格式是否合法（复用 ask 校验规则），
// exists=仓库内是否有文件路径以该前缀开头（FileHashes 全量路径前缀匹配）。
func (h *ChunkHandler) PathExists(c *gin.Context) {
	repoID := c.Param("repo_id")
	if !validRepoID(repoID) {
		respondError(c, model.CodeInvalidParam, model.MessageOf(model.CodeInvalidParam), invalidRepoIDDetail("repo_id"))
		return
	}
	prefix := c.Query("prefix")

	if details := validatePathFilter(prefix); len(details) > 0 {
		respondOK(c, gin.H{"prefix": prefix, "valid": false, "exists": false, "reason": details[0].Issue})
		return
	}

	hashes, err := h.chunks.FileHashes(c.Request.Context(), repoID)
	if err != nil {
		h.logger.Error("path exists: file hashes failed", zap.String("repo_id", repoID), zap.Error(err))
		respondError(c, model.CodeInternalError, model.MessageOf(model.CodeInternalError), nil)
		return
	}

	exists := false
	if prefix != "" {
		for p := range hashes {
			if strings.HasPrefix(p, prefix) {
				exists = true
				break
			}
		}
	} else {
		exists = true // 空前缀 = 全仓，恒存在
	}

	reason := ""
	if !exists {
		reason = "仓库中不存在该路径"
	}
	respondOK(c, gin.H{"prefix": prefix, "valid": true, "exists": exists, "reason": reason})
}
