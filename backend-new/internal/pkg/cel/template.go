package cel

import (
	"fmt"
	"strings"
)

// Template is a compiled `{{ CEL }}` interpolation template for the approval form's rendered
// markdown (and any other text field that interpolates an entity input). Literal text and
// compiled `{{ expr }}` spans are kept interleaved; Render evaluates each span over the
// caller's activation and stringifies it into the literals. The spans share the same env as
// bare CEL (payload/ctx/input, no now()), so a rendered prompt stays replay-deterministic.
//
// Template 是 approval 表单渲染 markdown（及其它插值实体输入的文本字段）的 `{{ CEL }}` 插值模板。
// 字面文本与编译后的 `{{ expr }}` 段交错保存；Render 对每段按调用方 activation 求值并字符串化插回字面。
// 段与裸 CEL 共用同一 env（payload/ctx/input、无 now()），故渲染结果重放确定。
type Template struct {
	parts []tmplPart
	src   string
}

// tmplPart is either a literal text chunk (prog == nil) or a compiled `{{ expr }}` span.
//
// tmplPart 是一段字面文本（prog == nil）或一个编译后的 `{{ expr }}` 段。
type tmplPart struct {
	literal string
	prog    *Program
}

// CompileTemplate parses `{{ expr }}` spans, compiles each via the shared env, and keeps
// the literal text between. An unterminated `{{`, or a syntax error / unknown function in
// any span, fails here — call it at create/edit time so authoring errors fail fast. A
// template with no spans is valid (a pure literal).
//
// CompileTemplate 解析 `{{ expr }}` 段，各自经共享 env 编译，保留其间字面文本。未闭合的 `{{`、
// 任一段语法错 / 未知函数在此失败——create/edit 时调以快速失败。无 `{{ }}` 段的纯字面模板亦合法。
func CompileTemplate(tmpl string) (*Template, error) {
	t := &Template{src: tmpl}
	rest := tmpl
	for {
		open := strings.Index(rest, "{{")
		if open < 0 {
			if rest != "" {
				t.parts = append(t.parts, tmplPart{literal: rest})
			}
			return t, nil
		}
		if open > 0 {
			t.parts = append(t.parts, tmplPart{literal: rest[:open]})
		}
		rest = rest[open+2:]
		end := strings.Index(rest, "}}")
		if end < 0 {
			return nil, fmt.Errorf("cel.CompileTemplate %q: unterminated {{", tmpl)
		}
		expr := strings.TrimSpace(rest[:end])
		prog, err := Compile(expr)
		if err != nil {
			return nil, fmt.Errorf("cel.CompileTemplate: %w", err)
		}
		t.parts = append(t.parts, tmplPart{prog: prog})
		rest = rest[end+2:]
	}
}

// Render evaluates each `{{ expr }}` span over the caller's activation (vars), stringifies
// the result, and splices it between the literal chunks. A span eval error aborts (the
// interpreter decides how to surface it). Used by the workflow durable interpreter (波次 4),
// not at authoring.
//
// Render 对每个 `{{ expr }}` 段按调用方 activation（vars）求值、字符串化、插回字面间。某段求值错
// 则中止（解释器决定如何上呈）。由 workflow durable 解释器（波次 4）使用，非编写期。
func (t *Template) Render(vars map[string]any) (string, error) {
	var b strings.Builder
	for _, p := range t.parts {
		if p.prog == nil {
			b.WriteString(p.literal)
			continue
		}
		v, err := p.prog.Eval(vars)
		if err != nil {
			return "", fmt.Errorf("cel.Render %q: %w", t.src, err)
		}
		b.WriteString(stringify(v))
	}
	return b.String(), nil
}

// stringify renders a CEL value as text for template interpolation (nil → "").
//
// stringify 把 CEL 值渲染成插值文本（nil → ""）。
func stringify(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}
