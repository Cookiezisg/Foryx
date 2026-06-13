// Package approval is the domain layer for approval-rendering entities (apf_): a named,
// versioned "approval form" — a markdown prompt template (with `{{ CEL }}` interpolation)
// plus the decision rules (allowReason / timeout / timeoutBehavior). A workflow's approval
// node references an ApprovalForm by id; the graph wires its fixed yes/no exits to
// downstream nodes. Like control it owns an append-only line of Versions with a free-moving
// ActiveVersionID pointer — no pending/accept, no sandbox/env/executions.
//
// NB: the prefix is apf_/apfv_, NOT apv_ — apv_ belongs to the `approvals` runtime table
// (parked/approved/... per-flowrun records, 波次 4). The form (config) and the runtime
// record are two different things, mirroring trigger entity (trg_) vs trigger_firings.
//
// CEL is NOT compiled here (domain must not import cel-go, 原则 #3) — the app layer compiles
// the template's `{{ CEL }}` spans via pkg/cel at create/edit time.
//
// Package approval 是审批渲染实体（apf_）的 domain 层：一个命名、版本化的「审批表」——markdown
// prompt 模板（含 `{{ CEL }}` 插值）+ 决策规则（allowReason / timeout / timeoutBehavior）。
// workflow 的 approval 节点按 id 引用 ApprovalForm；图把它固定的 yes/no 出口连到下游。与 control
// 同：持只增 Version 线 + 自由 ActiveVersionID 指针——无 pending/accept、无 sandbox/env/executions。
//
// 注：前缀 apf_/apfv_，**非** apv_——apv_ 属 `approvals` 运行时表（per-flowrun 的 parked/approved
// 记录，波次 4）。表（配置）与运行时记录是两回事，对位 trigger 实体（trg_）vs trigger_firings。
//
// CEL 不在此编译（domain 不准 import cel-go，原则 #3）——app 层 create/edit 时用 pkg/cel 编译模板的
// `{{ CEL }}` 段。
package approval

import (
	"strconv"
	"strings"
	"time"

	errorspkg "github.com/sunweilin/forgify/backend/internal/pkg/errors"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
)

// ApprovalForm is an approval-rendering entity; its template + rules live on the active Version.
//
// ApprovalForm 是审批渲染实体；模板 + 规则在 active Version 上，不在本表。
type ApprovalForm struct {
	ID              string     `db:"id,pk"               json:"id"`
	WorkspaceID     string     `db:"workspace_id,ws"     json:"-"`
	Name            string     `db:"name"                json:"name"`
	Description     string     `db:"description"         json:"description"`
	ActiveVersionID string     `db:"active_version_id"   json:"activeVersionId"`
	CreatedAt       time.Time  `db:"created_at,created"  json:"createdAt"`
	UpdatedAt       time.Time  `db:"updated_at,updated"  json:"updatedAt"`
	DeletedAt       *time.Time `db:"deleted_at,deleted"  json:"-"`

	// ActiveVersion is a computed (non-column) field attached by Service.Get.
	//
	// ActiveVersion 是计算字段（非列），由 Service.Get 附上。
	ActiveVersion *Version `db:"-" json:"activeVersion,omitempty"`
}

// Version is one immutable snapshot of an approval form's prompt + decision rules.
//
// Version 是审批表的 prompt + 决策规则的一份不可变快照。
type Version struct {
	ID                     string            `db:"id,pk"                     json:"id"`
	WorkspaceID            string            `db:"workspace_id,ws"           json:"-"`
	ApprovalID             string            `db:"approval_id"               json:"approvalId"`
	Version                int               `db:"version"                   json:"version"`
	Inputs                 []schemapkg.Field `db:"inputs,json"               json:"inputs"`          // declared inputs the workflow node feeds; template reads input.*
	Template               string            `db:"template"                  json:"template"`        // markdown，含 {{ input.* }} 插值
	AllowReason            bool              `db:"allow_reason"              json:"allowReason"`     // 是否允许填备注
	Timeout                string            `db:"timeout"                   json:"timeout"`         // duration（"30d"）；"" = 永不超时
	TimeoutBehavior        string            `db:"timeout_behavior"          json:"timeoutBehavior"` // reject|approve|fail（timeout 非空必填）
	ChangeReason           string            `db:"change_reason"             json:"changeReason,omitempty"`
	ForgedInConversationID *string           `db:"forged_in_conversation_id" json:"forgedInConversationId,omitempty"`
	CreatedAt              time.Time         `db:"created_at,created"        json:"createdAt"`
	UpdatedAt              time.Time         `db:"updated_at,updated"        json:"updatedAt"`
}

// Timeout behaviors: what happens when a parked approval's deadline passes (波次 4 runtime).
//
// 超时行为：parked 审批到期时怎么处理（波次 4 运行时）。
const (
	TimeoutReject  = "reject"
	TimeoutApprove = "approve"
	TimeoutFail    = "fail"
)

// IsValidTimeoutBehavior reports whether b is one of the 3 behaviors.
//
// IsValidTimeoutBehavior 报告 b 是否 3 种行为之一。
func IsValidTimeoutBehavior(b string) bool {
	switch b {
	case TimeoutReject, TimeoutApprove, TimeoutFail:
		return true
	}
	return false
}

// ParseTimeout parses a duration string, extending time.ParseDuration with d (days) and w
// (weeks) — approval timeouts are coarse (e.g. "30d"). "" is valid (never times out) → 0.
//
// ParseTimeout 解析 duration 串，在 time.ParseDuration 基础上支持 d（天）/ w（周）——审批超时
// 粒度粗（如 "30d"）。"" 合法（永不超时）→ 0。
func ParseTimeout(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if n, ok := strings.CutSuffix(s, "d"); ok {
		days, err := strconv.Atoi(n)
		if err != nil || days < 0 {
			return 0, ErrInvalidTimeout
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	if n, ok := strings.CutSuffix(s, "w"); ok {
		weeks, err := strconv.Atoi(n)
		if err != nil || weeks < 0 {
			return 0, ErrInvalidTimeout
		}
		return time.Duration(weeks) * 7 * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, ErrInvalidTimeout
	}
	return d, nil
}

// ValidateForm checks structural validity only (CEL template compile is the app layer's
// job, 原则 #3): template non-empty (an approval with no prompt is meaningless — the user
// sees a bare button); if timeout is set, timeoutBehavior must be valid AND timeout must
// parse. An empty timeout means "never times out" (no behavior needed).
//
// ValidateForm 仅校验结构（CEL 模板编译归 app 层，原则 #3）：template 非空（无说明的审批无意义
// ——用户看到孤零零按钮）；timeout 非空时 timeoutBehavior 必须合法**且** timeout 可解析。空 timeout
// = 永不超时（无需 behavior）。
func ValidateForm(template, timeout, timeoutBehavior string) error {
	if strings.TrimSpace(template) == "" {
		return ErrInvalidTemplate
	}
	if strings.TrimSpace(timeout) != "" {
		if !IsValidTimeoutBehavior(timeoutBehavior) {
			return ErrInvalidTimeout
		}
		if _, err := ParseTimeout(timeout); err != nil {
			return err
		}
	} else if strings.TrimSpace(timeoutBehavior) != "" && !IsValidTimeoutBehavior(timeoutBehavior) {
		// A stray behavior without a timeout is inert today but poisons the row the moment
		// a timeout is added — reject garbage at the door, not at the deadline.
		// 没 timeout 的孤 behavior 今天无害，但一旦补上 timeout 就毒化该行——垃圾在门口拒，
		// 别等到截止那一刻。
		return ErrInvalidTimeout
	}
	return nil
}

var (
	// ErrNotFound: approval form id miss (scoped to workspace).
	// ErrNotFound：审批表 id 未命中（按 workspace 隔离）。
	ErrNotFound = errorspkg.New(errorspkg.KindNotFound, "APPROVAL_NOT_FOUND", "approval form not found")

	// ErrDuplicateName: a live approval form already owns this name in the workspace.
	// ErrDuplicateName：workspace 内已有同名活跃审批表。
	ErrDuplicateName = errorspkg.New(errorspkg.KindConflict, "APPROVAL_NAME_DUPLICATE", "approval form name already exists")

	// ErrVersionNotFound: version id / number miss.
	// ErrVersionNotFound：version id / 号未命中。
	ErrVersionNotFound = errorspkg.New(errorspkg.KindNotFound, "APPROVAL_VERSION_NOT_FOUND", "approval form version not found")

	// ErrNoActiveVersion: approval form has no active version.
	// ErrNoActiveVersion：审批表无 active 版本。
	ErrNoActiveVersion = errorspkg.New(errorspkg.KindUnprocessable, "APPROVAL_NO_ACTIVE_VERSION", "approval form has no active version")

	// ErrInvalidName: name empty / malformed.
	// ErrInvalidName：name 空 / 畸形。
	ErrInvalidName = errorspkg.New(errorspkg.KindUnprocessable, "APPROVAL_INVALID_NAME", "invalid approval form name")

	// ErrInvalidTemplate: template empty, or its `{{ CEL }}` spans failed to compile (app maps the latter).
	// ErrInvalidTemplate：template 空，或其 `{{ CEL }}` 段编译失败（后者 app 映射）。
	ErrInvalidTemplate = errorspkg.New(errorspkg.KindUnprocessable, "APPROVAL_INVALID_TEMPLATE", "approval template empty or its {{ CEL }} failed to compile")

	// ErrInvalidTimeout: timeout not a valid duration, or set without a valid timeoutBehavior.
	// ErrInvalidTimeout：timeout 非合法 duration，或设了 timeout 却无合法 timeoutBehavior。
	ErrInvalidTimeout = errorspkg.New(errorspkg.KindUnprocessable, "APPROVAL_INVALID_TIMEOUT", "invalid timeout duration or missing/invalid timeoutBehavior")
)
