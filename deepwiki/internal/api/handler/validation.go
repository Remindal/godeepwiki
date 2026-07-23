package handler

import (
	"regexp"
	"strings"

	"deepwiki/internal/api/dto"
	"deepwiki/internal/model"
)

var (
	repoIDPattern = regexp.MustCompile(`^repo_[0-9A-HJKMNP-TV-Z]{26}$`)
	taskIDPattern = regexp.MustCompile(`^tsk_[0-9A-HJKMNP-TV-Z]{26}$`)
)

func validRepoID(id string) bool { return repoIDPattern.MatchString(id) }
func validTaskID(id string) bool { return taskIDPattern.MatchString(id) }

func invalidRepoIDDetail(field string) []model.ErrorDetail {
	return []model.ErrorDetail{{Field: field, Issue: "must match ^repo_[0-9A-HJKMNP-TV-Z]{26}$"}}
}

func invalidTaskIDDetail(field string) []model.ErrorDetail {
	return []model.ErrorDetail{{Field: field, Issue: "must match ^tsk_[0-9A-HJKMNP-TV-Z]{26}$"}}
}

// validateRepoURL 校验 git 仓库地址（§6.1）：≤512，拒绝 file:// 等本地协议。
func validateRepoURL(url string) []model.ErrorDetail {
	var details []model.ErrorDetail
	if url == "" {
		return []model.ErrorDetail{{Field: "repo_url", Issue: "required"}}
	}
	if len(url) > 512 {
		details = append(details, model.ErrorDetail{Field: "repo_url", Issue: "length must be <= 512"})
	}
	lower := strings.ToLower(url)
	if strings.HasPrefix(lower, "file://") || strings.HasPrefix(lower, "/") || strings.Contains(lower, ":\\") {
		details = append(details, model.ErrorDetail{Field: "repo_url", Issue: "local file protocol is not allowed"})
	}
	if !strings.Contains(url, "://") && !strings.HasPrefix(url, "git@") {
		details = append(details, model.ErrorDetail{Field: "repo_url", Issue: "must be a valid git URL"})
	}
	return details
}

// validateBranch 校验分支名（§6.1）：≤128，禁止 ..、空白与 ~^:?*[\ 等 git ref 非法字符。
func validateBranch(branch string) []model.ErrorDetail {
	if branch == "" {
		return nil
	}
	var details []model.ErrorDetail
	if len(branch) > 128 {
		details = append(details, model.ErrorDetail{Field: "branch", Issue: "length must be <= 128"})
	}
	if strings.Contains(branch, "..") || strings.ContainsAny(branch, " \t\n\r~^:?*[\\") {
		details = append(details, model.ErrorDetail{Field: "branch", Issue: "contains invalid git ref characters"})
	}
	return details
}

// validateIngestOptions 校验 options 字段（§6.1）。
func validateIngestOptions(opts *dto.IngestOptionsDTO) []model.ErrorDetail {
	if opts == nil {
		return nil
	}
	var details []model.ErrorDetail
	if opts.ChunkSize != nil && *opts.ChunkSize < 100 {
		details = append(details, model.ErrorDetail{Field: "options.chunk_size", Issue: "must be >= 100"})
	}
	if opts.ChunkOverlap != nil && *opts.ChunkOverlap < 0 {
		details = append(details, model.ErrorDetail{Field: "options.chunk_overlap", Issue: "must be >= 0"})
	}
	if opts.ChunkSize != nil && opts.ChunkOverlap != nil && *opts.ChunkOverlap > *opts.ChunkSize/2 {
		details = append(details, model.ErrorDetail{Field: "options.chunk_overlap", Issue: "must be <= chunk_size/2"})
	}
	if opts.IncludeExt != nil && len(opts.IncludeExt) == 0 {
		details = append(details, model.ErrorDetail{Field: "options.include_ext", Issue: "must not be empty when provided"})
	}
	return details
}
