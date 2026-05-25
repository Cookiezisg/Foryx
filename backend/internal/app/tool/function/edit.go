package function

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	envfixpkg "github.com/sunweilin/forgify/backend/internal/pkg/envfix"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	forgepkg "github.com/sunweilin/forgify/backend/internal/pkg/forge"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

type EditFunction struct {
	svc     *functionapp.Service
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
	forge   forgepkg.Publisher
}

func (t *EditFunction) Name() string { return "edit_function" }

func (t *EditFunction) Description() string {
	return "Edit an existing function by applying a sequence of ops. Creates (or iterates) " +
		"a pending version. Pass ops=[] to force-rebuild the active version's env (D-redo-22). " +
		"If the venv install fails, an internal env-fix loop retries up to 3 times by asking " +
		"the LLM to revise the dependency list. The tool returns the pending version's " +
		"terminal state — the user reviews and accepts/rejects."
}

func (t *EditFunction) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string", "description": "Function ID to edit"},
			"ops": {
				"type": "array",
				"description": "Sequence of ops to apply (see create_function). Empty array forces env rebuild.",
				"items": {"type": "object"}
			},
			"changeReason": {"type": "string", "description": "One-line reason for this edit"}
		},
		"required": ["id", "ops"]
	}`)
}

func (t *EditFunction) IsReadOnly() bool        { return false }
func (t *EditFunction) NeedsReadFirst() bool    { return false }
func (t *EditFunction) RequiresWorkspace() bool { return false }

func (t *EditFunction) ValidateInput(json.RawMessage) error { return nil }
func (t *EditFunction) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *EditFunction) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ID           string          `json:"id"`
		Ops          json.RawMessage `json:"ops"`
		ChangeReason string          `json:"changeReason"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("edit_function: bad args: %w", err)
	}
	if args.ID == "" {
		return "", fmt.Errorf("edit_function: id required")
	}
	ops, err := functionapp.ParseOps(args.Ops)
	if err != nil {
		return "", fmt.Errorf("edit_function: %w", err)
	}

	em := eventlogpkg.From(ctx)
	progID := em.StartBlock(ctx, eventlogdomain.BlockTypeProgress, map[string]any{
		"stage":      "applying ops",
		"count":      len(ops),
		"functionId": args.ID,
	})
	defer em.StopBlock(ctx, progID, eventlogdomain.StatusCompleted, nil)

	// Publish forge_started on the forge bus (C4 D-redo-4). The chat
	// eventlog progress block already provides per-op detail.
	// 在 forge bus 发 forge_started(C4 D-redo-4)。
	scope := eventlogdomain.Scope{Kind: eventlogdomain.KindFunction, ID: args.ID}
	convID, _ := reqctxpkg.GetConversationID(ctx)
	toolCallID, _ := reqctxpkg.GetToolCallID(ctx)
	t.forge.PublishStarted(ctx, scope, forgedomain.OperationEdit, convID, toolCallID)

	v, err := t.svc.Edit(ctx, functionapp.EditInput{
		ID:              args.ID,
		Ops:             ops,
		ChangeReason:    args.ChangeReason,
		ProgressBlockID: progID,
	})
	if err != nil {
		em.StopBlock(ctx, progID, eventlogdomain.StatusError, err)
		t.forge.PublishCompleted(ctx, scope, forgedomain.CompletedFailed, "", "", 0, err)
		return "", fmt.Errorf("edit_function: %w", err)
	}

	if v.EnvStatus == functiondomain.EnvStatusReady {
		t.forge.PublishEnvAttempt(ctx, scope, 1, forgedomain.EnvAttemptOK, "", "", nil)
		t.forge.PublishCompleted(ctx, scope, forgedomain.CompletedOK, v.ID, v.EnvStatus, 1, nil)
		return marshalEditOutput(v.ID, v.EnvStatus, "", 1, nil, len(ops)), nil
	}

	bundle, bundleErr := llmclientpkg.Resolve(ctx, t.picker, t.keys, t.factory)
	if bundleErr != nil {
		em.DeltaBlock(ctx, progID, fmt.Sprintf("[Attempt 1] env install failed: %s\n", truncForUI(v.EnvError)))
		em.DeltaBlock(ctx, progID, fmt.Sprintf("env-fix loop unavailable: %v\n", bundleErr))
		t.forge.PublishEnvAttempt(ctx, scope, 1, forgedomain.EnvAttemptFailed, "", "", errors.New(v.EnvError))
		t.forge.PublishCompleted(ctx, scope, forgedomain.CompletedFailed, v.ID, v.EnvStatus, 1, bundleErr)
		return marshalEditOutput(v.ID, v.EnvStatus, v.EnvError, 1, nil, len(ops)), nil
	}

	result := envfixpkg.RunLoop(ctx, envfixpkg.Options{
		Bundle: bundle,
		InitialAttempt: envfixpkg.Attempt{
			Number:    1,
			Deps:      append([]string(nil), v.Dependencies...),
			EnvStatus: v.EnvStatus,
			EnvError:  v.EnvError,
		},
		MaxAttempts: envfixpkg.DefaultMaxAttempts,
		ApplyDeps: func(ctx context.Context, newDeps []string) (string, string, error) {
			depsOp, _ := json.Marshal(map[string]any{"dependencies": newDeps})
			retryV, err := t.svc.Edit(ctx, functionapp.EditInput{
				ID: args.ID,
				Ops: []functionapp.Op{{
					Type: "set_dependencies",
					Raw:  depsOp,
				}},
				ChangeReason:    fmt.Sprintf("env-fix retry: %d deps", len(newDeps)),
				ProgressBlockID: progID,
			})
			if err != nil {
				return "", "", err
			}
			return retryV.EnvStatus, retryV.EnvError, nil
		},
		Hooks: envfixpkg.LoopHooks{
			OnFixing: func(ctx context.Context, attempt int) {
				em.DeltaBlock(ctx, progID, fmt.Sprintf("[Attempt %d] AI suggesting revised deps...\n", attempt))
				t.forge.PublishEnvAttempt(ctx, scope, attempt, forgedomain.EnvAttemptFixing, "AI suggesting deps", "", nil)
			},
			OnAttemptResult: func(ctx context.Context, a envfixpkg.Attempt) {
				if a.EnvStatus == "ready" {
					em.DeltaBlock(ctx, progID, fmt.Sprintf("[Attempt %d] env ready ✓\n", a.Number))
					t.forge.PublishEnvAttempt(ctx, scope, a.Number, forgedomain.EnvAttemptOK, "", "", nil)
				} else {
					em.DeltaBlock(ctx, progID, fmt.Sprintf("[Attempt %d] env failed: %s\n", a.Number, truncForUI(a.EnvError)))
					t.forge.PublishEnvAttempt(ctx, scope, a.Number, forgedomain.EnvAttemptFailed, "", "", errors.New(a.EnvError))
				}
			},
		},
	})

	if result.FatalErr != nil {
		em.StopBlock(ctx, progID, eventlogdomain.StatusError, result.FatalErr)
		t.forge.PublishCompleted(ctx, scope, forgedomain.CompletedFailed, v.ID, v.EnvStatus, result.AttemptsUsed, result.FatalErr)
		return "", fmt.Errorf("edit_function: %w", result.FatalErr)
	}

	completedStatus := forgedomain.CompletedFailed
	if result.FinalEnvStatus == functiondomain.EnvStatusReady {
		completedStatus = forgedomain.CompletedOK
	}
	t.forge.PublishCompleted(ctx, scope, completedStatus, v.ID, result.FinalEnvStatus, result.AttemptsUsed, nil)
	return marshalEditOutput(v.ID, result.FinalEnvStatus, result.FinalEnvError,
		result.AttemptsUsed, result.History, len(ops)), nil
}

// marshalEditOutput is edit_function's wire-shape source; unlike create, returns a pending awaiting user accept.
//
// marshalEditOutput edit_function 线协议；不像 create 翻 active，返待审 pending。
func marshalEditOutput(
	pendingID string,
	envStatus, envError string,
	attemptsUsed int,
	history []envfixpkg.Attempt,
	opsApplied int,
) string {
	out := map[string]any{
		"pendingId":    pendingID,
		"envStatus":    envStatus,
		"opsApplied":   opsApplied,
		"attemptsUsed": attemptsUsed,
	}
	if envError != "" {
		out["envError"] = envError
	}
	if len(history) > 1 {
		out["attemptHistory"] = history
	}
	b, _ := json.Marshal(out)
	return string(b)
}
