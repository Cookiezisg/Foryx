// Package workspace owns the workspace CRUD service — the local isolation root's
// lifecycle. It validates names, guards the last workspace, and answers the auth
// middleware's WorkspaceResolver port (Validate).
//
// Package workspace 持有 workspace CRUD service——本地隔离根的生命周期。校验名字、守最后一个
// workspace、应答 auth 中间件的 WorkspaceResolver 端口（Validate）。
package workspace

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"

	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	websearchdomain "github.com/sunweilin/forgify/backend/internal/domain/websearch"
	workspacedomain "github.com/sunweilin/forgify/backend/internal/domain/workspace"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Service orchestrates Workspace CRUD.
//
// Service 编排 Workspace CRUD。
type Service struct {
	repo workspacedomain.Repository
	log  *zap.Logger

	// reaper tears down a workspace's runtime + files before the row is deleted (review PD-1:
	// cascade destroy — kill automation, stop resident processes, remove the file tree).
	// Injected post-build by bootstrap (it alone sees every service); nil → row-delete only.
	//
	// reaper 在删行前拆掉 workspace 的运行时与文件（评审 PD-1：级联销毁——杀自动化、停常驻
	// 进程、删文件树）。bootstrap 后注入（只有它看得到全部 service）；nil → 仅删行。
	reaper Reaper
}

// Reaper destroys everything a workspace owns beyond its row: in-flight runs, trigger
// listeners, resident handler/mcp processes, and the on-disk file tree. Best-effort by
// contract — a partially failed reap still proceeds to the row delete (the row's absence
// is what makes the data unreachable and the background seeding skip it).
//
// Reaper 销毁 workspace 行之外的一切所有物：在途 run、trigger 监听、常驻 handler/mcp 进程、
// 盘上文件树。契约上 best-effort——部分失败仍继续删行（行的消失才是数据不可达、后台播种
// 跳过它的根因）。
type Reaper func(ctx context.Context, workspaceID string)

// SetReaper injects the cascade-destroy hook (bootstrap, post-build).
//
// SetReaper 注入级联销毁钩子（bootstrap 后注入）。
func (s *Service) SetReaper(r Reaper) { s.reaper = r }

// NewService wires dependencies; panics on nil logger.
//
// NewService 装配依赖；nil logger panic。
func NewService(repo workspacedomain.Repository, log *zap.Logger) *Service {
	if log == nil {
		panic("workspace.NewService: logger is nil")
	}
	return &Service{repo: repo, log: log.Named("workspaceapp")}
}

// CreateInput is the validated payload for Create.
//
// CreateInput 是 Create 的校验载荷。
type CreateInput struct {
	Name        string
	AvatarColor string
	Language    string // optional; defaults to zh-CN
}

// UpdateInput is the partial-update payload; nil fields are skipped.
//
// UpdateInput 是部分更新载荷；nil 字段跳过。
type UpdateInput struct {
	Name         *string
	AvatarColor  *string
	Language     *string
	WebFetchMode *string // "local" | "jina" (PD-4 C)
}

// Create makes a new workspace; name is required and length-bounded, language
// defaults to zh-CN. A duplicate name surfaces ErrNameConflict from the store.
//
// Create 创建新 workspace；name 必填限长，language 默认 zh-CN。重名由 store 冒泡 ErrNameConflict。
func (s *Service) Create(ctx context.Context, in CreateInput) (*workspacedomain.Workspace, error) {
	name, err := cleanName(in.Name)
	if err != nil {
		return nil, err
	}
	lang, err := resolveLanguage(in.Language)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	w := &workspacedomain.Workspace{
		ID:          idgenpkg.New("ws"),
		Name:        name,
		AvatarColor: strings.TrimSpace(in.AvatarColor),
		Language:    lang,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.repo.Save(ctx, w); err != nil {
		return nil, err
	}
	s.log.Info("workspace created", zap.String("workspace_id", w.ID), zap.String("name", w.Name))
	return w, nil
}

// Get returns one workspace by id.
//
// Get 按 id 取 workspace。
func (s *Service) Get(ctx context.Context, id string) (*workspacedomain.Workspace, error) {
	return s.repo.Get(ctx, id)
}

// List returns all workspaces (small set, no pagination).
//
// List 返所有 workspace（量小，不分页）。
func (s *Service) List(ctx context.Context) ([]*workspacedomain.Workspace, error) {
	return s.repo.List(ctx)
}

// Update applies partial fields to a workspace; nil = skip.
//
// Update 部分更新；nil 字段跳过。
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*workspacedomain.Workspace, error) {
	w, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		name, err := cleanName(*in.Name)
		if err != nil {
			return nil, err
		}
		w.Name = name
	}
	if in.AvatarColor != nil {
		w.AvatarColor = strings.TrimSpace(*in.AvatarColor)
	}
	if in.Language != nil {
		if !workspacedomain.IsValidLanguage(*in.Language) {
			return nil, workspacedomain.ErrLanguageInvalid
		}
		w.Language = *in.Language
	}
	if in.WebFetchMode != nil {
		if !workspacedomain.IsValidWebFetchMode(*in.WebFetchMode) {
			return nil, workspacedomain.ErrWebFetchModeInvalid
		}
		w.WebFetchMode = *in.WebFetchMode
	}
	w.UpdatedAt = time.Now().UTC()
	if err := s.repo.Save(ctx, w); err != nil {
		return nil, err
	}
	return w, nil
}

// Delete removes a workspace, refusing the last one — the isolation root must exist.
//
// Delete 删 workspace，拒删最后一个——隔离根必须存在。
func (s *Service) Delete(ctx context.Context, id string) error {
	n, err := s.repo.Count(ctx)
	if err != nil {
		return fmt.Errorf("workspace.Delete: count: %w", err)
	}
	if n <= 1 {
		return workspacedomain.ErrCannotDeleteLast
	}
	// Cascade destroy first (PD-1): kill in-flight runs, detach trigger listeners, stop the
	// workspace's resident handler/mcp processes, remove its file tree. Best-effort — the row
	// delete below is the point of no return that makes everything unreachable.
	//
	// 先级联销毁（PD-1）：杀在途 run、摘 trigger 监听、停本 workspace 常驻 handler/mcp 进程、
	// 删文件树。best-effort——下方删行才是让一切不可达的不可回退点。
	if s.reaper != nil {
		s.reaper(ctx, id)
	}
	return s.repo.Delete(ctx, id)
}

// TouchLastUsed bumps the last-used timestamp (called on :activate / switch).
//
// TouchLastUsed 刷 last-used 时间戳（:activate / 切换时调）。
func (s *Service) TouchLastUsed(ctx context.Context, id string) error {
	return s.repo.TouchLastUsed(ctx, id)
}

// Resolve implements the auth middleware's WorkspaceResolver port: it confirms id names an
// existing workspace and returns its UI locale (derived from the persisted language) so the
// middleware can make the workspace language authoritative over Accept-Language (AC-PD-2).
// Unknown id → error. workspace.Language values ("zh-CN"/"en") are exactly the Locale values, so
// the cast is direct; an empty/invalid one is dropped by the middleware's IsSupported() guard.
//
// Resolve 实现 auth 中间件的 WorkspaceResolver 端口：确认 id 为已存在 workspace 并返回其 UI locale
// （由持久化 language 派生），使中间件让 workspace 语言压过 Accept-Language（AC-PD-2）。未知 id→错。
// workspace.Language 取值（"zh-CN"/"en"）正是 Locale 取值，故直接 cast；空/非法值由中间件
// IsSupported() 守卫丢弃。
func (s *Service) Resolve(ctx context.Context, id string) (reqctxpkg.Locale, error) {
	w, err := s.repo.Get(ctx, id)
	if err != nil {
		return "", err
	}
	return reqctxpkg.Locale(w.Language), nil
}

// Pick implements modeldomain.ModelPicker: it returns the current workspace's default ModelRef for
// a scenario (workspace id from ctx). ErrNotConfigured when that scenario has no default, so the
// caller surfaces a "configure a model" prompt rather than failing opaquely.
//
// Pick 实现 modeldomain.ModelPicker：返回当前 workspace（id 取自 ctx）某 scenario 的默认 ModelRef。
// 该 scenario 无默认时返 ErrNotConfigured——caller 提示"去配置模型"而非晦涩报错。
func (s *Service) Pick(ctx context.Context, scenario string) (modeldomain.ModelRef, error) {
	if !modeldomain.IsValidScenario(scenario) {
		return modeldomain.ModelRef{}, modeldomain.ErrScenarioInvalid
	}
	wsID, err := reqctxpkg.RequireWorkspaceID(ctx)
	if err != nil {
		return modeldomain.ModelRef{}, err
	}
	w, err := s.repo.Get(ctx, wsID)
	if err != nil {
		return modeldomain.ModelRef{}, err
	}
	ref := w.DefaultFor(scenario)
	if ref == nil || ref.IsZero() {
		return modeldomain.ModelRef{}, modeldomain.ErrNotConfigured
	}
	return *ref, nil
}

// SetDefault sets (or clears, with a nil ref) the default model for one scenario of a workspace; a
// non-nil ref must carry both apiKeyId and modelId.
//
// SetDefault 设置（nil ref 则清除）某 workspace 某 scenario 的默认模型；非 nil ref 须带 apiKeyId+modelId。
func (s *Service) SetDefault(ctx context.Context, id, scenario string, ref *modeldomain.ModelRef) (*workspacedomain.Workspace, error) {
	if !modeldomain.IsValidScenario(scenario) {
		return nil, modeldomain.ErrScenarioInvalid
	}
	if ref != nil {
		if err := ref.Validate(); err != nil {
			return nil, err
		}
	}
	w, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	w.SetDefaultFor(scenario, ref)
	w.UpdatedAt = time.Now().UTC()
	if err := s.repo.Save(ctx, w); err != nil {
		return nil, err
	}
	s.log.Info("workspace default model set", zap.String("workspace_id", id), zap.String("scenario", scenario))
	return w, nil
}

// DefaultSearchKeyID implements websearch.SearchKeyPicker: it returns the current
// workspace's chosen search api-key id (workspace id from ctx); ok=false when none is
// configured or the workspace can't be loaded — WebSearch then falls through to its
// next backend rather than failing.
//
// DefaultSearchKeyID 实现 websearch.SearchKeyPicker：返回当前 workspace（id 取自 ctx）选定的搜索
// api-key id；未配置或 workspace 取不到时 ok=false——WebSearch 据此降级到下个后端而非报错。
func (s *Service) DefaultSearchKeyID(ctx context.Context) (string, bool) {
	wsID, err := reqctxpkg.RequireWorkspaceID(ctx)
	if err != nil {
		return "", false
	}
	w, err := s.repo.Get(ctx, wsID)
	if err != nil {
		return "", false
	}
	id := strings.TrimSpace(w.DefaultSearchKeyID)
	return id, id != ""
}

// ReferencesAPIKey implements apikeyapp.RefScanner: the current workspace (id from ctx)
// references an api-key when any scenario default model (dialogue/utility/agent) or the
// default search key points at it. Deleting such a key would silently dangle the
// workspace's model config, so apikey.Delete consults this scanner and refuses with
// API_KEY_IN_USE. A missing workspace in ctx or an unreadable row is a scan miss (false),
// never a hard error — a delete-guard must not block on its own lookup failing.
//
// ReferencesAPIKey 实现 apikeyapp.RefScanner：当前 workspace（id 取自 ctx）的任一 scenario
// 默认模型（dialogue/utility/agent）或默认搜索 key 指向某 api-key 即算引用。删它会静默悬空
// workspace 的模型配置，故 apikey.Delete 询问本 scanner、命中即拒删 API_KEY_IN_USE。ctx 无
// workspace 或行读不到都算未命中（false）、绝不硬错——删除守卫不能因自身查询失败而挡删。
func (s *Service) ReferencesAPIKey(ctx context.Context, apiKeyID string) (bool, error) {
	if apiKeyID == "" {
		return false, nil
	}
	wsID, err := reqctxpkg.RequireWorkspaceID(ctx)
	if err != nil {
		return false, nil
	}
	w, err := s.repo.Get(ctx, wsID)
	if err != nil {
		return false, nil
	}
	for _, scenario := range modeldomain.ListScenarios() {
		if ref := w.DefaultFor(scenario); ref != nil && ref.APIKeyID == apiKeyID {
			return true, nil
		}
	}
	return strings.TrimSpace(w.DefaultSearchKeyID) == apiKeyID, nil
}

// WebFetchMode resolves the current workspace's web-fetch mode for the WebFetch tool:
// "local" (direct GET, the default) or "jina" (third-party reader). Any failure to read the
// workspace falls back to local — never leak a URL on a degraded path (PD-4 decision C).
//
// WebFetchMode 为 WebFetch 工具解析当前 workspace 的抓取模式："local"（直接 GET，默认）或
// "jina"（第三方 reader）。读不到 workspace 一律落回 local——降级路径绝不外发 URL（PD-4 裁决 C）。
func (s *Service) WebFetchMode(ctx context.Context) string {
	wsID, err := reqctxpkg.RequireWorkspaceID(ctx)
	if err != nil {
		return workspacedomain.WebFetchModeLocal
	}
	w, err := s.repo.Get(ctx, wsID)
	if err != nil {
		return workspacedomain.WebFetchModeLocal
	}
	return workspacedomain.EffectiveWebFetchMode(w.WebFetchMode)
}

// SetDefaultSearch sets (or clears with "") the workspace's default search api-key id.
// No provider/category check — mirrors SetDefault's runtime-graceful style: the WebSearch
// tool rejects a non-search key at call time, and the UI only offers search-category keys.
//
// SetDefaultSearch 设置（""则清除）workspace 的默认搜索 api-key id。不校验 provider/category
// ——镜像 SetDefault 的运行时优雅风格：WebSearch 工具调用时拒非搜索 key，UI 只让选 search 类 key。
func (s *Service) SetDefaultSearch(ctx context.Context, id, keyID string) (*workspacedomain.Workspace, error) {
	w, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	w.DefaultSearchKeyID = strings.TrimSpace(keyID)
	w.UpdatedAt = time.Now().UTC()
	if err := s.repo.Save(ctx, w); err != nil {
		return nil, err
	}
	s.log.Info("workspace default search key set",
		zap.String("workspace_id", id), zap.Bool("cleared", w.DefaultSearchKeyID == ""))
	return w, nil
}

// Service implements ModelPicker and websearch.SearchKeyPicker — the LLM/search-using
// callers (波次 2/3/5) depend on these ports.
//
// Service 实现 ModelPicker 与 websearch.SearchKeyPicker——用 LLM/搜索的 caller（波次 2/3/5）依赖这些端口。
var (
	_ modeldomain.ModelPicker         = (*Service)(nil)
	_ websearchdomain.SearchKeyPicker = (*Service)(nil)
)

// cleanName trims, requires non-empty, and bounds the length of a workspace name.
//
// cleanName 去空白、要求非空、限制 workspace 名长度。
func cleanName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", workspacedomain.ErrNameRequired
	}
	if utf8.RuneCountInString(name) > workspacedomain.MaxNameLen {
		return "", workspacedomain.ErrNameTooLong
	}
	return name, nil
}

// resolveLanguage defaults an empty language to zh-CN and validates non-empty ones.
//
// resolveLanguage 把空 language 默认为 zh-CN，非空则校验。
func resolveLanguage(lang string) (string, error) {
	if lang == "" {
		return workspacedomain.LanguageZhCN, nil
	}
	if !workspacedomain.IsValidLanguage(lang) {
		return "", workspacedomain.ErrLanguageInvalid
	}
	return lang, nil
}
