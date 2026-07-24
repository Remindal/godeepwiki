package model

// Chunk 代码/文档分块领域模型（基线 §7）。
// Path 为仓库内相对路径，禁止 .. 与绝对路径（反 AI 错误 #11）。
// Vector 在检索路径中可不填充（按 chunk_id 懒加载）；落库为 chunks.embedding vector(1024) 列，
// 维度不符会被列类型直接拒绝（反 AI 错误 #14 第二道防线）。
type Chunk struct {
	ChunkID        string
	RepoID         string
	Path           string
	StartLine      int
	EndLine        int
	Language       string
	Content        string
	Vector         []float32
	FileHash       string // 所属文件 sha256 前 16 位，refresh diff 用
	EmbeddingModel string // 产出向量的 provider/model，维度一致性校验用（反 AI 错误 #14）
}

// ChunkHit 检索命中。Score 语义：keyword=BM25 分；embedding=余弦相似度 [0,1]；hybrid=RRF 融合分。
type ChunkHit struct {
	Chunk Chunk
	Score float64
}
