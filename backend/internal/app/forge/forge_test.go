// Package forge — Service unit tests covering the sandbox iteration lifecycle:
// create / draft / pending / accept / revert / delete + EnvID inheritance,
// trim, and the punt-to-AI error paths. Uses a real in-memory SQLite store
// and a fake sandbox that records its calls so we can assert what the
// Service drives through the port.
//
// Package forge — Service 单测，覆盖沙箱迭代生命周期：create / draft /
// pending / accept / revert / delete + EnvID 继承、trim、punt-to-AI 错误路径。
// 用真 in-memory SQLite store + 记录调用的 fake sandbox，断言 Service
// 通过 port 驱动的行为。

package forge

import (
	"context"
	"errors"
	"sync"
	"testing"

	gormlogger "gorm.io/gorm/logger"

	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	sandboxinfra "github.com/sunweilin/forgify/backend/internal/infra/sandbox"
	forgestore "github.com/sunweilin/forgify/backend/internal/infra/store/forge"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"

	"go.uber.org/zap"
)

// ── fake sandbox ─────────────────────────────────────────────────────────────

// fakeSandbox is a record-and-respond fake of the Sandbox port. Sync default-
// succeeds and dispatches three progress callbacks (resolving / preparing /
// installing) to exercise the OnProgress path; tests can override syncFunc /
// runFunc to inject failures or specific outputs.
//
// fakeSandbox 是 Sandbox 端口的 record-and-respond fake。Sync 默认成功并依次
// 触发三个 progress callback（resolving / preparing / installing）覆盖
// OnProgress 路径；测试可覆写 syncFunc / runFunc 注入失败或特定输出。
type fakeSandbox struct {
	mu sync.Mutex

	pythonPath string

	syncCalls   []sandboxinfra.SyncRequest
	runCalls    []sandboxinfra.RunRequest
	destroys    []string
	destroyEnvs []destroyEnvCall
	writeCodes  []writeCodeCall

	syncFunc func(req sandboxinfra.SyncRequest) error
	runFunc  func(req sandboxinfra.RunRequest) (*forgedomain.ExecutionResult, error)
}

type destroyEnvCall struct{ ForgeID, EnvID string }
type writeCodeCall struct{ ForgeID, VersionID, Code, EntryFunction string }

func (s *fakeSandbox) PythonPath() string { return s.pythonPath }

func (s *fakeSandbox) Sync(ctx context.Context, req sandboxinfra.SyncRequest) error {
	s.mu.Lock()
	s.syncCalls = append(s.syncCalls, req)
	fn := s.syncFunc
	s.mu.Unlock()
	if fn != nil {
		return fn(req)
	}
	// Default success path — drive OnProgress through the three uv stages
	// so SyncEnvForVersion's progress-write loop is exercised.
	// 默认成功——通过三个 uv stage 触发 OnProgress 让 SyncEnvForVersion 的
	// 进度写循环被覆盖。
	if req.OnProgress != nil {
		req.OnProgress("resolving", "Resolved 0 packages in 1ms")
		req.OnProgress("preparing", "Prepared 0 packages in 1ms")
		req.OnProgress("installing", "Installed 0 packages in 1ms")
	}
	return nil
}

func (s *fakeSandbox) Run(ctx context.Context, req sandboxinfra.RunRequest) (*forgedomain.ExecutionResult, error) {
	s.mu.Lock()
	s.runCalls = append(s.runCalls, req)
	fn := s.runFunc
	s.mu.Unlock()
	if fn != nil {
		return fn(req)
	}
	return &forgedomain.ExecutionResult{OK: true, Output: "default", ElapsedMs: 1}, nil
}

func (s *fakeSandbox) WriteCodeFile(ctx context.Context, forgeID, versionID, code, entry string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeCodes = append(s.writeCodes, writeCodeCall{forgeID, versionID, code, entry})
	return nil
}

func (s *fakeSandbox) Destroy(ctx context.Context, forgeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.destroys = append(s.destroys, forgeID)
	return nil
}

func (s *fakeSandbox) DestroyEnv(ctx context.Context, forgeID, envID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.destroyEnvs = append(s.destroyEnvs, destroyEnvCall{forgeID, envID})
	return nil
}

// Compile-time interface satisfaction.
var _ Sandbox = (*fakeSandbox)(nil)

// ── helpers ──────────────────────────────────────────────────────────────────

const userAlice = "u-alice"

func ctxAlice() context.Context {
	return reqctxpkg.SetUserID(context.Background(), userAlice)
}

func newServiceWithFakes(t *testing.T) (*Service, *fakeSandbox, *forgestore.Store) {
	t.Helper()
	db, err := dbinfra.Open(dbinfra.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = dbinfra.Close(db) })
	if err := dbinfra.Migrate(db,
		&forgedomain.Forge{},
		&forgedomain.ForgeVersion{},
		&forgedomain.ForgeTestCase{},
		&forgedomain.ForgeExecution{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := forgestore.New(db)
	sb := &fakeSandbox{}
	svc := NewService(repo, sb, nil, nil, zap.NewNop())
	return svc, sb, repo
}

// stdlibCreateInput returns a minimal CreateInput with a parseable single-
// function Python source and no third-party deps.
//
// stdlibCreateInput 返回最小的 CreateInput——单函数可解析 Python 源 + 无第三方依赖。
func stdlibCreateInput(name string) CreateInput {
	return CreateInput{
		Name:        name,
		Description: "test forge",
		Code:        "def " + name + "():\n    return 1\n",
	}
}

// ── Create ───────────────────────────────────────────────────────────────────

func TestCreate_BasicFlowDrivesActiveVersionAndSync(t *testing.T) {
	svc, sb, repo := newServiceWithFakes(t)

	f, err := svc.Create(ctxAlice(), stdlibCreateInput("hello"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if f.ActiveVersionID == "" {
		t.Fatal("ActiveVersionID should be set after Create")
	}
	if f.VersionCount != 1 {
		t.Errorf("VersionCount = %d, want 1", f.VersionCount)
	}
	// fakeSandbox.Sync default-succeeds → EnvStatus should land on ready,
	// computed-attached on the returned forge.
	// fakeSandbox.Sync 默认成功 → EnvStatus 应落 ready，已 attach 到返回的 forge。
	if f.EnvStatus != forgedomain.EnvStatusReady {
		t.Errorf("EnvStatus = %q, want ready (EnvError=%q)", f.EnvStatus, f.EnvError)
	}

	// Persist roundtrip: ActiveVersion should match v1.
	// 落库回查：ActiveVersion 应匹配 v1。
	av, err := repo.GetVersionByID(ctxAlice(), f.ActiveVersionID)
	if err != nil {
		t.Fatalf("GetVersionByID: %v", err)
	}
	if av.Status != forgedomain.VersionStatusAccepted || av.Version == nil || *av.Version != 1 {
		t.Errorf("v1 not accepted: status=%q version=%v", av.Status, av.Version)
	}
	if av.EnvStatus != forgedomain.EnvStatusReady {
		t.Errorf("v1 EnvStatus = %q, want ready", av.EnvStatus)
	}

	// Sandbox should have been called exactly once for sync.
	// sandbox.Sync 应恰好被调一次。
	if got := len(sb.syncCalls); got != 1 {
		t.Errorf("expected 1 Sync call, got %d", got)
	}
	if sb.syncCalls[0].EnvID == "" {
		t.Errorf("Sync called with empty EnvID")
	}
	if sb.syncCalls[0].EnvID != av.EnvID {
		t.Errorf("Sync EnvID %q != version EnvID %q", sb.syncCalls[0].EnvID, av.EnvID)
	}
}

func TestCreate_SyncFailureLeavesForgeInFailedState(t *testing.T) {
	svc, sb, repo := newServiceWithFakes(t)
	sb.syncFunc = func(sandboxinfra.SyncRequest) error {
		return &sandboxinfra.SyncError{
			Cause:  errors.New("exit 1"),
			Stderr: "× resolution failed: package not found",
		}
	}

	f, err := svc.Create(ctxAlice(), stdlibCreateInput("broken"))
	// Create itself does NOT propagate the sync error — punt-to-AI: forge
	// stays around with EnvStatus=failed for the LLM to read.
	// Create 本身不传播 sync error——punt-to-AI：forge 留下 EnvStatus=failed
	// 给 LLM 读。
	if err != nil {
		t.Fatalf("Create should swallow sync error, got %v", err)
	}
	if f.ActiveVersionID == "" {
		t.Fatal("ActiveVersionID should still be set even when sync fails")
	}
	if f.EnvStatus != forgedomain.EnvStatusFailed {
		t.Errorf("EnvStatus = %q, want failed", f.EnvStatus)
	}
	if !contains(f.EnvError, "resolution failed") {
		t.Errorf("EnvError should carry stderr text, got %q", f.EnvError)
	}

	// DB roundtrip confirms the failed state persisted.
	// DB 回查确认 failed 状态已落库。
	av, _ := repo.GetVersionByID(ctxAlice(), f.ActiveVersionID)
	if av.EnvStatus != forgedomain.EnvStatusFailed {
		t.Errorf("DB EnvStatus = %q, want failed", av.EnvStatus)
	}
}

func TestCreate_DependenciesPropagatedToSandbox(t *testing.T) {
	svc, sb, _ := newServiceWithFakes(t)

	in := stdlibCreateInput("uses_pandas")
	in.Dependencies = []string{"pandas>=2.0", "requests"}
	in.PythonVersion = ">=3.12"

	if _, err := svc.Create(ctxAlice(), in); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if got := len(sb.syncCalls); got != 1 {
		t.Fatalf("expected 1 Sync call, got %d", got)
	}
	got := sb.syncCalls[0]
	if len(got.Dependencies) != 2 || got.Dependencies[0] != "pandas>=2.0" || got.Dependencies[1] != "requests" {
		t.Errorf("Sync deps wrong: %v", got.Dependencies)
	}
	if got.PythonVersion != ">=3.12" {
		t.Errorf("Sync PythonVersion = %q, want >=3.12", got.PythonVersion)
	}
}

// ── CreateDraft ──────────────────────────────────────────────────────────────

func TestCreateDraft_NoVersionNoActiveID(t *testing.T) {
	svc, sb, repo := newServiceWithFakes(t)

	f, err := svc.CreateDraft(ctxAlice(), CreateInput{Name: "drafty", Description: "d"})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if f.ActiveVersionID != "" {
		t.Errorf("draft ActiveVersionID should be empty, got %q", f.ActiveVersionID)
	}
	if f.VersionCount != 0 {
		t.Errorf("draft VersionCount = %d, want 0", f.VersionCount)
	}
	// No version row should exist.
	// 不应有 version 行存在。
	versions, _ := repo.ListAcceptedVersions(ctxAlice(), f.ID)
	if len(versions) != 0 {
		t.Errorf("draft should have 0 accepted versions, got %d", len(versions))
	}
	if len(sb.syncCalls) != 0 {
		t.Errorf("draft must not trigger sandbox.Sync, got %d calls", len(sb.syncCalls))
	}
}

// ── CreatePending ────────────────────────────────────────────────────────────

func TestCreatePending_OnDraftFreshSync(t *testing.T) {
	svc, sb, _ := newServiceWithFakes(t)

	draft, err := svc.CreateDraft(ctxAlice(), CreateInput{Name: "drafty", Description: "d"})
	if err != nil {
		t.Fatal(err)
	}

	pv, err := svc.CreatePending(ctxAlice(), draft.ID, PendingSnapshot{
		Code:         "def hi():\n    return 'hi'\n",
		ChangeReason: "initial draft",
		Dependencies: []string{"requests"},
	})
	if err != nil {
		t.Fatalf("CreatePending: %v", err)
	}
	if pv.EnvStatus != forgedomain.EnvStatusReady {
		t.Errorf("pending EnvStatus = %q, want ready", pv.EnvStatus)
	}
	if got := len(sb.syncCalls); got != 1 {
		t.Errorf("expected 1 Sync call, got %d", got)
	}
}

func TestCreatePending_InheritsActiveDeps(t *testing.T) {
	svc, sb, _ := newServiceWithFakes(t)

	in := stdlibCreateInput("base")
	in.Dependencies = []string{"requests"}
	in.PythonVersion = ">=3.11"
	f, err := svc.Create(ctxAlice(), in)
	if err != nil {
		t.Fatal(err)
	}
	sb.mu.Lock()
	sb.syncCalls = nil
	sb.mu.Unlock()

	// edit_forge with no deps in snap → CreatePending should inherit.
	// edit_forge 不带 deps → CreatePending 应继承 active 的 deps。
	pv, err := svc.CreatePending(ctxAlice(), f.ID, PendingSnapshot{
		Code:         "def base():\n    return 2\n",
		ChangeReason: "tweak",
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := len(sb.syncCalls); got != 1 {
		t.Fatalf("expected 1 Sync, got %d", got)
	}
	got := sb.syncCalls[0]
	if len(got.Dependencies) != 1 || got.Dependencies[0] != "requests" {
		t.Errorf("inheritance failed: deps=%v", got.Dependencies)
	}
	if got.PythonVersion != ">=3.11" {
		t.Errorf("inheritance failed: python=%q", got.PythonVersion)
	}
	// Same deps + python → same EnvID as v1; venv reuse.
	// deps + python 相同 → EnvID 跟 v1 一致；venv 复用。
	if pv.EnvID == "" {
		t.Error("pending EnvID should be set")
	}
}

func TestCreatePending_SnapDepsOverrideActive(t *testing.T) {
	svc, sb, _ := newServiceWithFakes(t)

	in := stdlibCreateInput("base")
	in.Dependencies = []string{"requests"}
	if _, err := svc.Create(ctxAlice(), in); err != nil {
		t.Fatal(err)
	}
	f, _ := svc.Get(ctxAlice(), getOnlyForgeID(t, svc))
	sb.mu.Lock()
	sb.syncCalls = nil
	sb.mu.Unlock()

	// Snap supplies different deps — should win over inherited.
	// snap 提供不同 deps——应覆盖继承。
	if _, err := svc.CreatePending(ctxAlice(), f.ID, PendingSnapshot{
		Code:         "def base():\n    return 3\n",
		Dependencies: []string{"polars"},
	}); err != nil {
		t.Fatal(err)
	}

	if got := sb.syncCalls[0].Dependencies; len(got) != 1 || got[0] != "polars" {
		t.Errorf("snap deps should win: got %v", got)
	}
}

func TestCreatePending_ConflictWhenPendingAlreadyExists(t *testing.T) {
	svc, _, _ := newServiceWithFakes(t)

	f, err := svc.Create(ctxAlice(), stdlibCreateInput("conf"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreatePending(ctxAlice(), f.ID, PendingSnapshot{Code: "def conf():\n    return 9\n"}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.CreatePending(ctxAlice(), f.ID, PendingSnapshot{Code: "def conf():\n    return 10\n"})
	if !errors.Is(err, forgedomain.ErrPendingConflict) {
		t.Errorf("expected ErrPendingConflict on second pending, got %v", err)
	}
}

// ── AcceptPending ────────────────────────────────────────────────────────────

func TestAcceptPending_HappyPathSwitchesActive(t *testing.T) {
	svc, _, repo := newServiceWithFakes(t)

	f, err := svc.Create(ctxAlice(), stdlibCreateInput("base"))
	if err != nil {
		t.Fatal(err)
	}
	pv, err := svc.CreatePending(ctxAlice(), f.ID, PendingSnapshot{Code: "def base():\n    return 99\n"})
	if err != nil {
		t.Fatal(err)
	}

	accepted, err := svc.AcceptPending(ctxAlice(), f.ID)
	if err != nil {
		t.Fatalf("AcceptPending: %v", err)
	}
	if accepted.ActiveVersionID != pv.ID {
		t.Errorf("ActiveVersionID = %q, want %q", accepted.ActiveVersionID, pv.ID)
	}
	if accepted.VersionCount != 2 {
		t.Errorf("VersionCount = %d, want 2", accepted.VersionCount)
	}
	if accepted.EnvStatus != forgedomain.EnvStatusReady {
		t.Errorf("EnvStatus on accepted = %q, want ready", accepted.EnvStatus)
	}

	// Pending should be gone; accepted v2 should be there.
	// pending 应消失；accepted v2 应在。
	versions, _ := repo.ListAcceptedVersions(ctxAlice(), f.ID)
	if len(versions) != 2 {
		t.Errorf("expected 2 accepted versions, got %d", len(versions))
	}
}

func TestAcceptPending_NotReadyRejected(t *testing.T) {
	svc, sb, _ := newServiceWithFakes(t)

	f, err := svc.Create(ctxAlice(), stdlibCreateInput("base"))
	if err != nil {
		t.Fatal(err)
	}
	// Make the next sync fail so the pending lands in failed state.
	// 让下一次 sync 失败，pending 落 failed。
	sb.syncFunc = func(sandboxinfra.SyncRequest) error {
		return &sandboxinfra.SyncError{Stderr: "× boom"}
	}
	if _, err := svc.CreatePending(ctxAlice(), f.ID, PendingSnapshot{Code: "def base():\n    return 7\n"}); err != nil {
		t.Fatal(err)
	}
	sb.syncFunc = nil // restore

	_, err = svc.AcceptPending(ctxAlice(), f.ID)
	if !errors.Is(err, forgedomain.ErrEnvFailed) {
		t.Errorf("failed pending should yield ErrEnvFailed, got %v", err)
	}
}

func TestAcceptPending_SyncingRejected(t *testing.T) {
	svc, sb, repo := newServiceWithFakes(t)

	f, err := svc.Create(ctxAlice(), stdlibCreateInput("base"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreatePending(ctxAlice(), f.ID, PendingSnapshot{Code: "def base():\n    return 7\n"}); err != nil {
		t.Fatal(err)
	}

	// Force pending row into syncing state directly to simulate "user
	// clicked accept while sync still running".
	// 直接强制 pending 行进 syncing 模拟"sync 还在跑时 user 点 accept"。
	pending, _ := svc.GetActivePending(ctxAlice(), f.ID)
	_ = repo.UpdateVersionEnvStatus(ctxAlice(), pending.ID, forgedomain.EnvStatusSyncing, "")
	sb.mu.Lock()
	sb.syncCalls = nil
	sb.mu.Unlock()

	_, err = svc.AcceptPending(ctxAlice(), f.ID)
	if !errors.Is(err, forgedomain.ErrEnvNotReady) {
		t.Errorf("syncing pending should yield ErrEnvNotReady, got %v", err)
	}
}

// ── RejectPending ────────────────────────────────────────────────────────────

// TestRejectPending_DraftFirstPendingDeletesForge verifies that rejecting the
// first pending of a freshly-created draft (created via create_forge → user
// hits reject before any version is accepted) cleans up the entire forge —
// otherwise the user's library would accumulate empty-shell forges with no
// runnable code.
//
// TestRejectPending_DraftFirstPendingDeletesForge：用户拒绝 create_forge
// 首份代码时整个 forge 该消失，否则库里堆积无代码空壳。
func TestRejectPending_DraftFirstPendingDeletesForge(t *testing.T) {
	svc, sb, _ := newServiceWithFakes(t)

	draft, err := svc.CreateDraft(ctxAlice(), CreateInput{Name: "drafty", Description: "d"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreatePending(ctxAlice(), draft.ID, PendingSnapshot{
		Code:         "def hi():\n    return 1\n",
		ChangeReason: "initial",
	}); err != nil {
		t.Fatal(err)
	}

	if err := svc.RejectPending(ctxAlice(), draft.ID); err != nil {
		t.Fatalf("RejectPending: %v", err)
	}

	if _, err := svc.Get(ctxAlice(), draft.ID); !errors.Is(err, forgedomain.ErrNotFound) {
		t.Errorf("draft forge should be deleted after rejecting first pending, got %v", err)
	}
	if len(sb.destroys) != 1 || sb.destroys[0] != draft.ID {
		t.Errorf("expected sandbox.Destroy(%q) once, got %v", draft.ID, sb.destroys)
	}
}

// TestRejectPending_ActiveForgeKeepsAlive verifies a forge that already has
// an accepted version stays alive when its (later edit_forge) pending is
// rejected — the user keeps the prior good version.
//
// TestRejectPending_ActiveForgeKeepsAlive：已有 active 版本的 forge 在 edit
// 产生的 pending 被拒后保留 prior 版本。
func TestRejectPending_ActiveForgeKeepsAlive(t *testing.T) {
	svc, sb, _ := newServiceWithFakes(t)

	f, err := svc.Create(ctxAlice(), stdlibCreateInput("base"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreatePending(ctxAlice(), f.ID, PendingSnapshot{
		Code: "def base():\n    return 99\n",
	}); err != nil {
		t.Fatal(err)
	}
	sb.mu.Lock()
	sb.destroys = nil
	sb.mu.Unlock()

	if err := svc.RejectPending(ctxAlice(), f.ID); err != nil {
		t.Fatalf("RejectPending: %v", err)
	}

	got, err := svc.Get(ctxAlice(), f.ID)
	if err != nil {
		t.Fatalf("active forge should still exist, got %v", err)
	}
	if got.Pending != nil {
		t.Error("Pending should be detached after reject")
	}
	if len(sb.destroys) != 0 {
		t.Errorf("active forge should NOT trigger sandbox.Destroy, got %v", sb.destroys)
	}
}

// ── RevertToVersion ──────────────────────────────────────────────────────────

func TestRevertToVersion_ReadyEnvSkipsSync(t *testing.T) {
	svc, sb, _ := newServiceWithFakes(t)

	// Build v1 (ready) → v2 (ready)
	f, err := svc.Create(ctxAlice(), stdlibCreateInput("base"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreatePending(ctxAlice(), f.ID, PendingSnapshot{Code: "def base():\n    return 22\n"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AcceptPending(ctxAlice(), f.ID); err != nil {
		t.Fatal(err)
	}
	sb.mu.Lock()
	sb.syncCalls = nil
	sb.mu.Unlock()

	// Revert to v1 — same EnvID (stdlib), env still ready.
	// Revert 到 v1——同 EnvID（stdlib），env 仍 ready。
	reverted, err := svc.RevertToVersion(ctxAlice(), f.ID, 1)
	if err != nil {
		t.Fatalf("RevertToVersion: %v", err)
	}
	if reverted.EnvStatus != forgedomain.EnvStatusReady {
		t.Errorf("reverted EnvStatus = %q, want ready", reverted.EnvStatus)
	}
	if got := len(sb.syncCalls); got != 0 {
		t.Errorf("ready revert should not trigger sandbox.Sync, got %d calls", got)
	}
}

func TestRevertToVersion_EvictedTriggersSync(t *testing.T) {
	svc, sb, repo := newServiceWithFakes(t)

	if _, err := svc.Create(ctxAlice(), stdlibCreateInput("base")); err != nil {
		t.Fatal(err)
	}
	id := getOnlyForgeID(t, svc)
	v1, _ := repo.GetVersion(ctxAlice(), id, 1)

	// Create v2 with different deps so EnvID differs from v1.
	// 用不同 deps 建 v2，让 EnvID 跟 v1 不同。
	if _, err := svc.CreatePending(ctxAlice(), id, PendingSnapshot{
		Code:         "def base():\n    return 33\n",
		Dependencies: []string{"requests"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AcceptPending(ctxAlice(), id); err != nil {
		t.Fatal(err)
	}

	// Now manually mark v1 as evicted (simulating trim).
	// 手工把 v1 标 evicted（模拟 trim）。
	_ = repo.UpdateVersionEnvStatus(ctxAlice(), v1.ID, forgedomain.EnvStatusEvicted, "")
	sb.mu.Lock()
	sb.syncCalls = nil
	sb.mu.Unlock()

	if _, err := svc.RevertToVersion(ctxAlice(), id, 1); err != nil {
		t.Fatalf("RevertToVersion evicted: %v", err)
	}
	if got := len(sb.syncCalls); got != 1 {
		t.Errorf("evicted revert should trigger 1 Sync, got %d", got)
	}
}

// ── Update ───────────────────────────────────────────────────────────────────

func TestUpdate_CodeOnlyKeepsActiveEnvFields(t *testing.T) {
	svc, sb, repo := newServiceWithFakes(t)

	in := stdlibCreateInput("base")
	in.Dependencies = []string{"requests"}
	f, err := svc.Create(ctxAlice(), in)
	if err != nil {
		t.Fatal(err)
	}
	originalActiveID := f.ActiveVersionID
	originalActive, _ := repo.GetVersionByID(ctxAlice(), originalActiveID)
	sb.mu.Lock()
	sb.syncCalls = nil
	sb.mu.Unlock()

	newCode := "def base():\n    return 'updated'\n"
	updated, err := svc.Update(ctxAlice(), f.ID, UpdateInput{Code: &newCode})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.ActiveVersionID == originalActiveID {
		t.Error("ActiveVersionID should advance to the new version")
	}

	newActive, _ := repo.GetVersionByID(ctxAlice(), updated.ActiveVersionID)
	if newActive.EnvID != originalActive.EnvID {
		t.Errorf("EnvID should be inherited (deps unchanged): new=%q, old=%q", newActive.EnvID, originalActive.EnvID)
	}
	if newActive.Dependencies != originalActive.Dependencies {
		t.Errorf("Dependencies should be inherited: new=%q, old=%q", newActive.Dependencies, originalActive.Dependencies)
	}
	if newActive.EnvStatus != forgedomain.EnvStatusReady {
		t.Errorf("EnvStatus should be inherited as ready, got %q", newActive.EnvStatus)
	}
	// Update doesn't trigger sync — venv reuses the same EnvID.
	// Update 不触发 sync——venv 复用同 EnvID。
	if got := len(sb.syncCalls); got != 0 {
		t.Errorf("Update with same deps must not trigger Sync, got %d calls", got)
	}
}

// ── Delete ───────────────────────────────────────────────────────────────────

func TestDelete_TriggersSandboxDestroy(t *testing.T) {
	svc, sb, _ := newServiceWithFakes(t)

	f, err := svc.Create(ctxAlice(), stdlibCreateInput("doomed"))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctxAlice(), f.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(sb.destroys) != 1 || sb.destroys[0] != f.ID {
		t.Errorf("expected sandbox.Destroy(%q), got %v", f.ID, sb.destroys)
	}
}

// ── RunForge ─────────────────────────────────────────────────────────────────

func TestRunForge_DraftRejected(t *testing.T) {
	svc, _, _ := newServiceWithFakes(t)

	draft, err := svc.CreateDraft(ctxAlice(), CreateInput{Name: "drafty", Description: "d"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.RunForge(ctxAlice(), draft.ID, map[string]any{})
	if !errors.Is(err, forgedomain.ErrEnvNotReady) {
		t.Errorf("draft Run should return ErrEnvNotReady, got %v", err)
	}
}

func TestRunForge_PassesActiveVersionAndEnvID(t *testing.T) {
	svc, sb, repo := newServiceWithFakes(t)

	f, err := svc.Create(ctxAlice(), stdlibCreateInput("runme"))
	if err != nil {
		t.Fatal(err)
	}
	av, _ := repo.GetVersionByID(ctxAlice(), f.ActiveVersionID)
	sb.mu.Lock()
	sb.runCalls = nil
	sb.mu.Unlock()

	if _, err := svc.RunForge(ctxAlice(), f.ID, map[string]any{"x": 1}); err != nil {
		t.Fatalf("RunForge: %v", err)
	}
	if got := len(sb.runCalls); got != 1 {
		t.Fatalf("expected 1 Run call, got %d", got)
	}
	got := sb.runCalls[0]
	if got.ForgeID != f.ID {
		t.Errorf("Run ForgeID = %q, want %q", got.ForgeID, f.ID)
	}
	if got.VersionID != av.ID {
		t.Errorf("Run VersionID = %q, want %q (active)", got.VersionID, av.ID)
	}
	if got.EnvID != av.EnvID {
		t.Errorf("Run EnvID = %q, want %q (active)", got.EnvID, av.EnvID)
	}
}

// ── trimEnvBuffer ────────────────────────────────────────────────────────────

func TestTrimEnvBuffer_EvictsLRUBeyondCap(t *testing.T) {
	svc, sb, repo := newServiceWithFakes(t)

	// Build forge with a sequence of distinct EnvIDs:
	//   v1: deps=[]              → env_A
	//   v2: deps=[req-a]         → env_B
	//   v3: deps=[req-b]         → env_C
	//   v4: deps=[req-c]         → env_D  (triggers eviction of env_A)
	//
	// 用一连串不同 EnvID 建 forge：v1..v4，第 4 个触发驱逐 env_A。
	f, err := svc.Create(ctxAlice(), stdlibCreateInput("trim"))
	if err != nil {
		t.Fatal(err)
	}
	v1Active, _ := repo.GetVersionByID(ctxAlice(), f.ActiveVersionID)
	v1EnvID := v1Active.EnvID

	for i, dep := range []string{"req-a", "req-b", "req-c"} {
		if _, err := svc.CreatePending(ctxAlice(), f.ID, PendingSnapshot{
			Code:         "def trim():\n    return 1\n",
			Dependencies: []string{dep},
		}); err != nil {
			t.Fatalf("create pending %d: %v", i, err)
		}
		if _, err := svc.AcceptPending(ctxAlice(), f.ID); err != nil {
			t.Fatalf("accept pending %d: %v", i, err)
		}
	}

	// MaxEnvIDsPerForge == 3, so accepting v4 should have evicted v1's EnvID.
	// MaxEnvIDsPerForge == 3，所以 accept v4 应驱逐 v1 的 EnvID。
	if forgedomain.MaxEnvIDsPerForge != 3 {
		t.Skipf("test assumes MaxEnvIDsPerForge=3, got %d", forgedomain.MaxEnvIDsPerForge)
	}
	if len(sb.destroyEnvs) == 0 {
		t.Fatal("expected at least one DestroyEnv call after trim")
	}
	// Find a destroy targeting v1's EnvID.
	// 找针对 v1 EnvID 的 destroy。
	found := false
	for _, c := range sb.destroyEnvs {
		if c.ForgeID == f.ID && c.EnvID == v1EnvID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("v1's EnvID (%q) should have been destroyed; destroyEnvs=%v", v1EnvID, sb.destroyEnvs)
	}
}

// ── attachActiveEnv ──────────────────────────────────────────────────────────

func TestGet_AttachesActiveEnvFields(t *testing.T) {
	svc, _, _ := newServiceWithFakes(t)

	created, err := svc.Create(ctxAlice(), stdlibCreateInput("attach"))
	if err != nil {
		t.Fatal(err)
	}

	got, err := svc.Get(ctxAlice(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.EnvStatus != forgedomain.EnvStatusReady {
		t.Errorf("Get should attach active EnvStatus=ready, got %q", got.EnvStatus)
	}
}

func TestGet_DraftHasEmptyEnvFields(t *testing.T) {
	svc, _, _ := newServiceWithFakes(t)

	created, err := svc.CreateDraft(ctxAlice(), CreateInput{Name: "d", Description: "d"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(ctxAlice(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.EnvStatus != "" || got.EnvSyncStage != "" {
		t.Errorf("draft should have empty Env* fields; got status=%q stage=%q", got.EnvStatus, got.EnvSyncStage)
	}
}

// ── small util ───────────────────────────────────────────────────────────────

func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// getOnlyForgeID asserts the user has exactly one forge and returns its ID.
// Helper for tests that don't care about the specific ID.
//
// getOnlyForgeID 断言用户有且仅有一个 forge，返其 ID。供不关心具体 ID 的测试用。
func getOnlyForgeID(t *testing.T, svc *Service) string {
	t.Helper()
	all, err := svc.ListAll(ctxAlice())
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected exactly 1 forge, got %d", len(all))
	}
	return all[0].ID
}
