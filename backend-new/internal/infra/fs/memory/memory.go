// Package memory is the file-backed memorydomain.Repository: each memory is a
// markdown file under <base>/workspaces/<wsID>/memories/<name>.md. The workspace id
// comes from ctx (one directory per workspace = full isolation); the slug name maps
// 1:1 to a file and cannot traverse (validated). This is backend-new's first
// file-backed store — skills (波次 3) reuses the pattern.
//
// Package memory 是文件式 memorydomain.Repository：每条记忆是
// <base>/workspaces/<wsID>/memories/<name>.md。workspace id 取自 ctx（每 workspace 一个
// 目录 = 完全隔离）；slug name 1:1 映射文件、不能穿越（已校验）。这是 backend-new 第一个
// 文件式 store——skills（波次 3）复用此范式。
package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	memorydomain "github.com/sunweilin/forgify/backend/internal/domain/memory"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Store is the file-backed memory repository. base is the ~/.forgify root (injected
// at boot, M7; a temp dir in tests); the per-workspace memories dir lives under it.
//
// Store 是文件式 memory 仓库。base 是 ~/.forgify 根（boot 装配 M7；测试用 temp）；
// 各 workspace 的 memories 目录在其下。
type Store struct {
	base string
}

// New builds a Store rooted at base.
//
// New 构造以 base 为根的 Store。
func New(base string) *Store { return &Store{base: base} }

var _ memorydomain.Repository = (*Store)(nil)

// dir resolves the ctx workspace's memories directory.
//
// dir 解析 ctx workspace 的 memories 目录。
func (s *Store) dir(ctx context.Context) (string, error) {
	wsID, err := reqctxpkg.RequireWorkspaceID(ctx)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.base, "workspaces", wsID, "memories"), nil
}

// path resolves a memory's file path; name must be a valid slug (no traversal).
//
// path 解析一条记忆的文件路径；name 须为合法 slug（不穿越）。
func (s *Store) path(ctx context.Context, name string) (string, error) {
	if !memorydomain.IsValidName(name) {
		return "", memorydomain.ErrInvalidName
	}
	dir, err := s.dir(ctx)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".md"), nil
}

// List scans the workspace's memories dir, parses each .md, optionally filters by
// pinned, sorted by name. A missing dir (brand-new workspace) yields an empty list.
//
// List 扫该 workspace 的 memories 目录、解析每个 .md、可按 pinned 过滤、按 name 排序。
// 目录不存在（全新 workspace）返空列表。
func (s *Store) List(ctx context.Context, filter memorydomain.ListFilter) ([]*memorydomain.Memory, error) {
	dir, err := s.dir(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("memoryfs.List: %w", err)
	}
	var out []*memorydomain.Memory
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		m, err := s.readFile(filepath.Join(dir, e.Name()), name)
		if err != nil {
			continue // skip an unreadable file rather than fail the whole list
		}
		if filter.Pinned != nil && m.Pinned != *filter.Pinned {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Get reads one memory by name; ErrNotFound when the file is absent.
//
// Get 按 name 读一条；文件不存在返 ErrNotFound。
func (s *Store) Get(ctx context.Context, name string) (*memorydomain.Memory, error) {
	p, err := s.path(ctx, name)
	if err != nil {
		return nil, err
	}
	m, err := s.readFile(p, name)
	if os.IsNotExist(err) {
		return nil, memorydomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("memoryfs.Get: %w", err)
	}
	return m, nil
}

// Save writes a memory atomically (temp + rename), creating the dir if needed.
//
// Save 原子写一条记忆（temp + rename），按需建目录。
func (s *Store) Save(ctx context.Context, m *memorydomain.Memory) error {
	p, err := s.path(ctx, m.Name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("memoryfs.Save mkdir: %w", err)
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, []byte(renderFile(m)), 0o644); err != nil {
		return fmt.Errorf("memoryfs.Save write: %w", err)
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("memoryfs.Save rename: %w", err)
	}
	return nil
}

// Delete removes a memory file; ErrNotFound when absent.
//
// Delete 删一条记忆文件；不存在返 ErrNotFound。
func (s *Store) Delete(ctx context.Context, name string) error {
	p, err := s.path(ctx, name)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		if os.IsNotExist(err) {
			return memorydomain.ErrNotFound
		}
		return fmt.Errorf("memoryfs.Delete: %w", err)
	}
	return nil
}

// readFile reads + parses one memory file; UpdatedAt is the file mtime.
//
// readFile 读 + 解析一个记忆文件；UpdatedAt 取文件 mtime。
func (s *Store) readFile(path, name string) (*memorydomain.Memory, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := parseFile(string(raw), name)
	if info, err := os.Stat(path); err == nil {
		m.UpdatedAt = info.ModTime().UTC()
	}
	return m, nil
}
