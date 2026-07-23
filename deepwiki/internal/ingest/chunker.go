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

// ChunkFiles 把 SourceFile 切分为 Chunk（基线 §12.4 切分策略）：
// 行对齐固定窗口，按行累加 token 至 chunk_size 切块，块间回退 chunk_overlap 对应行数重叠；
// Markdown 遇标题行（#~######）强制切块，块内仍按窗口兜底；一块一文件，不跨文件合并。
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
			chunks = append(chunks, model.Chunk{
				ChunkID:   newChunkID(),
				RepoID:    repoID,
				Path:      f.Path,
				StartLine: span[0] + 1,
				EndLine:   span[1],
				Language:  f.Language,
				Content:   strings.Join(lines[span[0]:span[1]], "\n"),
				FileHash:  f.Hash,
			})
		}
	}
	return chunks, nil
}

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
