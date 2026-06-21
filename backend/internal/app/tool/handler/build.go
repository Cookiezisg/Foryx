package handler

import (
	"context"
	"encoding/json"
	"fmt"

	envfixapp "github.com/sunweilin/anselm/backend/internal/app/envfix"
	handlerapp "github.com/sunweilin/anselm/backend/internal/app/handler"
	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	handlerdomain "github.com/sunweilin/anselm/backend/internal/domain/handler"
)

// --- create_handler --------------------------------------------------------

type CreateHandler struct{ svc *handlerapp.Service }

func (t *CreateHandler) Name() string { return "create_handler" }

func (t *CreateHandler) Description() string {
	return `Build a new stateful handler (a Python class that stays resident across calls, so self.xxx persists — for DB connections, API sessions, caches). v1 takes effect immediately (no separate accept). Required ops: set_meta + at least one add_method. The class is assembled as HandlerImpl with __init__(self, ...initArgs), shutdown(self), and your methods.

OP SHAPES:
  {"op":"set_meta", "name":"snake_case", "description":"one line", "tags":["..."]}
  {"op":"set_imports", "imports":"import requests"}
  {"op":"set_init", "initBody":"self.session = requests.Session()"}     — __init__ body (after init args)
  {"op":"set_shutdown", "shutdownBody":"self.session.close()"}          — cleanup on stop/restart
  {"op":"set_init_args_schema", "args":[{"name":"api_key","type":"string","required":true,"sensitive":true}]}
  {"op":"add_method", "method":{"name":"fetch","inputs":[{"name":"url","type":"string"}],"outputs":[{"name":"body","type":"object"}],"body":"return self.session.get(url).json()","streaming":false,"timeout":30000}}
  {"op":"update_method", "name":"fetch", "patch":{"description":"..."}}  — RFC 7396 merge patch
  {"op":"delete_method", "name":"fetch"}
  {"op":"set_dependencies", "dependencies":["requests==2.31"]}
  {"op":"set_python_version", "version":"3.12"}

init_args (secrets like api_key) are NOT set here — the user fills them via the config; mark sensitive:true to encrypt at rest. A method's optional "timeout" (ms) bounds that one call's wall clock; omit it and the call falls back to the global handler-call default — set a tighter timeout for a method that could hang (a slow/blocking call holds the resident instance's serial pipe for its whole duration). A streaming method body yields {"progress": ...} items to stream progress; its call result is then either the last NON-progress value it yields OR its return-statement value (both honored — a bare return is NOT dropped). The instance starts once config is complete; failed dependency installs auto-fix (≤3) with an LLM.`
}

func (t *CreateHandler) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["ops"],
		"properties": {
			"ops": {"type": "array", "description": "Build ops; each has an 'op' discriminator + op-specific fields.", "items": {"type": "object"}},
			"changeReason": {"type": "string", "description": "One-line reason for this creation."}
		}
	}`)
}

func (t *CreateHandler) ValidateInput(args json.RawMessage) error {
	var a struct {
		Ops []json.RawMessage `json:"ops"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("create_handler: bad args: %w", err)
	}
	if len(a.Ops) == 0 {
		return ErrOpsRequired
	}
	return nil
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
	sink := newBuildSink(ctx)
	defer sink.Close()
	h, v, err := t.svc.Create(ctx, handlerapp.CreateInput{Ops: ops, ChangeReason: args.ChangeReason, Progress: sink})
	if err != nil {
		return "", fmt.Errorf("create_handler: %w", err)
	}
	// No runtimeState on create: a fresh handler does not spawn (it almost always needs config first),
	// so "not running" is expected here and would only be noise. The signal matters on EDIT, where a
	// broken change can brick a previously-running instance.
	// create 不报 runtimeState：新建 handler 不 spawn（几乎总要先配 config），此处"未运行"是预期、只会成噪声。
	return toolapp.ToJSON(buildOutput(h.ID, v, len(ops), sink.attempts, "", false)), nil
}

// --- edit_handler ----------------------------------------------------------

type EditHandler struct{ svc *handlerapp.Service }

func (t *EditHandler) Name() string { return "edit_handler" }

func (t *EditHandler) Description() string {
	return `Edit a handler: apply ops on top of its active version, producing a new version that takes effect immediately — the resident instance is restarted to load the new code (which WIPES in-memory state). EXCEPTION: a metadata-only edit (all ops are set_meta — just name/description/tags) does NOT mint a version or restart, so it preserves in-memory state; prefer it for pure renames. Same op shapes as create_handler. Empty ops rebuilds the environment + restarts the instance, which WIPES in-memory state (the result then carries restarted:true — it is not a no-op); if you only want to reset a misbehaving instance, prefer restart_handler. The result includes runtimeState: if it is not "running" after a code edit, the new version failed to spawn (broken __init__ or missing config) — call get_handler for details, fix the code, or revert_handler to the last good version. Use revert_handler to switch to an older version, restart_handler to just reset a misbehaving instance.`
}

func (t *EditHandler) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"required": ["handlerId", "ops"],
		"properties": {
			"handlerId": {"type": "string"},
			"ops": {"type": "array", "description": "Build ops (empty array = rebuild env + restart).", "items": {"type": "object"}},
			"changeReason": {"type": "string", "description": "One-line reason for this edit."}
		}
	}`)
}

func (t *EditHandler) ValidateInput(args json.RawMessage) error {
	var a struct {
		HandlerID string          `json:"handlerId"`
		Ops       json.RawMessage `json:"ops"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("edit_handler: bad args: %w", err)
	}
	if a.HandlerID == "" {
		return ErrHandlerIDRequired
	}
	return nil
}

func (t *EditHandler) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		HandlerID    string          `json:"handlerId"`
		Ops          json.RawMessage `json:"ops"`
		ChangeReason string          `json:"changeReason"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("edit_handler: bad args: %w", err)
	}
	var ops []handlerapp.Op
	if len(args.Ops) > 0 {
		parsed, perr := handlerapp.ParseOps(args.Ops)
		if perr != nil {
			return "", fmt.Errorf("edit_handler: %w", perr)
		}
		ops = parsed
	}
	sink := newBuildSink(ctx)
	defer sink.Close()
	v, err := t.svc.Edit(ctx, handlerapp.EditInput{ID: args.HandlerID, Ops: ops, ChangeReason: args.ChangeReason, Progress: sink})
	if err != nil {
		return "", fmt.Errorf("edit_handler: %w", err)
	}
	// Surface the post-edit runtime state: a broken __init__ (or other spawn failure) builds the env
	// fine (envStatus=ready) but fails to start the resident instance — the restart error is swallowed,
	// so without this the agent reads a "successful" edit and never learns it bricked the handler
	// (F-handler-broken-init-outage). runtimeState != running after a code edit → fix the code or revert.
	// 上呈编辑后的运行态：坏 __init__（或别的 spawn 失败）env 照样 ready、却起不了常驻实例——restart 错误被吞，
	// 没有这里 agent 读到"成功"编辑、永远不知 handler 已 brick。runtimeState != running → 改代码或 revert。
	runtimeState := ""
	if h, gerr := t.svc.Get(ctx, args.HandlerID); gerr == nil {
		runtimeState = h.RuntimeState
	}
	// Empty ops is the env-rebuild + restart path (no ops, no version) — flag the resulting state wipe.
	return toolapp.ToJSON(buildOutput(args.HandlerID, v, len(ops), sink.attempts, runtimeState, len(ops) == 0)), nil
}

func buildOutput(handlerID string, v *handlerdomain.Version, opsApplied int, attempts []envfixapp.Attempt, runtimeState string, restarted bool) map[string]any {
	out := map[string]any{
		"id":         handlerID,
		"versionId":  v.ID,
		"version":    v.Version,
		"envStatus":  v.EnvStatus,
		"opsApplied": opsApplied,
	}
	// An empty-ops edit_handler rebuilds the env and RESTARTS the resident instance but applies no ops
	// and mints no version — so opsApplied:0 + an unchanged version reads like a no-op while the restart
	// WIPED in-memory state. Signal the restart so a stateful handler's state loss is visible, not silent.
	// 空 ops 的 edit_handler 重建 env 并重启常驻实例、却不应用 op、不铸版本——故 opsApplied:0 + 版本不变读着像
	// no-op，而重启已抹掉内存态。显式上呈重启，使有状态 handler 的态丢失可见、非静默。
	if restarted {
		out["restarted"] = true
		out["restartNote"] = "rebuilt the environment and restarted the resident instance — in-memory state was wiped (no ops applied, no new version). If you only meant to reset a misbehaving instance, restart_handler does the same; a no-op was not intended here."
	}
	if v.EnvError != "" {
		out["envError"] = v.EnvError
	}
	if runtimeState != "" {
		out["runtimeState"] = runtimeState
		// A non-running instance after an edit means the new code/config didn't come up (broken
		// __init__, missing config, env not ready) — the env can be "ready" yet the handler unusable.
		if runtimeState != handlerdomain.RuntimeStateRunning {
			out["runtimeWarning"] = "the resident instance is not running after this edit — the new version may have a broken __init__ or need config; call get_handler for crash/config details, fix the code, or revert_handler to the last good version"
		}
	}
	if len(attempts) > 1 {
		out["envFixAttempts"] = attempts
	}
	return out
}
