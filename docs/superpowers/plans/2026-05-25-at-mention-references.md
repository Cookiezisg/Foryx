# @-Mention 引用 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用户在 chat 里 @ 一个实体（document / function / handler / workflow）→ 发送时把它的内容**快照**进这条用户消息 → 之后是普通历史消息，LLM 自然看得到，零每轮重注入。

**Architecture:** 新 `domain/mention` 端口（`Resolver` + `Reference`，参照 `domain/catalog`）。每个 app 实现 `AsMentionResolver()`。`chat.Service` 持 `type→resolver` 注册表；`Send` 时解析每个 mention → 存进 `Message.Attrs["mentions"]`（零迁移，跟 `attachments` 同构）；`buildUserLLMMessage` 把存好的快照渲染成 `<mention>` XML 块注入 user 消息。

**Tech Stack:** Go · GORM + modernc.org/sqlite · zap · React（前端 composer 收尾）。测试：`make test-backend`、`go test -tags=pipeline ./test/...`、前端 `npm run build` / vitest。

> 依据 spec：`docs/superpowers/specs/2026-05-25-at-mention-references-design.md`。

---

## 关键不变量

- mentions 存 `Message.Attrs["mentions"] = []mentiondomain.Reference`（`Attrs` 本就是 `gorm:"type:text;serializer:json"`，**无 schema 迁移**）。
- 解析时机：`chat.Service.Send`（消息创建）一次性解析 + 快照。`buildUserLLMMessage` 只读快照渲染，**不重解析**。
- 解析失败（实体被删 / 任何错误）→ 存 stub `Reference{Name:"(无法加载)", Content:""}`，**不阻断发消息**。
- 范围 4 类：document / function / handler / workflow。skill/mcp 不做。
- function/handler/workflow 的内容在 **active Version** 上（`Get(id)` → `entity.ActiveVersionID` → `GetVersion(...)`）；document 内容在主实体。

---

## File Structure

**Create:**
- `backend/internal/domain/mention/mention.go` — `MentionType` / `MentionInput` / `Reference` / `Resolver` 端口
- `backend/internal/app/document/mention_resolver.go` — document resolver
- `backend/internal/app/function/mention_resolver.go` — function resolver
- `backend/internal/app/handler/mention_resolver.go` — handler resolver
- `backend/internal/app/workflow/mention_resolver.go` — workflow resolver
- `backend/internal/app/chat/mention.go` — `renderMentionsXML` 渲染助手
- `backend/internal/app/chat/mention_test.go` — chat 侧单测（含 fake resolver）

**Modify:**
- `backend/internal/app/chat/chat.go` — `SendInput.Mentions`、`mentionResolvers` 字段、`RegisterMentionResolver`、`Send` 解析+存
- `backend/internal/app/chat/history.go` — `buildUserLLMMessage` 渲染 mentions
- `backend/internal/transport/httpapi/handlers/chat.go` — `sendMessageRequest.Mentions`、`SendMessage` 透传
- `backend/cmd/server/main.go` — `RegisterMentionResolver` ×4
- `frontend/src/panes/chat/Composer.jsx` — mentionPool 加 `type`、去 skill
- `frontend/src/panes/chat/ChatPane.jsx` — 发送 `{type, id}`
- 文档 7 处（Task 8）

---

## Task 1: domain/mention 端口

**Files:** Create `backend/internal/domain/mention/mention.go`

- [ ] **Step 1: 写端口文件**

```go
// Package mention is the domain layer for @-mention references injected into chat messages.
//
// Package mention 是 @ 引用的 domain 层：被引用实体解析成 Reference 烤进消息。
package mention

import "context"

// MentionType is the closed set of @-mentionable entity kinds.
//
// MentionType 是可被 @ 的实体类型（封闭集）。
type MentionType string

const (
	MentionDocument MentionType = "document"
	MentionFunction MentionType = "function"
	MentionHandler  MentionType = "handler"
	MentionWorkflow MentionType = "workflow"
)

// MentionInput is the wire shape the frontend sends per mention: type + id only.
//
// MentionInput 是前端每个 mention 发来的形状：只 type + id。
type MentionInput struct {
	Type MentionType `json:"type"`
	ID   string      `json:"id"`
}

// Reference is the resolved snapshot stored on the message + rendered into the transcript.
// Content is the type-specific body (doc markdown / function code / handler methods / workflow graph).
//
// Reference 是已解析快照，存进消息 + 渲进 transcript；Content 是各类型自渲的内文。
type Reference struct {
	Type    MentionType `json:"type"`
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Content string      `json:"content"`
}

// Resolver is implemented by each capability app; chat holds a type→resolver registry.
//
// Resolver 由各 app 实现；chat 持 type→resolver 注册表。
type Resolver interface {
	Type() MentionType
	Resolve(ctx context.Context, id string) (*Reference, error)
}
```

- [ ] **Step 2: 编译**

Run: `cd backend && go build ./internal/domain/mention/`
Expected: 通过（纯类型，无依赖）。

- [ ] **Step 3: Commit**

```bash
git add backend/internal/domain/mention/mention.go
git commit -m "feat(mention): domain 端口 — Reference + Resolver + MentionType"
```

---

## Task 2: chat 侧管线（注册表 + Send 解析存储 + 渲染）

**Files:**
- Modify: `backend/internal/app/chat/chat.go`
- Modify: `backend/internal/app/chat/history.go`
- Create: `backend/internal/app/chat/mention.go`
- Create: `backend/internal/app/chat/mention_test.go`

- [ ] **Step 1: 写 `mention.go`（渲染助手）**

`backend/internal/app/chat/mention.go`：

```go
package chat

import (
	"fmt"
	"strings"
	"time"

	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
)

// renderMentionsXML turns resolved references into one <mentions> block for the
// user LLM message. Code types carry a snapshot marker so the LLM refetches
// (get_function/...) before editing; documents are static reference content.
//
// renderMentionsXML 把已解析引用拼成一个 <mentions> 块；代码类带 snapshot 标记
// 提示 LLM 改前先 get 最新，document 是静态参考内容。
func renderMentionsXML(refs []mentiondomain.Reference, sentAt time.Time) string {
	var b strings.Builder
	b.WriteString("<mentions>\n")
	for _, r := range refs {
		fmt.Fprintf(&b, "<mention type=%q id=%q name=%q>\n", r.Type, r.ID, r.Name)
		if r.Type != mentiondomain.MentionDocument && r.Content != "" {
			fmt.Fprintf(&b, "(snapshot at %s)\n", sentAt.UTC().Format(time.RFC3339))
		}
		if r.Content == "" {
			b.WriteString("[引用的实体无法加载]")
		} else {
			b.WriteString(r.Content)
		}
		b.WriteString("\n</mention>\n")
	}
	b.WriteString("</mentions>\n")
	return b.String()
}
```

- [ ] **Step 2: 改 `chat.go` — 加 mentionResolvers 字段 + 注册 + SendInput.Mentions**

在 `Service` struct（`chat.go` 约 70-75，`documents DocumentResolver` 附近）加字段：

```go
	mentionResolvers map[mentiondomain.MentionType]mentiondomain.Resolver
```

文件顶部 import 加：

```go
	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
```

在 `SetDocumentResolver` 附近（约 184）加注册方法：

```go
// RegisterMentionResolver wires one capability app's @-mention resolver, keyed by its Type().
//
// RegisterMentionResolver 注册一个 app 的 @ resolver，按 Type() 入表。
func (s *Service) RegisterMentionResolver(r mentiondomain.Resolver) {
	if s.mentionResolvers == nil {
		s.mentionResolvers = map[mentiondomain.MentionType]mentiondomain.Resolver{}
	}
	s.mentionResolvers[r.Type()] = r
}
```

找到 `SendInput` 定义（grep `type SendInput struct`），加字段：

```go
	Mentions []mentiondomain.MentionInput
```

- [ ] **Step 3: 改 `chat.go::Send` — 空内容判定 + 解析存储**

`Send`（约 250）开头的空判定改为也认 mentions：

```go
	if strings.TrimSpace(in.Content) == "" && len(in.AttachmentIDs) == 0 && len(in.Mentions) == 0 {
		return "", fmt.Errorf("chat.Service.Send: %w", chatdomain.ErrEmptyContent)
	}
```

在 attachments 那段（`attrs["attachments"] = refs` 之后、`var attrsField` 之前，约 276 行后）插入：

```go
	if len(in.Mentions) > 0 {
		mrefs := make([]mentiondomain.Reference, 0, len(in.Mentions))
		for _, mi := range in.Mentions {
			resolver, ok := s.mentionResolvers[mi.Type]
			if !ok {
				s.log.Warn("chat.Service.Send: no resolver for mention type; skipping",
					zap.String("type", string(mi.Type)), zap.String("id", mi.ID))
				continue
			}
			ref, err := resolver.Resolve(ctx, mi.ID)
			if err != nil {
				// Deleted / transient / any error → stub; never fail the send.
				// 删了 / 瞬时 / 任何错误 → stub；绝不因引用毁掉发消息。
				s.log.Warn("chat.Service.Send: mention resolve failed; storing stub",
					zap.String("type", string(mi.Type)), zap.String("id", mi.ID), zap.Error(err))
				mrefs = append(mrefs, mentiondomain.Reference{Type: mi.Type, ID: mi.ID, Name: "(无法加载)"})
				continue
			}
			mrefs = append(mrefs, *ref)
		}
		if len(mrefs) > 0 {
			attrs["mentions"] = mrefs
		}
	}
```

- [ ] **Step 4: 改 `history.go::buildUserLLMMessage` — 渲染 mentions**

`history.go` import 加 `mentiondomain` + `time`（若未引）。在 text-blocks 循环之后（约 102 行，`if len(m.Attrs) > 0` attachments 块**之前**）插入：

```go
	if len(m.Attrs) > 0 {
		if rawMentions, ok := m.Attrs["mentions"]; ok {
			raw, err := json.Marshal(rawMentions)
			if err == nil {
				var refs []mentiondomain.Reference
				if err := json.Unmarshal(raw, &refs); err != nil {
					s.log.Warn("chat.Service.buildUserLLMMessage: malformed Message.Attrs mentions; dropped",
						zap.String("message_id", m.ID), zap.Error(err))
				} else if len(refs) > 0 {
					parts = append(parts, llminfra.ContentPart{Type: "text", Text: renderMentionsXML(refs, m.CreatedAt)})
				}
			}
		}
	}
```

- [ ] **Step 5: 写 `mention_test.go`（fake resolver + 端到端 chat 侧）**

```go
package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
)

func TestRenderMentionsXML_DocumentNoSnapshotMarker(t *testing.T) {
	refs := []mentiondomain.Reference{
		{Type: mentiondomain.MentionDocument, ID: "doc_1", Name: "Spec", Content: "# Body"},
	}
	out := renderMentionsXML(refs, time.Now())
	if !strings.Contains(out, `<mention type="document" id="doc_1" name="Spec">`) {
		t.Errorf("missing document tag: %q", out)
	}
	if strings.Contains(out, "snapshot at") {
		t.Errorf("document should not carry snapshot marker: %q", out)
	}
	if !strings.Contains(out, "# Body") {
		t.Errorf("missing doc content: %q", out)
	}
}

func TestRenderMentionsXML_CodeCarriesSnapshotMarker(t *testing.T) {
	refs := []mentiondomain.Reference{
		{Type: mentiondomain.MentionFunction, ID: "f_1", Name: "csv", Content: "def csv(): pass"},
	}
	out := renderMentionsXML(refs, time.Date(2026, 5, 25, 8, 0, 0, 0, time.UTC))
	if !strings.Contains(out, "(snapshot at 2026-05-25T08:00:00Z)") {
		t.Errorf("function should carry snapshot marker: %q", out)
	}
}

func TestRenderMentionsXML_StubRendersPlaceholder(t *testing.T) {
	refs := []mentiondomain.Reference{
		{Type: mentiondomain.MentionDocument, ID: "doc_x", Name: "(无法加载)", Content: ""},
	}
	out := renderMentionsXML(refs, time.Now())
	if !strings.Contains(out, "[引用的实体无法加载]") {
		t.Errorf("stub should render placeholder: %q", out)
	}
}

type fakeMentionResolver struct {
	typ mentiondomain.MentionType
	ref *mentiondomain.Reference
	err error
}

func (f fakeMentionResolver) Type() mentiondomain.MentionType { return f.typ }
func (f fakeMentionResolver) Resolve(_ context.Context, _ string) (*mentiondomain.Reference, error) {
	return f.ref, f.err
}

func TestRegisterMentionResolver_KeysByType(t *testing.T) {
	s := &Service{}
	s.RegisterMentionResolver(fakeMentionResolver{typ: mentiondomain.MentionDocument})
	if _, ok := s.mentionResolvers[mentiondomain.MentionDocument]; !ok {
		t.Error("resolver not registered under its Type()")
	}
}
```

- [ ] **Step 6: 测试 + commit**

Run: `cd backend && go build ./... && go test ./internal/app/chat/ -run 'Mention|RenderMentions' -v`
Expected: PASS。

```bash
git add backend/internal/app/chat/mention.go backend/internal/app/chat/mention_test.go backend/internal/app/chat/chat.go backend/internal/app/chat/history.go
git commit -m "feat(chat): @-mention 管线 — 注册表 + Send 解析存储 + transcript 渲染"
```

---

## Task 3: document MentionResolver

**Files:** Create `backend/internal/app/document/mention_resolver.go`; Test `backend/internal/app/document/mention_resolver_test.go`

- [ ] **Step 1: 写 resolver**

```go
package document

import (
	"context"
	"fmt"

	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
)

type mentionResolver struct{ svc *Service }

// AsMentionResolver exposes this service as a chat @-mention resolver for documents.
//
// AsMentionResolver 把本 service 暴露为 document 的 @ resolver。
func (s *Service) AsMentionResolver() mentiondomain.Resolver { return &mentionResolver{svc: s} }

func (r *mentionResolver) Type() mentiondomain.MentionType { return mentiondomain.MentionDocument }

func (r *mentionResolver) Resolve(ctx context.Context, id string) (*mentiondomain.Reference, error) {
	d, err := r.svc.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("document.mentionResolver.Resolve %s: %w", id, err)
	}
	content := d.Description
	if d.Content != "" {
		if content != "" {
			content += "\n\n"
		}
		content += d.Content
	}
	return &mentiondomain.Reference{
		Type: mentiondomain.MentionDocument, ID: d.ID, Name: d.Name, Content: content,
	}, nil
}
```

- [ ] **Step 2: 写测试**

```go
package document

import (
	"strings"
	"testing"

	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	documentstore "github.com/sunweilin/forgify/backend/internal/infra/store/document"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
	"go.uber.org/zap/zaptest"
)

func TestMentionResolver_ResolvesDocContent(t *testing.T) {
	gdb, err := dbinfra.Open(dbinfra.Config{DataDir: ""})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	svc := NewService(documentstore.New(gdb), zaptest.NewLogger(t))
	ctx := reqctxpkg.SetUserID(t.Context(), "u_test")

	d, err := svc.Create(ctx, CreateInput{Name: "Spec", Description: "the spec", Content: "# Hello"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	r := svc.AsMentionResolver()
	if r.Type() != "document" {
		t.Errorf("Type() = %q, want document", r.Type())
	}
	ref, err := r.Resolve(ctx, d.ID)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if ref.Name != "Spec" || !strings.Contains(ref.Content, "# Hello") || !strings.Contains(ref.Content, "the spec") {
		t.Errorf("bad reference: %+v", ref)
	}
}

func TestMentionResolver_NotFound_Errors(t *testing.T) {
	gdb, _ := dbinfra.Open(dbinfra.Config{DataDir: ""})
	svc := NewService(documentstore.New(gdb), zaptest.NewLogger(t))
	ctx := reqctxpkg.SetUserID(t.Context(), "u_test")
	if _, err := svc.AsMentionResolver().Resolve(ctx, "doc_missing"); err == nil {
		t.Error("Resolve of missing doc should error")
	}
}
```

> 注：`NewService` / `CreateInput` / `documentstore.New` 签名以 document 包现有为准（实现时 grep 确认；上面是标准用法）。

- [ ] **Step 3: 测试 + commit**

Run: `cd backend && go test ./internal/app/document/ -run Mention -v`
Expected: PASS。

```bash
git add backend/internal/app/document/mention_resolver.go backend/internal/app/document/mention_resolver_test.go
git commit -m "feat(document): @-mention resolver — 注入正文 + 描述"
```

---

## Task 4: function / handler / workflow MentionResolver（versioned 三件套）

> 三者都是 `Get(id)` → `entity.ActiveVersionID` → `GetVersion(versionID)` → 取内容。各 app 一个文件。

**Files:** Create `internal/app/function/mention_resolver.go`、`internal/app/handler/mention_resolver.go`、`internal/app/workflow/mention_resolver.go`（+ 各自 `_test.go`）。

- [ ] **Step 1: function resolver**

`backend/internal/app/function/mention_resolver.go`：

```go
package function

import (
	"context"
	"fmt"

	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
)

type mentionResolver struct{ svc *Service }

func (s *Service) AsMentionResolver() mentiondomain.Resolver { return &mentionResolver{svc: s} }

func (r *mentionResolver) Type() mentiondomain.MentionType { return mentiondomain.MentionFunction }

func (r *mentionResolver) Resolve(ctx context.Context, id string) (*mentiondomain.Reference, error) {
	fn, err := r.svc.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("function.mentionResolver.Resolve %s: %w", id, err)
	}
	content := fn.Description
	if fn.ActiveVersionID != "" {
		if v, err := r.svc.GetVersion(ctx, fn.ActiveVersionID); err == nil && v.Code != "" {
			if content != "" {
				content += "\n\n"
			}
			content += v.Code
		}
	}
	return &mentiondomain.Reference{
		Type: mentiondomain.MentionFunction, ID: fn.ID, Name: fn.Name, Content: content,
	}, nil
}
```

- [ ] **Step 2: handler resolver**

`backend/internal/app/handler/mention_resolver.go`：

```go
package handler

import (
	"context"
	"fmt"
	"strings"

	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
)

type mentionResolver struct{ svc *Service }

func (s *Service) AsMentionResolver() mentiondomain.Resolver { return &mentionResolver{svc: s} }

func (r *mentionResolver) Type() mentiondomain.MentionType { return mentiondomain.MentionHandler }

func (r *mentionResolver) Resolve(ctx context.Context, id string) (*mentiondomain.Reference, error) {
	hd, err := r.svc.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("handler.mentionResolver.Resolve %s: %w", id, err)
	}
	var b strings.Builder
	b.WriteString(hd.Description)
	if hd.ActiveVersionID != "" {
		if v, err := r.svc.GetVersion(ctx, hd.ActiveVersionID); err == nil {
			if len(v.InitArgsSchema) > 0 {
				fmt.Fprintf(&b, "\n\ninit args:")
				for _, a := range v.InitArgsSchema {
					fmt.Fprintf(&b, "\n- %s (%s)", a.Name, a.Type)
				}
			}
			if len(v.Methods) > 0 {
				b.WriteString("\n\nmethods:")
				for _, m := range v.Methods {
					fmt.Fprintf(&b, "\n- %s", m.Name)
				}
			}
		}
	}
	return &mentiondomain.Reference{
		Type: mentiondomain.MentionHandler, ID: hd.ID, Name: hd.Name, Content: b.String(),
	}, nil
}
```

> 注：`InitArgSpec` 字段（`Name` / `Type`）与 `MethodSpec.Name` 以 `internal/domain/handler/method.go` 实际为准（实现时 grep 确认字段名；上面是预期形状）。

- [ ] **Step 3: workflow resolver**

`backend/internal/app/workflow/mention_resolver.go`：

```go
package workflow

import (
	"context"
	"fmt"

	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
)

type mentionResolver struct{ svc *Service }

func (s *Service) AsMentionResolver() mentiondomain.Resolver { return &mentionResolver{svc: s} }

func (r *mentionResolver) Type() mentiondomain.MentionType { return mentiondomain.MentionWorkflow }

func (r *mentionResolver) Resolve(ctx context.Context, id string) (*mentiondomain.Reference, error) {
	w, err := r.svc.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("workflow.mentionResolver.Resolve %s: %w", id, err)
	}
	content := w.Description
	if w.ActiveVersionID != "" {
		if v, err := r.svc.GetVersion(ctx, w.ActiveVersionID); err == nil && v.Graph != "" {
			if content != "" {
				content += "\n\n"
			}
			content += v.Graph // 完整 frozen JSON 定义（nodes/edges）
		}
	}
	return &mentiondomain.Reference{
		Type: mentiondomain.MentionWorkflow, ID: w.ID, Name: w.Name, Content: content,
	}, nil
}
```

> 注：`Workflow.Description` 若不在主实体（grep `internal/domain/workflow`，描述可能在 active version 的 `Graph.Description`），实现时改为从 version 取；`v.Graph` 是 raw JSON 字符串（`version.go:13`）。

- [ ] **Step 4: 各写一个 resolver 单测**（仿 Task 3 的 document 测试：建实体 → Resolve → 断言 Name + Content 含代码/方法/图）。每个 app 用 `harness` 或各自 store + in-memory `dbinfra.Open(Config{DataDir:""})`。最小断言：`Type()` 正确 + `Resolve` 返回的 `Content` 含关键内容（function 含代码片段 / handler 含 method 名 / workflow 含 node）。

- [ ] **Step 5: 编译 + 测试 + commit**

Run: `cd backend && go build ./... && go test ./internal/app/function/ ./internal/app/handler/ ./internal/app/workflow/ -run Mention -v`
Expected: PASS。

```bash
git add backend/internal/app/function/mention_resolver*.go backend/internal/app/handler/mention_resolver*.go backend/internal/app/workflow/mention_resolver*.go
git commit -m "feat(forge): function/handler/workflow @-mention resolver — 取 active version 内容"
```

---

## Task 5: HTTP handler + main.go 装配

**Files:** Modify `backend/internal/transport/httpapi/handlers/chat.go`、`backend/cmd/server/main.go`

- [ ] **Step 1: handler 加 mentions 字段 + 透传**

`handlers/chat.go`，`sendMessageRequest`（约 72）改为：

```go
type sendMessageRequest struct {
	Content       string                       `json:"content"`
	AttachmentIDs []string                     `json:"attachmentIds"`
	Mentions      []mentiondomain.MentionInput `json:"mentions"`
}
```

import 加 `mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"`。`SendMessage` 里透传：

```go
	msgID, err := h.svc.Send(r.Context(), id, chatapp.SendInput{
		Content:       req.Content,
		AttachmentIDs: req.AttachmentIDs,
		Mentions:      req.Mentions,
	})
```

（这一步同时解掉"`DisallowUnknownFields` 拒收 mentions"的断链。）

- [ ] **Step 2: main.go 注册 4 resolver**

`cmd/server/main.go`，在 `chatService.SetDocumentResolver(documentService)` 之后加：

```go
	chatService.RegisterMentionResolver(documentService.AsMentionResolver())
	chatService.RegisterMentionResolver(functionService.AsMentionResolver())
	chatService.RegisterMentionResolver(handlerService.AsMentionResolver())
	chatService.RegisterMentionResolver(workflowService.AsMentionResolver())
```

> 实现时 grep 确认 4 个 service 变量名（`functionService` / `handlerService` / `workflowService` / `documentService`）在 main.go 的实际命名。

- [ ] **Step 3: 编译 + 全单测 + commit**

Run: `cd backend && go build ./... && make test-backend && staticcheck ./...`
Expected: build 通过；test-backend 全绿；staticcheck 无新告警。

```bash
git add backend/internal/transport/httpapi/handlers/chat.go backend/cmd/server/main.go
git commit -m "feat(chat): SendMessage 接收 mentions + main 装配 4 resolver"
```

---

## Task 6: 前端 composer 收尾

**Files:** Modify `frontend/src/panes/chat/Composer.jsx`、`frontend/src/panes/chat/ChatPane.jsx`

- [ ] **Step 1: Composer mentionPool 加 type + 去 skill**

`Composer.jsx`：import 去掉 `useSkills`（第 14 行改为 `import { useDocuments } from "../../api/library.js";`）；删 `const { data: skills = [] } = useSkills();`（约 28）。`mentionPool`（47-53）改为：

```js
  const mentionPool = () => [
    ...functions.map((f) => ({ type: "function", id: f.id, label: f.name + " · function", icon: "Code" })),
    ...handlers.map((h) => ({ type: "handler", id: h.id, label: h.name + " · handler", icon: "Server" })),
    ...workflows.map((w) => ({ type: "workflow", id: w.id, label: w.name + " · workflow", icon: "Workflow" })),
    ...documents.map((d) => ({ type: "document", id: d.id, label: (d.name || d.title || d.id) + " · doc", icon: "FileText" })),
  ];
```

文件头注释（2-3 行）把 "functions / handlers / workflows / skills / documents" 改为 "functions / handlers / workflows / documents"。

- [ ] **Step 2: ChatPane 发送 {type, id}**

`ChatPane.jsx` 第 116 行：

```js
    if (mentions?.length) body.mentions = mentions.map((m) => ({ type: m.type, id: m.id }));
```

- [ ] **Step 3: 构建 + commit**

Run: `cd frontend && npm run build`
Expected: 通过（无 `useSkills` 残引用）。

```bash
git add frontend/src/panes/chat/Composer.jsx frontend/src/panes/chat/ChatPane.jsx
git commit -m "feat(frontend): @ 发送 {type,id} + mentionPool 去 skill"
```

---

## Task 7: pipeline 端到端测试

**Files:** Create `backend/test/chat/mention_pipeline_test.go`（或并入现有 chat pipeline 目录；用 `test/harness`）

- [ ] **Step 1: 写 pipeline 测试**

要点（用 `th.New(t)` + `h.LocalCtx()`）：
1. **@ document → 正文进 transcript**：`h.Document.Create` 建 doc（Content 含独特串如 "ZEBRA-MARKER"）→ 走 chat send（带 `mentions:[{type:"document", id}]`，经 `h.Chat.Send` 或 PostMessage helper）→ 取 LLM transcript（fake LLM 的 LastSystemPrompt/messages 或 buildHistory 暴露点）→ 断言 user 消息含 `<mention type="document"` + "ZEBRA-MARKER"。
2. **@ 已删实体 → stub，消息照发**：mentions 指一个不存在的 id → `Send` 不报错 + transcript 含 "[引用的实体无法加载]"。
3. **快照稳定**：send 后改 doc Content → 重新 buildHistory → 那条老消息仍是旧 "ZEBRA-MARKER"（不随实体改动漂移）。

实现时参照 `test/integration/d9_test.go` 取 transcript / fake LLM 的方式（`th.NewFakeLLMServer` + `fake.LastSystemPrompt()` 或消息断点）。`h.Chat.Send` 的 `SendInput.Mentions` 直接传 `[]mentiondomain.MentionInput{{Type:"document", ID: doc.ID}}`。

- [ ] **Step 2: 跑 + commit**

Run: `cd backend && go test -count=1 -tags=pipeline -p 1 ./test/chat/... -run Mention -v`（或对应目录）
Expected: PASS。

```bash
git add backend/test/chat/mention_pipeline_test.go
git commit -m "test(mention): pipeline — @文档进 transcript / stub / 快照稳定"
```

---

## Task 8: 文档同步（§S14）

**Files:** `documents/version-1.2/` 多处 + `frontend-prd.md`

- [ ] **Step 1:** 新建 `service-design-documents/mention.md` —— 端口 / 4 resolver / `Attrs["mentions"]` / 渲染 / 范围 / 快照语义。
- [ ] **Step 2:** `service-design-documents/chat.md` —— `SendInput.Mentions`、`Message.Attrs["mentions"]`、`buildUserLLMMessage` 注入、`RegisterMentionResolver`。
- [ ] **Step 3:** `service-design-documents/{document,function,handler,workflow}.md` —— 各加一行"实现 `mention.Resolver`（`AsMentionResolver`）"。
- [ ] **Step 4:** `service-contract-documents/api-design.md` —— `POST /conversations/{id}/messages` 请求体加 `mentions: [{type, id}]`。
- [ ] **Step 5:** `frontend-prd.md` —— §17 mention 发送 shape（type+id）；mentionPool 去 skill（§16 补一行 bug/变更）。
- [ ] **Step 6:** `progress-record.md` —— dev log（做了什么 + 测试数 + 决策：snapshot / 范围 4 类 / skill-mcp 排除理由）。
- [ ] **Step 7: Commit**

```bash
git add documents/ frontend/  # 仅 PRD（若在 frontend 目录）
git commit -m "docs: @-mention 引用同步(mention.md + chat/api/prd/progress §S14)"
```

---

## Self-Review

**Spec coverage:**
- §3 范围 4 类 → Task 3（doc）+ Task 4（trinity）；skill/mcp 不实现 ✓
- §4.1 `Attrs["mentions"]` → Task 2 Step 3 ✓
- §4.2 数据流 → Task 2（Send 解析存）+ Task 5（handler 透传）+ Task 6（前端发 type+id）✓
- §4.3 MentionResolver 端口 + 注册表 → Task 1 + Task 2 Step 2 ✓
- §4.4 渲染 `<mention>` + 代码 snapshot 标记 → Task 2 Step 1（renderMentionsXML）✓
- §4.5 前端去 skill + 发 type+id → Task 6 ✓
- §5 错误 stub 不阻断 → Task 2 Step 3（resolve err → stub）+ Step 1（占位渲染）✓
- §7 测试 → Task 2/3/4 单测 + Task 7 pipeline ✓
- §8 文档 → Task 8 ✓

**Placeholder scan:** resolver 字段名处标了"实现时 grep 确认"（InitArgSpec 字段 / workflow.Description / main service 变量名）—— 这些是**对现有代码的引用确认**，非待定设计；核心代码均完整给出。Task 4 Step 4 / Task 7 Step 1 给了测试要点 + 关键断言串 + 参照文件，未逐行抄（versioned resolver 测试随各 app store 签名而定）。

**Type consistency:** `mentiondomain.{MentionType, MentionInput, Reference, Resolver}` 贯穿 Task 1→2→3→4→5；`Reference{Type,ID,Name,Content}` 字段一致；`renderMentionsXML(refs, sentAt)` 签名在 Task 2 定义、调用一致；`AsMentionResolver()` 在 4 个 app 一致。

**Scope:** 单一 feature（@ 引用），一个新 domain 包 + 4 resolver + chat 改动 + 前端收尾。任务按编译依赖排序（domain → chat → resolvers → wiring → 前端 → pipeline → docs），每个 commit 落 green。
