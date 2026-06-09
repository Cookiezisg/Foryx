package bootstrap

import (
	"context"

	chatapp "github.com/sunweilin/forgify/backend/internal/app/chat"
	contextmgrapp "github.com/sunweilin/forgify/backend/internal/app/contextmgr"
	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
)

// CapabilityLister is the slice of model.CapabilityService bootstrap needs: the workspace's usable
// (key, model) catalog, each entry carrying context window / max output / vision / native-docs.
// *modelapp.CapabilityService satisfies it.
//
// CapabilityLister 是 bootstrap 需要的 model.CapabilityService 切片：workspace 可用的 (key, model)
// 目录，每项带 context window / max output / vision / native-docs。*modelapp.CapabilityService 满足它。
type CapabilityLister interface {
	List(ctx context.Context) ([]modelapp.CapabilityView, error)
}

// ModelInfoLookup resolves a (provider, modelID) to its capability view. ONE lookup serves two
// consumers — contextmgr's window budget AND chat's content capabilities — so the per-model
// catalog is queried through a single path instead of two parallel adapters. Unknown model → the
// zero view (window 0 → contextmgr skips compaction; caps false → chat renders conservatively).
//
// ModelInfoLookup 把 (provider, modelID) 解析到其能力视图。**一个**查询服务两个消费者——contextmgr
// 的窗口预算 + chat 的内容能力——故 per-model 目录走单一路径而非两个并行适配器。未知模型 → 零视图
// （window 0 → contextmgr 跳过压缩；caps false → chat 保守渲染）。
type ModelInfoLookup struct{ caps CapabilityLister }

// NewModelInfoLookup wraps the capability catalog.
//
// NewModelInfoLookup 包裹能力目录。
func NewModelInfoLookup(caps CapabilityLister) ModelInfoLookup { return ModelInfoLookup{caps: caps} }

func (l ModelInfoLookup) find(ctx context.Context, provider, modelID string) (modelapp.CapabilityView, bool) {
	if l.caps == nil {
		return modelapp.CapabilityView{}, false
	}
	views, err := l.caps.List(ctx)
	if err != nil {
		return modelapp.CapabilityView{}, false
	}
	for _, v := range views {
		if v.Provider == provider && v.ModelID == modelID {
			return v, true
		}
	}
	return modelapp.CapabilityView{}, false
}

// contentCaps returns chat's content capabilities for a (provider, modelID); unknown → zero.
//
// contentCaps 返回 chat 对某 (provider, modelID) 的内容能力；未知 → 零。
func (l ModelInfoLookup) contentCaps(ctx context.Context, provider, modelID string) chatapp.ContentCapabilities {
	v, _ := l.find(ctx, provider, modelID)
	return chatapp.ContentCapabilities{Vision: v.Vision, NativeDocs: v.NativeDocs}
}

// WindowResolver adapts the lookup to contextmgr's WindowResolver port.
//
// WindowResolver 把查询适配成 contextmgr 的 WindowResolver 端口。
func (l ModelInfoLookup) WindowResolver() contextmgrapp.WindowResolver {
	return windowResolver{lookup: l}
}

type windowResolver struct{ lookup ModelInfoLookup }

var _ contextmgrapp.WindowResolver = windowResolver{}

func (w windowResolver) ContextBudget(ctx context.Context, provider, modelID string) (window, maxOutput int) {
	v, ok := w.lookup.find(ctx, provider, modelID)
	if !ok {
		return 0, 0
	}
	return v.ContextWindow, v.MaxOutput
}
