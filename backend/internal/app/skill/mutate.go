package skill

import (
	"context"
	"fmt"
	"strings"

	skilldomain "github.com/sunweilin/anselm/backend/internal/domain/skill"
)

// SaveInput is the create/replace request shape (Name doubles as the slug identity).
//
// SaveInput 是 create/replace 的请求形状（Name 兼作 slug 身份）。
type SaveInput struct {
	Name                   string
	Description            string
	Body                   string
	AllowedTools           []string
	Context                string // inline | fork；空 → inline
	Agent                  string // fork 必填
	Arguments              []string
	DisableModelInvocation bool
	UserInvocable          bool
	Source                 string // user | ai；空 → user
}

// Create writes a brand-new SKILL.md; existing name → ErrNameConflict.
//
// Create 写一个全新 SKILL.md；同名已存在 → ErrNameConflict。
func (s *Service) Create(ctx context.Context, in SaveInput) (*skilldomain.Skill, error) {
	if err := s.validate(in); err != nil {
		return nil, err
	}
	exists, err := s.repo.Exists(ctx, in.Name)
	if err != nil {
		return nil, fmt.Errorf("skillapp.Create: %w", err)
	}
	if exists {
		return nil, skilldomain.ErrNameConflict
	}
	if err := s.write(ctx, in); err != nil {
		return nil, fmt.Errorf("skillapp.Create: %w", err)
	}
	s.notify(ctx, "created", in.Name)
	s.syncBuiltEdge(ctx, in.Name)
	s.syncEquipEdges(ctx, in.Name, in.AllowedTools)
	return s.repo.Get(ctx, in.Name)
}

// Replace overwrites an existing SKILL.md; missing name → ErrNotFound.
//
// Replace 覆盖已存在的 SKILL.md；name 缺失 → ErrNotFound。
func (s *Service) Replace(ctx context.Context, in SaveInput) (*skilldomain.Skill, error) {
	if err := s.validate(in); err != nil {
		return nil, err
	}
	exists, err := s.repo.Exists(ctx, in.Name)
	if err != nil {
		return nil, fmt.Errorf("skillapp.Replace: %w", err)
	}
	if !exists {
		return nil, skilldomain.ErrNotFound
	}
	if err := s.write(ctx, in); err != nil {
		return nil, fmt.Errorf("skillapp.Replace: %w", err)
	}
	s.notify(ctx, "updated", in.Name)
	s.syncEquipEdges(ctx, in.Name, in.AllowedTools) // allowed-tools 可能变，重同步出边
	return s.repo.Get(ctx, in.Name)
}

// Delete removes a skill and purges its relation edges.
//
// Delete 删除一个 skill 并清其关系边。
func (s *Service) Delete(ctx context.Context, name string) error {
	if !skilldomain.IsValidName(name) {
		return skilldomain.ErrInvalidName
	}
	if err := s.repo.Delete(ctx, name); err != nil {
		return fmt.Errorf("skillapp.Delete: %w", err)
	}
	s.notify(ctx, "deleted", name)
	s.purgeRelations(ctx, name)
	return nil
}

// write assembles the frontmatter (defaulting context=inline, source=user) and persists.
//
// write 组装 frontmatter（context 默认 inline、source 默认 user）并持久化。
func (s *Service) write(ctx context.Context, in SaveInput) error {
	mode := in.Context
	if mode == "" {
		mode = skilldomain.ContextInline
	}
	source := in.Source
	if source == "" {
		source = skilldomain.SourceUser
	}
	fm := skilldomain.Frontmatter{
		Name:                   in.Name,
		Description:            in.Description,
		AllowedTools:           in.AllowedTools,
		Context:                mode,
		Agent:                  in.Agent,
		Arguments:              in.Arguments,
		DisableModelInvocation: in.DisableModelInvocation,
		UserInvocable:          in.UserInvocable,
		Source:                 source,
	}
	return s.repo.Save(ctx, in.Name, fm, in.Body)
}

// validate enforces the structural invariants before any write (slug / description / size /
// source / fork-needs-agent). Body/frontmatter physical limits also re-checked at the store.
//
// validate 在写前校验结构不变式（slug / description / 大小 / source / fork 必带 agent）。
func (s *Service) validate(in SaveInput) error {
	if !skilldomain.IsValidName(in.Name) {
		return skilldomain.ErrInvalidName
	}
	if strings.TrimSpace(in.Description) == "" {
		return skilldomain.ErrInvalidFrontmatter.WithDetails(map[string]any{"reason": "description required"})
	}
	if len(in.Description) > skilldomain.MaxDescriptionChars {
		return skilldomain.ErrInvalidFrontmatter.WithDetails(map[string]any{"reason": "description too long"})
	}
	if len(in.Body) > skilldomain.MaxBodyBytes {
		return skilldomain.ErrBodyTooLarge
	}
	// A body that opens with its own YAML frontmatter (--- … ---) would assemble into a DOUBLE
	// frontmatter: the platform writes the real frontmatter (from the name/allowedTools args) and the
	// body's block becomes plain content — so an agent that put allowedTools in a body frontmatter has
	// them SILENTLY dropped (never honored). Reject it with a clear pointer to the right place.
	// 以自带 YAML frontmatter（--- … ---）开头的 body 会组装成**双 frontmatter**：平台写真 frontmatter（取自
	// name/allowedTools 参数），body 的块沦为正文——把 allowedTools 塞进 body frontmatter 的 agent 就被静默丢。
	if bodyHasLeadingFrontmatter(in.Body) {
		return skilldomain.ErrInvalidFrontmatter.WithDetails(map[string]any{
			"reason": "the skill body must not begin with its own YAML frontmatter (--- ... ---); the platform assembles the frontmatter from the name/description/allowedTools arguments — put those there, the body is the instruction content only (otherwise a body frontmatter is silently treated as content and its allowedTools are dropped)"})
	}
	if in.Source != "" && !skilldomain.IsValidSource(in.Source) {
		return skilldomain.ErrInvalidFrontmatter.WithDetails(map[string]any{"reason": "invalid source"})
	}
	if in.Context == skilldomain.ContextFork && strings.TrimSpace(in.Agent) == "" {
		return skilldomain.ErrForkRequiresAgent
	}
	return nil
}

// bodyHasLeadingFrontmatter reports whether body opens with a YAML frontmatter block (a "---" fence
// line followed by a later closing "---" fence) — as opposed to a lone "---" markdown thematic break,
// which has no closing fence and is fine.
//
// bodyHasLeadingFrontmatter 报告 body 是否以 YAML frontmatter 块开头（"---" 围栏行 + 之后的闭合 "---" 围栏）
// ——区别于孤立的 "---" markdown 分隔线（无闭合围栏、无妨）。
func bodyHasLeadingFrontmatter(body string) bool {
	b := strings.TrimLeft(body, " \t\r\n")
	nl := strings.IndexByte(b, '\n')
	if nl < 0 || strings.TrimRight(b[:nl], "\r") != "---" {
		return false
	}
	for _, line := range strings.Split(b[nl+1:], "\n") {
		if strings.TrimRight(line, "\r") == "---" {
			return true // found the closing fence → it's a frontmatter block
		}
	}
	return false
}
