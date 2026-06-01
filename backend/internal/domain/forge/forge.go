// Package forge defines the trinity-forging SSE protocol (4 closed event types).
//
// Package forge 定义 trinity 锻造 SSE 协议（4 个封闭事件类型）。
package forge

import (
	"errors"
	"fmt"

	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
)

const (
	OperationCreate = "create"
	OperationEdit   = "edit"
	OperationRevert = "revert"
	OperationDelete = "delete"
)

func IsValidOperation(op string) bool {
	switch op {
	case OperationCreate, OperationEdit, OperationRevert, OperationDelete:
		return true
	}
	return false
}

// IsValidScopeKind reports whether kind is a forge-able entity. Extended from trinity
// (function/handler/workflow) to 6 kinds including agent + document + skill (doc 11 §S2 CANON-X1).
// document/skill are not forge entities but their right-pane editing streams via forge SSE.
//
// IsValidScopeKind 报告 kind 是否走 forge SSE（quadrinity + document/skill，共 6 种）。
func IsValidScopeKind(kind string) bool {
	switch kind {
	case eventlogdomain.KindFunction, eventlogdomain.KindHandler, eventlogdomain.KindWorkflow,
		eventlogdomain.KindAgent, eventlogdomain.KindDocument, eventlogdomain.KindSkill:
		return true
	}
	return false
}

const (
	EnvAttemptInstalling = "installing"
	EnvAttemptFixing     = "fixing"
	EnvAttemptOK         = "ok"
	EnvAttemptFailed     = "failed"
)

const (
	CompletedOK        = "ok"
	CompletedFailed    = "failed"
	CompletedCancelled = "cancelled"
)

// Event is the protocol unit; 4 closed concrete types.
//
// Event 是协议单位；4 个封闭具体类型。
type Event interface {
	EventType() string
}

// Envelope wraps an Event with the bridge-assigned seq.
//
// Envelope 给 Event 套上 bridge 分配的 seq。
type Envelope struct {
	Seq   int64
	Event Event
}

// ForgeStarted opens a forge operation; pairs 1:1 with ForgeCompleted.
//
// ForgeStarted 锻造开头发，与 ForgeCompleted 1:1 配对。
type ForgeStarted struct {
	Scope          eventlogdomain.Scope `json:"scope"`
	Operation      string               `json:"operation"`
	ConversationID string               `json:"conversationId,omitempty"`
	ToolCallID     string               `json:"toolCallId,omitempty"`
}

func (ForgeStarted) EventType() string { return "forge_started" }

// ForgeOpApplied is emitted per applied op; declared for forward compatibility.
//
// ForgeOpApplied 单 op 应用后发；当前 Service.ApplyOps 暂未暴露 per-op 回调。
type ForgeOpApplied struct {
	Scope eventlogdomain.Scope `json:"scope"`
	Index int                  `json:"index"`
	Op    string               `json:"op"`
}

func (ForgeOpApplied) EventType() string { return "forge_op_applied" }

// ForgeEnvAttempt is emitted per env install attempt (initial + LLM-fix retries).
//
// ForgeEnvAttempt 每次装环境尝试发（初次 + LLM 修建议的重试）。
type ForgeEnvAttempt struct {
	Scope   eventlogdomain.Scope `json:"scope"`
	Attempt int                  `json:"attempt"`
	Status  string               `json:"status"`
	Stage   string               `json:"stage,omitempty"`
	Detail  string               `json:"detail,omitempty"`
	Error   string               `json:"error,omitempty"`
}

func (ForgeEnvAttempt) EventType() string { return "forge_env_attempt" }

// ForgeCompleted closes a forge operation; pairs 1:1 with ForgeStarted.
//
// ForgeCompleted 锻造结束发，与 ForgeStarted 1:1 配对。
type ForgeCompleted struct {
	Scope        eventlogdomain.Scope `json:"scope"`
	Status       string               `json:"status"`
	VersionID    string               `json:"versionId,omitempty"`
	EnvStatus    string               `json:"envStatus,omitempty"`
	AttemptsUsed int                  `json:"attemptsUsed,omitempty"`
	Error        string               `json:"error,omitempty"`
}

func (ForgeCompleted) EventType() string { return "forge_completed" }

// ErrInvalidEvent is returned for malformed events (producer bug).
//
// ErrInvalidEvent 形状错误事件（Producer bug）。
var ErrInvalidEvent = errors.New("forge: invalid event")

// ErrSeqTooOld is returned when fromSeq has been evicted from the replay buffer.
//
// ErrSeqTooOld 在 fromSeq 已被 replay buffer 淘汰时返。
var ErrSeqTooOld = errors.New("forge: requested seq too old (evicted from replay buffer)")

// ValidateEvent runs minimal shape checks; Bridge implementations call this in Publish.
//
// ValidateEvent 跑最小形状检查；Bridge 在 Publish 中调用，让 producer bug 在边界暴露。
func ValidateEvent(e Event) error {
	switch v := e.(type) {
	case ForgeStarted:
		if err := validateScope(v.Scope); err != nil {
			return err
		}
		if !IsValidOperation(v.Operation) {
			return fmt.Errorf("%w: forge_started: unknown operation %q", ErrInvalidEvent, v.Operation)
		}
	case ForgeOpApplied:
		if err := validateScope(v.Scope); err != nil {
			return err
		}
		if v.Op == "" {
			return fmt.Errorf("%w: forge_op_applied: empty op name", ErrInvalidEvent)
		}
	case ForgeEnvAttempt:
		if err := validateScope(v.Scope); err != nil {
			return err
		}
		if v.Attempt <= 0 {
			return fmt.Errorf("%w: forge_env_attempt: attempt must be >= 1, got %d", ErrInvalidEvent, v.Attempt)
		}
		if !isValidEnvAttemptStatus(v.Status) {
			return fmt.Errorf("%w: forge_env_attempt: unknown status %q", ErrInvalidEvent, v.Status)
		}
	case ForgeCompleted:
		if err := validateScope(v.Scope); err != nil {
			return err
		}
		if !isValidCompletedStatus(v.Status) {
			return fmt.Errorf("%w: forge_completed: unknown status %q", ErrInvalidEvent, v.Status)
		}
	default:
		return fmt.Errorf("%w: unknown event type %T", ErrInvalidEvent, e)
	}
	return nil
}

func validateScope(s eventlogdomain.Scope) error {
	if !IsValidScopeKind(s.Kind) {
		return fmt.Errorf("%w: scope.kind %q is not forge-able (must be function/handler/workflow)",
			ErrInvalidEvent, s.Kind)
	}
	if s.ID == "" {
		return fmt.Errorf("%w: scope.id is empty", ErrInvalidEvent)
	}
	return nil
}

func isValidEnvAttemptStatus(s string) bool {
	switch s {
	case EnvAttemptInstalling, EnvAttemptFixing, EnvAttemptOK, EnvAttemptFailed:
		return true
	}
	return false
}

func isValidCompletedStatus(s string) bool {
	switch s {
	case CompletedOK, CompletedFailed, CompletedCancelled:
		return true
	}
	return false
}
