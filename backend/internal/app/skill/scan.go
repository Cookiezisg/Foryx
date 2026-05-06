// scan.go — Service.Scan walks ~/.forgify/skills/*/SKILL.md, parses
// each frontmatter, validates the minimum-required fields, and rebuilds
// the in-memory cache. Single hard requirement per skill.md §3: scan
// only the user-level dir; no project-level merge, no Claude Code /
// Cursor cross-vendor scanning.
//
// scan.go ——Service.Scan 走 ~/.forgify/skills/*/SKILL.md，解析每个
// frontmatter，校验最小必填字段，重建内存缓存。skill.md §3 唯一硬约束：
// 仅扫用户级目录；无项目级 merge，无跨厂扫描。
package skill

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
)

// Scan walks skillsDir, parses every SKILL.md, validates each one's
// frontmatter, and replaces the in-memory cache wholesale. Per-skill
// errors are logged + skipped (one bad skill must not silence the
// catalog); the call only returns top-level errors (skillsDir missing,
// I/O failure on the dir itself).
//
// After a successful scan, publishes the SSE 'skill' snapshot.
//
// Scan 走 skillsDir，解析每个 SKILL.md，校验各自的 frontmatter，整体替换
// 内存缓存。per-skill 错误 log + 跳过（一个坏 skill 不能让 catalog 静默）；
// 仅 top-level 错误（skillsDir 缺、目录 I/O 失败）才返回。成功后发 SSE
// 'skill' 快照。
func (s *Service) Scan(ctx context.Context) error {
	if s.skillsDir == "" {
		return fmt.Errorf("skillapp.Scan: skillsDir is empty")
	}

	// Missing dir is benign (user just hasn't installed any skills) —
	// reset cache to empty + return nil so downstream code (catalog,
	// search) sees a valid empty list.
	// 目录缺无害（用户还没装 skill）——重置 cache 为空 + 返 nil 让下游
	// （catalog / search）看到有效空列表。
	if _, err := os.Stat(s.skillsDir); errors.Is(err, fs.ErrNotExist) {
		s.mu.Lock()
		s.skills = map[string]*skilldomain.Skill{}
		s.mu.Unlock()
		s.publishSnapshot(ctx)
		return nil
	}

	entries, err := os.ReadDir(s.skillsDir)
	if err != nil {
		return fmt.Errorf("skillapp.Scan: read skillsDir: %w", err)
	}

	loaded := map[string]*skilldomain.Skill{}
	for _, ent := range entries {
		if !ent.IsDir() {
			// Top-level files are ignored — skill format is one-dir-per-skill.
			// 顶层文件忽略——skill 格式 one-dir-per-skill。
			continue
		}
		dir := filepath.Join(s.skillsDir, ent.Name())
		sk, err := s.parseSkillDir(dir)
		if err != nil {
			s.log.Warn("skill skipped",
				zap.String("dir", dir), zap.Error(err))
			continue
		}
		if existing, dup := loaded[sk.Name]; dup {
			// Duplicate frontmatter.name across two dirs — the first one
			// wins (alpha order from ReadDir on most fs); log the
			// rejected one with both paths for the user to deconflict.
			// 两目录 frontmatter.name 重——首个胜出（多数 fs 字母序）；
			// log 被拒条目含两路径让用户去重。
			s.log.Warn("skill name collision; later one ignored",
				zap.String("name", sk.Name),
				zap.String("kept", existing.DirPath),
				zap.String("rejected", dir))
			continue
		}
		loaded[sk.Name] = sk
	}

	s.mu.Lock()
	s.skills = loaded
	s.mu.Unlock()
	s.publishSnapshot(ctx)
	return nil
}

// parseSkillDir reads <dir>/SKILL.md, splits frontmatter from body,
// parses YAML, validates the required fields, and returns a populated
// Skill (without caching the body — bodies are re-read on every Activate
// per §9.5).
//
// parseSkillDir 读 <dir>/SKILL.md，分离 frontmatter 与 body，解析 YAML，
// 校验必填，返填充好的 Skill（不缓存 body——§9.5 每次 Activate 重读）。
func (s *Service) parseSkillDir(dir string) (*skilldomain.Skill, error) {
	bodyPath := filepath.Join(dir, "SKILL.md")
	raw, err := os.ReadFile(bodyPath)
	if err != nil {
		return nil, fmt.Errorf("read SKILL.md: %w", err)
	}
	if len(raw) > skilldomain.MaxBodyBytes {
		return nil, fmt.Errorf("%w: %d bytes (cap %d)",
			skilldomain.ErrBodyTooLarge, len(raw), skilldomain.MaxBodyBytes)
	}

	yamlPart, _, err := splitFrontmatter(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", skilldomain.ErrInvalidFrontmatter, err)
	}

	var fm skilldomain.Frontmatter
	if err := yaml.Unmarshal(yamlPart, &fm); err != nil {
		return nil, fmt.Errorf("%w: yaml: %v", skilldomain.ErrInvalidFrontmatter, err)
	}
	if err := validateFrontmatter(fm); err != nil {
		return nil, err
	}

	// frontmatter.name takes priority; fall back to dir base name (Claude
	// Code allows omitting name when dir name is the canonical identifier).
	// frontmatter.name 优先；缺则用目录名（Claude Code 允许省略 name 当目
	// 录名是规范标识符）。
	name := strings.TrimSpace(fm.Name)
	if name == "" {
		name = filepath.Base(dir)
	}

	return &skilldomain.Skill{
		Name:        name,
		Source:      "user",
		DirPath:     dir,
		BodyPath:    bodyPath,
		Description: fm.Description,
		Frontmatter: fm,
		LoadedAt:    time.Now().UTC(),
	}, nil
}

// splitFrontmatter separates the leading `---\nYAML\n---\n` block from
// the markdown body. The opening fence must be the very first line; the
// closing fence is the first `---\n` (or `---\r\n`) line on its own
// after the opener. Both LF and CRLF line endings are tolerated so
// Windows-edited files don't fail.
//
// splitFrontmatter 把首部 `---\nYAML\n---\n` 块从 markdown body 分离。
// 开围栏必须是首行；闭围栏是开后第一行独立 `---\n`（或 `---\r\n`）。
// LF / CRLF 兼容让 Windows 编辑过的文件不挂。
func splitFrontmatter(content []byte) (yamlPart, mdBody []byte, err error) {
	// Strip UTF-8 BOM if present (Notepad-edited files commonly carry one).
	// 剥 UTF-8 BOM（Notepad 编辑过的文件常带）。
	content = bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})

	// Normalize CRLF → LF for the splitter; preserve original in body.
	// CRLF → LF 给 splitter；body 保留原文。
	normalized := bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))

	if !bytes.HasPrefix(normalized, []byte("---\n")) {
		return nil, nil, errors.New("missing opening --- fence")
	}
	rest := normalized[4:]
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		// Allow closing fence at end-of-file with no trailing newline.
		// 允许闭围栏在文件末尾无末尾换行。
		if bytes.HasSuffix(rest, []byte("\n---")) {
			end = len(rest) - 4
			yamlPart = rest[:end]
			return yamlPart, nil, nil
		}
		return nil, nil, errors.New("missing closing --- fence")
	}
	yamlPart = rest[:end]
	mdBody = rest[end+5:]
	return yamlPart, mdBody, nil
}

// validateFrontmatter enforces the minimum required-fields contract per
// Anthropic spec: description must be non-empty (it's what the L1 catalog
// shows the LLM); description length cap 1536 chars.
//
// validateFrontmatter 强制 Anthropic spec 的最小必填契约：description 非
// 空（L1 catalog 给 LLM 看的就是它）；description 长度 ≤ 1536。
func validateFrontmatter(fm skilldomain.Frontmatter) error {
	desc := strings.TrimSpace(fm.Description)
	if desc == "" {
		return fmt.Errorf("%w: description required", skilldomain.ErrInvalidFrontmatter)
	}
	if len(desc) > skilldomain.MaxDescriptionChars {
		return fmt.Errorf("%w: description %d chars exceeds %d",
			skilldomain.ErrInvalidFrontmatter, len(desc), skilldomain.MaxDescriptionChars)
	}
	// fork mode requires an Agent type so we know which subagent to
	// dispatch to. Empty Agent + fork would silently fall back to
	// general-purpose; we require explicit declaration so the author's
	// intent is clear and configuration drift surfaces at scan time
	// rather than at first activate.
	// fork 模式要求声明 Agent type 让我们知道派发哪个 subagent。空 Agent
	// + fork 会静默回落 general-purpose；要求显式声明让作者意图清晰、配置
	// 漂移在 scan 时暴露而非首次 activate 时。
	if fm.Context == "fork" && strings.TrimSpace(fm.Agent) == "" {
		return fmt.Errorf("%w: context=fork requires agent: <type>",
			skilldomain.ErrInvalidFrontmatter)
	}
	return nil
}
