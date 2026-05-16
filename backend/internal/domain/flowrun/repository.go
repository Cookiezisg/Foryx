package flowrun

import (
	"context"
	"time"
)

// Repository is the persistence port for FlowRun + Node.
//
// Repository 是 FlowRun + Node 的持久化端口。
type Repository interface {
	Create(ctx context.Context, run *FlowRun) error
	Get(ctx context.Context, id string) (*FlowRun, error)
	List(ctx context.Context, filter ListFilter) ([]*FlowRun, string, error)

	// UpdateStatus transitions a FlowRun; on terminal state also writes ended_at/elapsed/output/error.
	//
	// UpdateStatus 转 status；转终态时一并写 ended_at / elapsed / output / error 字段。
	UpdateStatus(ctx context.Context, runID, status string, output any, errCode, errMsg string, endedAt *time.Time, elapsedMs int64) error

	SetPausedState(ctx context.Context, runID string, ps *PausedState) error
	ClearPausedState(ctx context.Context, runID string) error

	// ListPaused returns all paused FlowRuns for the ctx user (boot rehydrate).
	//
	// ListPaused 返当前用户所有 paused FlowRun，启动时 RehydrateOnBoot 用。
	ListPaused(ctx context.Context) ([]*FlowRun, error)

	CountRunning(ctx context.Context, workflowID string) (int, error)

	// HardDeleteOldest hard-deletes FlowRuns beyond retention; keeps `keep` newest.
	//
	// HardDeleteOldest 物理删超出 keep 个最旧的 FlowRun。
	HardDeleteOldest(ctx context.Context, workflowID string, keep int) error

	CreateNode(ctx context.Context, node *Node) error
	GetNode(ctx context.Context, id string) (*Node, error)
	ListNodes(ctx context.Context, filter NodeFilter) ([]*Node, string, error)
}
