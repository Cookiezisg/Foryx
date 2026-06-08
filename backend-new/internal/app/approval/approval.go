// Package approval (app layer) orchestrates the approval-form domain: forging versions
// from a full prompt + decision-rule set (no ops), compiling the template's `{{ CEL }}`
// spans via pkg/cel at create/edit time, and the relation / catalog adapters. The version
// model is a linear, append-only history with a free-moving ActiveVersionID pointer — no
// pending/accept, no sandbox/env/executions. There is no run/executions: an approval form
// is rendered + parked by the durable interpreter (波次 4), never invoked standalone — the
// Service exposes Resolve so the interpreter reads the pinned version's template + rules.
//
// Package approval（app 层）编排审批表 domain：从完整 prompt + 决策规则组锻造版本（无 ops）、
// create/edit 时用 pkg/cel 编译模板的 `{{ CEL }}` 段、relation / catalog 适配器。版本模型线性、
// 只增 + 自由 ActiveVersionID 指针——无 pending/accept、无 sandbox/env/executions。无 run/executions
// ——审批表由 durable 解释器（波次 4）渲染 + park、绝不独立调用——Service 暴露 Resolve 供解释器读
// pin 版本的 template + 规则。
package approval

import (
	"context"

	"go.uber.org/zap"

	approvaldomain "github.com/sunweilin/forgify/backend/internal/domain/approval"
	notificationdomain "github.com/sunweilin/forgify/backend/internal/domain/notification"
	relationdomain "github.com/sunweilin/forgify/backend/internal/domain/relation"
)

// RelationSyncer is the slice of relationapp.Service approval consumes (nil-tolerant).
//
// RelationSyncer 是 approval 消费的 relationapp.Service 切片（允许 nil）。
type RelationSyncer interface {
	SyncIncoming(ctx context.Context, toKind, toID string, kindScope []string, edges []relationdomain.SyncEdge) error
	PurgeEntity(ctx context.Context, kind, id string) error
}

// Service orchestrates the approval-form domain.
//
// Service 编排审批表 domain。
type Service struct {
	repo      approvaldomain.Repository
	notif     notificationdomain.Emitter // nil-tolerant
	relations RelationSyncer             // nil disables relation hooks
	log       *zap.Logger
}

// NewService wires the service; nil repo / log is a wiring bug (log defaults to nop).
//
// NewService 装配 service；nil repo / log 是装配 bug（log 默认 nop）。
func NewService(repo approvaldomain.Repository, notif notificationdomain.Emitter, log *zap.Logger) *Service {
	if repo == nil {
		panic("approvalapp.NewService: repo is nil")
	}
	if log == nil {
		log = zap.NewNop()
	}
	return &Service{repo: repo, notif: notif, log: log}
}

// SetRelationSyncer installs the relation Service post-construction (avoids an init cycle).
//
// SetRelationSyncer 装配后注入 relation Service（避 init 环）。
func (s *Service) SetRelationSyncer(r RelationSyncer) { s.relations = r }

// publish emits an approval lifecycle notification; nil emitter is a no-op.
//
// publish 发一条 approval 生命周期通知；nil emitter 为 no-op。
func (s *Service) publish(ctx context.Context, action, approvalID string, extra map[string]any) {
	if s.notif == nil {
		return
	}
	payload := map[string]any{"approvalId": approvalID}
	for k, v := range extra {
		payload[k] = v
	}
	if err := s.notif.Emit(ctx, "approval."+action, payload); err != nil {
		s.log.Warn("approvalapp.publish: emit failed", zap.String("action", action), zap.Error(err))
	}
}
