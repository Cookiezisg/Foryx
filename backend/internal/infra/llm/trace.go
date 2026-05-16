package llm

import (
	"context"
	"iter"
	"strings"
	"sync"
	"time"

	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

const defaultMaxTracesPerConv = 10

// Trace is one captured Stream() call: request + emitted events + summary.
//
// Trace 是一次 Stream() 调用的捕获：请求、所发事件与汇总。
type Trace struct {
	Timestamp      time.Time     `json:"timestamp"`
	ConversationID string        `json:"conversationId,omitempty"`
	Request        Request       `json:"request"`
	Events         []StreamEvent `json:"events"`
	ElapsedMs      int64         `json:"elapsedMs"`
	FinalText      string        `json:"finalText,omitempty"`
	Error          string        `json:"error,omitempty"`
}

// TraceRecorder is a concurrency-safe per-conversation ring buffer of Traces.
//
// TraceRecorder 是按对话维护的并发安全 Trace 环形 buffer。
type TraceRecorder struct {
	mu     sync.Mutex
	traces map[string][]Trace
	maxPer int
}

// NewTraceRecorder returns a recorder capped at 10 traces per conversation.
//
// NewTraceRecorder 返回每对话上限 10 traces 的 recorder。
func NewTraceRecorder() *TraceRecorder {
	return &TraceRecorder{
		traces: map[string][]Trace{},
		maxPer: defaultMaxTracesPerConv,
	}
}

// Record appends a trace to the conversation's ring; oldest is evicted when full.
//
// Record 追加 trace 到对话 ring；满 cap 时丢最早。
func (r *TraceRecorder) Record(t Trace) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := t.ConversationID
	if key == "" {
		key = "(no-conversation)"
	}
	ring := r.traces[key]
	ring = append(ring, t)
	if len(ring) > r.maxPer {
		ring = ring[len(ring)-r.maxPer:]
	}
	r.traces[key] = ring
}

// TracesFor returns a copy of the conversation's recorded traces, oldest-first.
//
// TracesFor 返回对话已记录 trace 的副本，最早在前。
func (r *TraceRecorder) TracesFor(conversationID string) []Trace {
	r.mu.Lock()
	defer r.mu.Unlock()
	ring := r.traces[conversationID]
	out := make([]Trace, len(ring))
	copy(out, ring)
	return out
}

// Conversations returns all conversation IDs that have at least one trace.
//
// Conversations 返回至少含一条 trace 的所有 conversation ID。
func (r *TraceRecorder) Conversations() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.traces))
	for k := range r.traces {
		out = append(out, k)
	}
	return out
}

// Clear drops all traces for one conversation and returns the count dropped.
//
// Clear 丢弃某对话的所有 trace 并返回丢弃数。
func (r *TraceRecorder) Clear(conversationID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(r.traces[conversationID])
	delete(r.traces, conversationID)
	return n
}

// recordingClient wraps an inner Client and records every Stream call to the recorder.
//
// recordingClient 包内部 Client，把每次 Stream 调用记入 recorder。
type recordingClient struct {
	inner    Client
	recorder *TraceRecorder
}

func (c *recordingClient) Stream(ctx context.Context, req Request) iter.Seq[StreamEvent] {
	convID, _ := reqctxpkg.GetConversationID(ctx)
	start := time.Now()
	innerSeq := c.inner.Stream(ctx, req)

	return func(yield func(StreamEvent) bool) {
		var (
			events    []StreamEvent
			finalText strings.Builder
			finalErr  string
		)
		stopped := false
		for ev := range innerSeq {
			events = append(events, ev)
			if ev.Type == EventText {
				finalText.WriteString(ev.Delta)
			}
			if ev.Type == EventError && ev.Err != nil {
				finalErr = ev.Err.Error()
			}
			if !stopped {
				if !yield(ev) {
					stopped = true
				}
			}
		}
		c.recorder.Record(Trace{
			Timestamp:      start,
			ConversationID: convID,
			Request:        req,
			Events:         events,
			ElapsedMs:      time.Since(start).Milliseconds(),
			FinalText:      finalText.String(),
			Error:          finalErr,
		})
	}
}
