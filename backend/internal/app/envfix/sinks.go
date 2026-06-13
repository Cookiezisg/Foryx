// sinks.go provides the reusable Sink adapters: WriterSink renders attempts as human
// lines onto any io.Writer (the entity panel's forge terminal), MultiSink fans one
// provision's events to several observers (caller's chat progress + the panel).
//
// sinks.go 提供可复用 Sink 适配器：WriterSink 把尝试渲染成人类可读行写进任意 io.Writer
// （实体面板锻造终端），MultiSink 把一次物化的事件扇给多个观察者（调用方 chat 进度 + 面板）。
package envfix

import (
	"fmt"
	"io"
	"strings"
)

// NewWriterSink renders provision events as terminal lines (nil writer → inert sink).
//
// NewWriterSink 把物化事件渲染成终端行（nil writer → 惰性 sink）。
func NewWriterSink(w io.Writer) Sink {
	return &writerSink{w: w}
}

type writerSink struct{ w io.Writer }

func (s *writerSink) OnAttempt(a Attempt) {
	if s.w == nil {
		return
	}
	if a.OK {
		fmt.Fprintf(s.w, "✓ env attempt %d ok (deps: %s)\n", a.Number, strings.Join(a.Deps, ", "))
		return
	}
	fmt.Fprintf(s.w, "✗ env attempt %d failed: %s\n", a.Number, a.Error)
}

func (s *writerSink) OnFixing(attempt int) {
	if s.w == nil {
		return
	}
	fmt.Fprintf(s.w, "… repairing dependencies for attempt %d (model-assisted)\n", attempt)
}

// MultiSink fans events to every non-nil sink.
//
// MultiSink 把事件扇给每个非 nil sink。
func MultiSink(sinks ...Sink) Sink {
	var live []Sink
	for _, s := range sinks {
		if s != nil {
			live = append(live, s)
		}
	}
	switch len(live) {
	case 0:
		return nil
	case 1:
		return live[0]
	}
	return multiSink(live)
}

type multiSink []Sink

func (m multiSink) OnAttempt(a Attempt) {
	for _, s := range m {
		s.OnAttempt(a)
	}
}

func (m multiSink) OnFixing(attempt int) {
	for _, s := range m {
		s.OnFixing(attempt)
	}
}
