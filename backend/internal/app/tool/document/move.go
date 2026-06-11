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

const moveDocumentDescription = `Reparent a document; parentId=null moves to root. position is the sibling index (0=first), omit to append. Path cascades to descendants. Cycles and self-parenting are rejected.`

var moveDocumentSchema = json.RawMessage(`{
	"type": "object",
	"required": ["id"],
	"properties": {
		"id":       {"type": "string"},
		"parentId": {"type": ["string", "null"], "description": "New parent ID; null = root."},
		"position": {"type": "integer", "minimum": 0, "description": "Sibling index (0=first); omit to append."}
	}
}`)

// MoveDocument implements the move_document system tool.
//
// MoveDocument 是 move_document 系统工具的实现。
type MoveDocument struct{ svc *documentapp.Service }

func (t *MoveDocument) Name() string                { return "move_document" }
func (t *MoveDocument) Description() string         { return moveDocumentDescription }
func (t *MoveDocument) Parameters() json.RawMessage { return moveDocumentSchema }

func (t *MoveDocument) ValidateInput(args json.RawMessage) error {
	var a struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("move_document: bad args: %w", err)
	}
	if strings.TrimSpace(a.ID) == "" {
		return ErrIDRequired
	}
	return nil
}

func (t *MoveDocument) Execute(ctx context.Context, argsJSON string) (string, error) {
	// Raw map distinguishes "parentId absent" from "parentId null" — both are legitimate
	// intents (absent = caller didn't mean to move; null = move to root).
	//
	// raw map 区分 "parentId 缺失" vs "parentId null"——皆合法（缺失=无意移动；null=移到根）。
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(argsJSON), &raw); err != nil {
		return "", fmt.Errorf("move_document: %w", err)
	}
	var a struct {
		ID       string  `json:"id"`
		ParentID *string `json:"parentId"`
		Position *int    `json:"position,omitempty"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
		return "", fmt.Errorf("move_document: %w", err)
	}
	if a.ParentID != nil && *a.ParentID == "" {
		a.ParentID = nil
	}
	if _, parentProvided := raw["parentId"]; !parentProvided {
		return "move_document: parentId required (pass null to move to root, or a doc ID to reparent).", nil
	}
	d, err := t.svc.Move(ctx, a.ID, documentapp.MoveInput{ParentID: a.ParentID, Position: a.Position})
	if err != nil {
		switch {
		case errors.Is(err, documentdomain.ErrNotFound):
			return fmt.Sprintf("Document %q not found.", a.ID), nil
		case errors.Is(err, documentdomain.ErrParentNotFound):
			return "New parent not found.", nil
		case errors.Is(err, documentdomain.ErrInvalidParent):
			return "Cannot move a document under itself or one of its own descendants (cycle).", nil
		default:
			return "", err
		}
	}
	return fmt.Sprintf("Moved %q to %s (new path: %s).", d.Name, parentLabel(d.ParentID), d.Path), nil
}

func parentLabel(parentID *string) string {
	if parentID == nil {
		return "root"
	}
	return *parentID
}
