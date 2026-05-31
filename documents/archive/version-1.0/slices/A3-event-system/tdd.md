# A3 · 事件系统 — 技术设计文档

**切片**：A3  
**状态**：待 Review

---

## 1. 架构

事件流方向：Go 业务层 → SSEBroker → 前端 EventSource

```
Go: srv.Events.Emit("chat.token", payload)
         ↓
    SSEBroker.publish() → 广播给所有 SSE 客户端
         ↓
    HTTP response: "event: chat.token\ndata: {...}\n\n"
         ↓
前端: EventSource.addEventListener("chat.token", handler)
```

---

## 2. 目录结构

```
backend/internal/
├── events/
│   └── events.go       # 事件名常量 + Bridge struct
└── server/
    ├── server.go        # HTTP server + SSEBroker 组装
    └── sse.go           # SSEBroker + GET /events handler

frontend/src/
└── lib/
    └── events.ts        # EventSource 封装 + onEvent()
```

---

## 3. Go 层

### events.go — 常量 + Bridge

```go
// 事件名常量
const (
    ChatToken     = "chat.token"
    ChatDone      = "chat.done"
    MailboxUpdated = "mailbox.updated"
    // ...
)

// Bridge 让业务层通过 SSEBroker 发事件
type Bridge struct { publish func(string, any) }

func (b *Bridge) Emit(event string, payload any) {
    b.publish(event, payload)
}
```

### server/sse.go — SSEBroker

```go
func (b *SSEBroker) handleSSE(w http.ResponseWriter, r *http.Request) {
    // 设置 SSE headers，注册 channel
    // 循环等待 channel 消息或 context 取消
    // 每条消息格式: "event: {name}\ndata: {json}\n\n"
}
```

CORS 在 `Server.ServeHTTP()` 统一设置 `Access-Control-Allow-Origin: *`，覆盖所有路由包括 SSE。

---

## 4. 前端层

### lib/events.ts — EventSource 封装

```typescript
let source: EventSource | null = null

// 在 main.tsx 里读 URL ?port= 后同步调用
export function initBackend(port: number): void {
    if (source) source.close()
    source = new EventSource(`http://127.0.0.1:${port}/events`)
}

export function onEvent<T>(name: string, handler: (payload: T) => void): () => void {
    if (!source) return () => {}
    const listener = (e: MessageEvent) => {
        try { handler(JSON.parse(e.data)) } catch {}
    }
    source.addEventListener(name, listener as EventListener)
    return () => source?.removeEventListener(name, listener as EventListener)
}
```

### main.tsx — 同步初始化

```typescript
// 在 ReactDOM.render 之前调用，保证组件 mount 时 EventSource 已就绪
const port = parseInt(new URLSearchParams(window.location.search).get('port') ?? '0')
if (port > 0) initBackend(port)
```

### 使用示例

```typescript
useEffect(() => {
    return onEvent<{ count: number }>(EventNames.MailboxUpdated, ({ count }) => {
        setUnreadCount(count)
    })
}, [])
```

---

## 5. 验收测试

```
1. Go emit MailboxUpdated → 前端 InboxContext 收到，unreadCount 更新
2. 多个 listener 订阅同一事件，都收到
3. 调用返回的 off() 后，不再收到事件
4. 后端未启动时（port=0）：onEvent 返回 no-op，不报错
5. SSE 连接断开后，EventSource 自动重连（浏览器原生行为）
6. CORS：从 http://localhost:5173 发起的 EventSource 连接正常（Go 返回 Access-Control-Allow-Origin: *）
```
