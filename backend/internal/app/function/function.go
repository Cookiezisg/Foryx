// Package function (app layer) orchestrates function CRUD, version lifecycle, sandbox execution, and env management.
//
// Package function（app 层）编排 function CRUD、版本生命周期、沙箱执行与 env 管理。
package function

import (
	"context"

	"go.uber.org/zap"

	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
)

// Sandbox is the port for materializing function venvs and executing code.
//
// Sandbox 是物化 function venv 与执行代码的端口。
type Sandbox interface {
	// PythonPath returns the absolute path to the bundled raw Python interpreter (no venv).
	//
	// PythonPath 返回捆绑 raw Python 解释器路径（不在任何 venv 内）。
	PythonPath() string

	// Sync materializes the venv for the given EnvID; idempotent.
	//
	// Sync 物化指定 EnvID 的 venv，幂等。
	Sync(ctx context.Context, req SyncRequest) error

	// Run executes a function in its EnvID's venv; ctx-cancel kills the whole process tree.
	//
	// Run 在 EnvID 的 venv 内执行 function，ctx-cancel 杀整个进程树。
	Run(ctx context.Context, req RunRequest) (*functiondomain.ExecutionResult, error)

	// WriteCodeFile rewrites main.py for a version without touching its venv.
	//
	// WriteCodeFile 重写 version 的 main.py，不动 venv。
	WriteCodeFile(ctx context.Context, functionID, versionID, code, entryFunction string) error

	// Destroy removes the entire function directory.
	//
	// Destroy 删除整个 function 目录。
	Destroy(ctx context.Context, functionID string) error

	// DestroyEnv removes a single EnvID directory under a function.
	//
	// DestroyEnv 删除 function 下单个 EnvID 目录。
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

// NewService wires Service dependencies; panics on nil logger or notif.
//
// NewService 装配 Service 依赖，nil logger 或 notif 直接 panic。
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
