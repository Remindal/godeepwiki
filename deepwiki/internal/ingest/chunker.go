package ingest

import (
	"context"
	crand "crypto/rand"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"deepwiki/internal/model"
)

// EstimateTokens 粗估 token 数：英文代码经验值 ≈ 字符数 / 4（向上取整）。
func EstimateTokens(s string) int {
	runes := len([]rune(s))
	return (runes + 3) / 4
}

// ChunkFiles 把 SourceFile 切分为两层 Chunk（父子块双层索引）：
// 父块 = 现有行对齐固定窗口（chunk_size，完整上下文单元，不嵌向量，仅供 LLM 上下文）；
// 子块 = 父块内再切 ~300 token 小窗（40 token 重叠，嵌向量用于检索，parent_chunk_id 指回父块）。
// 产出顺序：每个父块后紧跟其子块（持久化与索引按 ParentChunkID 区分）。
func ChunkFiles(ctx context.Context, repoID string, files []SourceFile, opts IngestOptions) ([]model.Chunk, error) {
	chunkSize := opts.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 500
	}
	chunkOverlap := opts.ChunkOverlap
	if chunkOverlap < 0 {
		chunkOverlap = 0
	}
	if chunkOverlap >= chunkSize {
		chunkOverlap = chunkSize / 2
	}

	var chunks []model.Chunk
	for _, f := range files {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if f.Content == "" {
			continue
		}
		lines := strings.Split(f.Content, "\n")
		var boundary func(line string) bool
		if f.Language == "markdown" {
			boundary = isMarkdownHeading
		}
		for _, span := range splitWindows(lines, chunkSize, chunkOverlap, boundary) {
			parent := model.Chunk{
				ChunkID:   newChunkID(),
				RepoID:    repoID,
				Path:      f.Path,
				StartLine: span[0] + 1,
				EndLine:   span[1],
				Language:  f.Language,
				Content:   strings.Join(lines[span[0]:span[1]], "\n"),
				FileHash:  f.Hash,
			}
			chunks = append(chunks, parent)
			// 子块：父块内 ~300 token 小窗（40 重叠），用于向量检索，上下文回查父块。
			parentLines := lines[span[0]:span[1]]
			for _, cspan := range splitWindows(parentLines, childWindowTokens, childOverlapTokens, nil) {
				chunks = append(chunks, model.Chunk{
					ChunkID:       newChunkID(),
					RepoID:        repoID,
					Path:          f.Path,
					StartLine:     parent.StartLine + cspan[0],
					EndLine:       parent.StartLine + cspan[1] - 1,
					Language:      f.Language,
					Content:       strings.Join(parentLines[cspan[0]:cspan[1]], "\n"),
					FileHash:      f.Hash,
					ParentChunkID: parent.ChunkID,
				})
			}
		}
	}
	return chunks, nil
}

const (
	// childWindowTokens 子块窗口（~300 token，检索粒度）。
	childWindowTokens = 300
	// childOverlapTokens 子块重叠（保持句意连续）。
	childOverlapTokens = 40
)

// splitWindows 行窗口切分，返回 [start, end) 行区间序列。
// boundary(line) 为 true 时该行强制成为新块首行（Markdown 标题）；重叠回退不越过强制边界。
func splitWindows(lines []string, chunkSize, chunkOverlap int, boundary func(line string) bool) [][2]int {
	lineTokens := make([]int, len(lines))
	for i, l := range lines {
		lineTokens[i] = EstimateTokens(l)
	}

	var spans [][2]int
	start := 0
	for start < len(lines) {
		tokens := 0
		end := start
		stoppedAtBoundary := false
		for end < len(lines) {
			if end > start && boundary != nil && boundary(lines[end]) {
				stoppedAtBoundary = true
				break
			}
			tokens += lineTokens[end]
			end++
			if tokens >= chunkSize {
				break
			}
		}
		spans = append(spans, [2]int{start, end})
		if end >= len(lines) {
			break
		}
		if stoppedAtBoundary {
			start = end
			continue
		}

		next := end
		acc := 0
		for next > start && acc < chunkOverlap {
			next--
			acc += lineTokens[next]
			if boundary != nil && boundary(lines[next]) {
				break
			}
		}
		if next <= start {
			next = end
		}
		start = next
	}
	return spans
}

// isMarkdownHeading 判定 ATX 标题行（1~6 个 # 后跟空格或行尾）。
func isMarkdownHeading(line string) bool {
	i := 0
	for i < len(line) && line[i] == '#' {
		i++
	}
	if i == 0 || i > 6 {
		return false
	}
	return i == len(line) || line[i] == ' '
}

func newChunkID() string {
	return "chk_" + ulid.MustNew(ulid.Timestamp(time.Now().UTC()), ulid.Monotonic(crand.Reader, 0)).String()
}
