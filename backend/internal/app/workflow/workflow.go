// Package workflow (app layer) orchestrates workflow authoring: CRUD, versions, ops, graph validation.
//
// Package workflow（app 层）编排 workflow 锻造：CRUD、版本、ops、图校验。
package workflow

import (
	"context"

	"go.uber.org/zap"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
)

// Service orchestrates the workflow domain.
//
// Service 编排 workflow domain。
type Service struct {
	repo        workflowdomain.Repository
	checker     CapabilityChecker
	notif       notificationspkg.Publisher
	relations   RelationSyncer                // optional; nil disables relation hooks
	keyProvider apikeydomain.KeyProvider      // optional; nil skips F1 node-override validation
	triggerSync TriggerSync                   // optional; nil disables :activate→listener registration
	log         *zap.Logger
}

// NewService wires Service; panics on nil log/notif; nil checker uses NopChecker.
//
// NewService 装配 Service；nil log/notif panic；nil checker 回落 NopChecker。
func NewService(
	repo workflowdomain.Repository,
	checker CapabilityChecker,
	notif notificationspkg.Publisher,
	log *zap.Logger,
) *Service {
	if log == nil {
		panic("workflowapp.NewService: logger is nil")
	}
	if notif == nil {
		panic("workflowapp.NewService: notif is nil")
	}
	if checker == nil {
		checker = NopChecker()
	}
	return &Service{
		repo:    repo,
		checker: checker,
		notif:   notif,
		log:     log.Named("workflowapp"),
	}
}

// SetRelationSyncer installs the relation Service post-construction (avoids DI cycle
// since relation Service consumes workflow reader too). Nil to disable.
//
// SetRelationSyncer 装配后注入 relation Service（避开 DI 循环——relation 也消费 workflow reader）。
// 传 nil 等于禁用。
func (s *Service) SetRelationSyncer(r RelationSyncer) {
	s.relations = r
}

// SetKeyProvider enables F1 validation on node modelOverride; call once at DI wire-up.
// Nil keyProvider tolerated (skips cross-user existence check).
//
// SetKeyProvider 启用节点 modelOverride F1 校验;装配阶段调一次。
// nil 兜底跳过跨用户存在性校验。
func (s *Service) SetKeyProvider(kp apikeydomain.KeyProvider) {
	s.keyProvider = kp
}

// WorkflowReader is the read-only contract Plan 05 consumes for active versions and enabled lists.
//
// WorkflowReader 是 Plan 05 消费的只读契约（active version + enabled 列表）。
type WorkflowReader interface {
	GetActiveVersion(ctx context.Context, workflowID string) (*workflowdomain.Version, error)
	GetWorkflow(ctx context.Context, workflowID string) (*workflowdomain.Workflow, error)
	ListEnabled(ctx context.Context) ([]*workflowdomain.Workflow, error)
}

var _ WorkflowReader = (*Service)(nil)
