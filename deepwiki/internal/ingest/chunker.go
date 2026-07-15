package ingest

import (
	"context"

	"deepwiki/internal/model"
)

// ChunkFiles 把 SourceFile 切分为 Chunk（基线 §12.4 切分策略）。
func ChunkFiles(ctx context.Context, repoID string, files []SourceFile, opts IngestOptions) ([]model.Chunk, error) {
	// TODO: 实现切分，要求：
	// ① 行对齐固定窗口：按行累加至 chunk_size（目标 token 数，tokens≈ceil(len([]rune(content))/4)）切块，
	//    块间回退 chunk_overlap 对应行数重叠；② Markdown 优先按标题层级切，再按窗口兜底；
	// ③ 一块一文件，保 path/start_line/end_line 精确；④ ChunkID = "chk_" + ULID；FileHash/EmbeddingModel 必填；
	// ⑤ 文件循环内必须 select ctx.Done()（反 AI 错误 #4）。
	panic("TODO: ChunkFiles not implemented")
}
