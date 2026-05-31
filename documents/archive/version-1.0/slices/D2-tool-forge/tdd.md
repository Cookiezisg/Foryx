# D2 · 工具锻造 — 技术设计文档

**切片**：D2  
**状态**：待 Review

---

## 1. 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 代码生成 | 用户选择的 LLM + ForgeSystemPrompt | 灵活，质量取决于模型 |
| 代码提取 | Python AST (`ast.parse`) + 正则 fallback | AST 100% 准确提取函数签名、泛型类型如 `list[int]`、@元数据 |
| 元数据标注 | `# @version / @category / @display_name / @description` | 代码即文档，零额外 API 调用 |
| 标注同步 | `NormalizeCodeAnnotations()` | DB 字段 = 权威，代码标注跟随同步 |
| 锻造会话 | 复用 ConversationService，ForgeSystemPrompt 注入 | 复用消息流 UI |
| 工具调用 | Eino InvokableTool (create_tool / update_tool_code) | 绑定对话用 tool calling，未绑定用 DetectCodeInResponse |

---

## 2. 目录结构

```
internal/forge/
├── agent.go             # ForgeSystemPrompt + DetectCodeInResponse
├── parser.go            # ParseFunction (AST+regex fallback)、ParseMeta、NormalizeCodeAnnotations、ExtractCodeBlock
├── ast_parser.go        # ParseFunctionAST — 调用 Python subprocess 100% 准确解析
├── parse_function.py    # go:embed 嵌入的 Python AST 脚本
└── tools.go             # Eino InvokableTool: CreateToolTool / UpdateToolCodeTool

frontend/src/components/forge/
├── ForgeCodeBlock.tsx   # 代码块检测 + "保存为工具"按钮（未绑定对话用）
└── SaveToolModal.tsx    # 保存工具弹窗（名称/描述/分类表单）
```

---

## 3. Go 层

### `internal/forge/parser.go`

```go
package forge

import (
    "regexp"
    "strings"

    "forgify/internal/service"
)

var (
    reFuncDef  = regexp.MustCompile(`^def (\w+)\((.*?)\)\s*->\s*dict:`)
    reParam    = regexp.MustCompile(`(\w+)\s*:\s*(\w+)`)
    reImport   = regexp.MustCompile(`^(?:import|from)\s+(\S+)`)
)

var stdlibPackages = map[string]bool{
    "os": true, "sys": true, "json": true, "re": true,
    "datetime": true, "time": true, "math": true, "random": true,
    "collections": true, "itertools": true, "functools": true,
    "pathlib": true, "io": true, "typing": true, "dataclasses": true,
    "enum": true, "abc": true, "copy": true, "hashlib": true,
    "hmac": true, "base64": true, "urllib": true, "http": true,
    "email": true, "smtplib": true, "csv": true, "sqlite3": true,
    "subprocess": true, "threading": true, "multiprocessing": true,
    "logging": true, "unittest": true, "contextlib": true,
}

// ParseFunction 从 Python 代码中提取函数元数据
func ParseFunction(code string) (funcName string, params []service.ToolParameter, requirements []string, err error) {
    lines := strings.Split(code, "\n")

    // 提取函数名和参数
    for _, line := range lines {
        if m := reFuncDef.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
            funcName = m[1]
            for _, pm := range reParam.FindAllStringSubmatch(m[2], -1) {
                params = append(params, service.ToolParameter{
                    Name:     pm[1],
                    Type:     normalizeType(pm[2]),
                    Required: true,
                })
            }
            break
        }
    }

    // 提取第三方依赖
    seen := map[string]bool{}
    for _, line := range lines {
        if m := reImport.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
            pkg := strings.Split(m[1], ".")[0]
            if !stdlibPackages[pkg] && !seen[pkg] {
                requirements = append(requirements, pkg)
                seen[pkg] = true
            }
        }
    }

    return funcName, params, requirements, nil
}

func normalizeType(t string) string {
    switch t {
    case "str": return "string"
    case "int": return "int"
    case "float": return "float"
    case "bool": return "bool"
    case "list", "List": return "list"
    case "dict", "Dict": return "dict"
    default: return "string"
    }
}
```

### `internal/forge/agent.go`（当前实现）

```go
// ForgeSystemPrompt — 注入到锻造对话的 system prompt
const ForgeSystemPrompt = `你是 Forgify 的工具锻造助手。
代码格式要求：
  # @version 1.0
  # @category 分类（email/data/web/file/system/other 选一个）
  # @display_name 中文工具名
  # @description 一句话描述
  def function_name(param1: str, param2: int = 0) -> dict:
      """功能描述"""
      return {"result": "..."}
前四行 @version/@category/@display_name/@description 注释绝对不能省略。`

// DetectCodeInResponse — 检查 AI 回复是否包含 Python 代码块并解析
// 用于未绑定工具的对话（用户手动"保存为工具"）
func DetectCodeInResponse(content string) *DetectResult {
    code := ExtractCodeBlock(content)
    // AST 解析 → 正则 fallback → 提取 @metadata
    // 返回 DetectResult{Code, FuncName, DisplayName, Description, Category, Params, Requirements}
}
```

**两种锻造路径：**
1. **未绑定对话**：`DetectCodeInResponse` → SSE `forge.code_detected` → 前端 ForgeCodeBlock 显示"保存为工具"按钮 → 用户点击后 `POST /api/tools` 创建
2. **已绑定对话**：Eino tool calling（`update_tool_code` InvokableTool）→ 代码存为 pending change → 用户在右侧面板 diff review → 接受/拒绝

### `internal/forge/parser.go`（当前实现）

关键函数：
- `ParseFunction(code)` — AST 优先 + 正则 fallback，提取函数签名/参数/依赖
- `ParseFunctionAST(code)` — 调用 Python subprocess（`parse_function.py` go:embed），100% 准确
- `ParseMeta(code)` — 提取 `# @display_name` / `# @description` / `# @category` / `# @version` / `# @builtin` / `# @custom`
- `NormalizeCodeAnnotations(code, ...)` — 确保代码 `# @` 标注与 DB 字段一致（幂等）
- `ExtractCodeBlock(content)` — 从 markdown 中提取第一个 ` ```python ` 代码块

### `internal/forge/tools.go`（Eino 工具定义）

```go
// CreateToolTool — 未绑定对话中 AI 调用创建工具（当前禁用，改为用户手动保存）
// UpdateToolCodeTool — 已绑定对话中 AI 调用更新工具代码（存为 pending change）
```

---

## 4. 事件扩展

在 A3 的事件系统中新增：

```go
// events/events.go
const (
    // ...现有事件...
    ForgeCodeDetected = "forge.code_detected" // AI 生成了工具代码
)
```

TypeScript 侧：
```ts
interface ForgeCodeDetectedPayload {
    conversationId: string
    toolId: string
    funcName: string
}
```

---

## 5. 前端组件

### `ForgeCodeBlock.tsx`

```tsx
// MarkdownContent 组件中，检测到 forge.code_detected 事件时
// 在代码块下方追加操作按钮
export function ForgeCodeBlock({ toolId, code }: { toolId: string; code: string }) {
    const [testing, setTesting] = useState(false)
    const [showSave, setShowSave] = useState(false)

    return (
        <div>
            <SyntaxHighlighter language="python">{code}</SyntaxHighlighter>
            <div className="flex gap-2 mt-2">
                <button
                    onClick={() => setTesting(true)}
                    className="px-3 py-1.5 text-sm bg-neutral-700 rounded-lg flex items-center gap-1"
                >
                    ▶ 测试运行
                </button>
                <button
                    onClick={() => setShowSave(true)}
                    className="px-3 py-1.5 text-sm bg-blue-600 rounded-lg flex items-center gap-1"
                >
                    💾 保存为工具
                </button>
            </div>
            {testing && <TestParamsModal toolId={toolId} onClose={() => setTesting(false)} />}
            {showSave && <SaveToolModal toolId={toolId} onClose={() => setShowSave(false)} />}
        </div>
    )
}
```

### `TestParamsModal.tsx`

```tsx
export function TestParamsModal({ toolId, onClose }: { toolId: string; onClose: () => void }) {
    const [tool, setTool] = useState<Tool | null>(null)
    const [values, setValues] = useState<Record<string, string>>({})
    const [running, setRunning] = useState(false)

    useEffect(() => { GetTool(toolId).then(setTool) }, [toolId])

    const run = async () => {
        setRunning(true)
        const params = Object.fromEntries(
            Object.entries(values).map(([k, v]) => [k, v])
        )
        await RunTool(toolId, params)
        setRunning(false)
        onClose()
    }

    if (!tool) return null
    return (
        <Modal title="测试参数" onClose={onClose}>
            {tool.parameters.map(p => (
                <div key={p.name} className="mb-3">
                    <label className="text-xs text-neutral-400 mb-1 block">{p.name}</label>
                    <input
                        value={values[p.name] ?? ''}
                        onChange={e => setValues(v => ({ ...v, [p.name]: e.target.value }))}
                        className="w-full px-3 py-2 bg-neutral-800 rounded-lg text-sm"
                    />
                </div>
            ))}
            <div className="flex justify-end gap-2">
                <button onClick={onClose} className="px-3 py-1.5 text-sm">取消</button>
                <button onClick={run} disabled={running}
                    className="px-3 py-1.5 text-sm bg-blue-600 rounded-lg">
                    {running ? '运行中...' : '运行测试'}
                </button>
            </div>
        </Modal>
    )
}
```

---

## 6. HTTP API 路由

```go
// backend/internal/server/routes.go
mux.HandleFunc("GET /api/tools/{id}", s.getTool)
mux.HandleFunc("POST /api/tools/{id}/run", s.runTool)
```

---

## 7. 验收测试

```
1. 对话中描述"做一个发送邮件的工具" → AI 回复包含符合规范的 Python 代码
2. 代码块下方有"测试运行"和"保存为工具"按钮
3. 点击"测试运行" → 弹出参数面板 → 填参数 → 运行 → 结果卡片出现
4. 代码有第三方依赖（如 requests）→ 首次运行自动安装
5. 点击"保存为工具" → 命名面板 → 保存 → 工具库可见
6. 对话中继续说"把超时改成 60 秒" → AI 生成新版本代码
```
