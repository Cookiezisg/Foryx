// Package skill is the domain layer for Anthropic Agent Skills (SKILL.md directories).
//
// Package skill 是 Anthropic Agent Skills（SKILL.md 目录）的 domain 层。
package skill

import (
	"errors"
	"time"
)

// Skill is the metadata cache for one ~/.forgify/skills/<name>/ entry; Body is re-read on Activate.
//
// Skill 是 ~/.forgify/skills/<name>/ 一条的元数据缓存；Body 每次 Activate 重读防 stale。
type Skill struct {
	Name        string      `json:"name"`
	Source      string      `json:"source"`
	DirPath     string      `json:"dirPath"`
	BodyPath    string      `json:"bodyPath"`
	Description string      `json:"description"`
	Frontmatter Frontmatter `json:"frontmatter"`
	LoadedAt    time.Time   `json:"loadedAt"`
}

// Frontmatter mirrors the Anthropic SKILL.md spec verbatim (cross-vendor fields preserved).
//
// Frontmatter 镜像 Anthropic SKILL.md spec，跨厂字段全保留以便无缝迁移。
type Frontmatter struct {
	Name                   string   `yaml:"name" json:"name"`
	Description            string   `yaml:"description" json:"description"`
	WhenToUse              string   `yaml:"when_to_use,omitempty" json:"whenToUse,omitempty"`
	AllowedTools           []string `yaml:"allowed-tools,omitempty" json:"allowedTools,omitempty"`
	DisableModelInvocation bool     `yaml:"disable-model-invocation,omitempty" json:"disableModelInvocation,omitempty"`
	UserInvocable          bool     `yaml:"user-invocable,omitempty" json:"userInvocable,omitempty"`
	Paths                  []string `yaml:"paths,omitempty" json:"paths,omitempty"`
	Context                string   `yaml:"context,omitempty" json:"context,omitempty"`
	Agent                  string   `yaml:"agent,omitempty" json:"agent,omitempty"`
	Arguments              []string `yaml:"arguments,omitempty" json:"arguments,omitempty"`
	ArgumentHint           string   `yaml:"argument-hint,omitempty" json:"argumentHint,omitempty"`
	Model                  string   `yaml:"model,omitempty" json:"model,omitempty"`
	Effort                 string   `yaml:"effort,omitempty" json:"effort,omitempty"`
}

var (
	ErrSkillNotFound      = errors.New("skill: not found")
	ErrInvalidFrontmatter = errors.New("skill: invalid frontmatter")
	ErrBodyTooLarge       = errors.New("skill: body exceeds size limit")
	ErrNameConflict       = errors.New("skill: name already exists")
	ErrInvalidName        = errors.New("skill: invalid name")
)

const MaxBodyBytes = 32 * 1024

const MaxDescriptionChars = 1536
