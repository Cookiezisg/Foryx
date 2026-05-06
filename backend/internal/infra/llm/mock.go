// mock.go — MockClient is a Client implementation that replays
// pre-pushed scripts of StreamEvent sequences. Used by the dev
// /dev/mock-llm/* endpoints (and corresponding testend tab) so
// developers can drive chat scenarios without a real LLM provider:
// you push a script via HTTP, send a chat message, the chat runner
// resolves the "mock" provider's apikey + invokes Stream(), Stream
// pops the next script and emits its events through the iterator
// exactly as if a real LLM had streamed them.
//
// This is the production-side cousin of test/harness/fake_llm.go —
// the test harness embeds an httptest server speaking the OpenAI
// wire format; this lives inside the Factory dispatch and runs
// in-process with no HTTP hop.
//
// mock.go ——MockClient 是回放预 push 脚本的 Client 实现。给 dev
// /dev/mock-llm/* 端点（+ 对应 testend tab）用，让开发者无需真 LLM
// provider 即可驱动 chat 场景：经 HTTP push 脚本，发 chat 消息，
// chat runner 解析 "mock" provider 的 apikey 调 Stream，Stream 弹出
// 下个脚本通过迭代器发事件，就像真 LLM 流过来的一样。
//
// 这是 test/harness/fake_llm.go 的生产端表亲——后者嵌 httptest server
// 说 OpenAI wire 格式；这个住在 Factory dispatch 里 in-process 无 HTTP。
package llm

import (
	"context"
	"errors"
	"iter"
	"sync"
)

// MockScript describes one canned response. Events fire in order
// through the iterator returned by Stream. ErrAfter, when set,
// causes the iterator to emit an EventError instead of the events
// (simulates a provider transport failure).
//
// MockScript 描述一段预设响应。Events 按顺序从 Stream 返回的迭代器
// 流出。ErrAfter 设了会让迭代器直接 emit EventError 而非 events
// （模拟 provider 传输失败）。
type MockScript struct {
	Events []StreamEvent

	// ErrAfter, when non-nil, replaces the entire script with a single
	// EventError carrying this error. Lets testers exercise error paths
	// (LLM_STREAM_ERROR / 500 etc.) without crafting an event sequence.
	//
	// ErrAfter 非 nil 时整段 script 替换为单个 EventError 携此 error。
	// 让测试无需手工编排事件即可触错误路径（LLM_STREAM_ERROR / 500 等）。
	ErrAfter error
}

// MockClient holds the FIFO script queue + last-request snapshot.
// Concurrent-safe: every public method takes the mutex briefly. The
// Stream iterator does NOT hold the mutex while yielding (blocking
// the consumer would also block PushScript).
//
// MockClient 持 FIFO script 队列 + 最近请求快照。并发安全：每个公共
// 方法短持锁。Stream 迭代器在 yield 时不持锁（阻塞消费方会同时阻塞
// PushScript）。
type MockClient struct {
	mu          sync.Mutex
	queue       []MockScript
	lastRequest Request
	callCount   int
}

// NewMockClient constructs an empty MockClient. Singleton in production
// — see Factory.NewFactory which wires it once and returns it from
// Build("mock").
//
// NewMockClient 构造空 MockClient。生产中是单例——Factory.NewFactory
// 接一次 Build("mock") 时返同一个。
func NewMockClient() *MockClient {
	return &MockClient{}
}

// PushScript enqueues one script. Multiple Pushes accumulate into a
// FIFO; consecutive Stream calls pop them in push order.
//
// PushScript 入队一段 script。多次 Push 累 FIFO；连续 Stream 调用按
// push 顺序弹。
func (m *MockClient) PushScript(s MockScript) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queue = append(m.queue, s)
}

// QueueDepth returns the current number of unconsumed scripts.
//
// QueueDepth 返回当前未消费的 script 数。
func (m *MockClient) QueueDepth() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.queue)
}

// Queue returns a copy of the current queue (defensive — callers
// shouldn't mutate the internal slice).
//
// Queue 返当前队列的副本（防御性——调用方不该改内部 slice）。
func (m *MockClient) Queue() []MockScript {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]MockScript, len(m.queue))
	copy(out, m.queue)
	return out
}

// Clear empties the queue. Returns the number of scripts dropped so
// the caller can confirm what happened.
//
// Clear 清空队列。返回丢掉的 script 数让调用方确认。
func (m *MockClient) Clear() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := len(m.queue)
	m.queue = nil
	return n
}

// LastRequest returns the most recent Stream call's Request payload
// (system prompt + messages + tool defs). Used by /dev/mock-llm/last-prompt
// so testers can verify what the chat runner actually sent the LLM —
// e.g. confirm the catalog block reached the wire.
//
// LastRequest 返最近一次 Stream 调用的 Request 载荷（system prompt +
// messages + tool defs）。供 /dev/mock-llm/last-prompt 用让测试验
// chat runner 真发了啥给 LLM——如确认 catalog 块进了 wire。
func (m *MockClient) LastRequest() Request {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastRequest
}

// CallCount returns the total number of Stream calls received since
// process start. Useful for asserting "the chat runner called the LLM
// exactly N times" in interactive testing.
//
// CallCount 返进程启动以来收到的 Stream 调用总数。让交互测试断言
// "chat runner 调 LLM 正好 N 次"。
func (m *MockClient) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// Stream pops the next queued script + emits its events through the
// iterator. Empty queue → emits a single EventError so the chat
// runner surfaces it as LLM_STREAM_ERROR (matches what would happen
// with a real provider returning 500). ctx cancellation stops the
// iteration cleanly between events.
//
// Stream 弹出下个 queued script + 通过迭代器发其事件。空队列 → 发
// 单个 EventError 让 chat runner 浮出 LLM_STREAM_ERROR（同真 provider
// 返 500 时的行为）。ctx 取消在 event 间隙干净停。
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
