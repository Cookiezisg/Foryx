package document

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	documentapp "github.com/sunweilin/forgify/backend/internal/app/document"
)

const searchDocumentsDescription = `Search documents by keyword over name / description / tags. Returns path + description per match so you can pick which to read. Prefer list_documents when you already know the folder.`

const searchDocumentsDefaultLimit = 10

var searchDocumentsSchema = json.RawMessage(`{
	"type": "object",
	"required": ["query"],
	"properties": {
		"query": {"type": "string"},
		"limit": {"type": "integer", "default": 10, "maximum": 50}
	}
}`)

// SearchDocuments implements the search_documents system tool.
//
// SearchDocuments 是 search_documents 系统工具的实现。
type SearchDocuments struct{ svc *documentapp.Service }

func (t *SearchDocuments) Name() string                { return "search_documents" }
func (t *SearchDocuments) Description() string         { return searchDocumentsDescription }
func (t *SearchDocuments) Parameters() json.RawMessage { return searchDocumentsSchema }

func (t *SearchDocuments) ValidateInput(args json.RawMessage) error {
	var a struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("search_documents: bad args: %w", err)
	}
	if strings.TrimSpace(a.Query) == "" {
		return ErrQueryRequired
	}
	if a.Limit < 0 || a.Limit > 50 {
		return fmt.Errorf("search_documents: limit must be 0..50, got %d", a.Limit)
	}
	return nil
}

func (t *SearchDocuments) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("search_documents: %w", err)
	}
	if a.Limit == 0 {
		a.Limit = searchDocumentsDefaultLimit
	}
	rows, err := t.svc.Search(ctx, a.Query, a.Limit)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return fmt.Sprintf("No documents matched %q. Try list_documents(parentId=null) to browse top-level docs or refine the query.", a.Query), nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d document(s) matching %q:\n\n", len(rows), a.Query)
	for _, d := range rows {
		fmt.Fprintf(&sb, "- %s (id=%s)\n", d.Path, d.ID)
		if d.Description != "" {
			fmt.Fprintf(&sb, "  %s\n", d.Description)
		}
	}
	sb.WriteString("\nUse read_document(id) to load full content.")
	return sb.String(), nil
}
