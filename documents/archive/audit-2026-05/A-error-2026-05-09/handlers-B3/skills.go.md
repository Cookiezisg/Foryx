# Audit trace — `transport/httpapi/handlers/skills.go` (384 LOC)

Phase A audit fork (B3) — §S3 / §S9 / §S15 / §S16 / §S17.
Skill subsystem HTTP transport (per `service-design-documents/skill.md` §11)。9 endpoints — CRUD + body fetch + drag-import + manual rescan + manual invoke。

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | `skills.go:60-62` | `if log == nil { log = zap.NewNop() }` | A.1 | OK | nil-log defense in constructor。`zap.NewNop()` 默认丢弃所有 log——**§S10 例外**：构造期防呆（main.go DI 应保证非 nil，但 testing convenience）。无错误吞。 | N-A | — | — | — |
| 2 | `skills.go:102-105` | `List`: `skills := h.svc.List(r.Context()); responsehttpapi.Success(w, http.StatusOK, skills)` | A.1 | OK | `skillapp.Service.List` 返 `[]Skill` 无 error（in-memory cache snapshot，§S10 同步 getter 设计）。handler 直透传无错误吞风险。 | N-A | — | — | — |
| 3 | `skills.go:110-118` | `Get`: `sk, err := h.svc.Get(r.Context(), name); if err != nil { responsehttpapi.FromDomainError(...) }` | A.1 / A.5 | OK | `ErrSkillNotFound` 经 errmap → 404。standard thin handler。 | N-A | — | — | — |
| 4 | `skills.go:123-131` | `GetBody`: `body, err := h.svc.Body(r.Context(), name); FromDomainError; ... Success(w, ..., {"body": string(body)})` | A.1 / A.5 | OK | 同 site 3。`Body` 是 IO read（disk read），错误透传 errmap（ErrSkillNotFound 路径）。 | N-A | — | — | — |
| 5 | `skills.go:152-169` | `Create`: `if err := decodeJSONLimit(w, r, ..., &req); err != nil { responsehttpapi.Error(w, ..., "INVALID_REQUEST", err.Error(), nil); return }` | A.4 | EDGE | `decodeJSONLimit` 内部返 stdlib `json.Decoder` err 或自创 "request body exceeds size limit" errors.New (site 18)。**§S16 问题**：(a) 无 `<pkg>.<Method>:` prefix；(b) 不经 errmap（直接 inline 字面量 `INVALID_REQUEST`），与 apikey.go decodeJSON 已修 pattern (handlers.decodeJSON: %w + errors.Join → errorsdomain.ErrInvalidRequest) 不一致。EDGE-LOW：风格分裂——helper 函数 decodeJSON vs decodeJSONLimit 两条不同错误路径。 | LOW | dev/curl 看到原 stdlib err 文本（OK 友好但缺 prefix）；客户端 wire code 一致是 INVALID_REQUEST 但不走 errmap unwrap | 重构 decodeJSONLimit 复用 decodeJSON 风格：`return fmt.Errorf("handlers.decodeJSONLimit: %w", joinInvalidRequest(err))` + handler 改用 `responsehttpapi.FromDomainError(w, h.log, err)` | FIXED (decodeJSONLimit refactor, this commit) |
| 6 | `skills.go:158-162` | `if strings.TrimSpace(req.Name) == "" { responsehttpapi.Error(w, ..., "INVALID_REQUEST", "name is required", nil); return }` | A.5 | OK | inline payload-shape 校验，§S6 handler 可校验单字段必填。`INVALID_REQUEST` 字面量与 errmap.go:44 errorsdomain.ErrInvalidRequest 一致 wire code。 | N-A | — | — | — |
| 7 | `skills.go:163-168` | `sk, err := h.svc.Create(r.Context(), req.Name, req.Frontmatter, req.Body); FromDomainError; Created(w, sk)` | A.1 / A.2 / A.5 | EDGE | `Create` 是终态写（写 SKILL.md disk + manifest scan）——属"终态"。`r.Context()` cancel → 半写文件 + 未完成 rescan。**§S9 EDGE**：handler 层用 r.Context() 透传，detached 责任在 skillapp 内部（**`skillapp.Service.Create` audit 跟踪**）。skill 文件系统 IO 比 sandbox install 短得多（几 KB 写），cancel 风险量级小，但严格 §S9 仍 EDGE。 | LOW | 用户提交 Create 后立即关 tab → SKILL.md 可能写入但 rescan 未跑（下次 List 不可见）；下次 manual Refresh 兜底 | skillapp audit batch 验证 Create 内部 detached ctx 兜终态——若不是，handler 层加 detached ctx 包装 | FOUND |
| 8 | `skills.go:175-188` | `Replace`: 同 Create 结构，调 `h.svc.Replace(r.Context(), name, req.Frontmatter, req.Body)` | A.1 / A.2 | EDGE | 同 site 7：Replace 是终态写（覆写 SKILL.md + rescan）。同 EDGE-LOW 评估——skillapp.Replace 内部 detached ctx 责任。 | LOW | 同 site 7 | 同 site 7 | FOUND |
| 9 | `skills.go:193-200` | `Delete`: `if err := h.svc.Delete(r.Context(), name); err != nil { FromDomainError; return }; NoContent(w)` | A.1 / A.2 | EDGE | 同 site 7：Delete 是终态写（rm SKILL.md + rescan）。**EDGE-LOW**——同模式。 | LOW | 同 site 7 | 同 site 7 | FOUND |
| 10 | `skills.go:209-215` | `Refresh`: `if err := h.svc.Scan(r.Context()); err != nil { FromDomainError; return }; Success(w, ..., h.svc.List(r.Context()))` | A.1 | OK | `Scan` 全量重扫 disk → manifest，错误经 errmap。**任务描述担忧"scan err 是否吞？"**：实际**不吞**——err 走 FromDomainError 路径暴露给客户端。**注**：skillapp.Scan 内部对单 skill parse fail 是否吞（`scan.go:153` `read SKILL.md` / `scan.go:156` `body too large`）属 skillapp audit 范畴；handler 层透传 batch-level error 是合规的 §S6 行为。 | N-A | — | — | — |
| 11 | `skills.go:247-318` | `Import` overall — multipart + JSON 双路径 | A.1 / A.4 | EDGE | 13+ 错误路径，inline 字面量 wire code。逐条评估：(a) 252-257 multipart parse fail → INVALID_REQUEST + err.Error() (同 site 5 EDGE)；(b) 264-268 empty file part → INVALID_REQUEST 字面量 (同 site 6 OK)；(c) 272-276 fh.Open err → INVALID_REQUEST + err.Error() (**§S16 fault**：`open part <name>: <stdlib err>` 缺 §S16 prefix);(d) 285-288 `for { ... if rerr != nil { break } }` —— **§S3 关键关注点**：multipart 文件读取，所有 read 错误 (含 io.EOF + non-EOF) 一律 break 不区分。io.EOF 是合法终止，**non-EOF（disk fail / connection drop）也被 silent break**——这是 §S3 §1 violation："silent fallback：upstream 失败后悄悄走 plan B 不告诉调用方"。**HIGH 候选**——但实际影响**有限**：read 完后 buf 内容已塞进 files，service 层会 parse 出 invalid frontmatter 报错（兜底）。**MED**：silent skip non-EOF io error，调试时丢线索。 | MED | upload 时 disk fail / network drop 中断读，handler 静默把"已读到的截断 buf"当合法 SKILL.md 入 service；service parse fail 时报"frontmatter invalid" 误导调试方向 | 区分 io.EOF vs 真错：`if rerr == io.EOF { break } else if rerr != nil { responsehttpapi.Error(... "INVALID_REQUEST", "read part "+fh.Filename+": "+rerr.Error(), nil); return }`；或换用 `io.ReadAll(f)` 一行替代手卷 loop（更安全 + 简洁） | FIXED (this commit — io.ReadAll 替代手卷 loop，non-EOF io err 显式 400 + open part err 加 `handlers.Import:` 前缀) |
| 11b | `skills.go:289` | `f.Close()` — multipart Open() 配对 | A.1 | EDGE | `defer f.Close()` 是只读路径常见模式，但本处是 inline 同步调用——`Close()` 错误被丢弃。**§S3 例外**：只读 path Close 错对调用方无意义。但这里**不是 defer**，是 inline 调用——风格不一致 + 漏写 defer 导致 panic 路径不 Close (loop body 内 `responsehttpapi.Error` 没 return 前已 close 完，但路径 fragile)。EDGE-LOW。 | LOW | 边缘 panic 路径漏 close——但 multipart File 是 in-memory tempfile，GC 后 finalize 兜底；实际无 leak | 改为 `defer f.Close()` 紧跟 Open；或显式注释 `// _ = err — close after read on multipart in-memory tempfile, GC finalize covers panic path` | FOUND |
| 11c | `skills.go:294-296` | JSON branch: `if err := decodeJSONLimit(w, r, ..., &req); err != nil { responsehttpapi.Error(w, ..., "INVALID_REQUEST", err.Error(), nil); return }` | A.4 | EDGE | 同 site 5 (decodeJSONLimit %w prefix 缺) | LOW | 同 site 5 | 同 site 5 | FIXED (decodeJSONLimit refactor, this commit) |
| 11d | `skills.go:298-302` | `if len(req.Files) == 0 { Error(... "no files in payload (expect {files:[{name,body}]})") }` | A.5 | OK | inline payload-shape 校验，wire code 一致。 | N-A | — | — | — |
| 11e | `skills.go:308-316` | `res, err := h.svc.Import(r.Context(), files, overwrite); if err != nil { FromDomainError(...) }; Success(w, ..., res)` | A.1 / A.2 / A.5 | EDGE | Import 是终态写（多文件入 disk + 全量 rescan）。同 site 7 EDGE-LOW。**重要**：注释 309-312 已说明 res.Errors 携带 per-file 错误（不冒泡 wrap）——**这是合规的"部分成功"设计**（不同 sandbox.go convEnvsAction site 15 的"立即返第一个错"），客户端能收完整结果。**§S3 模范**——错误"显式入 wire body" 不吞。 | LOW | 同 site 7（终态写 cancel 风险）；res.Errors 设计本身合规 | 同 site 7 | FOUND |
| 12 | `skills.go:336-364` | `NameAction`: `name, action := splitAction(...); if name == "" || action == "" { ... INVALID_REQUEST ... }; switch action { case "invoke": ... default: INVALID_REQUEST }` | A.5 | OK | dispatch table。inline 字面量 wire code 全部 INVALID_REQUEST，无新词汇引入。 | N-A | — | — | — |
| 13 | `skills.go:348-353` | `if r.ContentLength > 0 { if err := decodeJSONLimit(...); err != nil { ... INVALID_REQUEST + err.Error() } }` | A.4 | EDGE | 同 site 5 (decodeJSONLimit %w prefix 缺) | LOW | 同 site 5 | 同 site 5 | FIXED (decodeJSONLimit refactor, this commit) |
| 14 | `skills.go:354-358` | `out, err := h.svc.Activate(r.Context(), name, req.Arguments); if err != nil { FromDomainError; return }; Success(... {"result": out})` | A.1 / A.2 / A.5 | EDGE | `Activate` 是 skill 主入口——可能 spawn subagent (注释 activate.go:104-109 显示 fork 路径)。**§S9 关注**：subagent spawn 是 long-running，handler 层 r.Context() 透传给 service。**EDGE-LOW**：同 site 7 模式。**§S17**：Activate 路径 sentinel 是 ErrSkillNotFound (已登记) + ErrBodyTooLarge (已登记) + 其他 skillapp 内部 wrap 错（兜底 INTERNAL_ERROR 500）。**注**：activate.go:104 `"fork requested but SubagentService is nil"` 是无 sentinel 内部错——会触发 errmap 的"unmapped domain error"日志告警。**§S17 边界**：本不是 sentinel 路径设计的，但产线触达会污染日志。 | LOW | Activate 内部错（如 SubagentService nil）→ 客户端看 500 INTERNAL_ERROR，后端日志被告警 | activate.go 内部应用 errorsdomain.ErrInternal 包装内部状态错避免日志告警；或加 skillapp.ErrSubagentUnavailable sentinel + errmap 行 | FOUND |
| 15 | `skills.go:373-384` | `decodeJSONLimit`: `body := http.MaxBytesReader(w, r.Body, maxBytes); dec := json.NewDecoder(body); ... if errors.As(err, &maxBytesError) { return errors.New("request body exceeds size limit") } return err` | A.4 | EDGE | **§S16 问题集中**：(a) 触发 MaxBytesError 时返**裸 errors.New**——sentinel chain 断（无原 err 原因）；(b) 非 MaxBytes err 直 `return err`——透传 stdlib err 到 handler，无 wrap 也无 prefix；(c) **不返 errorsdomain.ErrInvalidRequest sentinel**，无法走 errmap → handler 必须 inline `INVALID_REQUEST` 字面量（解释 site 5/11c/13 的 inline 模式）。**对比 apikey.go decodeJSON (B2 已修)**：`return fmt.Errorf("handlers.decodeJSON: %w", joinInvalidRequest(err))`——一致 helper 应用一致 pattern。**EDGE-LOW**：本 helper 行为正确（wire 客户端看到 400 + msg），但 §S16 prefix + sentinel chain 缺。 | LOW | 客户端 wire 行为 OK；后端 errors.Is 链断（裸 errors.New），调试时无法 unwrap 到 stdlib err | 重构成：`return fmt.Errorf("handlers.decodeJSONLimit: request body exceeds size limit (%d bytes): %w", maxBytes, joinInvalidRequest(err))` (limit case); `return fmt.Errorf("handlers.decodeJSONLimit: %w", joinInvalidRequest(err))` (other) | FIXED (this commit — `handlers.decodeJSONLimit:` 前缀 + joinInvalidRequest sentinel chain；4 调用点改 FromDomainError) |
| 16 | `skills.go:264-268` | `if len(fhs) == 0 { responsehttpapi.Error(w, ..., "INVALID_REQUEST", "no 'file' parts found in multipart payload", nil); return }` | A.5 | OK | inline payload-shape 校验，§S6 合规。 | N-A | — | — | — |

## Sub-check 模板

A.1 §S3 错误吞没:
  - violations: site 11 (multipart read loop 静默 break non-EOF io error — **MED**)
  - EDGE-LOW: site 11b (inline f.Close() 错丢弃，不是 defer)
  - 备注：site 11 是本 batch 中**唯一非 EDGE 的 §S3 violation candidate**——multipart 文件读取 loop 不区分 io.EOF 与真 io 错，所有 rerr != nil 都 break。disk fail / connection drop 时 silent fallback 把截断 buf 当合法 SKILL.md 入 service。MED severity（影响有限因 service 层 frontmatter parse 兜底报错，但调试丢线索）。

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: site 7 (Create), site 8 (Replace), site 9 (Delete), site 11e (Import), site 14 (Activate fork)
  - 各自 ctx 来源: 全部 `r.Context()` 直传 service
  - violations: **N/A at handler layer; EDGE-FLAG 转 skillapp audit batch**——同 sandbox.go pattern：handler 层 r.Context() 透传是合理的中间层选择，detached ctx 责任在 `skillapp.Service.Create/Replace/Delete/Import/Activate` 内部。**B3 fork 不审 app 层**。skill 文件系统 IO 比 sandbox install 短得多（几 KB 写），cancel 风险量级显著较小，但严格 §S9 跟 sandbox 同模式。

A.3 §S15 ID 生成:
  - ID generation calls: 0
  - violations: N/A — skill 主键是 `name` (slug 字符串，非 prefix_hex)，不走 idgenpkg pattern。skill.md §11 设计选择——skill 用人类可读 name（"foo-bar-baz"）作 stable ID，方便 SKILL.md 路径 + UI 展示。这是 §S15 例外（域设计明示，non-business-id 性质的标识符）。

A.4 §S16 错误 wrap 格式:
  - violations: site 15 (decodeJSONLimit 裸 errors.New + 透传 stdlib err 无 prefix), 衍生 sites 5/11c/13 (handler 处 inline `err.Error()` 入 envelope detail，未走 errmap)
  - 备注：**单点修复（site 15）解决衍生 sites**——decodeJSONLimit 重构为 `handlers.decodeJSONLimit: %w` + joinInvalidRequest，handler 改用 FromDomainError，5 处 inline INVALID_REQUEST 全部消除。**HIGH-ROI** fix 候选（同 B2 已修 decodeJSON pattern 的延伸）。
  - site 11 path 的"open part" 错也属此类（line 274）：`"open part "+fh.Filename+": "+err.Error()` 直拼 stdlib err 无 §S16 prefix。

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: 本文件 0；消费的是 skilldomain 5 个 sentinel
  - 已登记 errmap (5/5):
    * `ErrSkillNotFound` errmap.go:167
    * `ErrInvalidFrontmatter` errmap.go:168
    * `ErrBodyTooLarge` errmap.go:169
    * `ErrNameConflict` errmap.go:170
    * `ErrInvalidName` errmap.go:171
  - missing: none — skilldomain sentinel 全登记
  - 备注：site 14 标识的潜在 unmapped path (`activate.go:104` "fork requested but SubagentService is nil") 不是 sentinel——属"内部状态错"无 sentinel 包装，会触发"unmapped domain error" 警告。**严格 §S17 不算违规**（不是 sentinel），但产品级建议同步登记 `errorsdomain.ErrInternal` 或加 `skillapp.ErrSubagentUnavailable` 抑制告警。**LOW**。

## Cross-cutting

1. **decodeJSONLimit vs decodeJSON 风格分裂 (HIGH-ROI)**：本文件 decodeJSONLimit (site 15) 与 apikey.go decodeJSON (B2 已修) 是**两个独立 helper 走两条不同错误路径**。建议合并：
    - 重构 decodeJSONLimit 复用 decodeJSON 的 §S16 prefix + joinInvalidRequest pattern
    - handler 5 处调用 `responsehttpapi.Error(w, ..., "INVALID_REQUEST", err.Error(), nil)` 改用 `responsehttpapi.FromDomainError(w, h.log, err)`
    - 消除 5 处 inline 字面量 wire code
    单点修复影响 5+ handler 路径。**HIGH-ROI**——B2 _summary 已识别 decodeJSON 是 cross-cutting，本文件是延伸。

2. **multipart 读取的 §S3 violation (MED)**：site 11 silent break non-EOF io err 是本 B3 batch **唯一非 EDGE 的 §S3 violation 候选**。修复成本低（1 个 io.EOF 区分 + 用 io.ReadAll 替代手卷 loop），影响是调试线索保留。**MED 优先级**——上下游链路（service frontmatter parse）兜底掩盖症状，但严格 §S3 §1 违规（silent fallback 不告诉调用方）。

3. **handler 终态写跨 5 个端点 (sandbox + skill 同模式)**：sandbox.go 6 个、skills.go 5 个端点全部 r.Context() 透传 service——detached ctx 责任**统一**在 app 层。这是 §S9 在 transport 层的**约定姿态**，B3 fork 边界明确。**Pattern 总结**：handler 不预设 detached（除非业务明示需"用户 cancel 不可逆终态"），由 service 内部决定。

4. **skill name 不走 §S15 prefix_hex 是设计选择**：skill 用人类可读 name 作主键（`foo-bar-baz`），不同于 forge/conv/msg 的 idgenpkg.New("f")/("cv")/("msg")。**§S15 例外有 domain doc 支撑**（skill.md §11 / 文件系统路径友好）。**OK**——但建议在 skill.md 显式注 §S15 例外避免后续审查误判。

5. **`zap.NewNop()` constructor defense (site 1)**：本文件 NewSkillsHandler 唯一含此 defense 的 handler——其他 handler 都假定 main.go DI 给非 nil log。**§S10 不一致**但合规（防呆例外）。可统一选 stance（要么全要 nil-defense 要么全靠 DI 契约）。**LOW**——非本审范围。

6. **handler 层 §S6 总体**：9 endpoint 中 List/Get/GetBody/Refresh/Delete/NameAction 全部薄；Create/Replace 是 thin + 1-line 校验；Import 是**最厚** (~70 LOC)——multipart 处理 + JSON 双路径。**业务逻辑**(`name = strings.TrimSuffix(...)` / loop read / file-list construction) 在 handler 层略多——但 multipart 文件解析是天然 transport 层职责（§S6 例外明示），合规。
