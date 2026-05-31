package workflow

import (
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types/ref"
)

// celEnv is the shared CEL environment (ADR-011). Expressions read `payload` and `ctx` only;
// the env exposes no now()/wall-clock function, so control-flow guards stay replay-deterministic
// (00 §determinism). This replaces the retired Go text/template engine for case.when / emit / args.
//
// celEnv 是共享 CEL 环境;表达式只读 payload/ctx,无 now()/墙钟,控制流重放确定。
var celEnv *cel.Env

func init() {
	env, err := cel.NewEnv(
		cel.Variable("payload", cel.DynType),
		cel.Variable("ctx", cel.DynType),
	)
	if err != nil {
		panic(fmt.Sprintf("workflow.celEnv init: %v", err))
	}
	celEnv = env
}

// CELProgram is a compiled bare-CEL expression (case.when guard / emit field / tool.args field).
//
// CELProgram 是编译后的裸 CEL 表达式。
type CELProgram struct {
	prg cel.Program
	src string
}

// CompileCEL compiles a bare CEL expression that reads payload/ctx. A syntax error or an
// unknown function (e.g. now()) fails here, at accept time.
//
// CompileCEL 编译裸 CEL 表达式;语法错 / 未知函数(如 now())在此 accept 期失败。
func CompileCEL(expr string) (*CELProgram, error) {
	ast, iss := celEnv.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return nil, fmt.Errorf("workflow.CompileCEL %q: %w", expr, iss.Err())
	}
	prg, err := celEnv.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("workflow.CompileCEL %q: program: %w", expr, err)
	}
	return &CELProgram{prg: prg, src: expr}, nil
}

// Eval evaluates to a native Go value (emit / tool.args produce typed values: payload.x+1 → 6).
//
// Eval 求值为 native Go 值(emit/args 产出 typed 值)。
func (p *CELProgram) Eval(payload, ctxVar map[string]any) (any, error) {
	out, _, err := p.prg.Eval(activation(payload, ctxVar))
	if err != nil {
		return nil, fmt.Errorf("workflow.Eval %q: %w", p.src, err)
	}
	return refToGo(out), nil
}

// EvalBool evaluates to bool (case.when). An evaluation error (e.g. missing field) returns it so
// the interpreter applies fail-to-false (G9). A non-bool result is an authoring error.
//
// EvalBool 求值为 bool(case.when);求值错(如缺字段)返 err,解释器据此 fail-to-false(G9)。
func (p *CELProgram) EvalBool(payload, ctxVar map[string]any) (bool, error) {
	out, _, err := p.prg.Eval(activation(payload, ctxVar))
	if err != nil {
		return false, fmt.Errorf("workflow.EvalBool %q: %w", p.src, err)
	}
	if b, ok := out.Value().(bool); ok {
		return b, nil
	}
	return false, fmt.Errorf("workflow.EvalBool %q: result is %T, not bool", p.src, out.Value())
}

func activation(payload, ctxVar map[string]any) map[string]any {
	if payload == nil {
		payload = map[string]any{}
	}
	if ctxVar == nil {
		ctxVar = map[string]any{}
	}
	return map[string]any{"payload": payload, "ctx": ctxVar}
}

// refToGo converts a CEL ref.Val to native Go, recursing into lists/maps so emit can build
// nested payloads.
//
// refToGo 把 CEL ref.Val 转 native Go,递归 list/map 让 emit 可构造嵌套 payload。
func refToGo(v ref.Val) any {
	switch val := v.Value().(type) {
	case []ref.Val:
		out := make([]any, len(val))
		for i, e := range val {
			out[i] = refToGo(e)
		}
		return out
	case map[ref.Val]ref.Val:
		out := make(map[string]any, len(val))
		for k, e := range val {
			out[fmt.Sprint(k.Value())] = refToGo(e)
		}
		return out
	default:
		return val
	}
}
