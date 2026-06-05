package sandbox

import "context"

// ToolRegistry resolves a support-tool (kind, version) to an absolute binary
// path, lazily installing the underlying runtime when absent. It is implemented
// by app/sandbox Service (production); tests use in-memory fakes.
//
// ToolRegistry 把支持工具 (kind, version) 解析为绝对二进制路径，缺则懒装底层 runtime。
// 由 app/sandbox Service 实现（生产）；测试用内存 fake。
type ToolRegistry interface {
	// EnsureTool returns the absolute path to kind's primary binary, installing
	// if absent. version="" means the kind's default. Returns
	// ErrRuntimeNotSupported when no installer is registered, or
	// ErrRuntimeInstallFailed (wrapping stderr) on install failure.
	//
	// EnsureTool 返 kind 主二进制绝对路径，缺则装。version="" 表该 kind 默认。无 installer
	// 返 ErrRuntimeNotSupported；装失败返 ErrRuntimeInstallFailed（含 stderr）。
	EnsureTool(ctx context.Context, kind, version string) (binPath string, err error)
}
