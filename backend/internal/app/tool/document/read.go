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

const readDocumentDescription = `Load a document's full markdown body plus path, description, and tags. Use after picking a doc via search_documents / list_documents.`

var readDocumentSchema = json.RawMessage(`{
	"type": "object",
	"required": ["id"],
	"properties": {
		"id": {"type": "string"}
	}
}`)

// ReadDocument implements the read_document system tool.
//
// ReadDocument 是 read_document 系统工具的实现。
type ReadDocument struct{ svc *documentapp.Service }

func (t *ReadDocument) Name() string                { return "read_document" }
func (t *ReadDocument) Description() string         { return readDocumentDescription }
func (t *ReadDocument) Parameters() json.RawMessage { return readDocumentSchema }

func (t *ReadDocument) ValidateInput(args json.RawMessage) error {
	var a struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("read_document: bad args: %w", err)
	}
	if strings.TrimSpace(a.ID) == "" {
		return ErrIDRequired
	}
	return nil
}

func (t *ReadDocument) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("read_document: %w", err)
	}
	d, err := t.svc.Get(ctx, a.ID)
	if err != nil {
		if errors.Is(err, documentdomain.ErrNotFound) {
			return fmt.Sprintf("Document %q not found. Try search_documents(query=...) or list_documents(parentId=null) to find available docs.", a.ID), nil
		}
		return "", err
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\nPath: %s\nID: %s\n", d.Name, d.Path, d.ID)
	if d.Description != "" {
		fmt.Fprintf(&sb, "Description: %s\n", d.Description)
	}
	if len(d.Tags) > 0 {
		fmt.Fprintf(&sb, "Tags: %s\n", strings.Join(d.Tags, ", "))
	}
	sb.WriteString("\n---\n\n")
	sb.WriteString(d.Content)
	return sb.String(), nil
}
