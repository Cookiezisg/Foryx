package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	catalogdomain "github.com/sunweilin/forgify/backend/internal/domain/catalog"
)

// loadFromDisk returns (cat, nil) on success, (nil, nil) when absent, (nil, err) on corruption.
//
// loadFromDisk 成功返 (cat, nil)，缺文件返 (nil, nil)，损坏返 (nil, err)。
func loadFromDisk(path string) (*catalogdomain.Catalog, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("catalog: read %s: %w", path, err)
	}
	var cat catalogdomain.Catalog
	if err := json.Unmarshal(raw, &cat); err != nil {
		bak := path + ".bak"
		_ = os.Rename(path, bak)
		return nil, fmt.Errorf("catalog: parse %s (moved to %s): %w", path, bak, err)
	}
	return &cat, nil
}

// saveToDisk writes the catalog atomically (.tmp + rename) at 0644.
//
// saveToDisk 用 .tmp + rename 原子写 catalog，权限 0644。
func saveToDisk(path string, cat *catalogdomain.Catalog) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("catalog: mkdir %s: %w", filepath.Dir(path), err)
	}
	raw, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		return fmt.Errorf("catalog: marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("catalog: write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("catalog: rename: %w", err)
	}
	return nil
}
