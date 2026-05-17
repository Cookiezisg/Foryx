// Package trigger is the workflow-trigger domain (listener types + spec/state).
//
// Package trigger 是 workflow 触发器域（listener 类型 + spec/state）。
package trigger

import (
	"errors"
	"time"
)

const (
	KindCron     = "cron"
	KindFsnotify = "fsnotify"
	KindWebhook  = "webhook"
	KindManual   = "manual"
)

const (
	StateActive = "active"
	StateIdle   = "idle"
	StateError  = "error"
)

// Spec is the normalized trigger configuration extracted from a workflow trigger node.
//
// Spec 是从 workflow trigger 节点解出的规范化触发器配置。
type Spec struct {
	WorkflowID string         `json:"workflowId"`
	UserID     string         `json:"userId"` // owner of the workflow; populated at registration so onFire fires with correct ctx
	NodeID     string         `json:"nodeId"`
	Kind       string         `json:"kind"`
	Config     map[string]any `json:"config"`
}

// State is the runtime state of one registered trigger (powers GET /workflows/{id}/triggers).
//
// State 是已注册触发器的 runtime 状态（供 GET /workflows/{id}/triggers 端点）。
type State struct {
	WorkflowID  string     `json:"workflowId"`
	NodeID      string     `json:"nodeId"`
	Kind        string     `json:"kind"`
	Status      string     `json:"status"`
	LastFiredAt *time.Time `json:"lastFiredAt,omitempty"`
	NextFireAt  *time.Time `json:"nextFireAt,omitempty"`
	LastError   string     `json:"lastError,omitempty"`
}

var (
	ErrPathNotExist          = errors.New("trigger: fsnotify path not exist")
	ErrPathConflict          = errors.New("trigger: webhook path conflict")
	ErrWebhookSecretMismatch = errors.New("trigger: webhook secret mismatch")
	ErrInvalidCronExpression = errors.New("trigger: invalid cron expression")
	ErrInvalidConfig         = errors.New("trigger: invalid config")
)
