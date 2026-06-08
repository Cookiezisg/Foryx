// Package sensor is the sensor source listener: it periodically invokes a bound function or
// handler method, evaluates a CEL condition over the return value, and fires (with a
// CEL-built payload) when the condition holds. EVERY probe is reported (fired or not) so the
// activation log can answer "why didn't it fire". Stateless by design — for incremental /
// cursor probing, target a handler method (the resident process keeps its own state). One
// goroutine per triggerID.
//
// Package sensor 是 sensor source listener：周期调用绑定的 function 或 handler method，对返回值求
// CEL condition，满足则用 CEL output 构造 payload 并 fire。**每次探测都报告**（触没触发都报）让
// activation 日志能回答「为什么没触发」。无状态——要游标/增量就绑 handler method（常驻进程自记状态）。
package sensor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
	triggerinfra "github.com/sunweilin/forgify/backend/internal/infra/trigger"
	celpkg "github.com/sunweilin/forgify/backend/internal/pkg/cel"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// SensorInvoker invokes the sensor's bound function or handler-method and returns its result
// as a map. Implemented by adapters over the function / handler app services (wired at boot).
// targetKind is "function" | "handler"; method is used only for handler.
//
// SensorInvoker 调用 sensor 绑定的 function 或 handler method，返回结果 map。由 function/handler app
// 适配器实现（boot 注入）。targetKind = "function" | "handler"；method 仅 handler 用。
type SensorInvoker interface {
	Invoke(ctx context.Context, targetKind, targetID, method string) (map[string]any, error)
}

type entry struct {
	cancel context.CancelFunc
}

// Listener runs one probe goroutine per sensor trigger.
//
// Listener 每个 sensor trigger 起一个探测 goroutine。
type Listener struct {
	mu      sync.Mutex
	entries map[string]*entry // key: triggerID
	invoker SensorInvoker
	report  triggerinfra.ReportFunc
	log     *zap.Logger
}

// New constructs a sensor Listener; invoker resolves the bound function/handler.
//
// New 构造 sensor Listener；invoker 解析绑定的 function/handler。
func New(invoker SensorInvoker, log *zap.Logger, report triggerinfra.ReportFunc) *Listener {
	return &Listener{
		entries: make(map[string]*entry),
		invoker: invoker,
		report:  report,
		log:     log.Named("trigger.sensor"),
	}
}

// Register compiles the CEL condition/output and starts the probe goroutine. workspaceID seeds
// the probe ctx so Invoke runs under the trigger's workspace isolation.
//
// Register 编译 CEL condition/output 并起探测 goroutine。workspaceID 种入探测 ctx，使 Invoke 在
// trigger 的 workspace 隔离下运行。
func (l *Listener) Register(triggerID, workspaceID string, config map[string]any) error {
	sc := triggerdomain.ParseSensorConfig(config)
	condProg, err := celpkg.Compile(sc.Condition)
	if err != nil {
		return fmt.Errorf("sensor.Register %s: condition: %w", triggerID, err)
	}
	outProg, err := celpkg.Compile(sc.Output)
	if err != nil {
		return fmt.Errorf("sensor.Register %s: output: %w", triggerID, err)
	}
	interval := time.Duration(sc.IntervalSec) * time.Second
	if min := triggerdomain.MinSensorIntervalSec * time.Second; interval < min {
		interval = min
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if e, ok := l.entries[triggerID]; ok {
		e.cancel()
	}
	ctx, cancel := context.WithCancel(reqctxpkg.SetWorkspaceID(context.Background(), workspaceID))
	l.entries[triggerID] = &entry{cancel: cancel}
	go l.loop(ctx, triggerID, sc, condProg, outProg, interval)
	return nil
}

// Unregister stops triggerID's probe goroutine; no-op when absent.
//
// Unregister 停 triggerID 的探测 goroutine；不存在则 no-op。
func (l *Listener) Unregister(triggerID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e, ok := l.entries[triggerID]; ok {
		e.cancel()
		delete(l.entries, triggerID)
	}
}

// Start is a no-op — each probe goroutine starts in Register.
//
// Start 是 no-op——每个探测 goroutine 在 Register 里启动。
func (l *Listener) Start() {}

// Stop cancels all probe goroutines.
//
// Stop 取消所有探测 goroutine。
func (l *Listener) Stop() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.entries {
		e.cancel()
	}
	l.entries = make(map[string]*entry)
}

func (l *Listener) loop(ctx context.Context, triggerID string, sc triggerdomain.SensorConfig, cond, out *celpkg.Program, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	l.probe(ctx, triggerID, sc, cond, out) // probe once on registration
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.probe(ctx, triggerID, sc, cond, out)
		}
	}
}

// probe runs one invoke→condition→output cycle and reports the outcome (fired or not).
//
// probe 跑一轮 invoke→condition→output 并报告结果（触没触发都报）。
func (l *Listener) probe(ctx context.Context, triggerID string, sc triggerdomain.SensorConfig, cond, out *celpkg.Program) {
	defer func() {
		if r := recover(); r != nil {
			l.log.Error("sensor probe panic", zap.String("triggerID", triggerID), zap.Any("recover", r))
		}
	}()

	rv, err := l.invoker.Invoke(ctx, sc.TargetKind, sc.TargetID, sc.Method)
	if err != nil {
		l.report(triggerID, triggerinfra.Activity{Fired: false, Error: err.Error(), Detail: "invoke failed"})
		return
	}
	met, evalErr := cond.EvalBool(map[string]any{"payload": rv})
	if evalErr != nil {
		l.report(triggerID, triggerinfra.Activity{Fired: false, ReturnValue: rv, Error: evalErr.Error(), Detail: "condition eval error"})
		return
	}
	if !met {
		l.report(triggerID, triggerinfra.Activity{Fired: false, ReturnValue: rv, Detail: "condition evaluated false"})
		return
	}
	payloadAny, outErr := out.Eval(map[string]any{"payload": rv})
	if outErr != nil {
		l.report(triggerID, triggerinfra.Activity{Fired: false, ReturnValue: rv, Error: outErr.Error(), Detail: "output eval error"})
		return
	}
	l.report(triggerID, triggerinfra.Activity{Fired: true, ReturnValue: rv, Payload: toMap(payloadAny)})
}

// toMap coerces a CEL output value into a payload map; a non-map result is wrapped under "value".
//
// toMap 把 CEL output 结果归一成 payload map；非 map 结果包在 "value" 下。
func toMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{"value": v}
}

var _ triggerinfra.Listener = (*Listener)(nil)
