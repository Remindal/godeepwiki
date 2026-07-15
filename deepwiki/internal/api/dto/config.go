package dto

import "deepwiki/internal/config"

// ConfigResponse GET /api/v1/config 200 响应 data（Config 为脱敏副本，§6.5）。
type ConfigResponse struct {
	Version         int64         `json:"version"`
	Config          config.Config `json:"config"`
	RestartRequired []string      `json:"restart_required"`
}

// ConfigUpdateResponse PUT /api/v1/config 200 响应 data。
type ConfigUpdateResponse struct {
	Version         int64          `json:"version"`
	Applied         map[string]any `json:"applied"`
	RestartRequired []string       `json:"restart_required"`
	Warnings        []string       `json:"warnings"`
}
