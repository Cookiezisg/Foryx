// Command doc-matrix generates a documentation freshness report.
//
// For each backend domain reference doc, it compares the `reviewed` date
// in the frontmatter against the last git commit date on the corresponding
// source code directory. Domains where code changed after the last review
// are marked STALE.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// domainCodePaths maps domain doc filename (without .md) to its backend source path.
var domainCodePaths = map[string]string{
	"apikey":       "backend/internal/domain/apikey",
	"ask":          "backend/internal/app/ask",
	"catalog":      "backend/internal/app/catalog",
	"chat":         "backend/internal/app/chat",
	"compaction":   "backend/internal/app/compaction",
	"conversation": "backend/internal/domain/conversation",
	"document":     "backend/internal/domain/document",
	"filesystem":   "backend/internal/app/tool/filesystem",
	"flowrun":      "backend/internal/domain/flowrun",
	"function":     "backend/internal/domain/function",
	"handler":      "backend/internal/domain/handler",
	"mcp":          "backend/internal/app/mcp",
	"memory":       "backend/internal/domain/memory",
	"mention":      "backend/internal/domain/mention",
	"model":        "backend/internal/domain/model",
	"permissions":  "backend/internal/app/permissions",
	"relation":     "backend/internal/domain/relation",
	"sandbox":      "backend/internal/infra/sandbox",
	"scheduler":    "backend/internal/app/scheduler",
	"search":       "backend/internal/app/search",
	"shell":        "backend/internal/app/tool/shell",
	"skill":        "backend/internal/domain/skill",
	"subagent":     "backend/internal/app/subagent",
	"todo":         "backend/internal/domain/todo",
	"trigger":      "backend/internal/domain/trigger",
	"user":         "backend/internal/domain/user",
	"web":          "backend/internal/app/tool/web",
	"workflow":     "backend/internal/domain/workflow",
}

type row struct {
	domain      string
	reviewed    string
	lastChanged string
	status      string
}

func main() {
	root := flag.String("root", ".", "repository root directory")
	flag.Parse()

	domainsDir := filepath.Join(*root, "documents", "references", "backend", "domains")
	entries, err := os.ReadDir(domainsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read domains dir: %v\n", err)
		os.Exit(1)
	}

	var rows []row
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		domain := strings.TrimSuffix(e.Name(), ".md")
		docPath := filepath.Join(domainsDir, e.Name())

		reviewed := extractReviewed(docPath)
		codePath, ok := domainCodePaths[domain]
		lastChanged := ""
		status := "UNKNOWN"

		if ok {
			lastChanged = lastGitChange(*root, codePath)
			if reviewed != "" && lastChanged != "" {
				rv, err1 := time.Parse("2006-01-02", reviewed)
				lc, err2 := time.Parse("2006-01-02", lastChanged)
				if err1 == nil && err2 == nil {
					if lc.After(rv) {
						status = "⚠️  STALE"
					} else {
						status = "✅ FRESH"
					}
				}
			}
		} else {
			status = "—  (no code path)"
		}

		rows = append(rows, row{domain, reviewed, lastChanged, status})
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].domain < rows[j].domain })

	fmt.Println("## Doc Freshness Matrix")
	fmt.Println()
	fmt.Printf("%-20s %-14s %-14s %s\n", "Domain", "Last Reviewed", "Code Changed", "Status")
	fmt.Printf("%-20s %-14s %-14s %s\n", strings.Repeat("-", 20), strings.Repeat("-", 14), strings.Repeat("-", 14), strings.Repeat("-", 12))
	for _, r := range rows {
		reviewed := r.reviewed
		if reviewed == "" {
			reviewed = "—"
		}
		changed := r.lastChanged
		if changed == "" {
			changed = "—"
		}
		fmt.Printf("%-20s %-14s %-14s %s\n", r.domain, reviewed, changed, r.status)
	}

	stale := 0
	for _, r := range rows {
		if strings.Contains(r.status, "STALE") {
			stale++
		}
	}
	fmt.Printf("\nTotal: %d domains | %d stale\n", len(rows), stale)
}

// extractReviewed reads the `reviewed:` field from a file's frontmatter.
func extractReviewed(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFM := false
	for scanner.Scan() {
		line := scanner.Text()
		if !inFM {
			if strings.TrimSpace(line) == "---" {
				inFM = true
			}
			continue
		}
		if strings.TrimSpace(line) == "---" {
			break
		}
		if strings.HasPrefix(line, "reviewed:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "reviewed:"))
		}
	}
	return ""
}

// lastGitChange returns the date (YYYY-MM-DD) of the most recent commit touching path.
func lastGitChange(root, path string) string {
	out, err := exec.Command("git", "-C", root, "log", "-1", "--format=%as", "--", path).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
