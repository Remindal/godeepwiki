package model

import (
	"errors"
	"fmt"
)

// ---------------- 错误码（基线 §5.3 冻结 16 个 + 变更总纲 §6 新增 4 个 = 固定 20 个，不得增删） ----------------

const (
	CodeInvalidParam           = 40001 // invalid_param
	CodeUnauthorized           = 40101 // unauthorized
	CodeForbidden              = 40301 // forbidden
	CodeTaskNotFound           = 40401 // task_not_found
	CodeRepoNotFound           = 40402 // repo_not_found
	CodeWikiNotFound           = 40403 // wiki_not_found
	CodeRepoAlreadyExists      = 40901 // repo_already_exists
	CodeInvalidTaskState       = 40902 // invalid_task_state
	CodeConfigValidationFailed = 42201 // config_validation_failed
	CodeRateLimited            = 42901 // rate_limited
	CodeQueueFull              = 42902 // queue_full
	CodeInternalError          = 50001 // internal_error
	CodeTaskInterrupted        = 50004 // task_interrupted（仅出现在 task.error，不直接作为 API 响应码）
	CodeLLMUnavailable         = 50201 // llm_unavailable
	CodeEmbeddingUnavailable   = 50202 // embedding_unavailable
	CodeServiceNotReady        = 50301 // service_not_ready
	// ---- v2 新增（变更总纲 §6，基础设施错误码）----
	CodeVectorStoreUnavailable  = 50203 // vector_store_unavailable：Postgres/pgvector 查询失败（ask embedding 路径）
	CodeQueueUnavailable        = 50302 // queue_unavailable：RabbitMQ 连接/发布确认失败
	CodeSearchUnavailable       = 50303 // search_unavailable：OpenSearch 不可用且影响 ask
	CodeConfigStoreUnavailable  = 50304 // config_store_unavailable：etcd 写路径不可用（GET 走缓存不报错）
)

// 错误码名称（message 前缀用，如 "invalid_param: field question length must be between 1 and 4000"）。
const (
	ErrNameInvalidParam           = "invalid_param"
	ErrNameUnauthorized           = "unauthorized"
	ErrNameForbidden              = "forbidden"
	ErrNameTaskNotFound           = "task_not_found"
	ErrNameRepoNotFound           = "repo_not_found"
	ErrNameWikiNotFound           = "wiki_not_found"
	ErrNameRepoAlreadyExists      = "repo_already_exists"
	ErrNameInvalidTaskState       = "invalid_task_state"
	ErrNameConfigValidationFailed = "config_validation_failed"
	ErrNameRateLimited            = "rate_limited"
	ErrNameQueueFull              = "queue_full"
	ErrNameInternalError          = "internal_error"
	ErrNameTaskInterrupted        = "task_interrupted"
	ErrNameLLMUnavailable         = "llm_unavailable"
	ErrNameEmbeddingUnavailable   = "embedding_unavailable"
	ErrNameServiceNotReady        = "service_not_ready"
	// ---- v2 新增 ----
	ErrNameVectorStoreUnavailable = "vector_store_unavailable"
	ErrNameQueueUnavailable       = "queue_unavailable"
	ErrNameSearchUnavailable      = "search_unavailable"
	ErrNameConfigStoreUnavailable = "config_store_unavailable"
)

// HTTPStatusOf 错误码 → HTTP 状态码映射（基线 §5.3 + 变更总纲 §6）。
func HTTPStatusOf(code int) int {
	switch code {
	case CodeInvalidParam:
		return 400
	case CodeUnauthorized:
		return 401
	case CodeForbidden:
		return 403
	case CodeTaskNotFound, CodeRepoNotFound, CodeWikiNotFound:
		return 404
	case CodeRepoAlreadyExists, CodeInvalidTaskState:
		return 409
	case CodeConfigValidationFailed:
		return 422
	case CodeRateLimited, CodeQueueFull:
		return 429
	case CodeLLMUnavailable, CodeEmbeddingUnavailable, CodeVectorStoreUnavailable:
		return 502
	case CodeServiceNotReady, CodeQueueUnavailable, CodeSearchUnavailable, CodeConfigStoreUnavailable:
		return 503
	default:
		return 500
	}
}

// MessageOf 错误码默认文案（脱敏固定文案；禁止向客户端回传 err.Error() 原文，反 AI 错误 #8）。
// v1 冻结 16 个保持英文固定文案；v2 新增 4 个按变更总纲 §6 message 列逐字使用中文文案。
func MessageOf(code int) string {
	switch code {
	case CodeInvalidParam:
		return "invalid param"
	case CodeUnauthorized:
		return "unauthorized"
	case CodeForbidden:
		return "forbidden"
	case CodeTaskNotFound:
		return "task not found"
	case CodeRepoNotFound:
		return "repo not found"
	case CodeWikiNotFound:
		return "wiki not found"
	case CodeRepoAlreadyExists:
		return "repo already exists"
	case CodeInvalidTaskState:
		return "invalid task state"
	case CodeConfigValidationFailed:
		return "config validation failed"
	case CodeRateLimited:
		return "rate limited"
	case CodeQueueFull:
		return "queue full"
	case CodeTaskInterrupted:
		return "task interrupted"
	case CodeLLMUnavailable:
		return "llm unavailable"
	case CodeEmbeddingUnavailable:
		return "embedding unavailable"
	case CodeServiceNotReady:
		return "service not ready"
	case CodeQueueUnavailable:
		return "任务队列暂不可用，请稍后重试"
	case CodeSearchUnavailable:
		return "检索服务暂不可用"
	case CodeConfigStoreUnavailable:
		return "配置中心暂不可用"
	case CodeVectorStoreUnavailable:
		return "向量检索暂不可用"
	default:
		return "internal error"
	}
}

// ---------------- 统一响应信封（基线 §5.2，冻结） ----------------

// ErrorDetail 字段级明细；ExistingRepoID 仅 40901 幂等命中时使用。
type ErrorDetail struct {
	Field          string `json:"field"`
	Issue          string `json:"issue"`
	ExistingRepoID string `json:"existing_repo_id,omitempty"`
}

// Envelope 统一响应信封。成功：code=0,message="ok",data 非空；失败：code!=0,details 可选。
type Envelope struct {
	Code      int           `json:"code"`
	Message   string        `json:"message"`
	Data      any           `json:"data,omitempty"`
	RequestID string        `json:"request_id"`
	Details   []ErrorDetail `json:"details,omitempty"`
}

// APIError 业务错误；Handler 层据此装配信封。Err 为原始错误，只进 zap 日志，禁止回传。
type APIError struct {
	Code    int
	Message string
	Details []ErrorDetail
	Err     error
}

func (e *APIError) Error() string { return fmt.Sprintf("api error %d: %s", e.Code, e.Message) }
func (e *APIError) Unwrap() error { return e.Err }

// NewAPIError 用默认文案构造（message 可传空串取默认）。
func NewAPIError(code int, message string) *APIError {
	if message == "" {
		message = MessageOf(code)
	}
	return &APIError{Code: code, Message: message}
}

// ---------------- 哨兵错误（跨层传递，Handler 映射为错误码） ----------------

var (
	ErrQueueFull         = errors.New("queue full")          // → 42902
	ErrInvalidTaskState  = errors.New("invalid task state")  // → 40902
	ErrTaskNotFound      = errors.New("task not found")      // → 40401
	ErrRepoNotFound      = errors.New("repo not found")      // → 40402
	ErrWikiNotFound      = errors.New("wiki not found")      // → 40403
	ErrRepoAlreadyExists = errors.New("repo already exists") // → 40901
	// ---- v2 新增（与新错误码配套）----
	ErrVectorStoreUnavailable = errors.New("vector store unavailable")  // → 50203
	ErrQueueUnavailable       = errors.New("queue unavailable")         // → 50302
	ErrSearchUnavailable      = errors.New("search unavailable")        // → 50303
	ErrConfigStoreUnavailable = errors.New("config store unavailable")  // → 50304
)
