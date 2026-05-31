# B2 · 流式对话核心 — 技术设计文档

**切片**：B2  
**状态**：待 Review

---

## 1. 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| LLM 编排 | Eino `ChatModel` 抽象 | 统一接口，换提供商只改适配器 |
| 流式传输 | Eino StreamReader → goroutine → EventBridge | 不开 HTTP 端口 |
| Markdown 渲染 | `react-markdown` + `rehype-highlight` | 成熟，支持代码高亮 |
| 停止机制 | Go `context.CancelFunc` | 标准 Go 取消模式 |

---

## 2. 目录结构

```
internal/
├── model/
│   ├── gateway.go        # ModelGateway：统一入口，选择提供商
│   ├── anthropic.go      # Anthropic 适配器（Eino ChatModel）
│   └── openai.go         # OpenAI 适配器（含 OpenAI 兼容端点）
└── service/
    └── chat.go           # ChatService：处理单次对话请求

frontend/src/
├── hooks/
│   └── useStreaming.ts   # 流式消息接收 hook
└── components/chat/
    ├── MessageList.tsx   # 消息列表
    ├── MessageItem.tsx   # 单条消息（含 Markdown 渲染）
    └── ChatInput.tsx     # 输入框
```

---

## 3. Go 层

### 3.1 `internal/model/gateway.go`

```go
package model

import (
    "context"
    "forgify/internal/service"

    "github.com/cloudwego/eino/components/model"
    "github.com/cloudwego/eino/schema"
)

// ModelGateway 根据配置返回正确的 Eino ChatModel
type ModelGateway struct{}

func (g *ModelGateway) GetModel(purpose string) (model.ChatModel, error) {
    // purpose: "conversation" | "codegen" | "cheap"
    // 从 B3 的 ModelSelector 获取配置（B3 完成后接入）
    // 本切片先硬编码返回第一个可用 Key 对应的模型
    keys, _ := service.ListAPIKeys()
    if len(keys) == 0 {
        return nil, ErrNoAPIKey
    }
    key := keys[0]
    switch key.Provider {
    case "anthropic":
        return NewAnthropicModel(key)
    case "openai", "openai_compat":
        return NewOpenAIModel(key)
    default:
        return nil, ErrUnsupportedProvider
    }
}
```

### 3.2 `internal/model/anthropic.go`

```go
package model

import (
    "forgify/internal/service"
    anthropic "github.com/cloudwego/eino-ext/components/model/anthropic"
)

func NewAnthropicModel(key *service.APIKey) (*anthropic.ChatModel, error) {
    rawKey, _ := service.GetRawKey(key.ID)
    return anthropic.NewChatModel(context.Background(), &anthropic.ChatModelConfig{
        APIKey: rawKey,
        Model:  "claude-sonnet-4-6",
    })
}
```

### 3.3 `internal/service/chat.go`

```go
package service

import (
    "context"
    "forgify/internal/events"
    "forgify/internal/model"

    "github.com/cloudwego/eino/schema"
)

type ChatService struct {
    gateway *model.ModelGateway
    bridge  *events.Bridge
    cancels map[string]context.CancelFunc  // conversationID → cancel
}

func (s *ChatService) SendMessage(conversationID, userMessage string) error {
    m, err := s.gateway.GetModel("conversation")
    if err != nil {
        return err
    }

    ctx, cancel := context.WithCancel(context.Background())
    s.cancels[conversationID] = cancel
    defer delete(s.cancels, conversationID)

    messages := []*schema.Message{
        schema.UserMessage(userMessage),
    }

    // Eino 流式调用
    stream, err := m.Stream(ctx, messages)
    if err != nil {
        s.bridge.Emit(events.ChatError, events.ChatErrorPayload{
            ConversationID: conversationID,
            Error:          classifyError(err),
        })
        return err
    }

    // goroutine 消费 stream，emit token 事件
    go func() {
        for {
            chunk, err := stream.Recv()
            if err != nil {
                s.bridge.Emit(events.ChatDone, events.ChatDonePayload{
                    ConversationID: conversationID,
                })
                return
            }
            s.bridge.Emit(events.ChatToken, events.ChatTokenPayload{
                ConversationID: conversationID,
                Token:          chunk.Content,
                Done:           false,
            })
        }
    }()
    return nil
}

func (s *ChatService) StopGeneration(conversationID string) {
    if cancel, ok := s.cancels[conversationID]; ok {
        cancel()
    }
}

func classifyError(err error) string {
    msg := err.Error()
    switch {
    case contains(msg, "401"), contains(msg, "invalid api key"):
        return "API Key 可能已失效，请前往设置检查"
    case contains(msg, "429"), contains(msg, "rate limit"):
        return "请求过于频繁，请稍后重试"
    case contains(msg, "timeout"), contains(msg, "deadline"):
        return "连接超时，请检查网络后重试"
    default:
        return "AI 服务暂时不可用，请稍后重试"
    }
}
```

### 3.4 HTTP API 路由

```go
// backend/internal/server/routes.go
mux.HandleFunc("POST /api/chat/send", s.sendMessage)
mux.HandleFunc("POST /api/chat/stop", s.stopGeneration)
```

---

## 4. 前端层

### 4.1 `hooks/useStreaming.ts`

```typescript
import { useState, useEffect, useCallback } from 'react'
import { onEvent, EV } from '@/lib/events'
import { getBackendPort } from '@/lib/backend'

interface Message {
    id: string
    role: 'user' | 'assistant'
    content: string
    status: 'done' | 'streaming' | 'error'
}

export function useStreaming(conversationId: string) {
    const [messages, setMessages] = useState<Message[]>([])
    const [isStreaming, setIsStreaming] = useState(false)

    useEffect(() => {
        const offs = [
            onEvent(EV.ChatToken, (e) => {
                if (e.conversationId !== conversationId) return
                setIsStreaming(true)
                setMessages(prev => appendToken(prev, e.token))
            }),
            onEvent(EV.ChatDone, (e) => {
                if (e.conversationId !== conversationId) return
                setIsStreaming(false)
                setMessages(prev => finalizeLastMessage(prev))
            }),
            onEvent(EV.ChatError, (e) => {
                if (e.conversationId !== conversationId) return
                setIsStreaming(false)
                setMessages(prev => setLastMessageError(prev, e.error))
            }),
        ]
        return () => offs.forEach(off => off())
    }, [conversationId])

    const sendMessage = useCallback(async (text: string) => {
        const userMsg: Message = {
            id: crypto.randomUUID(),
            role: 'user', content: text, status: 'done'
        }
        const assistantMsg: Message = {
            id: crypto.randomUUID(),
            role: 'assistant', content: '', status: 'streaming'
        }
        setMessages(prev => [...prev, userMsg, assistantMsg])
        await fetch(`http://127.0.0.1:${getBackendPort()}/api/chat/send`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ conversationId, message: text }),
        })
    }, [conversationId])

    const stopGeneration = useCallback(() => {
        fetch(`http://127.0.0.1:${getBackendPort()}/api/chat/stop`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ conversationId }),
        })
    }, [conversationId])

    return { messages, isStreaming, sendMessage, stopGeneration }
}

// 工具函数
function appendToken(messages: Message[], token: string): Message[] {
    return messages.map((m, i) =>
        i === messages.length - 1 && m.role === 'assistant'
            ? { ...m, content: m.content + token }
            : m
    )
}
function finalizeLastMessage(messages: Message[]): Message[] {
    return messages.map((m, i) =>
        i === messages.length - 1 ? { ...m, status: 'done' } : m
    )
}
function setLastMessageError(messages: Message[], error: string): Message[] {
    return messages.map((m, i) =>
        i === messages.length - 1 ? { ...m, content: error, status: 'error' } : m
    )
}
```

### 4.2 `components/chat/MessageItem.tsx`

```tsx
import ReactMarkdown from 'react-markdown'
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter'
import { oneDark } from 'react-syntax-highlighter/dist/esm/styles/prism'

export function MessageItem({ message }: { message: Message }) {
    return (
        <div className={`flex gap-3 ${message.role === 'user' ? 'justify-end' : 'justify-start'}`}>
            <div className={`max-w-[80%] rounded-xl px-4 py-3 text-sm ${
                message.role === 'user'
                    ? 'bg-blue-600 text-white'
                    : 'bg-neutral-800 text-neutral-100'
            }`}>
                {message.status === 'streaming' && message.content === '' ? (
                    <span className="text-neutral-400 italic">AI 正在思考...</span>
                ) : (
                    <ReactMarkdown
                        components={{
                            code({ className, children }) {
                                const lang = /language-(\w+)/.exec(className || '')?.[1]
                                return lang ? (
                                    <SyntaxHighlighter style={oneDark} language={lang}>
                                        {String(children)}
                                    </SyntaxHighlighter>
                                ) : <code className="bg-neutral-700 px-1 rounded">{children}</code>
                            }
                        }}
                    >
                        {message.content}
                    </ReactMarkdown>
                )}
                {message.status === 'streaming' && (
                    <span className="inline-block w-0.5 h-4 bg-neutral-100 ml-0.5 animate-pulse" />
                )}
            </div>
        </div>
    )
}
```

---

## 5. 新增 npm 依赖

```json
"react-markdown": "^9",
"rehype-highlight": "^7",
"react-syntax-highlighter": "^15",
"@types/react-syntax-highlighter": "^15"
```

---

## 6. 验收测试

```
1. 发送"你好"→ AI 流式回复，逐字出现，光标跟随
2. 生成中点"停止"→ 立即中断，已有内容保留
3. 断开网络发消息 → 显示"连接超时"错误
4. 删除 API Key 后发消息 → 显示 Key 失效错误
5. 发送含代码的问题 → 回复中代码块有语法高亮
6. 快速连续发两条消息 → 第一条完成后才能发第二条（或队列处理）
7. 长文本回复（>1000字）→ 正常流式显示，不卡顿
```
