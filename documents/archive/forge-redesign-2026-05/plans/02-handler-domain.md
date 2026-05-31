# Handler Domain Implementation Plan

> ⚠️ **STATUS: 主体已 merge(2026-05-12,11 commits 直推 main),env 相关章节已被 redesign 取代**
>
> Plan 02 的核心交付(domain / app / store / stdio client / 10 LLM tools / 16 HTTP / Handler Config 加密 / D22 handler_calls / pipeline test)全部落地。
>
> 但 **env 模型在 2026-05-12 大幅修订**(详见 [`../discussions/2026-05-12-env-and-sse-rework.md`](../discussions/2026-05-12-env-and-sse-rework.md) §D-E),跟 function domain 同模式:
> - EnvID 改为每 Version 独立生成(`hdenv_<16hex>`,跟 version_id 1:1 但解耦)
> - env sync **同步发生在 LLM tool 内**(create_handler / edit_handler)
> - AcceptPending 纯指针翻转;RejectPending 销 env + 删行
> - Edit 改 "iterate same pending",删 ErrPendingConflict
> - 内部 env-fix loop(maxAttempts=3,主 chat LLM 改 deps)
> - `ops=[]` 显式语义 = 强制重建 env(D-redo-22)
> - HTTP `:resync` 删除(若有)
> - Service.Create/Edit 前置 sandbox ping,失败 503 硬拒
>
> 本文档保留**实施过程记录**,env 设计细节请以 `03-handler.md` + 讨论文档为准。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 全新 `handler` domain — Python class + 多 method,Definition + Instance 二层(caller-owns lifetime),method-level ops (W),Handler Config (init_args 加密),stdio JSON-RPC client,catalog source。**前置依赖**:Plan 01 已 merge(Function domain 已就位)。

**Architecture:** 4-layer clean arch + `infra/handler/` 加一份 stdio JSON-RPC client(参考 mcp 但代码独立 per D2)。**Definition** 持久化(handlers + handler_versions 表),**Instance** 仅 in-memory(per-instance subprocess + venv + state)。Instance lifecycle 由 caller-context 决定(D3):chat conv / FlowRun / test。Handler Config(D16)走 AES-GCM 加密,复用 `infra/crypto.AESGCMEncryptor`。

**Tech Stack:** Go 1.25,GORM,modernc.org/sqlite,sandbox v2 (Python EnvManager),infra/crypto AES-GCM,pkg/eventlog Emitter,pkg/idgen,pkg/reqctx。

**关联**:[`03-handler.md`](../03-handler.md) 完整 spec / [`01-shared-tool-interface.md`](../01-shared-tool-interface.md) 工具接口 / [`07-notifications-and-eventlog.md`](../07-notifications-and-eventlog.md) eventlog scope / Plan 01 set 的实施模板。

---

## Phase 0:Branch Setup

### Task 1:创建 feature branch

**Files:** —

- [ ] **Step 1: 验证 main + Plan 01 已 merge**

```bash
cd /Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify
git checkout main && git pull origin main
ls backend/internal/domain/function/  # Plan 01 应已落 function domain
```

Expected: function 目录存在(Plan 01 merge 后)

- [ ] **Step 2: 创建分支**

```bash
git checkout -b feature/handler-domain
```

---

## Phase 1:Domain Layer(`internal/domain/handler/`)

### Task 2:Handler entity + 14 sentinels

**Files:**
- Create: `backend/internal/domain/handler/handler.go`

- [ ] **Step 1: 写 entity + sentinels**

```go
// Package handler defines the Handler domain — Python class with methods,
// Definition + Instance two-tier, caller-owns instance lifetime.
//
// Package handler 定义 Handler domain — Python 类 + 多 method,
// Definition + Instance 二层,Instance lifetime 由 caller-context 决定。
package handler

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Handler is the Definition entity (per-user, name-unique while not deleted).
// Code/methods/init_args/deps live on Version. Config (init_args values)
// is per-Definition AES-GCM encrypted.
//
// Handler 是 Definition 实体。代码 / methods / init_args schema / deps 在
// Version 上;config(init_args 实际值)per-Definition 加密存。
type Handler struct {
	ID              string `gorm:"primaryKey"`
	UserID          string `gorm:"index;not null"`
	Name            string `gorm:"not null"`
	Description     string
	Tags            []string `gorm:"serializer:json"`
	ActiveVersionID string   `gorm:"index"`
	ConfigEncrypted string   `gorm:""` // AES-GCM 加密的 init_args JSON;空表未配
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt `gorm:"index"`

	// Computed fields(`gorm:"-"`)by service.attach* fillers
	Pending       *Version   `gorm:"-"`
	EnvStatus     string     `gorm:"-"`
	EnvError      string     `gorm:"-"`
	EnvSyncedAt   *time.Time `gorm:"-"`
	EnvSyncStage  string     `gorm:"-"`
	EnvSyncDetail string     `gorm:"-"`
	ConfigState   string     `gorm:"-"` // "ready" / "partially_configured" / "unconfigured"
	LiveInstances int        `gorm:"-"` // count from in-memory registry
}

var (
	ErrNotFound              = errors.New("handler: not found")
	ErrDuplicateName         = errors.New("handler: duplicate name")
	ErrMethodNotFound        = errors.New("handler: method not found")
	ErrVersionNotFound       = errors.New("handler: version not found")
	ErrPendingNotFound       = errors.New("handler: pending not found")
	ErrPendingConflict       = errors.New("handler: pending conflict")
	ErrInstanceSpawnFailed   = errors.New("handler: instance spawn failed")
	ErrInstanceCrashed       = errors.New("handler: instance crashed")
	ErrInstanceRPCTimeout    = errors.New("handler: instance RPC timeout")
	ErrNoActiveVersion       = errors.New("handler: no active version")
	ErrEnvNotReady           = errors.New("handler: env not ready")
	ErrEnvFailed             = errors.New("handler: env failed")
	ErrOpInvalid             = errors.New("handler: op invalid")
	ErrInstanceNotFound      = errors.New("handler: instance not found")
	ErrConfigIncomplete      = errors.New("handler: config incomplete")
	ErrConfigInvalid         = errors.New("handler: config invalid")
	ErrConfigDecryptFailed   = errors.New("handler: config decrypt failed")
)
```

- [ ] **Step 2: 编译 + commit**

```bash
cd backend && go build ./internal/domain/handler/
git add backend/internal/domain/handler/handler.go
git commit -m "feat(handler): domain entity + 17 sentinels"
git push origin feature/handler-domain
```

---

### Task 3:Version + MethodSpec + InitArgSpec + ConfigState

**Files:**
- Create: `backend/internal/domain/handler/version.go`
- Create: `backend/internal/domain/handler/method.go`

- [ ] **Step 1: version.go**

```go
package handler

import "time"

const (
	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusRejected = "rejected"

	EnvStatusPending = "pending"
	EnvStatusSyncing = "syncing"
	EnvStatusReady   = "ready"
	EnvStatusFailed  = "failed"
	EnvStatusEvicted = "evicted"

	ConfigStateReady               = "ready"
	ConfigStatePartiallyConfigured = "partially_configured"
	ConfigStateUnconfigured        = "unconfigured"

	DefaultPythonVersion = ">=3.12"
)

// Version snapshots class code + methods + init_args_schema + deps.
//
// Version 快照:class code + methods + init_args schema + deps。
type Version struct {
	ID             string `gorm:"primaryKey"`
	HandlerID      string `gorm:"index;not null"`
	Status         string `gorm:"not null;check:status IN ('pending','accepted','rejected')"`
	Version        *int   // NULL on pending/rejected
	Imports        string // class 顶部 imports(set_imports op)
	InitBody       string // __init__ body(set_init op)
	ShutdownBody   string // shutdown body(set_shutdown op,可空)
	Methods        []MethodSpec  `gorm:"serializer:json"`
	InitArgsSchema []InitArgSpec `gorm:"serializer:json"`
	Dependencies   []string      `gorm:"serializer:json"`
	PythonVersion  string
	EnvID          string `gorm:"index"`
	EnvStatus      string
	EnvError       string
	EnvSyncedAt    *time.Time
	EnvSyncStage   string
	EnvSyncDetail  string
	ChangeReason   string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
```

- [ ] **Step 2: method.go**

```go
package handler

// MethodSpec is one Python method's full description (schema + body).
//
// MethodSpec 一个 Python method 的完整描述(schema + body)。
type MethodSpec struct {
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	Args         []ArgSpec      `json:"args"`
	ReturnSchema map[string]any `json:"returnSchema"`
	Body         string         `json:"body"` // Python method body 字符串(无 def 头)
	Streaming    bool           `json:"streaming"` // body 内有 yield → 翻 progress
	Timeout      int            `json:"timeout,omitempty"` // ms
}

type ArgSpec struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
	Default     any    `json:"default,omitempty"`
}

// InitArgSpec 是 __init__ 一次性参数的 schema(D16)。
//
// InitArgSpec describes one __init__ one-time parameter (D16).
type InitArgSpec struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
	Sensitive   bool   `json:"sensitive"` // true → 加密 + 密码框 + 永不返明文
	Default     any    `json:"default,omitempty"`
}
```

- [ ] **Step 3: 编译 + commit**

```bash
cd backend && go build ./internal/domain/handler/
git add backend/internal/domain/handler/
git commit -m "feat(handler): Version entity + MethodSpec + ArgSpec + InitArgSpec"
git push origin feature/handler-domain
```

---

### Task 4:Repository 接口

**Files:**
- Create: `backend/internal/domain/handler/repository.go`

按 Plan 01 Task 4 同模式。除 CRUD 外加 Config 操作:

```go
type Repository interface {
	// Handler CRUD(同 function 模式)
	Create / GetByID / GetByName / List / UpdateMeta / SoftDelete / SetActiveVersion ...

	// Version CRUD(同)
	CreateVersion / GetVersionByID / GetVersionByNumber / ListVersions / UpdateVersionEnv / GetPending / UpdateVersionStatus / HardDeleteOldestAccepted

	// Config(D16 — 加密存)
	UpdateConfigEncrypted(ctx context.Context, userID, handlerID, ciphertext string) error
	ClearConfig(ctx context.Context, userID, handlerID string) error
	GetConfigEncrypted(ctx context.Context, userID, handlerID string) (string, error)
}
```

- [ ] Step 1-3: 写接口 + 编译 + commit(参考 Plan 01 Task 4 模板)

---

### Task 5:Domain layer 单测(sentinel uniqueness + struct round-trip)

**Files:**
- Create: `backend/internal/domain/handler/handler_test.go`

参考 Plan 01 Task 5 模板,加 MethodSpec / InitArgSpec JSON round-trip 测试。

- [ ] Step 1-3:写 + 跑 + commit

---

## Phase 2:Store Layer

### Task 6:Repository GORM 实现 + AutoMigrate hook + partial UNIQUE

**Files:**
- Create: `backend/internal/infra/store/handler/handler.go`
- Modify: `backend/internal/infra/db/schema_extras.go`(加 partial UNIQUE)

参考 Plan 01 Task 6 + Function store 模板;关键差异:
- 加 `UpdateConfigEncrypted` / `ClearConfig` / `GetConfigEncrypted` 三方法(直接 GORM update 单字段)
- partial UNIQUE: `CREATE UNIQUE INDEX idx_handlers_user_name_unique ON handlers(user_id, name) WHERE deleted_at IS NULL`

- [ ] Step 1-3:写实现 + schema_extras + commit

---

### Task 7:Store 集成测试

**Files:**
- Create: `backend/internal/infra/store/handler/handler_test.go`

参考 Plan 01 Task 7。新增测试:
- `TestUpdateConfigEncrypted_RoundTrip`(写 + 读 ciphertext)
- `TestUpdateConfigEncrypted_CrossUserIsolated`

- [ ] Step 1-3

---

## Phase 3:Infra Layer — stdio JSON-RPC Client(`infra/handler/`)

### Task 8:Client 接口 + Wire format

**Files:**
- Create: `backend/internal/infra/handler/client.go`

```go
// Package handler provides the stdio JSON-RPC client wrapper for HandlerInstance
// subprocess. Wire format: line-delimited JSON (LF separator) over stdin/stdout.
//
// Package handler 提供 HandlerInstance 子进程的 stdio JSON-RPC 客户端。
// Wire format: 按行 JSON(LF 分隔)over stdin/stdout。
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

// Client is the interface to one running HandlerInstance (one subprocess).
// 5 methods: Init / Call / StreamCall / Shutdown / Crashed.
//
// Client 是 1 个运行中 HandlerInstance(1 个 subprocess)的接口。
type Client interface {
	Init(ctx context.Context, args map[string]any) error
	Call(ctx context.Context, method string, args map[string]any) (any, error)
	StreamCall(ctx context.Context, method string, args map[string]any, onProgress func(string)) (any, error)
	Shutdown(ctx context.Context) error
	Crashed() bool // returns true if subprocess died unexpectedly
}

// Message types(详见 spec 03-handler §12)
const (
	MsgInit       = "init"
	MsgReady      = "ready"
	MsgInitError  = "init_error"
	MsgCall       = "call"
	MsgReturn     = "return"
	MsgError      = "error"
	MsgProgress   = "progress"
	MsgShutdown   = "shutdown"
)

// New constructs a Client wrapping subprocess stdin/stdout pipes.
//
// New 构造一个包装 subprocess stdin/stdout pipe 的 Client。
func New(stdin io.WriteCloser, stdout io.Reader, log *zap.Logger) Client {
	return &client{stdin: stdin, stdout: bufio.NewReader(stdout), log: log.Named("handler.client")}
}

type client struct {
	mu       sync.Mutex // 序列化 RPC(per-instance method 串行 — V1 简化)
	stdin    io.WriteCloser
	stdout   *bufio.Reader
	log      *zap.Logger
	crashed  bool
	nextReqID int
}

// (具体实现 ~150 行 — Init / Call / StreamCall / Shutdown / message loop)
// 参考 progress-record D6-1 mcp client 实现模式(stdio JSON-RPC)。
```

- [ ] Step 1: 写 client.go(~200 行,参考 mcp client 模式但代码独立 per D2)
- [ ] Step 2: 单测 client_test.go(用 fake stdin/stdout 测协议解析)
- [ ] Step 3: 编译 + commit

---

## Phase 4:App Layer

### Task 9:Service struct + Sandbox port

**Files:**
- Create: `backend/internal/app/handler/handler.go`

参考 Plan 01 Task 8 模板。区别:
- Service 持 `Encryptor cryptodomain.Encryptor` 字段(D16)
- Service 持 `instanceRegistry *instanceRegistry` 字段(Task 11 实现)

```go
type Service struct {
	repo       handlerdomain.Repository
	sandbox    Sandbox
	clientFact ClientFactory  // for testing — fake client injection
	encryptor  cryptodomain.Encryptor
	registry   *instanceRegistry
	notif      *notificationspkg.Publisher
	log        *zap.Logger
}

type ClientFactory func(stdin io.WriteCloser, stdout io.Reader) handlerinfra.Client
```

- [ ] Step 1-3:写 + 编译 + commit

---

### Task 10:apply.go — method-level ops(W,跟 workflow 节点级 ops 一致)

**Files:**
- Create: `backend/internal/app/handler/apply.go`
- Create: `backend/internal/app/handler/apply_test.go`

参考 Plan 01 Task 9。区别:**ops 集合不同**(per spec D15):
- `set_meta` / `set_imports` / `set_init` / `set_shutdown` / `set_init_args_schema`
- `add_method` / `update_method` / `delete_method`(method-level)
- `set_dependencies` / `set_python_version`

`update_method` patch 走 **JSON Merge Patch**(RFC 7396),per spec。

```go
func applyOne(state *VersionDraft, op Op) error {
	switch op.Type {
	case "set_meta": ...
	case "set_imports": ...
	case "set_init": ...
	case "set_shutdown": ...
	case "set_init_args_schema":
		var p struct { Args []handlerdomain.InitArgSpec `json:"args"` }
		if err := json.Unmarshal(op.Raw, &p); err != nil { return err }
		state.InitArgsSchema = p.Args
	case "add_method":
		var p struct { Method handlerdomain.MethodSpec `json:"method"` }
		if err := json.Unmarshal(op.Raw, &p); err != nil { return err }
		// 校验 name 唯一
		for _, m := range state.Methods {
			if m.Name == p.Method.Name {
				return fmt.Errorf("method %q already exists", p.Method.Name)
			}
		}
		state.Methods = append(state.Methods, p.Method)
	case "update_method":
		var p struct { Name string `json:"name"`; Patch json.RawMessage `json:"patch"` }
		if err := json.Unmarshal(op.Raw, &p); err != nil { return err }
		// JSON Merge Patch:取现有 method,merge patch,写回
		idx := findMethodIdx(state.Methods, p.Name)
		if idx < 0 { return fmt.Errorf("method %q not found", p.Name) }
		merged, err := jsonMergePatch(state.Methods[idx], p.Patch)
		if err != nil { return err }
		state.Methods[idx] = merged
	case "delete_method":
		var p struct { Name string `json:"name"` }
		if err := json.Unmarshal(op.Raw, &p); err != nil { return err }
		idx := findMethodIdx(state.Methods, p.Name)
		if idx < 0 { return fmt.Errorf("method %q not found", p.Name) }
		state.Methods = append(state.Methods[:idx], state.Methods[idx+1:]...)
	case "set_dependencies": ...
	case "set_python_version": ...
	}
	return nil
}
```

- [ ] Step 1-3:写 apply.go + apply_test.go(类似 Plan 01 Task 9 但加 method-level op 测试)

---

### Task 11:registry.go — Instance lifecycle(caller-owns)

**Files:**
- Create: `backend/internal/app/handler/registry.go`
- Create: `backend/internal/app/handler/registry_test.go`

per spec D3 / 03-handler.md §3.3。**重点测**:scope 销毁级联 / idle GC / 跨 owner 隔离。

```go
package handler

import (
	"context"
	"sync"
	"time"
)

type Owner struct {
	Kind string // "conversation" | "flowrun" | "test" | "session"
	ID   string
}

type Instance struct {
	ID         string // hdi_<16hex>
	HandlerID  string
	Owner      Owner
	Client     handlerinfra.Client
	Cancel     context.CancelFunc
	LastUsedAt time.Time
}

// instanceRegistry tracks live instances per Owner.
//
// instanceRegistry 按 Owner 跟踪活 instance。
type instanceRegistry struct {
	mu        sync.RWMutex
	instances map[Owner]map[string]*Instance // owner → handlerName → instance
	idleGCTick time.Duration
	idleTimeout time.Duration
}

func newInstanceRegistry(idleTimeout time.Duration) *instanceRegistry {
	return &instanceRegistry{
		instances:   make(map[Owner]map[string]*Instance),
		idleGCTick:  30 * time.Second,
		idleTimeout: idleTimeout,
	}
}

// Acquire returns the live instance for (owner, handlerName), creating
// one if none exists via the spawn func.
//
// Acquire 返 (owner, handlerName) 的活 instance,无则用 spawn func 起。
func (r *instanceRegistry) Acquire(ctx context.Context, owner Owner, handlerName string, spawn func() (*Instance, error)) (*Instance, error) {
	// double-checked locking
	r.mu.RLock()
	if om, ok := r.instances[owner]; ok {
		if inst, ok := om[handlerName]; ok && !inst.Client.Crashed() {
			r.mu.RUnlock()
			r.mu.Lock()
			inst.LastUsedAt = time.Now()
			r.mu.Unlock()
			return inst, nil
		}
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	// 二次检查
	...

	// 实际 spawn
	inst, err := spawn()
	if err != nil {
		return nil, err
	}
	if r.instances[owner] == nil {
		r.instances[owner] = make(map[string]*Instance)
	}
	r.instances[owner][handlerName] = inst
	return inst, nil
}

// DestroyAll销毁某 owner 的所有 instance(scope 结束时 cascade)。
//
// DestroyAll destroys all instances for an owner (scope-end cascade).
func (r *instanceRegistry) DestroyAll(ctx context.Context, owner Owner) {
	r.mu.Lock()
	om := r.instances[owner]
	delete(r.instances, owner)
	r.mu.Unlock()

	for _, inst := range om {
		inst.Client.Shutdown(ctx) // best-effort
		inst.Cancel()
	}
}

// idleGC 周期性扫,destroy 超过 idleTimeout 没用过的 chat-scope instance。
//
// idleGC periodically scans, destroys chat-scope instances idle past timeout.
func (r *instanceRegistry) idleGC(ctx context.Context) {
	tick := time.NewTicker(r.idleGCTick)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			r.scanAndDestroyIdle()
		}
	}
}

func (r *instanceRegistry) scanAndDestroyIdle() {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	for owner, om := range r.instances {
		if owner.Kind != "conversation" {
			continue // only chat scope走 idle GC
		}
		for name, inst := range om {
			if now.Sub(inst.LastUsedAt) > r.idleTimeout {
				inst.Client.Shutdown(context.Background())
				inst.Cancel()
				delete(om, name)
			}
		}
	}
}
```

- [ ] Step 1: 写 registry.go(~250 行)
- [ ] Step 2: 写 registry_test.go(覆盖 Acquire / DestroyAll / idle GC / 跨 owner 隔离;~150 行)
- [ ] Step 3: 跑测试 + commit

---

### Task 12:config.go — 加密 / 解密 + ConfigState

**Files:**
- Create: `backend/internal/app/handler/config.go`
- Create: `backend/internal/app/handler/config_test.go`

```go
package handler

import (
	"context"
	"encoding/json"
	"fmt"

	cryptodomain "github.com/sunweilin/forgify/backend/internal/domain/crypto"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// LoadConfig 从 DB 取加密 config + 解密。返 nil if 未配。
//
// LoadConfig fetches encrypted config from DB + decrypts. nil if unconfigured.
func (s *Service) LoadConfig(ctx context.Context, handlerID string) (map[string]any, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil { return nil, err }

	ciphertext, err := s.repo.GetConfigEncrypted(ctx, uid, handlerID)
	if err != nil {
		return nil, fmt.Errorf("handler.LoadConfig: %w", err)
	}
	if ciphertext == "" {
		return nil, nil // 未配
	}
	plaintext, err := s.encryptor.Decrypt([]byte(ciphertext))
	if err != nil {
		return nil, handlerdomain.ErrConfigDecryptFailed
	}
	var config map[string]any
	if err := json.Unmarshal(plaintext, &config); err != nil {
		return nil, fmt.Errorf("handler.LoadConfig: unmarshal: %w", err)
	}
	return config, nil
}

// UpdateConfig 把 partial 跟现有 config 合并(JSON Merge Patch),加密回写。
//
// UpdateConfig merges partial into existing config (JSON Merge Patch),
// re-encrypts, persists.
func (s *Service) UpdateConfig(ctx context.Context, handlerID string, partial map[string]any) error {
	existing, _ := s.LoadConfig(ctx, handlerID)
	if existing == nil { existing = map[string]any{} }
	for k, v := range partial {
		existing[k] = v
	}
	plaintext, _ := json.Marshal(existing)
	ciphertext, err := s.encryptor.Encrypt(plaintext)
	if err != nil {
		return fmt.Errorf("handler.UpdateConfig: encrypt: %w", err)
	}
	uid, _ := reqctxpkg.RequireUserID(ctx)
	if err := s.repo.UpdateConfigEncrypted(ctx, uid, handlerID, string(ciphertext)); err != nil {
		return fmt.Errorf("handler.UpdateConfig: persist: %w", err)
	}
	// publish notification (action=config_updated)
	s.publishNotification(handlerID, "config_updated", nil)
	return nil
}

// ConfigState 比较 declared init_args_schema 跟实际已配的 keys,返状态。
//
// ConfigState computes the configState by comparing declared schema vs configured keys.
func (s *Service) ConfigState(ctx context.Context, handlerID string, schema []handlerdomain.InitArgSpec) (string, []string, error) {
	cfg, err := s.LoadConfig(ctx, handlerID)
	if err != nil { return "", nil, err }

	missing := []string{}
	for _, arg := range schema {
		if !arg.Required { continue }
		if cfg == nil || cfg[arg.Name] == nil {
			missing = append(missing, arg.Name)
		}
	}

	switch {
	case len(missing) == 0:
		return handlerdomain.ConfigStateReady, nil, nil
	case len(missing) == len(filterRequired(schema)):
		return handlerdomain.ConfigStateUnconfigured, missing, nil
	default:
		return handlerdomain.ConfigStatePartiallyConfigured, missing, nil
	}
}

func filterRequired(schema []handlerdomain.InitArgSpec) []handlerdomain.InitArgSpec {
	out := []handlerdomain.InitArgSpec{}
	for _, a := range schema {
		if a.Required { out = append(out, a) }
	}
	return out
}
```

- [ ] Step 1: 写 config.go
- [ ] Step 2: 写 config_test.go(用 fake encryptor 测 round-trip + ConfigState 三状态;~120 行)
- [ ] Step 3: commit

---

### Task 13:Validate.go(per-op + final)

**Files:**
- Create: `backend/internal/app/handler/validate.go`

类似 Plan 01 Task 10 + Handler-specific:
- `validateIncremental`:method name 唯一 / __init__ body Python AST 可解析
- `validateFinal`:整 class 拼出来 AST 通过 + class 名跟 Handler name 对齐 + 不 import handler client 相关(D7 nope — Handler 自己是 Handler,但不能 import 别的 handler client;V1 简化:不允许 `from forgify_handler import` — 实际项目不存在此 lib,纯防御)

- [ ] Step 1-3

---

### Task 14:rpc.go — System 拼 class + driver template

**Files:**
- Create: `backend/internal/app/handler/rpc.go`

per spec 03-handler.md §5.5(driver 模板 + class assembly)。

```go
// AssembleClass 拼出 Python class 字符串。
//
// AssembleClass assembles the Python class string from VersionDraft.
func AssembleClass(d *VersionDraft) string {
	var b strings.Builder
	b.WriteString("# Auto-assembled by Forgify from ops; do not edit by hand.\n")
	b.WriteString(d.Imports)
	b.WriteString("\n\nclass HandlerImpl:\n")
	if d.InitBody != "" {
		b.WriteString("    def __init__(self, **init_args):\n")
		writeIndented(&b, d.InitBody, "        ")
		b.WriteString("\n")
	}
	if d.ShutdownBody != "" {
		b.WriteString("    def shutdown(self):\n")
		writeIndented(&b, d.ShutdownBody, "        ")
		b.WriteString("\n")
	} else {
		b.WriteString("    def shutdown(self):\n        pass\n")
	}
	for _, m := range d.Methods {
		writeMethod(&b, m)
	}
	return b.String()
}

const driverTemplate = `import sys, json, traceback
sys.path.insert(0, '/sandbox/lib')
from user_handler import HandlerImpl

# ... (driver code per spec 03-handler.md §5.5)
`
```

- [ ] Step 1-3:写 + 单测 + commit

---

### Task 15:Service CRUD methods + sandbox_adapter.go

**Files:**
- Modify: `backend/internal/app/handler/handler.go`(Service methods)
- Create: `backend/internal/app/handler/sandbox_adapter.go`

类似 Plan 01 Task 11 + 12。差异:
- spawn 时拼 class + 写 user_handler.py + 起 long-lived subprocess + send Init message
- `Call(ctx, name, method, args)` 走 registry.Acquire → instance.Call

- [ ] Step 1-3:8-10 个 methods + sandbox adapter,commit

---

### Task 16:catalog_source.go(含 configState)

**Files:**
- Create: `backend/internal/app/handler/catalog_source.go`

per spec D9-1 minor — catalog item 含 configState 字段。

```go
func (cs *catalogSource) ListItems(ctx context.Context) ([]catalogdomain.Item, error) {
	hs, _, _ := cs.svc.repo.List(ctx, "local-user", ...)
	items := []catalogdomain.Item{}
	for _, h := range hs {
		// 计算 configState
		var schema []handlerdomain.InitArgSpec
		if h.ActiveVersionID != "" {
			v, _ := cs.svc.repo.GetVersionByID(ctx, h.ActiveVersionID)
			if v != nil { schema = v.InitArgsSchema }
		}
		state, missing, _ := cs.svc.ConfigState(ctx, h.ID, schema)

		// methods 概览(前 3 个)
		mNames := []string{}
		// ...

		desc := fmt.Sprintf("%s. Methods: %s.", h.Description, strings.Join(mNames, ", "))
		if state != handlerdomain.ConfigStateReady {
			desc += fmt.Sprintf(" configState: %s (missing: %v)", state, missing)
		} else {
			desc += " configState: ready"
		}

		items = append(items, catalogdomain.Item{
			Source: "handler", Name: h.Name, Description: desc, Category: "service",
		})
	}
	return items, nil
}
```

- [ ] Step 1-3:写 + 编译 + commit

---

## Phase 5:LLM Tools(8 个,`internal/app/tool/handler/`)

### Tasks 17-24:8 LLM tools

每个一个文件,~80-150 行。模板见 Plan 01 Task 14。

- [ ] Task 17: `search_handler` (factory + search.go)
- [ ] Task 18: `get_handler`(返 configState)
- [ ] Task 19: `create_handler`(ops 流式)
- [ ] Task 20: `edit_handler`(ops 流式)
- [ ] Task 21: `revert_handler`
- [ ] Task 22: `delete_handler`
- [ ] Task 23: `call_handler`(**关键** — 隐式 acquire instance,流式 progress)
- [ ] Task 24: `update_handler_config`(D16 — 写 partial config)

每 task 一个文件,加 var _ toolapp.Tool = (*X)(nil) compile-time assert。完成跑:

```bash
cd backend && go test ./internal/app/tool/handler/ -count=1
git add . && git commit -m "feat(handler): <name> LLM tool"
git push
```

---

## Phase 6:HTTP API

### Task 25:HTTP handlers 17 endpoints

**Files:**
- Create: `backend/internal/transport/httpapi/handlers/handler.go`
- Modify: `backend/internal/transport/httpapi/router/router.go`
- Modify: `backend/internal/transport/httpapi/router/deps.go`(加 HandlerService)
- Modify: `backend/internal/transport/httpapi/response/errmap.go`(加 17 sentinels)
- Create: `backend/internal/transport/httpapi/handlers/handler_test.go`

17 endpoints per spec 03-handler.md §8.2(含 config 三个 + instance 两个)。

参考 Plan 01 Task 21。

- [ ] Step 1-5

---

## Phase 7:Wire-up + Cross-domain Lifecycle Hooks

### Task 26:main.go + harness 装 HandlerService + 接 lifecycle hooks

**Files:**
- Modify: `backend/cmd/server/main.go`
- Modify: `backend/test/harness/harness.go`
- Modify: `backend/internal/app/conversation/conversation.go`(conv 删 → handler.DestroyAll)

```go
// main.go 装 HandlerService:
handlerRepo := handlerstore.New(gdb)
handlerSvc := handlerapp.NewService(
	handlerRepo,
	handlerapp.NewSandboxAdapter(sandboxSvc, *dataDir),
	handlerinfra.New, // ClientFactory(default)
	encryptor,
	notificationsPub,
	log,
)
go handlerSvc.RunIdleGC(srvBaseCtx) // 起 idle GC goroutine

// 跨 domain hook:conv 删时 → handler.DestroyAll
convService.OnDelete(func(convID string) {
	handlerSvc.DestroyInstancesForOwner(ctx, handlerapp.Owner{Kind: "conversation", ID: convID})
})
```

- [ ] Step 1: 装 handlerSvc
- [ ] Step 2: AutoMigrate 加 Handler + Version
- [ ] Step 3: catalog 注册 source
- [ ] Step 4: LLM 工具加进 tools slice
- [ ] Step 5: conv.OnDelete hook(若 conv service 没此机制,加一个 listener pattern)
- [ ] Step 6: 编译 + test-unit + commit

---

## Phase 7.5:Call Log(D22 — `handler_calls` 表)

类比 Plan 01 Phase 7.5。4 个 task:

### Task 26a:Domain `Call` entity + Repository extension

加 entity per spec 08 §4.2:含 method / instance_id / owner_kind / owner_id 等 handler-specific 字段。ID 前缀 `hcl_<16hex>`。

- [ ] Step 1-3:写 + 编译 + commit

### Task 26b:Store 实现 + 集成测试

参考 Plan 01 Task 23b。新增测试覆盖:`owner_kind` 字段写入(per spec 08 §4.2 — caller-context 标记)。

- [ ] Step 1-3

### Task 26c:Service.Call 终态写 Call row

**Files:** Modify `backend/internal/app/handler/handler.go::Service.Call`

参考 Plan 01 Task 23c。关键差异:**sensitive init_args 在写入前 mask**(spec 08 §9):

```go
input := args
if def.InitArgsSchema.HasSensitive() {
	input = maskSensitive(args, def.InitArgsSchema)
}
exec := buildCallRow(..., input, ...)
```

`maskSensitive` 把 args 里出现的 sensitive arg 名替换为 `"***"`。

- [ ] Step 1-3

### Task 26d:HTTP `GET /api/v1/handlers/{id}/calls`

参考 Plan 01 Task 23d。

- [ ] Step 1-3

### Task 26e:LLM 工具 `search_handler_executions` + `get_handler_execution`(D22 per-entity)

**Files:**
- Create: `backend/internal/app/tool/handler/search_executions.go`
- Create: `backend/internal/app/tool/handler/get_execution.go`
- Modify: `backend/internal/app/tool/handler/handler.go`(factory)

参考 Plan 01 Task 23e。Handler-specific filter:`handlerId / method / ownerKind / instanceId`。Get 工具的 input/output mask sensitive 字段(从 handler.config schema 推断哪些字段是 sensitive,替换为 `***`)。

- [ ] Step 1-4

---

## Phase 8:Pipeline Tests

### Task 27:Handler pipeline tests(3 场景)

**Files:**
- Create: `backend/test/handler/handler_pipeline_test.go`
- Create: `backend/test/handler/lifecycle_pipeline_test.go`

per spec 03-handler.md §14。3 关键场景:

1. **CreateAndCallHandler**:create → call_handler → spawn instance → 调 method → 返 result
2. **ConfigGate**:create → call (no config) → 422 CONFIG_INCOMPLETE → update_config → call (success)
3. **CallerOwnsLifetime**:conv A call → instance live;conv A delete → instance destroyed;conv B 同 handler → 新 instance

- [ ] Step 1-2:写 + 跑 pipeline test + commit

---

## Phase 9:Cross-platform + Doc Sync

### Task 28:三平台编译 + staticcheck + doc sync

参考 Plan 01 Task 25 + 26。

- [ ] **Step 1: 三平台 cross-compile**
- [ ] **Step 2: staticcheck**
- [ ] **Step 3: 写 service-design-documents/handler.md**(把 spec 03-handler.md 内容搬过来 + service-design 格式化)
- [ ] **Step 4: 4 contract docs sync**(api / database / error / events)
- [ ] **Step 5: progress-record dev log + backend-design tree**
- [ ] **Step 6: Commit + push**

---

## Phase 10:PR + Merge

### Task 29:Open PR + merge

- [ ] **Step 1: 推 + 开 PR**

```bash
gh pr create --title "feat(handler): new Handler domain (Definition+Instance, caller-owns, RPC)" --body "$(cat <<'EOF'
## Summary
- 全新 Handler domain — Python class + 多 method + Definition/Instance 二层
- caller-owns instance lifetime(chat conv / FlowRun / test execution scope)
- method-level ops (W) + JSON Merge Patch update_method
- Handler Config(init_args 加密 AES-GCM,复用 apikey domain crypto)
- stdio JSON-RPC client(infra/handler/,代码独立 per D2)
- 8 LLM tools + 17 HTTP endpoints + catalog source(含 configState)
- conv 删 / FlowRun 终态 → instance cascade destroy

## Test plan
- [x] make test-unit 全绿
- [x] make test-pipeline handler 套件通过(create/call + config gate + caller-owns lifetime)
- [x] 三平台 cross-compile 通过
- [x] staticcheck 0
- [x] S14 文档同步

## Related
- spec: documents/version-1.2/adhoc-topic-documents/forge_redesign/03-handler.md
- plan: documents/version-1.2/adhoc-topic-documents/forge_redesign/plans/02-handler-domain.md
EOF
)"
```

---

## Acceptance criteria

1. ✅ 29 task 全 checkbox 打勾
2. ✅ make test-unit + make test-pipeline 通过
3. ✅ 三平台 cross-compile 通
4. ✅ staticcheck 0
5. ✅ S14 文档同步(handler.md + 4 contract + progress + backend-design)
6. ✅ PR merge to main
7. ✅ origin/main 收到 push

完工后,Plan 03(Eventlog scope + HTTP/2 transport)接力。

---

(本 plan 完)
