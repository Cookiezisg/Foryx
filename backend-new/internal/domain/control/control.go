// Package control is the domain layer for control-logic entities (ctl_): a named,
// versioned set of ordered routing branches. A workflow's control node references a
// ControlLogic by id; the graph wires each branch's Port to a downstream node, while
// the branch's When (bool CEL guard, first-true-wins) and Emit (field→CEL reshape of
// the downstream payload) live here. Like function it owns an append-only line of
// Versions with a free-moving ActiveVersionID pointer — no pending/accept. Unlike
// function there is no sandbox/env/executions: a control node is pure control flow,
// evaluated by the durable interpreter (波次 4), never an activity. CEL is NOT compiled
// here (domain must not import cel-go, 原则 #3) — the app layer compiles when/emit via
// pkg/cel at create/edit time.
//
// Package control 是 control 逻辑实体（ctl_）的 domain 层：一组命名、版本化的有序路由分支。
// workflow 的 control 节点按 id 引用 ControlLogic；图把每个分支的 Port 连到下游，分支的 When
// （布尔 CEL 守卫，first-true-wins）与 Emit（字段→CEL 重塑下游 payload）在此。与 function 同：
// 持一条只增 Version 线 + 可自由移动的 ActiveVersionID 指针——无 pending/accept。与 function
// 异：无 sandbox/env/executions——control 节点是纯控制流，由 durable 解释器（波次 4）求值，绝非
// activity。CEL 不在此编译（domain 不准 import cel-go，原则 #3）——app 层 create/edit 时用
// pkg/cel 编译 when/emit。
package control

import (
	"strings"
	"time"

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
)

// ControlLogic is a control-logic entity; its branches live on the active Version.
//
// ControlLogic 是一个 control 逻辑实体；分支在 active Version 上，不在本表。
type ControlLogic struct {
	ID              string     `db:"id,pk"               json:"id"`
	WorkspaceID     string     `db:"workspace_id,ws"     json:"-"`
	Name            string     `db:"name"                json:"name"`
	Description     string     `db:"description"         json:"description"`
	ActiveVersionID string     `db:"active_version_id"   json:"activeVersionId"`
	CreatedAt       time.Time  `db:"created_at,created"  json:"createdAt"`
	UpdatedAt       time.Time  `db:"updated_at,updated"  json:"updatedAt"`
	DeletedAt       *time.Time `db:"deleted_at,deleted"  json:"-"`

	// ActiveVersion is a computed (non-column) field attached by Service.Get so a reader
	// sees the current branches in one round-trip.
	//
	// ActiveVersion 是计算字段（非列），由 Service.Get 附上，使读者一趟拿到当前分支。
	ActiveVersion *Version `db:"-" json:"activeVersion,omitempty"`
}

// Version is one immutable snapshot of a control logic's branch set. Version is a
// monotonic counter assigned at write time (max+1) — never reassigned, never renumbered.
//
// Version 是 control 逻辑分支组的一份不可变快照。Version 是写入时分配的单调号（max+1）——绝不
// 重分配、绝不重排号。
type Version struct {
	ID                     string    `db:"id,pk"                     json:"id"`
	WorkspaceID            string    `db:"workspace_id,ws"           json:"-"`
	ControlID              string            `db:"control_id"                json:"controlId"`
	Version                int               `db:"version"                   json:"version"`
	InputSchema            []schemapkg.Field `db:"input_schema,json"         json:"inputSchema"` // declared inputs the workflow node feeds; when/emit read input.*
	Branches               []Branch          `db:"branches,json"             json:"branches"`
	ChangeReason           string    `db:"change_reason"             json:"changeReason,omitempty"`
	ForgedInConversationID *string   `db:"forged_in_conversation_id" json:"forgedInConversationId,omitempty"`
	CreatedAt              time.Time `db:"created_at,created"        json:"createdAt"`
	UpdatedAt              time.Time `db:"updated_at,updated"        json:"updatedAt"`
}

// Branch is one ordered routing arm. The interpreter evaluates When (bool CEL over input.*)
// top to bottom; the first true wins. Emit (optional, field→CEL over input.*) builds this
// branch's output data (empty = pass input through). Port is the named outcome the workflow
// graph routes on: an edge with FromPort==this Port carries the branch's output to its
// downstream node (a port may loop back upstream = structured loop). The last branch must be
// When=="true" (catch-all, so an all-false fall-through still has a route). The control is
// thus (input) → (Port, Emit-data); the entity never knows which downstream node a Port
// connects to — that's the workflow's job.
//
// Branch 是一条有序路由臂。解释器自上而下求 When（读 input.* 的布尔 CEL），first-true-wins。Emit
// （可选，字段→读 input.* 的 CEL）构造本臂输出数据（空=透传 input）。Port 是具名结局，workflow 图据
// 此路由：FromPort==此 Port 的边把本臂输出带到其下游（出口可连回上游=结构化循环）。末条必须
// When=="true"（兜底）。故 control = (input) → (Port, Emit 数据)；实体永不知道 Port 连哪个下游——那是
// workflow 的事。
type Branch struct {
	Port string            `json:"port"`
	When string            `json:"when"`
	Emit map[string]string `json:"emit,omitempty"`
}

// ValidateBranches checks structural validity only (CEL syntax is the app layer's job,
// 原则 #3): non-empty; every Port non-empty and unique (the graph must address each exit
// distinctly); the last branch is the catch-all When=="true".
//
// ValidateBranches 仅校验结构（CEL 语法归 app 层，原则 #3）：非空；每个 Port 非空且唯一（图要
// 可区分地寻址每个出口）；末条是兜底 When=="true"。
func ValidateBranches(branches []Branch) error {
	if len(branches) == 0 {
		return ErrInvalidBranches
	}
	seen := make(map[string]bool, len(branches))
	for _, b := range branches {
		if strings.TrimSpace(b.Port) == "" || seen[b.Port] {
			return ErrInvalidBranches
		}
		seen[b.Port] = true
	}
	if strings.TrimSpace(branches[len(branches)-1].When) != "true" {
		return ErrNoCatchAll
	}
	return nil
}

var (
	// ErrNotFound: control id miss (scoped to workspace).
	// ErrNotFound：control id 未命中（按 workspace 隔离）。
	ErrNotFound = errorsdomain.New(errorsdomain.KindNotFound, "CONTROL_NOT_FOUND", "control logic not found")

	// ErrDuplicateName: a live control logic already owns this name in the workspace.
	// ErrDuplicateName：workspace 内已有同名活跃 control 逻辑。
	ErrDuplicateName = errorsdomain.New(errorsdomain.KindConflict, "CONTROL_NAME_DUPLICATE", "control logic name already exists")

	// ErrVersionNotFound: version id / number miss.
	// ErrVersionNotFound：version id / 号未命中。
	ErrVersionNotFound = errorsdomain.New(errorsdomain.KindNotFound, "CONTROL_VERSION_NOT_FOUND", "control logic version not found")

	// ErrNoActiveVersion: control logic has no active version.
	// ErrNoActiveVersion：control 逻辑无 active 版本。
	ErrNoActiveVersion = errorsdomain.New(errorsdomain.KindUnprocessable, "CONTROL_NO_ACTIVE_VERSION", "control logic has no active version")

	// ErrInvalidName: name empty / malformed.
	// ErrInvalidName：name 空 / 畸形。
	ErrInvalidName = errorsdomain.New(errorsdomain.KindUnprocessable, "CONTROL_INVALID_NAME", "invalid control logic name")

	// ErrInvalidBranches: branches empty, or a port is empty / duplicated.
	// ErrInvalidBranches：branches 空，或 port 空 / 重复。
	ErrInvalidBranches = errorsdomain.New(errorsdomain.KindUnprocessable, "CONTROL_INVALID_BRANCHES", "branches empty, or port empty/duplicate")

	// ErrNoCatchAll: the last branch is not the When=="true" catch-all.
	// ErrNoCatchAll：末条不是 When=="true" 兜底。
	ErrNoCatchAll = errorsdomain.New(errorsdomain.KindUnprocessable, "CONTROL_NO_CATCHALL", `last branch must be when:"true"`)

	// ErrInvalidCEL: a branch When/Emit expression failed to compile (the app layer maps this).
	// ErrInvalidCEL：分支 When/Emit 表达式编译失败（app 层映射）。
	ErrInvalidCEL = errorsdomain.New(errorsdomain.KindUnprocessable, "CONTROL_INVALID_CEL", "branch when/emit failed to compile")
)
