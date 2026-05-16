package skill

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
)

// ImportFile is one dropped skill carrying name + raw SKILL.md bytes.
//
// ImportFile 是一份拖入的 skill，含 name 与 SKILL.md 原始字节。
type ImportFile struct {
	Name       string
	RawSkillMD []byte
}

// ImportError pairs a file's name with the reason it couldn't be imported.
//
// ImportError 将文件名与无法 import 的原因配对。
type ImportError struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// ImportResult buckets results into Imported / Conflicts / Errors.
//
// ImportResult 将结果分桶为 Imported / Conflicts / Errors。
type ImportResult struct {
	Imported  []string      `json:"imported"`
	Conflicts []string      `json:"conflicts"`
	Errors    []ImportError `json:"errors"`
}

// Import processes a batch of SKILL.md files and rescans once at the end.
//
// Import 批量处理 SKILL.md 文件，结束时统一 Scan 一次。
func (s *Service) Import(ctx context.Context, files []ImportFile, overwrite bool) (ImportResult, error) {
	res := ImportResult{
		Imported:  []string{},
		Conflicts: []string{},
		Errors:    []ImportError{},
	}

	for _, f := range files {
		if err := validateName(f.Name); err != nil {
			res.Errors = append(res.Errors, ImportError{
				Name: f.Name, Reason: err.Error(),
			})
			continue
		}
		if len(f.RawSkillMD) > skilldomain.MaxBodyBytes {
			res.Errors = append(res.Errors, ImportError{
				Name:   f.Name,
				Reason: fmt.Sprintf("body %d bytes exceeds %d cap", len(f.RawSkillMD), skilldomain.MaxBodyBytes),
			})
			continue
		}
		yamlPart, body, err := splitFrontmatter(f.RawSkillMD)
		if err != nil {
			res.Errors = append(res.Errors, ImportError{
				Name: f.Name, Reason: "split frontmatter: " + err.Error(),
			})
			continue
		}
		var fm skilldomain.Frontmatter
		if err := yaml.Unmarshal(yamlPart, &fm); err != nil {
			res.Errors = append(res.Errors, ImportError{
				Name: f.Name, Reason: "yaml parse: " + err.Error(),
			})
			continue
		}
		if err := validateFrontmatter(fm); err != nil {
			res.Errors = append(res.Errors, ImportError{
				Name: f.Name, Reason: err.Error(),
			})
			continue
		}

		dir := filepath.Join(s.skillsDir, f.Name)
		exists := false
		if _, err := os.Stat(dir); err == nil {
			exists = true
		} else if !errors.Is(err, fs.ErrNotExist) {
			res.Errors = append(res.Errors, ImportError{
				Name: f.Name, Reason: "stat: " + err.Error(),
			})
			continue
		}
		if exists && !overwrite {
			res.Conflicts = append(res.Conflicts, f.Name)
			continue
		}

		if err := writeSkillDir(dir, fm, string(body)); err != nil {
			res.Errors = append(res.Errors, ImportError{
				Name: f.Name, Reason: "write: " + err.Error(),
			})
			continue
		}
		res.Imported = append(res.Imported, f.Name)
	}

	if len(res.Imported) > 0 {
		if err := s.Scan(ctx); err != nil {
			return res, fmt.Errorf("skillapp.Import: post-batch rescan: %w", err)
		}
	}
	return res, nil
}

