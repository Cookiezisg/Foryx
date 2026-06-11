package trigger

import "time"

// Firing is the durable inbox row — persist-before-act: written the moment a trigger
// fires, before any flowrun starts. A single fire fans out to one Firing per listening
// workflow. The scheduler (波次 4) drains pending firings, claiming each in one tx
// (pending→claimed→started) so there is never a claimed-but-no-flowrun strand (ADR-021).
// Terminal status IS the outcome ("every firing reaches a terminal status").
//
// Firing 是 durable 收件箱行——先持久化再动作：trigger fire 的瞬间就写，早于任何 flowrun。
// 一次 fire 按监听的 workflow 扇出成多条 Firing。scheduler（波次 4）排空 pending、单事务 claim
// 每条（pending→claimed→started），无 claimed-但-无-flowrun 残留态。终态 status 即 outcome。
type Firing struct {
	ID           string         `db:"id,pk"`
	WorkspaceID  string         `db:"workspace_id,ws"`
	TriggerID    string         `db:"trigger_id"`
	WorkflowID   string         `db:"workflow_id"`
	ActivationID string         `db:"activation_id"`
	Payload      map[string]any `db:"payload,json"`
	DedupKey     string         `db:"dedup_key"`
	Status       string         `db:"status"`
	FlowrunID    string         `db:"flowrun_id"`
	CreatedAt    time.Time      `db:"created_at,created"` // enqueue time — drained oldest-first
	UpdatedAt    time.Time      `db:"updated_at,updated"`
}

// Firing lifecycle+disposition — a single status enum, no separate outcome column.
//
// Firing 生命周期+处置——单一 status 枚举，无独立 outcome 列。
const (
	FiringPending    = "pending"    // written, awaiting scheduler claim
	FiringClaimed    = "claimed"    // claimed in the single tx (transient, inside the claim)
	FiringStarted    = "started"    // claimed + flowrun created (terminal-ok)
	FiringSkipped    = "skipped"    // overlap policy Skip
	FiringSuperseded = "superseded" // overlap policy buffer_one replaced it (v2)
	FiringShed       = "shed"       // resource cap
)
