// Package apikey (app layer) owns the Service (CRUD + KeyProvider), the
// HTTP-tester wiring, and the MaskKey helper used only by this service.
//
// All three apikey packages (domain / app / store) declare `package apikey`;
// external callers alias at import (e.g. apikeyapp "…/internal/app/apikey").
//
// Package apikey（app 层）负责 Service（CRUD + KeyProvider）、HTTP-tester
// 的装配、以及 MaskKey 等只给 Service 用的工具函数。
//
// 三个 apikey 包（domain / app / store）都声明 `package apikey`；
// 外部调用方 import 时按角色起别名（如 apikeyapp "…/internal/app/apikey"）。
package apikey

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	cryptodomain "github.com/sunweilin/forgify/backend/internal/domain/crypto"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Service orchestrates apikey CRUD + connectivity testing. It owns the
// encryption boundary — callers never see plaintext or ciphertext, only
// the APIKey entity (ciphertext hidden by `json:"-"`) and TestResult.
//
// Service 编排 apikey 的 CRUD + 连通性测试。持有加密边界——调用方既看
// 不到明文也看不到密文，只拿到 APIKey（密文由 json:"-" 隐藏）和 TestResult。
type Service struct {
	repo      apikeydomain.Repository
	encryptor cryptodomain.Encryptor
	tester    ConnectivityTester
	log       *zap.Logger
}

// NewService wires Service dependencies. Panics on nil logger — a nil
// logger is a wiring bug, not a runtime condition.
//
// NewService 装配 Service 依赖。nil logger 会 panic——nil logger 是接线
// bug，不是运行时状态。
func NewService(repo apikeydomain.Repository, enc cryptodomain.Encryptor, tester ConnectivityTester, log *zap.Logger) *Service {
	if log == nil {
		panic("apikey.NewService: logger is nil")
	}
	return &Service{repo: repo, encryptor: enc, tester: tester, log: log}
}

// CreateInput is the validated request for Service.Create.
//
// CreateInput 是 Service.Create 的已校验请求形状。
type CreateInput struct {
	Provider    string
	DisplayName string
	Key         string
	BaseURL     string
	APIFormat   string
}

// UpdateInput is the partial-update payload for Service.Update. nil
// fields are left unchanged; a non-nil pointer to "" clears the value.
// Key / Provider / APIFormat are intentionally absent — changing them
// means delete + recreate.
//
// UpdateInput 是 Service.Update 的部分更新载荷。nil 字段不改；指向 "" 的
// 非 nil 指针清空该值。故意不含 Key / Provider / APIFormat——改它们
// 意味着 delete + recreate。
type UpdateInput struct {
	DisplayName *string
	BaseURL     *string
}

// Compile-time guard: *Service satisfies apikeydomain.KeyProvider.
//
// 编译期守护：*Service 满足 apikeydomain.KeyProvider。
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
		KeyMasked:    MaskKey(in.Key),
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
	if !IsValidProvider(in.Provider) {
		return fmt.Errorf("provider %q: %w", in.Provider, apikeydomain.ErrInvalidProvider)
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
	if in.DisplayName != nil {
		k.DisplayName = strings.TrimSpace(*in.DisplayName)
	}
	if in.BaseURL != nil {
		k.BaseURL = strings.TrimSpace(*in.BaseURL)
	}
	k.UpdatedAt = time.Now().UTC()
	if err := s.repo.Save(ctx, k); err != nil {
		return nil, err
	}
	return k, nil
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

// Test fetches the APIKey, decrypts, probes the upstream, writes the
// outcome back, and returns the TestResult.
//
// Test 取回 APIKey、解密、探测上游、写回结果、返回 TestResult。
func (s *Service) Test(ctx context.Context, id string) (*TestResult, error) {
	k, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	plain, err := s.encryptor.Decrypt(ctx, []byte(k.KeyEncrypted))
	if err != nil {
		return nil, fmt.Errorf("apikey.Service.Test: decrypt: %w", err)
	}
	result, err := s.tester.Test(ctx, k.Provider, string(plain), k.BaseURL, k.APIFormat)
	if err != nil {
		// Best-effort write of failed status. If this DB update itself fails
		// the row stays at its previous test_status — log loudly so a stale
		// "ok" badge in the UI doesn't go unexplained.
		//
		// 尽力把失败状态写库。本次写入再次失败时 test_status 维持原值——
		// 必须高声记录，避免 UI 还显示 "ok" 但实际已坏却无线索可追。
		if uerr := s.repo.UpdateTestResult(ctx, id, apikeydomain.TestStatusError, err.Error(), nil); uerr != nil {
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
	if upErr := s.repo.UpdateTestResult(ctx, id, status, errMsg, models); upErr != nil {
		return nil, upErr
	}
	s.log.Info("apikey tested",
		zap.String("key_id", id),
		zap.String("provider", k.Provider),
		zap.Bool("ok", result.OK),
		zap.Int64("latency_ms", result.LatencyMs))
	return result, nil
}

// ResolveCredentials picks the best APIKey for (caller, provider), decrypts,
// and merges baseURL with the provider default.
//
// ResolveCredentials 为 (调用者, provider) 挑选最佳 APIKey，解密，
// 并合并 baseURL 和 provider 默认值。
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

// MarkInvalid updates test_status to error on the selected APIKey when a
// caller's LLM call got 401/403.
//
// MarkInvalid 在调用方 LLM 调用遇到 401/403 时，把选中 APIKey 的
// test_status 更新为 error。
func (s *Service) MarkInvalid(ctx context.Context, provider, reason string) error {
	k, err := s.repo.GetByProvider(ctx, provider)
	if err != nil {
		return err
	}
	if err := s.repo.UpdateTestResult(ctx, k.ID, apikeydomain.TestStatusError, reason, nil); err != nil {
		return err
	}
	s.log.Warn("apikey marked invalid",
		zap.String("key_id", k.ID),
		zap.String("provider", provider),
		zap.String("reason", reason))
	return nil
}

// MaskKey converts a plaintext API key into a display-safe masked form.
//
// Rules:
//   - length <  8  → "****"
//   - length 8-20  → first 3 + "..." + last 4
//   - length > 20  → first 7 + "..." + last 4
//
// MaskKey 把明文 API Key 转成展示安全的掩码。
func MaskKey(key string) string {
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
