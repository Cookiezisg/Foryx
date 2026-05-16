package scheduler

import (
	"context"
	"fmt"
)

// LLMCaller is the port LLMDispatcher consumes; main.go wires the concrete impl.
//
// LLMCaller 是 LLMDispatcher 消费的端口；main.go 装配具体实现。
type LLMCaller interface {
	// Generate calls the LLM for a scenario with prompt + vars and returns the text.
	//
	// Generate 按 scenario 调 LLM，返生成文本。
	Generate(ctx context.Context, scenario, prompt string, vars map[string]any) (string, error)
}

// LLMDispatcher bridges workflow llm nodes to the LLMCaller port.
//
// LLMDispatcher 把 workflow llm 节点桥接到 LLMCaller。
type LLMDispatcher struct {
	caller LLMCaller
}

// NewLLMDispatcher constructs LLMDispatcher; nil caller makes every dispatch error.
//
// NewLLMDispatcher 构造 LLMDispatcher；nil caller 时每次 dispatch 返错。
func NewLLMDispatcher(caller LLMCaller) *LLMDispatcher {
	return &LLMDispatcher{caller: caller}
}

// Dispatch reads scenario + prompt from node.Config and invokes the LLM.
//
// Dispatch 读 scenario + prompt 并调 LLM。
func (d *LLMDispatcher) Dispatch(ctx context.Context, in DispatchInput) DispatchOutput {
	if d.caller == nil {
		return DispatchOutput{Error: fmt.Errorf("llm node %q: no LLMCaller wired", in.Node.ID)}
	}
	scenario, _ := in.Node.Config["scenario"].(string)
	prompt, _ := in.Node.Config["prompt"].(string)
	if scenario == "" {
		scenario = "chat"
	}
	if prompt == "" {
		return DispatchOutput{Error: fmt.Errorf("llm node %q: prompt required", in.Node.ID)}
	}

	out, err := d.caller.Generate(ctx, scenario, prompt, in.ExecCtx.Variables)
	if err != nil {
		return DispatchOutput{Error: err}
	}
	return DispatchOutput{Outputs: map[string]any{"out": out}}
}
