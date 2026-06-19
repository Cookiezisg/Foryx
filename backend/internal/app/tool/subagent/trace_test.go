package subagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	messagesdomain "github.com/sunweilin/anselm/backend/internal/domain/messages"
	reqctxpkg "github.com/sunweilin/anselm/backend/internal/pkg/reqctx"
)

// fakeReader returns a fixed thread; gotConv records the conversation id LoadThread was called with.
//
// fakeReader 返回固定线程；gotConv 记 LoadThread 被调用的对话 id。
type fakeReader struct {
	thread  []*messagesdomain.Message
	gotConv string
}

func (r *fakeReader) LoadThread(_ context.Context, conversationID string) ([]*messagesdomain.Message, error) {
	r.gotConv = conversationID
	return r.thread, nil
}

// thread builds a conversation: a top-level assistant turn (SubagentID "") and two subagent runs.
//
// thread 构造一个对话：一个顶层 assistant 回合（SubagentID ""）+ 两个 subagent run。
func thread() []*messagesdomain.Message {
	return []*messagesdomain.Message{
		{ID: "msg_top", SubagentID: "", Role: messagesdomain.RoleAssistant, Status: messagesdomain.StatusCompleted,
			Blocks: []messagesdomain.Block{{ID: "blk_t", Seq: 1, Type: messagesdomain.BlockTypeText, Content: "parent answer"}}},
		{ID: "msg_sa1", SubagentID: "subagt_one", Role: messagesdomain.RoleAssistant, Status: messagesdomain.StatusCompleted,
			Attrs: map[string]any{attrParentBlockID: "blk_call_1"},
			Blocks: []messagesdomain.Block{
				{ID: "blk_r", Seq: 2, Type: messagesdomain.BlockTypeReasoning, Content: "thinking"},
				{ID: "blk_tc", Seq: 1, Type: messagesdomain.BlockTypeToolCall, Content: "grep"},
				{ID: "blk_tx", Seq: 3, Type: messagesdomain.BlockTypeText, Content: "found it in foo.go"},
			}},
		{ID: "msg_sa2", SubagentID: "subagt_two", Role: messagesdomain.RoleAssistant, Status: messagesdomain.StatusError,
			ErrorMessage: "boom",
			Blocks:       []messagesdomain.Block{{ID: "blk_x", Seq: 1, Type: messagesdomain.BlockTypeText, Content: "partial"}}},
	}
}

func convCtx() context.Context {
	return reqctxpkg.SetConversationID(context.Background(), "conv_1")
}

func TestTraceTool_List(t *testing.T) {
	rd := &fakeReader{thread: thread()}
	out, err := NewTraceTool(rd).Execute(convCtx(), "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if rd.gotConv != "conv_1" {
		t.Fatalf("LoadThread called with %q, want conv_1", rd.gotConv)
	}
	var res struct {
		Count        int `json:"count"`
		SubagentRuns []struct {
			SubagentRunID    string `json:"subagentRunId"`
			Status           string `json:"status"`
			FinalText        string `json:"finalText"`
			BlockCount       int    `json:"blockCount"`
			SpawningToolCall string `json:"spawningToolCallId"`
		} `json:"subagentRuns"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("bad json: %v\n%s", err, out)
	}
	// Only the two subagent runs — the top-level turn is excluded. 只两个 subagent run——顶层回合被排除。
	if res.Count != 2 || len(res.SubagentRuns) != 2 {
		t.Fatalf("want 2 runs, got %d: %s", res.Count, out)
	}
	r0 := res.SubagentRuns[0]
	if r0.SubagentRunID != "subagt_one" || r0.FinalText != "found it in foo.go" || r0.BlockCount != 3 || r0.SpawningToolCall != "blk_call_1" {
		t.Fatalf("run[0] wrong: %+v", r0)
	}
}

func TestTraceTool_Detail(t *testing.T) {
	rd := &fakeReader{thread: thread()}
	out, err := NewTraceTool(rd).Execute(convCtx(), `{"subagentRunId":"subagt_one"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var res struct {
		SubagentRunID      string `json:"subagentRunId"`
		SpawningToolCallID string `json:"spawningToolCallId"`
		Blocks             []struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("bad json: %v\n%s", err, out)
	}
	if res.SubagentRunID != "subagt_one" || res.SpawningToolCallID != "blk_call_1" {
		t.Fatalf("detail meta wrong: %+v", res)
	}
	// Blocks must be Seq-ordered: tool_call(1) → reasoning(2) → text(3). 块按 Seq 排序。
	if len(res.Blocks) != 3 ||
		res.Blocks[0].Type != messagesdomain.BlockTypeToolCall ||
		res.Blocks[1].Type != messagesdomain.BlockTypeReasoning ||
		res.Blocks[2].Type != messagesdomain.BlockTypeText {
		t.Fatalf("blocks not Seq-ordered: %+v", res.Blocks)
	}
}

func TestTraceTool_NoConversation(t *testing.T) {
	rd := &fakeReader{thread: thread()}
	out, err := NewTraceTool(rd).Execute(context.Background(), "")
	if err != nil {
		t.Fatalf("Execute must degrade, not error: %v", err)
	}
	if rd.gotConv != "" {
		t.Fatal("LoadThread must not run without a conversation")
	}
	if !strings.Contains(out, "only available inside a conversation") {
		t.Fatalf("want a clear no-conversation message, got %q", out)
	}
}

func TestTraceTool_UnknownID(t *testing.T) {
	rd := &fakeReader{thread: thread()}
	out, err := NewTraceTool(rd).Execute(convCtx(), `{"subagentRunId":"subagt_nope"}`)
	if err != nil {
		t.Fatalf("unknown id must degrade, not error: %v", err)
	}
	if !strings.Contains(out, "No subagent run") {
		t.Fatalf("want a clear not-found message, got %q", out)
	}
}

func TestTraceTool_ValidateInput(t *testing.T) {
	tl := NewTraceTool(&fakeReader{})
	if err := tl.ValidateInput(nil); err != nil {
		t.Fatalf("empty args (list mode) must validate: %v", err)
	}
	if err := tl.ValidateInput(json.RawMessage(`{"subagentRunId":"subagt_one"}`)); err != nil {
		t.Fatalf("valid args rejected: %v", err)
	}
	if err := tl.ValidateInput(json.RawMessage(`{bad`)); err == nil {
		t.Fatal("malformed JSON must be rejected")
	}
}
