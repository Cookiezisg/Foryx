// Package model is the domain layer for LLM model strategy (per-scenario provider/modelID).
//
// Package model 是 LLM 模型策略 domain 层（按 scenario 记录 provider/modelID）。
package model

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ModelConfig records the user's (provider, modelID) for one scenario.
//
// ModelConfig 记录用户某 scenario 下的 (provider, modelID)。
type ModelConfig struct {
	ID        string         `gorm:"primaryKey;type:text" json:"id"`
	UserID    string         `gorm:"not null;type:text;uniqueIndex:idx_mc_user_scenario,priority:1" json:"-"`
	Scenario  string         `gorm:"not null;type:text;uniqueIndex:idx_mc_user_scenario,priority:2" json:"scenario"`
	Provider  string         `gorm:"not null;type:text" json:"provider"`
	ModelID   string         `gorm:"not null;type:text" json:"modelId"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (ModelConfig) TableName() string { return "model_configs" }

const (
	ScenarioChat       = "chat"
	ScenarioWebSummary = "web_summary"
)

func IsValidScenario(s string) bool {
	switch s {
	case ScenarioChat, ScenarioWebSummary:
		return true
	default:
		return false
	}
}

// ListScenarios returns every recognised scenario; backs the contract test (not used by production).
//
// ListScenarios 返所有合法 scenario，支撑契约测试，生产不调。
func ListScenarios() []string {
	return []string{ScenarioChat, ScenarioWebSummary}
}

var (
	ErrNotConfigured    = errors.New("model: not configured for scenario")
	ErrInvalidScenario  = errors.New("model: invalid scenario")
	ErrProviderRequired = errors.New("model: provider is required")
	ErrModelIDRequired  = errors.New("model: model id is required")
	ErrProviderHasNoKey = errors.New("model: provider has no api-key configured")
)

// Repository is the storage contract for ModelConfig, scoped by ctx userID.
//
// Repository 是 ModelConfig 存储契约，按 ctx userID 过滤。
type Repository interface {
	GetByScenario(ctx context.Context, scenario string) (*ModelConfig, error)
	List(ctx context.Context) ([]*ModelConfig, error)
	Upsert(ctx context.Context, m *ModelConfig) error
}

// ModelPicker is the cross-domain port for LLM-using services; implemented by app/model.Service.
//
// ModelPicker 是跨 domain 端口，由 app/model.Service 实现。
type ModelPicker interface {
	PickForChat(ctx context.Context) (provider, modelID string, err error)

	// PickForWebSummary returns the web-summary model; caller must fall back to PickForChat.
	//
	// PickForWebSummary 返 web-summary 配置；未配置时调用方必须 fallback 到 PickForChat。
	PickForWebSummary(ctx context.Context) (provider, modelID string, err error)
}
