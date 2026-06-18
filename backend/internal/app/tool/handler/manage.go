package handler

import (
	"context"
	"encoding/json"
	"fmt"

	handlerapp "github.com/sunweilin/anselm/backend/internal/app/handler"
	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
)

// --- revert_handler --------------------------------------------------------

type RevertHandler struct{ svc *handlerapp.Service }

func (t *RevertHandler) Name() string { return "revert_handler" }

func (t *RevertHandler) Description() string {
	return "Switch a handler's active version to an existing version by number, then restart the resident instance to run it. Only moves the active pointer — newer versions stay in history. Note: name, description and tags are NOT versioned (they live on the handler), so a revert restores only the versioned snapshot (methods/code) and leaves name/description/tags unchanged — use edit_handler set_meta to also change those."
}

func (t *RevertHandler) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["handlerId", "version"],
		"properties": {
			"handlerId": {"type": "string"},
			"version": {"type": "integer", "description": "The version number to make active."}
		}
	}`)
}

func (t *RevertHandler) ValidateInput(args json.RawMessage) error {
	var a struct {
		HandlerID string `json:"handlerId"`
		Version   int    `json:"version"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("revert_handler: bad args: %w", err)
	}
	if a.HandlerID == "" {
		return ErrHandlerIDRequired
	}
	if a.Version <= 0 {
		return ErrVersionPositive
	}
	return nil
}

func (t *RevertHandler) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		HandlerID string `json:"handlerId"`
		Version   int    `json:"version"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("revert_handler: bad args: %w", err)
	}
	v, err := t.svc.Revert(ctx, args.HandlerID, args.Version)
	if err != nil {
		return "", fmt.Errorf("revert_handler: %w", err)
	}
	return toolapp.ToJSON(map[string]any{"id": args.HandlerID, "activeVersionId": v.ID, "version": v.Version}), nil
}

// --- delete_handler --------------------------------------------------------

type DeleteHandler struct{ svc *handlerapp.Service }

func (t *DeleteHandler) Name() string { return "delete_handler" }

func (t *DeleteHandler) Description() string {
	return "Delete a handler: stop its resident instance and remove all versions + environments. Not reversible."
}

func (t *DeleteHandler) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["handlerId"],
		"properties": {"handlerId": {"type": "string"}}
	}`)
}

func (t *DeleteHandler) ValidateInput(args json.RawMessage) error {
	var a struct {
		HandlerID string `json:"handlerId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("delete_handler: bad args: %w", err)
	}
	if a.HandlerID == "" {
		return ErrHandlerIDRequired
	}
	return nil
}

func (t *DeleteHandler) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		HandlerID string `json:"handlerId"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("delete_handler: bad args: %w", err)
	}
	if err := t.svc.Delete(ctx, args.HandlerID); err != nil {
		return "", fmt.Errorf("delete_handler: %w", err)
	}
	return toolapp.ToJSON(map[string]any{"id": args.HandlerID, "deleted": true}), nil
}

// --- restart_handler -------------------------------------------------------

type RestartHandler struct{ svc *handlerapp.Service }

func (t *RestartHandler) Name() string { return "restart_handler" }

func (t *RestartHandler) Description() string {
	return "Restart a handler's resident process: gracefully shut down the running instance (runs shutdown()) and start a fresh one with the latest config + code. Use when a handler is misbehaving — a stale DB connection, an expired session, a wedged state. Returns the new runtime state."
}

func (t *RestartHandler) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["handlerId"],
		"properties": {"handlerId": {"type": "string"}}
	}`)
}

func (t *RestartHandler) ValidateInput(args json.RawMessage) error {
	var a struct {
		HandlerID string `json:"handlerId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("restart_handler: bad args: %w", err)
	}
	if a.HandlerID == "" {
		return ErrHandlerIDRequired
	}
	return nil
}

func (t *RestartHandler) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		HandlerID string `json:"handlerId"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("restart_handler: bad args: %w", err)
	}
	state, err := t.svc.Restart(ctx, args.HandlerID)
	if err != nil {
		// Restart returns the (failed) state alongside the error — surface both.
		return toolapp.ToJSON(map[string]any{"id": args.HandlerID, "runtimeState": state, "error": err.Error()}), nil
	}
	return toolapp.ToJSON(map[string]any{"id": args.HandlerID, "runtimeState": state}), nil
}

// --- update_handler_config -------------------------------------------------

type UpdateHandlerConfig struct{ svc *handlerapp.Service }

func (t *UpdateHandlerConfig) Name() string { return "update_handler_config" }

func (t *UpdateHandlerConfig) Description() string {
	return "Set a handler's init-args config (the values passed to __init__), then restart the instance to apply them. Pass a partial object (JSON Merge Patch); null deletes a key. Note: secret values (api keys, db strings) are normally filled by the user, not here — only set values you actually have."
}

func (t *UpdateHandlerConfig) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["handlerId", "config"],
		"properties": {
			"handlerId": {"type": "string"},
			"config": {"type": "object", "description": "Partial init-args config (merge patch); null deletes a key."}
		}
	}`)
}

func (t *UpdateHandlerConfig) ValidateInput(args json.RawMessage) error {
	var a struct {
		HandlerID string `json:"handlerId"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("update_handler_config: bad args: %w", err)
	}
	if a.HandlerID == "" {
		return ErrHandlerIDRequired
	}
	return nil
}

func (t *UpdateHandlerConfig) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		HandlerID string         `json:"handlerId"`
		Config    map[string]any `json:"config"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("update_handler_config: bad args: %w", err)
	}
	if err := t.svc.UpdateConfig(ctx, args.HandlerID, args.Config); err != nil {
		return "", fmt.Errorf("update_handler_config: %w", err)
	}
	return toolapp.ToJSON(map[string]any{"id": args.HandlerID, "configUpdated": true}), nil
}
