package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	messagesdomain "github.com/sunweilin/anselm/backend/internal/domain/messages"
	reqctxpkg "github.com/sunweilin/anselm/backend/internal/pkg/reqctx"
)

var _ toolapp.Tool = (*TraceTool)(nil)

// attrParentBlockID is the sub-message Attrs key under which subagentapp stamps the spawning
// tool_call's block id (the E3 anchor). Re-declared here (not imported) because that const lives
// in the subagentapp package and crossing app→app just for a string key is not worth a dependency.
//
// attrParentBlockID 是 subagentapp 在 sub-message Attrs 里打入派它的 tool_call block id（E3 锚）的键。
// 此处重声明（不导入）：该 const 在 subagentapp 包，为一个字符串键拉 app→app 依赖不值当。
const attrParentBlockID = "parentBlockId"

// Reader is the narrow slice of the messages repository the trace tool needs — a DIP port so this
// package never depends on the messages store. *messagesstore.Store satisfies it. LoadThread is
// reused (not a new SubagentID-filtered query): the trace tool filters in-memory by SubagentID
// (single-user, one thread fits in memory — the same scale assumption LoadThread itself encodes).
//
// Reader 是 trace 工具所需 messages 仓储的窄切面——DIP 端口，本包不依赖 messages store。
// *messagesstore.Store 满足之。复用 LoadThread（不新加按 SubagentID 过滤的查询）：trace 工具内存按
// SubagentID 过滤（单用户、一条线程可装内存——正是 LoadThread 自身编码的规模假设）。
type Reader interface {
	LoadThread(ctx context.Context, conversationID string) ([]*messagesdomain.Message, error)
}

// TraceTool is get_subagent_trace: read back what a subagent did. A subagent owns no table — its
// turn lands as a sub-message tagged SubagentID in the PARENT conversation (E3 nesting), and the
// parent's LLM history excludes it, so without this tool the agent cannot answer "what did the
// subagent do?" beyond the final answer the Subagent tool already returned. This is the read path:
// list the conversation's subagent runs (no arg) or dump one run's full trace (with the run id).
//
// TraceTool 即 get_subagent_trace：读回 subagent 做了什么。subagent 无表——其回合作为带 SubagentID 的
// sub-message 落父对话（E3 嵌套），且父 LLM 历史排除它，故无此工具时 agent 无法回答「subagent 做了
// 什么」（除了 Subagent 工具已返回的最终答案）。这是读路径：列出本对话的 subagent run（无参）或导出某
// run 的完整 trace（带 run id）。
type TraceTool struct{ reader Reader }

// NewTraceTool constructs the tool over a messages Reader. NewTraceTool 基于 messages Reader 构造工具。
func NewTraceTool(reader Reader) *TraceTool { return &TraceTool{reader: reader} }

func (t *TraceTool) Name() string { return "get_subagent_trace" }

func (t *TraceTool) Description() string {
	return "Read back what a subagent did in THIS conversation. Subagents (spawned via the Subagent tool) " +
		"run in isolation and only their final answer comes back inline — their internal turn (reasoning, " +
		"tool calls, tool results) is hidden from your history. Call with no arguments to list this " +
		"conversation's subagent runs (each: subagentRunId, final answer, block count, the spawning " +
		"tool_call anchor). Call with subagentRunId to get that run's full trace. Read-only."
}

func (t *TraceTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"subagentRunId": {"type": "string", "description": "A subagent run id (subagt_…). Omit to list this conversation's subagent runs instead."}
		}
	}`)
}

type traceArgs struct {
	SubagentRunID string `json:"subagentRunId"`
}

// ValidateInput accepts both shapes (subagentRunId is optional) — only malformed JSON is rejected.
//
// ValidateInput 接受两种形态（subagentRunId 可选）——只拒绝坏 JSON。
func (t *TraceTool) ValidateInput(args json.RawMessage) error {
	if len(args) == 0 {
		return nil
	}
	var a traceArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("get_subagent_trace: bad args: %w", err)
	}
	return nil
}

// runEntry is one subagent run's summary in the list view.
//
// runEntry 是列表视图里一个 subagent run 的概要。
type runEntry struct {
	SubagentRunID    string `json:"subagentRunId"`
	Status           string `json:"status"`
	StopReason       string `json:"stopReason,omitempty"`
	FinalText        string `json:"finalText,omitempty"`          // last text block — what the subagent answered. 末个 text 块——subagent 的回答。
	BlockCount       int    `json:"blockCount"`                   // turns are always 1 (a subagent run = one sub-message). 回合恒 1。
	SpawningToolCall string `json:"spawningToolCallId,omitempty"` // E3 anchor (Attrs.parentBlockId). E3 锚点。
}

// blockView is one trace block, projected to the fields the LLM needs (omitting persistence
// plumbing like workspace_id / context_role).
//
// blockView 是一个 trace 块，投影成 LLM 需要的字段（略去 workspace_id / context_role 等落盘管线）。
type blockView struct {
	Type          string `json:"type"`
	Status        string `json:"status,omitempty"`
	Content       string `json:"content,omitempty"`
	Error         string `json:"error,omitempty"`
	BlockID       string `json:"blockId,omitempty"`
	ParentBlockID string `json:"parentBlockId,omitempty"`
}

// Execute lists the conversation's subagent runs (no id) or dumps one run's full trace. Failures
// degrade to a tool-result string (no conversation / unknown id) so the LLM can adjust — only a
// repository read error bubbles as a real error.
//
// Execute 列出本对话的 subagent run（无 id）或导出某 run 的完整 trace。失败降级为 tool-result 串
// （无对话 / 未知 id）使 LLM 可调整——只有仓储读错才作真错冒泡。
func (t *TraceTool) Execute(ctx context.Context, argsJSON string) (string, error) {
	var a traceArgs
	if len(argsJSON) > 0 {
		_ = json.Unmarshal([]byte(argsJSON), &a)
	}

	convID, ok := reqctxpkg.GetConversationID(ctx)
	if !ok {
		return "get_subagent_trace is only available inside a conversation (no conversationId in context).", nil
	}

	thread, err := t.reader.LoadThread(ctx, convID)
	if err != nil {
		return "", fmt.Errorf("get_subagent_trace: %w", err)
	}

	// Collect this conversation's subagent sub-messages (SubagentID != "").
	//
	// 收集本对话的 subagent sub-message（SubagentID != ""）。
	var runs []*messagesdomain.Message
	for _, m := range thread {
		if m != nil && m.SubagentID != "" {
			runs = append(runs, m)
		}
	}

	if a.SubagentRunID == "" {
		return t.list(runs), nil
	}

	for _, m := range runs {
		if m.SubagentID == a.SubagentRunID {
			return t.detail(m), nil
		}
	}
	return fmt.Sprintf("No subagent run %q in this conversation. Call get_subagent_trace with no arguments to list the runs that exist here.", a.SubagentRunID), nil
}

// list projects the runs to their summaries (newest-first by id-stable order is LoadThread's
// oldest-first; we keep chronological — the order the agent spawned them).
//
// list 把 run 投影成概要（保留 LoadThread 的最旧在前 = agent 派发的时序）。
func (t *TraceTool) list(runs []*messagesdomain.Message) string {
	entries := make([]runEntry, 0, len(runs))
	for _, m := range runs {
		entries = append(entries, runEntry{
			SubagentRunID:    m.SubagentID,
			Status:           m.Status,
			StopReason:       m.StopReason,
			FinalText:        finalText(m.Blocks),
			BlockCount:       len(m.Blocks),
			SpawningToolCall: spawningToolCall(m),
		})
	}
	return toolapp.ToJSON(map[string]any{"count": len(entries), "subagentRuns": entries})
}

// detail dumps one run's full block trace, blocks ordered by Seq (persist order).
//
// detail 导出某 run 的完整块 trace，块按 Seq（落盘序）排。
func (t *TraceTool) detail(m *messagesdomain.Message) string {
	blocks := append([]messagesdomain.Block(nil), m.Blocks...)
	sort.SliceStable(blocks, func(i, j int) bool { return blocks[i].Seq < blocks[j].Seq })
	views := make([]blockView, 0, len(blocks))
	for _, b := range blocks {
		views = append(views, blockView{
			Type:          b.Type,
			Status:        b.Status,
			Content:       b.Content,
			Error:         b.Error,
			BlockID:       b.ID,
			ParentBlockID: b.ParentBlockID,
		})
	}
	return toolapp.ToJSON(map[string]any{
		"subagentRunId":      m.SubagentID,
		"status":             m.Status,
		"stopReason":         m.StopReason,
		"errorMessage":       m.ErrorMessage,
		"spawningToolCallId": spawningToolCall(m),
		"blocks":             views,
	})
}

// finalText returns the last text block's content — the subagent's answer.
//
// finalText 返回末个 text 块内容——subagent 的回答。
func finalText(blocks []messagesdomain.Block) string {
	for i := len(blocks) - 1; i >= 0; i-- {
		if blocks[i].Type == messagesdomain.BlockTypeText {
			return blocks[i].Content
		}
	}
	return ""
}

// spawningToolCall reads the E3 anchor (the spawning tool_call's block id) from the sub-message's
// Attrs (subagent stamps it under "parentBlockId").
//
// spawningToolCall 从 sub-message 的 Attrs 读 E3 锚点（派它的 tool_call 的 block id；subagent 以
// "parentBlockId" 键打入）。
func spawningToolCall(m *messagesdomain.Message) string {
	if m.Attrs == nil {
		return ""
	}
	if v, ok := m.Attrs[attrParentBlockID].(string); ok {
		return v
	}
	return ""
}
