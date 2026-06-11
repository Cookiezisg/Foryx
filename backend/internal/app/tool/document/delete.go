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

const deleteDocumentDescription = `Soft-delete a document and all of its descendants recursively. Returns the deleted count. The user can still recover tombstoned docs; already-sent messages keep resolving.`

var deleteDocumentSchema = json.RawMessage(`{
	"type": "object",
	"required": ["id"],
	"properties": {
		"id": {"type": "string"}
	}
}`)

// DeleteDocument implements the delete_document system tool.
//
// DeleteDocument 是 delete_document 系统工具的实现。
type DeleteDocument struct{ svc *documentapp.Service }

func (t *DeleteDocument) Name() string                { return "delete_document" }
func (t *DeleteDocument) Description() string         { return deleteDocumentDescription }
func (t *DeleteDocument) Parameters() json.RawMessage { return deleteDocumentSchema }

func (t *DeleteDocument) ValidateInput(args json.RawMessage) error {
	var a struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("delete_document: bad args: %w", err)
	}
	if strings.TrimSpace(a.ID) == "" {
		return ErrIDRequired
	}
	return nil
}

func (t *DeleteDocument) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("delete_document: %w", err)
	}
	n, err := t.svc.Delete(ctx, a.ID)
	if err != nil {
		if errors.Is(err, documentdomain.ErrNotFound) {
			return fmt.Sprintf("Document %q not found (already deleted?).", a.ID), nil
		}
		return "", err
	}
	if n <= 1 {
		return fmt.Sprintf("Deleted document %s (no descendants).", a.ID), nil
	}
	return fmt.Sprintf("Deleted document %s along with %d descendant(s).", a.ID, n-1), nil
}
