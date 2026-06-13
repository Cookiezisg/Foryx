package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	agentdomain "github.com/sunweilin/forgify/backend/internal/domain/agent"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
)

// Config is the mutable agent configuration carried by create/edit (a full snapshot — edit
// REPLACES, it does not merge). It maps 1:1 onto an AgentVersion's mounted fields.
//
// Config 是 create/edit 携带的可变 agent 配置（全量快照——edit 是替换、非合并）。1:1 映射到
// AgentVersion 的挂载字段。
type Config struct {
	Prompt        string
	Skill         string
	Knowledge     []string
	Tools         []agentdomain.ToolRef
	Inputs        []schemapkg.Field
	Outputs       []schemapkg.Field
	ModelOverride *modeldomain.ModelRef
	ChangeReason  string
}

// CreateInput is the create payload: identity (name/description/tags) + the v1 Config.
//
// CreateInput 是 create 载荷：身份（name/description/tags）+ v1 Config。
type CreateInput struct {
	Name        string
	Description string
	Tags        []string
	Config
}

// EditInput is the edit payload: target id + the new full Config (becomes version max+1).
//
// EditInput 是 edit 载荷：目标 id + 新的完整 Config（成为版本 max+1）。
type EditInput struct {
	ID string
	Config
}

// UpdateMetaInput patches identity without a version bump; nil = unchanged.
//
// UpdateMetaInput 改身份不动版本；nil = 不变。
type UpdateMetaInput struct {
	ID          string
	Name        *string
	Description *string
	Tags        *[]string
}

// Get returns one agent with its active version attached.
//
// Get 返单 agent 并附上 active 版本。
func (s *Service) Get(ctx context.Context, id string) (*agentdomain.Agent, error) {
	a, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("agentapp.Get: %w", err)
	}
	if a.ActiveVersionID != "" {
		if v, verr := s.repo.GetVersion(ctx, a.ActiveVersionID); verr == nil {
			a.ActiveVersion = v
		}
	}
	return a, nil
}

// ListVersions returns one keyset page of an agent's versions (newest first, N4).
//
// ListVersions 返 agent 版本的一页 keyset（新→旧，N4）。
func (s *Service) ListVersions(ctx context.Context, agentID string, filter agentdomain.VersionListFilter) ([]*agentdomain.Version, string, error) {
	return s.repo.ListVersions(ctx, agentID, filter)
}

// GetVersion returns one version by its id.
//
// GetVersion 按 id 返单个版本。
func (s *Service) GetVersion(ctx context.Context, versionID string) (*agentdomain.Version, error) {
	return s.repo.GetVersion(ctx, versionID)
}

// GetVersionByNumber returns one version by (agentID, number).
//
// GetVersionByNumber 按 (agentID, 版本号) 返单个版本。
func (s *Service) GetVersionByNumber(ctx context.Context, agentID string, version int) (*agentdomain.Version, error) {
	return s.repo.GetVersionByNumber(ctx, agentID, version)
}

// List returns a cursor page of live agents.
func (s *Service) List(ctx context.Context, limit int, cursor string) ([]*agentdomain.Agent, string, error) {
	return s.repo.List(ctx, limit, cursor)
}

// ListAll returns every live agent (no pagination).
func (s *Service) ListAll(ctx context.Context) ([]*agentdomain.Agent, error) {
	return s.repo.ListAll(ctx)
}

// ReferencesAPIKey implements apikeyapp.RefScanner: an agent references an api-key when its
// active version pins a modelOverride on that key. Deleting the key would break the agent's
// next invoke with an opaque resolve error, so apikey.Delete consults this scanner and
// refuses with API_KEY_IN_USE while any agent overrides the key. An unreadable active
// version is treated as a non-reference (the delete-guard never blocks on a lookup miss).
//
// ReferencesAPIKey 实现 apikeyapp.RefScanner：当某 agent 的 active 版本以 modelOverride 钉在该
// api-key 上即算引用。删它会让 agent 下次 invoke 以晦涩的解析错误崩，故 apikey.Delete 询问本
// scanner、有 agent 引用时拒删 API_KEY_IN_USE。active 版本读不到按未引用处理（守卫不因查询失败挡删）。
func (s *Service) ReferencesAPIKey(ctx context.Context, apiKeyID string) (bool, error) {
	if apiKeyID == "" {
		return false, nil
	}
	agents, err := s.repo.ListAll(ctx)
	if err != nil {
		return false, fmt.Errorf("agentapp.ReferencesAPIKey: list agents: %w", err)
	}
	for _, a := range agents {
		if a.ActiveVersionID == "" {
			continue
		}
		v, err := s.GetVersion(ctx, a.ActiveVersionID)
		if err != nil {
			continue
		}
		if v.ModelOverride != nil && v.ModelOverride.APIKeyID == apiKeyID {
			return true, nil
		}
	}
	return false, nil
}

// Search filters live agents by case-insensitive substring over name / description / tags
// (no LLM rerank — the calling LLM judges relevance from the slim results).
//
// Search 按 name / description / tags 大小写不敏感子串过滤活跃 agent（无 LLM 排序——相关性由调用方
// LLM 从精简结果里判断）。
func (s *Service) Search(ctx context.Context, query string) ([]*agentdomain.Agent, error) {
	all, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("agentapp.Search: %w", err)
	}
	if strings.TrimSpace(query) == "" {
		return all, nil
	}
	needle := strings.ToLower(query)
	out := make([]*agentdomain.Agent, 0, len(all))
	for _, a := range all {
		if matchesAgent(a, needle) {
			out = append(out, a)
		}
	}
	return out, nil
}

// Create persists a new Agent + v1 (active) and syncs its relation edges. No env, no sandbox.
//
// Create 持久化新 Agent + v1（active）并同步 relation 边。无 env、无 sandbox。
func (s *Service) Create(ctx context.Context, in CreateInput) (*agentdomain.Agent, *agentdomain.Version, error) {
	if err := validateModelOverride(in.ModelOverride); err != nil {
		return nil, nil, err
	}

	now := time.Now().UTC()
	agentID := idgenpkg.New("ag")
	versionID := idgenpkg.New("agv")
	convID, _ := reqctxpkg.GetConversationID(ctx)

	v := buildVersion(versionID, agentID, 1, in.Config, now, convID)
	if err := v.ValidateTools(); err != nil {
		return nil, nil, err
	}
	a := &agentdomain.Agent{
		ID: agentID, Name: in.Name, Description: in.Description, Tags: orEmptyStrs(in.Tags),
		ActiveVersionID: versionID, CreatedAt: now, UpdatedAt: now,
	}

	if err := s.repo.CreateWithVersion(ctx, a, v); err != nil {
		return nil, nil, fmt.Errorf("agentapp.Create: %w", err) // ErrNameConflict on duplicate
	}
	a.ActiveVersion = v
	s.syncRelations(ctx, a, v)
	s.publish(ctx, "created", agentID, map[string]any{"versionId": versionID, "version": 1})
	return a, v, nil
}

// Edit writes a new version (max+1, full Config replace) and moves the active pointer to it.
//
// Edit 写新版本（max+1，全量 Config 替换）并把 active 指针移到它。
func (s *Service) Edit(ctx context.Context, in EditInput) (*agentdomain.Version, error) {
	a, err := s.repo.Get(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("agentapp.Edit: %w", err)
	}
	if err := validateModelOverride(in.ModelOverride); err != nil {
		return nil, err
	}

	nextN, err := s.repo.NextVersionNumber(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("agentapp.Edit: %w", err)
	}
	now := time.Now().UTC()
	versionID := idgenpkg.New("agv")
	convID, _ := reqctxpkg.GetConversationID(ctx)

	v := buildVersion(versionID, in.ID, nextN, in.Config, now, convID)
	if err := v.ValidateTools(); err != nil {
		return nil, err
	}

	if err := s.repo.SaveVersionAndActivate(ctx, v, in.ID); err != nil {
		return nil, fmt.Errorf("agentapp.Edit: %w", err)
	}
	if err := s.repo.TrimVersions(ctx, in.ID, agentdomain.AcceptedVersionCap); err != nil {
		s.log.Warn("agentapp.Edit: trim versions failed", zap.String("agentId", in.ID), zap.Error(err))
	}
	a.ActiveVersionID = versionID
	a.ActiveVersion = v
	s.syncRelations(ctx, a, v)
	s.publish(ctx, "edited", in.ID, map[string]any{"versionId": versionID, "version": nextN})
	return v, nil
}

// Revert moves the active pointer back to an existing version number (does not renumber).
//
// Revert 把 active 指针移回一个已存在的版本号（不重排号）。
func (s *Service) Revert(ctx context.Context, id string, targetVersion int) (*agentdomain.Version, error) {
	v, err := s.repo.GetVersionByNumber(ctx, id, targetVersion)
	if err != nil {
		return nil, fmt.Errorf("agentapp.Revert: %w", err) // ErrVersionNotFound
	}
	if err := s.repo.SetActiveVersion(ctx, id, v.ID); err != nil {
		return nil, fmt.Errorf("agentapp.Revert: %w", err)
	}
	if a, gerr := s.repo.Get(ctx, id); gerr == nil {
		a.ActiveVersion = v
		s.syncRelations(ctx, a, v)
	}
	s.publish(ctx, "reverted", id, map[string]any{"versionId": v.ID, "version": targetVersion})
	return v, nil
}

// UpdateMeta patches name/description/tags only (no version bump, no relation resync).
//
// UpdateMeta 仅改 name/description/tags（不升版本、不重算 relation）。
func (s *Service) UpdateMeta(ctx context.Context, in UpdateMetaInput) (*agentdomain.Agent, error) {
	a, err := s.repo.Get(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("agentapp.UpdateMeta: %w", err)
	}
	if in.Name != nil {
		a.Name = *in.Name
	}
	if in.Description != nil {
		a.Description = *in.Description
	}
	if in.Tags != nil {
		a.Tags = orEmptyStrs(*in.Tags)
	}
	if err := s.repo.UpdateMeta(ctx, a); err != nil {
		return nil, fmt.Errorf("agentapp.UpdateMeta: %w", err) // ErrNameConflict
	}
	s.publish(ctx, "updated", in.ID, nil)
	return a, nil
}

// Delete soft-deletes the agent and purges its relation edges.
//
// Delete 软删 agent 并清除其 relation 边。
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		return fmt.Errorf("agentapp.Delete: %w", err)
	}
	s.purgeRelations(ctx, id)
	s.publish(ctx, "deleted", id, nil)
	return nil
}

// --- helpers ---------------------------------------------------------------

func buildVersion(id, agentID string, ver int, cfg Config, now time.Time, convID string) *agentdomain.Version {
	return &agentdomain.Version{
		ID: id, AgentID: agentID, Version: ver,
		Prompt: cfg.Prompt, Skill: cfg.Skill,
		Knowledge: orEmptyStrs(cfg.Knowledge), Tools: orEmptyTools(cfg.Tools),
		Inputs: cfg.Inputs, Outputs: cfg.Outputs, ModelOverride: cfg.ModelOverride,
		ChangeReason: cfg.ChangeReason, ForgedInConversationID: convID,
		CreatedAt: now,
	}
}

// validateModelOverride requires both apiKeyId and modelId when an override is set.
//
// validateModelOverride 在设了 override 时要求 apiKeyId 和 modelId 都非空。
func validateModelOverride(o *modeldomain.ModelRef) error {
	if o == nil {
		return nil
	}
	if strings.TrimSpace(o.APIKeyID) == "" || strings.TrimSpace(o.ModelID) == "" {
		return agentdomain.ErrInvalidModelOverride
	}
	return nil
}

func matchesAgent(a *agentdomain.Agent, needle string) bool {
	if strings.Contains(strings.ToLower(a.Name), needle) || strings.Contains(strings.ToLower(a.Description), needle) {
		return true
	}
	for _, t := range a.Tags {
		if strings.Contains(strings.ToLower(t), needle) {
			return true
		}
	}
	return false
}

func orEmptyStrs(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func orEmptyTools(t []agentdomain.ToolRef) []agentdomain.ToolRef {
	if t == nil {
		return []agentdomain.ToolRef{}
	}
	return t
}
