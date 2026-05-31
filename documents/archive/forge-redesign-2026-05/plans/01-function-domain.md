# Function Domain Implementation Plan

> ⚠️ **STATUS: 主体已 merge(2026-05-11,13 commits 直推 main),后部分 env 相关章节已被 redesign 取代**
>
> Plan 01 的核心交付(domain / app / store / 7 LLM tools / 12 HTTP / D22 执行日志 / pipeline test)全部落地;forge 域已删除。
>
> 但 **env 模型在 2026-05-12 大幅修订**(详见 [`../discussions/2026-05-12-env-and-sse-rework.md`](../discussions/2026-05-12-env-and-sse-rework.md) §D-E):
> - EnvID 改为每 Version 独立生成(`fnenv_<16hex>`,跟 version_id 1:1 但解耦);原本 = sha256(deps, python) 共享逻辑已删
> - env sync **同步发生在 LLM tool 内**,删 SyncEnvForVersion / Resync 异步路径
> - AcceptPending **纯指针翻转**(env 已在 edit 阶段装好)
> - Edit 改 "iterate same pending",删 ErrPendingConflict
> - create_function / edit_function 加内部 env-fix loop(主 chat LLM 改 deps,maxAttempts=3)
> - `ops=[]` 显式语义 = 强制重建 env(D-redo-22),取代 fake set_meta hack
> - HTTP `:resync` 端点删除
> - Service.Create/Edit 前置 sandbox ping,失败 503 硬拒不建 entity
>
> 本文档保留**实施过程记录**(branching / phase / 测试驱动顺序),env 设计细节请以 `02-function.md` + 讨论文档为准。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace existing `forge` domain entirely with new `function` domain — Python sandbox 函数 with ops-driven streaming 锻造,7 LLM tools,~12 HTTP endpoints,sandbox v2 集成,catalog source。新 entity / table / API / LLM tool 全套,**不复用 forge 历史代码**(per spec D5)。

**Architecture:** 4-layer clean arch(`domain/function` → `app/function` → `infra/store/function` + `transport/httpapi/handlers`)。LLM tools 用 ops-driven 流式锻造(每 op emit progress block delta,前端 incremental 渲染)。Function code = 单 Python 函数,LLM 自报 parameters schema,后端 AST 校验对齐(D14)。Sandbox v2 Python EnvManager 管 venv,env_id = sha256(deps + python_version),同 deps 跨 Function 共享。

**Tech Stack:** Go 1.25,GORM v2,modernc.org/sqlite,sandbox v2 (mise + Python uv),eventlog 协议(5 events × 6 block types),pkg/idgen for IDs,pkg/llmclient for LLM resolution,infra/crypto AES-GCM(N/A V1 但 dependency 上有),pkg/eventlog Emitter,pkg/reqctx for caller-context。

**关联**:[`02-function.md`](../02-function.md) 完整 spec / [`01-shared-tool-interface.md`](../01-shared-tool-interface.md) 工具接口 / [`07-notifications-and-eventlog.md`](../07-notifications-and-eventlog.md) eventlog scope / 项目根 `CLAUDE.md` S/T/N/D/E 规范。

---

## Phase 0:Branch + 验证现状

### Task 1:创建 feature branch + 验证 spec 已 commit

**Files:** —

- [ ] **Step 1: 验证现状 clean**

```bash
cd /Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify && git status
```

Expected: `nothing to commit, working tree clean`(spec commit `f98c152` 应已落)

- [ ] **Step 2: 创建分支**

```bash
git checkout -b feature/function-domain
```

Expected: `Switched to a new branch 'feature/function-domain'`

- [ ] **Step 3: 验证 spec 文件存在**

```bash
ls documents/version-1.2/adhoc-topic-documents/forge_redesign/02-function.md
```

Expected: 文件列出

---

## Phase 1:Domain Layer(`internal/domain/function/`)

### Task 2:Function entity + 13 sentinels

**Files:**
- Create: `backend/internal/domain/function/function.go`

- [ ] **Step 1: 写 entity + sentinels**

```go
// Package function defines the Function domain — Python sandbox functions
// with declared parameters schema, ops-driven streaming forging.
//
// Package function 定义 Function domain — Python 沙箱函数,LLM 自报
// parameters schema,通过 ops 流式锻造。
package function

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Function is a forged Python function entity (per-user, name-unique while
// not deleted). Code/parameters/return_schema/deps live on FunctionVersion.
//
// Function 是一个锻造的 Python 函数实体(per-user,未软删时 name 唯一)。
// 代码/参数/返回 schema/依赖均在 FunctionVersion 上。
type Function struct {
	ID              string `gorm:"primaryKey"`
	UserID          string `gorm:"index;not null"`
	Name            string `gorm:"not null"`
	Description     string
	Tags            []string `gorm:"serializer:json"`
	ActiveVersionID string   `gorm:"index"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt `gorm:"index"`

	// Computed fields populated by service layer.
	// 计算字段由 service.attachPending / attachActiveEnv 填充。
	Pending       *Version   `gorm:"-"`
	EnvStatus     string     `gorm:"-"`
	EnvError      string     `gorm:"-"`
	EnvSyncedAt   *time.Time `gorm:"-"`
	EnvSyncStage  string     `gorm:"-"`
	EnvSyncDetail string     `gorm:"-"`
}

// Sentinel errors. Wire codes registered in transport/httpapi/response/errmap.go.
//
// 哨兵错误。HTTP wire code 在 errmap.go 登记。
var (
	ErrNotFound             = errors.New("function: not found")
	ErrDuplicateName        = errors.New("function: duplicate name")
	ErrVersionNotFound      = errors.New("function: version not found")
	ErrPendingNotFound      = errors.New("function: pending not found")
	ErrPendingConflict      = errors.New("function: pending conflict")
	ErrRunFailed            = errors.New("function: run failed")
	ErrASTParseError        = errors.New("function: AST parse error")
	ErrNoActiveVersion      = errors.New("function: no active version")
	ErrEnvNotReady          = errors.New("function: env not ready")
	ErrEnvFailed            = errors.New("function: env failed")
	ErrDependencyResolution = errors.New("function: dependency resolution failed")
	ErrSandboxUnavailable   = errors.New("function: sandbox unavailable")
	ErrOpInvalid            = errors.New("function: op invalid")
)
```

- [ ] **Step 2: 验证编译**

```bash
cd backend && go build ./internal/domain/function/
```

Expected: 无输出(成功)

- [ ] **Step 3: Commit + push**

```bash
cd /Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify
git add backend/internal/domain/function/function.go
git commit -m "feat(function): domain entity + 13 sentinels"
git push origin feature/function-domain
```

---

### Task 3:FunctionVersion entity + ParameterSpec

**Files:**
- Create: `backend/internal/domain/function/version.go`

- [ ] **Step 1: 写 Version + ParameterSpec**

```go
package function

import "time"

// VersionStatus 是 FunctionVersion 状态的封闭枚举(D3 — DB CHECK 约束)。
//
// VersionStatus is a closed enum for FunctionVersion status.
const (
	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusRejected = "rejected"
)

// EnvStatus 是 venv 同步状态(per FunctionVersion;5 值,app 层校验)。
//
// EnvStatus tracks venv sync lifecycle (V1: app-level whitelist).
const (
	EnvStatusPending  = "pending"
	EnvStatusSyncing  = "syncing"
	EnvStatusReady    = "ready"
	EnvStatusFailed   = "failed"
	EnvStatusEvicted  = "evicted"
)

// DefaultPythonVersion 是 LLM 不指定 python_version 时的回退(PEP 440 spec)。
//
// DefaultPythonVersion fallback when LLM omits python_version on create.
const DefaultPythonVersion = ">=3.12"

// Version is a snapshot of code + parameters + deps at one point in time.
// status=accepted has version int (sequential per Function); pending/rejected version=NULL.
//
// Version 是 code+parameters+deps 在某时刻的快照。
// status=accepted 的 version 整数递增;pending/rejected 时 version=NULL。
type Version struct {
	ID            string  `gorm:"primaryKey"`
	FunctionID    string  `gorm:"index;not null"`
	Status        string  `gorm:"not null"` // CHECK in (pending, accepted, rejected) — applied via schema_extras
	Version       *int    `gorm:""`         // NULL on pending/rejected
	Code          string
	Parameters    []ParameterSpec `gorm:"serializer:json"`
	ReturnSchema  map[string]any  `gorm:"serializer:json"`
	Dependencies  []string        `gorm:"serializer:json"`
	PythonVersion string
	EnvID         string `gorm:"index"`
	EnvStatus     string
	EnvError      string
	EnvSyncedAt   *time.Time
	EnvSyncStage  string
	EnvSyncDetail string
	ChangeReason  string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ParameterSpec 是单个入参的 JSON-Schema-like 描述(LLM 自报,D14)。
//
// ParameterSpec is a JSON-Schema-like description of one input parameter.
type ParameterSpec struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // string | number | integer | boolean | object | array
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
	Default     any    `json:"default,omitempty"`
	Enum        []any  `json:"enum,omitempty"`
}
```

- [ ] **Step 2: 验证编译**

```bash
cd backend && go build ./internal/domain/function/
```

Expected: 无输出

- [ ] **Step 3: Commit + push**

```bash
git add backend/internal/domain/function/version.go
git commit -m "feat(function): Version entity + ParameterSpec + status/env enums"
git push origin feature/function-domain
```

---

### Task 4:Repository 接口 + ListFilter

**Files:**
- Create: `backend/internal/domain/function/repository.go`

- [ ] **Step 1: 写接口**

```go
package function

import (
	"context"

	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
)

// Repository is the persistence port (impl in infra/store/function).
// service-level apply_ops calls this; cross-domain consumers go through
// app/function.Service which exposes a narrower interface.
//
// Repository 是持久化 port(实现在 infra/store/function)。service 层 apply_ops
// 经此调用;跨 domain 消费走 app/function.Service 暴露的更窄接口。
type Repository interface {
	// Function CRUD
	Create(ctx context.Context, f *Function) error
	GetByID(ctx context.Context, userID, id string) (*Function, error)
	GetByName(ctx context.Context, userID, name string) (*Function, error)
	List(ctx context.Context, userID string, filter ListFilter) ([]*Function, *paginationpkg.Cursor, error)
	UpdateMeta(ctx context.Context, f *Function) error // name / description / tags
	SoftDelete(ctx context.Context, userID, id string) error
	SetActiveVersion(ctx context.Context, userID, functionID, versionID string) error

	// Version CRUD
	CreateVersion(ctx context.Context, v *Version) error
	GetVersionByID(ctx context.Context, versionID string) (*Version, error)
	GetVersionByNumber(ctx context.Context, functionID string, versionN int) (*Version, error)
	ListVersions(ctx context.Context, functionID string, filter ListFilter) ([]*Version, *paginationpkg.Cursor, error)
	UpdateVersionEnv(ctx context.Context, versionID string, envStatus, envError, envSyncStage, envSyncDetail string, syncedAt *time.Time) error
	GetPending(ctx context.Context, functionID string) (*Version, error)
	UpdateVersionStatus(ctx context.Context, versionID string, status string, versionN *int) error
	HardDeleteOldestAccepted(ctx context.Context, functionID string, keep int) error // 50 cap
}

// ListFilter is shared by List / ListVersions.
type ListFilter struct {
	Cursor string
	Limit  int
	Status string // optional version status filter
}
```

注意:`time.Time` 引用要 import:

```go
import (
	"context"
	"time"

	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
)
```

- [ ] **Step 2: 验证编译**

```bash
cd backend && go build ./internal/domain/function/
```

Expected: 无输出

- [ ] **Step 3: Commit + push**

```bash
git add backend/internal/domain/function/repository.go
git commit -m "feat(function): Repository port + ListFilter"
git push origin feature/function-domain
```

---

### Task 5:Domain layer 单测(sentinel uniqueness + entity zero values)

**Files:**
- Create: `backend/internal/domain/function/function_test.go`

- [ ] **Step 1: 写测试**

```go
package function

import (
	"errors"
	"strings"
	"testing"
)

func TestSentinels_Unique(t *testing.T) {
	all := []error{
		ErrNotFound, ErrDuplicateName, ErrVersionNotFound, ErrPendingNotFound,
		ErrPendingConflict, ErrRunFailed, ErrASTParseError, ErrNoActiveVersion,
		ErrEnvNotReady, ErrEnvFailed, ErrDependencyResolution, ErrSandboxUnavailable,
		ErrOpInvalid,
	}
	seen := map[string]bool{}
	for _, e := range all {
		msg := e.Error()
		if !strings.HasPrefix(msg, "function: ") {
			t.Errorf("sentinel %q must start with 'function: '", msg)
		}
		if seen[msg] {
			t.Errorf("duplicate sentinel message: %q", msg)
		}
		seen[msg] = true
	}
}

func TestSentinels_ErrorsIsCompatible(t *testing.T) {
	wrapped := errors.New("upstream: " + ErrNotFound.Error())
	if errors.Is(wrapped, ErrNotFound) {
		// errors.New(string) doesn't wrap — confirm string-only nesting doesn't fool errors.Is
		t.Error("string concatenation should NOT satisfy errors.Is")
	}

	// 真包装应该能 unwrap
	wrappedReal := errWrap(ErrNotFound)
	if !errors.Is(wrappedReal, ErrNotFound) {
		t.Errorf("errors.Is failed to unwrap: %v", wrappedReal)
	}
}

func errWrap(err error) error {
	return &wrapped{err: err}
}

type wrapped struct{ err error }

func (w *wrapped) Error() string { return "wrap: " + w.err.Error() }
func (w *wrapped) Unwrap() error { return w.err }
```

- [ ] **Step 2: 跑测试**

```bash
cd backend && go test ./internal/domain/function/ -count=1
```

Expected: `PASS` 2 测试通过

- [ ] **Step 3: Commit + push**

```bash
git add backend/internal/domain/function/function_test.go
git commit -m "test(function): sentinel uniqueness + errors.Is compatibility"
git push origin feature/function-domain
```

---

## Phase 2:Store Layer(`internal/infra/store/function/`)

### Task 6:Repository GORM 实现 + AutoMigrate hook

**Files:**
- Create: `backend/internal/infra/store/function/function.go`

- [ ] **Step 1: 写 Repository 实现**

按 spec [`02-function.md`](../02-function.md) §5 表 schema 实现 GORM 操作。完整代码模板:

```go
// Package function provides the GORM-backed Repository implementation.
//
// Package function 提供 GORM 实现的 Repository。
package function

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	paginationpkg "github.com/sunweilin/forgify/backend/internal/pkg/pagination"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

type Store struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// Create inserts a new Function row. caller owns ID generation.
//
// Create 插入新 Function 行。caller 负责 ID 生成(per pkg/idgen)。
func (s *Store) Create(ctx context.Context, f *functiondomain.Function) error {
	if _, err := reqctxpkg.RequireUserID(ctx); err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Create(f).Error; err != nil {
		// translate UNIQUE violation → ErrDuplicateName
		if isUniqueViolation(err) {
			return functiondomain.ErrDuplicateName
		}
		return fmt.Errorf("functionstore.Create: %w", err)
	}
	return nil
}

// GetByID — soft-delete aware, scoped by userID.
//
// GetByID 软删感知,按 userID 过滤。
func (s *Store) GetByID(ctx context.Context, userID, id string) (*functiondomain.Function, error) {
	var row functiondomain.Function
	err := s.db.WithContext(ctx).Where("user_id = ? AND id = ?", userID, id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, functiondomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("functionstore.GetByID: %w", err)
	}
	return &row, nil
}

// 其余方法(GetByName / List / UpdateMeta / SoftDelete / SetActiveVersion +
// Version CRUD)按同模式实现。完整逐方法代码见 spec §5.1 表字段映射。
//
// (此处省略 ~150 行,实施时按 02-function.md §5.1 + Repository interface 逐方法填。
// 错误返 sentinel,不返 GORM 原始错误;包装格式 fmt.Errorf("functionstore.X: %w", err)。)

// AutoMigrateModels lists the GORM models for db.AutoMigrate registration in main.go.
//
// AutoMigrateModels 列出 main.go 注册 AutoMigrate 用的 GORM models。
func AutoMigrateModels() []interface{} {
	return []interface{}{
		&functiondomain.Function{},
		&functiondomain.Version{},
	}
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// modernc.org/sqlite returns "UNIQUE constraint failed" in error string
	return errors.Is(err, gorm.ErrDuplicatedKey) ||
		(err.Error() != "" && containsAny(err.Error(), "UNIQUE constraint failed", "constraint failed: UNIQUE"))
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(s) >= len(sub) && stringContains(s, sub) {
			return true
		}
	}
	return false
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

注:实施时填全所有 Repository 方法。每个方法的核心逻辑:

- `GetByName`:`Where("user_id = ? AND name = ?", userID, name)`
- `List`:用 `pagination.Cursor` 解析 + 软删过滤 + `created_at DESC` 排序
- `UpdateMeta`:`Updates({"name": f.Name, "description": f.Description, "tags": f.Tags})` 仅 meta 字段
- `SoftDelete`:`Delete(...)` GORM 软删
- `SetActiveVersion`:`UpdateColumn("active_version_id", versionID)`
- 各 Version 方法类似

- [ ] **Step 2: 写 schema_extras hook**(partial UNIQUE)

加到 `backend/internal/infra/db/schema_extras.go` 现有 extraGroup 列表里,新增一组:

```go
// Function partial UNIQUE: name 在用户内未软删时唯一。
//
// (DDL: CREATE UNIQUE INDEX idx_functions_user_name_unique
//   ON functions(user_id, name) WHERE deleted_at IS NULL;)
{
	Table: "functions",
	Hint:  "function partial UNIQUE on (user_id, name) WHERE deleted_at IS NULL",
	SQL: `CREATE UNIQUE INDEX IF NOT EXISTS idx_functions_user_name_unique
		ON functions(user_id, name) WHERE deleted_at IS NULL`,
},
```

并加 Version status CHECK:

```go
{
	Table: "function_versions",
	Hint:  "function_versions.status CHECK enum",
	SQL: `CREATE TABLE IF NOT EXISTS function_versions_check_marker (
		dummy INT CHECK (1=1) -- modernc/sqlite: GORM AutoMigrate 不带 CHECK,用此 marker
	)`,
	// 注:实际 CHECK 走 GORM tag 写到 Version 的 Status 字段:
	//   Status string `gorm:"check:status IN ('pending','accepted','rejected')"`
},
```

修正:**实际把 CHECK 直接放 GORM tag 上,不需 schema_extras**。改 `domain/function/version.go` 的 Status 字段:

```go
Status string `gorm:"not null;check:status IN ('pending','accepted','rejected')"`
```

- [ ] **Step 3: 验证编译**

```bash
cd backend && go build ./internal/infra/store/function/
```

Expected: 无输出

- [ ] **Step 4: Commit + push**

```bash
git add backend/internal/infra/store/function/ backend/internal/domain/function/version.go backend/internal/infra/db/schema_extras.go
git commit -m "feat(function): GORM Repository impl + schema_extras partial UNIQUE"
git push origin feature/function-domain
```

---

### Task 7:Store 集成测试(in-memory SQLite,真 CRUD)

**Files:**
- Create: `backend/internal/infra/store/function/function_test.go`

- [ ] **Step 1: 写集成测试**

```go
package function

import (
	"context"
	"testing"

	"gorm.io/gorm"

	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

func setupStore(t *testing.T) (*Store, *gorm.DB) {
	t.Helper()
	gdb, err := dbinfra.Open(dbinfra.Config{DataDir: ""})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := dbinfra.Migrate(gdb, AutoMigrateModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(gdb), gdb
}

func ctxWithUser(uid string) context.Context {
	return reqctxpkg.SetUserID(context.Background(), uid)
}

func TestCreate_HappyPath(t *testing.T) {
	store, _ := setupStore(t)
	f := &functiondomain.Function{
		ID:     idgenpkg.New("fn"),
		UserID: "user1",
		Name:   "to-pdf",
	}
	if err := store.Create(ctxWithUser("user1"), f); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.GetByID(ctxWithUser("user1"), "user1", f.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Name != "to-pdf" {
		t.Errorf("got name=%q, want to-pdf", got.Name)
	}
}

func TestCreate_DuplicateName(t *testing.T) {
	store, _ := setupStore(t)
	uid := "user1"
	ctx := ctxWithUser(uid)

	f1 := &functiondomain.Function{ID: idgenpkg.New("fn"), UserID: uid, Name: "to-pdf"}
	if err := store.Create(ctx, f1); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	f2 := &functiondomain.Function{ID: idgenpkg.New("fn"), UserID: uid, Name: "to-pdf"}
	err := store.Create(ctx, f2)
	if err != functiondomain.ErrDuplicateName {
		t.Errorf("expected ErrDuplicateName, got %v", err)
	}
}

func TestSoftDelete_AllowsRecreate(t *testing.T) {
	store, _ := setupStore(t)
	uid := "user1"
	ctx := ctxWithUser(uid)

	f1 := &functiondomain.Function{ID: idgenpkg.New("fn"), UserID: uid, Name: "to-pdf"}
	if err := store.Create(ctx, f1); err != nil {
		t.Fatal(err)
	}
	if err := store.SoftDelete(ctx, uid, f1.ID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	// 软删后可重名
	f2 := &functiondomain.Function{ID: idgenpkg.New("fn"), UserID: uid, Name: "to-pdf"}
	if err := store.Create(ctx, f2); err != nil {
		t.Errorf("Recreate after soft-delete should work, got: %v", err)
	}
}

// 加更多测试覆盖:
// - GetByID 跨 user 隔离(user2 拿不到 user1 的 fn)
// - List 分页(cursor 正确)
// - UpdateMeta(只改 name/description/tags,不动 active_version_id)
// - CreateVersion / GetVersionByID / GetPending / UpdateVersionStatus
// - HardDeleteOldestAccepted(50 cap 验证)
```

- [ ] **Step 2: 跑测试**

```bash
cd backend && go test ./internal/infra/store/function/ -count=1
```

Expected: 至少 `PASS` 上述 3 测试。完整覆盖时 ~12-15 测试通过。

- [ ] **Step 3: Commit + push**

```bash
git add backend/internal/infra/store/function/function_test.go
git commit -m "test(function): store integration tests (CRUD + cross-user + partial UNIQUE)"
git push origin feature/function-domain
```

---

## Phase 3:App Layer(`internal/app/function/`)

### Task 8:Service struct + 接口定义

**Files:**
- Create: `backend/internal/app/function/function.go`

- [ ] **Step 1: 写 Service**

```go
// Package function provides the Function service layer — coordinates domain
// Repository, sandbox v2 EnvManager, eventlog Emitter, and exposes ports for
// catalog source consumption.
//
// Package function 提供 Function service 层 — 协调 domain Repository,sandbox
// v2 EnvManager,eventlog Emitter,并对外暴露 catalog source。
package function

import (
	"context"

	"go.uber.org/zap"

	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
)

// Sandbox is the narrowed port forge service consumes from sandbox v2.
//
// Sandbox 是 forge 服务从 sandbox v2 消费的 port(narrowed)。
type Sandbox interface {
	EnsureEnv(ctx context.Context, owner string, deps []string, pythonVersion string, progress ProgressFunc) (envID string, err error)
	Spawn(ctx context.Context, owner string, opts SpawnOpts) (ExecutionResult, error)
	Destroy(ctx context.Context, owner string) error
}

type SpawnOpts struct {
	Cmd     string
	Args    []string
	Stdin   []byte
	Cwd     string
	Env     map[string]string
	Timeout int // ms
}

type ExecutionResult struct {
	OK        bool
	Stdout    []byte
	Stderr    []byte
	ExitCode  int
	ElapsedMs int
}

type ProgressFunc func(stage string, line string)

// Service is the Function domain entry point. CRUD + ops apply + run
// orchestration. Holds Repository + Sandbox + Logger + Notifications.
//
// Service 是 Function domain 入口。CRUD + ops apply + run 编排。
// 持 Repository / Sandbox / Logger / Notifications。
type Service struct {
	repo    functiondomain.Repository
	sandbox Sandbox
	notif   *notificationspkg.Publisher
	log     *zap.Logger
}

func NewService(repo functiondomain.Repository, sandbox Sandbox, notif *notificationspkg.Publisher, log *zap.Logger) *Service {
	return &Service{repo: repo, sandbox: sandbox, notif: notif, log: log.Named("function")}
}
```

- [ ] **Step 2: 验证编译**

```bash
cd backend && go build ./internal/app/function/
```

Expected: 无输出

- [ ] **Step 3: Commit + push**

```bash
git add backend/internal/app/function/function.go
git commit -m "feat(function): Service struct + Sandbox port + types"
git push origin feature/function-domain
```

---

### Task 9:apply.go — ops apply core

**Files:**
- Create: `backend/internal/app/function/apply.go`
- Create: `backend/internal/app/function/apply_test.go`

- [ ] **Step 1: 写 apply.go**

按 spec [`01-shared-tool-interface.md`](../01-shared-tool-interface.md) §12 模板:

```go
package function

import (
	"context"
	"encoding/json"
	"fmt"

	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
)

// Op is a discriminated union encoded as JSON. LLM emits []Op, system applies
// each in order with per-op + cumulative + final validation.
//
// Op 是判别式 union(JSON 序列化)。LLM 发 []Op,系统按序 apply,
// 每 op 后跑 per-op 校验,全部应用完跑 final 校验。
type Op struct {
	Type string          `json:"op"`
	Raw  json.RawMessage `json:"-"` // 解析后的原始 JSON,各 op 自取字段
}

// VersionDraft 是 ops 应用过程中的可变快照(累积态)。final 通过后转为持久化 Version。
//
// VersionDraft is the in-memory snapshot accumulated during ops apply.
type VersionDraft struct {
	Name          string
	Description   string
	Tags          []string
	Code          string
	Parameters    []functiondomain.ParameterSpec
	ReturnSchema  map[string]any
	Dependencies  []string
	PythonVersion string
}

// ApplyOps applies a series of ops to a base draft. Emits one progress
// delta per op. Returns the final draft + per-op outcomes. On per-op
// validation failure, returns the partial draft + the failing index.
//
// ApplyOps 把一组 ops 应用到 base 草稿上。每 op emit 一个 progress delta。
// 返最终 draft + per-op outcomes;失败时返部分 draft + 失败索引。
func (s *Service) ApplyOps(ctx context.Context, base *VersionDraft, ops []Op, progressBlockID string) (*VersionDraft, []OpResult, error) {
	state := cloneDraft(base)
	results := make([]OpResult, 0, len(ops))
	em := eventlogpkg.From(ctx) // never nil — falls back to no-op

	for i, op := range ops {
		if err := applyOne(state, op); err != nil {
			return nil, results, fmt.Errorf("function.ApplyOps: ops[%d] type=%q: %w", i, op.Type, err)
		}
		if err := validateIncremental(state); err != nil {
			return nil, results, fmt.Errorf("function.ApplyOps: ops[%d] left state invalid: %w", i, err)
		}
		results = append(results, OpResult{Index: i, Type: op.Type, OK: true})
		// emit progress delta — per spec D19 双写到 conversation + function scope
		if em != nil && progressBlockID != "" {
			payload, _ := json.Marshal(map[string]any{"op": op.Type, "raw": json.RawMessage(op.Raw)})
			em.DeltaBlock(ctx, progressBlockID, string(payload)+"\n")
		}
	}
	if err := validateFinal(state); err != nil {
		return nil, results, fmt.Errorf("function.ApplyOps: final validation: %w", err)
	}
	return state, results, nil
}

type OpResult struct {
	Index int
	Type  string
	OK    bool
}

func applyOne(state *VersionDraft, op Op) error {
	switch op.Type {
	case "set_meta":
		var p struct {
			Name        *string  `json:"name,omitempty"`
			Description *string  `json:"description,omitempty"`
			Tags        []string `json:"tags,omitempty"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return fmt.Errorf("set_meta unmarshal: %w", err)
		}
		if p.Name != nil {
			state.Name = *p.Name
		}
		if p.Description != nil {
			state.Description = *p.Description
		}
		if p.Tags != nil {
			state.Tags = p.Tags
		}
	case "set_code":
		var p struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return fmt.Errorf("set_code unmarshal: %w", err)
		}
		state.Code = p.Code
	case "set_parameters":
		var p struct {
			Parameters []functiondomain.ParameterSpec `json:"parameters"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return fmt.Errorf("set_parameters unmarshal: %w", err)
		}
		state.Parameters = p.Parameters
	case "set_return_schema":
		var p struct {
			ReturnSchema map[string]any `json:"returnSchema"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return fmt.Errorf("set_return_schema unmarshal: %w", err)
		}
		state.ReturnSchema = p.ReturnSchema
	case "set_dependencies":
		var p struct {
			Deps []string `json:"deps"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return fmt.Errorf("set_dependencies unmarshal: %w", err)
		}
		state.Dependencies = p.Deps
	case "set_python_version":
		var p struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal(op.Raw, &p); err != nil {
			return fmt.Errorf("set_python_version unmarshal: %w", err)
		}
		state.PythonVersion = p.Version
	default:
		return fmt.Errorf("unknown op type: %q", op.Type)
	}
	return nil
}

func cloneDraft(d *VersionDraft) *VersionDraft {
	if d == nil {
		return &VersionDraft{}
	}
	out := *d
	out.Tags = append([]string(nil), d.Tags...)
	out.Parameters = append([]functiondomain.ParameterSpec(nil), d.Parameters...)
	out.Dependencies = append([]string(nil), d.Dependencies...)
	if d.ReturnSchema != nil {
		out.ReturnSchema = make(map[string]any, len(d.ReturnSchema))
		for k, v := range d.ReturnSchema {
			out.ReturnSchema[k] = v
		}
	}
	return &out
}
```

校验函数 `validateIncremental` / `validateFinal` 在 Task 10 实现。

- [ ] **Step 2: 写 apply_test.go(部分覆盖,完整在 Task 10)**

```go
package function

import (
	"context"
	"encoding/json"
	"testing"
)

func TestApplyOps_SetMeta(t *testing.T) {
	s := &Service{} // no repo/sandbox needed for apply
	base := &VersionDraft{}
	rawMeta, _ := json.Marshal(map[string]any{"name": "to-pdf", "description": "convert markdown"})
	ops := []Op{{Type: "set_meta", Raw: rawMeta}}
	out, results, err := s.ApplyOps(context.Background(), base, ops, "")
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	if out.Name != "to-pdf" {
		t.Errorf("expected name=to-pdf, got %q", out.Name)
	}
	if len(results) != 1 || !results[0].OK {
		t.Errorf("expected 1 OK result, got %+v", results)
	}
}

func TestApplyOps_UnknownOpRejected(t *testing.T) {
	s := &Service{}
	ops := []Op{{Type: "frobnicate", Raw: json.RawMessage(`{}`)}}
	_, _, err := s.ApplyOps(context.Background(), &VersionDraft{}, ops, "")
	if err == nil {
		t.Error("expected error for unknown op type")
	}
}
```

- [ ] **Step 3: 跑测试**

```bash
cd backend && go test ./internal/app/function/ -count=1
```

Expected: PASS(2 测试,但需要 validateIncremental / validateFinal 占位 — 暂时实现为 `return nil`,Task 10 真做)。

- [ ] **Step 4: 临时占位 validate**

加到 apply.go 末尾:

```go
func validateIncremental(d *VersionDraft) error { return nil }
func validateFinal(d *VersionDraft) error       { return nil }
```

- [ ] **Step 5: Commit + push**

```bash
git add backend/internal/app/function/apply.go backend/internal/app/function/apply_test.go
git commit -m "feat(function): apply.go ops engine + 6 op handlers + skeleton tests"
git push origin feature/function-domain
```

---

### Task 10:validate.go — 校验规则(per-op + final)

**Files:**
- Create: `backend/internal/app/function/validate.go`
- Modify: `backend/internal/app/function/apply.go`(替换占位 validate*)
- Modify: `backend/internal/app/function/apply_test.go`(加校验失败 case)

- [ ] **Step 1: 写 validate.go**

```go
package function

import (
	"fmt"
	"regexp"
	"strings"
)

// validateIncremental runs after each op application — checks state is
// internally coherent without requiring all ops applied.
//
// validateIncremental 每 op 应用后跑 — 检查累积态自洽,但不要求全部 ops 完成。
func validateIncremental(d *VersionDraft) error {
	// name 长度 + 字符集(若已 set)
	if d.Name != "" {
		if !validNameRe.MatchString(d.Name) {
			return fmt.Errorf("name %q invalid: lowercase alphanum + dashes/underscores only", d.Name)
		}
	}
	// parameters name 唯一(若已 set)
	if len(d.Parameters) > 0 {
		seen := map[string]bool{}
		for _, p := range d.Parameters {
			if p.Name == "" {
				return fmt.Errorf("parameter has empty name")
			}
			if seen[p.Name] {
				return fmt.Errorf("duplicate parameter name: %q", p.Name)
			}
			seen[p.Name] = true
			if !isValidParamType(p.Type) {
				return fmt.Errorf("parameter %q invalid type: %q", p.Name, p.Type)
			}
		}
	}
	return nil
}

// validateFinal runs after all ops applied — required for the entity to be
// persisted. Includes Python AST scan + parameters/code consistency check.
//
// validateFinal 全部 ops 应用完跑 — entity 持久化的前置条件。包括 Python AST
// 扫 + parameters/code 签名一致性校验(D14)。
func validateFinal(d *VersionDraft) error {
	// 必填
	if d.Name == "" {
		return fmt.Errorf("name is required")
	}
	if d.Code == "" {
		return fmt.Errorf("code is required")
	}
	// Python AST 扫:函数名跟 Name 匹配 + 不 import Handler client(D7)
	if err := scanPythonAST(d.Code, d.Name); err != nil {
		return fmt.Errorf("AST scan: %w", err)
	}
	// 签名 vs parameters 一致性(D14)
	if err := checkParamConsistency(d.Code, d.Name, d.Parameters); err != nil {
		return fmt.Errorf("param consistency: %w", err)
	}
	return nil
}

var validNameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

func isValidParamType(t string) bool {
	switch t {
	case "string", "number", "integer", "boolean", "object", "array":
		return true
	}
	return false
}

// scanPythonAST 用 Python subprocess(sandbox 跑 ast.parse)校验代码可解析,
// 含目标函数 def,且没有 import Handler client(D7 黑名单)。
//
// scanPythonAST runs `python -c 'import ast; ast.parse(...)'` in sandbox to
// validate code parses, has a top-level def matching name, and no Handler
// imports (D7 blacklist).
func scanPythonAST(code, name string) error {
	// V1 实现:简单字符串扫描 + Python AST 后续完善。
	// V1.5 切到 sandbox 跑真 ast.parse。
	if !strings.Contains(code, "def "+name) {
		return fmt.Errorf("code must define a function named %q", name)
	}
	for _, blacklisted := range handlerImportBlacklist {
		if strings.Contains(code, blacklisted) {
			return fmt.Errorf("D7: handler import not allowed: %q", blacklisted)
		}
	}
	return nil
}

var handlerImportBlacklist = []string{
	"from forgify_handler import",
	"import forgify_handler",
}

// checkParamConsistency 比对 Python 函数签名跟 declared parameters。V1 简化:
// 提取 def 行的 parens 内容,split 逗号,对比 name + 是否 has default。
//
// checkParamConsistency cross-checks declared ParameterSpec against the
// Python function signature.
func checkParamConsistency(code, name string, params []functiondomain.ParameterSpec) error {
	// 提取 def name(...) 的参数列表(简单字符串处理;Task 10b 切 sandbox 真 AST)
	// 略(~30 行)— 实施时按 forge 现有 ast.go 的模式重写
	return nil // V1 占位,Task 10b 实现
}
```

`functiondomain` 上方需 import:

```go
import (
	"fmt"
	"regexp"
	"strings"

	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
)
```

- [ ] **Step 2: 删 apply.go 末尾占位 validate**

```bash
# 编辑 apply.go,把:
#   func validateIncremental(d *VersionDraft) error { return nil }
#   func validateFinal(d *VersionDraft) error       { return nil }
# 删掉(已迁移到 validate.go)
```

- [ ] **Step 3: 加校验失败测试**

```go
// 加到 apply_test.go 末尾

func TestApplyOps_DuplicateParam(t *testing.T) {
	s := &Service{}
	rawParams, _ := json.Marshal(map[string]any{
		"parameters": []map[string]any{
			{"name": "x", "type": "string", "required": true},
			{"name": "x", "type": "integer", "required": false},
		},
	})
	ops := []Op{{Type: "set_parameters", Raw: rawParams}}
	_, _, err := s.ApplyOps(context.Background(), &VersionDraft{}, ops, "")
	if err == nil || !strings.Contains(err.Error(), "duplicate parameter") {
		t.Errorf("expected duplicate parameter error, got %v", err)
	}
}

func TestApplyOps_FinalMissingCode(t *testing.T) {
	s := &Service{}
	rawMeta, _ := json.Marshal(map[string]any{"name": "test-fn"})
	ops := []Op{{Type: "set_meta", Raw: rawMeta}}
	_, _, err := s.ApplyOps(context.Background(), &VersionDraft{}, ops, "")
	if err == nil || !strings.Contains(err.Error(), "code is required") {
		t.Errorf("expected 'code is required' error, got %v", err)
	}
}
```

- [ ] **Step 4: 跑测试**

```bash
cd backend && go test ./internal/app/function/ -count=1
```

Expected: PASS 4 测试

- [ ] **Step 5: Commit + push**

```bash
git add backend/internal/app/function/validate.go backend/internal/app/function/apply.go backend/internal/app/function/apply_test.go
git commit -m "feat(function): validate.go (per-op + final + AST scan + D7 blacklist)"
git push origin feature/function-domain
```

---

### Task 11:Service.Create + Service.Edit + Pending → Accept 流

**Files:**
- Modify: `backend/internal/app/function/function.go`(加 CRUD methods)
- Create: `backend/internal/app/function/service_test.go`(基础 service 测试)

(完整代码见 spec [`02-function.md`](../02-function.md) §7;此 task 重点是把 Service 串起 apply.go + repo + sandbox + emit progress。)

- [ ] **Step 1: 加 Service.Create 等方法**

参考 forge 现有 `app/forge/forge.go::Service.CreatePending` / `AcceptPending` / `RejectPending` 模式,实现:

- `Search(ctx, query, limit, cursor) ([]*Function, *Cursor, error)`
- `Get(ctx, id) (*Function, *Version, *Version, error)` — 含 active + pending
- `Create(ctx, name, desc, ops, changeReason) (*Function, *Version, error)` — first-create auto-accept(对齐 forge TE-15)
- `Edit(ctx, id, ops, changeReason) (*Version, error)` — 写 pending
- `AcceptPending(ctx, id) (*Version, error)` — 翻 active version
- `RejectPending(ctx, id) error`
- `Revert(ctx, id, targetVersion) (*Version, error)`
- `Delete(ctx, id) error`(软删 + publish notification)

每个方法实施细节:
- 参数解析 → repo CRUD → publish notification(`s.notif.Publish(notificationsdomain.Envelope{Type: "function", ID: f.ID, Data: ...})`)
- Create / Edit 内调 ApplyOps,把 progress block ID 串下去
- 加密 / sandbox sync 暂占位(Task 12 处理)

- [ ] **Step 2: 跑测试**

```bash
cd backend && go test ./internal/app/function/ -count=1
```

Expected: 现有 4 测试仍通过 + Service 编译

- [ ] **Step 3: Commit + push**

```bash
git add backend/internal/app/function/function.go backend/internal/app/function/service_test.go
git commit -m "feat(function): Service CRUD + pending/accept flow + notification publish"
git push origin feature/function-domain
```

---

### Task 12:sandbox_adapter.go + Service.RunFunction + env sync

**Files:**
- Create: `backend/internal/app/function/sandbox_adapter.go`
- Modify: `backend/internal/app/function/function.go`(加 RunFunction)

参考 forge 现有 `app/forge/sandbox_adapter.go` 实现。重点:

- `EnsureEnv` 把 deps + python_version 喂给 sandbox v2 EnvManager,拿 envID
- `Spawn` 写 driver.py + user_function.py 到 sandbox env,跑 `python -u driver.py`,stdin 喂 args JSON
- 进度 emit 通过 ProgressFunc → eventlog Emitter 接驳

- [ ] **Step 1: 写 sandbox_adapter.go**(~150 行,参考 forge 原型)
- [ ] **Step 2: 写 RunFunction**(~80 行,handle ok/error 翻译)
- [ ] **Step 3: 跑单测**

```bash
cd backend && go test ./internal/app/function/ -count=1
```

Expected: 现有测试 PASS;sandbox 集成的真测试在 pipeline test 里(Task 25)。

- [ ] **Step 4: Commit + push**

```bash
git add backend/internal/app/function/sandbox_adapter.go backend/internal/app/function/function.go
git commit -m "feat(function): sandbox adapter + RunFunction + env sync"
git push origin feature/function-domain
```

---

### Task 13:catalog_source.go(D9 + 决策 D9-1 minor:configState 给 handler 用,Function 不需要)

**Files:**
- Create: `backend/internal/app/function/catalog_source.go`

- [ ] **Step 1: 实现 CatalogSource port**

```go
package function

import (
	"context"

	catalogdomain "github.com/sunweilin/forgify/backend/internal/domain/catalog"
)

// AsCatalogSource exposes the Function service as a CatalogSource (per-item granularity).
//
// AsCatalogSource 把 Function 服务暴露为 CatalogSource(per-item)。
func (s *Service) AsCatalogSource() catalogdomain.CatalogSource {
	return &catalogSource{svc: s}
}

type catalogSource struct{ svc *Service }

func (cs *catalogSource) Name() string                              { return "function" }
func (cs *catalogSource) Granularity() catalogdomain.Granularity    { return catalogdomain.PerItem }
func (cs *catalogSource) ListItems(ctx context.Context) ([]catalogdomain.Item, error) {
	fns, _, err := cs.svc.repo.List(ctx, "local-user", functiondomain.ListFilter{Limit: 1000})
	if err != nil {
		return nil, err
	}
	items := make([]catalogdomain.Item, 0, len(fns))
	for _, fn := range fns {
		desc := fn.Description
		if desc == "" {
			desc = "(no description)"
		}
		items = append(items, catalogdomain.Item{
			Source:      "function",
			Name:        fn.Name,
			Description: desc,
			Category:    "computation",
		})
	}
	return items, nil
}
```

- [ ] **Step 2: 验证编译**

```bash
cd backend && go build ./internal/app/function/
```

- [ ] **Step 3: Commit + push**

```bash
git add backend/internal/app/function/catalog_source.go
git commit -m "feat(function): CatalogSource implementation (per-item, D9)"
git push origin feature/function-domain
```

---

## Phase 4:LLM Tools(`internal/app/tool/function/`)

### Task 14-20:7 LLM tools

每个工具一个文件,~50-100 行。参考现有 `app/tool/forge/{search,get,create,edit,run}.go` 模式。

(因篇幅,仅给 Task 14 完整模板,Task 15-20 同模式实施。)

### Task 14:`search_function` 工具

**Files:**
- Create: `backend/internal/app/tool/function/function.go`(factory + 共享类型)
- Create: `backend/internal/app/tool/function/search.go`

- [ ] **Step 1: 写 factory function.go**

```go
// Package function provides LLM-facing tools for Function CRUD and execution.
//
// Package function 提供 Function CRUD + 执行的 LLM 工具集。
package function

import (
	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

// FunctionTools constructs the 7 LLM tools (search/get/create/edit/revert/delete/run).
//
// FunctionTools 构造 7 个 LLM 工具。
func FunctionTools(svc *functionapp.Service) []toolapp.Tool {
	return []toolapp.Tool{
		&SearchFunction{svc: svc},
		&GetFunction{svc: svc},
		&CreateFunction{svc: svc},
		&EditFunction{svc: svc},
		&RevertFunction{svc: svc},
		&DeleteFunction{svc: svc},
		&RunFunction{svc: svc},
	}
}
```

- [ ] **Step 2: 写 search.go**

参考 `app/tool/forge/search.go` 模式;9 方法实现 toolapp.Tool 接口:

```go
package function

import (
	"context"
	"encoding/json"

	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

type SearchFunction struct {
	svc *functionapp.Service
}

var _ toolapp.Tool = (*SearchFunction)(nil)

func (t *SearchFunction) Name() string {
	return "search_function"
}

func (t *SearchFunction) Description() string {
	return `Search the user's Function library for forged Python functions.

Returns a list of {id, name, description, parameters, tags}. Use this when:
- User asks for a capability that might already be forged ("用 PG-Handler 查...")
- Before forging a new Function, check if one already exists
- Discovering what computational capabilities the user has

Returns LLM-ranked top K matches (or full list if query is empty).`
}

func (t *SearchFunction) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "search keywords (e.g., 'csv parser', 'pdf')"},
			"limit": {"type": "integer", "description": "max results", "default": 20}
		}
	}`)
}

func (t *SearchFunction) IsReadOnly() bool        { return true }
func (t *SearchFunction) NeedsReadFirst() bool    { return false }
func (t *SearchFunction) RequiresWorkspace() bool { return false }

func (t *SearchFunction) ValidateInput(args json.RawMessage) error {
	return nil // schema 校验已在 Parameters
}

func (t *SearchFunction) CheckPermissions(args json.RawMessage, mode toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *SearchFunction) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", err
	}
	if args.Limit == 0 {
		args.Limit = 20
	}
	fns, _, err := t.svc.Search(ctx, args.Query, args.Limit, "")
	if err != nil {
		return "", err
	}
	out, _ := json.Marshal(map[string]any{"functions": fns})
	return string(out), nil
}
```

- [ ] **Step 3: 验证编译 + commit**

```bash
cd backend && go build ./internal/app/tool/function/
git add backend/internal/app/tool/function/
git commit -m "feat(function): search_function LLM tool"
git push origin feature/function-domain
```

---

### Task 15-20:剩 6 个工具(get / create / edit / revert / delete / run)

每个用同 Task 14 模板,逐 task 一个文件。具体每个的 Description / Parameters / Execute 见 spec [`02-function.md`](../02-function.md) §7。

- [ ] Task 15:`get_function`(只读,简单)
- [ ] Task 16:`create_function`(ops-driven 流式;**核心工具**)
- [ ] Task 17:`edit_function`(ops-driven 流式)
- [ ] Task 18:`revert_function`(简单)
- [ ] Task 19:`delete_function`(软删 + notification)
- [ ] Task 20:`run_function`(execution + sandbox 流式 progress)

每 task 完成跑:
```bash
cd backend && go test ./internal/app/tool/function/ -count=1
git add backend/internal/app/tool/function/<name>.go
git commit -m "feat(function): <name>_function LLM tool"
git push origin feature/function-domain
```

---

## Phase 5:HTTP API(`transport/httpapi/handlers/function.go`)

### Task 21:HTTP handlers 12 endpoints

**Files:**
- Create: `backend/internal/transport/httpapi/handlers/function.go`
- Modify: `backend/internal/transport/httpapi/router/router.go`(注册路由)
- Modify: `backend/internal/transport/httpapi/router/deps.go`(加 FunctionService 字段)
- Modify: `backend/internal/transport/httpapi/response/errmap.go`(13 个 sentinel 注册)
- Create: `backend/internal/transport/httpapi/handlers/function_test.go`(httptest)

12 endpoints per spec [`02-function.md`](../02-function.md) §8:

```
POST   /api/v1/functions
GET    /api/v1/functions
GET    /api/v1/functions/{id}
PATCH  /api/v1/functions/{id}
DELETE /api/v1/functions/{id}
POST   /api/v1/functions/{id}:run
POST   /api/v1/functions/{id}:resync
GET    /api/v1/functions/{id}/versions
GET    /api/v1/functions/{id}/versions/{v}
POST   /api/v1/functions/{id}:revert
GET    /api/v1/functions/{id}/pending
POST   /api/v1/functions/{id}/pending:accept
POST   /api/v1/functions/{id}/pending:reject
```

参考 forge handlers + `idAndAction` helper 模式。

- [ ] **Step 1: 写 handlers**(~250 行)
- [ ] **Step 2: 路由注册**
- [ ] **Step 3: errmap 加 13 sentinels**
- [ ] **Step 4: 写 httptest**(每端点至少 1 个 happy + 1 个 error case;~12 测试)
- [ ] **Step 5: 跑测试 + commit**

```bash
cd backend && go test ./internal/transport/httpapi/handlers/ -count=1 -run TestFunction
git add backend/internal/transport/httpapi/handlers/function.go backend/internal/transport/httpapi/handlers/function_test.go backend/internal/transport/httpapi/router/router.go backend/internal/transport/httpapi/router/deps.go backend/internal/transport/httpapi/response/errmap.go
git commit -m "feat(function): HTTP API 12 endpoints + httptest + errmap"
git push origin feature/function-domain
```

---

## Phase 6:Wire-up

### Task 22:main.go + harness 装配

**Files:**
- Modify: `backend/cmd/server/main.go`
- Modify: `backend/test/harness/harness.go`

- [ ] **Step 1: main.go 装 Function service**

参考现有 forge 装配段,但替换。

```go
// main.go 大约 line 220 附近(现有 forge service 装配处),改为:
functionRepo := functionstore.New(gdb)
functionSvc := functionapp.NewService(
	functionRepo,
	functionapp.NewSandboxAdapter(sandboxSvc, *dataDir),
	notificationsPub,
	log,
)
// catalog 注册:
catalogService.RegisterSource(functionSvc.AsCatalogSource())

// LLM 工具:
tools = append(tools, functiontool.FunctionTools(functionSvc)...)

// router deps:
Deps{
	...
	FunctionService: functionSvc,
	...
}
```

- [ ] **Step 2: harness 装类似**
- [ ] **Step 3: AutoMigrate 注册**

```go
// main.go 现有 dbinfra.Migrate 处加:
&functiondomain.Function{},
&functiondomain.Version{},
```

- [ ] **Step 4: 编译 + 跑 unit test**

```bash
cd backend && go build ./... && make test-unit
cd .. && cd backend
```

Expected: 全绿(若 forge 还没删,会与 forge 共存;Phase 7 删 forge)

- [ ] **Step 5: Commit + push**

```bash
git add backend/cmd/server/main.go backend/test/harness/harness.go
git commit -m "feat(function): wire FunctionService in main + harness + catalog source"
git push origin feature/function-domain
```

---

## Phase 7:Forge Removal

### Task 23:删 app/forge 整个

**Files:**
- Delete: `backend/internal/app/forge/`
- Delete: `backend/internal/app/tool/forge/`
- Delete: `backend/internal/domain/forge/`
- Delete: `backend/internal/infra/store/forge/`
- Modify: `backend/cmd/server/main.go`(删 forgeService 引用)
- Modify: `backend/internal/transport/httpapi/handlers/forge.go`(删)
- Modify: `backend/internal/transport/httpapi/router/router.go`(删 forge 路由)
- Modify: `backend/internal/transport/httpapi/response/errmap.go`(删 forge sentinels)
- Modify: `backend/cmd/server/main.go`(AutoMigrate 删 forge tables)
- Delete: `backend/test/forge/`(legacy pipeline tests)

- [ ] **Step 1: 删整批 forge 目录**

```bash
rm -rf backend/internal/app/forge
rm -rf backend/internal/app/tool/forge
rm -rf backend/internal/domain/forge
rm -rf backend/internal/infra/store/forge
rm backend/internal/transport/httpapi/handlers/forge.go
rm -rf backend/test/forge
```

- [ ] **Step 2: main.go / harness 删 forge 引用**

手动 grep 找所有 forge 提及,逐一删:

```bash
grep -rn "forgeapp\|forgedomain\|forgestore\|forgetool" backend/
```

逐个文件 edit。

- [ ] **Step 3: errmap 删 forge sentinels**(11 行)
- [ ] **Step 4: router 删 forge 路由组**

- [ ] **Step 5: AutoMigrate 删 4 行**

```go
// 删:
&forgedomain.Forge{},
&forgedomain.ForgeVersion{},
&forgedomain.ForgeTestCase{},
&forgedomain.ForgeExecution{},
```

- [ ] **Step 6: 编译验证**

```bash
cd backend && go build ./... && GOOS=windows go build ./... && GOOS=linux go build ./...
```

Expected: 三平台全绿

- [ ] **Step 7: 跑 unit test**

```bash
make test-unit
```

Expected: 全绿(forge tests 已删,function tests 仍在)

- [ ] **Step 8: Commit + push**

```bash
git add -A
git commit -m "refactor(function): remove forge entirely (D5 — function replaces forge)"
git push origin feature/function-domain
```

---

## Phase 7.5:Execution Log(D22 — `function_executions` 表)

### Task 23a:Domain `Execution` entity + Repository extension

**Files:** Modify `backend/internal/domain/function/repository.go`(加 Execution + ExecutionRepository)

加 entity:

```go
type Execution struct {
	ID               string `gorm:"primaryKey"`  // fne_<16hex>
	UserID           string `gorm:"index"`
	FunctionID       string `gorm:"index"`
	VersionID        string
	Status           string `gorm:"check:status IN ('ok','failed','cancelled','timeout')"`
	TriggeredBy      string `gorm:"check:triggered_by IN ('chat','workflow','http','test')"`
	Input            string // JSON
	Output           string // JSON (NULL on non-ok)
	ErrorCode        string
	ErrorMessage     string
	ElapsedMs        int
	StartedAt        time.Time `gorm:"index:idx_fne_fn_started,priority:2"`
	EndedAt          time.Time
	ConversationID   string `gorm:"index"`
	MessageID        string
	ToolCallID       string
	FlowrunID        string `gorm:"index"`
	FlowrunNodeID    string
	PythonVersion    string
	CreatedAt        time.Time
}

// 复合索引(主路径)
// idx_fne_fn_started:(function_id, started_at DESC) — entity 历史
```

加 Repository methods:`CreateExecution(ctx, *Execution) error` / `ListExecutions(ctx, filter ExecutionFilter) ([]*Execution, *Cursor, error)` / `GetExecution(ctx, id) (*Execution, error)` / `PruneExecutionsOlderThan(ctx, functionID string, keep int) error`(per spec 08 §5,默认 keep=200)。

- [ ] Step 1-3:写 domain + 编译 + commit

### Task 23b:Store 实现 + 集成测试

**Files:** Modify `backend/internal/infra/store/function/function.go`

加 4 个 method GORM 实现 + 3 个测试(Create + List by function_id + Prune 200 cap)。

- [ ] Step 1-3

### Task 23c:Service.Run 终态写 Execution

**Files:** Modify `backend/internal/app/function/function.go::Service.Run`

```go
func (s *Service) Run(ctx context.Context, fnID string, args map[string]any) (out *RunResult, err error) {
	startedAt := time.Now()
	defer func() {
		// 终态 detached ctx 写 execution(per §S9)
		writeCtx := reqctxpkg.SetUserID(context.Background(), reqctxpkg.DefaultLocalUserID)
		exec := buildExecutionRow(fnID, args, out, err, startedAt, time.Now())
		if writeErr := s.repo.CreateExecution(writeCtx, exec); writeErr != nil {
			s.log.Warn("function.Run: write execution failed", zap.Error(writeErr))
		}
		// 异步 prune
		go s.repo.PruneExecutionsOlderThan(writeCtx, fnID, 200)
	}()
	// ... 原有 run 逻辑 ...
}
```

`buildExecutionRow` 从 ctx 拿 conversation/flowrun 上下文 + triggered_by 推断。Sensitive Handler config 字段不存在 Function 域,正常存。

- [ ] Step 1-3:实现 + 单测 + commit

### Task 23d:HTTP `GET /api/v1/functions/{id}/executions`

**Files:** Modify `backend/internal/transport/httpapi/handlers/function.go`

加端点 + cursor 分页 + 过滤参 + 1-2 个 httptest。

- [ ] Step 1-3

### Task 23e:LLM 工具 `search_function_executions` + `get_function_execution`(D22 per-entity)

**Files:**
- Create: `backend/internal/app/tool/function/search_executions.go`
- Create: `backend/internal/app/tool/function/get_execution.go`
- Modify: `backend/internal/app/tool/function/function.go`(factory 加 2 个工具到 FunctionTools())

参考 Plan 01 Task 14 工具模板。两个工具:
- `search_function_executions`:filter functionId / versionId / status / conversationId / flowrunId / since / until + cursor 分页;返摘要 + aggregates
- `get_function_execution`:单 id 查;返完整 input/output 截 4KB + hints(output_empty / significantly_slower / duplicates_previous_input)

`hints` 计算:同 entity p50 elapsed_ms 用滚动平均;duplicates_previous_input 用 SHA256(input) 索引(每次写时计算)。

- [ ] Step 1: 写 search_executions.go + get_execution.go(~150 行)
- [ ] Step 2: factory function.go 加 2 个工具实例
- [ ] Step 3: 单测覆盖各 filter + hints 计算
- [ ] Step 4: Commit + push

---

## Phase 8:Pipeline Test

### Task 24:Function pipeline test

**Files:**
- Create: `backend/test/function/function_pipeline_test.go`

- [ ] **Step 1: 写 E2E pipeline test**

参考 `backend/test/forge/forge_pipeline_test.go` 现状作模板,改 forge → function。

测试场景(至少 3 个):

1. **CreateFunctionEndToEnd**:LLM ops 流式 create_function → DB 落 → run_function → 拿结果
2. **EditFunctionGoesPending**:create + edit → pending 写入 → accept → active 翻
3. **DeleteFunctionPublishesNotification**:delete → notification type=function action=deleted 推

- [ ] **Step 2: 跑 pipeline test**

```bash
cd /Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify
make test-pipeline
```

Expected: 包含 function pipeline 在内的全套 pipeline tests 通过

- [ ] **Step 3: Commit + push**

```bash
git add backend/test/function/function_pipeline_test.go
git commit -m "test(function): pipeline E2E tests (create / edit/accept / delete+notification)"
git push origin feature/function-domain
```

---

## Phase 9:Cross-platform compile + Doc Sync

### Task 25:三平台编译 + staticcheck

- [ ] **Step 1: 三平台编译**

```bash
cd backend
go build ./...
GOOS=windows go build ./...
GOOS=linux go build ./...
```

Expected: 三平台全绿

- [ ] **Step 2: staticcheck**

```bash
staticcheck ./...
```

Expected: 0 警告(若有 SA1029 / S1016 等,逐一修)

- [ ] **Step 3: Commit 修复(若有)**

```bash
git add -A && git commit -m "fix(function): staticcheck cleanup"
git push origin feature/function-domain
```

---

### Task 26:文档同步(per S14 — 4 contract docs + service-design + progress + backend-design)

**Files:**
- Create: `documents/version-1.2/service-design-documents/function.md`
- Modify: `documents/version-1.2/service-contract-documents/api-design.md`(加 function 段,删 forge 段)
- Modify: `documents/version-1.2/service-contract-documents/database-design.md`(加 function 表,删 forge 表段)
- Modify: `documents/version-1.2/service-contract-documents/error-codes.md`(加 13 function 错误码,删 forge)
- Modify: `documents/version-1.2/service-contract-documents/events-design.md`(加 function notification type)
- Modify: `documents/version-1.2/progress-record.md`(加 dev log)
- Modify: `documents/version-1.2/backend-design.md`(domain/app/infra 树更新)

- [ ] **Step 1: 写 function.md service-design**

把 spec [`02-function.md`](../02-function.md) 内容搬到 `service-design-documents/function.md`,适应 service-design 格式(参考 forge.md 现有结构)。

- [ ] **Step 2: 4 contract docs 同步**(每份 ~50 行改动)

- [ ] **Step 3: progress-record dev log**

加一行(per S19 节制):

```
| 2026-05-XX | **[refactor]** function domain 替代 forge:domain + store + app + 7 LLM tools + 12 HTTP endpoints + sandbox 集成 + catalog source。删 forge 整个(legacy/)。~3000 LOC + 25 测试全绿;3 平台 cross 通。 |
```

- [ ] **Step 4: backend-design.md tree update**(`domain/forge` → `domain/function`,各处替换)

- [ ] **Step 5: Commit + push**

```bash
git add documents/
git commit -m "docs(function): service-design + 4 contract docs + progress + backend-design sync (S14)"
git push origin feature/function-domain
```

---

## Phase 10:PR + Merge

### Task 27:Open PR

- [ ] **Step 1: 推 branch + 开 PR**

```bash
gh pr create --title "feat(function): replace forge with new Function domain" --body "$(cat <<'EOF'
## Summary
- 完全替代 forge 的新 Function domain
- ops-driven 流式锻造(create/edit 走 ops + progress block delta)
- LLM 自报 parameters schema + 后端 AST 校验对齐(D14)
- 7 LLM tools + 12 HTTP endpoints + catalog source(per-item)
- sandbox v2 复用(Python EnvManager,共享 venv by env_id)
- 删 forge 整个(domain / app / infra/store / handlers / tests / 4 表)

## Test plan
- [x] `make test-unit` 全绿
- [x] `make test-pipeline` function 套件通过
- [x] 三平台 cross-compile 通过
- [x] `staticcheck ./...` 0 警告
- [x] 4 contract docs + service-design-documents/function.md + progress-record + backend-design 同步
- [ ] manual smoke:启 backend,curl 创建 function + run + list

## Related
- spec: documents/version-1.2/adhoc-topic-documents/forge_redesign/02-function.md
- plan: documents/version-1.2/adhoc-topic-documents/forge_redesign/plans/01-function-domain.md
EOF
)"
```

- [ ] **Step 2: 等 review + merge**

merge 后 task 4(写 plan)task 完成。后续 plan 02-handler-domain 接力。

---

## 总 Task 数:27 个 task,~6-9 个工作日(per task 平均 30-90 min)

**Acceptance criteria(全部满足才算 plan 完工)**:

1. ✅ 27 个 task 全 checkbox 打勾
2. ✅ `make test-unit` 全绿
3. ✅ `make test-pipeline` function 套件通过
4. ✅ 三平台 cross-compile 通过
5. ✅ `staticcheck ./...` 0 警告
6. ✅ 4 contract docs + service-design-documents/function.md + progress-record + backend-design 同步(S14)
7. ✅ `forge` 关键字在 `backend/` 下零出现(`grep -rn "forge\|forgify forge" backend/` 验证 — 注意 `forgify` 项目名不算)
8. ✅ PR merge 到 main
9. ✅ origin/main 收到 push(投资人可见)

完工后,plan 02-handler-domain 接力。

---

(本 plan 完)
