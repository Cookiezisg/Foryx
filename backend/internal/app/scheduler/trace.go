package scheduler

import (
	"context"
	"fmt"
	"time"
)

// TraceEntry is one journaled step projected for the orchestration UI's per-node diagnostic
// (08 §6). The flowrun journal is the durable truth; this read-only projection is what the UI
// pulls — whole-run or per-node — to render "what this run recorded here" + reconnect catch-up
// (the tick stream is best-effort/lossy, so the UI re-pulls the full trace on reconnect).
//
// TraceEntry 是 journal 一条记账投影给编排 UI 节点诊断(08 §6);journal 是 durable 真相,本投影只读。
type TraceEntry struct {
	Seq          int64     `json:"seq"`
	Type         string    `json:"type"`
	NodeID       string    `json:"nodeId"`
	IterationKey int       `json:"iterationKey"`
	Generation   int       `json:"generation"`
	Turn         int       `json:"turn,omitempty"`
	ToolCallID   string    `json:"toolCallId,omitempty"`
	Result       any       `json:"result,omitempty"`
	At           time.Time `json:"at"`
}

// GetTrace reads a flowrun's journal (the durable truth, seq-ordered) and projects it to the UI
// trace. nodeID empty = the whole run; non-empty = only that node's entries (loop iterations
// included, distinguished by IterationKey). Read-only: it never touches the running engine.
//
// GetTrace 读 flowrun journal(seq 序)投影成 UI trace;nodeID 空=整 run,非空=过滤到该节点。
func (s *Service) GetTrace(ctx context.Context, flowrunID, nodeID string) ([]TraceEntry, error) {
	if s.journal == nil {
		return nil, nil
	}
	evs, err := s.journal.LoadJournal(ctx, flowrunID)
	if err != nil {
		return nil, fmt.Errorf("schedulerapp.GetTrace: %w", err)
	}
	out := make([]TraceEntry, 0, len(evs))
	for i := range evs {
		e := evs[i]
		if nodeID != "" && e.NodeID != nodeID {
			continue
		}
		out = append(out, TraceEntry{
			Seq:          e.Seq,
			Type:         e.Type,
			NodeID:       e.NodeID,
			IterationKey: e.IterationKey,
			Generation:   e.Generation,
			Turn:         e.Turn,
			ToolCallID:   e.ToolCallID,
			Result:       e.Result,
			At:           e.CreatedAt,
		})
	}
	return out, nil
}
