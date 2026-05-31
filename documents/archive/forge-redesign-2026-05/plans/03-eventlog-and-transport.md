# Eventlog Scope Generalization + HTTP/2 Transport Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> ⚠️ **本 Plan 已大幅修订(2026-05-12)**,**下面的原方案不再执行**。新方向见
> [`../discussions/2026-05-12-env-and-sse-rework.md`](../discussions/2026-05-12-env-and-sse-rework.md)。
> 关键变化:
> - **Phase 5 (TLS + HTTP/2 + mkcert) 永久搁置** — Wails 桌面端用 native events 绕开 HTTP,TLS 不需要
> - **Phase 1-4 (eventlog scope) 重设计** — 不再走 per-entity Scope,改为 **3 条 user_id 流**(chat eventlog / notifications / forge)— 前端 client-side demux
> - 顺便回头改 Plan 01/02 的 env 模型:同步在 LLM tool 内装环境 + 内部 retry loop
> - 实际 commit 切分见 discussion 文档 §G

---

## (历史) 原方案 — 不再执行 ↓

**Goal:** 两个相关基础设施改造,为后续 Workflow domain 准备件:

1. **Eventlog scope 泛化(D19)**:`conversationId` 字段升为 `Scope{Kind, ID}`,加 `function:` / `handler:` / `workflow:` / `flowrun:` 4 种 entity-level scope 类型;Bridge 加 multi-scope subscribe;HTTP `?scope=<kind>:<id>` 多参 + backward-compat `?conversationId=`
2. **HTTP/2 + TLS Transport(D18)**:Backend 默认 ListenAndServeTLS 自动 h2;mkcert auto-setup bootstrap;**彻底解 HTTP/1.1 6-connection 限制**

**前置依赖**:Plan 01 + Plan 02 已 merge(Function + Handler 已就位,但还没消费 entity scope;workflow / flowrun 之后才用)。

**Architecture:** 两块改动相对独立,但都为 Workflow / FlowRun 流式打基础。Eventlog 改动是协议层 + Bridge / HTTP layer / Emitter;Transport 是 main.go + cmd/resources 工具链。

**Tech Stack:** Go `net/http`(原生 HTTP/2 over TLS),mkcert(本地自签),`crypto/tls`,现有 `domain/eventlog` + `infra/eventlog`。

**关联**:[`05-execution-plane.md`](../05-execution-plane.md) §4 / §12 / [`07-notifications-and-eventlog.md`](../07-notifications-and-eventlog.md) §2 / §4。

---

## Phase 0:Branch Setup

### Task 1:Branch + 验证 prereq

- [ ] **Step 1: 验证 main 包含 Plan 01+02**

```bash
cd /Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify
git checkout main && git pull origin main
ls backend/internal/domain/function backend/internal/domain/handler
# 两个目录都应存在
```

- [ ] **Step 2: 创建分支**

```bash
git checkout -b feature/eventlog-and-transport
```

---

## Phase 1:Eventlog Domain — Scope Struct(D19)

### Task 2:加 Scope struct,deprecate 裸 conversationId

**Files:**
- Modify: `backend/internal/domain/eventlog/eventlog.go`(加 Scope struct)
- Create: `backend/internal/domain/eventlog/scope.go`

- [ ] **Step 1: 写 scope.go**

```go
package eventlog

import "fmt"

// Scope identifies the recipient of an event stream. 5 known kinds in V1:
// conversation, flowrun, function, handler, workflow.
//
// Scope 标识事件流的接收者。V1 已知 5 种 kind:
// conversation / flowrun / function / handler / workflow。
type Scope struct {
	Kind string `json:"kind"` // "conversation" | "flowrun" | "function" | "handler" | "workflow"
	ID   string `json:"id"`   // entity ID(对应 kind)
}

// Known kinds.
const (
	KindConversation = "conversation"
	KindFlowRun      = "flowrun"
	KindFunction     = "function"
	KindHandler      = "handler"
	KindWorkflow     = "workflow"
)

// String 返 "<kind>:<id>" 形式(用作 Bridge map key + HTTP 协议)。
//
// String returns "<kind>:<id>" form (Bridge map key + HTTP wire format).
func (s Scope) String() string {
	return s.Kind + ":" + s.ID
}

// ParseScope 解析 "<kind>:<id>" 字符串。
//
// ParseScope parses a "<kind>:<id>" string into a Scope.
func ParseScope(raw string) (Scope, error) {
	for i, c := range raw {
		if c == ':' {
			return Scope{Kind: raw[:i], ID: raw[i+1:]}, nil
		}
	}
	return Scope{}, fmt.Errorf("eventlog.ParseScope: missing ':' in %q", raw)
}

// IsValidKind 校验 kind 在 V1 白名单内。
//
// IsValidKind validates kind against V1 whitelist.
func IsValidKind(kind string) bool {
	switch kind {
	case KindConversation, KindFlowRun, KindFunction, KindHandler, KindWorkflow:
		return true
	}
	return false
}
```

- [ ] **Step 2: 单测**

```go
// scope_test.go
func TestScopeRoundTrip(t *testing.T) {
	s := Scope{Kind: "function", ID: "fn_abc"}
	parsed, err := ParseScope(s.String())
	if err != nil { t.Fatal(err) }
	if parsed != s { t.Errorf("got %+v, want %+v", parsed, s) }
}
```

- [ ] **Step 3: 编译 + commit**

```bash
git add backend/internal/domain/eventlog/scope.go backend/internal/domain/eventlog/scope_test.go
git commit -m "feat(eventlog): add Scope struct + 5 V1 kinds (D19)"
git push origin feature/eventlog-and-transport
```

---

### Task 3:Event 接口 / 现有 ChatMessage 等 events 加 Scope 字段

**Files:**
- Modify: `backend/internal/domain/eventlog/eventlog.go`
- Modify: 现有 event types(MessageStart / MessageStop / BlockStart / BlockDelta / BlockStop)

现状每个 event 都有 `ConversationID string`。改为 `Scope eventlog.Scope`;backward compat:HTTP wire format 仍接受 `conversationId` 字段(server-side 转 Scope{Kind:"conversation", ID:cid})。

- [ ] **Step 1: 改 Event types 加 Scope 字段**

```go
// 例:MessageStart event
type MessageStart struct {
	Scope          Scope                  `json:"scope"`
	ConversationID string                 `json:"conversationId,omitempty"` // deprecated, kept for compat
	ID             string                 `json:"id"`
	ParentBlockID  string                 `json:"parentBlockId,omitempty"`
	Role           string                 `json:"role"`
	Attrs          map[string]interface{} `json:"attrs,omitempty"`
}

// MarshalJSON 写时:Scope 必出,conversationId 仅当 Kind="conversation" 时也输出兼容
// (transport 层 wire 兼容,frontend 不需要立刻切)
```

类似改 5 个 event 类型(MessageStart / MessageStop / BlockStart / BlockDelta / BlockStop)。

- [ ] **Step 2: 单测**:验证 JSON 序列化字段 + Scope round-trip

- [ ] **Step 3: Commit**

```bash
git add backend/internal/domain/eventlog/
git commit -m "feat(eventlog): event types carry Scope field (deprecate ConversationID, kept for compat)"
git push
```

---

## Phase 2:Infra Layer — Bridge Multi-scope

### Task 4:Bridge 改 map key from string to Scope

**Files:**
- Modify: `backend/internal/infra/eventlog/bridge.go`

- [ ] **Step 1: 改 Bridge 内部 map**

```go
type Bridge struct {
	mu          sync.RWMutex
	subscribers map[eventlogdomain.Scope][]chan Event // was: map[string]
	seqs        map[eventlogdomain.Scope]int64        // per-scope monotonic
	buffers     map[eventlogdomain.Scope]*ringBuf     // replay buffer
	log         *zap.Logger
}

func (b *Bridge) Publish(scope eventlogdomain.Scope, ev Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seqs[scope]++
	ev.Seq = b.seqs[scope]
	// ... 写 buffer + 推 subscribers
}

// Subscribe(now multi-scope)— 一个 channel 收多 scope 事件
func (b *Bridge) Subscribe(scopes []eventlogdomain.Scope, opts SubscribeOpts) (<-chan Event, func(), error) {
	ch := make(chan Event, 256)
	cancel := func() { ... }
	// 把 ch 加到所有 scope 的 subscribers 列表
	b.mu.Lock()
	for _, s := range scopes {
		b.subscribers[s] = append(b.subscribers[s], ch)
		// replay if Last-Event-ID > 0
		if seq, ok := opts.LastSeqs[s]; ok && seq > 0 {
			// 从 buffer 重放 seq+1 起的事件
		}
	}
	b.mu.Unlock()
	return ch, cancel, nil
}

type SubscribeOpts struct {
	LastSeqs map[eventlogdomain.Scope]int64 // for Last-Event-ID resume
}
```

- [ ] **Step 2: 单测**(参考现有 bridge_test.go,加 multi-scope 场景)

测试新加:
- `TestSubscribe_MultiScope` — 一个 ch 订 3 scope,publish 到任一都应送达
- `TestSubscribe_LastSeqs_PerScope` — 不同 scope 的 last seq 独立 resume
- `TestPublish_PerScopeMonotonicSeq` — 不同 scope 的 seq 独立增

- [ ] **Step 3: Commit**

```bash
git add backend/internal/infra/eventlog/
git commit -m "feat(eventlog): Bridge multi-scope subscribe + Scope-keyed maps"
git push
```

---

### Task 5:Pkg/eventlog Emitter 加 PublishToScopes(双写支持)

**Files:**
- Modify: `backend/internal/pkg/eventlog/eventlog.go`

per D19:LLM 在 chat 锻造 entity 时双写到 conversation:cv + entity:fn/hd/wf 两个 scope。

- [ ] **Step 1: 加 helper**

```go
// PublishToScopes emits one event to multiple scopes (double-write per D19).
//
// PublishToScopes 把一个事件 emit 到多个 scope(D19 双写)。
func (e *Emitter) PublishToScopes(ctx context.Context, ev Event, scopes ...eventlogdomain.Scope) error {
	for _, s := range scopes {
		ev.Scope = s // override
		if err := e.bridge.Publish(s, ev); err != nil {
			return err
		}
	}
	return nil
}

// 现有 Publish(ctx, ev) 仍工作 — 默认从 ctx 拿单 scope。
```

- [ ] **Step 2: 单测**:加 TestPublishToScopes 验证 fan-out 正确

- [ ] **Step 3: Commit**

---

## Phase 3:HTTP Layer

### Task 6:HTTP handler 解多 `?scope=` query 参 + backward compat

**Files:**
- Modify: `backend/internal/transport/httpapi/handlers/eventlog.go`

- [ ] **Step 1: 改 handler 解析**

```go
func (h *Handler) handleEventlogStream(w http.ResponseWriter, r *http.Request) {
	// 解 ?scope=<kind>:<id> 多次出现
	scopes := []eventlogdomain.Scope{}
	for _, raw := range r.URL.Query()["scope"] {
		s, err := eventlogdomain.ParseScope(raw)
		if err != nil { http.Error(w, err.Error(), 400); return }
		if !eventlogdomain.IsValidKind(s.Kind) {
			http.Error(w, "unknown scope kind", 400); return
		}
		scopes = append(scopes, s)
	}
	// backward compat: ?conversationId=cv_xxx
	if cid := r.URL.Query().Get("conversationId"); cid != "" {
		scopes = append(scopes, eventlogdomain.Scope{Kind: "conversation", ID: cid})
	}
	if len(scopes) == 0 {
		http.Error(w, "must specify at least one ?scope= or ?conversationId=", 400); return
	}

	// Last-Event-ID resume(per-scope SemiColon-separated:"<scope1>=<seq1>;<scope2>=<seq2>")
	lastSeqs := parseLastEventID(r.Header.Get("Last-Event-ID"))

	// Subscribe + stream SSE
	ch, cancel, _ := h.bridge.Subscribe(scopes, infraeventlog.SubscribeOpts{LastSeqs: lastSeqs})
	defer cancel()
	streamSSE(w, r.Context(), ch)
}

// parseLastEventID 解 multi-scope last seqs(扩展 V1 wire format,backward compat 单 seq 也接)。
func parseLastEventID(raw string) map[eventlogdomain.Scope]int64 {
	out := map[eventlogdomain.Scope]int64{}
	if raw == "" { return out }
	// 旧 client 发裸数字(单 conversation scope)→ 视为 conversation scope last seq
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		// 需要从 ?conversationId 拿 cid 拼 scope — handler 内已有 scopes 列表,可在此处 fallback
		return out // V1 简化:旧 client 不支持多 scope resume(向后兼容降级)
	}
	// 新 wire: "<kind>:<id>=<seq>;<kind>:<id>=<seq>"
	for _, part := range strings.Split(raw, ";") {
		// 略 — 实施时按格式 parse
	}
	return out
}
```

- [ ] **Step 2: 写 handler 测试** httptest 覆盖:
  - 单 scope `?conversationId=cv_x`(旧)
  - 单 scope `?scope=conversation:cv_x`(新)
  - 多 scope `?scope=conversation:cv_x&scope=function:fn_y`
  - Bad scope kind reject 400

- [ ] **Step 3: Commit**

---

## Phase 4:Producer 双写 — Function / Handler 锻造时也写 entity scope

**Files:**
- Modify: `backend/internal/app/function/apply.go`(emit progress 时也写 function:fn_x scope)
- Modify: `backend/internal/app/handler/apply.go`(同上,handler:hd_x scope)

per D19 双写策略。

### Task 7:Function apply.go 双写

- [ ] **Step 1: 改 ApplyOps 内 progress emit**

```go
// 之前:
em.DeltaBlock(ctx, progressBlockID, payload)
// 现在(双写):
funcScope := eventlogdomain.Scope{Kind: "function", ID: functionID}
em.PublishToScopes(ctx, deltaEvent, ctxScope, funcScope) // ctxScope 来自 ctx
```

具体:Service 的 ApplyOps 拿到 functionID,内部 emit 时 PublishToScopes 同时写两个 scope。

- [ ] **Step 2: pipeline test 加双写验证**:订 conversation scope + function scope,各收到完整事件流

- [ ] **Step 3: Commit**

### Task 8:Handler apply.go 双写

同 Task 7,handler:hd_x scope。

- [ ] Step 1-3

---

## Phase 5:HTTP/2 Transport(D18)

### Task 9:cmd/server main.go 加 TLS 启动 + flag

**Files:**
- Modify: `backend/cmd/server/main.go`

- [ ] **Step 1: 加 flag + TLS 启动**

```go
var (
	tlsCert  = flag.String("tls-cert", "", "TLS cert path (default ~/.forgify/.tls/cert.pem)")
	tlsKey   = flag.String("tls-key", "", "TLS key path (default ~/.forgify/.tls/key.pem)")
	httpOnly = flag.Bool("http", false, "Force HTTP/1.1 cleartext (disable TLS)")
)

func main() {
	flag.Parse()
	// ... 现有 setup ...

	// resolve cert paths
	if *tlsCert == "" {
		*tlsCert = filepath.Join(homeRoot, ".tls", "cert.pem")
	}
	if *tlsKey == "" {
		*tlsKey = filepath.Join(homeRoot, ".tls", "key.pem")
	}

	srv := &http.Server{
		Handler:     handler,
		ReadTimeout: 15 * time.Second,
		IdleTimeout: 60 * time.Second,
		BaseContext: func(_ net.Listener) context.Context { return srvBaseCtx },
	}

	// TLS or plain HTTP
	go func() {
		if *httpOnly {
			log.Info("starting HTTP/1.1 (--http flag, no TLS)", zap.Int("port", actualPort))
			err := srv.Serve(listener)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("serve", zap.Error(err))
			}
		} else {
			// 校验 cert 存在
			if _, err := os.Stat(*tlsCert); os.IsNotExist(err) {
				log.Error("TLS cert not found; run cmd/resources or use --http", zap.String("path", *tlsCert))
				os.Exit(1)
			}
			log.Info("starting HTTPS + HTTP/2", zap.Int("port", actualPort))
			err := srv.ServeTLS(listener, *tlsCert, *tlsKey)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("serve TLS", zap.Error(err))
			}
		}
	}()

	// 现有 BACKEND_PORT 输出可能要带 protocol:
	scheme := "https"
	if *httpOnly { scheme = "http" }
	fmt.Printf("BACKEND_URL=%s://127.0.0.1:%d\n", scheme, actualPort) // 替代 BACKEND_PORT
	fmt.Printf("BACKEND_PORT=%d\n", actualPort) // backward compat for testend
}
```

- [ ] **Step 2: 测试 manual 启动(default → HTTPS,--http → HTTP)**

```bash
cd backend && go run ./cmd/server --dev --data-dir /tmp/forgify-test --http
# 应该正常启,$BACKEND_PORT 输出
```

cert 存在时:

```bash
go run ./cmd/server --dev --data-dir /tmp/forgify-test
# Should start HTTPS;若 cert 不存在应报错出
```

- [ ] **Step 3: Commit**

---

### Task 10:cmd/resources 扩展 mkcert auto-setup

**Files:**
- Modify: `backend/cmd/resources/main.go`

- [ ] **Step 1: 加 mkcert subcommand**

```go
// cmd/resources 现状是 mise binary fetcher。加 mkcert auto-setup:
//   --setup-tls   bootstrap mkcert + generate cert/key 到 ~/.forgify/.tls/

func setupTLS(forgifyHome string) error {
	tlsDir := filepath.Join(forgifyHome, ".tls")
	if err := os.MkdirAll(tlsDir, 0700); err != nil { return err }

	certPath := filepath.Join(tlsDir, "cert.pem")
	keyPath := filepath.Join(tlsDir, "key.pem")

	// 1. 检查 cert 是否已存在 + 有效
	if certValid(certPath) {
		log.Info("TLS cert already valid, skip", zap.String("path", certPath))
		return nil
	}

	// 2. 检查 mkcert binary
	mkcert, err := exec.LookPath("mkcert")
	if err != nil {
		return fmt.Errorf("mkcert not found in PATH;请先 `brew install mkcert` (macOS) / `apt install mkcert` (Linux) / `scoop install mkcert` (Windows)")
	}

	// 3. mkcert -install(装 root CA)
	cmd := exec.Command(mkcert, "-install")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mkcert -install: %w", err)
	}

	// 4. mkcert 生成 localhost cert
	cmd = exec.Command(mkcert,
		"-cert-file", certPath,
		"-key-file", keyPath,
		"localhost", "127.0.0.1", "::1",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mkcert: %w", err)
	}

	log.Info("TLS cert generated", zap.String("cert", certPath), zap.String("key", keyPath))
	return nil
}

func certValid(certPath string) bool {
	data, err := os.ReadFile(certPath)
	if err != nil { return false }
	block, _ := pem.Decode(data)
	if block == nil { return false }
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil { return false }
	// 检查未过期 + 至少剩 30 天
	return time.Now().Before(cert.NotAfter.Add(-30 * 24 * time.Hour))
}
```

- [ ] **Step 2: Makefile 加 target**

```makefile
# Makefile 加:
.PHONY: setup-tls
setup-tls:
	cd backend && go run ./cmd/resources --setup-tls

# environment target 加 setup-tls 依赖
environment: ensure-resources setup-tls
	@echo "Environment ready"
```

- [ ] **Step 3: 跑 setup-tls 验证**

```bash
make setup-tls
ls ~/.forgify/.tls/  # 应该有 cert.pem + key.pem
```

- [ ] **Step 4: Commit**

---

### Task 11:testend URL 协议切到 https + 文档

**Files:**
- Modify: `testend/index.html`(若有 hardcoded http://)
- Modify: `testend/script.js` 等
- Modify: `documents/version-1.2/testend-design.md`(更新启动说明)

- [ ] **Step 1: grep + 替换 testend 中的 http://localhost / http:// 为 协议-relative**

```bash
grep -rn "http://" testend/ | grep -v node_modules
```

替换为相对路径或 https:// 跟随 backend。

- [ ] **Step 2: testend-design.md 加新启动说明**

> testend 现在默认走 HTTPS。第一次启动前 `make setup-tls`(仅一次,装 root CA)。之后 `make test-console` 会自动开 https://localhost:port。
> 临时回退 HTTP:`go run ./cmd/server --dev --http`(然后 testend URL 用 http://)。

- [ ] **Step 3: Commit**

---

## Phase 6:Cross-platform + Doc Sync

### Task 12:三平台 cross-compile + staticcheck

参考 Plan 01 Task 25。注意 Windows 的 mkcert 是单独 binary,不在我们的 embed 范围。

- [ ] Step 1-3

---

### Task 13:Doc sync(per S14)

**Files:**
- Modify: `documents/version-1.2/service-contract-documents/api-design.md`(eventlog 端点 query 参更新)
- Modify: `documents/version-1.2/service-contract-documents/events-design.md`(Scope 字段说明)
- Modify: `documents/version-1.2/progress-record.md`(dev log)
- Modify: `documents/version-1.2/backend-design.md`(Verification 段提 HTTPS 默认)

- [ ] Step 1-3:写 + commit

---

## Phase 7:PR + Merge

### Task 14:Open PR

```bash
gh pr create --title "feat(eventlog+transport): scope generalization + HTTP/2 + mkcert" --body "$(cat <<'EOF'
## Summary
- Eventlog scope generalization (D19): conversationId → Scope{Kind, ID}
- 5 V1 kinds: conversation / flowrun / function / handler / workflow
- Bridge multi-scope subscribe (one channel, multiple scope keys)
- Function/Handler ApplyOps 双写到 conversation scope + entity scope
- HTTP /api/v1/eventlog accepts ?scope=<kind>:<id> repeated + backward compat ?conversationId=
- HTTPS + HTTP/2 default (Go net/http auto h2 over TLS)
- mkcert auto-setup via cmd/resources --setup-tls
- testend URLs switched to https://
- --http flag for backward-compat fallback

## Test plan
- [x] make test-unit 全绿
- [x] make test-pipeline (eventlog scope tests)
- [x] manual: start HTTPS, curl --insecure work, browser opens with no warning post-mkcert
- [x] manual: --http fallback works (legacy testend mode)
- [x] 三平台 cross-compile

## Related
- spec: 05-execution-plane.md §4 (eventlog) + §12 (transport)
- plan: plans/03-eventlog-and-transport.md
EOF
)"
```

---

## Acceptance criteria

1. ✅ 14 task 全 done
2. ✅ make test-unit + make test-pipeline 通过
3. ✅ HTTPS + HTTP/2 自动启,浏览器免警告(mkcert 信任装好后)
4. ✅ `?scope=` 多参 + `?conversationId=` 兼容都工作
5. ✅ Function / Handler 锻造时事件双写到 conversation + entity scope
6. ✅ S14 文档同步
7. ✅ PR merge to main + push

完工后,Plan 04(Workflow authoring)接力。

---

(本 plan 完)
