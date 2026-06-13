package trigger

import "time"

// Activation is the per-action audit log — ONE row every time a trigger does something,
// fired or NOT. This is what makes "why didn't it fire?" answerable: for a sensor that
// probed but didn't trigger, ReturnValue records what the function/handler returned and
// Error/Detail says why (condition false vs invoke error). Firing is only the fired path;
// Activation is the whole story. A non-fired Activation produces 0 Firings; a fired one
// produces FiringCount (fan-out width).
//
// Activation 是逐动作审计日志——trigger 每做一次事就一行，**触没触发都记**。这让「为什么没触发」
// 可查：sensor 探测但没触发时，ReturnValue 记下 function/handler 返回了什么、Error/Detail 说明
// 原因（条件 false 还是调用出错）。Firing 只是触发路径；Activation 是全程。没触发的 Activation
// 产 0 条 Firing，触发的产 FiringCount 条（扇出宽度）。
type Activation struct {
	ID          string         `db:"id,pk"               json:"id"`
	WorkspaceID string         `db:"workspace_id,ws"     json:"-"`
	TriggerID   string         `db:"trigger_id"          json:"triggerId"`
	Kind        string         `db:"kind"                json:"kind"`
	Fired       bool           `db:"fired"               json:"fired"`
	ReturnValue map[string]any `db:"return_value,json"   json:"returnValue,omitempty"` // sensor: what the probe returned (kept even when not fired)
	Payload     map[string]any `db:"payload,json"        json:"payload,omitempty"`     // the payload fired out (empty when not fired)
	Error       string         `db:"error"               json:"error,omitempty"`       // invoke/probe error (empty on success)
	Detail      string         `db:"detail"              json:"detail,omitempty"`      // human-readable note, e.g. "condition evaluated false"
	FiringCount int            `db:"firing_count"        json:"firingCount"`           // how many workflows it fanned out to
	CreatedAt   time.Time      `db:"created_at,created"  json:"createdAt"`             // when the action occurred
}

// ActivationFilter queries the activation log for one trigger (newest first), optionally
// only the misses (FiredOnly is the opposite — only the hits).
//
// ActivationFilter 查某 trigger 的 activation 日志（最新优先），FiredOnly 只看触发的。
type ActivationFilter struct {
	TriggerID string
	FiredOnly bool
	Cursor    string
	Limit     int
}
