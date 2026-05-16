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

type CreateHandler struct {
	svc     *handlerapp.Service
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
	forge   forgepkg.Publisher
}

func (t *CreateHandler) Name() string { return "create_handler" }

func (t *CreateHandler) Description() string {
	return `Create a new handler (stateful Python class) by applying a sequence of ops.
v1 is auto-accepted; user must configure init_args via update_handler_config before
call_handler can succeed. If the venv install fails, an internal env-fix loop retries
up to 3 times by asking the LLM to revise the dependency list.

MINIMAL COMPLETE EXAMPLE — a counter handler with persistent state:
  ops = [
    {"op":"set_meta", "name":"counter", "description":"In-memory counter; bump/get"},
    {"op":"set_init_args_schema", "args":[
        {"name":"start","type":"integer","required":true,"description":"initial count"}
    ]},
    {"op":"set_init", "init_body":"self.count = start"},
    {"op":"add_method", "method":{
        "name":"bump",
        "args":[{"name":"by","type":"integer","required":false,"default":1}],
        "body":"self.count += by\nreturn self.count"
    }},
    {"op":"add_method", "method":{
        "name":"get",
        "args":[],
        "body":"return self.count"
    }}
  ]
This generates user_handler.py with class HandlerImpl __init__(self, start: int)
plus bump(self, by: int = 1) and get(self) — bare-name access throughout.

OPS:
  {"op":"set_meta", "name":"...", "description":"..."}
  {"op":"set_imports", "imports":"import psycopg2\nimport json"}
  {"op":"set_init_args_schema", "args":[
      {"name":"dsn", "type":"string", "required":true, "sensitive":true, "description":"PG DSN"},
      {"name":"timeout", "type":"integer", "required":false, "default":30}
  ]}
  {"op":"set_init", "init_body":"self.conn = psycopg2.connect(dsn, connect_timeout=timeout)"}
  {"op":"set_shutdown", "shutdown_body":"self.conn.close()"}
  {"op":"add_method", "method":{"name":"query", "args":[{"name":"sql","type":"string","required":true}], "body":"return self.conn.cursor().execute(sql).fetchall()"}}
  {"op":"update_method", "name":"query", "patch":{"body":"new body"}}
  {"op":"delete_method", "name":"query"}
  {"op":"set_dependencies", "dependencies":["psycopg2-binary"]}

CRITICAL — METHOD BODY CONTRACT (2026-05 refactor):
The framework generates user_handler.py with EXPLODED named params from your
schemas — both __init__ and methods get real Python signatures. Write bodies
using BARE NAMES, not dict access:

  ✅ CORRECT — bare names match the generated signature:
     set_init_args_schema: [{"name":"start","type":"integer","required":true}]
     set_init body:        self.count = start
     add_method query args: [{"name":"key","type":"string","required":true}]
     query body:            return self.data.get(key)

  ❌ WRONG — old dict-access pattern is no longer needed:
     ❌ self.count = init_args["start"]
     ❌ key = args["key"]; return self.data.get(key)

The generated class looks like:
    class HandlerImpl:
        def __init__(self, start: int):
            self.count = start
        def query(self, key: str):
            return self.data.get(key)

ARG TYPES (JSON Schema names, required):
  string / integer / number / boolean / object / array

NAMES — strict Python identifier rules (the framework rejects invalid):
  - method names / arg names / init_arg names: [a-zA-Z_][a-zA-Z0-9_]*
  - no dashes, no Python keywords (class / def / return / for / if / ...)
  - sensitive=true on init_args masks the value in GET / list output

State (self.X) persists across method calls when invoked from a workflow node
with persistent-instance scope; HTTP :call invocations get a fresh instance
per call (chat-scope = per-call lifetime).`
}

func (t *CreateHandler) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"ops": {"type": "array", "items": {"type": "object"}, "description": "Method-level ops"},
			"changeReason": {"type": "string", "description": "One-line reason"}
		},
		"required": ["ops"]
	}`)
}

func (t *CreateHandler) IsReadOnly() bool        { return false }
func (t *CreateHandler) NeedsReadFirst() bool    { return false }
func (t *CreateHandler) RequiresWorkspace() bool { return false }

func (t *CreateHandler) ValidateInput(json.RawMessage) error { return nil }
func (t *CreateHandler) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

func (t *CreateHandler) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Ops          json.RawMessage `json:"ops"`
		ChangeReason string          `json:"changeReason"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("create_handler: bad args: %w", err)
	}
	ops, err := handlerapp.ParseOps(args.Ops)
	if err != nil {
		return "", fmt.Errorf("create_handler: %w", err)
	}

	em := eventlogpkg.From(ctx)
	progID := em.StartBlock(ctx, eventlogdomain.BlockTypeProgress, map[string]any{
		"stage": "applying ops", "count": len(ops),
	})
	defer em.StopBlock(ctx, progID, eventlogdomain.StatusCompleted, nil)

	h, v, err := t.svc.Create(ctx, handlerapp.CreateInput{
		Ops:             ops,
		ChangeReason:    args.ChangeReason,
		ProgressBlockID: progID,
	})
	if err != nil {
		em.StopBlock(ctx, progID, eventlogdomain.StatusError, err)
		return "", fmt.Errorf("create_handler: %w", err)
	}

	// Publish forge_started (C4 D-redo-4) — now that we have the handler ID.
	// 现在有 handler ID,发 forge_started(C4 D-redo-4)。
	scope := eventlogdomain.Scope{Kind: eventlogdomain.KindHandler, ID: h.ID}
	convID, _ := reqctxpkg.GetConversationID(ctx)
	toolCallID, _ := reqctxpkg.GetToolCallID(ctx)
	t.forge.PublishStarted(ctx, scope, forgedomain.OperationCreate, convID, toolCallID)

	if v.EnvStatus == handlerdomain.EnvStatusReady {
		t.forge.PublishEnvAttempt(ctx, scope, 1, forgedomain.EnvAttemptOK, "", "", nil)
		t.forge.PublishCompleted(ctx, scope, forgedomain.CompletedOK, v.ID, v.EnvStatus, 1, nil)
		return marshalCreateOutput(h.ID, v.ID, v.Version, v.Status, v.EnvStatus, "", 1, nil, len(ops)), nil
	}

	bundle, bundleErr := llmclientpkg.Resolve(ctx, t.picker, t.keys, t.factory)
	if bundleErr != nil {
		em.DeltaBlock(ctx, progID, fmt.Sprintf("[Attempt 1] env install failed: %s\n", truncForUI(v.EnvError)))
		em.DeltaBlock(ctx, progID, fmt.Sprintf("env-fix loop unavailable: %v\n", bundleErr))
		t.forge.PublishEnvAttempt(ctx, scope, 1, forgedomain.EnvAttemptFailed, "", "", errors.New(v.EnvError))
		t.forge.PublishCompleted(ctx, scope, forgedomain.CompletedFailed, v.ID, v.EnvStatus, 1, bundleErr)
		return marshalCreateOutput(h.ID, v.ID, v.Version, v.Status, v.EnvStatus, v.EnvError, 1, nil, len(ops)), nil
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
			depsOp, _ := json.Marshal(map[string]any{"deps": newDeps})
			editV, err := t.svc.Edit(ctx, handlerapp.EditInput{
				ID: h.ID,
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

	if result.FatalErr != nil {
		em.StopBlock(ctx, progID, eventlogdomain.StatusError, result.FatalErr)
		t.forge.PublishCompleted(ctx, scope, forgedomain.CompletedFailed, v.ID, v.EnvStatus, result.AttemptsUsed, result.FatalErr)
		return "", fmt.Errorf("create_handler: %w", result.FatalErr)
	}

	if result.FinalEnvStatus == handlerdomain.EnvStatusReady {
		acceptedV, acceptErr := t.svc.AcceptPending(ctx, h.ID)
		if acceptErr != nil && !errors.Is(acceptErr, handlerdomain.ErrPendingNotFound) {
			em.DeltaBlock(ctx, progID, fmt.Sprintf("[final] AcceptPending failed: %v\n", acceptErr))
			t.forge.PublishCompleted(ctx, scope, forgedomain.CompletedFailed, v.ID, "failed", result.AttemptsUsed, acceptErr)
			return marshalCreateOutput(h.ID, v.ID, v.Version, v.Status,
				"failed", acceptErr.Error(), result.AttemptsUsed, result.History, len(ops)), nil
		}
		if acceptedV != nil {
			v = acceptedV
		}
	}

	completedStatus := forgedomain.CompletedFailed
	if result.FinalEnvStatus == handlerdomain.EnvStatusReady {
		completedStatus = forgedomain.CompletedOK
	}
	t.forge.PublishCompleted(ctx, scope, completedStatus, v.ID, result.FinalEnvStatus, result.AttemptsUsed, nil)
	return marshalCreateOutput(h.ID, v.ID, v.Version, v.Status,
		result.FinalEnvStatus, result.FinalEnvError, result.AttemptsUsed, result.History, len(ops)), nil
}

// marshalCreateOutput is the single source of truth for the create_handler
// tool's wire shape (mirrors the function-side helper for consistency).
//
// marshalCreateOutput create_handler 工具线协议 — 跟 function 那侧同形。
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
		"note":         "Use update_handler_config to set init_args before call_handler.",
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

func truncForUI(s string) string {
	const max = 240
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
