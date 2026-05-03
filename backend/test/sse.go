//go:build pipeline

// sse.go — SSE collector for pipeline tests. Opens GET /api/v1/events?conversationId=
// over HTTP, parses the text/event-stream format, and exposes both raw events
// and entity-state snapshots (latest Message keyed by id, latest Forge keyed by
// id) so test cases can assert against eventual state without polling.
//
// sse.go — pipeline 测试用的 SSE 收集器。HTTP 打开 GET /api/v1/events?conversationId=，
// 解 text/event-stream，同时暴露原始事件流和按 entity-state 模型整理过的快照
// （Message 按 id keyed，Forge 按 id keyed），让测试可以断言"最终状态"而无需轮询。
package test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
)

// RawEvent is one parsed SSE event.
//
// RawEvent 是一条解析过的 SSE 事件。
type RawEvent struct {
	ID   string
	Type string // "chat.message" | "forge" | "conversation"
	Data []byte // raw JSON payload
	At   time.Time
}

// SSESub is a live SSE subscription. Cancel via Close().
//
// SSESub 是活动的 SSE 订阅，通过 Close() 取消。
type SSESub struct {
	t      *testing.T
	cancel context.CancelFunc
	resp   *http.Response

	mu sync.Mutex
	// raw holds every event in arrival order.
	// raw 按到达顺序保留每一条事件。
	raw []RawEvent
	// messages: id → latest Message snapshot (entity-state model).
	// messages：id → 最新 Message 快照（entity-state 模型）。
	messages map[string]*chatdomain.Message
	// orderedMsgIDs: insertion order so AllMessages can return chronologically.
	// orderedMsgIDs：插入顺序，让 AllMessages 按时间序返回。
	orderedMsgIDs []string
	// forges: id → latest Forge snapshot.
	// forges：id → 最新 Forge 快照。
	forges        map[string]*forgedomain.Forge
	orderedForges []string
	// conv: latest Conversation snapshot (only one per subscription).
	// conv：最新 Conversation 快照（一个订阅只有一个）。
	conv *convdomain.Conversation

	closed   bool
	streamCh chan struct{} // closed when stream goroutine exits
}

// SubscribeSSE opens a long-lived SSE subscription on the conversation. The
// goroutine reading the stream is registered for cleanup via t.Cleanup; tests
// rarely need to call Close() explicitly.
//
// SubscribeSSE 在该对话上开启长连接 SSE 订阅。读流的 goroutine 通过 t.Cleanup
// 注册自动清理，测试通常不必显式调 Close()。
func (h *Harness) SubscribeSSE(t *testing.T, conversationID string) *SSESub {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, "GET",
		h.URL()+"/api/v1/events?conversationId="+conversationID, nil)
	if err != nil {
		cancel()
		t.Fatalf("build SSE request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	// Don't use the harness HTTPClient — its 30s timeout would kill the long
	// SSE connection. Use a no-timeout client; ctx cancellation is the kill switch.
	//
	// 不能用 harness HTTPClient——它 30s timeout 会切断 SSE 长连。
	// 用无 timeout 的 client，ctx 取消才是 kill switch。
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("open SSE: %v", err)
	}
	if resp.StatusCode != 200 {
		_ = resp.Body.Close()
		cancel()
		t.Fatalf("SSE: status %d", resp.StatusCode)
	}

	sub := &SSESub{
		t:        t,
		cancel:   cancel,
		resp:     resp,
		messages: map[string]*chatdomain.Message{},
		forges:   map[string]*forgedomain.Forge{},
		streamCh: make(chan struct{}),
	}
	go sub.readLoop()
	t.Cleanup(sub.Close)
	return sub
}

// Close terminates the subscription and waits for the read loop to exit.
//
// Close 终止订阅并等读循环退出。
func (s *SSESub) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	s.cancel()
	_ = s.resp.Body.Close()
	<-s.streamCh
}

// readLoop drains the SSE stream until the context is cancelled or the
// connection drops. Each parsed event updates the in-memory snapshot maps.
//
// readLoop 排空 SSE 流直到 ctx 取消或连接断开；每条事件就地更新快照 map。
func (s *SSESub) readLoop() {
	defer close(s.streamCh)
	scanner := bufio.NewScanner(s.resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	var (
		curID, curType string
		dataLines      []string
	)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			// Blank line = event boundary.
			// 空行 = 事件边界。
			if curType != "" && len(dataLines) > 0 {
				data := strings.Join(dataLines, "\n")
				s.dispatch(RawEvent{
					ID: curID, Type: curType, Data: []byte(data), At: time.Now(),
				})
			}
			curID, curType, dataLines = "", "", nil
			continue
		}
		if strings.HasPrefix(line, ":") {
			// keep-alive ping; ignore.
			continue
		}
		if rest, ok := strings.CutPrefix(line, "id: "); ok {
			curID = rest
		} else if rest, ok := strings.CutPrefix(line, "event: "); ok {
			curType = rest
		} else if rest, ok := strings.CutPrefix(line, "data: "); ok {
			dataLines = append(dataLines, rest)
		}
	}
}

func (s *SSESub) dispatch(e RawEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.raw = append(s.raw, e)

	switch e.Type {
	case "chat.message":
		var m chatdomain.Message
		if err := json.Unmarshal(e.Data, &m); err != nil {
			s.t.Logf("SSE: malformed chat.message: %v", err)
			return
		}
		if _, seen := s.messages[m.ID]; !seen {
			s.orderedMsgIDs = append(s.orderedMsgIDs, m.ID)
		}
		s.messages[m.ID] = &m
	case "forge":
		var f forgedomain.Forge
		if err := json.Unmarshal(e.Data, &f); err != nil {
			s.t.Logf("SSE: malformed forge: %v", err)
			return
		}
		if _, seen := s.forges[f.ID]; !seen {
			s.orderedForges = append(s.orderedForges, f.ID)
		}
		s.forges[f.ID] = &f
	case "conversation":
		var c convdomain.Conversation
		if err := json.Unmarshal(e.Data, &c); err != nil {
			s.t.Logf("SSE: malformed conversation: %v", err)
			return
		}
		s.conv = &c
	}
}

// ── snapshot accessors ───────────────────────────────────────────────────────

// AllMessages returns the latest snapshot of every Message seen so far,
// in arrival order. Caller-owned copy — mutating won't affect the collector.
//
// AllMessages 返回到目前为止见过的每条 Message 的最新快照，按到达顺序。
// 返回的是拷贝——修改不影响收集器。
func (s *SSESub) AllMessages() []*chatdomain.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*chatdomain.Message, 0, len(s.orderedMsgIDs))
	for _, id := range s.orderedMsgIDs {
		out = append(out, copyMessage(s.messages[id]))
	}
	return out
}

// LastMessage returns the most-recently-arrived Message snapshot, or nil.
//
// LastMessage 返回最新到达的 Message 快照，无则 nil。
func (s *SSESub) LastMessage() *chatdomain.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.orderedMsgIDs) == 0 {
		return nil
	}
	return copyMessage(s.messages[s.orderedMsgIDs[len(s.orderedMsgIDs)-1]])
}

// MessageByID returns the latest snapshot for a specific Message id, or nil.
//
// MessageByID 返回指定 Message id 的最新快照，无则 nil。
func (s *SSESub) MessageByID(id string) *chatdomain.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyMessage(s.messages[id])
}

// AllForges returns latest Forge snapshots in arrival order.
//
// AllForges 返回 Forge 最新快照，按到达顺序。
func (s *SSESub) AllForges() []*forgedomain.Forge {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*forgedomain.Forge, 0, len(s.orderedForges))
	for _, id := range s.orderedForges {
		out = append(out, copyForge(s.forges[id]))
	}
	return out
}

// LastForge returns the most-recently-arrived Forge snapshot, or nil.
//
// LastForge 返回最新到达的 Forge 快照，无则 nil。
func (s *SSESub) LastForge() *forgedomain.Forge {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.orderedForges) == 0 {
		return nil
	}
	return copyForge(s.forges[s.orderedForges[len(s.orderedForges)-1]])
}

// Conversation returns the latest Conversation snapshot, or nil.
//
// Conversation 返回最新 Conversation 快照，无则 nil。
func (s *SSESub) Conversation() *convdomain.Conversation {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conv == nil {
		return nil
	}
	c := *s.conv
	return &c
}

// RawEvents returns a copy of every parsed event in arrival order. Useful for
// debugging "what actually came over the wire" when an assertion fails.
//
// RawEvents 返回到目前为止每条解析过的事件（按到达顺序的拷贝）。
// 断言失败时排查"实际 wire 上发了什么"很好用。
func (s *SSESub) RawEvents() []RawEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RawEvent, len(s.raw))
	copy(out, s.raw)
	return out
}

// ── wait helpers ─────────────────────────────────────────────────────────────

// WaitForMessage polls for a chat.message snapshot whose latest version
// matches predicate. Fails the test on timeout.
//
// WaitForMessage 轮询直到某条 chat.message 的最新版本满足 predicate；超时 fail。
func (s *SSESub) WaitForMessage(predicate func(*chatdomain.Message) bool, timeout time.Duration) *chatdomain.Message {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		for _, id := range s.orderedMsgIDs {
			if m := s.messages[id]; m != nil && predicate(m) {
				out := copyMessage(m)
				s.mu.Unlock()
				return out
			}
		}
		s.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	s.t.Fatalf("WaitForMessage: timed out after %s; saw %d messages, %d raw events",
		timeout, len(s.orderedMsgIDs), len(s.raw))
	return nil
}

// WaitForMessageStatus polls until the message with id reaches status, or
// the timeout fires. status="" matches any non-empty terminal status.
//
// WaitForMessageStatus 轮询直到指定 id 的消息达到 status，超时 fail。
// status="" 匹配任何非空终态。
func (s *SSESub) WaitForMessageStatus(id, status string, timeout time.Duration) *chatdomain.Message {
	s.t.Helper()
	return s.WaitForMessage(func(m *chatdomain.Message) bool {
		if m.ID != id {
			return false
		}
		if status == "" {
			return m.Status != "" && m.Status != chatdomain.StatusStreaming && m.Status != chatdomain.StatusPending
		}
		return m.Status == status
	}, timeout)
}

// WaitForAssistantTerminal waits for any assistant message to reach a
// non-streaming terminal status (completed/error/cancelled). Useful when the
// test doesn't know the message id up front (server allocates it).
//
// WaitForAssistantTerminal 等任意 assistant 消息进入非 streaming 终态
// （completed/error/cancelled）。测试事先不知道 message id（服务端分配）时用。
func (s *SSESub) WaitForAssistantTerminal(timeout time.Duration) *chatdomain.Message {
	s.t.Helper()
	return s.WaitForMessage(func(m *chatdomain.Message) bool {
		return m.Role == chatdomain.RoleAssistant &&
			(m.Status == chatdomain.StatusCompleted ||
				m.Status == chatdomain.StatusError ||
				m.Status == chatdomain.StatusCancelled)
	}, timeout)
}

// WaitForForge waits for a forge snapshot matching predicate.
//
// WaitForForge 等满足 predicate 的 forge 快照。
func (s *SSESub) WaitForForge(predicate func(*forgedomain.Forge) bool, timeout time.Duration) *forgedomain.Forge {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		for _, id := range s.orderedForges {
			if f := s.forges[id]; f != nil && predicate(f) {
				out := copyForge(f)
				s.mu.Unlock()
				return out
			}
		}
		s.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	s.t.Fatalf("WaitForForge: timed out after %s; saw %d forges, %d raw events",
		timeout, len(s.orderedForges), len(s.raw))
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func copyMessage(m *chatdomain.Message) *chatdomain.Message {
	if m == nil {
		return nil
	}
	c := *m
	if len(m.Blocks) > 0 {
		c.Blocks = make([]chatdomain.Block, len(m.Blocks))
		copy(c.Blocks, m.Blocks)
	}
	return &c
}

func copyForge(f *forgedomain.Forge) *forgedomain.Forge {
	if f == nil {
		return nil
	}
	c := *f
	if f.Pending != nil {
		p := *f.Pending
		c.Pending = &p
	}
	return &c
}

// FormatRawEvents returns a multi-line debug string of every raw event seen,
// truncated for readability. Use in test failure messages.
//
// FormatRawEvents 返回多行 debug 字符串列出所有原始事件（适当截断）。
// 测试失败 message 里用。
func (s *SSESub) FormatRawEvents() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b strings.Builder
	for i, e := range s.raw {
		dataPreview := string(e.Data)
		if len(dataPreview) > 200 {
			dataPreview = dataPreview[:200] + "…"
		}
		fmt.Fprintf(&b, "  [%d] %s id=%s data=%s\n",
			i, e.Type, e.ID, dataPreview)
	}
	return b.String()
}
