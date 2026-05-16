package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	sandboxapp "github.com/sunweilin/forgify/backend/internal/app/sandbox"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

// SandboxAdapter satisfies handler.Sandbox by delegating to sandboxapp.Service.
//
// SandboxAdapter 把 runtime/env/spawn 委托给 sandboxapp.Service 满足 handler.Sandbox。
type SandboxAdapter struct {
	svc            *sandboxapp.Service
	handlerDataDir string

	pythonPathOnce sync.Once
	pythonPath     string
}

var _ Sandbox = (*SandboxAdapter)(nil)

// NewSandboxAdapter wires the adapter to a sandbox service and data root.
//
// NewSandboxAdapter 装配 adapter。
func NewSandboxAdapter(svc *sandboxapp.Service, handlerDataDir string) *SandboxAdapter {
	return &SandboxAdapter{svc: svc, handlerDataDir: handlerDataDir}
}

// PythonPath lazy-resolves the bundled Python interpreter.
//
// PythonPath 懒解析捆绑 Python 解释器。
func (a *SandboxAdapter) PythonPath() string {
	a.pythonPathOnce.Do(func() {
		path, err := a.svc.EnsureTool(context.Background(), "python", "")
		if err != nil {
			return
		}
		a.pythonPath = path
	})
	return a.pythonPath
}

// Sync materializes the venv via Service.EnsureEnv.
//
// Sync 经 Service.EnsureEnv 物化 venv。
func (a *SandboxAdapter) Sync(ctx context.Context, req SyncRequest) error {
	owner := sandboxdomain.Owner{
		Kind: sandboxdomain.OwnerKindHandler,
		ID:   req.HandlerID + "_" + req.EnvID,
	}
	spec := sandboxdomain.EnvSpec{
		Runtime: sandboxdomain.RuntimeSpec{Kind: "python", Version: req.PythonVersion},
		Deps:    req.Dependencies,
	}
	var stream sandboxdomain.ProgressFunc
	if req.OnProgress != nil {
		stream = func(stage, message string, _ int) {
			req.OnProgress(stage, message)
		}
	}
	if _, err := a.svc.EnsureEnv(ctx, owner, spec, stream); err != nil {
		return &SyncError{Cause: err, Stderr: err.Error()}
	}
	return nil
}

// SpawnLongLived starts the python driver subprocess; caller must have written the code first.
//
// SpawnLongLived 启动 python driver 子进程，调用方需先 WriteCodeFile。
func (a *SandboxAdapter) SpawnLongLived(ctx context.Context, req SpawnRequest) (sandboxdomain.LongLivedHandle, error) {
	owner := sandboxdomain.Owner{
		Kind: sandboxdomain.OwnerKindHandler,
		ID:   req.HandlerID + "_" + req.EnvID,
	}
	verDir := a.versionDir(req.HandlerID, req.VersionID)
	driverPath := filepath.Join(verDir, "driver.py")

	env := map[string]string{
		"PYTHONPATH":       verDir,
		"PYTHONUNBUFFERED": "1",
	}
	for k, v := range req.Env {
		env[k] = v
	}

	handle, err := a.svc.SpawnLongLived(ctx, owner, sandboxdomain.SpawnOpts{
		Cmd:       "python",
		Args:      []string{driverPath},
		Env:       env,
		LongLived: true,
	})
	if err != nil {
		return nil, fmt.Errorf("handlerapp.SandboxAdapter.SpawnLongLived: %w", err)
	}
	return handle, nil
}

// WriteCodeFile writes user_handler.py and driver.py to the version dir.
//
// WriteCodeFile 写 user_handler.py 与 driver.py 到版本目录。
func (a *SandboxAdapter) WriteCodeFile(ctx context.Context, handlerID, versionID, classCode string) error {
	verDir := a.versionDir(handlerID, versionID)
	if err := os.MkdirAll(verDir, 0o755); err != nil {
		return fmt.Errorf("handlerapp.SandboxAdapter.WriteCodeFile: mkdir: %w", err)
	}
	if err := writeAtomic(filepath.Join(verDir, "user_handler.py"), []byte(classCode), 0o644); err != nil {
		return fmt.Errorf("handlerapp.SandboxAdapter.WriteCodeFile: user_handler.py: %w", err)
	}
	if err := writeAtomic(filepath.Join(verDir, "driver.py"), []byte(DriverScript), 0o644); err != nil {
		return fmt.Errorf("handlerapp.SandboxAdapter.WriteCodeFile: driver.py: %w", err)
	}
	return nil
}

// Destroy removes every env owned by this handler and the handler dir on disk.
//
// Destroy 删除该 handler 的所有 env 与盘上目录。
func (a *SandboxAdapter) Destroy(ctx context.Context, handlerID string) error {
	envs, err := a.svc.ListEnvs(ctx, sandboxdomain.OwnerKindHandler)
	if err != nil {
		return fmt.Errorf("handlerapp.SandboxAdapter.Destroy: list envs: %w", err)
	}
	prefix := handlerID + "_"
	for _, e := range envs {
		if !strings.HasPrefix(e.OwnerID, prefix) {
			continue
		}
		owner := sandboxdomain.Owner{Kind: sandboxdomain.OwnerKindHandler, ID: e.OwnerID}
		if err := a.svc.Destroy(ctx, owner); err != nil {
			return fmt.Errorf("handlerapp.SandboxAdapter.Destroy %s: %w", owner.ID, err)
		}
	}
	handlerDir := filepath.Join(a.handlerDataDir, "handlers", handlerID)
	if err := os.RemoveAll(handlerDir); err != nil {
		return fmt.Errorf("handlerapp.SandboxAdapter.Destroy: rm %s: %w", handlerDir, err)
	}
	return nil
}

// DestroyEnv removes a single (handlerID, envID) env.
//
// DestroyEnv 删除单个 (handlerID, envID) env。
func (a *SandboxAdapter) DestroyEnv(ctx context.Context, handlerID, envID string) error {
	owner := sandboxdomain.Owner{
		Kind: sandboxdomain.OwnerKindHandler,
		ID:   handlerID + "_" + envID,
	}
	return a.svc.Destroy(ctx, owner)
}

func (a *SandboxAdapter) versionDir(handlerID, versionID string) string {
	return filepath.Join(a.handlerDataDir, "handlers", handlerID, "versions", versionID)
}

// writeAtomic is concurrency-safe — uses os.CreateTemp so parallel writers
// don't collide on the same `<path>.tmp`. See sister copy in
// function/sandbox_adapter.go for the rationale.
//
// writeAtomic 并发安全——经 os.CreateTemp 取唯一 tmp 名，避免多 goroutine
// 撞同名 `.tmp` 导致 rename "no such file" 错误。同型 fix 见 function 包。
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir, base := filepath.Split(path)
	f, err := os.CreateTemp(dir, base+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
