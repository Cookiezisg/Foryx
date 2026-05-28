// Package model is the GORM-backed modeldomain.Repository, scoped by ctx userID.
//
// Package model 是 modeldomain.Repository 的 GORM 实现，按 ctx userID 过滤。
package model

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Store is the GORM implementation of modeldomain.Repository.
//
// Store 是 modeldomain.Repository 的 GORM 实现。
type Store struct {
	db *gorm.DB
}

// New constructs a Store bound to the given *gorm.DB.
//
// New 基于给定 *gorm.DB 构造 Store。
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// GetByScenario fetches the active config for (current user, scenario); ErrNotConfigured on miss.
//
// GetByScenario 返 (当前用户, scenario) 的活跃配置；无则 ErrNotConfigured。
func (s *Store) GetByScenario(ctx context.Context, scenario string) (*modeldomain.ModelConfig, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var m modeldomain.ModelConfig
	err = s.db.WithContext(ctx).
		Where("user_id = ? AND scenario = ?", uid, scenario).
		First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, modeldomain.ErrNotConfigured
	}
	if err != nil {
		return nil, fmt.Errorf("modelstore.GetByScenario: %w", err)
	}
	return &m, nil
}

// List returns all active configs for the current user, ordered by scenario.
//
// List 返回当前用户所有活跃配置，按 scenario 排序。
func (s *Store) List(ctx context.Context) ([]*modeldomain.ModelConfig, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	var rows []*modeldomain.ModelConfig
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", uid).
		Order("scenario").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("modelstore.List: %w", err)
	}
	return rows, nil
}

// Upsert saves m by primary key (INSERT if new, UPDATE if exists).
//
// Upsert 按主键保存 m（ID 新 INSERT，已存在 UPDATE）。
func (s *Store) Upsert(ctx context.Context, m *modeldomain.ModelConfig) error {
	if err := s.db.WithContext(ctx).Save(m).Error; err != nil {
		return fmt.Errorf("modelstore.Upsert: %w", err)
	}
	return nil
}

// AnyReferencesApiKey reports whether any model_config row references this api_key.
//
// AnyReferencesApiKey 报告是否有 model_config 行引用该 api_key。
func (s *Store) AnyReferencesApiKey(ctx context.Context, apiKeyID string) (bool, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return false, fmt.Errorf("modelstore.AnyReferencesApiKey: %w", err)
	}
	var count int64
	if err := s.db.WithContext(ctx).
		Model(&modeldomain.ModelConfig{}).
		Where("user_id = ? AND api_key_id = ?", uid, apiKeyID).
		Limit(1).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("modelstore.AnyReferencesApiKey: %w", err)
	}
	return count > 0, nil
}
