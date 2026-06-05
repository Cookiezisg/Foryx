// Package memory is the domain for per-workspace long-term memory: facts the agent
// keeps across conversations, stored as markdown files under the workspace's
// memories/ directory (user-editable, git-friendly). This package defines the Memory
// value, the file-backed Repository contract, the system-prompt projection port, and
// the name/source rules. There is NO generated id — the slug filename IS the identity.
//
// Package memory 是按 workspace 的长期记忆 domain：agent 跨对话保留的事实，以 markdown
// 文件存在该 workspace 的 memories/ 目录（用户可编辑、git 友好）。本包定义 Memory 值、
// 文件存储契约 Repository、system-prompt 投影端口、name/source 规则。**无生成 id**——slug
// 文件名即身份。
package memory

import (
	"context"
	"regexp"
	"time"

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
)

// Memory is one long-term fact stored as <name>.md (frontmatter + body). Name (the
// slug filename) is the stable identity. Pinned controls injection (full text vs an
// index line); Source records who wrote it (user-authored rule vs ai-learned note).
// UpdatedAt is the file mtime, so a direct file edit is reflected too.
//
// Memory 是一条长期事实，以 <name>.md 存储（frontmatter + 正文）。Name（slug 文件名）即
// 稳定身份。Pinned 控制注入（全文 vs 目录行）；Source 记谁写的（用户规则 vs AI 笔记）。
// UpdatedAt 是文件 mtime，故直接改文件也反映。
type Memory struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Content     string    `json:"content"`
	Pinned      bool      `json:"pinned"`
	Source      string    `json:"source"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

const (
	SourceUser = "user"
	SourceAI   = "ai"
)

// IsValidSource reports whether s is user or ai.
//
// IsValidSource 报告 s 是否 user 或 ai。
func IsValidSource(s string) bool { return s == SourceUser || s == SourceAI }

// NameRegex constrains a memory name to a filesystem-safe slug: lowercase start, then
// lowercase/digits/_/- up to 64 chars. The slug IS the filename (no "/" "." ".."), so
// it cannot escape the memories directory — the file store needs no path guard.
//
// NameRegex 约束 memory name 为文件名安全 slug：小写开头 + 小写/数字/_/-，≤64。slug 即
// 文件名（无 "/" "." ".."），不能逃出 memories 目录——文件 store 无需 path guard。
var NameRegex = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// IsValidName reports whether name is a valid slug.
//
// IsValidName 报告 name 是否合法 slug。
func IsValidName(name string) bool { return NameRegex.MatchString(name) }

var (
	// ErrNotFound: no memory file for the given name.
	// ErrNotFound：给定 name 无对应 memory 文件。
	ErrNotFound = errorsdomain.New(errorsdomain.KindNotFound, "MEMORY_NOT_FOUND", "memory not found")

	// ErrInvalidName: name is not a lowercase slug.
	// ErrInvalidName：name 非小写 slug。
	ErrInvalidName = errorsdomain.New(errorsdomain.KindInvalid, "MEMORY_INVALID_NAME", "invalid memory name (must be a lowercase slug)")

	// ErrInvalidSource: source is not user/ai.
	// ErrInvalidSource：source 非 user/ai。
	ErrInvalidSource = errorsdomain.New(errorsdomain.KindInvalid, "MEMORY_INVALID_SOURCE", "invalid memory source (must be user or ai)")

	// ErrInvalidInput: description or content missing.
	// ErrInvalidInput：description 或 content 缺失。
	ErrInvalidInput = errorsdomain.New(errorsdomain.KindInvalid, "MEMORY_INVALID_INPUT", "memory description and content required")
)

// ListFilter optionally narrows List; nil Pinned = all.
//
// ListFilter 可选收窄 List；nil Pinned = 全部。
type ListFilter struct {
	Pinned *bool
}

// Repository is the file-backed storage contract, scoped to the ctx workspace's
// memories/ directory; the slug name maps 1:1 to a markdown file.
//
// Repository 是文件存储契约，作用于 ctx workspace 的 memories/ 目录；slug name 1:1 映射 md 文件。
type Repository interface {
	List(ctx context.Context, filter ListFilter) ([]*Memory, error)
	Get(ctx context.Context, name string) (*Memory, error)
	Save(ctx context.Context, m *Memory) error
	Delete(ctx context.Context, name string) error
}

// SystemPromptProvider is the narrow port chat consumes to fetch the memory section.
//
// SystemPromptProvider 是 chat 取 memory 段的窄接口。
type SystemPromptProvider interface {
	ForSystemPrompt(ctx context.Context) string
}
