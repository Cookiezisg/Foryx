package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// LoadConfig fetches encrypted config from DB and decrypts; returns nil when unconfigured.
//
// LoadConfig 从 DB 取加密 config 并解密；未配置时返 nil。
func (s *Service) LoadConfig(ctx context.Context, handlerID string) (map[string]any, error) {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return nil, fmt.Errorf("handlerapp.LoadConfig: %w", err)
	}
	ciphertext, err := s.repo.GetConfigEncrypted(ctx, handlerID)
	if err != nil {
		return nil, fmt.Errorf("handlerapp.LoadConfig: %w", err)
	}
	if ciphertext == "" {
		return nil, nil
	}
	plaintext, err := s.encryptor.Decrypt(ctx, []byte(ciphertext))
	if err != nil {
		return nil, fmt.Errorf("handlerapp.LoadConfig: %w: %v", handlerdomain.ErrConfigDecryptFailed, err)
	}
	var config map[string]any
	if err := json.Unmarshal(plaintext, &config); err != nil {
		return nil, fmt.Errorf("handlerapp.LoadConfig: unmarshal: %w", err)
	}
	return config, nil
}

// UpdateConfig applies a JSON Merge Patch and re-encrypts the whole blob; publishes config_updated.
//
// UpdateConfig 应用 JSON Merge Patch 后整 blob 重新加密回写，并推 config_updated。
func (s *Service) UpdateConfig(ctx context.Context, handlerID string, partial map[string]any) error {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return fmt.Errorf("handlerapp.UpdateConfig: %w", err)
	}
	existing, err := s.LoadConfig(ctx, handlerID)
	if err != nil && !errors.Is(err, handlerdomain.ErrConfigDecryptFailed) {
		return fmt.Errorf("handlerapp.UpdateConfig: load: %w", err)
	}
	if existing == nil {
		existing = map[string]any{}
	}
	merged := mergePatch(existing, partial)

	plaintext, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("handlerapp.UpdateConfig: marshal: %w", err)
	}
	ciphertext, err := s.encryptor.Encrypt(ctx, plaintext)
	if err != nil {
		return fmt.Errorf("handlerapp.UpdateConfig: encrypt: %w", err)
	}
	if err := s.repo.UpdateConfigEncrypted(ctx, handlerID, string(ciphertext)); err != nil {
		return fmt.Errorf("handlerapp.UpdateConfig: persist: %w", err)
	}
	s.publishHandlerEvent(ctx, handlerID, "config_updated", nil)
	return nil
}

// ClearConfig wipes the ciphertext blob back to unconfigured.
//
// ClearConfig 清空密文 blob 回到未配置。
func (s *Service) ClearConfig(ctx context.Context, handlerID string) error {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return fmt.Errorf("handlerapp.ClearConfig: %w", err)
	}
	if err := s.repo.ClearConfig(ctx, handlerID); err != nil {
		return fmt.Errorf("handlerapp.ClearConfig: %w", err)
	}
	s.publishHandlerEvent(ctx, handlerID, "config_cleared", nil)
	return nil
}

// ComputeConfigState compares declared schema against stored config and returns state + missing required keys.
//
// ComputeConfigState 比较 declared schema 与已存 config，返状态加缺失必填 key 列表。
func (s *Service) ComputeConfigState(ctx context.Context, handlerID string, schema []handlerdomain.InitArgSpec) (string, []string, error) {
	cfg, err := s.LoadConfig(ctx, handlerID)
	if err != nil {
		return handlerdomain.ConfigStateUnconfigured, nil, err
	}

	missing := []string{}
	totalRequired := 0
	for _, arg := range schema {
		if !arg.Required {
			continue
		}
		totalRequired++
		if cfg == nil {
			missing = append(missing, arg.Name)
			continue
		}
		if v, ok := cfg[arg.Name]; !ok || v == nil {
			missing = append(missing, arg.Name)
		}
	}

	switch {
	case len(missing) == 0:
		return handlerdomain.ConfigStateReady, nil, nil
	case len(missing) == totalRequired:
		return handlerdomain.ConfigStateUnconfigured, missing, nil
	default:
		return handlerdomain.ConfigStatePartiallyConfigured, missing, nil
	}
}

// MaskedConfig returns the loaded config with sensitive values replaced by "********".
//
// MaskedConfig 返加载 config 的副本，sensitive 字段替换为 "********"。
func (s *Service) MaskedConfig(ctx context.Context, handlerID string, schema []handlerdomain.InitArgSpec) (map[string]any, error) {
	cfg, err := s.LoadConfig(ctx, handlerID)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, nil
	}
	sensitive := make(map[string]bool, len(schema))
	for _, a := range schema {
		if a.Sensitive {
			sensitive[a.Name] = true
		}
	}
	out := make(map[string]any, len(cfg))
	for k, v := range cfg {
		if sensitive[k] {
			out[k] = "********"
			continue
		}
		out[k] = v
	}
	return out, nil
}

func (s *Service) publishHandlerEvent(ctx context.Context, handlerID, action string, extra map[string]any) {
	envelope := map[string]any{"action": action}
	for k, v := range extra {
		envelope[k] = v
	}
	s.notif.Publish(ctx, "handler", handlerID, envelope, "")
}
