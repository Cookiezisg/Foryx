// destroy.go: forge / EnvID directory removal. Used when a forge is soft-
// deleted (Destroy clears everything) or when N=3 EnvID buffer evicts an
// old environment (DestroyEnv clears one).
//
// Both methods take the per-forge sync lock to avoid yanking a directory
// while a Sync is materializing it; running forges (Run) can race with
// DestroyEnv but that's the LLM's problem and surfaces naturally as a uv
// error on the next call.
//
// destroy.go：forge / EnvID 目录删除。Destroy 在 forge 软删时清整个目录；
// DestroyEnv 在 N=3 EnvID 缓冲驱逐旧环境时清单个 EnvID。
//
// 两者都拿 per-forge sync 锁，避免在 Sync 正在物化时拽走目录；运行中的
// Run 跟 DestroyEnv 可能 race 但那是 LLM 的问题——下一次调用时 uv 自然
// 报错。

package sandbox

import (
	"context"
	"os"
)

// Destroy removes the entire forge directory (envs + versions). Service
// layer calls this when a forge is soft-deleted.
//
// Destroy 删整个 forge 目录（envs + versions）。service 层软删 forge 时调。
func (s *Sandbox) Destroy(ctx context.Context, forgeID string) error {
	if err := s.ensureReady(); err != nil {
		return err
	}

	unlock := s.syncMu.Lock(forgeID)
	defer unlock()

	return os.RemoveAll(forgeDir(s.cfg.DataDir, forgeID))
}

// DestroyEnv removes a single EnvID directory under a forge. Service layer
// calls this when the N=3 EnvID buffer evicts an old environment that no
// longer needs to be kept warm for fast revert.
//
// DestroyEnv 删 forge 下单个 EnvID 目录。service 层在 N=3 EnvID 缓冲驱逐
// 不再需要保温供快速 revert 的旧环境时调。
func (s *Sandbox) DestroyEnv(ctx context.Context, forgeID, envID string) error {
	if err := s.ensureReady(); err != nil {
		return err
	}

	unlock := s.syncMu.Lock(forgeID)
	defer unlock()

	return os.RemoveAll(envDir(s.cfg.DataDir, forgeID, envID))
}
