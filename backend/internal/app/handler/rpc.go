// rpc.go — System composes the Python class string from VersionDraft +
// produces the driver template. Driver is identical across handlers (it's
// the JSON-line RPC dispatcher that imports user_handler.HandlerImpl).
//
// Both strings are written to the sandboxed handler's versions/<vID>/ dir
// before SpawnLongLived; the subprocess `python driver.py` imports the user
// class and serves RPC over stdin/stdout.
//
// rpc.go —— 系统从 VersionDraft 拼 Python class 字符串 + driver 模板。
// driver 全 handler 一致(JSON-line RPC 派发器,import user_handler.HandlerImpl)。
// spawn 前两个文件落到 versions/<vID>/,子进程 `python driver.py`。

package handler

import (
	"fmt"
	"strings"

	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
)

// AssembleClass produces the user_handler.py contents from a VersionDraft.
// Layout (mirrors spec/03-handler.md §5.1, 2026-05 refactored to exploded
// named params per #8 to match Function pattern):
//
//	<Imports>
//
//	class HandlerImpl:
//	    def __init__(self, <p1>: <type1>, <p2>: <type2> = None, ...):
//	        <InitBody>   # uses bare names: self.x = p1
//	    def shutdown(self):
//	        <ShutdownBody>     # or `pass` if not provided
//	    def <m1.Name>(self, <a1>: <typeA>, <a2>: <typeB> = None, ...):
//	        <m1.Body>    # uses bare names: do_something(a1, a2)
//	    ... per method
//
// Indentation: each body is reindented to 8 spaces (class body 4 + def body 4).
// Param types use Python annotations from the JSON-schema type whitelist
// (string→str / integer→int / number→float / boolean→bool / object→dict /
// array→list). Optional (Required=false) params get `= None` default unless
// an explicit Default is provided.
//
// AssembleClass 2026-05 重构:用 exploded named params(从 initArgsSchema /
// method.args 展开)跟 function 框架对齐;body 写裸名 self.x=p1 而非
// init_args["p1"]。
func AssembleClass(d *VersionDraft) string {
	var b strings.Builder
	b.WriteString("# Auto-assembled by Forgify from ops; do not edit by hand.\n")
	if d.Imports != "" {
		b.WriteString(d.Imports)
		if !strings.HasSuffix(d.Imports, "\n") {
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	b.WriteString("class HandlerImpl:\n")

	// __init__ — exploded params from InitArgsSchema.
	// __init__ 从 InitArgsSchema 展开 named params。
	b.WriteString("    def __init__(self")
	for _, a := range d.InitArgsSchema {
		writeInitArgParam(&b, a)
	}
	b.WriteString("):\n")
	if d.InitBody == "" {
		b.WriteString("        pass\n")
	} else {
		writeIndented(&b, d.InitBody, "        ")
	}
	b.WriteByte('\n')

	// shutdown
	b.WriteString("    def shutdown(self):\n")
	if d.ShutdownBody == "" {
		b.WriteString("        pass\n")
	} else {
		writeIndented(&b, d.ShutdownBody, "        ")
	}
	b.WriteByte('\n')

	// Methods
	for _, m := range d.Methods {
		writeMethod(&b, m)
		b.WriteByte('\n')
	}
	return b.String()
}

// writeInitArgParam writes one `, name: type` (or `, name: type = default`)
// param suffix. Caller already wrote the open paren + `self`.
//
// writeInitArgParam 写一个 init arg 参数后缀。
func writeInitArgParam(b *strings.Builder, a handlerdomain.InitArgSpec) {
	fmt.Fprintf(b, ", %s: %s", a.Name, pythonType(a.Type))
	if !a.Required {
		fmt.Fprintf(b, " = %s", pythonDefault(a.Default))
	}
}

// writeMethod writes one `def <name>(self, <arg1>: ..., ...):` block with
// exploded params from m.Args.
//
// writeMethod 写一个 `def <name>(self, <args 展开>):` 方法块。
func writeMethod(b *strings.Builder, m handlerdomain.MethodSpec) {
	fmt.Fprintf(b, "    def %s(self", m.Name)
	for _, a := range m.Args {
		fmt.Fprintf(b, ", %s: %s", a.Name, pythonType(a.Type))
		if !a.Required {
			fmt.Fprintf(b, " = %s", pythonDefault(a.Default))
		}
	}
	b.WriteString("):\n")
	if m.Body == "" {
		b.WriteString("        pass\n")
		return
	}
	writeIndented(b, m.Body, "        ")
}

// pythonType maps the JSON-schema arg type to a Python type annotation.
// Whitelist matches isValidArgType() in validate.go.
//
// pythonType 把 JSON-schema arg type 映射到 Python 类型注解。
func pythonType(t string) string {
	switch t {
	case "string":
		return "str"
	case "integer":
		return "int"
	case "number":
		return "float"
	case "boolean":
		return "bool"
	case "object":
		return "dict"
	case "array":
		return "list"
	default:
		// Should never happen — validate.go's isValidArgType gates this.
		// 不可能到这,validate.go::isValidArgType 已过滤。
		return "object"
	}
}

// pythonDefault renders a Go any value as a Python expression for use as
// a default arg value. Nil → "None"; strings → repr-quoted; bools →
// Python-cased; numbers → fmt.Sprint.
//
// pythonDefault 把 Go any 渲染成 Python 默认值表达式。
func pythonDefault(v any) string {
	if v == nil {
		return "None"
	}
	switch x := v.(type) {
	case bool:
		if x {
			return "True"
		}
		return "False"
	case string:
		// Python single-quoted with backslash-escape; \" / \\ both escaped.
		// Python 单引号 + 反斜杠转义。
		var sb strings.Builder
		sb.WriteByte('"')
		for _, r := range x {
			switch r {
			case '\\':
				sb.WriteString(`\\`)
			case '"':
				sb.WriteString(`\"`)
			case '\n':
				sb.WriteString(`\n`)
			case '\r':
				sb.WriteString(`\r`)
			case '\t':
				sb.WriteString(`\t`)
			default:
				sb.WriteRune(r)
			}
		}
		sb.WriteByte('"')
		return sb.String()
	case float64, float32, int, int32, int64:
		return fmt.Sprintf("%v", x)
	default:
		// Maps / slices: render as Python via JSON (JSON and Python literals
		// align for these types — true/false uppercase notwithstanding, and
		// our v=nil/bool cases are already handled above).
		// map / slice 走 JSON 渲染(JSON 跟 Python 字面量对得上)。
		return fmt.Sprintf("%v", x)
	}
}

// writeIndented writes each non-empty line of src prefixed with indent.
// Blank lines stay blank (no trailing whitespace).
//
// writeIndented 每非空行加 indent;空行保空。
func writeIndented(b *strings.Builder, src, indent string) {
	for _, line := range strings.Split(src, "\n") {
		if line == "" {
			b.WriteByte('\n')
			continue
		}
		b.WriteString(indent)
		b.WriteString(line)
		b.WriteByte('\n')
	}
}

// DriverScript is the constant Python driver that serves the JSON-line RPC
// protocol (matches spec/03-handler.md §5.5). Identical across handlers;
// the user's HandlerImpl is imported at module level.
//
// DriverScript 是恒定的 Python driver(JSON-line RPC 派发器,spec §5.5)。
// 全 handler 一致;user 的 HandlerImpl module 级 import。
const DriverScript = `# Auto-generated by Forgify; do not edit.
import sys, json, traceback
from user_handler import HandlerImpl


def respond(payload):
    sys.stdout.write(json.dumps(payload) + "\n")
    sys.stdout.flush()


def main():
    # init
    init_line = sys.stdin.readline()
    if not init_line:
        return
    init_msg = json.loads(init_line)
    try:
        handler = HandlerImpl(**init_msg.get("args", {}))
        respond({"type": "ready"})
    except Exception as e:
        respond({
            "type": "init_error",
            "error": str(e),
            "trace": traceback.format_exc(),
        })
        return

    # message loop
    for line in sys.stdin:
        if not line.strip():
            continue
        msg = json.loads(line)
        msg_type = msg.get("type")
        if msg_type == "shutdown":
            try:
                handler.shutdown()
            except Exception:
                pass
            break
        if msg_type == "call":
            request_id = msg["id"]
            method_name = msg["method"]
            args = msg.get("args", {}) or {}
            # private methods (leading underscore) are not LLM-callable
            if method_name.startswith("_"):
                respond({
                    "type": "error",
                    "id": request_id,
                    "error": "private method: " + method_name,
                    "trace": "",
                })
                continue
            try:
                method = getattr(handler, method_name)
            except AttributeError:
                respond({
                    "type": "error",
                    "id": request_id,
                    "error": "no such method: " + method_name,
                    "trace": "",
                })
                continue
            try:
                result = method(**args)
                # generator → progress + final
                if hasattr(result, "__iter__") and not isinstance(result, (str, bytes, list, dict)):
                    final = None
                    for item in result:
                        if isinstance(item, dict) and "progress" in item:
                            respond({"type": "progress", "id": request_id, "data": item["progress"]})
                        else:
                            final = item
                    respond({"type": "return", "id": request_id, "data": final})
                else:
                    respond({"type": "return", "id": request_id, "data": result})
            except Exception as e:
                respond({
                    "type": "error",
                    "id": request_id,
                    "error": str(e),
                    "trace": traceback.format_exc(),
                })


if __name__ == "__main__":
    main()
`
