//go:build pipeline

package harness

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
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
)

// RawEvent is one parsed SSE event from either endpoint.
//
// RawEvent 是任一端点解析后的一条 SSE 事件。
type RawEvent struct {
	Source string
	ID     string
	Type   string
	Data   []byte
	At     time.Time
}

// SSESub holds reconstructed message + block state plus raw event log.
//
// SSESub 持重构的 message + block 状态与原始事件日志。
type SSESub struct {
	t              *testing.T
	cancelEventLog context.CancelFunc
	cancelNotif    context.CancelFunc
	respEventLog   *http.Response
	respNotif      *http.Response

	mu sync.Mutex

	raw []RawEvent

	messages      map[string]*chatdomain.Message
	orderedMsgIDs []string

	blocks map[string]*chatdomain.Block

	conv *convdomain.Conversation

	closed       bool
	streamELdone chan struct{}
	streamNFdone chan struct{}
}

// SubscribeSSE opens eventlog (filtered to conversationID) + notifications subscriptions; cleanup on t.
//
// SubscribeSSE 同时开 eventlog + notifications 订阅，清理挂 t。
func (h *Harness) SubscribeSSE(t *testing.T, conversationID string) *SSESub {
	t.Helper()
	sub := &SSESub{
		t:            t,
		messages:     map[string]*chatdomain.Message{},
		blocks:       map[string]*chatdomain.Block{},
		streamELdone: make(chan struct{}),
		streamNFdone: make(chan struct{}),
	}

	elCtx, elCancel := context.WithCancel(context.Background())
	sub.cancelEventLog = elCancel
	elReq, err := http.NewRequestWithContext(elCtx, "GET",
		h.URL()+"/api/v1/eventlog?conversationId="+conversationID, nil)
	if err != nil {
		elCancel()
		t.Fatalf("build eventlog SSE request: %v", err)
	}
	elReq.Header.Set("Accept", "text/event-stream")
	noTimeoutClient := &http.Client{}
	elResp, err := noTimeoutClient.Do(elReq)
	if err != nil {
		elCancel()
		t.Fatalf("open eventlog SSE: %v", err)
	}
	if elResp.StatusCode != 200 {
		_ = elResp.Body.Close()
		elCancel()
		t.Fatalf("eventlog SSE: status %d", elResp.StatusCode)
	}
	sub.respEventLog = elResp

	nfCtx, nfCancel := context.WithCancel(context.Background())
	sub.cancelNotif = nfCancel
	nfReq, err := http.NewRequestWithContext(nfCtx, "GET",
		h.URL()+"/api/v1/notifications", nil)
	if err != nil {
		_ = elResp.Body.Close()
		elCancel()
		nfCancel()
		t.Fatalf("build notifications SSE request: %v", err)
	}
	nfReq.Header.Set("Accept", "text/event-stream")
	nfResp, err := noTimeoutClient.Do(nfReq)
	if err != nil {
		_ = elResp.Body.Close()
		elCancel()
		nfCancel()
		t.Fatalf("open notifications SSE: %v", err)
	}
	if nfResp.StatusCode != 200 {
		_ = elResp.Body.Close()
		_ = nfResp.Body.Close()
		elCancel()
		nfCancel()
		t.Fatalf("notifications SSE: status %d", nfResp.StatusCode)
	}
	sub.respNotif = nfResp

	go sub.readLoop(elResp.Body, "eventlog", sub.streamELdone)
	go sub.readLoop(nfResp.Body, "notifications", sub.streamNFdone)
	t.Cleanup(sub.Close)
	return sub
}

// Close terminates both subscriptions and waits for read loops.
//
// Close 终止两个订阅，等读循环退出。
func (s *SSESub) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	s.cancelEventLog()
	s.cancelNotif()
	_ = s.respEventLog.Body.Close()
	_ = s.respNotif.Body.Close()
	<-s.streamELdone
	<-s.streamNFdone
}

func (s *SSESub) readLoop(body interface{ Read(p []byte) (int, error) }, source string, done chan struct{}) {
	defer close(done)
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	var (
		curID, curType string
		dataLines      []string
	)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if curType != "" && len(dataLines) > 0 {
				data := strings.Join(dataLines, "\n")
				s.dispatch(RawEvent{
					Source: source,
					ID:     curID,
					Type:   curType,
					Data:   []byte(data),
					At:     time.Now(),
				})
			}
			curID, curType, dataLines = "", "", nil
			continue
		}
		if strings.HasPrefix(line, ":") {
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

	if e.Source == "eventlog" {
		s.applyEventLog(e)
		return
	}
	s.applyNotification(e)
}

func (s *SSESub) applyEventLog(e RawEvent) {
	switch e.Type {
	case "message_start":
		var ev eventlogdomain.MessageStart
		if err := json.Unmarshal(e.Data, &ev); err != nil {
			s.t.Logf("SSE: malformed message_start: %v", err)
			return
		}
		m := &chatdomain.Message{
			ID:             ev.ID,
			ConversationID: ev.ConversationID,
			ParentBlockID:  ev.ParentBlockID,
			Role:           ev.Role,
			Attrs:          ev.Attrs,
			Status:         chatdomain.StatusStreaming,
		}
		if _, seen := s.messages[m.ID]; !seen {
			s.orderedMsgIDs = append(s.orderedMsgIDs, m.ID)
		}
		s.messages[m.ID] = m

	case "message_stop":
		var ev eventlogdomain.MessageStop
		if err := json.Unmarshal(e.Data, &ev); err != nil {
			s.t.Logf("SSE: malformed message_stop: %v", err)
			return
		}
		m := s.messages[ev.ID]
		if m == nil {
			return
		}
		m.Status = ev.Status
		m.StopReason = ev.StopReason
		m.ErrorCode = ev.ErrorCode
		m.ErrorMessage = ev.ErrorMessage
		m.InputTokens = ev.InputTokens
		m.OutputTokens = ev.OutputTokens

	case "block_start":
		var ev eventlogdomain.BlockStart
		if err := json.Unmarshal(e.Data, &ev); err != nil {
			s.t.Logf("SSE: malformed block_start: %v", err)
			return
		}
		blk := &chatdomain.Block{
			ID:             ev.ID,
			ConversationID: ev.ConversationID,
			MessageID:      ev.MessageID,
			ParentBlockID:  ev.ParentID,
			Type:           ev.BlockType,
			Attrs:          ev.Attrs,
			Status:         chatdomain.StatusStreaming,
		}
		s.blocks[blk.ID] = blk
		if m := s.messages[ev.MessageID]; m != nil {
			m.Blocks = append(m.Blocks, *blk)
		}

	case "block_delta":
		var ev eventlogdomain.BlockDelta
		if err := json.Unmarshal(e.Data, &ev); err != nil {
			s.t.Logf("SSE: malformed block_delta: %v", err)
			return
		}
		blk := s.blocks[ev.ID]
		if blk == nil {
			return
		}
		blk.Content += ev.Delta
		s.syncBlockIntoMessage(blk)

	case "block_stop":
		var ev eventlogdomain.BlockStop
		if err := json.Unmarshal(e.Data, &ev); err != nil {
			s.t.Logf("SSE: malformed block_stop: %v", err)
			return
		}
		blk := s.blocks[ev.ID]
		if blk == nil {
			return
		}
		blk.Status = ev.Status
		blk.Error = ev.Error
		s.syncBlockIntoMessage(blk)
	}
}

// syncBlockIntoMessage refreshes the block entry in parent message.Blocks (values, not pointers).
//
// syncBlockIntoMessage 把 block 同步回父 message.Blocks（存值非指针）。
func (s *SSESub) syncBlockIntoMessage(blk *chatdomain.Block) {
	m := s.messages[blk.MessageID]
	if m == nil {
		return
	}
	for i := range m.Blocks {
		if m.Blocks[i].ID == blk.ID {
			m.Blocks[i] = *blk
			return
		}
	}
}

type notificationEnvelope struct {
	Type           string          `json:"type"`
	ID             string          `json:"id"`
	Data           json.RawMessage `json:"data"`
	ConversationID string          `json:"conversationId,omitempty"`
}

func (s *SSESub) applyNotification(e RawEvent) {
	var env notificationEnvelope
	if err := json.Unmarshal(e.Data, &env); err != nil {
		s.t.Logf("SSE: malformed notification: %v", err)
		return
	}
	if env.Type == "conversation" {
		var c convdomain.Conversation
		if err := json.Unmarshal(env.Data, &c); err != nil {
			s.t.Logf("SSE: malformed conversation notification: %v", err)
			return
		}
		s.conv = &c
	}
}

// AllMessages returns reconstructed Message snapshots in arrival order.
//
// AllMessages 按到达顺序返重构的 Message 快照。
func (s *SSESub) AllMessages() []*chatdomain.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*chatdomain.Message, 0, len(s.orderedMsgIDs))
	for _, id := range s.orderedMsgIDs {
		out = append(out, copyMessage(s.messages[id]))
	}
	return out
}

// LastMessage returns the most recently started Message, or nil.
//
// LastMessage 返最近开始的 Message，无则 nil。
func (s *SSESub) LastMessage() *chatdomain.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.orderedMsgIDs) == 0 {
		return nil
	}
	return copyMessage(s.messages[s.orderedMsgIDs[len(s.orderedMsgIDs)-1]])
}

// MessageByID returns the reconstructed Message for id, or nil.
//
// MessageByID 返 id 对应的重构 Message，无则 nil。
func (s *SSESub) MessageByID(id string) *chatdomain.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return copyMessage(s.messages[id])
}

// Conversation returns the latest conversation snapshot from notifications, or nil.
//
// Conversation 返 notifications 收到的最新 conversation 快照，无则 nil。
func (s *SSESub) Conversation() *convdomain.Conversation {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conv == nil {
		return nil
	}
	c := *s.conv
	return &c
}

// RawEvents returns a copy of every event seen across both streams.
//
// RawEvents 返两条流上所有事件的拷贝。
func (s *SSESub) RawEvents() []RawEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RawEvent, len(s.raw))
	copy(out, s.raw)
	return out
}

// WaitForMessage polls reconstructed messages until predicate is true; fatals on timeout.
//
// WaitForMessage 轮询 messages 至 predicate 真，超时 fatal。
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

// WaitForMessageStatus waits for id to reach status; "" means any terminal.
//
// WaitForMessageStatus 等 id 达到 status；"" 表示任意终态。
func (s *SSESub) WaitForMessageStatus(id, status string, timeout time.Duration) *chatdomain.Message {
	s.t.Helper()
	return s.WaitForMessage(func(m *chatdomain.Message) bool {
		if m.ID != id {
			return false
		}
		if status == "" {
			return m.Status != "" &&
				m.Status != chatdomain.StatusStreaming &&
				m.Status != chatdomain.StatusPending
		}
		return m.Status == status
	}, timeout)
}

// WaitForAssistantTerminal waits for any assistant message to reach a terminal status.
//
// WaitForAssistantTerminal 等任意 assistant 消息进入终态。
func (s *SSESub) WaitForAssistantTerminal(timeout time.Duration) *chatdomain.Message {
	s.t.Helper()
	return s.WaitForMessage(func(m *chatdomain.Message) bool {
		return m.Role == chatdomain.RoleAssistant &&
			(m.Status == chatdomain.StatusCompleted ||
				m.Status == chatdomain.StatusError ||
				m.Status == chatdomain.StatusCancelled)
	}, timeout)
}

// WaitForConversation polls for a conversation snapshot matching predicate.
//
// WaitForConversation 等满足 predicate 的 conversation 快照。
func (s *SSESub) WaitForConversation(predicate func(*convdomain.Conversation) bool, timeout time.Duration) *convdomain.Conversation {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		if s.conv != nil && predicate(s.conv) {
			out := *s.conv
			s.mu.Unlock()
			return &out
		}
		s.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	s.t.Fatalf("WaitForConversation: timed out after %s; saw %d raw events",
		timeout, len(s.raw))
	return nil
}

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

// FormatRawEvents returns a multi-line debug string of every raw event (truncated).
//
// FormatRawEvents 返每条原始事件的多行 debug 字符串（截断）。
func (s *SSESub) FormatRawEvents() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b strings.Builder
	for i, e := range s.raw {
		dataPreview := string(e.Data)
		if len(dataPreview) > 200 {
			dataPreview = dataPreview[:200] + "…"
		}
		fmt.Fprintf(&b, "  [%d] %s/%s id=%s data=%s\n",
			i, e.Source, e.Type, e.ID, dataPreview)
	}
	return b.String()
}
