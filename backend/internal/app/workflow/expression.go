package workflow

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// EnvWhitelist is the safe set of OS env variables exposed to expressions; non-whitelisted return "".
//
// EnvWhitelist 是允许暴露到表达式的 OS env 白名单，名单外返 ""。
var EnvWhitelist = map[string]bool{
	"USER":     true,
	"HOME":     true,
	"LANG":     true,
	"TZ":       true,
	"HOSTNAME": true,
}

// EvalContext is the runtime input bag for expression evaluation.
//
// EvalContext 是运行期表达式求值的输入袋。
type EvalContext struct {
	Vars     map[string]any
	In       map[string]any
	NodesOut map[string]map[string]any
	Loop     *LoopContext
	Run      RunContext
	Env      map[string]string
}

// LoopContext is the per-iteration state inside a loop body.
//
// LoopContext 是 loop body 每次迭代的状态。
type LoopContext struct {
	Item  any
	Index int
}

// RunContext is the FlowRun-level metadata exposed as {{ run.* }}.
//
// RunContext 是 FlowRun 级元信息，暴露为 {{ run.* }}。
type RunContext struct {
	ID        string
	StartedAt string
}

// Compile parses s as a Go text/template; returns nil template for pure literals.
//
// Compile 把 s 解析为 Go text/template；纯字面量返 nil template。
func Compile(s string) (*template.Template, error) {
	if !strings.Contains(s, "{{") {
		return nil, nil
	}
	tmpl, err := template.New("expr").Funcs(funcMap()).Parse(s)
	if err != nil {
		return nil, fmt.Errorf("expression syntax error: %w (source: %q)", err, s)
	}
	return tmpl, nil
}

// Execute runs a compiled template against ctx; nil template passes literal through.
//
// Execute 在 ctx 上执行已编译 template；nil template 直接返 literal。
func Execute(tmpl *template.Template, ctx EvalContext, literal string) (string, error) {
	if tmpl == nil {
		return literal, nil
	}
	safeEnv := make(map[string]string, len(ctx.Env))
	for k, v := range ctx.Env {
		if EnvWhitelist[k] {
			safeEnv[k] = v
		}
	}
	data := map[string]any{
		"vars":  ctx.Vars,
		"in":    ctx.In,
		"nodes": ctx.NodesOut,
		"run": map[string]any{
			"id":        ctx.Run.ID,
			"startedAt": ctx.Run.StartedAt,
		},
		"env": safeEnv,
	}
	if ctx.Loop != nil {
		data["loop"] = map[string]any{
			"item":  ctx.Loop.Item,
			"index": ctx.Loop.Index,
		}
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("expression eval: %w", err)
	}
	return buf.String(), nil
}

func funcMap() template.FuncMap {
	return template.FuncMap{}
}
