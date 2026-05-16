package skill

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
)

var nameRegexp = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// Body returns the raw SKILL.md bytes for one skill.
//
// Body 返回单个 skill 的 SKILL.md 原始字节。
func (s *Service) Body(_ context.Context, name string) ([]byte, error) {
	s.mu.RLock()
	sk, ok := s.skills[name]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("skillapp.Body: %w: %q", skilldomain.ErrSkillNotFound, name)
	}
	body, err := os.ReadFile(sk.BodyPath)
	if err != nil {
		return nil, fmt.Errorf("skillapp.Body %s: %w", name, err)
	}
	return body, nil
}

// Create writes a brand-new SKILL.md; conflict returns ErrNameConflict.
//
// Create 创建新的 SKILL.md；同名已存在返 ErrNameConflict。
func (s *Service) Create(ctx context.Context, name string, fm skilldomain.Frontmatter, body string) (*skilldomain.Skill, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	if err := validateFrontmatter(fm); err != nil {
		return nil, err
	}
	if err := validateBodySize(body); err != nil {
		return nil, err
	}

	dir := filepath.Join(s.skillsDir, name)
	if _, err := os.Stat(dir); err == nil {
		return nil, fmt.Errorf("skillapp.Create: %w: %q", skilldomain.ErrNameConflict, name)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("skillapp.Create: stat: %w", err)
	}

	if err := writeSkillDir(dir, fm, body); err != nil {
		return nil, fmt.Errorf("skillapp.Create %s: %w", name, err)
	}
	if err := s.Scan(ctx); err != nil {
		return nil, fmt.Errorf("skillapp.Create %s: rescan: %w", name, err)
	}
	return s.Get(ctx, name)
}

// Replace overwrites an existing SKILL.md; missing name returns ErrSkillNotFound.
//
// Replace 覆盖已存在 SKILL.md；name 缺失返 ErrSkillNotFound。
func (s *Service) Replace(ctx context.Context, name string, fm skilldomain.Frontmatter, body string) (*skilldomain.Skill, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	if err := validateFrontmatter(fm); err != nil {
		return nil, err
	}
	if err := validateBodySize(body); err != nil {
		return nil, err
	}

	dir := filepath.Join(s.skillsDir, name)
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("skillapp.Replace: %w: %q", skilldomain.ErrSkillNotFound, name)
	} else if err != nil {
		return nil, fmt.Errorf("skillapp.Replace: stat: %w", err)
	}

	if err := writeSkillDir(dir, fm, body); err != nil {
		return nil, fmt.Errorf("skillapp.Replace %s: %w", name, err)
	}
	if err := s.Scan(ctx); err != nil {
		return nil, fmt.Errorf("skillapp.Replace %s: rescan: %w", name, err)
	}
	return s.Get(ctx, name)
}

// Delete removes the entire skill directory; missing name returns ErrSkillNotFound.
//
// Delete 删除整个 skill 目录；name 缺失返 ErrSkillNotFound。
func (s *Service) Delete(ctx context.Context, name string) error {
	if err := validateName(name); err != nil {
		return err
	}
	s.mu.RLock()
	_, ok := s.skills[name]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("skillapp.Delete: %w: %q", skilldomain.ErrSkillNotFound, name)
	}
	dir := filepath.Join(s.skillsDir, name)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("skillapp.Delete %s: %w", name, err)
	}
	if err := s.Scan(ctx); err != nil {
		return fmt.Errorf("skillapp.Delete %s: rescan: %w", name, err)
	}
	return nil
}

func validateName(name string) error {
	if !nameRegexp.MatchString(name) {
		return fmt.Errorf("skillapp.validateName: %w: %q (must match %s)",
			skilldomain.ErrInvalidName, name, nameRegexp.String())
	}
	return nil
}

func validateBodySize(body string) error {
	if len(body) > skilldomain.MaxBodyBytes {
		return fmt.Errorf("skillapp.validateBodySize: %w: body %d bytes (cap %d)",
			skilldomain.ErrBodyTooLarge, len(body), skilldomain.MaxBodyBytes)
	}
	return nil
}

// writeSkillDir atomically writes SKILL.md via .tmp + rename.
//
// writeSkillDir 用 .tmp + rename 原子写 SKILL.md。
func writeSkillDir(dir string, fm skilldomain.Frontmatter, body string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	yamlBytes, err := yaml.Marshal(&fm)
	if err != nil {
		return fmt.Errorf("marshal frontmatter: %w", err)
	}
	content := "---\n" + string(yamlBytes) + "---\n" + body
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	target := filepath.Join(dir, "SKILL.md")
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
