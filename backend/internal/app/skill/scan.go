package skill

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
)

// Scan walks skillsDir, parses each SKILL.md, and rebuilds the cache.
//
// Scan 遍历 skillsDir 解析每个 SKILL.md，并整体替换内存缓存。
func (s *Service) Scan(ctx context.Context) error {
	if s.skillsDir == "" {
		return fmt.Errorf("skillapp.Scan: skillsDir is empty")
	}

	loaded := map[string]*skilldomain.Skill{}

	if _, err := os.Stat(s.skillsDir); !errors.Is(err, fs.ErrNotExist) {
		entries, err := os.ReadDir(s.skillsDir)
		if err != nil {
			return fmt.Errorf("skillapp.Scan: read skillsDir: %w", err)
		}
		for _, ent := range entries {
			if !ent.IsDir() {
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
				s.log.Warn("skill name collision; later one ignored",
					zap.String("name", sk.Name),
					zap.String("kept", existing.DirPath),
					zap.String("rejected", dir))
				continue
			}
			loaded[sk.Name] = sk
		}
	}

	fp := skillsFingerprint(loaded)
	s.mu.Lock()
	s.skills = loaded
	s.mu.Unlock()

	last, _ := s.lastFP.Load().(string)
	s.lastFP.Store(fp)
	if last != fp {
		s.notif.Publish(ctx, "skill", "*",
			map[string]any{"changed": true, "count": len(loaded)}, "")
	}
	return nil
}

// skillsFingerprint hashes (name + frontmatter YAML) in sorted name order.
//
// skillsFingerprint 按 name 排序后哈希 (name + frontmatter YAML)。
func skillsFingerprint(skills map[string]*skilldomain.Skill) string {
	names := make([]string, 0, len(skills))
	for n := range skills {
		names = append(names, n)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, n := range names {
		fmtBytes, err := yaml.Marshal(&skills[n].Frontmatter)
		if err != nil {
			h.Write([]byte(n))
			h.Write([]byte{0})
			continue
		}
		h.Write([]byte(n))
		h.Write([]byte{0})
		h.Write(fmtBytes)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// parseSkillDir reads SKILL.md, parses + validates frontmatter, returns Skill (no body).
//
// parseSkillDir 读 SKILL.md、解析校验 frontmatter，返回 Skill（不缓存 body）。
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
		return nil, fmt.Errorf("%w: %w", skilldomain.ErrInvalidFrontmatter, err)
	}

	var fm skilldomain.Frontmatter
	if err := yaml.Unmarshal(yamlPart, &fm); err != nil {
		return nil, fmt.Errorf("%w: yaml: %w", skilldomain.ErrInvalidFrontmatter, err)
	}
	if err := validateFrontmatter(fm); err != nil {
		return nil, err
	}

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

// splitFrontmatter separates the leading YAML frontmatter from the markdown body.
//
// splitFrontmatter 将文件开头的 YAML frontmatter 与 markdown body 分离。
func splitFrontmatter(content []byte) (yamlPart, mdBody []byte, err error) {
	content = bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})

	normalized := bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))

	if !bytes.HasPrefix(normalized, []byte("---\n")) {
		return nil, nil, errors.New("missing opening --- fence")
	}
	rest := normalized[4:]
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
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

// validateFrontmatter enforces description non-empty + length cap + fork agent.
//
// validateFrontmatter 校验 description 非空、长度上限以及 fork 必须声明 agent。
func validateFrontmatter(fm skilldomain.Frontmatter) error {
	desc := strings.TrimSpace(fm.Description)
	if desc == "" {
		return fmt.Errorf("%w: description required", skilldomain.ErrInvalidFrontmatter)
	}
	if len(desc) > skilldomain.MaxDescriptionChars {
		return fmt.Errorf("%w: description %d chars exceeds %d",
			skilldomain.ErrInvalidFrontmatter, len(desc), skilldomain.MaxDescriptionChars)
	}
	if fm.Context == "fork" && strings.TrimSpace(fm.Agent) == "" {
		return fmt.Errorf("%w: context=fork requires agent: <type>",
			skilldomain.ErrInvalidFrontmatter)
	}
	return nil
}
