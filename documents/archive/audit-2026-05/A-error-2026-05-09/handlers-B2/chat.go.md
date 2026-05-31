# chat.go — handlers/chat (148 LOC)

Audit scope: §S3 / §S9 / §S15 / §S16 / §S17.

## Trace 表

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | chat.go:55-58 | `if err := r.ParseMultipartForm(chatdomain.MaxAttachmentBytes); err != nil { responsehttpapi.FromDomainError(w, h.log, fmt.Errorf("%w: %v", chatdomain.ErrAttachmentTooLarge, err)) ; return }` | A.4 / A.5 | EDGE | §S16 wrap 必须 `<pkg>.<Method>:` 前缀。这里用 `%w: %v` 把 sentinel 接到内层、把 detail 用 `%v` 拼上下文。但 §S16 要求**前缀 + %w**，应为 `fmt.Errorf("chathandler.UploadAttachment: %w (parse multipart: %v)", chatdomain.ErrAttachmentTooLarge, err)` 或直接 `responsehttpapi.FromDomainError(w, h.log, chatdomain.ErrAttachmentTooLarge)`. 现状仍可 unwrap（`%w` 链不断），且 sentinel 已登记 errmap → 实际 `errors.Is` + errmap 翻译生效；缺位置前缀仅影响 log 可读性。**注意 §S16 spec 例 3** 也允许"直接返 sentinel（最里层无需 wrap）"——本现状属于 ad-hoc 形式但未严格 pkg.Method 前缀。 | LOW | log 中"unmapped domain error"或后续 trace 缺位置定位（实际 sentinel 登记了不会触发）。客户端无影响——errmap 仍正确映射 413 ATTACHMENT_TOO_LARGE。 | 改成 `fmt.Errorf("handlers.UploadAttachment: %w", chatdomain.ErrAttachmentTooLarge)` 丢弃 detail，或直接 `responsehttpapi.FromDomainError(w, h.log, chatdomain.ErrAttachmentTooLarge)`。原 detail (`err.Error()`) 在浏览器看不到（msg 由 errmap 决定），仅 log 损失。 | FIXED (this commit — 加 `handlers.UploadAttachment:` 前缀；site 4 同时保留原 io err 经 `%v` 注解) |
| 2 | chat.go:60-62 | `file, header, err := r.FormFile("file"); if err != nil { responsehttpapi.FromDomainError(w, h.log, fmt.Errorf("%w: missing file field", chatdomain.ErrAttachmentParseFailed)) ; return }` | A.4 | EDGE | 同 site 1：`%w: <静态字符串>` 形式，缺 `<pkg>.<Method>:` 前缀。但 sentinel 登记了，`errors.Is` 链通，errmap → 422 ATTACHMENT_PARSE_FAILED 正常。"missing file field" 是死字符串而非 wrap 别人 err，纯文本注解。 | LOW | 同 site 1。 | 同 site 1。 | FIXED (this commit — 加 `handlers.UploadAttachment:` 前缀；site 4 同时保留原 io err 经 `%v` 注解) |
| 3 | chat.go:64 | `defer file.Close()` | A.1 | OK | §S3 例外：`defer f.Close()` 在只读路径（Close 返错对调用方无意义）。multipart file 是上传读取，Close 失败不影响业务。 | — | — | — | — |
| 4 | chat.go:66-69 | `data, err := io.ReadAll(file); if err != nil { responsehttpapi.FromDomainError(w, h.log, fmt.Errorf("%w: read failed", chatdomain.ErrAttachmentParseFailed)); return }` | A.4 | EDGE | 同 site 1/2：`%w: <static>` 形式缺 pkg.Method 前缀。`err` 内容（IO 错误）丢失——只 wrap sentinel 不带原 err。这里 `err` 是 io.ReadAll 错（disk 满 / 客户端 conn drop / etc），原 err 不在链中。**§S16 严格说**：`return fmt.Errorf("%w", err)` 不带前缀也算违规——这里更糟，原 err 干脆不在链中。 | LOW | 调试 IO 失败时 log 没线索；客户端依然看 422 ATTACHMENT_PARSE_FAILED。 | `fmt.Errorf("handlers.UploadAttachment: read failed: %w", err)` + 让 errmap 通过 `%w` 链匹配——但 errmap 里没注册 io 错误 sentinel，会 fallback 到 INTERNAL 500。**或**两层 wrap：`return errors.Join(chatdomain.ErrAttachmentParseFailed, fmt.Errorf("handlers.UploadAttachment: read failed: %w", err))`。最简单：保留 sentinel 走 422 + zap.Error(err) 单独打一行 log。**EDGE-LOW** 因实际 errmap 翻译生效，UX 正确。 | FIXED (this commit — 加 `handlers.UploadAttachment:` 前缀；site 4 同时保留原 io err 经 `%v` 注解) |
| 5 | chat.go:77 | `att, err := h.svc.UploadAttachment(r.Context(), data, mimeType, header.Filename)` | A.2 | OK | upload 是单步落盘 —— `r.Context()` cancel = 浏览器 disconnect = "用户不要这次 upload"。**不**是 §S9 "终态写"——cancel = drop 是正确语义；attachment 表记录是 upload 流程的中间步骤、不是必须落地的最后一步。 | — | — | — | — |
| 6 | chat.go:96-105 | `id := r.PathValue("id"); var req sendMessageRequest; if err := decodeJSON(r, &req); ...; msgID, err := h.svc.Send(r.Context(), id, ...)` | A.2 | OK | SendMessage POST 体本身**仅触发** chat stream（service 内 fire-and-forget 起 goroutine 跑 LLM）；handler 立刻返 messageId。**§S9 detached ctx 的"终态写"** 指 stream goroutine 内部完成时写 assistant final message——那不在本 handler 范围，归 chatapp.Service 内部 audit。本 handler 只是触发器，r.Context() 完全适合（持续到 service 起完 goroutine 并返 ID 即可）。 | — | — | — | — |
| 7 | chat.go:110 | `responsehttpapi.Success(w, http.StatusAccepted, map[string]string{"messageId": msgID})` | A.3 | OK | `msgID` 来自 svc.Send 返回 —— ID 由 chatapp.Service 内 idgenpkg.New("msg") 生成。handler 不 mint ID。§S15 N/A in handler scope. | — | — | — | — |
| 8 | chat.go:120 | `if err := h.svc.Cancel(r.Context(), id); err != nil { responsehttpapi.FromDomainError(w, h.log, err); return }` | A.2 / A.5 | OK | Cancel 是用户主动操作；r.Context() 合适（cancel == drop OK）。Cancel 内部如果有"写 assistant final cancelled"那是 service 内部职责，handler scope 不参与。chatdomain sentinel `ErrStreamNotFound` / `ErrStreamInProgress` 全已登记 errmap.go:62-63。 | — | — | — | — |
| 9 | chat.go:139-142 | `items, next, err := h.svc.ListMessages(r.Context(), id, chatdomain.ListFilter{...})` | A.2 | OK | 纯读路径，r.Context() 是 §S9 推荐项（cancel = "客户端不再要" = stop reading early）。无终态写。 | — | — | — | — |

## Sub-checks

A.1 §S3 错误吞没:
  - violations: not present
  - 1 处 `defer file.Close()` 是 §S3 例外（只读 close）

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: **none in handler scope**
  - 各自 ctx 来源: handler 全部用 `r.Context()`——SendMessage 是 stream 触发器（异步 goroutine 在 chatapp 内启动），Cancel 是用户取消请求，UploadAttachment 是单步 upload，ListMessages 是读路径
  - violations: **N/A: handler 层不做终态写**——chat stream 终态（取消后写 assistant final message）由 chatapp.Service 内 fire-and-forget goroutine 用 detached ctx 写，归 chat-app audit batch

A.3 §S15 ID 生成:
  - ID generation calls: **none in handler**——msgID 来自 svc.Send 返回，attID 来自 svc.UploadAttachment 返回
  - violations: **N/A: handler 是 transport pure shell，不 mint ID**

A.4 §S16 错误 wrap 格式:
  - violations: **3 EDGE-LOW** sites 1/2/4——`%w: <text>` 形式缺 §S16 要求的 `<pkg>.<Method>:` 前缀；site 4 还把原 IO err 丢出链外（仅留 sentinel + 静态字符串）。所有 3 处的 sentinel 已登记 errmap，客户端 wire 行为正确。
  - 推荐统一：site 1/2 改成 `fmt.Errorf("handlers.UploadAttachment: %w", chatdomain.ErrXxx)` 或直接传 sentinel；site 4 改成保留原 err 通过 `errors.Join` 或单独 zap.Error 配 sentinel 路径

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: **0 in chat.go itself**（handler 不定义 sentinel，仅消费 chatdomain）
  - chatdomain sentinels reachable from this handler: ErrAttachmentTooLarge / ErrAttachmentParseFailed / ErrAttachmentTypeUnsupported / ErrVisionNotSupported / ErrMessageNotFound / ErrStreamNotFound / ErrStreamInProgress / ErrProviderUnavailable
  - 已登记 errmap: errmap.go:61-68（全部 8 个 chatdomain sentinel）+ errmap.go:44 errorsdomain.ErrInvalidRequest（decodeJSON 路径）
  - missing: **all registered**
  - 备注：chatdomain.ErrBlockNotFound 仅在 infra/store/chat 层使用（chatstore.UpdateBlock / GetBlock 返），消费者是 chatapp 内 stream-writer，不冒泡到 handler——按 §S17 流程 step 3 "完全包内 / handler 层翻译" 不需登记。已确认 grep 范围 (`ErrBlockNotFound`) 只命中 store + domain 文件，不进 handler 路径。
