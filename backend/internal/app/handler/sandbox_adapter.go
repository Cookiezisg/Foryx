// sandbox_adapter.go — bridges the handler.Sandbox port to sandboxapp.Service.
//
// Mirrors function/sandbox_adapter.go structure (D5 — each trinity owns its
// own copy). Owner.Kind = "handler"; Owner.ID = "<handlerID>_<envID>".
//
// File layout:  <dataDir>/handlers/<handlerID>/versions/<versionID>/
//                   user_handler.py     (AssembleClass output)
//                   driver.py           (constant DriverScript)
//
// Subprocess runs `python driver.py` with PYTHONPATH set to the version dir.
//
// sandbox_adapter.go —— 桥接 handler.Sandbox 端口到 sandboxapp.Service。
// 跟 function adapter 同结构(D5)。Owner.Kind="handler",ID=<handlerID>_<envID>。

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

// SandboxAdapter satisfies the handler.Sandbox interface by delegating to
// sandboxapp.Service for runtime / env / spawn while owning the handler-
// specific file layout.
//
// SandboxAdapter 通过委托 sandboxapp.Service 管 runtime/env/spawn 满足
// handler.Sandbox 接口,同时拥有 handler 专属文件布局。
type SandboxAdapter struct {
	svc             *sandboxapp.Service
	handlerDataDir  string // <dataDir> — adapter writes versions/<vID>/ files under <dataDir>/handlers/

	pythonPathOnce sync.Once
	pythonPath     string
}

// Static-assert SandboxAdapter implements Sandbox.
var _ Sandbox = (*SandboxAdapter)(nil)

// NewSandboxAdapter wires the adapter to a sandbox service + data root.
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

// Sync materializes the venv via Service.EnsureEnv. Owner.Kind="handler".
//
// Sync 经 Service.EnsureEnv 物化 venv;Owner.Kind="handler"。
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

// SpawnLongLived starts the python driver subprocess. Caller (Service) must
// have WriteCodeFile'd user_handler.py + driver.py first.
//
// SpawnLongLived 起 python driver 子进程;调用方先 WriteCodeFile。
func (a *SandboxAdapter) SpawnLongLived(ctx context.Context, req SpawnRequest) (sandboxdomain.LongLivedHandle, error) {
	owner := sandboxdomain.Owner{
		Kind: sandboxdomain.OwnerKindHandler,
		ID:   req.HandlerID + "_" + req.EnvID,
	}
	verDir := a.versionDir(req.HandlerID, req.VersionID)
	driverPath := filepath.Join(verDir, "driver.py")

	env := map[string]string{
		// PYTHONPATH lets driver.py do `from user_handler import HandlerImpl`.
		"PYTHONPATH":  verDir,
		"PYTHONUNBUFFERED": "1", // disable stdout buffering — stdio RPC needs prompt lines
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

// WriteCodeFile writes user_handler.py + driver.py to the version dir.
//
// WriteCodeFile 写 user_handler.py + driver.py 到版本目录。
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

// Destroy removes every env owned by this handler + the handler dir on disk.
//
// Destroy 删该 handler 所有 env + 盘上 versions 目录。
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
// DestroyEnv 删单个 (handlerID, envID) env。
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

// writeAtomic writes via tmp + rename so readers never see a half-written
// file. Same helper as function's; duplicated per D5.
//
// writeAtomic tmp + rename 原子写。
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
