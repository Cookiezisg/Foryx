// Package model owns the Service for model-config CRUD and ModelPicker.
//
// Package model 提供模型配置 CRUD 与 ModelPicker 实现。
package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Service orchestrates model-config CRUD and implements modeldomain.ModelPicker.
//
// Service 编排模型配置 CRUD,并实现 modeldomain.ModelPicker。
type Service struct {
	repo modeldomain.Repository
	keys apikeydomain.KeyProvider
	log  *zap.Logger
}

// NewService wires Service dependencies; panics on nil logger.
// keys is the cross-domain port used by Upsert F1 to verify the referenced
// api_key exists and belongs to the current user (avoids dangling refs).
//
// NewService 装配依赖；nil logger 直接 panic。keys 是跨 domain 端口,
// Upsert F1 校验引用的 api_key 存在且归属当前 user(防悬垂引用)。
func NewService(repo modeldomain.Repository, keys apikeydomain.KeyProvider, log *zap.Logger) *Service {
	if log == nil {
		panic("model.NewService: logger is nil")
	}
	return &Service{repo: repo, keys: keys, log: log}
}

// UpsertInput is the validated payload for Service.Upsert.
//
// UpsertInput 是 Service.Upsert 的已校验载荷。
type UpsertInput struct {
	APIKeyID string
	ModelID  string
	Thinking *modeldomain.ThinkingSpec
}

var _ modeldomain.ModelPicker = (*Service)(nil)

// List returns all active model configs for the current user.
//
// List 返回当前用户的所有活跃模型配置。
func (s *Service) List(ctx context.Context) ([]*modeldomain.ModelConfig, error) {
	return s.repo.List(ctx)
}

// GetByScenario returns the config for one scenario; ErrInvalidScenario for
// bad name, ErrNotConfigured for unconfigured.
//
// GetByScenario 返指定 scenario 的配置；非法 scenario 返 ErrInvalidScenario,
// 未配置返 ErrNotConfigured。
func (s *Service) GetByScenario(ctx context.Context, scenario string) (*modeldomain.ModelConfig, error) {
	if !modeldomain.IsValidScenario(scenario) {
		return nil, modeldomain.ErrInvalidScenario
	}
	return s.repo.GetByScenario(ctx, scenario)
}

// Upsert creates or updates the config for the given scenario.
//
// Upsert 为指定 scenario 创建或更新配置。
func (s *Service) Upsert(ctx context.Context, scenario string, in UpsertInput) (*modeldomain.ModelConfig, error) {
	if !modeldomain.IsValidScenario(scenario) {
		return nil, modeldomain.ErrInvalidScenario
	}
	if strings.TrimSpace(in.APIKeyID) == "" {
		return nil, modeldomain.ErrAPIKeyIDRequired
	}
	if strings.TrimSpace(in.ModelID) == "" {
		return nil, modeldomain.ErrModelIDRequired
	}
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, fmt.Errorf("model.Service.Upsert: %w", err)
	}
	// F1: api_key must exist and belong to current user.
	//
	// F1 校验:api_key 必须存在且属当前 user。
	if s.keys != nil {
		if _, err := s.keys.ResolveCredentialsByID(ctx, strings.TrimSpace(in.APIKeyID)); err != nil {
			return nil, fmt.Errorf("model.Service.Upsert: %w", err)
		}
	}
	m, err := s.repo.GetByScenario(ctx, scenario)
	if err != nil && !errors.Is(err, modeldomain.ErrNotConfigured) {
		return nil, err
	}
	if errors.Is(err, modeldomain.ErrNotConfigured) {
		m = &modeldomain.ModelConfig{
			ID:       newID(),
			UserID:   uid,
			Scenario: scenario,
		}
	}
	m.APIKeyID = strings.TrimSpace(in.APIKeyID)
	m.ModelID = strings.TrimSpace(in.ModelID)
	m.Thinking = in.Thinking
	if err := s.repo.Upsert(ctx, m); err != nil {
		return nil, err
	}
	s.log.Info("model config upserted",
		zap.String("user_id", uid),
		zap.String("scenario", scenario),
		zap.String("api_key_id", m.APIKeyID),
		zap.String("model_id", m.ModelID))
	return m, nil
}

// PickForDialogue returns the (apiKeyID, modelID, thinking) for the dialogue scenario.
//
// PickForDialogue 返回 dialogue scenario 的 (apiKeyID, modelID, thinking),未配置返 ErrNotConfigured。
func (s *Service) PickForDialogue(ctx context.Context) (apiKeyID, modelID string, thinking *modeldomain.ThinkingSpec, err error) {
	m, err := s.repo.GetByScenario(ctx, modeldomain.ScenarioDialogue)
	if err != nil {
		return "", "", nil, err
	}
	return m.APIKeyID, m.ModelID, m.Thinking, nil
}

// PickForUtility returns the (apiKeyID, modelID, thinking) for the utility scenario.
//
// PickForUtility 返回 utility scenario 的 (apiKeyID, modelID, thinking),未配置返 ErrNotConfigured。
func (s *Service) PickForUtility(ctx context.Context) (apiKeyID, modelID string, thinking *modeldomain.ThinkingSpec, err error) {
	m, err := s.repo.GetByScenario(ctx, modeldomain.ScenarioUtility)
	if err != nil {
		return "", "", nil, err
	}
	return m.APIKeyID, m.ModelID, m.Thinking, nil
}

// PickForAgent returns the (apiKeyID, modelID, thinking) for the agent scenario.
//
// PickForAgent 返回 agent scenario 的 (apiKeyID, modelID, thinking),未配置返 ErrNotConfigured。
func (s *Service) PickForAgent(ctx context.Context) (apiKeyID, modelID string, thinking *modeldomain.ThinkingSpec, err error) {
	m, err := s.repo.GetByScenario(ctx, modeldomain.ScenarioAgent)
	if err != nil {
		return "", "", nil, err
	}
	return m.APIKeyID, m.ModelID, m.Thinking, nil
}

func newID() string { return idgenpkg.New("mc") }
