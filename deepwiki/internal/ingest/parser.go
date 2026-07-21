package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// extToLanguage 扩展名（小写、含点）→ 语言标识；未知扩展名 Language 留空仍入库。
var extToLanguage = map[string]string{
	".go":    "go",
	".py":    "python",
	".js":    "javascript",
	".jsx":   "javascript",
	".ts":    "typescript",
	".tsx":   "typescript",
	".md":    "markdown",
	".java":  "java",
	".c":     "c",
	".h":     "c",
	".cc":    "cpp",
	".cpp":   "cpp",
	".hpp":   "cpp",
	".cs":    "csharp",
	".rs":    "rust",
	".rb":    "ruby",
	".php":   "php",
	".swift": "swift",
	".kt":    "kotlin",
	".kts":   "kotlin",
	".scala": "scala",
	".sh":    "shell",
	".bash":  "shell",
	".zsh":   "shell",
	".sql":   "sql",
	".json":  "json",
	".yaml":  "yaml",
	".yml":   "yaml",
	".toml":  "toml",
	".xml":   "xml",
	".html":  "html",
	".css":   "css",
	".scss":  "scss",
	".vue":   "vue",
	".lua":   "lua",
	".dart":  "dart",
	".txt":   "text",
}

// ParseFiles 遍历 workDir，按 include_ext/exclude_dirs 过滤，产出 SourceFile 列表（基线 §12.4）。
// 目录按名字整体跳过（DefaultSkipDirs ∪ opts.ExcludeDirs）；文件经 FileFilter 做
// 二进制/超大/路径穿越过滤；IncludeExt 为空表示不过滤扩展名。
func ParseFiles(ctx context.Context, workDir string, opts IngestOptions) ([]SourceFile, error) {
	workDir, err := validatePath(workDir)
	if err != nil {
		return nil, err
	}
	filter := NewFileFilter(nil, nil, 0)

	excludeDirs := make(map[string]struct{}, len(opts.ExcludeDirs))
	for _, d := range opts.ExcludeDirs {
		excludeDirs[d] = struct{}{}
	}
	includeExt := make(map[string]struct{}, len(opts.IncludeExt))
	for _, e := range opts.IncludeExt {
		includeExt[strings.ToLower(e)] = struct{}{}
	}

	var files []SourceFile
	err = filepath.WalkDir(workDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			if path != workDir {
				name := d.Name()
				if filter.SkipDir(name) {
					return filepath.SkipDir
				}
				if _, ok := excludeDirs[name]; ok {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(d.Name()))
		if len(includeExt) > 0 {
			if _, ok := includeExt[ext]; !ok {
				return nil
			}
		}

		rel, err := filepath.Rel(workDir, path)
		if err != nil {
			return err
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return fmt.Errorf("invalid relative path: %q", rel)
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if filter.SkipFile(rel, info.Size(), isBinary(content)) {
			return nil
		}

		sum := sha256.Sum256(content)
		files = append(files, SourceFile{
			Path:     filepath.ToSlash(rel),
			Language: extToLanguage[ext],
			Content:  string(content),
			Hash:     hex.EncodeToString(sum[:])[:16],
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse files: %w", err)
	}
	return files, nil
}

// isBinary 以首 8KB 内是否含 NUL 字节判定二进制（与 git 的启发式一致）。
func isBinary(content []byte) bool {
	n := len(content)
	if n > 8192 {
		n = 8192
	}
	for i := 0; i < n; i++ {
		if content[i] == 0 {
			return true
		}
	}
	return false
}
