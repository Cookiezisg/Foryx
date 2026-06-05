package sandbox

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// computeDirSize sums file sizes recursively; returns 0 on any error (a missing
// or unreadable dir simply contributes nothing).
//
// computeDirSize 递归汇总文件大小；任何错误返回 0（缺失/不可读目录就贡献 0）。
func computeDirSize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip broken entries. 跳过损坏条目。
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}

// removeAll wraps os.RemoveAll, refusing a relative path or a filesystem root —
// a guard against a corrupted manifest path triggering a catastrophic delete.
//
// removeAll 包 os.RemoveAll，拒绝相对路径与文件系统根——防 manifest 路径损坏触发灾难性删除。
func removeAll(path string) error {
	if !filepath.IsAbs(path) {
		return os.ErrInvalid
	}
	clean := filepath.Clean(path)
	if isFilesystemRoot(clean) {
		return os.ErrInvalid
	}
	return os.RemoveAll(clean)
}

func isFilesystemRoot(path string) bool {
	if path == "/" || path == "\\" {
		return true
	}
	if len(path) <= 3 && len(path) >= 2 && path[1] == ':' {
		return true
	}
	if strings.TrimSpace(path) == "." {
		return true
	}
	return false
}
