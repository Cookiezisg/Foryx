// Package apikey owns the credential CRUD service, the encryption boundary, the
// dumb connectivity probe, and the by-id KeyProvider/ProbeReader ports. It holds
// no provider semantics beyond "how to connect/probe" — choosing keys and reading
// models live downstream (model / search config).
//
// Package apikey 提供凭证 CRUD service、加密边界、哑连通探针，以及按 id 的 KeyProvider/
// ProbeReader 端口。除「怎么连/怎么探」外不持任何 provider 语义——选 key 与读模型在下游
// （model / 搜索配置）。
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

// RefScanner reports whether some entity still references a given api_key id.
// Delete consults every registered scanner; any true → ErrInUse. Implementations
// live in the referencing modules (model / conversation / workflow) and are
// injected at wiring — apikey holds only this port, depending on none of them.
//
// RefScanner 检测某 api_key 是否仍被引用。Delete 询问每个已注册 scanner，任一 true → ErrInUse。
// 实现住在引用方模块（model/conversation/workflow），装配时注入——apikey 只持端口、不依赖它们。
type RefScanner interface {
	ReferencesAPIKey(ctx context.Context, apiKeyID string) (bool, error)
}

// Service orchestrates apikey CRUD + connectivity probing; owns the encryption boundary.
//
// Service 编排 apikey CRUD + 连通探测，持有加密边界。
type Service struct {
	repo      apikeydomain.Repository
	encryptor cryptodomain.Encryptor
	tester    ConnectivityTester
	log       *zap.Logger
	scanners  []RefScanner
}

// NewService wires dependencies; panics on nil logger.
//
// NewService 装配依赖；nil logger panic。
func NewService(repo apikeydomain.Repository, enc cryptodomain.Encryptor, tester ConnectivityTester, log *zap.Logger) *Service {
	if log == nil {
		panic("apikey.NewService: logger is nil")
	}
	return &Service{repo: repo, encryptor: enc, tester: tester, log: log.Named("apikeyapp")}
}

// AddRefScanner registers a delete-time reference scanner (DIP, wiring-time).
//
// AddRefScanner 注册删除期引用 scanner（DIP，装配时）。
func (s *Service) AddRefScanner(rs RefScanner) { s.scanners = append(s.scanners, rs) }

var (
	_ apikeydomain.KeyProvider = (*Service)(nil)
	_ apikeydomain.ProbeReader = (*Service)(nil)
)

// CreateInput is the validated request for Create.
//
// CreateInput 是 Create 的已校验请求。
type CreateInput struct {
	Provider    string
	DisplayName string
	Key         string
	BaseURL     string
	APIFormat   string
}

// UpdateInput is the partial-update payload; nil fields unchanged. A non-empty
// Key rotates the credential (re-encrypt + re-mask + reset probe archive).
//
// UpdateInput 部分更新载荷；nil 不动。非空 Key 旋转凭证（重新加密 + 重新脱敏 + 重置探测档案）。
type UpdateInput struct {
	DisplayName *string
	BaseURL     *string
	Key         *string
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*apikeydomain.APIKey, error) {
	if err := validateCreate(in); err != nil {
		return nil, err
	}
	ciphertext, err := s.encryptor.Encrypt(ctx, []byte(in.Key))
	if err != nil {
		return nil, fmt.Errorf("apikey.Service.Create: encrypt: %w", err)
	}
	now := time.Now().UTC()
	k := &apikeydomain.APIKey{
		ID:           newID(),
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
	// WorkspaceID is stamped by orm from ctx on Save — never set by hand.
	// WorkspaceID 由 orm 在 Save 时从 ctx 打上——不手设。
	if err := s.repo.Save(ctx, k); err != nil {
		return nil, err
	}
	s.log.Info("apikey created", zap.String("key_id", k.ID), zap.String("provider", k.Provider))
	return k, nil
}

func validateCreate(in CreateInput) error {
	if !isValidProvider(in.Provider) {
		return apikeydomain.ErrInvalidProvider
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
	if in.DisplayName == nil && in.BaseURL == nil && in.Key == nil {
		return k, nil // no-op PATCH {}; don't bump updated_at
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
		// Rotating the key invalidates the old probe archive.
		// 旋转 key 使旧探测档案作废。
		k.TestStatus = apikeydomain.TestStatusPending
		k.TestError = ""
		k.TestResponse = ""
		k.LastTestedAt = nil
	}
	k.UpdatedAt = time.Now().UTC()
	if err := s.repo.Save(ctx, k); err != nil {
		return nil, err
	}
	return k, nil
}

// Delete refuses if any registered scanner reports this key is still referenced.
//
// Delete 命中任一引用 scanner 即拒删（ErrInUse）。
func (s *Service) Delete(ctx context.Context, id string) error {
	for _, sc := range s.scanners {
		used, err := sc.ReferencesAPIKey(ctx, id)
		if err != nil {
			return fmt.Errorf("apikey.Service.Delete: ref scan: %w", err)
		}
		if used {
			return apikeydomain.ErrInUse
		}
	}
	return s.repo.Delete(ctx, id)
}

func (s *Service) Get(ctx context.Context, id string) (*apikeydomain.APIKey, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) List(ctx context.Context, filter apikeydomain.ListFilter) ([]*apikeydomain.APIKey, string, error) {
	return s.repo.List(ctx, filter)
}

// Test probes the upstream (dumb: live or not) and persists the outcome via a
// detached ctx (§S9) so a cancelled request can't drop the result. The raw
// response is archived verbatim; this layer never parses it.
//
// Test 探测上游（哑探针：活没活）并用 detached ctx 写回（§S9），避免请求取消丢落库。
// 原始返回原样存档；本层不解析。
func (s *Service) Test(ctx context.Context, id string) (*TestResult, error) {
	k, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("apikey.Service.Test: %w", err)
	}
	plain, err := s.encryptor.Decrypt(ctx, []byte(k.KeyEncrypted))
	if err != nil {
		return nil, fmt.Errorf("apikey.Service.Test: decrypt: %w", err)
	}
	detached, err := s.detach(ctx)
	if err != nil {
		return nil, fmt.Errorf("apikey.Service.Test: %w", err)
	}
	result, err := s.tester.Test(ctx, k.Provider, string(plain), k.BaseURL, k.APIFormat)
	if err != nil {
		if uerr := s.repo.UpdateTestResult(detached, id, apikeydomain.TestStatusError, err.Error(), ""); uerr != nil {
			s.log.Warn("apikey.Service.Test: persisting failure status itself failed",
				zap.String("key_id", id), zap.Error(uerr))
		}
		return nil, fmt.Errorf("apikey.Service.Test: probe: %w", err)
	}
	status, errMsg, raw := apikeydomain.TestStatusError, result.Message, ""
	if result.OK {
		status, errMsg, raw = apikeydomain.TestStatusOK, "", result.RawResponse
	}
	if upErr := s.repo.UpdateTestResult(detached, id, status, errMsg, raw); upErr != nil {
		return nil, fmt.Errorf("apikey.Service.Test: persist result: %w", upErr)
	}
	s.log.Info("apikey probed", zap.String("key_id", id), zap.String("provider", k.Provider), zap.Bool("ok", result.OK))
	return result, nil
}

// ResolveCredentialsByID resolves ready-to-use credentials by key id; repo.Get is
// workspace-scoped, so cross-workspace ids naturally surface ErrNotFound.
//
// ResolveCredentialsByID 按 key id 解析可用凭证；repo.Get 按 workspace 隔离，跨 workspace
// 的 id 天然走 ErrNotFound。
func (s *Service) ResolveCredentialsByID(ctx context.Context, apiKeyID string) (apikeydomain.Credentials, error) {
	k, err := s.repo.Get(ctx, apiKeyID)
	if err != nil {
		return apikeydomain.Credentials{}, fmt.Errorf("apikey.Service.ResolveCredentialsByID: %w", err)
	}
	plain, err := s.encryptor.Decrypt(ctx, []byte(k.KeyEncrypted))
	if err != nil {
		return apikeydomain.Credentials{}, fmt.Errorf("apikey.Service.ResolveCredentialsByID: decrypt: %w", err)
	}
	return apikeydomain.Credentials{
		Provider:  k.Provider,
		Key:       string(plain),
		BaseURL:   resolveBaseURL(k),
		APIFormat: k.APIFormat,
	}, nil
}

// MarkInvalidByID flips a key's status to error by id (e.g. caller hit 401/403),
// via detached ctx (§S9).
//
// MarkInvalidByID 按 id 把某 key 状态标 error（如调用方撞 401/403），用 detached ctx（§S9）。
func (s *Service) MarkInvalidByID(ctx context.Context, apiKeyID, reason string) error {
	detached, err := s.detach(ctx)
	if err != nil {
		return fmt.Errorf("apikey.Service.MarkInvalidByID: %w", err)
	}
	if err := s.repo.UpdateTestResult(detached, apiKeyID, apikeydomain.TestStatusError, reason, ""); err != nil {
		return fmt.Errorf("apikey.Service.MarkInvalidByID: %w", err)
	}
	s.log.Warn("apikey marked invalid", zap.String("key_id", apiKeyID), zap.String("reason", reason))
	return nil
}

// ListProbed exposes the workspace's probe archive for the model module to parse.
//
// ListProbed 暴露本 workspace 的探测档案供 model 模块解析。
func (s *Service) ListProbed(ctx context.Context) ([]apikeydomain.ProbedKey, error) {
	return s.repo.ListProbed(ctx)
}

// detach builds a background ctx carrying the same workspace, for finalize writes
// that must survive request cancellation (§S9).
//
// detach 构造携带同一 workspace 的 background ctx，供必须熬过请求取消的 finalize 写入（§S9）。
func (s *Service) detach(ctx context.Context) (context.Context, error) {
	ws, err := reqctxpkg.RequireWorkspaceID(ctx)
	if err != nil {
		return nil, err
	}
	return reqctxpkg.SetWorkspaceID(context.Background(), ws), nil
}

// resolveBaseURL uses the key's own base, falling back to the provider default.
//
// resolveBaseURL 用 key 自己的 base，回退到 provider 默认。
func resolveBaseURL(k *apikeydomain.APIKey) string {
	if k.BaseURL != "" {
		return k.BaseURL
	}
	if meta, ok := GetProviderMeta(k.Provider); ok {
		return meta.DefaultBaseURL
	}
	return ""
}

// maskKey renders a display-safe masked form: keep a head + last 4.
//
// maskKey 渲染可展示的脱敏形式：保留头部 + 末 4 位。
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
