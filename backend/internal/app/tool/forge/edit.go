// edit.go — edit_forge system tool: proposes a change to an existing forge.
// All edits go through pending review (user must accept/reject before changes
// take effect). Two paths:
//
//   - With Instruction: LLM regenerates code, forge snapshots stream as the
//     pending entity's code grows; final svc.CreatePending persists and
//     drives sandbox.Sync (synchronous).
//   - Without Instruction (metadata-only): no LLM call, no streaming;
//     a single forge snapshot fires after the metadata-only pending is saved.
//
// Unified entry: works on both draft forges (no active version yet) and
// activated forges. If a pending already exists (e.g. previous edit_forge
// left one), it is rejected first so we can write a fresh pending — venv
// directories shared between the rejected pending and other versions are
// preserved (reject only flips status, never touches the filesystem).
//
// SSE: emits forge events (entity-state). The pending row's ID is
// pre-allocated up front so every snapshot during streaming carries the same
// identity as the eventually persisted row. Snapshots include the parent
// Forge with its computed .Pending field populated by the in-memory draft.
//
// edit.go — edit_forge 系统工具：对现有 forge 提出变更。所有编辑走 pending 审核
// （用户 accept/reject 后才生效）。两条路径：
//
//   - 含 Instruction：LLM 重生代码，pending entity 的 code 随每帧 forge 快照
//     生长；最终 svc.CreatePending 落库 + 同步驱动 sandbox.Sync。
//   - 不含 Instruction（仅元数据）：不调 LLM，不推流；
//     metadata-only pending 落库后发一帧 forge 快照。
//
// 统一入口：对草稿 forge（无活跃版本）和已激活 forge 都可用。若已有 pending
// （如前次 edit_forge 留下），先 reject 再写新 pending——被 reject 的 pending
// 跟其他版本共享的 venv 目录保留（reject 仅改 status，不动文件系统）。
//
// SSE：发 forge 事件（entity-state）。pending 行 ID 预分配，让流式每帧快照与
// 最终落库行身份一致。快照携带父 Forge，.Pending 由内存 draft 填充。
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

// EditForge implements the edit_forge system tool.
//
// EditForge 实现 edit_forge 系统工具。
type EditForge struct {
	svc     *forgeapp.Service
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
}

// ── Identity ──────────────────────────────────────────────────────────────────

func (t *EditForge) Name() string { return "edit_forge" }

func (t *EditForge) Description() string {
	return "Propose a change to an existing forge. You can update the code (via instruction), " +
		"name, description, or dependencies. All changes become a pending proposal that the user must confirm. " +
		"Returns env_status reflecting the pending venv state ('ready' = waiting for user accept; 'failed' = retry edit_forge with corrected deps)."
}

func (t *EditForge) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"forge_id":       {"type": "string", "description": "Forge to edit"},
			"instruction":    {"type": "string", "description": "Code modification instruction (omit to only update metadata / dependencies)"},
			"name":           {"type": "string", "description": "New forge name"},
			"description":    {"type": "string", "description": "New description"},
			"dependencies":   {
				"type": "array",
				"items": {"type": "string"},
				"description": "PEP 508 specifiers if the (possibly regenerated) code uses non-stdlib packages. Pass the FULL desired list — it replaces the active version's deps. Omit to inherit the active version's deps unchanged."
			},
			"python_version": {
				"type": "string",
				"description": "Optional PEP 440 Python version specifier. Omit to inherit the active version's setting."
			}
		},
		"required": ["forge_id"]
	}`)
}

// ── Static metadata ───────────────────────────────────────────────────────────

func (t *EditForge) IsReadOnly() bool        { return false }
func (t *EditForge) NeedsReadFirst() bool    { return false }
func (t *EditForge) RequiresWorkspace() bool { return false }

// ── Args-dependent hooks ──────────────────────────────────────────────────────

func (t *EditForge) IsConcurrencySafe(json.RawMessage) bool { return false }

func (t *EditForge) ValidateInput(json.RawMessage) error { return nil }

func (t *EditForge) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

// ── Execute ───────────────────────────────────────────────────────────────────

func (t *EditForge) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		ForgeID       string   `json:"forge_id"`
		Instruction   string   `json:"instruction"`
		Name          string   `json:"name"`
		Description   string   `json:"description"`
		Dependencies  []string `json:"dependencies"`
		PythonVersion string   `json:"python_version"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("edit_forge: bad args: %w", err)
	}
	convID, _ := reqctxpkg.GetConversationID(ctx)

	current, err := t.svc.Get(ctx, args.ForgeID)
	if err != nil {
		return "", fmt.Errorf("edit_forge: get forge: %w", err)
	}

	// If a pending already exists (typically from create_forge or a prior
	// edit_forge that stayed un-accepted), reject it so CreatePending below
	// can write a fresh row. The rejected pending's venv directory is shared
	// across versions by EnvID — reject only flips status, never deletes
	// venv files; trim runs only on accept/revert paths.
	//
	// 已有 pending（通常来自 create_forge 或前次未 accept 的 edit_forge）时
	// 先 reject，让下方 CreatePending 写新行。被 reject 的 pending 的 venv
	// 目录按 EnvID 跨版本共享——reject 仅改 status 不删 venv 文件；trim 仅在
	// accept/revert 路径触发。
	if current.Pending != nil {
		if err := t.svc.RejectPending(ctx, args.ForgeID); err != nil {
			return "", fmt.Errorf("edit_forge: reject existing pending: %w", err)
		}
		// Detach in memory too so the upcoming streaming snapshots show the
		// new draft pending without flashing the rejected one.
		// 内存里也清空，让接下来的流式快照只展示新 draft pending，不闪现旧的。
		current.Pending = nil
	}

	pendingID := forgeapp.NewVersionID()
	snap := forgeapp.PendingSnapshot{
		ID:            pendingID,
		Name:          args.Name,
		Description:   args.Description,
		ChangeReason:  args.Instruction,
		Dependencies:  args.Dependencies,
		PythonVersion: args.PythonVersion,
	}

	// Code-regen path: only when instruction is provided.
	// 代码重生路径：仅当 instruction 非空。
	if args.Instruction != "" {
		// Build a draft pending in memory so streaming snapshots carry the
		// growing code as Forge.Pending.Code.
		//
		// 构造内存 draft pending，让流式快照通过 Forge.Pending.Code 携带生长中的代码。
		now := time.Now().UTC()
		draftPending := &forgedomain.ForgeVersion{
			ID:            pendingID,
			ForgeID:       current.ID,
			UserID:        current.UserID,
			Status:        forgedomain.VersionStatusPending,
			Name:          pickNonEmpty(args.Name, current.Name),
			Description:   pickNonEmpty(args.Description, current.Description),
			Code:          "",
			Parameters:    current.Parameters,
			ReturnSchema:  current.ReturnSchema,
			Tags:          current.Tags,
			ChangeReason:  args.Instruction,
			Dependencies:  "[]", // final deps land via svc.CreatePending below
			PythonVersion: args.PythonVersion,
			EnvStatus:     forgedomain.EnvStatusPending,
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		current.Pending = draftPending
		t.svc.PublishSnapshot(ctx, convID, current)

		newCode, err := streamCode(ctx,
			buildEditPrompt(current.Code, args.Instruction),
			t.picker, t.keys, t.factory,
			func(accumulated string) {
				draftPending.Code = accumulated
				draftPending.UpdatedAt = time.Now().UTC()
				t.svc.PublishSnapshot(ctx, convID, current)
			},
		)
		if err != nil {
			return "", fmt.Errorf("edit_forge: generate code: %w", err)
		}
		// Dry-run AST before committing pending. Same rationale as CreateForge.
		// 提交 pending 前 dry-run AST。理由同 CreateForge。
		if err := t.svc.ParseCode(newCode); err != nil {
			return "", fmt.Errorf("edit_forge: generated code failed AST parse, please regenerate: %w", err)
		}
		snap.Code = newCode
	}

	pending, err := t.svc.CreatePending(ctx, args.ForgeID, snap)
	if err != nil {
		return "", fmt.Errorf("edit_forge: create pending: %w", err)
	}

	// Final snapshot reflects the persisted pending (real timestamps,
	// parsed parameters / return schema, post-sync env state).
	//
	// 最终快照反映落库后的 pending（真实时间戳、解析过的 parameters / return schema、sync 后 env 状态）。
	final, err := t.svc.Get(ctx, args.ForgeID)
	if err != nil {
		// reread failure shouldn't fail the whole tool — fall back to the
		// in-memory current with attached pending for the snapshot.
		// 重读失败不该让整个 tool 失败——fallback 到内存 current + pending。
		current.Pending = pending
		current.UpdatedAt = time.Now().UTC()
		final = current
	}
	t.svc.PublishSnapshot(ctx, convID, final)

	b, _ := json.Marshal(map[string]any{
		"forge_id":   args.ForgeID,
		"pending_id": pending.ID,
		"env_status": pending.EnvStatus,
		"env_error":  pending.EnvError,
	})
	return string(b), nil
}

// pickNonEmpty returns a if non-empty, otherwise b.
// pickNonEmpty 非空返 a，否则返 b。
func pickNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
