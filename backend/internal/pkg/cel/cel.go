// Package cel compiles and evaluates bare CEL expressions. The permissive package env (Compile)
// declares `payload`/`ctx`/`input` together, and the caller's activation binds whichever roots its
// expression reads — fine at RUNTIME, where the stored expression is already validated. CompileFor
// compiles over EXACTLY a caller-given root set (no auto-ctx) and is used at AUTHOR time to reject a
// wrong-namespace ref up front, mirroring each context's real runtime activation: a control's
// when/emit and an approval template read `input` only; a sensor's condition/output reads `payload`
// only. The env exposes no now()/wall-clock function, so guards stay replay-deterministic. Lives in
// pkg (shared by trigger sensors and the entity layer), not a domain.
//
// Package cel 编译并求值裸 CEL。宽容包 env（Compile）一并声明 `payload`/`ctx`/`input`，调用方 activation
// 绑表达式实际读的根——RUNTIME 用（存储的表达式已校验过）足够。CompileFor 在恰好给定的根集上编译（无
// 自动 ctx），AUTHOR 期用于当场拒绝错命名空间引用，镜像各上下文真实运行时活化：control 的 when/emit 与
// approval 模板只读 `input`；sensor 的 condition/output 只读 `payload`。env 无 now()/墙钟，保证重放确定。
// 放 pkg（trigger sensor 与实体层共用）、非某个 domain。
package cel

import (
	"fmt"
	"strings"

	celgo "github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types/ref"
)

// env is the shared CEL environment. Expressions read payload / ctx / input only.
//
// env 是共享 CEL 环境；表达式只读 payload / ctx / input。
var env *celgo.Env

func init() {
	e, err := celgo.NewEnv(
		celgo.Variable("payload", celgo.DynType),
		celgo.Variable("ctx", celgo.DynType),
		celgo.Variable("input", celgo.DynType),
	)
	if err != nil {
		panic(fmt.Sprintf("pkg/cel: env init: %v", err))
	}
	env = e
}

// Program is a compiled bare-CEL expression.
//
// Program 是编译后的裸 CEL 表达式。
type Program struct {
	prg celgo.Program
	src string
}

// Compile compiles a bare CEL expression over payload/ctx/input. A syntax error or an
// unknown function (e.g. now()) fails here — call it at create/edit time so authoring errors
// fail fast.
//
// Compile 编译读 payload/ctx/input 的裸 CEL 表达式；语法错 / 未知函数（如 now()）在此失败——
// create/edit 时调以快速失败。
func Compile(expr string) (*Program, error) {
	ast, iss := env.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return nil, fmt.Errorf("cel.Compile %q: %w", expr, iss.Err())
	}
	prg, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("cel.Compile %q: program: %w", expr, err)
	}
	return &Program{prg: prg, src: expr}, nil
}

// CompileFor compiles expr over EXACTLY the given root variables (each DynType) — a reference to any
// OTHER root fails compile. Use it at AUTHOR time for entity CEL whose runtime activation binds a
// known, restricted set: control/approval read only `input`, a sensor reads only `payload`. The
// permissive package env (Compile, above) declares payload/ctx/input all at once, so a wrong-namespace
// ref (e.g. payload.x in a control) compiles there and only fails at runtime; CompileFor rejects it
// at create/edit. Author-time only (envs are built per call — not a hot path). No auto-`ctx`: pass
// every root the runtime actually binds, so the env mirrors the activation exactly.
//
// CompileFor 在恰好给定的根变量上编译 expr（各 DynType），引用任何其它根即编译失败。用于 AUTHOR 期
// 校验运行时活化绑定已知受限集的实体 CEL：control/approval 只读 `input`、sensor 只读 `payload`。宽容
// 包 env（上面的 Compile）一次声明 payload/ctx/input，故错命名空间引用（如 control 里的 payload.x）在那
// 能编过、仅运行时崩；CompileFor 在 create/edit 即拒。仅编写期（env 每次现建，非热路径）。无自动 `ctx`：
// 传运行时真正绑定的每个根，使 env 与活化精确一致。
func CompileFor(roots []string, expr string) (*Program, error) {
	opts := make([]celgo.EnvOption, 0, len(roots))
	for _, r := range roots {
		opts = append(opts, celgo.Variable(r, celgo.DynType))
	}
	e, err := celgo.NewEnv(opts...)
	if err != nil {
		return nil, fmt.Errorf("cel.CompileFor %v: %w", roots, err)
	}
	ast, iss := e.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return nil, fmt.Errorf("cel.Compile %q: %w", expr, iss.Err())
	}
	prg, err := e.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("cel.Compile %q: program: %w", expr, err)
	}
	return &Program{prg: prg, src: expr}, nil
}

// ScopedEnv is a CEL environment whose roots are a caller-supplied set of names (each DynType)
// plus the always-present `ctx`. It serves the workflow layer, whose node Input wiring reads
// upstream results by node id (`reviewer.score`) — a per-graph variable set the fixed package
// env (payload/ctx/input) can't name. Build one per graph from its node ids, then Compile each
// wiring expression; a reference to a name outside the set fails compile (a free "the expression
// only references existing nodes" check).
//
// ScopedEnv 是根为「调用方给的一组名字（各 DynType）+ 恒有的 ctx」的 CEL 环境。服务 workflow 层
// ——节点 Input 接线按 node id 读上游结果（`reviewer.score`），是固定包 env（payload/ctx/input）
// 命名不了的、随图而变的变量集。按一张图的 node ids 建一个，再编译它每条接线；引用集合外的名字
// 编译失败（白送一个「只引用存在的节点」校验）。
type ScopedEnv struct{ env *celgo.Env }

// NewScopedEnv builds a ScopedEnv declaring each root (+ ctx) as DynType. A root equal to "ctx"
// is skipped (ctx is always declared) to avoid a duplicate-variable env error.
//
// NewScopedEnv 建一个把每个 root（+ ctx）声明为 DynType 的 ScopedEnv。等于 "ctx" 的 root 跳过
// （ctx 恒已声明），避免重复变量导致 env 报错。
func NewScopedEnv(roots []string) (*ScopedEnv, error) {
	opts := make([]celgo.EnvOption, 0, len(roots)+1)
	opts = append(opts, celgo.Variable("ctx", celgo.DynType))
	for _, r := range roots {
		if r == "ctx" {
			continue
		}
		opts = append(opts, celgo.Variable(r, celgo.DynType))
	}
	e, err := celgo.NewEnv(opts...)
	if err != nil {
		return nil, fmt.Errorf("cel.NewScopedEnv: %w", err)
	}
	return &ScopedEnv{env: e}, nil
}

// Compile compiles a bare CEL expression against the scoped roots, reusing the shared Program.
//
// Compile 针对 scoped 根编译一条裸 CEL 表达式，复用共享 Program 类型。
func (s *ScopedEnv) Compile(expr string) (*Program, error) {
	ast, iss := s.env.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return nil, fmt.Errorf("cel.Compile %q: %w", expr, iss.Err())
	}
	prg, err := s.env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("cel.Compile %q: program: %w", expr, err)
	}
	return &Program{prg: prg, src: expr}, nil
}

// Eval evaluates to a native Go value (recursing into lists/maps). vars is the activation: a
// map naming the root variables the expression reads (e.g. {"input": {...}} for an entity
// expression, or {"payload": {...}} for a sensor expression). A root the expression does not
// reference may be omitted.
//
// Eval 求值为 native Go 值（递归 list/map）。vars 是 activation：命名表达式所读根变量的 map
// （实体表达式传 {"input": …}，sensor 表达式传 {"payload": …}）。表达式不引用的根可省略。
func (p *Program) Eval(vars map[string]any) (any, error) {
	if vars == nil {
		vars = map[string]any{}
	}
	out, _, err := p.prg.Eval(vars)
	if err != nil {
		return nil, fmt.Errorf("cel.Eval %q: %w%s", p.src, err, overloadHint(err))
	}
	return refToGo(out), nil
}

// overloadHint appends an actionable note for the most common CEL eval failure: arithmetic that
// mixes numeric types. A JSON payload/result number binds as a CEL double, an integer literal is a
// CEL int, and cel-go has no cross-type +/-/* overload — so the natural "start.n + 5" fails every
// run with a bare "no such overload" the agent can't decode (it burns turns guessing). Why here:
// this is the one chokepoint every node-input / condition eval passes through.
//
// overloadHint 为最常见的 CEL 求值失败补一句可操作提示：混合数值类型的算术。JSON payload/结果里的数
// 绑成 CEL double、整数字面量是 CEL int，cel-go 无跨类型 +/-/* 重载——故自然的 "start.n + 5" 每次
// run 都崩在裸 "no such overload"、agent 无从解码（白烧若干轮）。放这：每条节点 input/条件求值的唯一收口。
func overloadHint(err error) string {
	if err != nil && strings.Contains(err.Error(), "no such overload") {
		return " (operands have mismatched types — if mixing a payload/result number with an integer literal, cast one: int(start.n) + 5, or write the literal as 5.0)"
	}
	return ""
}

// EvalBool evaluates to bool (a condition guard). A non-bool result is an authoring error.
//
// EvalBool 求值为 bool（条件守卫）；非 bool 结果是编写错误。
func (p *Program) EvalBool(vars map[string]any) (bool, error) {
	if vars == nil {
		vars = map[string]any{}
	}
	out, _, err := p.prg.Eval(vars)
	if err != nil {
		return false, fmt.Errorf("cel.EvalBool %q: %w%s", p.src, err, overloadHint(err))
	}
	if b, ok := out.Value().(bool); ok {
		return b, nil
	}
	return false, fmt.Errorf("cel.EvalBool %q: result is %T, not bool", p.src, out.Value())
}

// refToGo converts a CEL ref.Val to native Go, recursing into lists/maps.
//
// refToGo 把 CEL ref.Val 转 native Go，递归 list/map。
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
