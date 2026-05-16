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
// Service 编排模型配置 CRUD，并实现 modeldomain.ModelPicker。
type Service struct {
	repo modeldomain.Repository
	keys apikeydomain.KeyProvider
	log  *zap.Logger
}

// NewService wires Service dependencies; panics on nil logger.
// keys is the cross-domain port used to verify a provider has at least one
// api-key before Upsert accepts the config (avoids green-save / red-runtime UX).
//
// NewService 装配依赖；nil logger 直接 panic。keys 是跨 domain 端口，
// Upsert 时校验 provider 已有 api-key（防止"绿保存红运行时"反模式）。
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
	Provider string
	ModelID  string
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
	if strings.TrimSpace(in.Provider) == "" {
		return nil, modeldomain.ErrProviderRequired
	}
	if strings.TrimSpace(in.ModelID) == "" {
		return nil, modeldomain.ErrModelIDRequired
	}
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, fmt.Errorf("model.Service.Upsert: %w", err)
	}
	if s.keys != nil {
		has, hkErr := s.keys.HasKeyForProvider(ctx, strings.TrimSpace(in.Provider))
		if hkErr != nil {
			return nil, fmt.Errorf("model.Service.Upsert: %w", hkErr)
		}
		if !has {
			return nil, modeldomain.ErrProviderHasNoKey
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
	m.Provider = strings.TrimSpace(in.Provider)
	m.ModelID = strings.TrimSpace(in.ModelID)
	if err := s.repo.Upsert(ctx, m); err != nil {
		return nil, err
	}
	s.log.Info("model config upserted",
		zap.String("user_id", uid),
		zap.String("scenario", scenario),
		zap.String("provider", m.Provider),
		zap.String("model_id", m.ModelID))
	return m, nil
}

// PickForChat returns the (provider, modelID) for the chat scenario.
//
// PickForChat 返回 chat scenario 的 (provider, modelID)，未配置返 ErrNotConfigured。
func (s *Service) PickForChat(ctx context.Context) (provider, modelID string, err error) {
	m, err := s.repo.GetByScenario(ctx, modeldomain.ScenarioChat)
	if err != nil {
		return "", "", err
	}
	return m.Provider, m.ModelID, nil
}

// PickForWebSummary returns the (provider, modelID) for WebFetch summarisation.
//
// PickForWebSummary 返回 WebFetch 摘要的 (provider, modelID)，未配置时调用方应 fallback。
func (s *Service) PickForWebSummary(ctx context.Context) (provider, modelID string, err error) {
	m, err := s.repo.GetByScenario(ctx, modeldomain.ScenarioWebSummary)
	if err != nil {
		return "", "", err
	}
	return m.Provider, m.ModelID, nil
}

func newID() string { return idgenpkg.New("mc") }
