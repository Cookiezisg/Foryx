// create.go — create_function system tool: applies a sequence of ops to
// create a new Function with an auto-accepted v1. Streams one progress delta
// per op via a progress block wrapped around the Service call.
//
// create.go —— create_function 系统工具:对空状态应用 ops 建新 Function 含
// 自动 accept 的 v1。开 progress block 包住 Service 调用,每 op emit 一行。

package function

import (
	"context"
	"encoding/json"
	"fmt"

	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
)

type CreateFunction struct {
	svc *functionapp.Service
}

func (t *CreateFunction) Name() string { return "create_function" }

func (t *CreateFunction) Description() string {
	return "Create a new function by applying a sequence of ops. The ops must include " +
		"set_meta (name + description), set_code (Python source), set_parameters (input " +
		"schema), and optionally set_return_schema / set_dependencies / set_python_version. " +
		"On success the function's v1 is auto-accepted and env-sync starts in background."
}

func (t *CreateFunction) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"ops": {
				"type": "array",
				"description": "Sequence of ops. Each op has 'op' discriminator + op-specific fields.",
				"items": {"type": "object"}
			},
			"changeReason": {"type": "string", "description": "One-line reason for this creation"}
		},
		"required": ["ops"]
	}`)
}

func (t *CreateFunction) IsReadOnly() bool        { return false }
func (t *CreateFunction) NeedsReadFirst() bool    { return false }
func (t *CreateFunction) RequiresWorkspace() bool { return false }

func (t *CreateFunction) ValidateInput(json.RawMessage) error { return nil }
func (t *CreateFunction) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *CreateFunction) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Ops          json.RawMessage `json:"ops"`
		ChangeReason string          `json:"changeReason"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("create_function: bad args: %w", err)
	}
	ops, err := functionapp.ParseOps(args.Ops)
	if err != nil {
		return "", fmt.Errorf("create_function: %w", err)
	}

	em := eventlogpkg.From(ctx)
	progID := em.StartBlock(ctx, eventlogdomain.BlockTypeProgress, map[string]any{
		"stage": "applying ops",
		"count": len(ops),
	})
	defer em.StopBlock(ctx, progID, eventlogdomain.StatusCompleted, nil)

	f, v, err := t.svc.Create(ctx, functionapp.CreateInput{
		Ops:             ops,
		ChangeReason:    args.ChangeReason,
		ProgressBlockID: progID,
	})
	if err != nil {
		em.StopBlock(ctx, progID, eventlogdomain.StatusError, err)
		return "", fmt.Errorf("create_function: %w", err)
	}

	// Env sync is synchronous inside Service.Create (D-redo-9). v.EnvStatus
	// is already terminal here. C2 will wrap this with the env-fix loop;
	// for C1 the tool just surfaces the final env state to the LLM.
	out := map[string]any{
		"id":         f.ID,
		"versionId":  v.ID,
		"version":    v.Version,
		"status":     v.Status,
		"envStatus":  v.EnvStatus,
		"envError":   v.EnvError,
		"opsApplied": len(ops),
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}
