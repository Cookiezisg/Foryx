// create.go — create_forge system tool: LLM streams Python code that becomes
// a new forge. Uses the draft → pending → user-accept lifecycle introduced
// by the sandbox iteration: the forge entity is persisted as a draft up
// front, the LLM-generated code lands in a pending ForgeVersion, the
// sandbox synchronously materializes a venv for that pending, and the
// tool_result tells the LLM whether the env is ready (so it can prompt the
// user to accept) or failed (so it can call edit_forge to fix deps).
//
// SSE: emits forge events (entity-state). Both forge.id and pending.id are
// pre-allocated so every snapshot during streaming carries the same identity
// the persisted rows will have. The draft Forge is in DB from the start;
// the pending row's content lives in-memory until the final svc.CreatePending
// call (which also drives sandbox.Sync). LLM-generation failures discard
// the in-memory pending cleanly without DB writes for it; the draft Forge
// stays around so the LLM can call edit_forge to retry without losing
// the conversation context.
//
// create.go — create_forge 系统工具：LLM 流式生成 Python 代码作为新 forge。
// 走沙箱迭代引入的 draft → pending → user-accept 生命周期：forge entity
// 先以 draft 形式落库，LLM 生成的代码进入 pending ForgeVersion，沙箱
// 同步物化 venv，tool_result 告诉 LLM 环境是否 ready（可让用户审核）还是
// failed（可调 edit_forge 修复 deps）。
//
// SSE：发 forge 事件（entity-state）。forge.id 和 pending.id 都预分配，
// 让流式每帧快照与最终落库行身份一致。Draft Forge 一开始就在 DB；pending
// 内容在 svc.CreatePending 落库前留在内存（CreatePending 同时驱动
// sandbox.Sync）。LLM 生成失败时内存 pending 干净丢弃不污染 DB；draft
// Forge 留下让 LLM 调 edit_forge 重试，不丢对话上下文。
package forge

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	forgeapp "github.com/sunweilin/forgify/backend/internal/app/forge"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// CreateForge implements the create_forge system tool.
//
// CreateForge 实现 create_forge 系统工具。
type CreateForge struct {
	svc     *forgeapp.Service
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
}

// ── Identity ──────────────────────────────────────────────────────────────────

func (t *CreateForge) Name() string { return "create_forge" }

func (t *CreateForge) Description() string {
	return "Create a new Python forge in the user's library. " +
		"Provide name, description, instruction, and any non-stdlib dependencies. " +
		"The system generates code, installs the dependencies into a per-forge venv, " +
		"and returns env_status ('ready' = waiting for user accept; 'failed' = call edit_forge with corrected deps). " +
		"The user sees code stream in real time and reviews the result before activation."
}

func (t *CreateForge) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name":           {"type": "string", "description": "Short unique forge name (snake_case)"},
			"description":    {"type": "string", "description": "What this forge does"},
			"instruction":    {"type": "string", "description": "Detailed code generation instruction"},
			"dependencies":   {
				"type": "array",
				"items": {"type": "string"},
				"description": "PEP 508 specifiers for non-stdlib packages used by the forge code (e.g. ['pandas>=2.0','requests']). Omit or pass [] for stdlib-only forges. The system auto-installs these into a per-forge venv before the user can run it."
			},
			"python_version": {
				"type": "string",
				"description": "Optional PEP 440 Python version specifier (e.g. '>=3.12'). Omit to use the bundled default."
			}
		},
		"required": ["name", "description", "instruction"]
	}`)
}

// ── Static metadata ───────────────────────────────────────────────────────────

func (t *CreateForge) IsReadOnly() bool        { return false }
func (t *CreateForge) NeedsReadFirst() bool    { return false }
func (t *CreateForge) RequiresWorkspace() bool { return false }

// ── Args-dependent hooks ──────────────────────────────────────────────────────

func (t *CreateForge) IsConcurrencySafe(json.RawMessage) bool { return false }

func (t *CreateForge) ValidateInput(json.RawMessage) error { return nil }

func (t *CreateForge) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

// ── Execute ───────────────────────────────────────────────────────────────────

func (t *CreateForge) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Name          string   `json:"name"`
		Description   string   `json:"description"`
		Instruction   string   `json:"instruction"`
		Dependencies  []string `json:"dependencies"`
		PythonVersion string   `json:"python_version"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("create_forge: bad args: %w", err)
	}
	convID, _ := reqctxpkg.GetConversationID(ctx)

	// Pre-allocate forge + pending IDs so every snapshot during streaming
	// carries the identity the persisted rows will eventually have.
	// 预分配 forge + pending ID，让流式每帧快照与最终落库行身份一致。
	forgeID := forgeapp.NewForgeID()
	pendingID := forgeapp.NewVersionID()

	// Step 1: persist the forge as a draft (no version row yet).
	// Step 1：以 draft 落 forge entity（无 version 行）。
	draft, err := t.svc.CreateDraft(ctx, forgeapp.CreateInput{
		ID:          forgeID,
		Name:        args.Name,
		Description: args.Description,
	})
	if err != nil {
		return "", fmt.Errorf("create_forge: create draft: %w", err)
	}
	t.svc.PublishSnapshot(ctx, convID, draft)

	// Step 2: build an in-memory pending draft and stream LLM-generated code
	// into its Code field. Each chunk publishes a snapshot through draft.Pending.
	//
	// Step 2：构造内存 pending draft，把 LLM 流出的代码写进 Code 字段。
	// 每个 chunk 通过 draft.Pending 发一帧快照。
	now := time.Now().UTC()
	draftPending := &forgedomain.ForgeVersion{
		ID:            pendingID,
		ForgeID:       forgeID,
		UserID:        draft.UserID,
		Status:        forgedomain.VersionStatusPending,
		Name:          args.Name,
		Description:   args.Description,
		Code:          "",
		Parameters:    "[]",
		ReturnSchema:  "{}",
		Tags:          "[]",
		ChangeReason:  args.Instruction,
		Dependencies:  "[]", // final deps land via svc.CreatePending below
		PythonVersion: args.PythonVersion,
		EnvStatus:     forgedomain.EnvStatusPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	draft.Pending = draftPending
	t.svc.PublishSnapshot(ctx, convID, draft)

	code, err := streamCode(ctx,
		buildCreatePrompt(args.Name, args.Description, args.Instruction),
		t.picker, t.keys, t.factory,
		func(accumulated string) {
			draftPending.Code = accumulated
			draftPending.UpdatedAt = time.Now().UTC()
			t.svc.PublishSnapshot(ctx, convID, draft)
		},
	)
	if err != nil {
		return "", fmt.Errorf("create_forge: generate code: %w", err)
	}

	// Dry-run AST parse before persisting. Syntactically invalid Python →
	// clean retry signal to the LLM, draft forge survives without a pending
	// row so LLM can call edit_forge to retry.
	//
	// 持久化前 dry-run AST。语法错 → 干净的重试信号，draft forge 保留无
	// pending 行，LLM 可调 edit_forge 重试。
	if err := t.svc.ParseCode(code); err != nil {
		return "", fmt.Errorf("create_forge: generated code failed AST parse, please regenerate: %w", err)
	}

	// Step 3: persist the pending + synchronously sync the venv. CreatePending
	// returns the post-sync row, so its EnvStatus reflects ready/failed.
	//
	// Step 3：持久化 pending + 同步 sync venv。CreatePending 返 sync 后的行，
	// EnvStatus 反映 ready/failed。
	pending, err := t.svc.CreatePending(ctx, forgeID, forgeapp.PendingSnapshot{
		ID:            pendingID,
		Name:          args.Name,
		Description:   args.Description,
		Code:          code,
		ChangeReason:  args.Instruction,
		Dependencies:  args.Dependencies,
		PythonVersion: args.PythonVersion,
	})
	if err != nil {
		return "", fmt.Errorf("create_forge: create pending: %w", err)
	}

	// Step 4: final snapshot reflects the persisted state (pending row's
	// real timestamps + parsed parameters/return schema + post-sync env).
	//
	// Step 4：最终快照反映落库状态（pending 真实时间戳 + 已解析的
	// parameters/return schema + sync 后 env 状态）。
	final, err := t.svc.Get(ctx, forgeID)
	if err != nil {
		// reread failure shouldn't fail the whole tool — fall back to
		// in-memory draft with attached pending for the snapshot.
		// 重读失败不该让整个 tool 失败——fallback 到内存 draft 推快照。
		draft.Pending = pending
		final = draft
	}
	t.svc.PublishSnapshot(ctx, convID, final)

	var params any
	if err := json.Unmarshal([]byte(pending.Parameters), &params); err != nil {
		return "", fmt.Errorf("create_forge: corrupted parameters after save for forge %q: %w", forgeID, err)
	}
	b, _ := json.Marshal(map[string]any{
		"forge_id":   forgeID,
		"pending_id": pending.ID,
		"name":       pending.Name,
		"parameters": params,
		"env_status": pending.EnvStatus,
		"env_error":  pending.EnvError,
	})
	return string(b), nil
}
