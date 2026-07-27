package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// staticSPA 托管前端构建产物（web/dist，长期使用形态：前后端同端口 8080）。
// 静态文件存在则直出；否则回退 index.html（Vue Router history 模式的 SPA 回退）。
// /api/ 前缀不匹配时返回 JSON 404，不回退 index.html（防止 API 404 被 SPA 吞掉）。
func staticSPA(dir string) gin.HandlerFunc {
	index := filepath.Join(dir, "index.html")
	return func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "not found"})
			return
		}
		// filepath.Clean("/"+p) 以 / 开头，Join 后必然落在 dir 内，天然防穿越（反 AI 错误 #11）。
		full := filepath.Join(dir, filepath.Clean("/"+p))
		if st, err := os.Stat(full); err == nil && !st.IsDir() {
			c.File(full)
			return
		}
		c.File(index)
	}
}
