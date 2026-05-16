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

	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Service orchestrates model-config CRUD and implements modeldomain.ModelPicker.
//
// Service 编排模型配置 CRUD，并实现 modeldomain.ModelPicker。
type Service struct {
	repo modeldomain.Repository
	log  *zap.Logger
}

// NewService wires Service dependencies; panics on nil logger.
//
// NewService 装配依赖；nil logger 直接 panic。
func NewService(repo modeldomain.Repository, log *zap.Logger) *Service {
	if log == nil {
		panic("model.NewService: logger is nil")
	}
	return &Service{repo: repo, log: log}
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
