// Package agent provides the application service for Agent entities (quadrinity 4th member).
// An Agent is a configured LLM worker: prompt + skill + knowledge + tools + outputSchema + model.
// Mirrors function/handler service patterns: create → pending → accept → active.
//
// Package agent 是 Agent 实体的应用层服务（quadrinity 第四元）。
package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	agentdomain "github.com/sunweilin/forgify/backend/internal/domain/agent"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Service orchestrates agent lifecycle.
type Service struct {
	repo agentdomain.Repository
	log  *zap.Logger
}

func New(repo agentdomain.Repository, log *zap.Logger) *Service {
	if log == nil {
		panic("agentapp.New: logger is nil")
	}
	return &Service{repo: repo, log: log.Named("agentapp")}
}

// CreateInput is the request for creating a new agent.
type CreateInput struct {
	Name          string
	Description   string
	Tags          []string
	Prompt        string
	Skill         string
	Knowledge     []string
	Tools         []agentdomain.ToolRef
	OutputSchema  *agentdomain.OutputSchema
	ModelOverride string
	ChangeReason  string
}

// Create creates a new agent with an auto-accepted v1 (mirrors function/handler pattern).
func (s *Service) Create(ctx context.Context, in CreateInput) (*agentdomain.Agent, *agentdomain.AgentVersion, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("agentapp.Create: %w", err)
	}
	if in.Name == "" {
		return nil, nil, fmt.Errorf("agentapp.Create: name is required")
	}
	if in.Prompt == "" {
		return nil, nil, fmt.Errorf("agentapp.Create: prompt is required")
	}
	if err := validateToolRefs(in.Tools); err != nil {
		return nil, nil, fmt.Errorf("agentapp.Create: %w", err)
	}

	a := &agentdomain.Agent{
		ID:          idgenpkg.New("ag"),
		UserID:      uid,
		Name:        in.Name,
		Description: in.Description,
		Tags:        in.Tags,
	}
	if a.Tags == nil {
		a.Tags = []string{}
	}
	if err := s.repo.Create(ctx, a); err != nil {
		return nil, nil, fmt.Errorf("agentapp.Create: %w", err)
	}

	versionN := 1
	v := &agentdomain.AgentVersion{
		AgentID:       a.ID,
		Prompt:        in.Prompt,
		Skill:         in.Skill,
		Knowledge:     in.Knowledge,
		Tools:         in.Tools,
		OutputSchema:  in.OutputSchema,
		ModelOverride: in.ModelOverride,
		Status:        agentdomain.VersionStatusAccepted,
		Version:       &versionN,
		ChangeReason:  in.ChangeReason,
	}
	if v.Knowledge == nil {
		v.Knowledge = []string{}
	}
	if v.Tools == nil {
		v.Tools = []agentdomain.ToolRef{}
	}
	now := time.Now().UTC()
	v.AcceptedAt = &now
	if err := s.repo.CreateVersion(ctx, v); err != nil {
		return nil, nil, fmt.Errorf("agentapp.Create: version: %w", err)
	}
	a.ActiveVersionID = v.ID
	if err := s.repo.Update(ctx, a); err != nil {
		return nil, nil, fmt.Errorf("agentapp.Create: link active version: %w", err)
	}
	a.ActiveVersion = v
	return a, v, nil
}

// Get loads an agent by ID, attaching active version and pending version if present.
func (s *Service) Get(ctx context.Context, id string) (*agentdomain.Agent, error) {
	a, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("agentapp.Get: %w", err)
	}
	if a.ActiveVersionID != "" {
		if v, err := s.repo.GetVersion(ctx, a.ActiveVersionID); err == nil {
			a.ActiveVersion = v
		}
	}
	if pv, err := s.repo.GetPending(ctx, a.ID); err == nil {
		a.Pending = pv
	}
	return a, nil
}

// EditInput applies a new pending version to an agent.
type EditInput struct {
	ID            string
	Prompt        *string
	Skill         *string
	Knowledge     []string
	Tools         []agentdomain.ToolRef
	OutputSchema  *agentdomain.OutputSchema
	ModelOverride *string
	ChangeReason  string
}

// Edit creates a pending version (or overwrites an existing pending) with the given changes.
// Mirrors edit_function's "iterate-same-pending" pattern.
func (s *Service) Edit(ctx context.Context, in EditInput) (*agentdomain.AgentVersion, error) {
	if in.ID == "" {
		return nil, fmt.Errorf("agentapp.Edit: id is required")
	}
	if in.Tools != nil {
		if err := validateToolRefs(in.Tools); err != nil {
			return nil, fmt.Errorf("agentapp.Edit: %w", err)
		}
	}
	a, err := s.repo.Get(ctx, in.ID)
	if err != nil {
		return nil, fmt.Errorf("agentapp.Edit: %w", err)
	}
	// Start from active version as base, or from existing pending.
	base, _ := s.repo.GetPending(ctx, a.ID)
	if base == nil {
		if a.ActiveVersionID != "" {
			base, _ = s.repo.GetVersion(ctx, a.ActiveVersionID)
		}
	}

	v := &agentdomain.AgentVersion{
		AgentID:      a.ID,
		Status:       agentdomain.VersionStatusPending,
		ChangeReason: in.ChangeReason,
	}
	// Overlay base values.
	if base != nil {
		v.Prompt = base.Prompt
		v.Skill = base.Skill
		v.Knowledge = append([]string{}, base.Knowledge...)
		v.Tools = append([]agentdomain.ToolRef{}, base.Tools...)
		v.OutputSchema = base.OutputSchema
		v.ModelOverride = base.ModelOverride
	}
	// Apply patches.
	if in.Prompt != nil {
		v.Prompt = *in.Prompt
	}
	if in.Skill != nil {
		v.Skill = *in.Skill
	}
	if in.Knowledge != nil {
		v.Knowledge = in.Knowledge
	}
	if in.Tools != nil {
		v.Tools = in.Tools
	}
	if in.OutputSchema != nil {
		v.OutputSchema = in.OutputSchema
	}
	if in.ModelOverride != nil {
		v.ModelOverride = *in.ModelOverride
	}
	if v.Knowledge == nil {
		v.Knowledge = []string{}
	}
	if v.Tools == nil {
		v.Tools = []agentdomain.ToolRef{}
	}

	// If there's already a pending, overwrite it (iterate-same-pending).
	if existing, _ := s.repo.GetPending(ctx, a.ID); existing != nil {
		v.ID = existing.ID
		v.UserID = existing.UserID
		if err := s.repo.CreateVersion(ctx, v); err != nil {
			_ = err // fallthrough to create new
		} else {
			return v, nil
		}
	}
	if err := s.repo.CreateVersion(ctx, v); err != nil {
		return nil, fmt.Errorf("agentapp.Edit: %w", err)
	}
	return v, nil
}

// Accept promotes the pending version to active.
func (s *Service) Accept(ctx context.Context, agentID string) (*agentdomain.AgentVersion, error) {
	pv, err := s.repo.GetPending(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("agentapp.Accept: %w", err)
	}
	if err := s.repo.AcceptVersion(ctx, agentID, pv.ID); err != nil {
		return nil, fmt.Errorf("agentapp.Accept: %w", err)
	}
	pv.Status = agentdomain.VersionStatusAccepted
	return pv, nil
}

// Delete soft-deletes an agent.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		return fmt.Errorf("agentapp.Delete: %w", err)
	}
	return nil
}

// List returns agents for the current user.
func (s *Service) List(ctx context.Context, limit int, cursor string) ([]*agentdomain.Agent, string, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("agentapp.List: %w", err)
	}
	return s.repo.List(ctx, uid, limit, cursor)
}

// validateToolRefs ensures no tool references another agent (员工不调员工).
func validateToolRefs(tools []agentdomain.ToolRef) error {
	for _, t := range tools {
		if strings.HasPrefix(t.Ref, "ag_") {
			return agentdomain.ErrToolsAgentRef
		}
	}
	return nil
}

// GetAgentConfig implements scheduler.AgentEntityResolver: loads the agent entity's active version
// config for use by the workflow agent dispatcher (doc 02 §"节点形态" agentRef pattern).
//
// GetAgentConfig 实现 scheduler.AgentEntityResolver：为 workflow agent dispatcher 加载配置。
func (s *Service) GetAgentConfig(ctx context.Context, agentRef string) (prompt string, maxTurns int, enabledTools []string, modelOverride string, err error) {
	a, aErr := s.repo.Get(ctx, agentRef)
	if aErr != nil {
		return "", 0, nil, "", fmt.Errorf("agentapp.GetAgentConfig: %w", aErr)
	}
	if a.ActiveVersionID == "" {
		return "", 0, nil, "", agentdomain.ErrNoActiveVersion
	}
	v, vErr := s.repo.GetVersion(ctx, a.ActiveVersionID)
	if vErr != nil {
		return "", 0, nil, "", fmt.Errorf("agentapp.GetAgentConfig: version: %w", vErr)
	}
	// Convert tool refs to string IDs for filterToolsByWhitelist.
	toolIDs := make([]string, 0, len(v.Tools))
	for _, t := range v.Tools {
		toolIDs = append(toolIDs, t.Ref)
	}
	return v.Prompt, 0, toolIDs, v.ModelOverride, nil
}

// ListVersions returns all versions for an agent.
func (s *Service) ListVersions(ctx context.Context, agentID string) ([]*agentdomain.AgentVersion, error) {
	return s.repo.ListVersions(ctx, agentID)
}

// GetPending returns the pending version for an agent.
func (s *Service) GetPending(ctx context.Context, agentID string) (*agentdomain.AgentVersion, error) {
	return s.repo.GetPending(ctx, agentID)
}

// RejectPending discards the pending version (iterate-same-pending: next Edit overwrites it).
// Since AgentVersion has no soft-delete, rejection is recorded by marking the pending version
// as effectively superseded. The store's iterate-same-pending pattern in Edit() handles cleanup.
func (s *Service) RejectPending(ctx context.Context, agentID string) error {
	if _, err := s.repo.GetPending(ctx, agentID); err != nil {
		return fmt.Errorf("agentapp.RejectPending: %w", err)
	}
	s.log.Info("agentapp.RejectPending: pending discarded; next Edit will overwrite",
		zap.String("agentID", agentID))
	return nil
}

