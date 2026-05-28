// Package model is the domain layer for LLM model strategy (per-scenario apiKeyID/modelID).
//
// Package model 是 LLM 模型策略 domain 层（按 scenario 记录 apiKeyID/modelID）。
package model

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ModelRef is a stable (apiKeyId, modelId) pair reusable across domains
// (Conversation.ModelOverride, NodeSpec.ModelOverride). Provider is implicit
// via the api_key referenced by ApiKeyID.
//
// ModelRef 是可跨 domain 复用的 (apiKeyId, modelId) 对(conv 和 node 的 override 复用)。
// Provider 由 ApiKeyID 引用的 api_key 隐含。
type ModelRef struct {
	ApiKeyID string `json:"apiKeyId"`
	ModelID  string `json:"modelId"`
}

// ModelConfig records the user's (apiKeyId, modelId) for one scenario.
//
// ModelConfig 记录用户某 scenario 下的 (apiKeyId, modelId)。
type ModelConfig struct {
	ID        string         `gorm:"primaryKey;type:text" json:"id"`
	UserID    string         `gorm:"not null;type:text;uniqueIndex:idx_mc_user_scenario,priority:1" json:"-"`
	Scenario  string         `gorm:"not null;type:text;uniqueIndex:idx_mc_user_scenario,priority:2" json:"scenario"`
	ApiKeyID  string         `gorm:"not null;type:text;column:api_key_id" json:"apiKeyId"`
	ModelID   string         `gorm:"not null;type:text;column:model_id" json:"modelId"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (ModelConfig) TableName() string { return "model_configs" }

const (
	ScenarioDialogue = "dialogue"
	ScenarioUtility  = "utility"
	ScenarioAgent    = "agent"
)

// IsValidScenario reports whether s is a recognised scenario name.
//
// IsValidScenario 报告 s 是否为合法 scenario 名。
func IsValidScenario(s string) bool {
	switch s {
	case ScenarioDialogue, ScenarioUtility, ScenarioAgent:
		return true
	default:
		return false
	}
}

// ListScenarios returns every recognised scenario in canonical order.
//
// ListScenarios 按规范顺序返回所有合法 scenario。
func ListScenarios() []string {
	return []string{ScenarioDialogue, ScenarioUtility, ScenarioAgent}
}

var (
	ErrNotConfigured    = errors.New("model: not configured for scenario")
	ErrInvalidScenario  = errors.New("model: invalid scenario")
	ErrApiKeyIDRequired = errors.New("model: api_key_id is required")
	ErrModelIDRequired  = errors.New("model: model id is required")
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
// Returns (apiKeyID, modelID) — provider is derived later from apikey.ResolveCredentialsByID.
//
// ModelPicker 是跨 domain 端口,由 app/model.Service 实现。
// 返回 (apiKeyID, modelID)——provider 由 apikey.ResolveCredentialsByID 在解析阶段拿到。
type ModelPicker interface {
	PickForDialogue(ctx context.Context) (apiKeyID, modelID string, err error)
	PickForUtility(ctx context.Context) (apiKeyID, modelID string, err error)
	PickForAgent(ctx context.Context) (apiKeyID, modelID string, err error)
}
