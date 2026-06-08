package trigger

import (
	"context"
	"strings"

	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
	croninfra "github.com/sunweilin/forgify/backend/internal/infra/trigger/cron"
	celpkg "github.com/sunweilin/forgify/backend/internal/pkg/cel"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
)

// CreateInput is a new trigger's fields.
//
// CreateInput 是新建 trigger 的字段。
type CreateInput struct {
	Name        string
	Description string
	Kind        string
	Config      map[string]any
	Outputs     []schemapkg.Field
}

// EditInput patches a trigger; nil pointers / nil Config leave fields unchanged. Kind is
// immutable (change of source kind = delete + recreate).
//
// EditInput 局部更新；nil 指针 / nil Config 不改。Kind 不可变（换 source 种类 = 删了重建）。
type EditInput struct {
	Name        *string
	Description *string
	Config      map[string]any
	Outputs     []schemapkg.Field
}

// Create validates + persists a new trigger and syncs its relation edges. It does NOT attach
// a listener — a listener starts only when an active workflow references it (Attach, 波次 4).
//
// Create 校验 + 持久化新 trigger 并同步关系边。**不挂 listener**——listener 仅在 active workflow 引用时启动。
func (s *Service) Create(ctx context.Context, in CreateInput) (*triggerdomain.Trigger, error) {
	if err := s.validate(in.Kind, in.Config); err != nil {
		return nil, err
	}
	cfg := in.Config
	if cfg == nil {
		cfg = map[string]any{}
	}
	t := &triggerdomain.Trigger{
		ID:          idgenpkg.New("trg"),
		Name:        in.Name,
		Description: in.Description,
		Kind:        in.Kind,
		Config:      cfg,
		Outputs:     in.Outputs,
	}
	if err := s.repo.SaveTrigger(ctx, t); err != nil {
		return nil, err
	}
	s.syncSensorBinding(ctx, t)
	s.syncForgedEdge(ctx, t.ID)
	return t, nil
}

// Edit patches name/description/config (not kind), re-validates, and re-registers the listener
// if the trigger is currently hot (so a config change takes effect immediately).
//
// Edit 改 name/description/config（不改 kind），重校验，若 trigger 正热则重注册 listener（config 立即生效）。
func (s *Service) Edit(ctx context.Context, id string, in EditInput) (*triggerdomain.Trigger, error) {
	t, err := s.repo.GetTrigger(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		t.Name = *in.Name
	}
	if in.Description != nil {
		t.Description = *in.Description
	}
	if in.Config != nil {
		t.Config = in.Config
	}
	if in.Outputs != nil {
		t.Outputs = in.Outputs
	}
	if err := s.validate(t.Kind, t.Config); err != nil {
		return nil, err
	}
	if err := s.repo.SaveTrigger(ctx, t); err != nil {
		return nil, err
	}
	s.syncSensorBinding(ctx, t)
	s.restartIfListening(t)
	s.attachRuntime(t)
	return t, nil
}

// Delete stops any hot listener, soft-deletes the trigger, and purges its relation edges.
//
// Delete 停掉热 listener、软删 trigger、清除关系边。
func (s *Service) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	if e, ok := s.listeners[id]; ok {
		if l := s.listenerFor(e.kind); l != nil {
			l.Unregister(id)
		}
		delete(s.listeners, id)
	}
	s.mu.Unlock()
	if err := s.repo.DeleteTrigger(ctx, id); err != nil {
		return err
	}
	s.purgeRelations(ctx, id)
	return nil
}

// Get returns a trigger with its runtime RefCount/Listening attached.
//
// Get 返回 trigger 并附加运行时 RefCount/Listening。
func (s *Service) Get(ctx context.Context, id string) (*triggerdomain.Trigger, error) {
	t, err := s.repo.GetTrigger(ctx, id)
	if err != nil {
		return nil, err
	}
	s.attachRuntime(t)
	return t, nil
}

// List pages triggers with runtime state attached.
//
// List 分页 trigger 并附加运行时状态。
func (s *Service) List(ctx context.Context, filter triggerdomain.ListFilter) ([]*triggerdomain.Trigger, string, error) {
	ts, next, err := s.repo.ListTriggers(ctx, filter)
	if err != nil {
		return nil, "", err
	}
	for _, t := range ts {
		s.attachRuntime(t)
	}
	return ts, next, nil
}

// ListAll returns every trigger (used by the catalog source).
//
// ListAll 返回所有 trigger（catalog source 用）。
func (s *Service) ListAll(ctx context.Context) ([]*triggerdomain.Trigger, error) {
	return s.repo.ListAllTriggers(ctx)
}

// Search returns triggers whose name / description / kind contain query (case-insensitive);
// an empty query returns all. Runtime state is attached. Backs the search_triggers tool.
//
// Search 返回 name/description/kind 含 query（大小写不敏感）的 trigger；空 query 返全部，附运行时状态。
func (s *Service) Search(ctx context.Context, query string) ([]*triggerdomain.Trigger, error) {
	ts, err := s.repo.ListAllTriggers(ctx)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]*triggerdomain.Trigger, 0, len(ts))
	for _, t := range ts {
		if q == "" || strings.Contains(strings.ToLower(t.Name), q) ||
			strings.Contains(strings.ToLower(t.Description), q) ||
			strings.Contains(strings.ToLower(t.Kind), q) {
			s.attachRuntime(t)
			out = append(out, t)
		}
	}
	return out, nil
}

// SearchActivations / GetActivation expose the action log for the search_activations /
// get_activation tools ("why didn't it fire?").
//
// SearchActivations / GetActivation 暴露动作日志，供 search_activations / get_activation 工具（"为什么没触发"）。
func (s *Service) SearchActivations(ctx context.Context, filter triggerdomain.ActivationFilter) ([]*triggerdomain.Activation, string, error) {
	return s.repo.SearchActivations(ctx, filter)
}

func (s *Service) GetActivation(ctx context.Context, id string) (*triggerdomain.Activation, error) {
	return s.repo.GetActivation(ctx, id)
}

// validate checks kind + structural config (domain) then source-specific syntax: cron
// expression parse and sensor CEL compile (condition/output). CEL/cron syntax can't live in
// the domain (no cel-go/robfig import), so it's verified here and mapped to a domain error.
//
// validate 校验 kind + 结构 config（domain），再做 source 专属语法：cron 表达式解析、sensor CEL 编译。
// CEL/cron 语法不能放 domain（不能 import cel-go/robfig），故在此校验并映射成 domain 错误。
func (s *Service) validate(kind string, config map[string]any) error {
	if !triggerdomain.IsValidKind(kind) {
		return triggerdomain.ErrInvalidKind
	}
	if err := triggerdomain.ValidateConfig(kind, config); err != nil {
		return err
	}
	switch kind {
	case triggerdomain.KindCron:
		if err := croninfra.Validate(triggerdomain.CronExpression(config)); err != nil {
			return triggerdomain.ErrInvalidCron
		}
	case triggerdomain.KindSensor:
		sc := triggerdomain.ParseSensorConfig(config)
		if _, err := celpkg.Compile(sc.Condition); err != nil {
			return triggerdomain.ErrInvalidCEL
		}
		if _, err := celpkg.Compile(sc.Output); err != nil {
			return triggerdomain.ErrInvalidCEL
		}
	}
	return nil
}
