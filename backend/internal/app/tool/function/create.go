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

type CreateFunction struct {
	svc     *functionapp.Service
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
	forge   forgepkg.Publisher
}

func (t *CreateFunction) Name() string { return "create_function" }

func (t *CreateFunction) Description() string {
	return "Create a new function by applying a sequence of ops. The ops must include " +
		"set_meta (name + description), set_code (Python source), set_parameters (input " +
		"schema), and optionally set_return_schema / set_dependencies / set_python_version. " +
		"On success the function's v1 is auto-accepted. If the venv install fails, an internal " +
		"env-fix loop retries up to 3 times by asking the LLM to revise the dependency list; " +
		"the final tool result carries envStatus + attemptsUsed + attemptHistory."
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

	// Now we have a Function ID — publish forge_started on the forge bus
	// (C4 D-redo-4). The chat eventlog already saw the apply-ops deltas via
	// ApplyOps; forge stream gets the high-level lifecycle markers.
	// 现在有 Function ID — 在 forge bus 发 forge_started(C4 D-redo-4)。
	// chat eventlog 已经看到 ApplyOps 的 deltas;forge 流拿高层生命周期标记。
	scope := eventlogdomain.Scope{Kind: eventlogdomain.KindFunction, ID: f.ID}
	convID, _ := reqctxpkg.GetConversationID(ctx)
	toolCallID, _ := reqctxpkg.GetToolCallID(ctx)
	t.forge.PublishStarted(ctx, scope, forgedomain.OperationCreate, convID, toolCallID)

	// If the initial env install was already ready, emit attempt=1 ok +
	// completed=ok, return immediately.
	// 初次装即 ready → 发 attempt=1 ok + completed,直接返。
	if v.EnvStatus == functiondomain.EnvStatusReady {
		t.forge.PublishEnvAttempt(ctx, scope, 1, forgedomain.EnvAttemptOK, "", "", nil)
		t.forge.PublishCompleted(ctx, scope, forgedomain.CompletedOK, v.ID, v.EnvStatus, 1, nil)
		return marshalCreateOutput(f.ID, v.ID, v.Version, v.Status, v.EnvStatus, "", 1, nil, len(ops)), nil
	}

	// Env failed — enter env-fix loop. Resolve main-chat LLM bundle.
	// env 失败 → 进 env-fix loop;解析主 chat LLM。
	bundle, bundleErr := llmclientpkg.Resolve(ctx, t.picker, t.keys, t.factory)
	if bundleErr != nil {
		// Without an LLM we cannot fix; surface the install failure as-is.
		// Emit attempt=1 failed + completed=failed.
		// 无 LLM 无法 fix;发 attempt=1 failed + completed=failed。
		em.DeltaBlock(ctx, progID, fmt.Sprintf("[Attempt 1] env install failed: %s\n", truncForUI(v.EnvError)))
		em.DeltaBlock(ctx, progID, fmt.Sprintf("env-fix loop unavailable: %v\n", bundleErr))
		t.forge.PublishEnvAttempt(ctx, scope, 1, forgedomain.EnvAttemptFailed, "", "", errors.New(v.EnvError))
		t.forge.PublishCompleted(ctx, scope, forgedomain.CompletedFailed, v.ID, v.EnvStatus, 1, bundleErr)
		return marshalCreateOutput(f.ID, v.ID, v.Version, v.Status, v.EnvStatus, v.EnvError, 1, nil, len(ops)), nil
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
			// Each retry: ask Service.Edit to apply a new set_dependencies op on
			// top of (initially) active or (after first retry) pending; iterate-
			// same-pending semantics keep the row count bounded.
			//
			// 每次重试调 svc.Edit 加 set_dependencies op;iterate-same-pending
			// 保证行数不爆炸。
			depsOp, _ := json.Marshal(map[string]any{"deps": newDeps})
			editV, err := t.svc.Edit(ctx, functionapp.EditInput{
				ID: f.ID,
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
			return editV.EnvStatus, editV.EnvError, nil
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

	// If the loop ended with a service-level fatal (e.g. sandbox unavailable
	// mid-loop), surface as tool error.
	// 循环中遇 service 级致命错(如 sandbox 不可用)→ 直接抛。
	if result.FatalErr != nil {
		em.StopBlock(ctx, progID, eventlogdomain.StatusError, result.FatalErr)
		t.forge.PublishCompleted(ctx, scope, forgedomain.CompletedFailed, v.ID, v.EnvStatus, result.AttemptsUsed, result.FatalErr)
		return "", fmt.Errorf("create_function: %w", result.FatalErr)
	}

	// On success, flip active version to the fixed pending (v1 stays as a
	// failed historical version). Idempotent for ready path; skipped on
	// failed path.
	// 成功 → AcceptPending 把修好的 pending 翻 active(失败的 v1 留版本史)。
	if result.FinalEnvStatus == functiondomain.EnvStatusReady {
		acceptedV, acceptErr := t.svc.AcceptPending(ctx, f.ID)
		if acceptErr != nil && !errors.Is(acceptErr, functiondomain.ErrPendingNotFound) {
			// AcceptPending failed; surface as env failure on the originally-
			// created v1 row so the LLM sees a coherent state.
			// AcceptPending 失败 → 把它当 env 失败返(LLM 看 v1 状态一致)。
			em.DeltaBlock(ctx, progID, fmt.Sprintf("[final] AcceptPending failed: %v\n", acceptErr))
			t.forge.PublishCompleted(ctx, scope, forgedomain.CompletedFailed, v.ID, "failed", result.AttemptsUsed, acceptErr)
			return marshalCreateOutput(f.ID, v.ID, v.Version, v.Status,
				"failed", acceptErr.Error(), result.AttemptsUsed, result.History, len(ops)), nil
		}
		if acceptedV != nil {
			v = acceptedV
		}
	}

	completedStatus := forgedomain.CompletedFailed
	if result.FinalEnvStatus == functiondomain.EnvStatusReady {
		completedStatus = forgedomain.CompletedOK
	}
	t.forge.PublishCompleted(ctx, scope, completedStatus, v.ID, result.FinalEnvStatus, result.AttemptsUsed, nil)
	return marshalCreateOutput(f.ID, v.ID, v.Version, v.Status,
		result.FinalEnvStatus, result.FinalEnvError, result.AttemptsUsed, result.History, len(ops)), nil
}

// marshalCreateOutput is the single source of truth for create_function's wire shape across success / fallback paths.
//
// marshalCreateOutput 是 create_function 线协议唯一源；成功与 fallback 都走同一 envelope。
func marshalCreateOutput(
	id, versionID string,
	versionN *int,
	status string,
	envStatus, envError string,
	attemptsUsed int,
	history []envfixpkg.Attempt,
	opsApplied int,
) string {
	out := map[string]any{
		"id":           id,
		"versionId":    versionID,
		"version":      versionN,
		"status":       status,
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

// truncForUI shortens a long error message for the UI delta. The DB row
// retains the full envError; this is just to keep the progress block legible.
//
// truncForUI 把过长的错误截短给 UI delta 用;DB 行仍存全文。
func truncForUI(s string) string {
	const max = 240
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
