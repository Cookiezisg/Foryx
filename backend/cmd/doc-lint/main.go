// Command doc-lint validates documentation frontmatter and lifecycle rules.
//
// Checks performed:
//   - All non-archive .md files have valid frontmatter
//   - All required frontmatter fields are present
//   - review-due dates (warns on past dates, does not fail build)
//   - working/ docs older than 90 days without landed-into (fails)
//   - decisions/ files modified after creation (warns via git log)
//   - INDEX.md is ≤ 50 lines
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	root := flag.String("root", ".", "repository root directory")
	flag.Parse()

	docsDir := filepath.Join(*root, "documents")
	exitCode := 0
	warnings := 0
	checked := 0

	err := filepath.WalkDir(docsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "archive" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}

		// Skip the governance meta-docs themselves from strict checks
		base := filepath.Base(path)
		if base == "GOVERNANCE.md" || base == "INDEX.md" {
			return nil
		}
		// Skip decisions README and template (no frontmatter required)
		rel, _ := filepath.Rel(*root, path)
		if strings.HasPrefix(rel, "documents/decisions/README") ||
			strings.HasPrefix(rel, "documents/decisions/template") {
			return nil
		}

		checked++
		fm, parseErr := parseFrontmatter(path)
		if parseErr != nil {
			fmt.Printf("ERROR %s: cannot read file: %v\n", rel, parseErr)
			exitCode = 1
			return nil
		}

		issues := validateFrontmatter(path, fm)
		for _, issue := range issues {
			if strings.HasPrefix(issue, "WARN:") {
				fmt.Printf("WARN  %s: %s\n", rel, strings.TrimPrefix(issue, "WARN: "))
				warnings++
			} else {
				fmt.Printf("ERROR %s: %s\n", rel, issue)
				exitCode = 1
			}
		}

		// Check working/ lifecycle: active + no landed-into + older than 90 days
		if fm != nil && fm.Type == "working" && fm.Status == "active" && fm.LandedInto == "" {
			if fm.Created != "" {
				created, err := time.Parse("2006-01-02", fm.Created)
				if err == nil && time.Since(created) > 90*24*time.Hour {
					fmt.Printf("ERROR %s: working doc older than 90 days with no landed-into\n", rel)
					exitCode = 1
				}
			}
		}

		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "walk error: %v\n", err)
		os.Exit(1)
	}

	// Check INDEX.md line count
	indexPath := filepath.Join(docsDir, "INDEX.md")
	if data, err := os.ReadFile(indexPath); err == nil {
		lines := strings.Count(string(data), "\n")
		if lines > 50 {
			fmt.Printf("ERROR documents/INDEX.md: %d lines (must be ≤ 50)\n", lines)
			exitCode = 1
		}
	}

	// Check decisions/ immutability: warn if a decision file has >1 commit
	decisionsDir := filepath.Join(docsDir, "decisions")
	if entries, err := os.ReadDir(decisionsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			name := e.Name()
			if name == "template.md" || name == "README.md" {
				continue
			}
			relPath := filepath.Join("documents", "decisions", name)
			out, err := exec.Command("git", "-C", *root, "log", "--oneline", "--", relPath).Output()
			if err == nil {
				commitLines := strings.Split(strings.TrimSpace(string(out)), "\n")
				count := 0
				for _, l := range commitLines {
					if l != "" {
						count++
					}
				}
				if count > 1 {
					fmt.Printf("WARN  documents/decisions/%s: %d commits (ADRs should be immutable after merge)\n", name, count)
					warnings++
				}
			}
		}
	}

	fmt.Printf("\ndoc-lint: checked %d files — ", checked)
	if exitCode == 0 {
		fmt.Printf("OK (%d warnings)\n", warnings)
	} else {
		fmt.Printf("FAILED (%d warnings)\n", warnings)
	}
	os.Exit(exitCode)
}
