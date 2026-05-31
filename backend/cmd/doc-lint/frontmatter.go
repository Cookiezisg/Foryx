package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

// Frontmatter holds the parsed YAML front-matter fields from a markdown file.
type Frontmatter struct {
	ID           string
	Type         string
	Status       string
	Owner        string
	Created      string
	Reviewed     string
	ReviewDue    string
	LandedInto   string
	SupersededBy string
}

var requiredFields = []string{"id", "type", "status", "owner", "created", "reviewed", "review-due"}

var validTypes = map[string]bool{
	"concept": true, "reference": true, "how-to": true,
	"decision": true, "log": true, "working": true,
}

var validStatuses = map[string]bool{
	"draft": true, "active": true, "superseded": true,
	"deprecated": true, "archived": true,
}

// validADRStatuses are the accepted status values for ADR decision files.
var validADRStatuses = map[string]bool{
	"accepted": true, "rejected": true, "deprecated": true,
	"superseded": true, "draft": true,
}

// parseFrontmatter reads YAML front-matter from a markdown file.
// Returns nil if the file has no front-matter block.
func parseFrontmatter(path string) (*Frontmatter, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return nil, nil
	}
	if strings.TrimSpace(scanner.Text()) != "---" {
		return nil, nil
	}

	fields := map[string]string{}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		fields[key] = val
	}

	return &Frontmatter{
		ID:           fields["id"],
		Type:         fields["type"],
		Status:       fields["status"],
		Owner:        fields["owner"],
		Created:      fields["created"],
		Reviewed:     fields["reviewed"],
		ReviewDue:    fields["review-due"],
		LandedInto:   fields["landed-into"],
		SupersededBy: fields["superseded-by"],
	}, nil
}

// isADRFrontmatter returns true when the parsed fields look like an ADR
// (id starts with "ADR-"), indicating the file uses ADR schema rather than
// the standard doc governance schema.
func isADRFrontmatter(fm *Frontmatter) bool {
	return strings.HasPrefix(fm.ID, "ADR-")
}

// validateFrontmatter returns a list of issue strings for the given file.
// Issues prefixed with "WARN:" are non-fatal; others are errors.
// Files with no frontmatter at all are flagged as WARN (grandfathered pre-governance docs).
func validateFrontmatter(path string, fm *Frontmatter) []string {
	var issues []string

	if fm == nil {
		// Pre-existing docs without frontmatter: warn only, do not fail build.
		return []string{"WARN: missing frontmatter block (grandfathered; add frontmatter when editing)"}
	}

	// ADR files use a different schema — validate only status.
	if isADRFrontmatter(fm) {
		if fm.Status != "" && !validADRStatuses[fm.Status] {
			issues = append(issues, fmt.Sprintf("invalid ADR status %q (valid: accepted, rejected, deprecated, superseded, draft)", fm.Status))
		}
		return issues
	}

	fieldMap := map[string]string{
		"id":         fm.ID,
		"type":       fm.Type,
		"status":     fm.Status,
		"owner":      fm.Owner,
		"created":    fm.Created,
		"reviewed":   fm.Reviewed,
		"review-due": fm.ReviewDue,
	}
	for _, field := range requiredFields {
		if fieldMap[field] == "" {
			issues = append(issues, fmt.Sprintf("missing required field: %s", field))
		}
	}

	if fm.Type != "" && !validTypes[fm.Type] {
		issues = append(issues, fmt.Sprintf("invalid type %q (valid: concept, reference, how-to, decision, log, working)", fm.Type))
	}

	if fm.Status != "" && !validStatuses[fm.Status] {
		issues = append(issues, fmt.Sprintf("invalid status %q (valid: draft, active, superseded, deprecated, archived)", fm.Status))
	}

	if fm.ReviewDue != "" && fm.ReviewDue != "never" {
		due, err := time.Parse("2006-01-02", fm.ReviewDue)
		if err == nil && time.Now().After(due) {
			issues = append(issues, fmt.Sprintf("WARN: review-due %s is in the past", fm.ReviewDue))
		}
	}

	return issues
}
