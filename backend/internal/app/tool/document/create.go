package document

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	documentapp "github.com/sunweilin/forgify/backend/internal/app/document"
	documentdomain "github.com/sunweilin/forgify/backend/internal/domain/document"
)

const createDocumentDescription = `Create a document in the user's library. parentId nests it under another doc (Notion-style); null/omit = root. content is the full markdown body (split into child docs if >1MB). Name must be unique among siblings (auto-suffixed on collision).`

var createDocumentSchema = json.RawMessage(`{
	"type": "object",
	"required": ["name"],
	"properties": {
		"name":        {"type": "string", "description": "Document title; no slashes, up to 256 chars."},
		"parentId":    {"type": ["string", "null"], "description": "Parent doc ID; null/omit = root."},
		"description": {"type": "string", "description": "One-line catalog summary."},
		"content":     {"type": "string", "description": "Full markdown body."},
		"tags":        {"type": "array", "items": {"type": "string"}}
	}
}`)

// CreateDocument implements the create_document system tool.
//
// CreateDocument 是 create_document 系统工具的实现。
type CreateDocument struct{ svc *documentapp.Service }

func (t *CreateDocument) Name() string                { return "create_document" }
func (t *CreateDocument) Description() string         { return createDocumentDescription }
func (t *CreateDocument) Parameters() json.RawMessage { return createDocumentSchema }

func (t *CreateDocument) ValidateInput(args json.RawMessage) error {
	var a struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("create_document: bad args: %w", err)
	}
	if strings.TrimSpace(a.Name) == "" {
		return ErrNameRequired
	}
	return nil
}

func (t *CreateDocument) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a struct {
		Name        string   `json:"name"`
		ParentID    *string  `json:"parentId"`
		Description string   `json:"description"`
		Content     string   `json:"content"`
		Tags        []string `json:"tags"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("create_document: %w", err)
	}
	// Empty-string parentId is treated as null (root-level create).
	//
	// 空字符串 parentId 视为 null（根级创建）。
	if a.ParentID != nil && *a.ParentID == "" {
		a.ParentID = nil
	}
	d, err := t.svc.Create(ctx, documentapp.CreateInput{
		Name:        a.Name,
		ParentID:    a.ParentID,
		Description: a.Description,
		Content:     a.Content,
		Tags:        a.Tags,
	})
	if err != nil {
		switch {
		case errors.Is(err, documentdomain.ErrParentNotFound):
			return "Parent doc not found. Confirm it with list_documents or search_documents.", nil
		case errors.Is(err, documentdomain.ErrContentTooLarge):
			return "Content exceeds 1 MB. Split into smaller child docs.", nil
		case errors.Is(err, documentdomain.ErrInvalidName):
			return fmt.Sprintf("Invalid name %q (no slashes; non-empty; up to 256 chars).", a.Name), nil
		default:
			return "", err
		}
	}
	// Service auto-suffixes on name collision ("X" → "X 2"); tell the LLM when that
	// happened so it reasons about the real name.
	//
	// Service 重名自动加后缀（"X" → "X 2"）；命中时告知 LLM 真实名字。
	if a.Name != "" && d.Name != a.Name {
		return fmt.Sprintf("Created document %q (id=%s, path=%s). Note: requested name %q was taken; auto-renamed.", d.Name, d.ID, d.Path, a.Name), nil
	}
	return fmt.Sprintf("Created document %q (id=%s, path=%s).", d.Name, d.ID, d.Path), nil
}
