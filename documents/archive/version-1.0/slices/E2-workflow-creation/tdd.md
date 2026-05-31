# E2 · 对话驱动的工作流创建 — 技术设计文档

**切片**：E2  
**状态**：待 Review

---

## 1. 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| AI 操作工作流 | AI 输出结构化 JSON，Go 解析后更新 DB | 不让 AI 直接调用 DB，有验证层 |
| 工作流 AI 感知 | 隐藏 system 消息注入（每次用户发消息前检查）| 不打扰用户，AI 始终知道当前状态 |
| FlowDefinition 操作 | diff-less：每次 AI 输出完整 FlowDefinition | 简单可靠，不需要 diff/patch |
| 创建时机 | AI 第一次输出完整节点时创建工作流 + 绑定对话 | 不在对话一开始就创建空工作流 |

---

## 2. Go 层

### 工作流创建 Agent 系统 Prompt

```go
const workflowSystemPrompt = `你是 Forgify 的工作流设计助手。

当你准备好创建或更新工作流时，在回复中包含以下 JSON 块：

\`\`\`flow-definition
{
  "name": "工作流名称",
  "nodes": [...],
  "edges": [...]
}
\`\`\`

节点类型和配置规范：
- trigger_schedule: {"cron": "0 9 * * *"}
- trigger_manual: {}
- trigger_webhook: {}
- trigger_file: {"path": "/path/to/watch", "pattern": "*.xlsx"}
- tool: {"tool_name": "gmail_read", "params": {"query": "from:supplier"}}
- condition: {"expression": "{{node_1.result.count}} > 0", "branches": ["yes", "no"]}
- llm: {"prompt": "总结以下内容：{{node_1.result}}", "output_format": "text"}
- agent: {"goal": "目标描述", "tools": ["http_request", "json_parse"]}
- approval: {"title": "确认操作", "message": "即将发送邮件至 {{to}}"}

节点 position 可以省略，系统会自动布局。
每个节点必须有唯一 id（使用 node_1, node_2...）。`
```

### `internal/service/flow_parser.go`

```go
package service

import (
    "encoding/json"
    "regexp"
    "strings"
)

var flowDefRegex = regexp.MustCompile("(?s)```flow-definition\n(.*?)\n```")

// ExtractFlowDefinition 从 AI 回复中提取 flow-definition 代码块
func ExtractFlowDefinition(content string) (json.RawMessage, bool) {
    m := flowDefRegex.FindStringSubmatch(content)
    if m == nil { return nil, false }
    raw := []byte(strings.TrimSpace(m[1]))
    if !json.Valid(raw) { return nil, false }
    return json.RawMessage(raw), true
}

// AutoLayout 为没有 position 的节点自动生成布局
func AutoLayout(def json.RawMessage) json.RawMessage {
    var d map[string]any
    json.Unmarshal(def, &d)
    nodes, _ := d["nodes"].([]any)
    for i, n := range nodes {
        node := n.(map[string]any)
        if _, ok := node["position"]; !ok {
            node["position"] = map[string]any{"x": float64(i * 200), "y": 100}
        }
        nodes[i] = node
    }
    d["nodes"] = nodes
    result, _ := json.Marshal(d)
    return result
}
```

### ChatService 扩展

```go
// service/chat.go — AI 回复处理后检查是否有工作流定义
func (s *ChatService) OnAssistantMessage(ctx context.Context, convID, content string) error {
    def, ok := ExtractFlowDefinition(content)
    if !ok { return nil }

    def = AutoLayout(def)
    var flowDef struct{ Name string `json:"name"` }
    json.Unmarshal(def, &flowDef)

    // 查看当前对话是否已绑定工作流
    conv, _ := s.convSvc.Get(convID)
    var wfID string

    if conv.AssetID != "" && conv.AssetType == "workflow" {
        // 更新已有工作流
        wfID = conv.AssetID
        s.workflowSvc.UpdateDefinition(wfID, def)
    } else {
        // 创建新工作流并绑定
        wf, err := s.workflowSvc.Create(flowDef.Name)
        if err != nil { return err }
        wfID = wf.ID
        s.workflowSvc.UpdateDefinition(wfID, def)
        s.convSvc.Bind(convID, wfID, "workflow")
    }

    s.bridge.Emit(events.CanvasUpdated, map[string]any{"workflowId": wfID})
    return nil
}
```

### 工作流状态注入

```go
// service/chat.go — 在每次用户发消息前调用
func (s *ChatService) BuildContextInjection(convID string) string {
    conv, _ := s.convSvc.Get(convID)
    if conv.AssetID == "" || conv.AssetType != "workflow" { return "" }

    wf, _ := s.workflowSvc.Get(conv.AssetID)
    if wf == nil { return "" }

    var def struct {
        Nodes []struct {
            ID   string `json:"id"`
            Type string `json:"type"`
        } `json:"nodes"`
        Edges []any `json:"edges"`
    }
    json.Unmarshal(wf.Definition, &def)

    nodeNames := make([]string, len(def.Nodes))
    for i, n := range def.Nodes { nodeNames[i] = n.Type + "(" + n.ID + ")" }

    return "[当前工作流状态]\n" +
        "名称：" + wf.Name + "\n" +
        "状态：" + wf.Status + "\n" +
        "节点：" + strings.Join(nodeNames, " → ") + "\n" +
        "共 " + strconv.Itoa(len(def.Nodes)) + " 个节点，" + strconv.Itoa(len(def.Edges)) + " 条连线"
}
```

---

## 3. 验收测试

```
1. 对话说"做一个每天读 Gmail 发邮件的工作流"
   → AI 输出 flow-definition → 画布出现节点 → 对话绑定到工作流
2. 继续说"再加一个人工确认节点"
   → AI 输出更新后的完整 flow-definition → 画布更新，新节点出现
3. 用户拖动节点位置后发消息
   → AI 的回复中提到"我看到你移动了节点"（通过状态注入感知）
4. 对话绑定到工作流后，左半区画布出现，指示条显示工作流名
```
