package logger

import (
	"encoding/json"
	"sync"
	"time"

	"go.uber.org/zap/zapcore"
)

const (
	ringCap   = 500
	subBufCap = 128
)

// LogEntry is one structured log line emitted over SSE.
//
// LogEntry 是通过 SSE 推送的单条结构化日志。
type LogEntry struct {
	Time   string         `json:"time"`
	Level  string         `json:"level"`
	Msg    string         `json:"msg"`
	Fields map[string]any `json:"fields,omitempty"`
}

type logSub struct {
	ch   chan []byte
	once sync.Once
	done chan struct{}
}

// LogBroadcaster is a zapcore.Core that fans entries to SSE subscribers; drops on slow consumers.
//
// LogBroadcaster 实现 zapcore.Core，把日志扇出给 SSE 订阅者；慢订阅者丢条。
type LogBroadcaster struct {
	mu    sync.RWMutex
	ring  [ringCap][]byte
	head  int
	count int
	subs  []*logSub
	ctx   []zapcore.Field
}

// NewLogBroadcaster returns a ready-to-use broadcaster.
//
// NewLogBroadcaster 返回一个可直接使用的广播器。
func NewLogBroadcaster() *LogBroadcaster {
	return &LogBroadcaster{}
}

// Ring returns all buffered entries chronologically (oldest first).
//
// Ring 按时间顺序返回所有缓冲条目（最旧在前）。
func (b *LogBroadcaster) Ring() [][]byte {
	b.mu.RLock()
	defer b.mu.RUnlock()

	n := min(b.count, ringCap)
	out := make([][]byte, n)
	if b.count <= ringCap {
		copy(out, b.ring[:b.count])
	} else {
		for i := range ringCap {
			out[i] = b.ring[(b.head+i)%ringCap]
		}
	}
	return out
}

// Subscribe returns a JSON-bytes channel and an idempotent cancel; drain promptly.
//
// Subscribe 返回 JSON 字节 channel 和幂等 cancel；及时消费否则丢条。
func (b *LogBroadcaster) Subscribe() (<-chan []byte, func()) {
	sub := &logSub{
		ch:   make(chan []byte, subBufCap),
		done: make(chan struct{}),
	}
	b.mu.Lock()
	b.subs = append(b.subs, sub)
	b.mu.Unlock()

	cancel := func() {
		sub.once.Do(func() {
			close(sub.done)
			b.removeSub(sub)
		})
	}
	return sub.ch, cancel
}

func (b *LogBroadcaster) removeSub(target *logSub) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, s := range b.subs {
		if s == target {
			b.subs = append(b.subs[:i], b.subs[i+1:]...)
			return
		}
	}
}

func (b *LogBroadcaster) Enabled(zapcore.Level) bool { return true }

func (b *LogBroadcaster) With(fields []zapcore.Field) zapcore.Core {
	return &broadcasterWith{
		parent: b,
		ctx:    append(append([]zapcore.Field{}, b.ctx...), fields...),
	}
}

func (b *LogBroadcaster) Check(e zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return ce.AddCore(e, b)
}

func (b *LogBroadcaster) Write(e zapcore.Entry, fields []zapcore.Field) error {
	return b.write(e, append(b.ctx, fields...))
}

func (b *LogBroadcaster) Sync() error { return nil }

func (b *LogBroadcaster) write(e zapcore.Entry, fields []zapcore.Field) error {
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range fields {
		f.AddTo(enc)
	}

	entry := LogEntry{
		Time:  e.Time.UTC().Format(time.RFC3339),
		Level: e.Level.String(),
		Msg:   e.Message,
	}
	if len(enc.Fields) > 0 {
		entry.Fields = enc.Fields
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return nil
	}

	b.mu.Lock()
	b.ring[b.head] = data
	b.head = (b.head + 1) % ringCap
	b.count++
	snapshot := make([]*logSub, len(b.subs))
	copy(snapshot, b.subs)
	b.mu.Unlock()

	for _, s := range snapshot {
		select {
		case s.ch <- data:
		case <-s.done:
		default:
		}
	}
	return nil
}

type broadcasterWith struct {
	parent *LogBroadcaster
	ctx    []zapcore.Field
}

func (w *broadcasterWith) Enabled(l zapcore.Level) bool { return w.parent.Enabled(l) }
func (w *broadcasterWith) With(fields []zapcore.Field) zapcore.Core {
	return &broadcasterWith{
		parent: w.parent,
		ctx:    append(append([]zapcore.Field{}, w.ctx...), fields...),
	}
}
func (w *broadcasterWith) Check(e zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return ce.AddCore(e, w)
}
func (w *broadcasterWith) Write(e zapcore.Entry, fields []zapcore.Field) error {
	return w.parent.write(e, append(w.ctx, fields...))
}
func (w *broadcasterWith) Sync() error { return nil }
