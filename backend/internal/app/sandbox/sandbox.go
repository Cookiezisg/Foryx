// Package sandbox is the application layer of PluginSandbox v2.
//
// Package sandbox 是 PluginSandbox v2 的应用层 façade。
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	sandboxinfra "github.com/sunweilin/forgify/backend/internal/infra/sandbox"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
)

// Service is the sandbox application façade.
//
// Service 是 sandbox 应用 façade。
type Service struct {
	repo        sandboxdomain.Repository
	sandboxRoot string
	log         *zap.Logger

	notif notificationspkg.Publisher

	miseBin string

	bootstrapped atomic.Bool
	bootstrapErr atomic.Pointer[error]

	regMu       sync.RWMutex
	installers  map[string]sandboxdomain.RuntimeInstaller
	envManagers map[string]sandboxdomain.EnvManager

	installLocks sync.Map
	envLocks     sync.Map

	activeHandles sync.Map
	nextHandleID  atomic.Uint64
}

// New constructs a Service; Bootstrap must succeed before EnsureRuntime/Spawn.
//
// New 构造 Service；EnsureRuntime/Spawn 前必须 Bootstrap 成功。
func New(repo sandboxdomain.Repository, dataDir string, notif notificationspkg.Publisher, log *zap.Logger) *Service {
	if log == nil {
		panic("sandboxapp.New: nil logger")
	}
	if notif == nil {
		notif = notificationspkg.New(nil, log)
	}
	return &Service{
		repo:        repo,
		sandboxRoot: filepath.Join(dataDir, "sandbox"),
		notif:       notif,
		log:         log,
		installers:  make(map[string]sandboxdomain.RuntimeInstaller),
		envManagers: make(map[string]sandboxdomain.EnvManager),
	}
}

// SandboxRoot returns the file-system root path (<dataDir>/sandbox/).
//
// SandboxRoot 返回文件系统根路径（<dataDir>/sandbox/）。
func (s *Service) SandboxRoot() string { return s.sandboxRoot }

// MiseBin returns the extracted mise binary path, or "" before Bootstrap.
//
// MiseBin 返回 mise 二进制路径，Bootstrap 前为空串。
func (s *Service) MiseBin() string { return s.miseBin }

// IsReady reports whether Bootstrap has succeeded.
//
// IsReady 报告 Bootstrap 是否已成功。
func (s *Service) IsReady() bool { return s.bootstrapped.Load() }

// BootstrapError returns the most recent Bootstrap failure or nil.
//
// BootstrapError 返回最近一次 Bootstrap 失败原因，无失败则 nil。
func (s *Service) BootstrapError() error {
	if e := s.bootstrapErr.Load(); e != nil {
		return *e
	}
	return nil
}

// Bootstrap extracts the embedded mise binary; idempotent, failure → degraded mode.
//
// Bootstrap 抽取 embed mise 二进制；幂等，失败进入 degraded mode。
func (s *Service) Bootstrap(ctx context.Context) error {
	miseBin, err := sandboxinfra.ExtractMiseBinary(ctx, s.sandboxRoot, s.log)
	if err != nil {
		s.log.Warn("sandbox bootstrap failed (degraded mode active)", zap.Error(err))
		captured := err
		s.bootstrapErr.Store(&captured)
		s.bootstrapped.Store(false)
		return fmt.Errorf("sandboxapp.Bootstrap: %w", err)
	}
	s.miseBin = miseBin
	s.bootstrapErr.Store(nil)
	s.bootstrapped.Store(true)
	s.log.Info("sandbox bootstrap ready", zap.String("mise_bin", miseBin))

	s.RestoreOrCleanupOnBoot(ctx)
	return nil
}

// RetryBootstrap re-runs Bootstrap (called by POST /sandbox:retry-bootstrap).
//
// RetryBootstrap 重跑 Bootstrap，由 POST /sandbox:retry-bootstrap 触发。
func (s *Service) RetryBootstrap(ctx context.Context) error {
	return s.Bootstrap(ctx)
}

// RegisterInstaller adds a RuntimeInstaller; idempotent per kind.
//
// RegisterInstaller 注册 RuntimeInstaller，同 kind 二次注册会替换。
func (s *Service) RegisterInstaller(installer sandboxdomain.RuntimeInstaller) {
	s.regMu.Lock()
	defer s.regMu.Unlock()
	s.installers[installer.Kind()] = installer
}

// RegisterEnvManager binds an EnvManager to its kind.
//
// RegisterEnvManager 把 EnvManager 绑定到对应 kind。
func (s *Service) RegisterEnvManager(manager sandboxdomain.EnvManager) {
	s.regMu.Lock()
	defer s.regMu.Unlock()
	s.envManagers[manager.Kind()] = manager
}

// EnsureTool resolves (kind, version) to a binary path, lazily installing.
//
// EnsureTool 把 (kind, version) 解析为二进制路径，缺则懒装 runtime。
func (s *Service) EnsureTool(ctx context.Context, kind, version string) (string, error) {
	rt, err := s.EnsureRuntime(ctx, sandboxdomain.RuntimeSpec{Kind: kind, Version: version}, nil)
	if err != nil {
		return "", fmt.Errorf("sandboxapp.EnsureTool %s: %w", kind, err)
	}
	s.regMu.RLock()
	installer, ok := s.installers[kind]
	s.regMu.RUnlock()
	if !ok {
		return "", fmt.Errorf("sandboxapp.EnsureTool %s: %w", kind, sandboxdomain.ErrRuntimeNotSupported)
	}
	bin, err := installer.Locate(rt.Version, s.sandboxRoot)
	if err != nil {
		return "", fmt.Errorf("sandboxapp.EnsureTool %s: %w", kind, err)
	}
	return bin, nil
}

// ListRuntimes returns all installed runtimes.
//
// ListRuntimes 返回所有已安装 runtime。
func (s *Service) ListRuntimes(ctx context.Context) ([]*sandboxdomain.Runtime, error) {
	return s.repo.ListRuntimes(ctx)
}

// ListEnvs returns envs for the given owner kind.
//
// ListEnvs 返回指定 owner kind 的 env 列表。
func (s *Service) ListEnvs(ctx context.Context, ownerKind string) ([]*sandboxdomain.Env, error) {
	return s.repo.ListEnvsByOwnerKind(ctx, ownerKind)
}

// TotalDiskUsage sums size_bytes across runtimes + envs.
//
// TotalDiskUsage 汇总 runtime + env 的 size_bytes。
func (s *Service) TotalDiskUsage(ctx context.Context) (int64, error) {
	return s.repo.TotalSizeBytes(ctx)
}

// GetEnv returns a single env by id; surfaces ErrEnvNotFound on miss.
//
// GetEnv 按 id 返回单个 env，缺失返 ErrEnvNotFound。
func (s *Service) GetEnv(ctx context.Context, id string) (*sandboxdomain.Env, error) {
	return s.repo.GetEnv(ctx, id)
}

// DeleteRuntime hard-removes a runtime; refuses if any env still references it.
//
// DeleteRuntime 硬删 runtime；仍有 env 引用时返 ErrEnvInUse。
func (s *Service) DeleteRuntime(ctx context.Context, id string) error {
	rt, err := s.repo.GetRuntime(ctx, id)
	if err != nil {
		return fmt.Errorf("sandboxapp.DeleteRuntime: get %s: %w", id, err)
	}
	envs, err := s.repo.ListEnvsByRuntime(ctx, id)
	if err != nil {
		return fmt.Errorf("sandboxapp.DeleteRuntime: list refs: %w", err)
	}
	if len(envs) > 0 {
		return fmt.Errorf("sandboxapp.DeleteRuntime: %d env(s) still reference %s: %w",
			len(envs), id, sandboxdomain.ErrEnvInUse)
	}
	rtPath := filepath.Join(s.sandboxRoot, rt.Path)
	if err := removeAll(rtPath); err != nil {
		s.log.Warn("sandbox: delete runtime dir failed (continuing to delete row)",
			zap.String("path", rtPath), zap.Error(err))
	}
	return s.repo.DeleteRuntime(ctx, id)
}

// GC destroys envs whose LastUsedAt is older than now-olderThan.
//
// GC 删除 LastUsedAt 早于 now-olderThan 的 env，返回实际删除数。
func (s *Service) GC(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan)
	stale, err := s.repo.ListEnvsLastUsedBefore(ctx, cutoff)
	if err != nil {
		return 0, fmt.Errorf("sandboxapp.GC: list stale: %w", err)
	}
	removed := 0
	for _, e := range stale {
		owner := sandboxdomain.Owner{Kind: e.OwnerKind, ID: e.OwnerID}
		if err := s.Destroy(ctx, owner); err != nil {
			s.log.Warn("sandbox GC: destroy env failed (continuing)",
				zap.String("env_id", e.ID), zap.Error(err))
			continue
		}
		removed++
	}
	s.log.Info("sandbox GC complete",
		zap.Int("scanned", len(stale)),
		zap.Int("removed", removed),
		zap.Duration("older_than", olderThan))
	return removed, nil
}

// EnsureRuntime installs if absent, returns existing row otherwise.
//
// EnsureRuntime 缺则装 runtime，否则返回已有 manifest 行。
func (s *Service) EnsureRuntime(ctx context.Context, spec sandboxdomain.RuntimeSpec, stream sandboxdomain.ProgressFunc) (*sandboxdomain.Runtime, error) {
	if !s.IsReady() {
		return nil, fmt.Errorf("sandboxapp.EnsureRuntime: %w", sandboxdomain.ErrRuntimeInstallFailed)
	}

	s.regMu.RLock()
	installer, ok := s.installers[spec.Kind]
	s.regMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("sandboxapp.EnsureRuntime %s: %w", spec.Kind, sandboxdomain.ErrRuntimeNotSupported)
	}

	version := spec.Version
	if version == "" {
		v, err := installer.ResolveDefault(ctx)
		if err != nil {
			return nil, fmt.Errorf("sandboxapp.EnsureRuntime: resolve default %s: %w", spec.Kind, err)
		}
		version = v
	}
	version = installer.NormalizeVersion(version)

	if existing, err := s.repo.FindRuntime(ctx, spec.Kind, version); err == nil {
		return existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("sandboxapp.EnsureRuntime: lookup %s@%s: %w", spec.Kind, version, err)
	}

	lock := s.kindLock(spec.Kind)
	lock.Lock()
	defer lock.Unlock()

	if existing, err := s.repo.FindRuntime(ctx, spec.Kind, version); err == nil {
		return existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("sandboxapp.EnsureRuntime: re-lookup %s@%s: %w", spec.Kind, version, err)
	}

	relPath, err := installer.Install(ctx, version, s.sandboxRoot, stream)
	if err != nil {
		return nil, fmt.Errorf("sandboxapp.EnsureRuntime: install %s@%s: %w", spec.Kind, version, err)
	}

	runtime := &sandboxdomain.Runtime{
		ID:          idgenpkg.New("sr"),
		Kind:        spec.Kind,
		Version:     version,
		Path:        relPath,
		SizeBytes:   computeDirSize(filepath.Join(s.sandboxRoot, relPath)),
		InstalledAt: time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := s.repo.CreateRuntime(ctx, runtime); err != nil {
		return nil, fmt.Errorf("sandboxapp.EnsureRuntime: persist %s@%s: %w", spec.Kind, version, err)
	}
	return runtime, nil
}

// EnsureEnv idempotently materializes a per-owner env; deps drift triggers rebuild.
//
// EnsureEnv 幂等物化 per-owner env；deps 漂移触发销毁重建。
func (s *Service) EnsureEnv(ctx context.Context, owner sandboxdomain.Owner, spec sandboxdomain.EnvSpec, stream sandboxdomain.ProgressFunc) (*sandboxdomain.Env, error) {
	if !s.IsReady() {
		return nil, fmt.Errorf("sandboxapp.EnsureEnv: %w", sandboxdomain.ErrEnvCreateFailed)
	}
	if owner.Kind == "" || owner.ID == "" {
		panic("sandboxapp.EnsureEnv: missing owner.Kind or owner.ID — caller wiring bug")
	}
	// owner.ID becomes a directory name and joins PATH at exec time; reject separators / shell metachars.
	// owner.ID 进 PATH 段，含分隔符 / shell 元字符则提前 reject。
	if strings.ContainsAny(owner.ID, ":;= \t\n\r\x00") {
		return nil, fmt.Errorf("sandboxapp.EnsureEnv: %w: %q", sandboxdomain.ErrInvalidOwnerID, owner.ID)
	}

	envLock := s.ownerLock(owner)
	envLock.Lock()
	defer envLock.Unlock()

	if existing, err := s.repo.FindEnvByOwner(ctx, owner.Kind, owner.ID); err == nil {
		if existing.Status == sandboxdomain.EnvStatusReady && depsEqual(existing.Deps, spec.Deps) {
			s.touchLastUsed(ctx, existing)
			return existing, nil
		}
		if err := s.destroyLocked(ctx, owner, existing); err != nil {
			return nil, fmt.Errorf("sandboxapp.EnsureEnv: destroy stale: %w", err)
		}
	} else if !errors.Is(err, sandboxdomain.ErrEnvNotFound) {
		return nil, fmt.Errorf("sandboxapp.EnsureEnv: lookup %s/%s: %w", owner.Kind, owner.ID, err)
	}

	rt, err := s.EnsureRuntime(ctx, spec.Runtime, stream)
	if err != nil {
		return nil, fmt.Errorf("sandboxapp.EnsureEnv: ensure runtime %s: %w", spec.Runtime.Kind, err)
	}

	s.regMu.RLock()
	em, ok := s.envManagers[spec.Runtime.Kind]
	s.regMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("sandboxapp.EnsureEnv %s: no env manager registered: %w", spec.Runtime.Kind, sandboxdomain.ErrRuntimeNotSupported)
	}

	envID := idgenpkg.New("se")
	envRel := filepath.Join("envs", owner.Kind, owner.ID)
	envPath := filepath.Join(s.sandboxRoot, envRel)

	now := time.Now()
	env := &sandboxdomain.Env{
		ID:         envID,
		OwnerKind:  owner.Kind,
		OwnerID:    owner.ID,
		OwnerName:  owner.Name,
		RuntimeID:  rt.ID,
		Deps:       spec.Deps,
		Path:       envRel,
		Status:     sandboxdomain.EnvStatusInstalling,
		CreatedAt:  now,
		LastUsedAt: now,
		UpdatedAt:  now,
	}
	if err := s.repo.CreateEnv(ctx, env); err != nil {
		return nil, fmt.Errorf("sandboxapp.EnsureEnv: persist row: %w", err)
	}
	s.publishEnv(ctx, env)

	runtimePath := filepath.Join(s.sandboxRoot, rt.Path)
	if err := em.CreateEnv(ctx, runtimePath, envPath); err != nil {
		s.markEnvFailed(ctx, env, err)
		return nil, fmt.Errorf("sandboxapp.EnsureEnv create: %w", err)
	}
	if err := em.InstallDeps(ctx, runtimePath, envPath, spec.Deps, stream); err != nil {
		s.markEnvFailed(ctx, env, err)
		return nil, fmt.Errorf("sandboxapp.EnsureEnv deps: %w", err)
	}

	env.Status = sandboxdomain.EnvStatusReady
	env.SizeBytes = computeDirSize(envPath)
	env.UpdatedAt = time.Now()
	if err := s.repo.UpdateEnv(context.Background(), env); err != nil {
		return nil, fmt.Errorf("sandboxapp.EnsureEnv: persist ready: %w", err)
	}
	s.publishEnv(ctx, env)
	return env, nil
}

// Destroy removes an env (DB row + on-disk dir); idempotent.
//
// Destroy 删除 env（DB 行 + 磁盘目录），幂等。
func (s *Service) Destroy(ctx context.Context, owner sandboxdomain.Owner) error {
	envLock := s.ownerLock(owner)
	envLock.Lock()
	defer envLock.Unlock()

	existing, err := s.repo.FindEnvByOwner(ctx, owner.Kind, owner.ID)
	if errors.Is(err, sandboxdomain.ErrEnvNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("sandboxapp.Destroy: lookup %s/%s: %w", owner.Kind, owner.ID, err)
	}
	return s.destroyLocked(ctx, owner, existing)
}

func (s *Service) destroyLocked(ctx context.Context, owner sandboxdomain.Owner, env *sandboxdomain.Env) error {
	envPath := filepath.Join(s.sandboxRoot, env.Path)
	if err := removeAll(envPath); err != nil {
		s.log.Warn("sandbox destroy: rm env dir failed (continuing to delete row)",
			zap.String("path", envPath), zap.Error(err))
	}
	if err := s.repo.DeleteEnv(ctx, env.ID); err != nil {
		return fmt.Errorf("sandboxapp.Destroy: delete row %s: %w", env.ID, err)
	}
	s.publishEnvDeleted(ctx, env.ID)
	return nil
}

// markEnvFailed flips Status=failed via detached ctx (§S9 terminal-state write).
//
// markEnvFailed 用 detached ctx 把 Status 翻为 failed，避免 caller 取消丢失败记录。
func (s *Service) markEnvFailed(ctx context.Context, env *sandboxdomain.Env, cause error) {
	env.Status = sandboxdomain.EnvStatusFailed
	env.ErrorMsg = cause.Error()
	env.UpdatedAt = time.Now()
	if err := s.repo.UpdateEnv(context.Background(), env); err != nil {
		s.log.Warn("sandbox: failed-status persist failed",
			zap.String("env_id", env.ID),
			zap.Error(err))
	}
	s.publishEnv(ctx, env)
}

// publishEnv emits a slim sandbox_env state-change notification.
//
// publishEnv 发送 sandbox_env 瘦身状态通知（含 action + status + ownerKind/ID）。
func (s *Service) publishEnv(ctx context.Context, env *sandboxdomain.Env) {
	data := map[string]any{
		"action":    "status_changed",
		"status":    env.Status,
		"ownerKind": env.OwnerKind,
		"ownerId":   env.OwnerID,
	}
	if env.ErrorMsg != "" {
		data["errorMsg"] = env.ErrorMsg
	}
	s.notif.Publish(ctx, "sandbox_env", env.ID, data, "")
}

func (s *Service) publishEnvDeleted(ctx context.Context, envID string) {
	s.notif.Publish(ctx, "sandbox_env", envID,
		map[string]any{"action": "deleted"}, "")
}

func envRuntimeKind(env *sandboxdomain.Env, s *Service) string {
	rt, err := s.repo.GetRuntime(context.Background(), env.RuntimeID)
	if err != nil || rt == nil {
		return ""
	}
	return rt.Kind
}

func (s *Service) touchLastUsed(ctx context.Context, env *sandboxdomain.Env) {
	env.LastUsedAt = time.Now()
	if err := s.repo.UpdateEnv(ctx, env); err != nil {
		s.log.Warn("sandbox: touch last_used_at failed",
			zap.String("env_id", env.ID),
			zap.Error(err))
	}
}

func (s *Service) kindLock(kind string) *sync.Mutex {
	mu, _ := s.installLocks.LoadOrStore(kind, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

func (s *Service) ownerLock(owner sandboxdomain.Owner) *sync.Mutex {
	key := owner.Kind + ":" + owner.ID
	mu, _ := s.envLocks.LoadOrStore(key, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

func depsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	seen := make(map[string]int, len(a))
	for _, d := range a {
		seen[d]++
	}
	for _, d := range b {
		seen[d]--
		if seen[d] < 0 {
			return false
		}
	}
	return true
}

// MarkReadyForTest forces IsReady true; production code MUST NOT call this.
//
// MarkReadyForTest 强制 IsReady 为 true，仅测试用，生产禁用。
func (s *Service) MarkReadyForTest(miseBin string) {
	s.miseBin = miseBin
	s.bootstrapped.Store(true)
}

// ActiveHandleCountForTest returns the count of registered LongLived handles.
//
// ActiveHandleCountForTest 返回已注册 LongLived handle 数量（仅测试用）。
func (s *Service) ActiveHandleCountForTest() int {
	count := 0
	s.activeHandles.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}
