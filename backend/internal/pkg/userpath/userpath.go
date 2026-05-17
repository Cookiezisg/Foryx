// Package userpath builds per-user filesystem roots under ~/.forgify/users/<uid>/
// and migrates pre-multiuser legacy paths into the default user's bucket.
//
// Package userpath 构建 per-user 文件系统根目录 ~/.forgify/users/<uid>/
// 并把多用户化前的 legacy 路径迁到默认 user 的桶里。
package userpath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// UserHome returns <homeRoot>/users/<uid>/ ; mkdir-p on first call.
//
// UserHome 返 <homeRoot>/users/<uid>/；首次调用 mkdir-p。
func UserHome(homeRoot, uid string) (string, error) {
	if homeRoot == "" || uid == "" {
		return "", fmt.Errorf("userpath.UserHome: homeRoot and uid required")
	}
	dir := filepath.Join(homeRoot, "users", uid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("userpath.UserHome: %w", err)
	}
	return dir, nil
}

// MigrateLegacy moves pre-multiuser files from homeRoot/<name> to homeRoot/users/<uid>/<name>
// when the new path doesn't exist yet. No-op when source missing or target already present.
//
// MigrateLegacy 把单用户期残留的 homeRoot/<name> 迁到 homeRoot/users/<uid>/<name>，
// 仅当目标不存在时迁；source 不存在或 target 已在 → 静默 no-op。
func MigrateLegacy(homeRoot, uid string, names ...string) error {
	if homeRoot == "" || uid == "" {
		return fmt.Errorf("userpath.MigrateLegacy: homeRoot and uid required")
	}
	userDir := filepath.Join(homeRoot, "users", uid)
	if err := os.MkdirAll(userDir, 0o755); err != nil {
		return fmt.Errorf("userpath.MigrateLegacy: mkdir userDir: %w", err)
	}
	for _, name := range names {
		legacy := filepath.Join(homeRoot, name)
		target := filepath.Join(userDir, name)
		if !exists(legacy) {
			continue
		}
		if exists(target) {
			continue
		}
		if err := os.Rename(legacy, target); err != nil {
			return fmt.Errorf("userpath.MigrateLegacy: rename %s → %s: %w", legacy, target, err)
		}
	}
	return nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil || !errors.Is(err, os.ErrNotExist)
}
