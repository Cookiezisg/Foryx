// Package function (app layer) owns the Service that orchestrates the function
// domain: CRUD, version/pending lifecycle, sandbox execution, env management.
//
// All three function packages (domain / app / store) declare `package function`;
// external callers alias at import (e.g. functionapp "…/internal/app/function").
// Design: documents/version-1.2/adhoc-topic-documents/forge_redesign/02-function.md.
//
// Package function(app 层)负责 Service 编排 function domain:CRUD、版本/
// pending 生命周期、沙箱执行、env 管理。
//
// 三个 function 包均声明 `package function`;外部按角色起别名,如
// functionapp "…/internal/app/function"。设计见 02-function.md。
package function

import (
	"context"

	"go.uber.org/zap"

	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
)

// Sandbox is the port through which Service materializes function venvs and
// executes function code. The infra/sandbox v2 service (wrapped by an adapter)
// provides the concrete implementation. Service tracks env state in
// FunctionVersion rows; the Sandbox is filesystem / subprocess only.
//
// Sandbox 是 Service 物化 function venv + 执行代码的端口。具体实现由
// infra/sandbox v2 service(经 adapter 包装)提供。Service 负责把环境状态
// 记到 FunctionVersion 行;Sandbox 只管文件系统 / 子进程。
type Sandbox interface {
	// PythonPath returns the absolute path to the bundled Python interpreter
	// (raw, not in any venv). Used by Service.validate to invoke the AST
	// extraction helper without going through uv.
	//
	// PythonPath 返捆绑 Python 解释器的绝对路径(raw,不在任何 venv 内)。
	// validate 调 AST 提取 helper 时用,不走 uv。
	PythonPath() string

	// Sync materializes the venv directory for the given EnvID. Idempotent —
	// already-existing .venv returns nil immediately. Adapter implementations
	// wrap underlying errors (e.g. uv stderr) in *SyncError.
	//
	// Sync 物化指定 EnvID 的 venv 目录。幂等——.venv 已存在则立即返 nil。
	// adapter 在底层工具报错时返 *SyncError。
	Sync(ctx context.Context, req SyncRequest) error

	// Run executes a function in its EnvID's venv. ctx-cancel kills the whole
	// process tree. No timeout enforced (per-call timeout is enforced by
	// Service.Run wrapping ctx with context.WithTimeout).
	//
	// Run 在 EnvID 的 venv 中执行 function。ctx-cancel 杀整个进程树。
	// 自身不限 timeout(节点级 timeout 由 Service.Run 包 ctx.WithTimeout 控制)。
	Run(ctx context.Context, req RunRequest) (*functiondomain.ExecutionResult, error)

	// WriteCodeFile updates main.py for a version without touching its venv.
	// Used when EnvID is unchanged but code changed.
	//
	// WriteCodeFile 写 version 的 main.py 不动 venv。EnvID 不变只代码变时用。
	WriteCodeFile(ctx context.Context, functionID, versionID, code, entryFunction string) error

	// Destroy removes the entire function directory.
	// Destroy 删整个 function 目录。
	Destroy(ctx context.Context, functionID string) error

	// DestroyEnv removes a single EnvID directory under a function — used to
	// evict an old EnvID's venv beyond the per-function EnvID buffer cap.
	//
	// DestroyEnv 删 function 下单个 EnvID 目录——超过 per-function EnvID buffer
	// 上限时驱逐旧 EnvID 的 venv 用。
	DestroyEnv(ctx context.Context, functionID, envID string) error
}

// Service orchestrates the function domain.
//
// Service 编排 function domain。
type Service struct {
	repo    functiondomain.Repository
	sandbox Sandbox
	notif   notificationspkg.Publisher
	log     *zap.Logger
}

// NewService wires Service dependencies. Panics on nil logger.
//
// Notifications: Service publishes `function` entity events (created /
// updated / deleted / pending_created / version_accepted / pending_rejected /
// reverted / env_rebuilt) via notif.Publish — per-user routing (D-redo-3
// post-2026-05-12) with slim payloads (D-redo-6 — UI does GET for full
// entity).
//
// Function Service itself does NOT push eventlog or forge SSE streams;
// the tools that wrap function operations (create_function / edit_function /
// run_function / etc.) emit eventlog progress / tool_result blocks via
// pkg/eventlog.Emitter, and forge_started / forge_env_attempt / forge_completed
// via pkg/forge.Publisher (per CLAUDE.md §S18 + §E1).
//
// NewService 装配 Service 依赖。nil logger panic。
//
// 通知:Service 经 notif.Publish 推 `function` entity 事件(per-user 路由,
// D-redo-3;瘦身 payload D-redo-6)。Service 不推 eventlog / forge SSE;包装
// function 操作的 tool 经 pkg/eventlog + pkg/forge 推(§S18 + §E1)。
func NewService(
	repo functiondomain.Repository,
	sandbox Sandbox,
	notif notificationspkg.Publisher,
	log *zap.Logger,
) *Service {
	if log == nil {
		panic("functionapp.NewService: logger is nil")
	}
	if notif == nil {
		panic("functionapp.NewService: notif is nil")
	}
	return &Service{
		repo:    repo,
		sandbox: sandbox,
		notif:   notif,
		log:     log,
	}
}
