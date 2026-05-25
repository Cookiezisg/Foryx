package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	envfixpkg "github.com/sunweilin/forgify/backend/internal/pkg/envfix"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	forgepkg "github.com/sunweilin/forgify/backend/internal/pkg/forge"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

type EditHandler struct {
	svc     *handlerapp.Service
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
	forge   forgepkg.Publisher
}

func (t *EditHandler) Name() string { return "edit_handler" }

func (t *EditHandler) Description() string {
	return `Edit a handler by applying ops (same op shapes as create_handler). Creates/iterates a pending version awaiting user accept; pass ops=[] to force-rebuild the active version's env. Use update_method for in-place body changes (JSON Merge Patch). A failed venv install triggers an internal env-fix loop (≤3 LLM dep-revision retries).

BODY CONTRACT: method/init bodies use BARE NAMES from the schema — write 'self.x = dsn' not 'self.x = init_args["dsn"]'; write 'return self.run(sql)' not 'args["sql"]'.

Keep the entity description (set_meta.description) to one short line — it appears in the capability menu.`
}

func (t *EditHandler) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"id": {"type": "string"},
			"ops": {"type": "array", "items": {"type": "object"}},
			"changeReason": {"type": "string"}
		},
		"required": ["id", "ops"]
	}`)
}

func (t *EditHandler) IsReadOnly() bool        { return false }
func (t *EditHandler) NeedsReadFirst() bool    { return false }
func (t *EditHandler) RequiresWorkspace() bool { return false }

func (t *EditHandler) ValidateInput(json.RawMessage) error { return nil }
func (t *EditHandler) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *EditHandler) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ID           string          `json:"id"`
		Ops          json.RawMessage `json:"ops"`
		ChangeReason string          `json:"changeReason"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("edit_handler: bad args: %w", err)
	}
	if args.ID == "" {
		return "", fmt.Errorf("edit_handler: id required")
	}
	ops, err := handlerapp.ParseOps(args.Ops)
	if err != nil {
		return "", fmt.Errorf("edit_handler: %w", err)
	}

	em := eventlogpkg.From(ctx)
	progID := em.StartBlock(ctx, eventlogdomain.BlockTypeProgress, map[string]any{
		"stage": "applying ops", "count": len(ops), "handlerId": args.ID,
	})
	defer em.StopBlock(ctx, progID, eventlogdomain.StatusCompleted, nil)

	// Publish forge_started (C4 D-redo-4).
	// 发 forge_started(C4 D-redo-4)。
	scope := eventlogdomain.Scope{Kind: eventlogdomain.KindHandler, ID: args.ID}
	convID, _ := reqctxpkg.GetConversationID(ctx)
	toolCallID, _ := reqctxpkg.GetToolCallID(ctx)
	t.forge.PublishStarted(ctx, scope, forgedomain.OperationEdit, convID, toolCallID)

	v, err := t.svc.Edit(ctx, handlerapp.EditInput{
		ID:              args.ID,
		Ops:             ops,
		ChangeReason:    args.ChangeReason,
		ProgressBlockID: progID,
	})
	if err != nil {
		em.StopBlock(ctx, progID, eventlogdomain.StatusError, err)
		t.forge.PublishCompleted(ctx, scope, forgedomain.CompletedFailed, "", "", 0, err)
		return "", fmt.Errorf("edit_handler: %w", err)
	}

	if v.EnvStatus == handlerdomain.EnvStatusReady {
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
			retryV, err := t.svc.Edit(ctx, handlerapp.EditInput{
				ID: args.ID,
				Ops: []handlerapp.Op{{
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
		return "", fmt.Errorf("edit_handler: %w", result.FatalErr)
	}

	completedStatus := forgedomain.CompletedFailed
	if result.FinalEnvStatus == handlerdomain.EnvStatusReady {
		completedStatus = forgedomain.CompletedOK
	}
	t.forge.PublishCompleted(ctx, scope, completedStatus, v.ID, result.FinalEnvStatus, result.AttemptsUsed, nil)
	return marshalEditOutput(v.ID, result.FinalEnvStatus, result.FinalEnvError,
		result.AttemptsUsed, result.History, len(ops)), nil
}

// marshalEditOutput is the single source of truth for the edit_handler tool's
// wire shape. Distinct from create_handler — Edit returns a pending awaiting
// user accept.
//
// marshalEditOutput edit_handler 工具线协议;跟 create 不同 — 不翻 active。
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
