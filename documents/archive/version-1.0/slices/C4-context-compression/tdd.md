# C4 · 上下文压缩 — 技术设计文档

**切片**：C4  
**状态**：待 Review

---

## 1. 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| Token 估算 | 字符数 / 3.5（近似）| 避免引入 tokenizer 依赖，误差在 10% 以内，够用 |
| AutoCompact 模型 | PurposeCheap（Haiku / Flash）| 摘要任务不需要强模型，节省成本 |
| FullCompact 模型 | PurposeConversation（主模型）| 全量摘要质量要求更高 |
| 压缩状态存储 | messages 表 content_type='summary' | 摘要消息和普通消息同表，加载时统一处理 |

---

## 2. 目录结构

```
internal/
└── context/
    ├── compressor.go     # 三层压缩逻辑
    └── estimator.go      # Token 用量估算

frontend/src/
└── components/chat/
    ├── CompactBanner.tsx  # AutoCompact 后的顶部提示条
    └── ChatInput.tsx      # 加入"压缩对话"按钮（FullCompact 触发）
```

---

## 3. Go 层

### `internal/context/estimator.go`

```go
package context

import "github.com/cloudwego/eino/schema"

// EstimateTokens 估算消息列表的 token 用量（字符数 / 3.5）
func EstimateTokens(messages []*schema.Message) int {
    total := 0
    for _, m := range messages {
        total += len([]rune(m.Content))
        // 多模态内容：图片估算 1000 token
        total += len(m.MultiContent) * 1000
    }
    return total * 10 / 35 // ≈ /3.5
}
```

### `internal/context/compressor.go`

```go
package context

import (
    "context"
    "fmt"
    "strings"

    "forgify/internal/model"
    "forgify/internal/service"

    "github.com/cloudwego/eino/schema"
)

const (
    thresholdMicro = 0.75
    thresholdAuto  = 0.88
)

type Compressor struct {
    gateway    *model.ModelGateway
    messageSvc *service.MessageService
}

// MaybeCompress 在每次 LLM 调用前检查并按需压缩
// 返回压缩后的消息列表和压缩级别（"", "micro", "auto"）
func (c *Compressor) MaybeCompress(
    ctx context.Context,
    convID string,
    messages []*schema.Message,
    modelLimit int,
) ([]*schema.Message, string, error) {
    usage := EstimateTokens(messages)
    ratio := float64(usage) / float64(modelLimit)

    switch {
    case ratio >= thresholdAuto:
        compressed, err := c.autoCompact(ctx, convID, messages)
        return compressed, "auto", err
    case ratio >= thresholdMicro:
        return c.microCompact(messages), "micro", nil
    default:
        return messages, "", nil
    }
}

// microCompact 纯代码裁剪，不调用 LLM
func (c *Compressor) microCompact(messages []*schema.Message) []*schema.Message {
    result := make([]*schema.Message, 0, len(messages))
    seenCanvasUpdate := false

    for i := len(messages) - 1; i >= 0; i-- {
        m := messages[i]

        // 只保留最新一条画布状态注入
        if strings.HasPrefix(m.Content, "[当前工作流状态") {
            if seenCanvasUpdate {
                continue // 跳过旧的画布状态
            }
            seenCanvasUpdate = true
        }

        // 截断超长消息
        runes := []rune(m.Content)
        if len(runes) > 4000 {
            m = &schema.Message{
                Role:    m.Role,
                Content: string(runes[:2000]) + "\n...[内容过长已截断]...\n" + string(runes[len(runes)-500:]),
            }
        }

        result = append(result, m)
    }

    // 反转回正序
    for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
        result[i], result[j] = result[j], result[i]
    }
    return result
}

// autoCompact 用便宜模型摘要早期历史，保留最近 20 条
func (c *Compressor) autoCompact(
    ctx context.Context, convID string, messages []*schema.Message,
) ([]*schema.Message, error) {
    const keepRecent = 20
    if len(messages) <= keepRecent {
        return messages, nil
    }

    toSummarize := messages[:len(messages)-keepRecent]
    recent := messages[len(messages)-keepRecent:]

    // 构造摘要 prompt
    var sb strings.Builder
    for _, m := range toSummarize {
        sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", m.Role, m.Content))
    }

    m, _, err := c.gateway.GetModel(ctx, model.PurposeCheap)
    if err != nil { return messages, err } // 降级：不压缩，返回原始

    resp, err := m.Generate(ctx, []*schema.Message{
        schema.SystemMessage(`你是一个对话摘要助手。请对以下对话历史做精炼摘要，必须包含：
1. 用户的核心目标
2. 已做出的关键决策
3. 当前进展状态
4. 重要约束条件

用"[对话摘要]"开头，500字以内。`),
        schema.UserMessage(sb.String()),
    })
    if err != nil { return messages, err }

    summaryMsg := &schema.Message{
        Role:    schema.System,
        Content: resp.Content,
    }

    // 保存摘要消息到 DB（content_type='summary'）
    c.messageSvc.SaveSummary(convID, resp.Content)

    return append([]*schema.Message{summaryMsg}, recent...), nil
}

// FullCompact 用户手动触发，完整摘要整个对话
func (c *Compressor) FullCompact(ctx context.Context, convID string) error {
    msgs, err := c.messageSvc.List(convID)
    if err != nil { return err }

    var sb strings.Builder
    for _, m := range msgs {
        sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", m.Role, m.Content))
    }

    llm, _, err := c.gateway.GetModel(ctx, model.PurposeConversation)
    if err != nil { return err }

    resp, err := llm.Generate(ctx, []*schema.Message{
        schema.SystemMessage(`请对以下完整对话做全面结构化摘要，包含：
- 用户目标
- 所有关键决策（含时间顺序）
- 已创建的工具和工作流
- 当前状态和下一步

用"[完整对话摘要]"开头，1000字以内。`),
        schema.UserMessage(sb.String()),
    })
    if err != nil { return err }

    // 清空所有消息，插入摘要
    return c.messageSvc.ReplaceWithSummary(convID, resp.Content)
}
```

---

## 4. MessageService 扩展

```go
// service/message.go — 新增方法
func (s *MessageService) SaveSummary(convID, content string) error {
    return s.Save(&Message{
        ID: uuid.NewString(), ConversationID: convID,
        Role: "system", Content: content, ContentType: "summary",
    })
}

func (s *MessageService) ReplaceWithSummary(convID, content string) error {
    tx, _ := storage.DB().Begin()
    tx.Exec("DELETE FROM messages WHERE conversation_id=?", convID)
    tx.Exec(`INSERT INTO messages (id, conversation_id, role, content, content_type)
             VALUES (?, ?, 'system', ?, 'summary')`,
        uuid.NewString(), convID, content)
    return tx.Commit()
}
```

---

## 5. HTTP API 路由

```go
// backend/internal/server/routes.go
mux.HandleFunc("POST /api/conversations/{id}/compact", s.fullCompact)
```

---

## 6. 前端组件

### `CompactBanner.tsx`

```tsx
// 监听 chat.compacted 事件，显示顶部提示条
export function CompactBanner({ conversationId }: { conversationId: string }) {
    const [level, setLevel] = useState<string | null>(null)
    const [summaryVisible, setSummaryVisible] = useState(false)

    useEffect(() => {
        return onEvent(EV.ChatCompacted, (e) => {
            if (e.conversationId === conversationId) setLevel(e.level)
        })
    }, [conversationId])

    if (!level || level === 'micro') return null

    return (
        <div className="px-4 py-2 bg-neutral-800/80 border-b border-neutral-700
                        text-xs text-neutral-400 flex items-center justify-between">
            <span>部分早期消息已压缩以节省上下文</span>
            <button onClick={() => setSummaryVisible(true)}
                    className="text-blue-400 hover:underline">
                查看摘要
            </button>
            {summaryVisible && <SummaryModal onClose={() => setSummaryVisible(false)} />}
        </div>
    )
}
```

### 输入框工具栏加"压缩对话"按钮

```tsx
// ChatInput.tsx 里加一个工具图标
<button title="压缩对话历史" onClick={() => {
    if (confirm('压缩后原始历史不可恢复，继续？')) {
        FullCompact(conversationId)
    }
}}>
    <Minimize2 size={16} />
</button>
```

---

## 7. 验收测试

```
1. 构造 100 条长消息，MicroCompact 触发后对话仍可继续
2. AutoCompact 触发 → 顶部 banner 出现 → 点"查看摘要"显示内容
3. AutoCompact 后发"我之前的目标是什么？" → AI 能正确引用摘要中的目标
4. FullCompact 触发 → 确认框 → 历史消息变为单条摘要
5. FullCompact 后 AI 仍能基于摘要正常对话
6. 压缩后重启 Forgify，对话状态与压缩后一致（持久化正确）
```
