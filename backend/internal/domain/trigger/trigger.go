// Package trigger is the trigger-entity domain: a standalone signal source that fires
// when its source condition is met (cron tick / webhook hit / file change / sensor probe),
// fanning the signal out to every active workflow that listens to it. A trigger is a
// CONFIG entity — no version model, no sandbox, no env. Its listener runs only while at
// least one active workflow references it (reference-counted lifecycle, owned by app).
//
// Package trigger 是 trigger 实体域：独立的信号源，source 条件满足即 fire（cron 刻度 /
// webhook / 文件变 / sensor 探测），把信号扇给所有监听它的 active workflow。trigger 是
// **配置实体**——无版本、无 sandbox、无 env。listener 仅在 ≥1 个 active workflow 引用它时
// 运行（引用计数生命周期，由 app 持有）。
package trigger

import (
	"time"

	errorspkg "github.com/sunweilin/forgify/backend/internal/pkg/errors"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
)

// Source kinds. manual is intentionally absent — running a workflow by hand is the
// workflow's own ability (it listens to nothing), not a trigger source.
//
// Source 种类。故意没有 manual——手动跑 workflow 是 workflow 自己的能力（它不监听任何东西），不是 trigger source。
const (
	KindCron     = "cron"     // robfig/cron expression, fires on schedule tick
	KindWebhook  = "webhook"  // external HTTP push to a mounted path
	KindFsnotify = "fsnotify" // local filesystem change on a watched path
	KindSensor   = "sensor"   // periodically invoke a function/handler, fire when a CEL condition holds
)

// IsValidKind reports whether k is one of the 4 source kinds.
//
// IsValidKind 报告 k 是否 4 种 source 之一。
func IsValidKind(k string) bool {
	switch k {
	case KindCron, KindWebhook, KindFsnotify, KindSensor:
		return true
	}
	return false
}

// Trigger is the entity row. Config holds the source-specific settings (see config.go);
// it is kept as a free map so adding a source kind needs no column change.
//
// Trigger 是实体行。Config 持有 source 专属配置（见 config.go），用自由 map 存——加 source 种类无需改列。
type Trigger struct {
	ID          string            `db:"id,pk"              json:"id"`
	WorkspaceID string            `db:"workspace_id,ws"    json:"-"` // D2 物理隔离列,不上线缆
	Name        string            `db:"name"               json:"name"`
	Description string            `db:"description"        json:"description"`
	Kind        string            `db:"kind"               json:"kind"`
	Config      map[string]any    `db:"config,json"        json:"config"`
	Outputs     []schemapkg.Field `db:"outputs,json"       json:"outputs"` // declared payload fields delivered to listening workflows (downstream reads these)
	CreatedAt   time.Time         `db:"created_at,created" json:"createdAt"`
	UpdatedAt   time.Time         `db:"updated_at,updated" json:"updatedAt"`
	DeletedAt   *time.Time        `db:"deleted_at,deleted" json:"-"`

	// RefCount / Listening are computed at read time from the app's in-memory listen
	// registry (how many active workflows reference it / whether its listener is hot).
	// Not persisted — the persistent truth is the workflow side (who is active).
	//
	// RefCount / Listening 读时由 app 内存监听表算出（多少 active workflow 引用它 / listener 热否），不落库。
	RefCount  int  `db:"-" json:"refCount"`
	Listening bool `db:"-" json:"listening"`
}

// ListFilter paginates the trigger list.
//
// ListFilter 分页 trigger 列表。
type ListFilter struct {
	Cursor string
	Limit  int
}

// Domain errors. Wire codes are stable; Kind maps to HTTP status (errorspkg).
//
// Domain 错误。wire code 稳定；Kind 映射 HTTP status。
var (
	ErrNotFound              = errorspkg.New(errorspkg.KindNotFound, "TRIGGER_NOT_FOUND", "trigger not found")
	ErrDuplicateName         = errorspkg.New(errorspkg.KindConflict, "TRIGGER_NAME_DUPLICATE", "trigger name already exists")
	ErrInvalidKind           = errorspkg.New(errorspkg.KindUnprocessable, "TRIGGER_INVALID_KIND", "unknown trigger kind")
	ErrInvalidConfig         = errorspkg.New(errorspkg.KindUnprocessable, "TRIGGER_INVALID_CONFIG", "invalid trigger config")
	ErrInvalidCron           = errorspkg.New(errorspkg.KindUnprocessable, "TRIGGER_INVALID_CRON", "invalid cron expression — use a 5-field expression (minute granularity); @every and seconds are not supported")
	ErrInvalidCEL            = errorspkg.New(errorspkg.KindUnprocessable, "TRIGGER_INVALID_CEL", "invalid CEL expression")
	ErrInvalidInterval       = errorspkg.New(errorspkg.KindUnprocessable, "TRIGGER_INVALID_INTERVAL", "sensor interval below minimum")
	ErrSensorTargetRequired  = errorspkg.New(errorspkg.KindUnprocessable, "TRIGGER_SENSOR_TARGET_REQUIRED", "sensor requires a function or handler target")
	ErrWebhookSecretMismatch = errorspkg.New(errorspkg.KindUnauthorized, "TRIGGER_WEBHOOK_SECRET_MISMATCH", "webhook secret mismatch")
	ErrActivationNotFound    = errorspkg.New(errorspkg.KindNotFound, "TRIGGER_ACTIVATION_NOT_FOUND", "activation not found")
	ErrListenerUnavailable   = errorspkg.New(errorspkg.KindUnavailable, "TRIGGER_LISTENER_UNAVAILABLE", "trigger listener not available")
	// ErrFiringNotPending: a ClaimFiring lost the race — already claimed/terminal (consumed by scheduler, 波次 4).
	// ErrFiringNotPending：claim 竞争失败（已被认领/终态），波次 4 scheduler 消费。
	ErrFiringNotPending = errorspkg.New(errorspkg.KindConflict, "TRIGGER_FIRING_NOT_PENDING", "firing already claimed")
)
