package ingest

import (
	ignore "github.com/sabhiram/go-gitignore"
)

// DefaultSkipDirs 默认跳过的目录名（非路径匹配；无论 include/exclude 如何配置都生效）。
var DefaultSkipDirs = []string{".git", "vendor", "node_modules"}

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
	// TODO: 用 ignore.CompileIgnoreLines(lines...) 分别编译 include/exclude 规则；
	// 空规则列表时对应字段保持 nil；maxFileSize ≤ 0 取 DefaultMaxFileSize。
	panic("TODO: NewFileFilter not implemented")
}

// SkipDir 判断目录名（非路径）是否应整体跳过：DefaultSkipDirs ∪ 用户 ExcludeDirs。
func (f *FileFilter) SkipDir(dirName string) bool {
	// TODO: 目录名命中 f.skipDirs 即返回 true（硬约束 #11：只按目录名比较，不做路径匹配，避免误伤同名深层目录）。
	panic("TODO: FileFilter.SkipDir not implemented")
}

// SkipFile 判断仓库内相对路径是否应跳过，要求：
// TODO: ① relPath 先 filepath.Clean，含 ".." 或绝对路径一律跳过（硬约束 #11）；
// ② exclude 命中（MatchesPath）→ true；include 非 nil 且未命中 → true；
// ③ size > f.maxFileSize → true（超大文件）；④ 二进制文件（调用方探测后传 isBinary=true）→ true。
func (f *FileFilter) SkipFile(relPath string, size int64, isBinary bool) bool {
	panic("TODO: FileFilter.SkipFile not implemented")
}
