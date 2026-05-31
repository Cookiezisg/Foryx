# D-redo audit — events-design.md

审计对象：`documents/version-1.2/service-contract-documents/events-design.md`（273 行）
对比事实源：
- `documents/version-1.2/event-log-protocol.md`（事件日志协议设计文档）
- `backend/internal/domain/eventlog/{eventlog.go,bridge.go}`（5 events × 6 block types + Bridge 接口）
- `backend/internal/domain/notifications/notifications.go`（通用 Event envelope + Bridge）
- `backend/internal/infra/eventlog/bridge.go` + `infra/notifications/bridge.go`（in-process Bridge 实现）
- `backend/internal/pkg/eventlog/eventlog.go` + `pkg/notifications/notifications.go`（Emitter / Publisher）
- producer 位点：`app/chat/{chat.go,runner.go,host.go}`、`app/loop/{stream.go,tools.go}`、`app/subagent/{spawn.go,host.go}`、`app/conversation/conversation.go`、`app/todo/todo.go`、`app/mcp/mcp.go`、`app/skill/scan.go`、`app/catalog/polling.go`、`app/sandbox/sandbox.go`
- handler：`transport/httpapi/handlers/{eventlog.go,notifications.go}`

## In code but not in doc

| Item | Code location | Severity |
|---|---|---|
| sandbox Publisher 持 raw nil 而非 noopPublisher 兜底；`publishEnv` / `publishEnvDeleted` 内部 `if s.notif == nil { return }` guard。其他 5 个 service（conversation / todo / mcp / skill / catalog / chat）都在构造器 `if notif == nil { notif = notificationspkg.New(nil, log) }`。doc §11.4 只描述统一的 noop 兜底约定 | `backend/internal/app/sandbox/sandbox.go:140-152, 648-650, 669-671` | LOW |
| `transport/httpapi/handlers/eventlog.go` 有 `History` 端点（`GET /api/v1/conversations/{id}/eventlog?from=<seq>`）— 在 doc §11/§5 提到了"超 buffer → refetch"和 doc 头部"历史 refetch"，但 doc 未单独列其行为：返 `{events: [...], tailSeq, count}` JSON envelope（不是 SSE）；client 应用 `tailSeq` 作下次重订的 `Last-Event-ID`。doc 未提及 `tailSeq` 字段或 response shape | `backend/internal/transport/httpapi/handlers/eventlog.go:137-175` | LOW |
| skill 通知用 `id="*"` 作 sentinel（"all skills changed"），而非真实 skill ID。doc §11.2 表格未注明此字段是"批量哨兵"特殊语义 | `backend/internal/app/skill/scan.go:106` | LOW |
| catalog 通知用 `id=cat.Fingerprint`（hash 字符串），而非 catalog 业务 ID（catalog 是单实体无 ID 体系）。doc §11.2 表格未提该字段语义 | `backend/internal/app/catalog/polling.go:253` | LOW |

## In doc but not in code (stale)

| Item | Doc location | Severity |
|---|---|---|
| doc §11.2 标 `conversation` producer 行 `app/conversation/Service.{Create,Rename,SetSystemPrompt}` — code 没有 `SetSystemPrompt` 方法。实际方法是 `Create / Rename(→Update) / Update / Delete`，`Delete` 在 doc 行未列 | events-design.md:206 | MED |
| doc §11.2 `conversation` 行 producer 标 line 番号 "168/117/128" — 实际行号是 69/117/128。Create 的 publish 在 line 69 不是 168 | events-design.md:206 | MED |
| doc §11.2 `conversation` 行 还说 "app/chat/runner.afterStreamFinalize 自动改名后" — code 没有 `afterStreamFinalize` 函数。实际是 `app/chat/runner.autoTitle` (line 232) | events-design.md:206 | MED |
| doc §11.2 `mcp_server` 行 producer 标 `Service.{updateStatus,setTools}` line 326/379 — code 没有 `updateStatus` / `setTools` 方法。实际是 `publishStatus` (line 371) 和 `RemoveServer` (line 308)；actual publish 在 line 329/382 | events-design.md:208 | MED |
| doc §11.2 `skill` 行说 producer 触发场景 "fsnotify 触发 rescan 后" — code 完全无 fsnotify 使用。skill 走 1s polling（`backend/internal/app/skill/polling.go::pollLoop` ticker.C）；`fsnotify` 在 `go.mod` 标 indirect，仅作 indirect 依赖存在。`backend/test/integration/d9_test.go` 注释也说 fsnotify 但 production code 无引用 | events-design.md:209 | MED |
| doc §11.2 `sandbox_env` 行 producer 标 `Service.publishEnvUpdate` line 661/682 — code 实际方法名是 `publishEnv` (line 647-666) 和 `publishEnvDeleted` (line 668-676)。`publishEnvUpdate` 不存在；publish 行号是 651/672 | events-design.md:211 | MED |
| doc §11.2 `todo` 行 producer 标 `Service.{Create,Update,Delete}` `(todo.go:249)` — actual publish helper 在 line 247-249（`publish` 函数体），doc 行号 249 偏到了空行收尾位 | events-design.md:207 | LOW |
| doc §11.2 `catalog` 行 producer 标 `Service.applyRefresh` line 253 — code 中 publish 在 `polling.go::applyRefresh` line 253 是对的，但 doc §11.2 未明示 `applyRefresh` 是 polling.go 内函数（doc 写 `app/catalog/Service.applyRefresh` 隐含主 catalog.go，实际在 polling.go）| events-design.md:210 | LOW |

## Semantic drift

| Event / type | Doc says | Code does | Severity |
|---|---|---|---|
| Notifications SSE wire format | doc §11.3 / 表格行 23-25：`event: <type>\nid: <seq>\ndata: <event JSON, 不重复 type/seq>` — `<type>` 是 placeholder，暗示展开为实体类型（`event: conversation` / `event: todo` 等） | code `transport/httpapi/handlers/notifications.go:98`: `fmt.Fprintf(out, "event: notification\nid: %d\ndata: %s\n\n", ...)` — SSE event name 固定字面量 `notification`，实体类型在 JSON `data.type` 字段。doc wire format 误导前端会以为按 SSE event-name 路由 | **HIGH** |
| Notifications subscribe — 单订阅 | doc §11.3：`GET /api/v1/notifications` 单流，无 query 参数 | code 一致：`handlers/notifications.go:60-62` 路由 `GET /api/v1/notifications`，`Stream` 不读任何 query 参 | ✓ no drift |
| Notifications 410 行为 | doc §11.3：超 buffer 返 410 Gone + code=SEQ_TOO_OLD → 客户端清缓存重订（无 fromSeq）+ 经 REST refetch | code `handlers/notifications.go:77-81`：返 410 + SEQ_TOO_OLD + msg "resubscribe without it"。一致 | ✓ no drift |
| Eventlog 410 行为 | doc §5（§N7 wire format 节）：超 buffer 返 410 + code=SEQ_TOO_OLD → 客户端 `GET /api/v1/conversations/{id}/eventlog?from=<seq>` refetch 全态 | code `handlers/eventlog.go:107-113`：返 410 + SEQ_TOO_OLD + msg "refetch full state"。一致 | ✓ no drift |
| Publisher 第 5 参 conversationID 语义 | doc §11.4：`// 第 5 参 conversationID 必填——不绑对话的实体传 ""` | code `pkg/notifications.go:37`：`Publish(ctx, eventType, id, data, conversationID)` — Go 函数所有参数都必填；doc 说"必填"但允许传 `""`，与 Go 语法 "required parameter" 概念冲突。实际语义是"位置参数必传，conversation-scoped 实体传真值，非 scoped 传空"。措辞模糊但无功能 drift | LOW |
| Bridge buffer 大小 | doc §11.5 表："eventlog: per-conv seq + 4096 replay buffer / notifications: global seq + replay buffer"；§eventlog 顶部 doc 说"4096 replay" | code eventlog `replayBufferSize=4096`、notifications `replayBufferSize=1024`。doc §11.5 notifications 行只说 "replay buffer" 未给具体值（4096 vs 1024 差 4x），易让人以为相同 | LOW |
| Emitter ctx-injected 模式 | doc §11.5 表："eventlog 紧密耦合（5 类型固定 schema）/ notifications 松散耦合（Publisher 接受任意 type 字符串）" + §11.4 "pkg/notifications.Publisher (constructor-injected struct field; no ctx wiring — no producer needs it)" | code `pkg/eventlog/eventlog.go:421-441` 有 `With(ctx, em)` + `From(ctx)` ctx-injection；`pkg/notifications/notifications.go` 无对应 ctx helpers，纯 struct field 注入。一致 | ✓ no drift |

## Producer-less event (defined but no publisher)

| Event type | Code defined | Severity |
|---|---|---|
| Eventlog 5 events × 6 block types | `domain/eventlog/eventlog.go:128-214`（5 events）+ `domain/eventlog/eventlog.go:45-79`（6 block types）。每个 event 与 block type 都有 producer：MessageStart/Stop 在 chat/chat.go (user) + chat/runner.go (assistant) + subagent/spawn.go (sub) + subagent/host.go (sub stop)；BlockStart/Delta/Stop 在 loop/stream.go (text/reasoning/tool_call) + loop/tools.go (tool_result) + subagent/spawn.go (message)。progress block 由 tool Execute 内部 emit（§S18 推流约定，doc 未列具体 producer 位点） | ✓ no orphan |
| Notifications types | doc §11.2 列 6 个：conversation / todo / mcp_server / skill / catalog / sandbox_env。code grep 全部 publish 调用确认全 6 个均有 producer（无 orphan type 字符串） | ✓ no orphan |

## Sub-check

- **Eventlog 协议 events × block types aligned**: **yes**
  - 5 events (`MessageStart` / `MessageStop` / `BlockStart` / `BlockDelta` / `BlockStop`) doc §1 vs code `domain/eventlog/eventlog.go:128-214` 完全一致
  - 6 block types (`text` / `reasoning` / `tool_call` / `tool_result` / `progress` / `message`) doc §2 vs code `eventlog.go:45-79` 完全一致
  - 4 status (`streaming` / `completed` / `error` / `cancelled`) doc §3 vs code `eventlog.go:95-100` 完全一致
  - DB CHECK 约束 6 值 / 4 值 在 `domain/chat/chat.go:116,119` 的 gorm tag 与 doc §9 一致

- **Notifications protocol entity types aligned**: **partial**
  - 6 entity types 数量与名字 doc §11.2 vs code 完全一致（conversation / todo / mcp_server / skill / catalog / sandbox_env）
  - 但 doc §11.2 producer 行号 + 方法名几乎每行都漂移（详 "In doc but not in code (stale)" 表）

- **Producer 位点全 documented**: **partial**
  - Eventlog producer §8 表覆盖了主要位点；但 Tool.Execute 内部经 ctx emitter 推 progress 仅作"惯例"提及（§8 末行），未列具体 tool 位点（acceptable — tool 是开放词表）
  - Notifications producer §11.2 表每个实体类型均有行，但每行的"代码位置"细节不准确（方法名 / 行号大多漂移）

- **§S21 invariants doc 与代码一致**: **yes**
  - doc §10 列出 5 条 invariant：parentId 必先于本事件、block.status / message.status 单向流转、conv 内 seq 全局单调（DB UNIQUE）、deltas append-only、tool_call ID 复用 LLM tc_id
  - code 实现一致：`pkg/eventlog/eventlog.go:289` parentID 回退到 msgID；`infra/eventlog/bridge.go:117-127` UNIQUE seq 由 mu+seq++ 保证；`infra/eventlog/bridge.go:131-141` block-on-slow 保 delta 无 gap；`loop/tools.go:150` tool_result emit 用 `tc.ID` 作 parentID（即 LLM tool-call ID）
  - subagent `spawn.go:249-259` 用 detached ctx 兜底 StopBlock 保 §S21 invariant（doc §10 invariant#1 dangling parentId 是 producer bug 的具体防范）

## Cross-cutting findings

1. **HIGH：notifications SSE wire format 误导**
   doc §11.3 用 `event: <type>` 占位符，强烈暗示 SSE event-name 是动态实体类型；但 code 硬码 `event: notification`，实体类型在 JSON `data.type` 字段。前端按 doc 写的 `EventSource.addEventListener("conversation", ...)` / `addEventListener("todo", ...)` 路由会全部失效，必须改 `addEventListener("notification", ...)` 再解 JSON 派发。这是协议设计层的 doc-code 不一致，需要 doc 改成实际行为或 code 改成 doc 行为（推荐 doc 改：`event: notification` 单 SSE event-name 是合理设计，让前端无需扩展 SSE 路由表）。

2. **MED 群：§11.2 producer 表 5 行行号 + 4 个方法名漂移 + 1 个触发场景描述错**
   `conversation`（3 处漂移：方法名 `SetSystemPrompt` 不存在、line 168 错、`afterStreamFinalize` 不存在）/ `mcp_server`（方法名 `updateStatus,setTools` 都不存在）/ `skill`（fsnotify 触发描述错，实际 1s polling）/ `sandbox_env`（方法名 `publishEnvUpdate` 不存在）这一组 MED 表明 §11.2 表是按"想象中的代码"写的而非按实际代码。这种漂移在"D1 之前的 grep+表对比审计 + 整段重写"之后又出现说明：(a) 重写时 doc 作者没逐条对照代码方法名；(b) 后续 G 删 dead types / fix-A bridge 重构动了某些行号、改了部分方法名（`Publisher.Publish` 变参 → 单参），但 §11.2 表未跟进。建议把 §11.2 改成"按 producer-domain + entity-type 两列写、不写具体方法名 + 行号"——line 漂移是高频维护负担，方法名漂移说明 doc 作者不 grep。

3. **LOW 群：sandbox 兜底不一致 + skill/catalog ID sentinel + History endpoint 响应 shape 未文档化**
   这些都是次要细节，不影响协议正确性。sandbox 的 `if s.notif == nil` guard 工作，noop Publisher fallback 也工作；两种 pattern 共存只是 inconsistency。`skill` 用 `id="*"` 和 `catalog` 用 hash 作 ID 是合理的（这俩实体类型本就是 batch / fingerprint 型而非单 entity-row 型），但 doc §11.2 表格"id"列语义未明示。

4. **doc §10 / §11.3 / §11.4 / §11.5 主体协议描述全部 OK** — 5 events × 6 block types 闭枚举（事件日志，封闭词表）+ 1 envelope × 开放 type 字符串（通知，开放词表）的 dual-protocol 设计与 code 完全一致；Bridge 接口、buffer-on-slow 语义、Last-Event-ID 重连、§S21 invariants 全部对齐。

5. **doc §12 测试覆盖说明 OK** — 提到 `infra/eventlog/bridge_test.go` / `pkg/eventlog/eventlog_test.go` / `handlers/eventlog_test.go` 都存在（本审计未读 _test.go，仅核查文件存在）。

## 总结

- 1 HIGH（notifications SSE event-name 占位符 vs 硬码字面量）
- 6 MED（§11.2 producer 表的方法名 / 行号 / 触发场景描述漂移群）
- 5 LOW（sandbox noop pattern 不一致 + skill/catalog ID 语义未文档化 + History endpoint 响应 shape 未列 + Publisher 第 5 参 "必填" 措辞模糊 + buffer size 在 §11.5 表未明示差异 + todo/catalog 微小行号偏差）
