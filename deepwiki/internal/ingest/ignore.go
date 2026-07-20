package ingest

import (
	"path/filepath"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"
)

// DefaultSkipDirs 默认跳过的目录名（非路径匹配；无论 include/exclude 如何配置都生效）。
var DefaultSkipDirs = []string{".git", "vendor", "node_modules"}

// DefaultBinaryExcludePatterns 默认按扩展名跳过的二进制/非文本文件（.gitignore 语法）。
var DefaultBinaryExcludePatterns = []string{
	"*.exe", "*.dll", "*.so", "*.bin",
	"*.jpg", "*.jpeg", "*.png", "*.gif", "*.bmp", "*.svg",
	"*.zip", "*.tar.gz", "*.tar", "*.gz", "*.bz2", "*.7z", "*.rar",
	"*.pdf", "*.mp3", "*.mp4", "*.avi", "*.mov", "*.webm",
}

// DefaultMaxFileSize 默认跳过的单文件大小上限（1 MiB；超出按超大文件跳过，避免内存与分块失真）。
const DefaultMaxFileSize int64 = 1 << 20

// FileFilter include/exclude 规则匹配器（go-gitignore，.gitignore 语义）。
// include 为空表示全部允许；exclude 命中即拒绝；优先级：DefaultSkipDirs > exclude > include。
type FileFilter struct {
	include     *ignore.GitIgnore // nil 表示未配置 include（全部允许）
	exclude     *ignore.GitIgnore // nil 表示未配置 exclude
	skipDirs    []string
	maxFileSize int64
}

// NewFileFilter includePatterns / excludePatterns 为空切片时对应规则不生效；
// maxFileSize ≤ 0 取 DefaultMaxFileSize。
func NewFileFilter(includePatterns, excludePatterns []string, maxFileSize int64) *FileFilter {
	if maxFileSize <= 0 {
		maxFileSize = DefaultMaxFileSize
	}
	var inc *ignore.GitIgnore
	if len(includePatterns) > 0 {
		inc = ignore.CompileIgnoreLines(includePatterns...)
	}
	excPatterns := append([]string{}, DefaultBinaryExcludePatterns...)
	excPatterns = append(excPatterns, excludePatterns...)
	var exc *ignore.GitIgnore
	if len(excPatterns) > 0 {
		exc = ignore.CompileIgnoreLines(excPatterns...)
	}
	return &FileFilter{
		include:     inc,
		exclude:     exc,
		skipDirs:    append([]string{}, DefaultSkipDirs...),
		maxFileSize: maxFileSize,
	}
}

// SkipDir 判断目录名（非路径）是否应整体跳过：DefaultSkipDirs ∪ 用户 ExcludeDirs。
func (f *FileFilter) SkipDir(dirName string) bool {
	for _, d := range f.skipDirs {
		if d == dirName {
			return true
		}
	}
	return false
}

// SkipFile 判断仓库内相对路径是否应跳过。
// ① relPath 先 filepath.Clean，含 ".." 或绝对路径一律跳过（硬约束 #11）；
// ② 路径中包含 DefaultSkipDirs 目录 → true；
// ③ 二进制文件（isBinary=true）→ true；④ size > maxFileSize → true；
// ⑤ exclude 命中 → true；⑥ include 非 nil 且未命中 → true。
func (f *FileFilter) SkipFile(relPath string, size int64, isBinary bool) bool {
	clean := filepath.Clean(relPath)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
		return true
	}
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		if f.SkipDir(part) {
			return true
		}
	}
	if isBinary {
		return true
	}
	if size > f.maxFileSize {
		return true
	}
	if f.exclude != nil && f.exclude.MatchesPath(clean) {
		return true
	}
	if f.include != nil && !f.include.MatchesPath(clean) {
		return true
	}
	return false
}
