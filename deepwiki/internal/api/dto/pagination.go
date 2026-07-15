package dto

// Pagination 分页元信息（排序固定 created_at DESC）。
type Pagination struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}

// PageResult 分页响应 data：{ items, pagination }；越界返回空 items 与真实 total，不报错。
type PageResult[T any] struct {
	Items      []T        `json:"items"`
	Pagination Pagination `json:"pagination"`
}
