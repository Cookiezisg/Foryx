package forge

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ParsedCode holds the structural metadata extracted from a single-function
// Python source file using a Python subprocess.
//
// ParsedCode 保存从单函数 Python 源文件通过 Python subprocess 提取的结构元数据。
type ParsedCode struct {
	FuncName   string
	Parameters []ParsedParam
	Return     ParsedReturn
	Docstring  string
}

// ParsedParam describes one function parameter.
//
// ParsedParam 描述一个函数参数。
type ParsedParam struct {
	Name        string
	Type        string
	Required    bool    // true when no default value / 无默认值时为 true
	Description string  // from Google-style Args: section / 从 Google-style Args: 段提取
	Default     *string // nil when no default / 无默认值时为 nil
}

// ParsedReturn describes the function return value.
//
// ParsedReturn 描述函数返回值。
type ParsedReturn struct {
	Type        string // from return annotation / 从返回类型注解提取
	Description string // from Google-style Returns: section / 从 Returns: 段提取
}

// astScript is the Python code executed in a subprocess to extract function
// metadata. It outputs a single JSON object to stdout.
//
// astScript 是在 subprocess 中执行的 Python 代码，从函数中提取元数据，
// 将结果以单个 JSON 对象输出到 stdout。
const astScript = `
import ast, json, sys, inspect, textwrap

src = sys.stdin.read()
try:
    tree = ast.parse(src)
except SyntaxError as e:
    print(json.dumps({"error": str(e)}))
    sys.exit(0)

func_def = None
for node in ast.walk(tree):
    if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
        func_def = node
        break

if func_def is None:
    print(json.dumps({"error": "no function definition found"}))
    sys.exit(0)

# --- parameters ---
params = []
args = func_def.args
defaults = args.defaults
n = len(args.args)
offset = n - len(defaults)

for i, arg in enumerate(args.args):
    if arg.arg == "self":
        continue
    annotation = ""
    if arg.annotation:
        annotation = ast.unparse(arg.annotation)
    default_val = None
    required = True
    if i >= offset:
        raw = ast.unparse(defaults[i - offset])
        default_val = raw
        required = False
    params.append({
        "name": arg.arg,
        "type": annotation,
        "required": required,
        "description": "",
        "default": default_val,
    })

# --- return annotation ---
return_type = ""
if func_def.returns:
    return_type = ast.unparse(func_def.returns)

# --- docstring & Google-style extraction ---
docstring = ast.get_docstring(func_def) or ""
args_desc = {}
returns_desc = ""

if docstring:
    lines = textwrap.dedent(docstring).splitlines()
    section = None
    for line in lines:
        stripped = line.strip()
        if stripped in ("Args:", "Arguments:"):
            section = "args"
            continue
        if stripped in ("Returns:", "Return:"):
            section = "returns"
            continue
        if stripped.endswith(":") and not stripped.startswith(" "):
            section = None
            continue
        if section == "args" and stripped:
            if ":" in stripped:
                pname, pdesc = stripped.split(":", 1)
                args_desc[pname.strip()] = pdesc.strip()
        elif section == "returns" and stripped:
            returns_desc += (" " if returns_desc else "") + stripped

for p in params:
    p["description"] = args_desc.get(p["name"], "")

print(json.dumps({
    "func_name": func_def.name,
    "parameters": params,
    "return": {"type": return_type, "description": returns_desc},
    "docstring": docstring,
}))
`

// parseForgeCode launches a Python subprocess to parse the function structure
// of code. pythonPath is the absolute path to a Python interpreter (typically
// the bundled python-build-standalone via Sandbox.PythonPath()); empty falls
// back to the PATH-resolved "python3" for tests / dev that haven't bootstrapped.
//
// Requires Python 3.9+ for ast.unparse. Description fields are empty when the
// code lacks a Google-style docstring — this is not an error.
//
// parseForgeCode 启动 Python subprocess 解析 code 的函数结构。pythonPath 是
// Python 解释器绝对路径（通常 Sandbox.PythonPath() 给出的捆绑
// python-build-standalone）；空时回退到 PATH 上的 "python3"（测试 / 未
// bootstrap 的 dev 环境）。
//
// 需要 Python 3.9+（ast.unparse）。代码缺 Google-style docstring 时
// Description 字段为空字符串，不报错。
func parseForgeCode(pythonPath, code string) (*ParsedCode, error) {
	if pythonPath == "" {
		pythonPath = "python3"
	}
	tmp, err := os.CreateTemp("", "forgify-ast-*.py")
	if err != nil {
		return nil, fmt.Errorf("parseForgeCode: create temp: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.WriteString(astScript); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("parseForgeCode: write script: %w", err)
	}
	tmp.Close()

	cmd := exec.Command(pythonPath, tmp.Name())
	cmd.Stdin = strings.NewReader(code)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("%w: %s", errASTProcess, exitErr.Stderr)
		}
		return nil, fmt.Errorf("%w: %v", errASTProcess, err)
	}

	var raw struct {
		Error    string `json:"error"`
		FuncName string `json:"func_name"`
		Params   []struct {
			Name        string  `json:"name"`
			Type        string  `json:"type"`
			Required    bool    `json:"required"`
			Description string  `json:"description"`
			Default     *string `json:"default"`
		} `json:"parameters"`
		Return struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"return"`
		Docstring string `json:"docstring"`
	}
	if err = json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parseForgeCode: unmarshal: %w", err)
	}
	if raw.Error != "" {
		return nil, fmt.Errorf("parseForgeCode: %w: %s", errASTProcess, raw.Error)
	}

	params := make([]ParsedParam, len(raw.Params))
	for i, p := range raw.Params {
		params[i] = ParsedParam{
			Name:        p.Name,
			Type:        p.Type,
			Required:    p.Required,
			Description: p.Description,
			Default:     p.Default,
		}
	}

	return &ParsedCode{
		FuncName:   raw.FuncName,
		Parameters: params,
		Return:     ParsedReturn{Type: raw.Return.Type, Description: raw.Return.Description},
		Docstring:  raw.Docstring,
	}, nil
}

// errASTProcess is a sentinel wrapping all Python-side parse failures.
// The caller maps this to tooldomain.ErrASTParseError.
//
// errASTProcess 封装所有 Python 侧解析失败。调用方将其映射到 tooldomain.ErrASTParseError。
var errASTProcess = fmt.Errorf("ast parse failed")
