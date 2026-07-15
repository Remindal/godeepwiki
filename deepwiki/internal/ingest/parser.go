package ingest

import (
	"context"
)

// ParseFiles 遍历 workDir，按 include_ext/exclude_dirs 过滤，产出 SourceFile 列表（基线 §12.4）。
func ParseFiles(ctx context.Context, workDir string, opts IngestOptions) ([]SourceFile, error) {
	// TODO: 实现解析，要求：
	// ① filepath.WalkDir 遍历，跳过 opts.ExcludeDirs 目录名（非路径匹配）；
	// ② 仅保留 opts.IncludeExt 中的扩展名（小写比较）；③ 每文件算 Hash = sha256(content)[:16]；
	// ④ 扩展名→语言映射（.go→go、.py→python、.md→markdown……），未知扩展名 Language="" 仍入库；
	// ⑤ 相对路径禁止 .. 与绝对路径（反 AI 错误 #11）；⑥ 文件循环内必须 select ctx.Done()（反 AI 错误 #4）；
	// ⑦ 文件级 include/exclude 与默认跳过清单统一走 FileFilter（ignore.go，go-gitignore 语义）。
	panic("TODO: ParseFiles not implemented")
}
