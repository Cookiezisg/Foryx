package trigger

import "context"

// Repository persists triggers (soft-deleted) + the firings inbox + the activation log.
// The single-tx claim (pending→claimed + flowrun INSERT) is NOT here — it spans the
// flowruns table (波次 4) and must not leak *orm.DB into a domain port; the store exposes
// it as a concrete method for the scheduler to call.
//
// Repository 持久化 triggers（软删）+ firings 收件箱 + activation 日志。单事务 claim
// （pending→claimed + 建 flowrun）不在此——它跨 flowruns 表（波次 4）、不该把 *orm.DB 漏进
// domain 端口；store 以具体方法暴露给 scheduler。
type Repository interface {
	// triggers
	SaveTrigger(ctx context.Context, t *Trigger) error
	GetTrigger(ctx context.Context, id string) (*Trigger, error)
	GetTriggerByName(ctx context.Context, name string) (*Trigger, error)
	GetTriggersByIDs(ctx context.Context, ids []string) ([]*Trigger, error)
	ListTriggers(ctx context.Context, filter ListFilter) ([]*Trigger, string, error)
	ListAllTriggers(ctx context.Context) ([]*Trigger, error)
	DeleteTrigger(ctx context.Context, id string) error

	// firings inbox (persist-before-act). AppendFiring is idempotent on the dedup key.
	// firings 收件箱（先持久化再动作）。AppendFiring 按 dedup key 幂等。
	AppendFiring(ctx context.Context, f *Firing) (*Firing, error)
	ListPendingFirings(ctx context.Context, limit int) ([]*Firing, error)
	// SearchFirings pages a trigger's firing inbox (the disposition surface: started /
	// skipped / superseded / shed). SearchFirings 分页 trigger 的 firing 收件箱（处置面）。
	SearchFirings(ctx context.Context, filter FiringFilter) ([]*Firing, string, error)
	MarkFiringOutcome(ctx context.Context, firingID, status string) error

	// activation log (append-only; D1 no delete).
	// activation 日志（只增；D1 不删）。
	AppendActivation(ctx context.Context, a *Activation) error
	GetActivation(ctx context.Context, id string) (*Activation, error)
	SearchActivations(ctx context.Context, filter ActivationFilter) ([]*Activation, string, error)
}
