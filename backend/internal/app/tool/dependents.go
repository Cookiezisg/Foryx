package tool

import "context"

// DependentCounter reports how many live entities reference (mounted/linked) a given entity — the
// honest "what breaks if I delete this" signal (incoming equip/link relation edges). The relation
// app Service satisfies it. A delete tool reads this BEFORE deleting (the purge erases the edges)
// so its result can warn how many dependents may now fail (F48). One shared port instead of a
// per-package copy, mirroring how ToJSON is shared.
//
// DependentCounter 报告有多少存活实体引用（挂载/外链）了某实体——诚实的「删了它什么会坏」信号
// （入向 equip/link 边）。relation app Service 满足之。delete 工具在删**前**读它（purge 会抹掉边），
// 使结果能警示有多少依赖可能失效（F48）。一个共享端口、非各包各抄一份（同 ToJSON 共享思路）。
type DependentCounter interface {
	CountDependents(ctx context.Context, kind, id string) (int, error)
}

// DependentCount returns how many live entities reference (kind,id) via equip/link edges, or 0 when
// the counter is nil (delete tool wired without relations) or the read fails — advisory only, a
// delete must never fail because the dependent-count read did.
//
// DependentCount 返回有多少存活实体经 equip/link 边引用 (kind,id)，counter 为 nil（delete 工具未接
// relations）或读失败时返 0——仅 advisory，绝不因依赖计数读失败而让 delete 失败。
func DependentCount(ctx context.Context, counter DependentCounter, kind, id string) int {
	if counter == nil {
		return 0
	}
	n, err := counter.CountDependents(ctx, kind, id)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// AnnotateDependents adds a dependents count + repair note to a delete tool's result map when any
// live entity still references the just-deleted one — so the agent learns what may now break (and
// that the fix is to repair those referencing entities, since the edges are already purged). deps==0
// → the map is returned unchanged (no false alarm). Each delete tool keeps its own result shape and
// just folds this annotation in.
//
// AnnotateDependents 在仍有存活实体引用刚删实体时，给 delete 工具结果 map 加依赖数 + 修复提示——
// 使 agent 知道什么可能因此坏（且修法是修那些引用方，因为边已被 purge）。deps==0 → map 原样返回
// （不虚惊）。各 delete 工具保留自己的结果形状、只折入此标注。
func AnnotateDependents(out map[string]any, deps int) map[string]any {
	if deps > 0 {
		out["dependents"] = deps
		out["note"] = dependentsNote
	}
	return out
}

// DependentSuffix is the string-result counterpart of AnnotateDependents for delete tools that
// return a human sentence (e.g. delete_agent) rather than a JSON map. Empty when deps==0.
//
// DependentSuffix 是 AnnotateDependents 的字符串结果对应物，供返回人话句子（如 delete_agent）而非
// JSON map 的 delete 工具用。deps==0 时为空。
func DependentSuffix(deps int) string {
	if deps <= 0 {
		return ""
	}
	return " Note: " + dependentsNote + "."
}

const dependentsNote = "this entity was referenced by other entities (workflows/agents that equipped it, or documents that linked it); they may now fail — run get_relations on those referencing entities to repair them"
