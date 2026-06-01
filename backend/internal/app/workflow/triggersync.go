package workflow

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
)

// TriggerSync registers/unregisters a workflow's trigger listeners when it is activated/deactivated.
// Implemented by an adapter over triggerapp.Service in main.go (workflow→trigger wiring). Nil = no-op.
// This is the missing link that makes `:activate` actually start cron/fsnotify/webhook/polling
// listeners — before this, activation only flipped `enabled` and the listeners never registered.
//
// TriggerSync 在 workflow 激活/停用时注册/注销其 trigger listener。main.go 中适配 triggerapp.Service。
// 这是让 :activate 真正启动 listener 的缺失环节——此前激活只翻 enabled、listener 从不注册。
type TriggerSync interface {
	SyncWorkflowTriggers(ctx context.Context, workflowID string, enabled bool, triggers []TriggerNodeInfo) error
}

// TriggerNodeInfo is one trigger node extracted from a workflow graph (neutral shape — no
// triggerdomain import, keeping the app→app dependency one-directional through this port).
//
// TriggerNodeInfo 是从 workflow 图抽出的一个 trigger 节点（中性结构，不引 triggerdomain）。
type TriggerNodeInfo struct {
	NodeID string
	Kind   string
	Config map[string]any
}

// SetTriggerSync installs the trigger-sync port post-construction (avoids a DI cycle: trigger
// Service also consumes workflow via SetWorkflowDeactivator). Nil disables (tests / no scheduler).
//
// SetTriggerSync 装配后注入 trigger-sync 端口（避开 DI 循环）。nil 禁用。
func (s *Service) SetTriggerSync(ts TriggerSync) { s.triggerSync = ts }

// syncActiveTriggers re-registers a workflow's listeners to match its CURRENT active graph + persisted
// enabled state. Called after create / accept / enable-toggle so listeners always track the live
// version (enable→register, disable→unregister, graph change→re-register). Best-effort; no-op when
// triggerSync is unset (tests / no scheduler). Reads enabled from the repo so callers needn't thread it.
//
// syncActiveTriggers 让 listener 始终对齐当前 active 图 + enabled 态；create/accept/enable 后调。
func (s *Service) syncActiveTriggers(ctx context.Context, workflowID string) {
	if s.triggerSync == nil {
		return
	}
	wf, err := s.repo.GetWorkflow(ctx, workflowID)
	if err != nil {
		s.log.Warn("workflowapp.syncActiveTriggers: GetWorkflow failed", zap.String("workflowId", workflowID), zap.Error(err))
		return
	}
	triggers, err := s.ActiveTriggers(ctx, workflowID)
	if err != nil {
		s.log.Warn("workflowapp.syncActiveTriggers: ActiveTriggers failed", zap.String("workflowId", workflowID), zap.Error(err))
		return
	}
	if err := s.triggerSync.SyncWorkflowTriggers(ctx, workflowID, wf.Enabled, triggers); err != nil {
		s.log.Warn("workflowapp.syncActiveTriggers: sync failed (some listeners may be down)",
			zap.String("workflowId", workflowID), zap.Bool("enabled", wf.Enabled), zap.Error(err))
	}
}

// ActiveTriggers returns the trigger nodes of a workflow's active version. Empty (not error) when the
// workflow has no active version yet. Used by syncActiveTriggers and boot registration.
//
// ActiveTriggers 返 workflow active version 的 trigger 节点；无 active version 时返空（非错）。
func (s *Service) ActiveTriggers(ctx context.Context, workflowID string) ([]TriggerNodeInfo, error) {
	v, err := s.GetActiveVersion(ctx, workflowID)
	if err != nil {
		if errors.Is(err, workflowdomain.ErrNoActiveVersion) || errors.Is(err, workflowdomain.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("workflowapp.ActiveTriggers: %w", err)
	}
	if v == nil || v.GraphParsed == nil {
		return nil, nil
	}
	return extractTriggers(v.GraphParsed), nil
}

// extractTriggers pulls every trigger node from a graph, normalizing the drifted config shapes:
// kind comes from config.kind (17 §7 canon) with a config.triggerType fallback (legacy tests);
// the listener spec is config.spec (nested canon) or the flat config minus meta keys.
//
// extractTriggers 从图抽所有 trigger 节点，兼容漂移的 config 写法（kind/triggerType + 嵌套/扁平 spec）。
func extractTriggers(g *workflowdomain.Graph) []TriggerNodeInfo {
	var out []TriggerNodeInfo
	for _, n := range g.Nodes {
		if n.Type != workflowdomain.NodeTypeTrigger {
			continue
		}
		kind, _ := n.Config["kind"].(string)
		if kind == "" {
			kind, _ = n.Config["triggerType"].(string)
		}
		if kind == "" {
			kind = "manual"
		}
		cfg, _ := n.Config["spec"].(map[string]any)
		if cfg == nil {
			// Flat shape: the listener-expected keys (expression/path/method) live directly on
			// the node config; strip the meta keys so only the listener spec passes through.
			cfg = map[string]any{}
			for k, v := range n.Config {
				switch k {
				case "kind", "triggerType", "payloadSchema", "spec":
				default:
					cfg[k] = v
				}
			}
		}
		out = append(out, TriggerNodeInfo{NodeID: n.ID, Kind: kind, Config: cfg})
	}
	return out
}
