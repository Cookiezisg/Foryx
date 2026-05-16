package llm

import (
	"context"
	"errors"
	"iter"
	"sync"
)

// MockScript is one canned response; ErrAfter replaces the entire script with a single error event.
//
// MockScript 描述一段预设响应；ErrAfter 非 nil 时整段换成一个错误事件。
type MockScript struct {
	Events   []StreamEvent
	ErrAfter error
}

// MockClient queues MockScripts FIFO and replays them through Stream; concurrent-safe.
//
// MockClient FIFO 队列化 MockScript，通过 Stream 回放；并发安全。
type MockClient struct {
	mu          sync.Mutex
	queue       []MockScript
	lastRequest Request
	callCount   int
}

// NewMockClient constructs an empty MockClient (singleton in production).
//
// NewMockClient 构造空 MockClient（生产为单例）。
func NewMockClient() *MockClient {
	return &MockClient{}
}

// PushScript enqueues one script onto the FIFO.
//
// PushScript 把一段 script 入队（FIFO）。
func (m *MockClient) PushScript(s MockScript) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queue = append(m.queue, s)
}

// QueueDepth returns the number of unconsumed scripts.
//
// QueueDepth 返回未消费的 script 数。
func (m *MockClient) QueueDepth() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.queue)
}

// Queue returns a defensive copy of the script queue.
//
// Queue 返回 script 队列的防御性副本。
func (m *MockClient) Queue() []MockScript {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]MockScript, len(m.queue))
	copy(out, m.queue)
	return out
}

// Clear empties the queue and returns the number of scripts dropped.
//
// Clear 清空队列并返回丢掉的 script 数。
func (m *MockClient) Clear() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := len(m.queue)
	m.queue = nil
	return n
}

// LastRequest returns the most recent Stream call's Request payload.
//
// LastRequest 返最近一次 Stream 调用的 Request 载荷。
func (m *MockClient) LastRequest() Request {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastRequest
}

// CallCount returns the total Stream invocations since process start.
//
// CallCount 返进程启动以来 Stream 调用总数。
func (m *MockClient) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// Stream pops the next script and emits its events; empty queue emits a single EventError.
//
// Stream 弹下个 script 并发其事件；队列空时只发一个 EventError。
func (m *MockClient) Stream(ctx context.Context, req Request) iter.Seq[StreamEvent] {
	m.mu.Lock()
	m.lastRequest = req
	m.callCount++
	var script MockScript
	var ok bool
	if len(m.queue) > 0 {
		script = m.queue[0]
		m.queue = m.queue[1:]
		ok = true
	}
	m.mu.Unlock()

	return func(yield func(StreamEvent) bool) {
		if !ok {
			yield(StreamEvent{
				Type: EventError,
				Err:  errors.New("mock-llm: queue empty — push a script via /dev/mock-llm/scripts before sending the chat message"),
			})
			return
		}
		if script.ErrAfter != nil {
			yield(StreamEvent{Type: EventError, Err: script.ErrAfter})
			return
		}
		for _, ev := range script.Events {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if !yield(ev) {
				return
			}
		}
	}
}
