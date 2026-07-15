package service

import (
	"context"

	"go.uber.org/zap"

	"deepwiki/internal/api/dto"
	"deepwiki/internal/config"
	"deepwiki/internal/llm"
	"deepwiki/internal/model"
	"deepwiki/internal/retriever"
)

// AskService 问答编排（§6.2、§6.3；Ask 不得直接访问 pgvector/OpenSearch，只面向 Retriever 接口，建议⑥——
// keyword 实现走 OpenSearch BM25（总纲 §4.2），embedding 实现走 pgvector HNSW（总纲 §4.1），hybrid 为 RRF 融合）。
type AskService struct {
	cfg        *config.Manager
	retrievers map[string]retriever.Retriever // key = mode：keyword|embedding|hybrid
	llm        llm.LLM
	logger     *zap.Logger
}

func NewAskService(cfg *config.Manager, retrievers map[string]retriever.Retriever, l llm.LLM, logger *zap.Logger) *AskService {
	return &AskService{cfg: cfg, retrievers: retrievers, llm: l, logger: logger}
}

// Ask POST /api/v1/ask（§6.2）。
func (s *AskService) Ask(ctx context.Context, req dto.AskRequest) (*dto.AskResponse, error) {
	_ = model.CodeLLMUnavailable
	// TODO: 实现非流式问答，要求：
	// ① 仓库须 state=ready，否则 40902；② mode 缺省取 retriever.mode 配置，top_k 缺省取 retriever.top_k（1~30），
	//    temperature 省略取 llm.temperature 且钳制 [0,2]；
	// ③ 按 mode 选 Retriever.Search 取 hits（topK）；vector 路径先 Embed 查询向量再走 pgvector <=> 余弦距离
	//    （SET LOCAL hnsw.ef_search 取 storage.vector.ef_search，默认 64）；
	// ④ 组装 prompt：system 明确"只能依据给定片段作答，引用格式 [path:start-end]，禁止编造行号与文件路径，
	//    片段不足时回答'未在仓库中找到相关代码'"（硬约束 #15）；
	// ⑤ llm.Generate 生成回答；references 就是 Retriever 返回的 hits（禁止从 LLM 输出解析），
	//    响应装配前逐一校验 chunk_id 存在（硬约束 #15）；
	// ⑥ usage 来自 provider；缺失时按 tokens≈ceil(len([]rune(content))/4) 估算兜底（§12.4）；
	// ⑦ 错误映射：LLM 不可用 → 50201；Embedding 不可用 → 50202；OpenSearch 不可用 → 50303 search_unavailable；
	//    pgvector 查询失败 → 50203 vector_store_unavailable（总纲 §6 新增码）；mode 回显实际生效值。
	panic("TODO: AskService.Ask not implemented")
}

// AskStream POST /api/v1/ask/stream 的业务实现（§6.3；sink 由 handler 提供，负责 SSE 帧写出）。
func (s *AskService) AskStream(ctx context.Context, req dto.AskRequest, sink func(event string, payload any) error) error {
	// TODO: 实现流式问答，要求：
	// ① 事件顺序：references（恰好 1 个）→ token（0~N 个）→ done（恰好 1 个）；error 可出现在任意位置并终止流；
	// ② 检索完成立即 sink("references", ...)；llm.GenerateStream 的每个 delta → sink("token", {delta})；
	// ③ 结束 sink("done", {usage, latency_ms})；provider 不返回 usage 时估算兜底；
	// ④ 客户端断开 → ctx 取消 → LLM 流中断、goroutine 退出，不补偿不重放（§6.3）；
	// ⑤ 其余约束与错误映射同 Ask ③~⑦。
	panic("TODO: AskService.AskStream not implemented")
}
