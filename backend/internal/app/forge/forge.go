// Package forge (app layer) owns the Service that orchestrates the forge domain:
// CRUD, version/pending lifecycle, sandbox execution, test cases, AI-powered
// test-case generation, and unified execution history.
//
// All three forge packages (domain / app / store) declare `package forge`;
// external callers alias at import (e.g. forgeapp "…/internal/app/forge").
//
// Package forge（app 层）负责 Service 编排 forge domain：CRUD、版本/pending
// 生命周期、沙箱执行、测试用例、AI 辅助测试用例生成、统一执行历史。
//
// 三个 forge 包均声明 `package forge`；外部调用方 import 时按角色起别名，
// 如 forgeapp "…/internal/app/forge"。
package forge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	eventsdomain "github.com/sunweilin/forgify/backend/internal/domain/events"
	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	llmparsepkg "github.com/sunweilin/forgify/backend/internal/pkg/llmparse"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// ── Interfaces ────────────────────────────────────────────────────────────────

// Sandbox is the port through which Service materializes forge venvs and
// executes forge code. The infra/sandbox package provides the concrete
// implementation. Service is responsible for tracking environment state
// in ForgeVersion rows; the Sandbox is filesystem / subprocess only.
//
// Sandbox 是 Service 物化 forge venv + 执行代码的端口。具体实现由
// infra/sandbox 提供。Service 负责把环境状态记到 ForgeVersion 行；
// Sandbox 只管文件系统 / 子进程。
type Sandbox interface {
	// PythonPath returns the absolute path to the bundled Python interpreter
	// (raw, not in any venv). Used by Service.parse to invoke the AST
	// extraction helper without going through uv — same Python that uv uses
	// as `--python` target.
	//
	// PythonPath 返回捆绑 Python 解释器的绝对路径（raw，不在任何 venv 内）。
	// Service.parse 调 AST 提取 helper 时用这个，不走 uv——跟 uv 的
	// --python 目标是同一个 Python。
	PythonPath() string

	// Sync materializes the venv directory for the given EnvID. Idempotent —
	// already-existing .venv returns nil immediately. Adapter
	// implementations wrap underlying errors (e.g. uv stderr) in their
	// own error type.
	//
	// Sync 物化指定 EnvID 的 venv 目录。幂等——.venv 已存在则立即返 nil。
	// adapter 实现包装底层错误（如 uv stderr）为自己的错类型。
	Sync(ctx context.Context, req SyncRequest) error

	// Run executes a forge in its EnvID's venv. ctx-cancel kills the whole
	// process tree. No timeout enforced.
	//
	// Run 在 EnvID 的 venv 中执行 forge。ctx-cancel 杀整个进程树。无 timeout。
	Run(ctx context.Context, req RunRequest) (*forgedomain.ExecutionResult, error)

	// WriteCodeFile updates main.py for a version without touching its venv.
	// Used when EnvID is unchanged but code changed.
	//
	// WriteCodeFile 写 version 的 main.py 不动 venv。EnvID 不变只代码变时用。
	WriteCodeFile(ctx context.Context, forgeID, versionID, code, entryFunction string) error

	// Destroy removes the entire forge directory.
	// Destroy 删整个 forge 目录。
	Destroy(ctx context.Context, forgeID string) error

	// DestroyEnv removes a single EnvID directory under a forge — used by
	// trimEnvBuffer to evict an old EnvID's venv beyond MaxEnvIDsPerForge.
	//
	// DestroyEnv 删 forge 下单个 EnvID 目录——trimEnvBuffer 在超过
	// MaxEnvIDsPerForge 时驱逐旧 EnvID 的 venv 时用。
	DestroyEnv(ctx context.Context, forgeID, envID string) error
}

// LLMClient makes non-streaming LLM calls that return complete JSON responses.
// Used by GenerateTestCases. The implementation resolves model/key internally.
//
// LLMClient 进行非流式 LLM 调用，返回完整 JSON 响应。
// 供 GenerateTestCases 使用；实现层内部解析 model/key。
type LLMClient interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

// GenerateResult is the synchronous return shape of GenerateTestCases.
// Either NotSupported is true (with Reason) or TestCases contains the saved cases.
//
// GenerateResult 是 GenerateTestCases 同步返回的形状。
// 要么 NotSupported=true（含 Reason），要么 TestCases 含已保存的用例。
type GenerateResult struct {
	NotSupported bool                         `json:"notSupported"`
	Reason       string                       `json:"reason,omitempty"`
	TestCases    []*forgedomain.ForgeTestCase `json:"testCases,omitempty"`
}

// ── Input / Output types ──────────────────────────────────────────────────────

// CreateInput is the request shape for Service.Create. ID is optional —
// when set (typically by a tool that pre-allocated an ID for streaming
// snapshot identity stability), Service.Create uses it; otherwise a
// fresh ID is generated.
//
// CreateInput 是 Service.Create 的请求形状。ID 可选——若调用方（通常是
// 预分配 ID 以保证流式快照身份稳定的工具）设置了 ID，Service.Create 直接
// 使用；否则生成新 ID。
type CreateInput struct {
	ID            string
	Name          string
	Description   string
	Code          string
	Tags          []string
	Dependencies  []string // PEP 508 specifiers, e.g. ["pandas>=2.0"]; empty = stdlib only
	PythonVersion string   // PEP 440 spec, e.g. ">=3.12"; empty falls back to forgedomain.DefaultPythonVersion
}

// UpdateInput is the request shape for Service.Update. Nil fields are unchanged.
//
// UpdateInput 是 Service.Update 的请求形状。nil 字段不更新。
type UpdateInput struct {
	Name        *string
	Description *string
	Tags        *[]string
	Code        *string
}

// PendingSnapshot is the proposed new state passed to Service.CreatePending.
// ID is optional — when set (typically by a tool that pre-allocated an ID
// for streaming snapshot identity stability), it overrides the generated one.
//
// PendingSnapshot 是传给 Service.CreatePending 的提案新状态。
// ID 可选——预分配（通常用于流式快照身份稳定）时覆盖生成 ID。
type PendingSnapshot struct {
	ID            string
	Name          string
	Description   string
	Code          string
	Tags          string // JSON string
	ChangeReason  string
	Dependencies  []string // PEP 508 specifiers; nil/empty = inherit from active version's deps
	PythonVersion string   // PEP 440 spec; empty = inherit from active version's pythonVersion
}

// TestCaseInput is the request shape for Service.CreateTestCase.
//
// TestCaseInput 是 Service.CreateTestCase 的请求形状。
type TestCaseInput struct {
	Name           string
	InputData      string // JSON object string
	ExpectedOutput string // JSON string; empty = no assertion
}

// ForgeDetail extends Forge with a pre-computed TestSummary for get_forge.
//
// ForgeDetail 在 Forge 基础上追加预计算的 TestSummary，供 get_forge 使用。
type ForgeDetail struct {
	*forgedomain.Forge
	TestSummary TestSummary
}

// TestSummary is a short digest of the most recent :test batch run.
//
// TestSummary 是最近一次 :test 批跑的简要摘要。
type TestSummary struct {
	Total        int    // current test case count
	LastPassRate string // "3/3" | "2/3" | "" (no record)
	LastRunAt    string // ISO 8601 or ""
}

// ── Service ───────────────────────────────────────────────────────────────────

// Service orchestrates the forge domain.
//
// Service 编排 forge domain。
type Service struct {
	repo    forgedomain.Repository
	sandbox Sandbox
	llm     LLMClient
	bridge  eventsdomain.Bridge // optional — nil for non-chat callers (HTTP forge handlers)
	log     *zap.Logger
}

// NewService wires Service dependencies. bridge is optional — pass nil when
// the service is used outside the chat agent (forge.PublishSnapshot is a
// no-op in that case). Panics on nil logger.
//
// NewService 装配 Service 依赖。bridge 可选——chat agent 之外的调用方传 nil
// 即可（PublishSnapshot 此时是 no-op）。nil logger 会 panic。
func NewService(repo forgedomain.Repository, sandbox Sandbox, llm LLMClient, bridge eventsdomain.Bridge, log *zap.Logger) *Service {
	if log == nil {
		panic("forgeapp.NewService: logger is nil")
	}
	return &Service{repo: repo, sandbox: sandbox, llm: llm, bridge: bridge, log: log}
}

// PublishSnapshot emits a forge entity-state event over the events bridge.
// Centralised so create_forge / edit_forge tools (and future sandbox workers)
// publish through one helper instead of inlining bridge.Publish at every site
// — same pattern as chat.runner.publishMessageSnapshot for chat.message.
//
// No-op when bridge is unset (HTTP forge handler caller) or convID is empty
// (HTTP-driven flow with no chat conversation context).
//
// PublishSnapshot 把 forge entity-state 事件发到事件 bridge。集中管理，
// 让 create_forge / edit_forge 工具（及未来的 sandbox worker）走同一 helper，
// 不在每处内联 bridge.Publish——与 chat.runner.publishMessageSnapshot 同模式。
//
// bridge 未设置（HTTP forge handler 调用方）或 convID 空（无 chat 上下文的
// HTTP 触发流程）时静默返回。
func (s *Service) PublishSnapshot(ctx context.Context, convID string, f *forgedomain.Forge) {
	if s.bridge == nil || convID == "" || f == nil {
		return
	}
	s.bridge.Publish(ctx, convID, eventsdomain.Forge{Forge: f})
}

// ── CRUD ──────────────────────────────────────────────────────────────────────

// Create parses the code, persists the Forge, and saves v1 accepted version.
// If in.ID is set the caller-provided value is used; otherwise a fresh ID
// is generated.
//
// Create 解析代码，持久化 Forge，保存 v1 已接受版本，并同步触发 venv 物化。
// in.ID 已设则用调用方传入的值；否则生成新 ID。
//
// venv sync 失败不让 Create 整体失败——forge 已落库，EnvStatus="failed" +
// EnvError 含 uv 输出，调用方（HTTP 客户端 / LLM 工具）通过 forge.EnvStatus
// 判断后续动作（重试 / 改 deps / 接受错误）。这跟设计文档 §11.1 punt-to-AI
// 一致：错误暴露在 entity 字段而非 error 返回值上。
func (s *Service) Create(ctx context.Context, in CreateInput) (*forgedomain.Forge, error) {
	parsed, err := s.parse(in.Code)
	if err != nil {
		return nil, err
	}
	id := in.ID
	if id == "" {
		id = newID("f")
	}
	now := time.Now().UTC()
	f := &forgedomain.Forge{
		ID:           id,
		Name:         in.Name,
		Description:  in.Description,
		Code:         in.Code,
		Parameters:   parsed.parametersJSON,
		ReturnSchema: parsed.returnSchemaJSON,
		Tags:         tagsJSON(in.Tags),
		VersionCount: 1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err = mustSetUserID(ctx, f); err != nil {
		return nil, err
	}
	one := 1
	v := newVersion(f, forgedomain.VersionStatusAccepted, &one, "initial")
	s.fillEnvFields(v, in.Dependencies, in.PythonVersion)
	// Wire ActiveVersionID before saving so the Forge row reflects the
	// active version atomically with creation. SaveForge then SaveVersion
	// is two writes — the version is the FK target, but no FK constraint
	// requires order; we keep this order so a partial failure leaves a
	// forge with no version (cleaner than a version with no forge).
	//
	// 落库前先 wire ActiveVersionID，让 Forge 行原子反映活跃版本。
	// SaveForge 然后 SaveVersion 是两次写——version 是 FK target 但无
	// FK 约束强制顺序；保持此顺序让部分失败留下"forge 无 version"
	// （比"version 无 forge"干净）。
	f.ActiveVersionID = v.ID
	if err = s.repo.SaveForge(ctx, f); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, forgedomain.ErrDuplicateName
		}
		return nil, fmt.Errorf("forgeapp.Create: %w", err)
	}
	if err = s.repo.SaveVersion(ctx, v); err != nil {
		return nil, fmt.Errorf("forgeapp.Create: save version: %w", err)
	}
	// Sync env synchronously. Failure → EnvStatus="failed" + EnvError set
	// by SyncEnvForVersion; we log and continue so caller still gets the
	// forge entity (with the failed env state visible via attachActiveEnv).
	//
	// 同步等 sync。失败 → SyncEnvForVersion 设 EnvStatus="failed" + EnvError；
	// 我们 log 并继续，让调用方仍拿到 forge entity（失败状态通过
	// attachActiveEnv 暴露给前端）。
	if err := s.SyncEnvForVersion(ctx, "", v.ID); err != nil {
		s.log.Warn("forgeapp.Create: sync env failed",
			zap.String("forge_id", f.ID),
			zap.String("version_id", v.ID),
			zap.Error(err))
	}
	if err := s.attachActiveEnv(ctx, f); err != nil {
		s.log.Warn("forgeapp.Create: attach active env",
			zap.String("forge_id", f.ID), zap.Error(err))
	}
	return f, nil
}

// CreateDraft persists a Forge entity with no version yet — ActiveVersionID
// is "" and VersionCount is 0. Used by the create_forge LLM tool which then
// builds a pending ForgeVersion via CreatePending and waits for user accept
// before promoting to v1. HTTP POST /forges uses Create instead (immediate
// v1 accepted).
//
// CreateDraft 持久化一个**没有 version** 的 Forge entity——ActiveVersionID 空、
// VersionCount=0。供 create_forge LLM 工具使用：之后通过 CreatePending 建
// pending ForgeVersion，等用户 accept 后才升 v1。HTTP POST /forges 走
// Create（直接 v1 accepted）。
func (s *Service) CreateDraft(ctx context.Context, in CreateInput) (*forgedomain.Forge, error) {
	id := in.ID
	if id == "" {
		id = newID("f")
	}
	now := time.Now().UTC()
	f := &forgedomain.Forge{
		ID:           id,
		Name:         in.Name,
		Description:  in.Description,
		Code:         "", // draft has no canonical code yet — that's the pending row's job
		Parameters:   "[]",
		ReturnSchema: "{}",
		Tags:         tagsJSON(in.Tags),
		VersionCount: 0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := mustSetUserID(ctx, f); err != nil {
		return nil, err
	}
	if err := s.repo.SaveForge(ctx, f); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, forgedomain.ErrDuplicateName
		}
		return nil, fmt.Errorf("forgeapp.CreateDraft: %w", err)
	}
	return f, nil
}

// Get fetches a single live Forge with its pending change populated (if any).
//
// Get 查询单条活跃 Forge，并填充 pending 变更（如有）+ 活跃版本的 env 状态。
func (s *Service) Get(ctx context.Context, id string) (*forgedomain.Forge, error) {
	f, err := s.repo.GetForge(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.attachPending(ctx, f); err != nil {
		return nil, err
	}
	if err := s.attachActiveEnv(ctx, f); err != nil {
		return nil, err
	}
	return f, nil
}

// GetDetail returns the Forge plus a TestSummary for get_forge system tool.
//
// GetDetail 返回 Forge 及 TestSummary，供 get_forge system tool 使用。
func (s *Service) GetDetail(ctx context.Context, id string) (*ForgeDetail, error) {
	f, err := s.repo.GetForge(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.attachPending(ctx, f); err != nil {
		return nil, err
	}
	if err := s.attachActiveEnv(ctx, f); err != nil {
		return nil, err
	}

	cases, err := s.repo.ListTestCases(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("forgeapp.GetDetail: list test cases: %w", err)
	}
	summary := TestSummary{Total: len(cases)}

	// Find the most recent test batch: peek at the latest test execution row,
	// then pull all rows sharing that batchID. nextCursor is irrelevant for
	// this internal lookup.
	//
	// 找最近一次 test 批次：取最新一行 test 执行，再按 batchID 拉齐整批。
	// 内部查询不关心 nextCursor。
	recent, _, err := s.repo.ListExecutions(ctx, forgedomain.ExecutionFilter{
		ForgeID: id, Kind: forgedomain.ExecutionKindTest, Limit: 1,
	})
	if err != nil {
		return nil, fmt.Errorf("forgeapp.GetDetail: list recent test execution: %w", err)
	}
	if len(recent) > 0 && recent[0].BatchID != "" {
		batch, _, err := s.repo.ListExecutions(ctx, forgedomain.ExecutionFilter{
			ForgeID: id, BatchID: recent[0].BatchID, Limit: forgedomain.MaxExecutionsPerForge,
		})
		if err != nil {
			return nil, fmt.Errorf("forgeapp.GetDetail: list batch executions: %w", err)
		}
		if len(batch) > 0 {
			passed := 0
			for _, e := range batch {
				if e.Pass != nil && *e.Pass {
					passed++
				}
			}
			summary.LastPassRate = fmt.Sprintf("%d/%d", passed, len(batch))
			summary.LastRunAt = batch[len(batch)-1].CreatedAt.UTC().Format(time.RFC3339)
		}
	}
	return &ForgeDetail{Forge: f, TestSummary: summary}, nil
}

// List returns a cursor-paginated page of forges.
//
// List 返回 cursor 分页的 forge 列表。
func (s *Service) List(ctx context.Context, filter forgedomain.ListFilter) ([]*forgedomain.Forge, string, error) {
	rows, next, err := s.repo.ListForges(ctx, filter)
	if err != nil {
		return nil, "", err
	}
	for _, f := range rows {
		if err := s.attachPending(ctx, f); err != nil {
			return nil, "", err
		}
		if err := s.attachActiveEnv(ctx, f); err != nil {
			return nil, "", err
		}
	}
	return rows, next, nil
}

// ListAll returns all live forges without pagination (used by SearchForge).
//
// ListAll 返回所有活跃 forge，不分页（供 SearchForge 使用）。
func (s *Service) ListAll(ctx context.Context) ([]*forgedomain.Forge, error) {
	return s.repo.ListAllForges(ctx)
}

// GetForgesByIDs fetches multiple live forges by ID slice, preserving order.
//
// GetForgesByIDs 按 ID 切片批量查活跃 forge，保持顺序。
func (s *Service) GetForgesByIDs(ctx context.Context, ids []string) ([]*forgedomain.Forge, error) {
	return s.repo.GetForgesByIDs(ctx, ids)
}

// ListExecutions exposes Repository.ListExecutions for handlers / system tools.
// Returns (rows, nextCursor, err); nextCursor "" means no more pages.
//
// ListExecutions 把 Repository.ListExecutions 暴露给 handler / system tool 使用。
// 返回 (rows, nextCursor, err)；nextCursor 为 "" 表示无下一页。
func (s *Service) ListExecutions(ctx context.Context, filter forgedomain.ExecutionFilter) ([]*forgedomain.ForgeExecution, string, error) {
	return s.repo.ListExecutions(ctx, filter)
}

// Update applies partial changes to a Forge. Code changes trigger an AST
// re-parse and auto-reject any active pending.
//
// Update 对 Forge 做局部更新。代码变更触发 AST 重解析并自动 reject 现有 pending。
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*forgedomain.Forge, error) {
	f, err := s.repo.GetForge(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		f.Name = *in.Name
	}
	if in.Description != nil {
		f.Description = *in.Description
	}
	if in.Tags != nil {
		f.Tags = tagsJSON(*in.Tags)
	}
	if in.Code != nil {
		if err = s.autoRejectPending(ctx, id); err != nil {
			return nil, err
		}
		parsed, err := s.parse(*in.Code)
		if err != nil {
			return nil, err
		}
		f.Code = *in.Code
		f.Parameters = parsed.parametersJSON
		f.ReturnSchema = parsed.returnSchemaJSON
		f.VersionCount++
		v := newVersion(f, forgedomain.VersionStatusAccepted, &f.VersionCount, "manual edit")
		// Inherit deps + python + EnvID from the current ActiveVersion —
		// Update only takes a Code change, never deps. The new version row
		// shares the existing venv (same EnvID), so no re-sync needed; we
		// also inherit EnvStatus so a previously-ready env stays ready and
		// only the code file gets rewritten on next Run (via
		// sandbox.WriteCodeFile, called from C2 path here is unnecessary
		// since Run does the write).
		//
		// 沿用当前 ActiveVersion 的 deps + python + EnvID——Update 只接受
		// Code 改动不接受 deps。新 version 行共用现有 venv（同 EnvID），
		// 不需重 sync；继承 EnvStatus 让原本 ready 的 env 保持 ready，
		// 下次 Run 只重写代码文件（Run 自己写，无需此处调
		// sandbox.WriteCodeFile）。
		if f.ActiveVersionID != "" {
			active, err := s.repo.GetVersionByID(ctx, f.ActiveVersionID)
			if err != nil {
				return nil, fmt.Errorf("forgeapp.Update: load active version: %w", err)
			}
			v.Dependencies = active.Dependencies
			v.PythonVersion = active.PythonVersion
			v.EnvID = active.EnvID
			v.EnvStatus = active.EnvStatus
			v.EnvError = active.EnvError
			v.EnvSyncedAt = active.EnvSyncedAt
		} else {
			// No prior active version (first PATCH on a draft forge that
			// has no v1 yet). Treat as a fresh stdlib-only env.
			//
			// 没活跃版本（草稿 forge 首次 PATCH）。当 stdlib-only 处理。
			s.fillEnvFields(v, nil, "")
		}
		if err = s.repo.SaveVersion(ctx, v); err != nil {
			return nil, fmt.Errorf("forgeapp.Update: save version: %w", err)
		}
		f.ActiveVersionID = v.ID
		if err = s.trimVersions(ctx, id); err != nil {
			return nil, err
		}
	}
	f.UpdatedAt = time.Now().UTC()
	if err = s.repo.SaveForge(ctx, f); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, forgedomain.ErrDuplicateName
		}
		return nil, fmt.Errorf("forgeapp.Update: %w", err)
	}
	return f, nil
}

// Delete soft-deletes a Forge and removes its on-disk venvs / code files.
// The DB rows (forges + forge_versions + forge_executions) survive in the
// soft-deleted state; the bin / forge directory is fully wiped because there's
// no value in keeping orphaned venvs around — if the forge is restored later,
// SyncEnvForVersion will rebuild from scratch.
//
// sandbox.Destroy failure is logged but doesn't fail Delete: DB soft-delete
// is the user-visible truth, and a stray directory is harmless until the
// next Bootstrap GC pass (planned, not yet implemented per §11.1 punt-to-AI).
//
// Delete 软删 Forge 并清掉它在磁盘上的 venvs / 代码文件。DB 行
// （forges + forge_versions + forge_executions）以软删状态保留；
// bin / forge 目录全清——保留孤立 venv 没价值，将来若 restore，
// SyncEnvForVersion 会从头重建。
//
// sandbox.Destroy 失败仅 log，不让 Delete 失败：DB 软删才是用户可见真相，
// 残留目录无害，下次 Bootstrap GC（计划中，按 §11.1 punt-to-AI 暂未实现）
// 会清。
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.repo.DeleteForge(ctx, id); err != nil {
		return err
	}
	if err := s.sandbox.Destroy(ctx, id); err != nil {
		s.log.Warn("forgeapp.Delete: sandbox destroy failed",
			zap.String("forge_id", id), zap.Error(err))
	}
	return nil
}

// ── Version management ────────────────────────────────────────────────────────

// ListVersions returns accepted versions newest-first.
//
// ListVersions 返回已接受版本，最新在前。
func (s *Service) ListVersions(ctx context.Context, forgeID string) ([]*forgedomain.ForgeVersion, error) {
	return s.repo.ListAcceptedVersions(ctx, forgeID)
}

// GetVersion returns a specific accepted version.
//
// GetVersion 返回指定已接受版本。
func (s *Service) GetVersion(ctx context.Context, forgeID string, version int) (*forgedomain.ForgeVersion, error) {
	return s.repo.GetVersion(ctx, forgeID, version)
}

// RevertToVersion restores a forge to the complete snapshot of a prior version.
//
// RevertToVersion 将 forge 恢复到指定历史版本的完整快照。
//
// 行为：(1) 创建 newV 复制目标版本所有快照字段（含 deps + EnvID +
// EnvStatus），保留同 EnvID 让 venv 共享生效；(2) 把 ActiveVersionID 切到
// newV；(3) 如目标 EnvStatus="evicted"（venv 被 N=3 缓冲清掉），同步触发
// SyncEnvForVersion 重建；(4) trimEnvBuffer 整理。
//
// EnvID 跨过 evicted 后被一次性恢复——sync 用同 EnvID，原 venv 物理目录
// 重建后所有引用此 EnvID 的旧 ForgeVersion 行立刻可用。
func (s *Service) RevertToVersion(ctx context.Context, forgeID string, version int) (*forgedomain.Forge, error) {
	v, err := s.repo.GetVersion(ctx, forgeID, version)
	if err != nil {
		return nil, err
	}
	f, err := s.repo.GetForge(ctx, forgeID)
	if err != nil {
		return nil, err
	}
	if err = s.autoRejectPending(ctx, forgeID); err != nil {
		return nil, err
	}
	f.Name = v.Name
	f.Description = v.Description
	f.Code = v.Code
	f.Parameters = v.Parameters
	f.ReturnSchema = v.ReturnSchema
	f.Tags = v.Tags
	f.VersionCount++
	f.UpdatedAt = time.Now().UTC()
	reason := fmt.Sprintf("reverted to v%d", version)
	newV := newVersion(f, forgedomain.VersionStatusAccepted, &f.VersionCount, reason)
	// Inherit deps + EnvID from the target version — the new accepted row
	// shares the same venv directory (same EnvID), so revert is venv-free
	// when the env still exists. Inherit EnvStatus too: if the target was
	// evicted, the new row is also evicted until SyncEnvForVersion below
	// rebuilds the venv.
	//
	// 沿用目标版本的 deps + EnvID——新 accepted 行共用同一 venv 目录
	// （同 EnvID），env 还在时 revert 不动 venv。EnvStatus 也继承：
	// 目标若 evicted，新行也 evicted，直到下面的 SyncEnvForVersion 重建。
	newV.Dependencies = v.Dependencies
	newV.PythonVersion = v.PythonVersion
	newV.EnvID = v.EnvID
	newV.EnvStatus = v.EnvStatus
	newV.EnvError = v.EnvError
	newV.EnvSyncedAt = v.EnvSyncedAt
	if err = s.repo.SaveVersion(ctx, newV); err != nil {
		return nil, fmt.Errorf("forgeapp.RevertToVersion: %w", err)
	}
	f.ActiveVersionID = newV.ID
	if err = s.repo.SaveForge(ctx, f); err != nil {
		return nil, fmt.Errorf("forgeapp.RevertToVersion: %w", err)
	}
	if err = s.trimVersions(ctx, forgeID); err != nil {
		return nil, err
	}

	// If the target version's env was evicted (its venv directory was
	// cleared by a prior trimEnvBuffer), rebuild it now so the next Run
	// sees a ready env. Failure surfaces via EnvStatus="failed" + EnvError
	// just like Create.
	//
	// 目标版本 env 若被驱逐（venv 目录被之前 trimEnvBuffer 清掉），现在
	// 重建让下次 Run 看到 ready。失败通过 EnvStatus="failed" + EnvError
	// 暴露，跟 Create 一致。
	if v.EnvStatus == forgedomain.EnvStatusEvicted {
		if err := s.SyncEnvForVersion(ctx, "", newV.ID); err != nil {
			s.log.Warn("forgeapp.RevertToVersion: rebuild env failed",
				zap.String("forge_id", forgeID),
				zap.String("version_id", newV.ID),
				zap.Error(err))
		}
	}
	s.trimEnvBuffer(ctx, forgeID)

	if err := s.attachActiveEnv(ctx, f); err != nil {
		s.log.Warn("forgeapp.RevertToVersion: attach active env",
			zap.String("forge_id", forgeID), zap.Error(err))
	}
	return f, nil
}

// ── Pending management ────────────────────────────────────────────────────────

// GetActivePending returns the pending ForgeVersion or ErrPendingNotFound.
//
// GetActivePending 返回 pending ForgeVersion，不存在时返回 ErrPendingNotFound。
func (s *Service) GetActivePending(ctx context.Context, forgeID string) (*forgedomain.ForgeVersion, error) {
	return s.repo.GetActivePending(ctx, forgeID)
}

// CreatePending checks for conflict, parses code if present, and saves a
// pending ForgeVersion. Called by edit_forge system tool.
//
// CreatePending 检查冲突，解析代码（如有），保存 pending ForgeVersion，
// 同步触发 venv 物化。由 create_forge / edit_forge system tool 调用。
//
// snap.Dependencies / PythonVersion 决定 EnvID 计算结果——传 nil/空则继承
// 活跃版本的 deps/python（草稿 forge 时按 stdlib-only 处理）。venv sync 失败
// 不让 CreatePending 整体失败：pending 已落库，EnvStatus="failed" + EnvError
// 含 uv 输出，调用方据此决定后续动作（按 §11.1 punt-to-AI）。
func (s *Service) CreatePending(ctx context.Context, forgeID string, snap PendingSnapshot) (*forgedomain.ForgeVersion, error) {
	f, err := s.repo.GetForge(ctx, forgeID)
	if err != nil {
		return nil, err
	}
	_, err = s.repo.GetActivePending(ctx, forgeID)
	if err == nil {
		return nil, forgedomain.ErrPendingConflict
	}
	if !errors.Is(err, forgedomain.ErrPendingNotFound) {
		return nil, fmt.Errorf("forgeapp.CreatePending: %w", err)
	}

	name := f.Name
	if snap.Name != "" {
		name = snap.Name
	}
	description := f.Description
	if snap.Description != "" {
		description = snap.Description
	}
	tags := f.Tags
	if snap.Tags != "" {
		tags = snap.Tags
	}
	code := f.Code
	params := f.Parameters
	returnSchema := f.ReturnSchema
	if snap.Code != "" {
		code = snap.Code
		parsed, err := s.parse(code)
		if err != nil {
			return nil, err
		}
		params = parsed.parametersJSON
		returnSchema = parsed.returnSchemaJSON
	}

	uid, _ := uidFromForge(f)
	pendingID := snap.ID
	if pendingID == "" {
		pendingID = newID("fv")
	}
	v := &forgedomain.ForgeVersion{
		ID:           pendingID,
		ForgeID:      forgeID,
		UserID:       uid,
		Status:       forgedomain.VersionStatusPending,
		Name:         name,
		Description:  description,
		Code:         code,
		Parameters:   params,
		ReturnSchema: returnSchema,
		Tags:         tags,
		ChangeReason: snap.ChangeReason,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	// Deps inheritance: snap-supplied wins; otherwise inherit from active
	// version (or stdlib-only when forge is still a draft with no active).
	//
	// deps 继承：snap 提供的优先；否则继承活跃版本（草稿 forge 无活跃时
	// 按 stdlib-only 处理）。
	deps := snap.Dependencies
	pythonVersion := snap.PythonVersion
	if snap.Dependencies == nil && snap.PythonVersion == "" && f.ActiveVersionID != "" {
		active, activeErr := s.repo.GetVersionByID(ctx, f.ActiveVersionID)
		if activeErr == nil {
			// Decode the JSON deps back into a slice for ComputeEnvID.
			// 解码 JSON deps 回切片供 ComputeEnvID。
			var inheritedDeps []string
			if active.Dependencies != "" {
				_ = json.Unmarshal([]byte(active.Dependencies), &inheritedDeps)
			}
			deps = inheritedDeps
			pythonVersion = active.PythonVersion
		}
	}
	s.fillEnvFields(v, deps, pythonVersion)

	if err = s.repo.SaveVersion(ctx, v); err != nil {
		return nil, fmt.Errorf("forgeapp.CreatePending: %w", err)
	}

	// Sync env synchronously — caller's tool_result reflects whether the
	// pending venv is ready. Failures populate EnvError on the pending row
	// for the LLM to read via subsequent attachActiveEnv on the forge.
	//
	// 同步等 sync——调用方 tool_result 反映 pending venv 是否就绪。
	// 失败时 EnvError 写到 pending 行，LLM 通过后续 attachActiveEnv 读到。
	if err := s.SyncEnvForVersion(ctx, "", v.ID); err != nil {
		s.log.Warn("forgeapp.CreatePending: sync env failed",
			zap.String("forge_id", forgeID),
			zap.String("version_id", v.ID),
			zap.Error(err))
	}
	// Re-read so caller gets the post-sync env state.
	// 重读让调用方拿到 sync 后的 env 状态。
	updated, err := s.repo.GetVersionByID(ctx, v.ID)
	if err != nil {
		return v, nil // fall back to in-memory if reread fails
	}
	return updated, nil
}

// AcceptPending promotes the active pending for forgeID to accepted and updates the forge.
//
// AcceptPending 将 forgeID 的 active pending 提升为 accepted，并更新 forge 主表。
//
// 守卫：仅 pending 的 EnvStatus="ready" 时允许 accept。其他状态返
// ErrEnvNotReady（pending/syncing）或 ErrEnvFailed（failed/evicted）——
// LLM 通过 forge entity-state 等待 ready 或 调 edit_forge / :resync 自救。
//
// 成功后：(1) ActiveVersionID 切到 pv.ID；(2) trimVersions 裁剪 accepted
// 版本上限；(3) trimEnvBuffer 驱逐超出 N=3 的旧 EnvID 目录。
func (s *Service) AcceptPending(ctx context.Context, forgeID string) (*forgedomain.Forge, error) {
	pv, err := s.repo.GetActivePending(ctx, forgeID)
	if err != nil {
		return nil, err
	}
	// Env state guard — accept only when the pending's venv is materialized
	// and ready. Any other state means the user / LLM should wait or
	// retry via edit_forge.
	//
	// env 状态守卫——仅 pending 的 venv 已物化且 ready 才允许 accept。
	// 其他状态意味着 user / LLM 应等或调 edit_forge 重试。
	switch pv.EnvStatus {
	case forgedomain.EnvStatusReady:
		// proceed
	case forgedomain.EnvStatusFailed:
		return nil, fmt.Errorf("%w: %s", forgedomain.ErrEnvFailed, pv.EnvError)
	default:
		// pending / syncing / evicted
		return nil, forgedomain.ErrEnvNotReady
	}

	f, err := s.repo.GetForge(ctx, forgeID)
	if err != nil {
		return nil, err
	}
	f.Name = pv.Name
	f.Description = pv.Description
	f.Code = pv.Code
	f.Parameters = pv.Parameters
	f.ReturnSchema = pv.ReturnSchema
	f.Tags = pv.Tags
	f.VersionCount++
	f.ActiveVersionID = pv.ID
	f.UpdatedAt = time.Now().UTC()

	if err = s.repo.UpdateVersionStatus(ctx, pv.ID, forgedomain.VersionStatusAccepted, &f.VersionCount); err != nil {
		return nil, fmt.Errorf("forgeapp.AcceptPending: %w", err)
	}
	if err = s.repo.SaveForge(ctx, f); err != nil {
		return nil, fmt.Errorf("forgeapp.AcceptPending: %w", err)
	}
	if err = s.trimVersions(ctx, forgeID); err != nil {
		return nil, err
	}
	s.trimEnvBuffer(ctx, forgeID)

	// Re-attach computed fields so caller sees the post-accept entity
	// (Pending=nil since it just got promoted; EnvStatus reflects the new
	// active version's state).
	//
	// 重新 attach 计算字段，让调用方拿到 accept 后的 entity（Pending=nil
	// 因刚被提升；EnvStatus 反映新活跃版本状态）。
	if err := s.attachActiveEnv(ctx, f); err != nil {
		s.log.Warn("forgeapp.AcceptPending: attach active env",
			zap.String("forge_id", forgeID), zap.Error(err))
	}
	return f, nil
}

// RejectPending marks the active pending for forgeID as rejected. When the
// forge has no accepted versions yet (ActiveVersionID empty) — i.e. this was
// the first pending of a freshly-created draft, produced by create_forge —
// the whole forge is deleted as well, since rejecting the only candidate
// code leaves an empty shell that has no business living in the user's
// library. For forges that already have an active version (edit_forge
// pending got rejected), the forge stays put with its prior active code.
//
// RejectPending 将 forgeID 的 active pending 标为 rejected。如果该 forge
// 还没有任何 accepted 版本（ActiveVersionID 为空）—— 即由 create_forge 生成
// 后用户拒绝其首份代码——整个 forge 一并删除，否则用户库里会留下一个无代码
// 的空壳。已有 active 版本的 forge（edit_forge 产生的 pending 被拒）保留。
func (s *Service) RejectPending(ctx context.Context, forgeID string) error {
	pv, err := s.repo.GetActivePending(ctx, forgeID)
	if err != nil {
		return err
	}
	if err = s.repo.UpdateVersionStatus(ctx, pv.ID, forgedomain.VersionStatusRejected, nil); err != nil {
		return fmt.Errorf("forgeapp.RejectPending: %w", err)
	}

	f, err := s.repo.GetForge(ctx, forgeID)
	if err != nil {
		return fmt.Errorf("forgeapp.RejectPending: re-read forge: %w", err)
	}
	if f.ActiveVersionID == "" {
		if err := s.Delete(ctx, forgeID); err != nil {
			return fmt.Errorf("forgeapp.RejectPending: cleanup draft: %w", err)
		}
	}
	return nil
}

// ── Execution ─────────────────────────────────────────────────────────────────

// RunForge executes the forge's current code in the sandbox and records an
// execution row. Chat-context (conversation/message/toolCallID) is read from
// ctx via reqctxpkg; if present, the row is tagged TriggeredByChat, otherwise
// TriggeredByHTTP. input must already have att_ids resolved by the caller.
//
// RunForge 在沙箱中执行 forge 当前代码并记录一行执行历史。chat 上下文
// （conversation/message/toolCallID）从 ctx 通过 reqctxpkg 读取；存在则标
// TriggeredByChat，否则 TriggeredByHTTP。input 中的 att_id 必须由调用方
// 预先解析为真实路径。
func (s *Service) RunForge(ctx context.Context, forgeID string, input map[string]any) (*forgedomain.ExecutionResult, error) {
	f, err := s.repo.GetForge(ctx, forgeID)
	if err != nil {
		return nil, err
	}
	// Draft forges (ActiveVersionID empty, no version accepted yet) can't
	// be run — the LLM should accept the pending first or call edit_forge
	// to fix any failed sync.
	//
	// 草稿 forge（ActiveVersionID 空，无 accept 版本）不能跑——LLM 应先
	// accept pending 或调 edit_forge 修复失败的 sync。
	if f.ActiveVersionID == "" {
		return nil, forgedomain.ErrEnvNotReady
	}
	av, err := s.repo.GetVersionByID(ctx, f.ActiveVersionID)
	if err != nil {
		return nil, fmt.Errorf("forgeapp.RunForge: %w", err)
	}
	result, err := s.sandbox.Run(ctx, RunRequest{
		ForgeID:   forgeID,
		VersionID: av.ID,
		EnvID:     av.EnvID,
		Code:      f.Code,
		Input:     input,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", forgedomain.ErrRunFailed, err)
	}
	if _, err := s.recordExecution(ctx, f, forgedomain.ExecutionKindRun, input, result, "", "", nil); err != nil {
		return nil, err
	}
	return result, nil
}

// RunTestCase executes a single test case and records an execution row.
// batchID is empty for individual runs. Returns the persisted row so callers
// can present the result without re-querying.
//
// RunTestCase 执行单条测试用例并记录一行执行历史。单跑时 batchID 为空。
// 返回已落库的行，调用方无需再查。
func (s *Service) RunTestCase(ctx context.Context, testCaseID, batchID string) (*forgedomain.ForgeExecution, error) {
	tc, err := s.repo.GetTestCase(ctx, testCaseID)
	if err != nil {
		return nil, err
	}
	f, err := s.repo.GetForge(ctx, tc.ForgeID)
	if err != nil {
		return nil, err
	}
	// InputData is enforced to be valid JSON object at create time
	// (forge_test_cases stores it as parsed JSON). Failing parse here means
	// the row was corrupted — surface it loudly so test results don't lie.
	//
	// InputData 在创建测试用例时已校验为合法 JSON 对象。这里解析失败说明数据
	// 损坏——必须显式报错，避免空 input 跑出假结果。
	var input map[string]any
	if err := json.Unmarshal([]byte(tc.InputData), &input); err != nil {
		return nil, fmt.Errorf("forgeapp.RunTestCase: corrupted test case %q input_data: %w", testCaseID, err)
	}

	// Test runs go through the active version's venv. Draft forges (no
	// accepted version yet) can't run tests — same rule as RunForge.
	//
	// 测试也走活跃版本的 venv。草稿 forge（无 accept 版本）不能跑测试——
	// 跟 RunForge 同规则。
	if f.ActiveVersionID == "" {
		return nil, forgedomain.ErrEnvNotReady
	}
	av, err := s.repo.GetVersionByID(ctx, f.ActiveVersionID)
	if err != nil {
		return nil, fmt.Errorf("forgeapp.RunTestCase: %w", err)
	}
	result, sandboxErr := s.sandbox.Run(ctx, RunRequest{
		ForgeID:   f.ID,
		VersionID: av.ID,
		EnvID:     av.EnvID,
		Code:      f.Code,
		Input:     input,
	})
	if sandboxErr != nil {
		return nil, fmt.Errorf("%w: %v", forgedomain.ErrRunFailed, sandboxErr)
	}

	var pass *bool
	if tc.ExpectedOutput != "" && result.OK {
		actual, _ := json.Marshal(result.Output)
		p := strings.TrimSpace(string(actual)) == strings.TrimSpace(tc.ExpectedOutput)
		pass = &p
	}

	return s.recordExecution(ctx, f, forgedomain.ExecutionKindTest, input, result, testCaseID, batchID, pass)
}

// RunAllTests runs all test cases for a forge under a shared batch ID.
//
// RunAllTests 使用共享 batchID 运行 forge 的全部测试用例。
func (s *Service) RunAllTests(ctx context.Context, forgeID string) ([]*forgedomain.ForgeExecution, error) {
	cases, err := s.repo.ListTestCases(ctx, forgeID)
	if err != nil {
		return nil, err
	}
	batchID := newID("b")
	results := make([]*forgedomain.ForgeExecution, 0, len(cases))
	for _, tc := range cases {
		r, err := s.RunTestCase(ctx, tc.ID, batchID)
		if err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

// recordExecution serialises the input/output, fills chat context from ctx,
// inserts a ForgeExecution row, and trims oldest if MaxExecutionsPerForge is
// exceeded. Returns (row, nil) on full success; (nil, err) if the row didn't
// persist — callers MUST NOT treat the in-memory entity as if saved.
// Retention failures are logged but don't fail the call (table can grow a
// few rows over the cap until the next successful run trims it).
//
// recordExecution 序列化 input/output，从 ctx 填 chat 上下文，插入 ForgeExecution
// 行，并在超过 MaxExecutionsPerForge 时裁剪最旧记录。完全成功返 (row, nil)；
// 写库失败返 (nil, err)——调用方**不能**把内存 entity 当作已落库使用。
// 裁剪失败仅记日志，不让本次调用整体失败（最多让表暂时多几行，下次成功时会修剪）。
func (s *Service) recordExecution(
	ctx context.Context,
	f *forgedomain.Forge,
	kind string,
	input map[string]any,
	result *forgedomain.ExecutionResult,
	testCaseID, batchID string,
	pass *bool,
) (*forgedomain.ForgeExecution, error) {
	inputJSON, _ := json.Marshal(input)
	outputJSON := ""
	if result.Output != nil {
		if b, e := json.Marshal(result.Output); e == nil {
			outputJSON = string(b)
		}
	}
	uid, _ := uidFromForge(f)

	convID, _ := reqctxpkg.GetConversationID(ctx)
	msgID, _ := reqctxpkg.GetMessageID(ctx)
	toolCallID, _ := reqctxpkg.GetToolCallID(ctx)
	triggeredBy := forgedomain.TriggeredByHTTP
	if convID != "" {
		triggeredBy = forgedomain.TriggeredByChat
	}

	e := &forgedomain.ForgeExecution{
		ID:             newID("fe"),
		ForgeID:        f.ID,
		UserID:         uid,
		ForgeVersion:   f.VersionCount,
		Kind:           kind,
		Input:          string(inputJSON),
		Output:         outputJSON,
		OK:             result.OK,
		ErrorMsg:       result.ErrorMsg,
		ElapsedMs:      result.ElapsedMs,
		TestCaseID:     testCaseID,
		BatchID:        batchID,
		Pass:           pass,
		TriggeredBy:    triggeredBy,
		ConversationID: convID,
		MessageID:      msgID,
		ToolCallID:     toolCallID,
		CreatedAt:      time.Now().UTC(),
	}
	if err := s.repo.SaveExecution(ctx, e); err != nil {
		return nil, fmt.Errorf("forgeapp.recordExecution: save: %w", err)
	}
	n, err := s.repo.CountExecutions(ctx, f.ID)
	if err != nil {
		s.log.Warn("recordExecution: retention count failed; table may grow over cap",
			zap.String("forge_id", f.ID), zap.Error(err))
		return e, nil
	}
	if n > forgedomain.MaxExecutionsPerForge {
		if err := s.repo.DeleteOldestExecution(ctx, f.ID); err != nil {
			s.log.Warn("recordExecution: retention delete failed; table over cap",
				zap.String("forge_id", f.ID), zap.Int64("count", n), zap.Error(err))
		}
	}
	return e, nil
}

// ── Test cases ────────────────────────────────────────────────────────────────

// CreateTestCase adds a test case to a forge.
//
// CreateTestCase 为 forge 添加测试用例。
func (s *Service) CreateTestCase(ctx context.Context, forgeID string, in TestCaseInput) (*forgedomain.ForgeTestCase, error) {
	f, err := s.repo.GetForge(ctx, forgeID)
	if err != nil {
		return nil, err
	}
	uid, _ := uidFromForge(f)
	tc := &forgedomain.ForgeTestCase{
		ID:             newID("tc"),
		ForgeID:        forgeID,
		UserID:         uid,
		Name:           in.Name,
		InputData:      in.InputData,
		ExpectedOutput: in.ExpectedOutput,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	if err = s.repo.SaveTestCase(ctx, tc); err != nil {
		return nil, fmt.Errorf("forgeapp.CreateTestCase: %w", err)
	}
	return tc, nil
}

// ListTestCases returns all test cases for a forge.
//
// ListTestCases 返回 forge 所有测试用例。
func (s *Service) ListTestCases(ctx context.Context, forgeID string) ([]*forgedomain.ForgeTestCase, error) {
	return s.repo.ListTestCases(ctx, forgeID)
}

// DeleteTestCase hard-deletes a test case.
//
// DeleteTestCase 硬删除测试用例。
func (s *Service) DeleteTestCase(ctx context.Context, id string) error {
	return s.repo.DeleteTestCase(ctx, id)
}

// GenerateTestCases asks the LLM to generate test cases and returns them
// as a single batch. The LLM call is non-streaming, so any "streaming" of
// individual cases would be cosmetic—plain JSON keeps the contract simple.
//
// GenerateTestCases 请求 LLM 一次性生成测试用例并整批返回。
// LLM 调用本身是非流式的，逐条"流式推送"只是化妆——直接返回 JSON 更清晰。
func (s *Service) GenerateTestCases(ctx context.Context, forgeID string, count int) (*GenerateResult, error) {
	f, err := s.repo.GetForge(ctx, forgeID)
	if err != nil {
		return nil, err
	}
	prompt := buildGeneratePrompt(f, count)
	raw, err := s.llm.Generate(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("forgeapp.GenerateTestCases: llm: %w", err)
	}
	jsonRaw, ok := llmparsepkg.ExtractJSON(raw)
	if !ok {
		return nil, fmt.Errorf("forgeapp.GenerateTestCases: LLM response contained no JSON: %q", raw)
	}
	var resp struct {
		NotSupported bool   `json:"not_supported"`
		Reason       string `json:"reason"`
		TestCases    []struct {
			Name           string          `json:"name"`
			Input          json.RawMessage `json:"input"`
			ExpectedOutput json.RawMessage `json:"expected_output"`
		} `json:"test_cases"`
	}
	if err = json.Unmarshal([]byte(jsonRaw), &resp); err != nil {
		return nil, fmt.Errorf("forgeapp.GenerateTestCases: parse response: %w", err)
	}
	if resp.NotSupported {
		return &GenerateResult{NotSupported: true, Reason: resp.Reason}, nil
	}
	uid, _ := uidFromForge(f)
	saved := make([]*forgedomain.ForgeTestCase, 0, len(resp.TestCases))
	for _, tc := range resp.TestCases {
		item := &forgedomain.ForgeTestCase{
			ID:             newID("tc"),
			ForgeID:        forgeID,
			UserID:         uid,
			Name:           tc.Name,
			InputData:      string(tc.Input),
			ExpectedOutput: string(tc.ExpectedOutput),
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
		}
		if err = s.repo.SaveTestCase(ctx, item); err != nil {
			return nil, fmt.Errorf("forgeapp.GenerateTestCases: save: %w", err)
		}
		saved = append(saved, item)
	}
	return &GenerateResult{TestCases: saved}, nil
}

// ── Import / Export ───────────────────────────────────────────────────────────

// exportShape is the JSON shape for forge export/import.
//
// exportShape 是 forge 导入/导出的 JSON 形状。
type exportShape struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Code        string          `json:"code"`
	Tags        []string        `json:"tags"`
	TestCases   []TestCaseInput `json:"testCases"`
}

// Export serialises a forge and its test cases to JSON.
//
// Export 把 forge 及测试用例序列化为 JSON。
func (s *Service) Export(ctx context.Context, forgeID string) ([]byte, error) {
	f, err := s.repo.GetForge(ctx, forgeID)
	if err != nil {
		return nil, err
	}
	cases, err := s.repo.ListTestCases(ctx, forgeID)
	if err != nil {
		s.log.Warn("export: failed to list test cases, exporting without them",
			zap.String("forge_id", forgeID), zap.Error(err))
		cases = nil
	}
	var tags []string
	if err := json.Unmarshal([]byte(f.Tags), &tags); err != nil {
		s.log.Warn("export: malformed tags JSON, exporting empty tags",
			zap.String("forge_id", forgeID), zap.Error(err))
		tags = nil
	}
	tcInputs := make([]TestCaseInput, len(cases))
	for i, tc := range cases {
		tcInputs[i] = TestCaseInput{Name: tc.Name, InputData: tc.InputData, ExpectedOutput: tc.ExpectedOutput}
	}
	return json.Marshal(exportShape{
		Name: f.Name, Description: f.Description, Code: f.Code,
		Tags: tags, TestCases: tcInputs,
	})
}

// Import creates a new forge from exported JSON, including test cases.
//
// Import 从导出的 JSON 新建 forge，包含测试用例。
func (s *Service) Import(ctx context.Context, data []byte) (*forgedomain.Forge, error) {
	var shape exportShape
	if err := json.Unmarshal(data, &shape); err != nil || shape.Name == "" || shape.Code == "" {
		return nil, forgedomain.ErrImportInvalid
	}
	f, err := s.Create(ctx, CreateInput{
		Name: shape.Name, Description: shape.Description,
		Code: shape.Code, Tags: shape.Tags,
	})
	if err != nil {
		return nil, err
	}
	for _, tc := range shape.TestCases {
		if _, err := s.CreateTestCase(ctx, f.ID, tc); err != nil {
			s.log.Warn("import: skipped test case",
				zap.String("forge_id", f.ID),
				zap.String("test_case_name", tc.Name),
				zap.Error(err),
			)
		}
	}
	return f, nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

type parsedFields struct {
	parametersJSON   string
	returnSchemaJSON string
}

// ParseCode validates that code is parseable as a single-function Python forge.
// Returns forgedomain.ErrASTParseError if AST parsing fails. Used by callers
// (e.g. CreateForge system tool) to dry-run validation before calling Create()
// which also does storage I/O — keeps the error path simple and fast.
//
// ParseCode 验证 code 是否可解析为单函数 Python forge。AST 解析失败返
// forgedomain.ErrASTParseError。供调用方（如 CreateForge 系统工具）在调用
// Create()（含存储 I/O）前先做 dry-run 验证——错误路径简单且快。
func (s *Service) ParseCode(code string) error {
	_, err := s.parse(code)
	return err
}

func (s *Service) parse(code string) (parsedFields, error) {
	pythonPath := ""
	if s.sandbox != nil {
		pythonPath = s.sandbox.PythonPath()
	}
	p, err := parseForgeCode(pythonPath, code)
	if err != nil {
		// Preserve the underlying cause (e.g. "fork/exec /...: no such file or
		// directory" when sandbox isn't bootstrapped, or the actual Python
		// SyntaxError) wrapped with the sentinel. Without this the LLM only
		// sees "AST parse failed" and loops regenerating valid code that
		// the missing interpreter can never run. errors.Is(err, ErrASTParseError)
		// still works.
		//
		// 保留底层错因（如 sandbox 没 bootstrap 时的 fork/exec ...: no such
		// file or directory，或真实的 Python SyntaxError），用 sentinel 包装。
		// 不保留的话 LLM 只看到 "AST parse failed"，会循环重写正确的代码而
		// 永远跑不通缺失的解释器。errors.Is(err, ErrASTParseError) 仍然有效。
		return parsedFields{}, fmt.Errorf("%w: %v", forgedomain.ErrASTParseError, err)
	}
	params := make([]map[string]any, len(p.Parameters))
	for i, pp := range p.Parameters {
		m := map[string]any{
			"name": pp.Name, "type": pp.Type,
			"required": pp.Required, "description": pp.Description,
		}
		if pp.Default != nil {
			m["default"] = *pp.Default
		} else {
			m["default"] = nil
		}
		params[i] = m
	}
	pb, _ := json.Marshal(params)
	rb, _ := json.Marshal(map[string]string{"type": p.Return.Type, "description": p.Return.Description})
	return parsedFields{parametersJSON: string(pb), returnSchemaJSON: string(rb)}, nil
}

func (s *Service) autoRejectPending(ctx context.Context, forgeID string) error {
	v, err := s.repo.GetActivePending(ctx, forgeID)
	if errors.Is(err, forgedomain.ErrPendingNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("forgeapp.autoRejectPending: %w", err)
	}
	return s.repo.UpdateVersionStatus(ctx, v.ID, forgedomain.VersionStatusRejected, nil)
}

// attachPending populates f.Pending from the pending row. ErrPendingNotFound
// is treated as a normal "no pending" state — f.Pending stays nil and nil
// error is returned. Any other error is propagated so the caller can
// distinguish "no pending" from "DB problem" (an earlier silent-fallback
// version of this helper made GET responses lie when SQLite hiccupped).
//
// attachPending 填充 f.Pending。ErrPendingNotFound 视为正常的"无 pending"
// 状态——f.Pending 保持 nil 并返 nil error。其他错误向上传播，调用方据此
// 区分"无 pending"和"DB 故障"——之前静默 fallback 版本会让 GET 响应在
// SQLite 抖动时撒谎。
func (s *Service) attachPending(ctx context.Context, f *forgedomain.Forge) error {
	pv, err := s.repo.GetActivePending(ctx, f.ID)
	if err == nil {
		f.Pending = pv
		return nil
	}
	if errors.Is(err, forgedomain.ErrPendingNotFound) {
		return nil
	}
	return fmt.Errorf("forgeapp.attachPending: %w", err)
}

// attachActiveEnv copies the active version's env state onto f's Env*
// computed fields. No-op (returns nil) when ActiveVersionID is empty
// (draft forge). Mirrors the attachPending pattern; same convention that
// callers (Get / List / GetDetail) call this before serialization so
// HTTP and SSE consumers see identical entity shape.
//
// Inconsistent state (ActiveVersionID points at a missing version) is
// logged and treated as "no env data" rather than failing the whole
// load — matches attachPending's tolerant policy.
//
// attachActiveEnv 把活跃版本的 env 状态拷到 f 的 Env* 计算字段。
// ActiveVersionID 为空（草稿 forge）时静默 no-op。镜像 attachPending
// 模式；约定调用方（Get / List / GetDetail）在序列化前调，HTTP 和 SSE
// 消费者拿到一致的 entity 形状。
//
// 不一致状态（ActiveVersionID 指向已不存在的版本）记日志并当"无 env 数据"
// 处理，不让整个 load 失败——同 attachPending 的宽容策略。
func (s *Service) attachActiveEnv(ctx context.Context, f *forgedomain.Forge) error {
	if f == nil || f.ActiveVersionID == "" {
		return nil
	}
	av, err := s.repo.GetVersionByID(ctx, f.ActiveVersionID)
	if err != nil {
		if errors.Is(err, forgedomain.ErrVersionNotFound) {
			s.log.Warn("attachActiveEnv: active version not found",
				zap.String("forge_id", f.ID),
				zap.String("active_version_id", f.ActiveVersionID))
			return nil
		}
		return fmt.Errorf("forgeapp.attachActiveEnv: %w", err)
	}
	f.EnvStatus = av.EnvStatus
	f.EnvError = av.EnvError
	f.EnvSyncedAt = av.EnvSyncedAt
	f.EnvSyncStage = av.EnvSyncStage
	f.EnvSyncDetail = av.EnvSyncDetail
	return nil
}

// publishForgeAfterChange re-reads the forge fresh, attaches Pending and
// active-env computed fields, and pushes the full snapshot via
// PublishSnapshot. Centralised because SyncEnvForVersion fires several
// publish points (markSyncing → progress → markReady/Failed) and each
// must surface the latest entity state.
//
// No-op if bridge is unset or convID is empty (HTTP-driven flow without
// chat context).
//
// publishForgeAfterChange 重读 forge，attach Pending 和活跃 env 计算字段，
// 通过 PublishSnapshot 推完整快照。集中管理——SyncEnvForVersion 有多个
// publish 点（markSyncing → progress → markReady/Failed），每次都要反映
// 最新 entity 状态。
//
// bridge 未设或 convID 空（HTTP 驱动流程无 chat 上下文）时静默 no-op。
func (s *Service) publishForgeAfterChange(ctx context.Context, convID, forgeID string) {
	if s.bridge == nil || convID == "" {
		return
	}
	f, err := s.repo.GetForge(ctx, forgeID)
	if err != nil {
		s.log.Warn("publishForgeAfterChange: get forge",
			zap.String("forge_id", forgeID), zap.Error(err))
		return
	}
	_ = s.attachPending(ctx, f)
	_ = s.attachActiveEnv(ctx, f)
	s.PublishSnapshot(ctx, convID, f)
}

// SyncEnvForVersion materializes the venv for the given version's EnvID
// synchronously. Drives EnvStatus through the lifecycle (pending →
// syncing → ready/failed), pipes uv stderr stages into EnvSyncStage /
// EnvSyncDetail via UpdateVersionEnvProgress, and PublishSnapshots after
// every state mutation when convID is set. On uv failure, captures the
// full stderr into EnvError and returns ErrEnvFailed wrapping the
// captured stderr text — the LLM tool result includes the diagnostic.
//
// Caller passes convID="" for HTTP-driven syncs (no chat snapshot push).
//
// SyncEnvForVersion 同步物化指定版本 EnvID 对应的 venv。状态机驱动 EnvStatus
// （pending → syncing → ready/failed），通过 UpdateVersionEnvProgress 把 uv
// stderr stage 接到 EnvSyncStage / EnvSyncDetail，每次状态变更后
// PublishSnapshot（convID 非空时）。uv 失败时 stderr 进 EnvError，返
// ErrEnvFailed wrap 捕获的 stderr 文本——LLM tool result 含诊断。
//
// HTTP 驱动 sync 调用方传 convID="" （不推 chat 快照）。
func (s *Service) SyncEnvForVersion(ctx context.Context, convID, versionID string) error {
	v, err := s.repo.GetVersionByID(ctx, versionID)
	if err != nil {
		return fmt.Errorf("forgeapp.SyncEnvForVersion: %w", err)
	}

	// Decode stored deps JSON into []string. Malformed deps abort sync
	// loud — the LLM should not have stored bad JSON in the first place,
	// but if it did, fail with a clear error instead of silently passing
	// nil deps to uv (which would yield a working but wrong venv).
	//
	// 解码存的 deps JSON 成 []string。畸形 deps 直接 fail loud——LLM 本
	// 不该存非法 JSON；万一存了，明确报错，而非静默给 uv 传 nil 跑出错的 venv。
	var deps []string
	if v.Dependencies != "" {
		if err := json.Unmarshal([]byte(v.Dependencies), &deps); err != nil {
			errMsg := fmt.Sprintf("malformed dependencies JSON: %v (raw=%q)", err, v.Dependencies)
			_ = s.repo.UpdateVersionEnvStatus(ctx, versionID, forgedomain.EnvStatusFailed, errMsg)
			s.publishForgeAfterChange(ctx, convID, v.ForgeID)
			return fmt.Errorf("%w: %s", forgedomain.ErrEnvFailed, errMsg)
		}
	}

	// markSyncing.
	if err := s.repo.UpdateVersionEnvStatus(ctx, versionID, forgedomain.EnvStatusSyncing, ""); err != nil {
		return fmt.Errorf("forgeapp.SyncEnvForVersion: %w", err)
	}
	s.publishForgeAfterChange(ctx, convID, v.ForgeID)

	syncErr := s.sandbox.Sync(ctx, SyncRequest{
		ForgeID:       v.ForgeID,
		VersionID:     v.ID,
		EnvID:         v.EnvID,
		Dependencies:  deps,
		PythonVersion: v.PythonVersion,
		OnProgress: func(stage, detail string) {
			// Best-effort progress write — drop errors so a transient DB
			// hiccup doesn't fail the whole sync.
			//
			// 尽力写进度——吞错避免短暂 DB 抖动让整个 sync 失败。
			_ = s.repo.UpdateVersionEnvProgress(ctx, versionID, stage, detail)
			s.publishForgeAfterChange(ctx, convID, v.ForgeID)
		},
	})

	if syncErr != nil {
		stderr := syncErr.Error()
		var se *SyncError
		if errors.As(syncErr, &se) {
			stderr = se.Stderr
		}
		_ = s.repo.UpdateVersionEnvStatus(ctx, versionID, forgedomain.EnvStatusFailed, stderr)
		s.publishForgeAfterChange(ctx, convID, v.ForgeID)
		return fmt.Errorf("%w: %s", forgedomain.ErrEnvFailed, stderr)
	}

	// markReady.
	if err := s.repo.UpdateVersionEnvStatus(ctx, versionID, forgedomain.EnvStatusReady, ""); err != nil {
		return fmt.Errorf("forgeapp.SyncEnvForVersion: %w", err)
	}
	s.publishForgeAfterChange(ctx, convID, v.ForgeID)
	return nil
}

func (s *Service) trimVersions(ctx context.Context, forgeID string) error {
	n, err := s.repo.CountAcceptedVersions(ctx, forgeID)
	if err != nil {
		return fmt.Errorf("forgeapp.trimVersions: %w", err)
	}
	if n > forgedomain.MaxAcceptedVersions {
		return s.repo.DeleteOldestAcceptedVersion(ctx, forgeID)
	}
	return nil
}

// trimEnvBuffer evicts the LRU EnvIDs beyond MaxEnvIDsPerForge for a forge,
// removing each victim's on-disk venv directory. Called after AcceptPending
// and RevertToVersion — both of which can grow the distinct-EnvID count.
//
// ForgeVersion rows that referenced an evicted EnvID are NOT updated to
// EnvStatus="evicted" here — that's intentional. The next Run hitting an
// evicted version surfaces uv's "no virtual environment" error directly to
// the LLM, which then calls :resync to rebuild. This is the documented
// punt-to-AI path (sandbox iteration §11.1) — eager DB updates would just
// add complexity for no user benefit.
//
// Errors are logged, never returned: the caller (AcceptPending / Revert)
// has already committed the lifecycle change; a botched env eviction at
// most leaves stale venv dirs around until the next trim, which is
// harmless.
//
// trimEnvBuffer 驱逐 forge 中超过 MaxEnvIDsPerForge 的 LRU EnvID，删每个
// 被驱逐 EnvID 的 venv 目录。AcceptPending / RevertToVersion 后调——这俩
// 都可能让不同 EnvID 数增长。
//
// 引用被驱逐 EnvID 的 ForgeVersion 行**不**主动改 EnvStatus="evicted"——
// 这是有意的。下次 Run 命中 evicted 版本时，uv 自然报"no virtual
// environment"错误透传给 LLM，LLM 调 :resync 重建。这是文档定的
// punt-to-AI 路径（沙箱迭代 §11.1）——主动 DB 更新只增加复杂度无收益。
//
// 错误仅 log 不返：调用方（AcceptPending / Revert）已提交生命周期变更；
// 驱逐失败最多留下旧 venv 目录到下次 trim，无害。
func (s *Service) trimEnvBuffer(ctx context.Context, forgeID string) {
	envIDs, err := s.repo.ListEnvIDsForForge(ctx, forgeID)
	if err != nil {
		s.log.Warn("forgeapp.trimEnvBuffer: list env ids",
			zap.String("forge_id", forgeID), zap.Error(err))
		return
	}
	if len(envIDs) <= forgedomain.MaxEnvIDsPerForge {
		return
	}
	for _, evictID := range envIDs[forgedomain.MaxEnvIDsPerForge:] {
		if err := s.sandbox.DestroyEnv(ctx, forgeID, evictID); err != nil {
			s.log.Warn("forgeapp.trimEnvBuffer: destroy env",
				zap.String("forge_id", forgeID),
				zap.String("env_id", evictID),
				zap.Error(err))
		}
	}
}

func newVersion(f *forgedomain.Forge, status string, version *int, changeReason string) *forgedomain.ForgeVersion {
	now := time.Now().UTC()
	return &forgedomain.ForgeVersion{
		ID:           newID("fv"),
		ForgeID:      f.ID,
		UserID:       f.UserID,
		Version:      version,
		Status:       status,
		Name:         f.Name,
		Description:  f.Description,
		Code:         f.Code,
		Parameters:   f.Parameters,
		ReturnSchema: f.ReturnSchema,
		Tags:         f.Tags,
		ChangeReason: changeReason,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// NewForgeID returns a fresh forge entity ID ("f_<16hex>"). Exposed so
// callers (e.g. the create_forge system tool) can pre-allocate an ID for
// stable identity across streaming snapshots before the entity is persisted.
//
// NewForgeID 返回新的 forge 主键 ID（"f_<16hex>"）。导出供调用方
// （如 create_forge 系统工具）在落库前预分配，使流式快照身份稳定。
func NewForgeID() string { return newID("f") }

// NewVersionID returns a fresh forge_version entity ID ("fv_<16hex>").
// Same use-case as NewForgeID but for the pending row in edit_forge.
//
// NewVersionID 返回新的 forge_version 主键 ID（"fv_<16hex>"）。
// 用途同 NewForgeID，但用于 edit_forge 的 pending 行。
func NewVersionID() string { return newID("fv") }

func newID(prefix string) string { return idgenpkg.New(prefix) }

func tagsJSON(tags []string) string {
	if tags == nil {
		tags = []string{}
	}
	b, _ := json.Marshal(tags)
	return string(b)
}

// depsJSON serializes a deps slice into the JSON form stored in
// ForgeVersion.Dependencies. Always emits "[]" for nil/empty so the column
// stays valid JSON.
//
// depsJSON 把 deps 切片序列化为 ForgeVersion.Dependencies 存的 JSON。
// nil/empty 总是输出 "[]" 以保持列合法 JSON。
func depsJSON(deps []string) string {
	if deps == nil {
		deps = []string{}
	}
	b, _ := json.Marshal(deps)
	return string(b)
}

// fillEnvFields populates a fresh ForgeVersion's deps + EnvID + EnvStatus
// fields. EnvStatus starts at pending — SyncEnvForVersion drives it forward.
// pythonVersion="" is preserved verbatim (the version row records the user's
// declared spec, not the resolved default); ComputeEnvID treats empty string
// as part of the hash domain so version=""/version=">=3.12" produce different
// EnvIDs (correctly, since the resolved venvs would differ).
//
// fillEnvFields 填充新建 ForgeVersion 的 deps + EnvID + EnvStatus 字段。
// EnvStatus 起始 pending——SyncEnvForVersion 推进。pythonVersion=""
// 原样保留（version 行记的是用户声明的 spec，不是解析后的默认）；
// ComputeEnvID 把空字符串当作 hash domain 一部分——version=""
// 跟 version=">=3.12" 得到不同 EnvID（正确，因为解析出的 venv 会不同）。
func (s *Service) fillEnvFields(v *forgedomain.ForgeVersion, deps []string, pythonVersion string) {
	v.Dependencies = depsJSON(deps)
	v.PythonVersion = pythonVersion
	v.EnvID = ComputeEnvID(deps, pythonVersion)
	v.EnvStatus = forgedomain.EnvStatusPending
}

func mustSetUserID(ctx context.Context, f *forgedomain.Forge) error {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return fmt.Errorf("forgeapp: %w", err)
	}
	f.UserID = uid
	return nil
}

func uidFromForge(f *forgedomain.Forge) (string, bool) {
	return f.UserID, f.UserID != ""
}

func buildGeneratePrompt(f *forgedomain.Forge, count int) string {
	return fmt.Sprintf(`Analyze this Python function and generate test cases.

Function name: %s
Description: %s
Code:
%s

If the function depends on external state (file paths, network, randomness, side effects),
respond with: {"not_supported": true, "reason": "<explanation>"}

Otherwise, generate %d diverse test cases and respond with:
{"test_cases": [{"name": "<name>", "input": <json_object>, "expected_output": <json_value>}, ...]}

Respond with valid JSON only, no explanation.`,
		f.Name, f.Description, f.Code, count)
}
