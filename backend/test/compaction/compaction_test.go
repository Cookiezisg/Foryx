//go:build pipeline

// Package compaction_test runs pipeline tests for app/contextmgr post-turn compaction.
//
// Package compaction_test 跑 app/contextmgr 收尾压缩 pipeline 测试。
package compaction_test

import (
	"strings"
	"testing"
	"time"

	contextmgrapp "github.com/sunweilin/forgify/backend/internal/app/contextmgr"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

func TestCompaction_NoOpBelowThreshold(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptText("hello"))
	fake.PushDefault(th.ScriptText("Title"))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "compact-noop")
	sub := h.SubscribeSSE(t, conv.ID)
	th.PostMessage(t, h, conv.ID, "hi.")
	final := sub.WaitForAssistantTerminal(60 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q raw:\n%s", final.Status, sub.FormatRawEvents())
	}

	time.Sleep(50 * time.Millisecond)

	var compactionCount int64
	h.DB.Raw("SELECT COUNT(*) FROM message_blocks WHERE conversation_id=? AND type='compaction'", conv.ID).
		Scan(&compactionCount)
	if compactionCount != 0 {
		t.Errorf("expected no compaction blocks under default thresholds, got %d", compactionCount)
	}
	var demoted int64
	h.DB.Raw("SELECT COUNT(*) FROM message_blocks WHERE conversation_id=? AND context_role IN ('warm','cold','archived')", conv.ID).
		Scan(&demoted)
	if demoted != 0 {
		t.Errorf("expected no demoted blocks, got %d", demoted)
	}
}

func TestCompaction_DemoteOnly(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	for i := 0; i < 4; i++ {
		fake.PushScript(th.ScriptSingleToolCall(
			"TodoList", "call_fake_demote_"+itoa(i),
			`{"summary":"list todos"}`,
		))
		fake.PushScript(th.ScriptText("ok"))
	}
	fake.PushDefault(th.ScriptText("Title"))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")

	h.ContextManager.SetThresholds(contextmgrapp.Thresholds{
		Soft:           0.0,
		Hard:           10.0, // unreachable
		RecentTurns:    1,
		RecentTRKeep:   1,
		WarmCutoff:     2,
		MaxSummaryRune: 6000,
	})

	conv := h.NewConversation(t, "compact-demote")

	for i := 0; i < 4; i++ {
		sub := h.SubscribeSSE(t, conv.ID)
		th.PostMessage(t, h, conv.ID, "round "+itoa(i))
		final := sub.WaitForAssistantTerminal(60 * time.Second)
		if final.Status != chatdomain.StatusCompleted {
			t.Fatalf("round %d status=%q raw:\n%s", i, final.Status, sub.FormatRawEvents())
		}
		_ = sub
	}

	var warmCnt, coldCnt, hotCnt, archivedCnt int64
	h.DB.Raw("SELECT COUNT(*) FROM message_blocks WHERE conversation_id=? AND type='tool_result' AND context_role='warm'", conv.ID).Scan(&warmCnt)
	h.DB.Raw("SELECT COUNT(*) FROM message_blocks WHERE conversation_id=? AND type='tool_result' AND context_role='cold'", conv.ID).Scan(&coldCnt)
	h.DB.Raw("SELECT COUNT(*) FROM message_blocks WHERE conversation_id=? AND type='tool_result' AND context_role='hot'", conv.ID).Scan(&hotCnt)
	h.DB.Raw("SELECT COUNT(*) FROM message_blocks WHERE conversation_id=? AND context_role='archived'", conv.ID).Scan(&archivedCnt)

	if archivedCnt != 0 {
		t.Errorf("expected no archived blocks (Hard unreachable), got %d", archivedCnt)
	}
	if hotCnt < 1 {
		t.Errorf("expected ≥1 hot tool_result, got %d (warm=%d cold=%d)", hotCnt, warmCnt, coldCnt)
	}
	if warmCnt+coldCnt == 0 {
		t.Errorf("expected some tool_result demoted to warm/cold; got hot=%d warm=%d cold=%d",
			hotCnt, warmCnt, coldCnt)
	}
}

func TestCompaction_FullCompactSummaryReachesNextTurn(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptText("first reply."))
	fake.PushScript(th.ScriptText("- User said hi.\n- Assistant replied first reply."))
	fake.PushScript(th.ScriptText("second reply."))
	fake.PushDefault(th.ScriptText("Title"))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")

	h.ContextManager.SetThresholds(contextmgrapp.Thresholds{
		Soft:           0.0,
		Hard:           0.0,
		RecentTurns:    0,
		RecentTRKeep:   0,
		WarmCutoff:     0,
		MaxSummaryRune: 6000,
	})

	conv := h.NewConversation(t, "compact-fullcompact")

	sub1 := h.SubscribeSSE(t, conv.ID)
	th.PostMessage(t, h, conv.ID, "hi.")
	final1 := sub1.WaitForAssistantTerminal(60 * time.Second)
	if final1.Status != chatdomain.StatusCompleted {
		t.Fatalf("round1 status=%q raw:\n%s", final1.Status, sub1.FormatRawEvents())
	}
	_ = sub1

	var conv1 convdomain.Conversation
	h.DB.Raw("SELECT * FROM conversations WHERE id=?", conv.ID).Scan(&conv1)
	if !strings.Contains(conv1.Summary, "first reply") {
		t.Errorf("conv.Summary missing expected text\nsummary:\n%s", conv1.Summary)
	}
	var archivedCnt, compactionCnt int64
	h.DB.Raw("SELECT COUNT(*) FROM message_blocks WHERE conversation_id=? AND context_role='archived'", conv.ID).Scan(&archivedCnt)
	h.DB.Raw("SELECT COUNT(*) FROM message_blocks WHERE conversation_id=? AND type='compaction'", conv.ID).Scan(&compactionCnt)
	if archivedCnt == 0 {
		t.Errorf("expected ≥1 archived block after fullCompact, got 0")
	}
	if compactionCnt != 1 {
		t.Errorf("expected exactly 1 compaction block, got %d", compactionCnt)
	}

	// Lift Hard so round 2 does NOT trigger another fullCompact (LastMessages would otherwise see the compaction call).
	h.ContextManager.SetThresholds(contextmgrapp.Thresholds{
		Soft:           10.0,
		Hard:           10.0,
		RecentTurns:    10,
		RecentTRKeep:   10,
		WarmCutoff:     20,
		MaxSummaryRune: 6000,
	})

	sub2 := h.SubscribeSSE(t, conv.ID)
	th.PostMessage(t, h, conv.ID, "what did I say?")
	final2 := sub2.WaitForAssistantTerminal(60 * time.Second)
	if final2.Status != chatdomain.StatusCompleted {
		t.Fatalf("round2 status=%q raw:\n%s", final2.Status, sub2.FormatRawEvents())
	}
	_ = sub2

	gotMessages := fake.LastMessages()
	if len(gotMessages) == 0 {
		t.Fatal("fake LLM saw no messages on round 2 — chat runner did not send history")
	}
	var foundSummary bool
	for _, m := range gotMessages {
		if strings.Contains(m.Content, "<conversation_summary>") &&
			strings.Contains(m.Content, "first reply") {
			foundSummary = true
			break
		}
	}
	if !foundSummary {
		t.Errorf("round2 LLM history missing <conversation_summary> wrapper")
		for i, m := range gotMessages {
			t.Logf("  msg %d role=%s content=%q", i, m.Role, m.Content)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
