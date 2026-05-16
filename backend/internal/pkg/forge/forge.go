// Package forge provides the producer-side Publisher helper around the forge Bridge.
//
// Package forge 提供 forge Bridge 的 producer 侧 Publisher helper。
package forge

import (
	"context"

	"go.uber.org/zap"

	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
)

// Publisher is the high-level API for emitting forge events; failures are best-effort log-only.
//
// Publisher 是 forge 事件的高层 API；失败仅 log，不阻断底层操作。
type Publisher interface {
	PublishStarted(ctx context.Context, scope eventlogdomain.Scope, operation, convID, toolCallID string)
	PublishOpApplied(ctx context.Context, scope eventlogdomain.Scope, index int, op string)
	PublishEnvAttempt(ctx context.Context, scope eventlogdomain.Scope, attempt int, status, stage, detail string, err error)
	PublishCompleted(ctx context.Context, scope eventlogdomain.Scope, status, versionID, envStatus string, attemptsUsed int, err error)
}

// New constructs a Publisher backed by bridge; bridge nil returns a no-op.
//
// New 构造由 bridge 支撑的 Publisher；bridge 为 nil 返 no-op。
func New(bridge forgedomain.Bridge, log *zap.Logger) Publisher {
	if bridge == nil {
		return noopPublisher{}
	}
	if log == nil {
		log = zap.NewNop()
	}
	return &publisher{bridge: bridge, log: log.Named("forge.publisher")}
}

type publisher struct {
	bridge forgedomain.Bridge
	log    *zap.Logger
}

func (p *publisher) PublishStarted(ctx context.Context, scope eventlogdomain.Scope, operation, convID, toolCallID string) {
	p.emit(ctx, forgedomain.ForgeStarted{
		Scope:          scope,
		Operation:      operation,
		ConversationID: convID,
		ToolCallID:     toolCallID,
	})
}

func (p *publisher) PublishOpApplied(ctx context.Context, scope eventlogdomain.Scope, index int, op string) {
	p.emit(ctx, forgedomain.ForgeOpApplied{
		Scope: scope, Index: index, Op: op,
	})
}

func (p *publisher) PublishEnvAttempt(ctx context.Context, scope eventlogdomain.Scope, attempt int, status, stage, detail string, err error) {
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	p.emit(ctx, forgedomain.ForgeEnvAttempt{
		Scope: scope, Attempt: attempt, Status: status, Stage: stage, Detail: detail, Error: errStr,
	})
}

func (p *publisher) PublishCompleted(ctx context.Context, scope eventlogdomain.Scope, status, versionID, envStatus string, attemptsUsed int, err error) {
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	p.emit(ctx, forgedomain.ForgeCompleted{
		Scope: scope, Status: status, VersionID: versionID, EnvStatus: envStatus,
		AttemptsUsed: attemptsUsed, Error: errStr,
	})
}

func (p *publisher) emit(ctx context.Context, e forgedomain.Event) {
	if _, err := p.bridge.Publish(ctx, e); err != nil {
		p.log.Warn("forge publish failed",
			zap.String("type", e.EventType()),
			zap.Error(err))
	}
}

type noopPublisher struct{}

func (noopPublisher) PublishStarted(context.Context, eventlogdomain.Scope, string, string, string) {
}
func (noopPublisher) PublishOpApplied(context.Context, eventlogdomain.Scope, int, string) {}
func (noopPublisher) PublishEnvAttempt(context.Context, eventlogdomain.Scope, int, string, string, string, error) {
}
func (noopPublisher) PublishCompleted(context.Context, eventlogdomain.Scope, string, string, string, int, error) {
}
