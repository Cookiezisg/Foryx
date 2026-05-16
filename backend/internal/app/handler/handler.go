// Package handler (app layer) orchestrates handler CRUD, versions, encrypted config, and stdio RPC.
//
// Package handler（app 层）编排 handler CRUD、版本、加密 config 与 stdio RPC。
package handler

import (
	"context"
	"io"
	"time"

	"go.uber.org/zap"

	cryptodomain "github.com/sunweilin/forgify/backend/internal/domain/crypto"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	handlerinfra "github.com/sunweilin/forgify/backend/internal/infra/handler"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
)

// Sandbox is the port for materializing handler venvs and spawning long-lived subprocesses.
//
// Sandbox 是物化 handler venv 与起长跑子进程的端口。
type Sandbox interface {
	// PythonPath returns the bundled Python interpreter path.
	//
	// PythonPath 返回捆绑 Python 解释器路径。
	PythonPath() string

	// Sync materializes the venv for the given EnvID; idempotent.
	//
	// Sync 物化指定 EnvID 的 venv，幂等。
	Sync(ctx context.Context, req SyncRequest) error

	// SpawnLongLived starts a long-running subprocess for one HandlerInstance.
	//
	// SpawnLongLived 为单个 HandlerInstance 启动长跑子进程。
	SpawnLongLived(ctx context.Context, req SpawnRequest) (sandboxdomain.LongLivedHandle, error)

	// WriteCodeFile writes user_handler.py + driver.py to the version's sandbox dir.
	//
	// WriteCodeFile 写 user_handler.py + driver.py 到 version 的 sandbox 目录。
	WriteCodeFile(ctx context.Context, handlerID, versionID, classCode string) error

	// Destroy removes the entire handler directory and every env owned by it.
	//
	// Destroy 删除整个 handler 目录与其下所有 env。
	Destroy(ctx context.Context, handlerID string) error

	// DestroyEnv removes a single (handlerID, envID) env.
	//
	// DestroyEnv 删除单个 (handlerID, envID) env。
	DestroyEnv(ctx context.Context, handlerID, envID string) error
}

// ClientFactory builds a handlerinfra.Client around the given pipes.
//
// ClientFactory 把 pipe 包成 handlerinfra.Client。
type ClientFactory func(stdin io.WriteCloser, stdout io.Reader, log *zap.Logger) handlerinfra.Client

// DefaultClientFactory wraps handlerinfra.New.
//
// DefaultClientFactory 包 handlerinfra.New。
func DefaultClientFactory(stdin io.WriteCloser, stdout io.Reader, log *zap.Logger) handlerinfra.Client {
	return handlerinfra.New(stdin, stdout, log)
}

// Service orchestrates the handler domain.
//
// Service 编排 handler domain。
type Service struct {
	repo       handlerdomain.Repository
	sandbox    Sandbox
	clientFact ClientFactory
	encryptor  cryptodomain.Encryptor
	registry   *instanceRegistry
	notif      notificationspkg.Publisher
	log        *zap.Logger
}

// NewService wires Service; panics on nil log/notif/encryptor.
//
// NewService 装配 Service；nil log/notif/encryptor 直接 panic。
func NewService(
	repo handlerdomain.Repository,
	sandbox Sandbox,
	clientFact ClientFactory,
	encryptor cryptodomain.Encryptor,
	notif notificationspkg.Publisher,
	log *zap.Logger,
) *Service {
	if log == nil {
		panic("handlerapp.NewService: logger is nil")
	}
	if notif == nil {
		panic("handlerapp.NewService: notif is nil")
	}
	if encryptor == nil {
		panic("handlerapp.NewService: encryptor is nil")
	}
	if clientFact == nil {
		clientFact = DefaultClientFactory
	}
	return &Service{
		repo:       repo,
		sandbox:    sandbox,
		clientFact: clientFact,
		encryptor:  encryptor,
		registry:   newInstanceRegistry(),
		notif:      notif,
		log:        log.Named("handlerapp"),
	}
}

// Shutdown destroys every live instance across owners.
//
// Shutdown 销毁所有 owner 的所有实例。
func (s *Service) Shutdown(ctx context.Context) {
	s.registry.DestroyEverything(ctx)
}

var _ = time.Second
