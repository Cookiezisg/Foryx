# G3 · 事件触发 — 技术设计文档

**切片**：G3  
**状态**：待 Review

---

## 1. 依赖

```
github.com/fsnotify/fsnotify   # 文件系统监听
```

---

## 2. 目录结构

```
internal/
└── triggers/
    ├── manager.go       # EventTriggerManager：注册/取消文件和 Webhook 触发
    ├── file_watcher.go  # 文件监听
    └── webhook.go       # HTTP Webhook 服务
```

---

## 3. EventTriggerManager

```go
// triggers/manager.go
type EventTriggerManager struct {
    watchers   map[string]*fsnotify.Watcher  // workflowID → watcher
    webhooks   map[string]*http.Server        // workflowID → server
    runner     *runner.Runner
    mu         sync.Mutex
}

func (m *EventTriggerManager) RegisterWorkflow(wf *service.Workflow) {
    m.mu.Lock()
    defer m.mu.Unlock()

    var fd FlowDefinition
    json.Unmarshal(wf.Definition, &fd)

    for _, node := range fd.Nodes {
        switch node.Type {
        case "trigger_file":
            m.registerFileWatcher(wf.ID, node.Config)
        case "trigger_webhook":
            m.registerWebhook(wf.ID, node.Config)
        }
    }
}

func (m *EventTriggerManager) DeregisterWorkflow(workflowID string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    if w, ok := m.watchers[workflowID]; ok {
        w.Close()
        delete(m.watchers, workflowID)
    }
    if s, ok := m.webhooks[workflowID]; ok {
        s.Shutdown(context.Background())
        delete(m.webhooks, workflowID)
    }
}
```

---

## 4. 文件监听

```go
// triggers/file_watcher.go
func (m *EventTriggerManager) registerFileWatcher(workflowID string, config map[string]any) {
    path := config["path"].(string)
    pattern := config["pattern"].(string)
    eventType, _ := config["event"].(string)
    if eventType == "" { eventType = "created" }

    watcher, _ := fsnotify.NewWatcher()
    watcher.Add(path)
    m.watchers[workflowID] = watcher

    debounce := map[string]time.Time{}

    go func() {
        for {
            select {
            case event, ok := <-watcher.Events:
                if !ok { return }
                if !matchEvent(event.Op, eventType) { continue }
                if !matchGlob(filepath.Base(event.Name), pattern) { continue }

                // 防抖：同一文件 5 秒内只触发一次
                if last, ok := debounce[event.Name]; ok && time.Since(last) < 5*time.Second { continue }
                debounce[event.Name] = time.Now()

                m.runner.Run(context.Background(), workflowID, map[string]any{
                    "trigger_input": map[string]any{"file_path": event.Name},
                })
            case <-watcher.Errors:
                return
            }
        }
    }()
}

func matchEvent(op fsnotify.Op, eventType string) bool {
    switch eventType {
    case "created":  return op.Has(fsnotify.Create)
    case "modified": return op.Has(fsnotify.Write)
    case "deleted":  return op.Has(fsnotify.Remove)
    }
    return false
}
```

---

## 5. Webhook 监听

```go
// triggers/webhook.go
func (m *EventTriggerManager) registerWebhook(workflowID string, config map[string]any) {
    port, _ := config["port"].(float64)
    if port == 0 { port = 8080 }
    urlPath, _ := config["path"].(string)
    secret, _ := config["secret"].(string)

    mux := http.NewServeMux()
    mux.HandleFunc(urlPath, func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            w.WriteHeader(http.StatusMethodNotAllowed)
            return
        }

        if secret != "" && r.Header.Get("X-Webhook-Secret") != secret {
            w.WriteHeader(http.StatusUnauthorized)
            return
        }

        var body map[string]any
        json.NewDecoder(r.Body).Decode(&body)

        runID, err := m.runner.Run(context.Background(), workflowID, map[string]any{
            "trigger_input": map[string]any{"body": body},
        })
        if err != nil {
            w.WriteHeader(http.StatusInternalServerError)
            return
        }

        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]any{"status": "accepted", "runId": runID})
    })

    srv := &http.Server{Addr: fmt.Sprintf(":%d", int(port)), Handler: mux}
    m.webhooks[workflowID] = srv
    go srv.ListenAndServe()
}
```

---

## 6. 验收测试

```
1. RegisterWorkflow() with trigger_file → watcher 启动 → 文件创建触发 runner.Run()
2. 防抖：5 秒内同文件事件只触发一次
3. RegisterWorkflow() with trigger_webhook → HTTP server 启动 → POST 触发 runner.Run()
4. Webhook secret 不匹配 → 返回 401
5. DeregisterWorkflow() → watcher 关闭，HTTP server 关闭
6. pattern 不匹配的文件 → 不触发
```
