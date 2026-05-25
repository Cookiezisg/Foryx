// Package apikey owns the CRUD service, KeyProvider, and HTTP-tester wiring.
//
// Package apikey 提供 CRUD service、KeyProvider 与 HTTP-tester 装配。
package apikey

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	cryptodomain "github.com/sunweilin/forgify/backend/internal/domain/crypto"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Service orchestrates apikey CRUD + connectivity testing; owns the encryption boundary.
//
// Service 编排 apikey CRUD 与连通性测试，持有加密边界。
type Service struct {
	repo      apikeydomain.Repository
	encryptor cryptodomain.Encryptor
	tester    ConnectivityTester
	log       *zap.Logger
}

// NewService wires Service dependencies; panics on nil logger.
//
// NewService 装配 Service 依赖；nil logger 直接 panic。
func NewService(repo apikeydomain.Repository, enc cryptodomain.Encryptor, tester ConnectivityTester, log *zap.Logger) *Service {
	if log == nil {
		panic("apikey.NewService: logger is nil")
	}
	return &Service{repo: repo, encryptor: enc, tester: tester, log: log}
}

// CreateInput is the validated request for Service.Create.
//
// CreateInput 是 Service.Create 的已校验请求。
type CreateInput struct {
	Provider    string
	DisplayName string
	Key         string
	BaseURL     string
	APIFormat   string
}

// UpdateInput is the partial-update payload; nil fields unchanged, "" clears.
// Key rotation: non-nil non-empty Key re-encrypts + masks + resets test_status to pending.
// IsDefault true clears siblings in the same category before setting this key as default.
//
// UpdateInput 部分更新载荷；nil 不动、空串清空。Key 非空时旋转密钥（重新加密 +
// 重新 mask + 重置 test_status=pending），不允许改 Provider/APIFormat（删了重建）。
// IsDefault=true 先清除同 category 其他 key 的 is_default，再设置此 key 为 default。
type UpdateInput struct {
	DisplayName *string
	BaseURL     *string
	Key         *string
	IsDefault   *bool
}

var _ apikeydomain.KeyProvider = (*Service)(nil)

func (s *Service) Create(ctx context.Context, in CreateInput) (*apikeydomain.APIKey, error) {
	if err := validateCreate(in); err != nil {
		return nil, err
	}
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, fmt.Errorf("apikey.Service.Create: %w", err)
	}
	ciphertext, err := s.encryptor.Encrypt(ctx, []byte(in.Key))
	if err != nil {
		return nil, fmt.Errorf("apikey.Service.Create: encrypt: %w", err)
	}
	now := time.Now().UTC()
	k := &apikeydomain.APIKey{
		ID:           newID(),
		UserID:       uid,
		Provider:     in.Provider,
		DisplayName:  strings.TrimSpace(in.DisplayName),
		KeyEncrypted: string(ciphertext),
		KeyMasked:    maskKey(in.Key),
		BaseURL:      strings.TrimSpace(in.BaseURL),
		APIFormat:    in.APIFormat,
		TestStatus:   apikeydomain.TestStatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repo.Save(ctx, k); err != nil {
		return nil, err
	}
	s.log.Info("apikey created",
		zap.String("key_id", k.ID),
		zap.String("user_id", uid),
		zap.String("provider", k.Provider))
	return k, nil
}

func validateCreate(in CreateInput) error {
	if !isValidProvider(in.Provider) {
		return fmt.Errorf("apikey.validateCreate: provider %q: %w", in.Provider, apikeydomain.ErrInvalidProvider)
	}
	if strings.TrimSpace(in.Key) == "" {
		return apikeydomain.ErrKeyRequired
	}
	meta, _ := GetProviderMeta(in.Provider)
	if meta.BaseURLRequired && strings.TrimSpace(in.BaseURL) == "" {
		return apikeydomain.ErrBaseURLRequired
	}
	if in.Provider == "custom" && strings.TrimSpace(in.APIFormat) == "" {
		return apikeydomain.ErrAPIFormatRequired
	}
	return nil
}

func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*apikeydomain.APIKey, error) {
	k, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	// No-op: empty PATCH `{}` returns the row unchanged (skip updated_at bump).
	// No-op:空 PATCH `{}` 不 bump updated_at,直接返当前行。
	if in.DisplayName == nil && in.BaseURL == nil && in.Key == nil && in.IsDefault == nil {
		return k, nil
	}
	if in.DisplayName != nil {
		k.DisplayName = strings.TrimSpace(*in.DisplayName)
	}
	if in.BaseURL != nil {
		k.BaseURL = strings.TrimSpace(*in.BaseURL)
	}
	if in.Key != nil {
		newKey := strings.TrimSpace(*in.Key)
		if newKey == "" {
			return nil, apikeydomain.ErrKeyRequired
		}
		ciphertext, encErr := s.encryptor.Encrypt(ctx, []byte(newKey))
		if encErr != nil {
			return nil, fmt.Errorf("apikey.Service.Update: encrypt: %w", encErr)
		}
		k.KeyEncrypted = string(ciphertext)
		k.KeyMasked = maskKey(newKey)
		k.TestStatus = apikeydomain.TestStatusPending
		k.TestError = ""
		k.LastTestedAt = nil
		k.ModelsFound = nil
	}
	if in.IsDefault != nil {
		if *in.IsDefault {
			cat := providerCategory(k.Provider)
			if err := s.repo.ClearDefaultForCategory(ctx, providersInCategory(cat)); err != nil {
				return nil, fmt.Errorf("apikey.Update: %w", err)
			}
		}
		k.IsDefault = *in.IsDefault
	}
	k.UpdatedAt = time.Now().UTC()
	if err := s.repo.Save(ctx, k); err != nil {
		return nil, err
	}
	return k, nil
}

// HasKeyForProvider reports whether any active key exists for provider under the ctx user.
//
// HasKeyForProvider 报告当前用户在 provider 下是否有活跃 key（用于 model upsert 早校验）。
func (s *Service) HasKeyForProvider(ctx context.Context, provider string) (bool, error) {
	_, err := s.repo.GetByProvider(ctx, provider)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, apikeydomain.ErrNotFoundForProvider) {
		return false, nil
	}
	return false, fmt.Errorf("apikey.Service.HasKeyForProvider: %w", err)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) Get(ctx context.Context, id string) (*apikeydomain.APIKey, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, filter apikeydomain.ListFilter) ([]*apikeydomain.APIKey, string, error) {
	return s.repo.List(ctx, filter)
}

// Test probes the upstream and persists outcome via detached ctx (§S9).
//
// Test 探测上游并用 detached ctx 写回结果，避免请求 cancel 丢落库。
func (s *Service) Test(ctx context.Context, id string) (*TestResult, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, fmt.Errorf("apikey.Service.Test: %w", err)
	}
	k, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("apikey.Service.Test: %w", err)
	}
	plain, err := s.encryptor.Decrypt(ctx, []byte(k.KeyEncrypted))
	if err != nil {
		return nil, fmt.Errorf("apikey.Service.Test: decrypt: %w", err)
	}
	detached := reqctxpkg.SetUserID(context.Background(), uid)
	result, err := s.tester.Test(ctx, k.Provider, string(plain), k.BaseURL, k.APIFormat)
	if err != nil {
		if uerr := s.repo.UpdateTestResult(detached, id, apikeydomain.TestStatusError, err.Error(), nil); uerr != nil {
			s.log.Warn("apikey.Service.Test: persist test failure status itself failed; row stays at previous status",
				zap.String("api_key_id", id), zap.NamedError("test_err", err), zap.Error(uerr))
		}
		return nil, fmt.Errorf("apikey.Service.Test: tester: %w", err)
	}
	status := apikeydomain.TestStatusError
	errMsg := result.Message
	var models []string
	if result.OK {
		status = apikeydomain.TestStatusOK
		errMsg = ""
		models = result.ModelsFound
	}
	if upErr := s.repo.UpdateTestResult(detached, id, status, errMsg, models); upErr != nil {
		return nil, fmt.Errorf("apikey.Service.Test: persist result: %w", upErr)
	}
	s.log.Info("apikey tested",
		zap.String("key_id", id),
		zap.String("provider", k.Provider),
		zap.Bool("ok", result.OK),
		zap.Int64("latency_ms", result.LatencyMs))
	return result, nil
}

// ResolveCredentials picks the best APIKey for provider, decrypts, merges baseURL.
//
// ResolveCredentials 挑选最佳 APIKey 并解密，合并 baseURL 与默认值。
func (s *Service) ResolveCredentials(ctx context.Context, provider string) (apikeydomain.Credentials, error) {
	k, err := s.repo.GetByProvider(ctx, provider)
	if err != nil {
		return apikeydomain.Credentials{}, err
	}
	plain, err := s.encryptor.Decrypt(ctx, []byte(k.KeyEncrypted))
	if err != nil {
		return apikeydomain.Credentials{}, fmt.Errorf("apikey.Service.ResolveCredentials: decrypt: %w", err)
	}
	baseURL := k.BaseURL
	if baseURL == "" {
		if meta, ok := GetProviderMeta(provider); ok {
			baseURL = meta.DefaultBaseURL
		}
	}
	return apikeydomain.Credentials{Key: string(plain), BaseURL: baseURL}, nil
}

// MarkInvalid flips test_status to error on 401/403 via detached ctx (§S9).
//
// MarkInvalid 在 401/403 时用 detached ctx 把 test_status 标为 error。
func (s *Service) MarkInvalid(ctx context.Context, provider, reason string) error {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return fmt.Errorf("apikey.Service.MarkInvalid: %w", err)
	}
	k, err := s.repo.GetByProvider(ctx, provider)
	if err != nil {
		return fmt.Errorf("apikey.Service.MarkInvalid: %w", err)
	}
	detached := reqctxpkg.SetUserID(context.Background(), uid)
	if err := s.repo.UpdateTestResult(detached, k.ID, apikeydomain.TestStatusError, reason, nil); err != nil {
		return fmt.Errorf("apikey.Service.MarkInvalid: %w", err)
	}
	s.log.Warn("apikey marked invalid",
		zap.String("key_id", k.ID),
		zap.String("provider", provider),
		zap.String("reason", reason))
	return nil
}

// DefaultSearchProvider returns the provider name of the user's is_default search key,
// or "" on error or if none is marked. Implements apikeydomain.KeyProvider.
//
// DefaultSearchProvider 返回用户标为 is_default 的搜索 provider 名，出错或无则返 ""。
func (s *Service) DefaultSearchProvider(ctx context.Context) string {
	p, err := s.repo.DefaultProvider(ctx, apikeydomain.SearchProviderPriority)
	if err != nil {
		s.log.Debug("apikey.Service.DefaultSearchProvider: repo error; returning empty",
			zap.Error(err))
		return ""
	}
	return p
}

// providerCategory maps a provider name to its ProviderCategory using the providers registry.
//
// providerCategory 通过 providers 注册表把 provider 名映射到 ProviderCategory。
func providerCategory(name string) ProviderCategory {
	if meta, ok := GetProviderMeta(name); ok {
		return meta.Category
	}
	return CategoryLLM
}

// providersInCategory returns all provider names belonging to the given category.
//
// providersInCategory 返回属于指定 category 的全部 provider 名。
func providersInCategory(cat ProviderCategory) []string {
	var out []string
	for name, meta := range providers {
		if meta.Category == cat {
			out = append(out, name)
		}
	}
	return out
}

func maskKey(key string) string {
	n := len(key)
	switch {
	case n < 8:
		return "****"
	case n <= 20:
		return key[:3] + "..." + key[n-4:]
	default:
		return key[:7] + "..." + key[n-4:]
	}
}

func newID() string { return idgenpkg.New("aki") }
