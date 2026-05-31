# apikey.go — handlers/apikey (231 LOC)

Audit scope: §S3 / §S9 / §S15 / §S16 / §S17.

## Trace 表

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | apikey.go:80-83 | `if err := decodeJSON(r, &req); err != nil { responsehttpapi.FromDomainError(w, h.log, err); return }` | A.4 / A.5 | OK | decodeJSON 内部包装为 `fmt.Errorf("decode body: %w", joinInvalidRequest(err))`——见 site 8 评估。错误透传 errmap，`errors.Is` 链能匹配到 `errorsdomain.ErrInvalidRequest`（已登记 errmap.go:44 → 400 INVALID_REQUEST）。site 1 本身只是 forward 路径，OK。 | — | — | — | — |
| 2 | apikey.go:84-93 | `k, err := h.svc.Create(r.Context(), apikeyapp.CreateInput{...}); if err != nil { responsehttpapi.FromDomainError(w, h.log, err); return }; responsehttpapi.Created(w, k)` | A.2 / A.5 | OK | 单步 CRUD 创建——`r.Context()` 是合理选择（cancel = 客户端不再要这次创建 = drop 是合规语义；apikey 表无 fire-and-forget 后续步骤）。Service 层 sentinel ErrInvalidProvider / ErrBaseURLRequired / ErrAPIFormatRequired / ErrKeyRequired 全已登记 errmap.go:50-53。 | — | — | — | — |
| 3 | apikey.go:101-117 | `func (h *APIKeyHandler) List(...)` | A.2 / A.5 | OK | 纯读路径，r.Context() 合规。pagination + provider filter 透传，apikeydomain.ListFilter 不会产生 sentinel；ctx canceled 已登记 errmap.go:228。 | — | — | — | — |
| 4 | apikey.go:122-138 | `func (h *APIKeyHandler) Update(...)` | A.2 / A.5 | OK | 单步 PATCH。r.Context() OK——cancel = 不写。`ErrNotFound` 已登记 errmap.go:48 → 404。 | — | — | — | — |
| 5 | apikey.go:143-150 | `func (h *APIKeyHandler) Delete(...)` | A.2 / A.5 | OK | 单步 DELETE。r.Context() OK。`ErrNotFound` 已登记。 | — | — | — | — |
| 6 | apikey.go:157-172 | `func (h *APIKeyHandler) postOnID(...) { id, action, found := idAndAction(r, "idAction"); if !found { responsehttpapi.Error(... "NOT_FOUND", "route not found", nil); return }; switch action { case "test": h.test(...); default: responsehttpapi.Error(... fmt.Sprintf("unknown action %q", action), nil) } }` | A.5 | OK | 路由分派 dispatcher。两个 404 是 transport-level 路径"找不到"，handler 直接返 envelope 而非走 sentinel——不在 §S17 错误流（无 sentinel "NotFoundRoute" 之类的概念）。`fmt.Sprintf` 仅渲染 message，不创错误。**注意**：未登记 sentinel 但走 `responsehttpapi.Error` 直接路径合规——同 B1 answers.go site 2 推理（"输入 shape 校验属 transport，sentinel 仅 1 调用点 = boilerplate > 价值"）。 | — | — | — | — |
| 7 | apikey.go:181-210 | `func (h *APIKeyHandler) test(...) { res, err := h.svc.Test(r.Context(), id); if err != nil { responsehttpapi.FromDomainError(...); return }; if !res.OK { responsehttpapi.Error(w, http.StatusUnprocessableEntity, "API_KEY_TEST_FAILED", res.Message, ...); return } ; responsehttpapi.Success(...) }` | A.2 | OK | **关键点**：apikey.Test 内部探测后写 test result（lastUsed / status flags 等）——按 spec-extracts §S9 例子是"前一轮已修"，即 apikeyapp.Service.Test 内部用 detached ctx 写 test result（svc 层职责）。handler 层用 r.Context() 触发，等待结果，仅 transport pure shell——合规。**注意**：res.OK=false 时走 `responsehttpapi.Error` 而不是 sentinel——与 errmap.go:54 已登记的 `ErrTestFailed` 重复但**不冲突**：errmap entry 是为 svc 层返 sentinel 时 fallback 路径准备；handler 显式 envelope 是为附 latencyMs detail（sentinel + errmap 路径不能塞 details map）。**这是合理的设计**——§S6 handler 负责"业务结果转 wire"包括 details 注入。 | — | — | — | — |
| 8 | apikey.go:217-224 | `func decodeJSON(r *http.Request, v any) error { dec := json.NewDecoder(r.Body); dec.DisallowUnknownFields(); if err := dec.Decode(v); err != nil { return fmt.Errorf("decode body: %w", joinInvalidRequest(err)) } ; return nil }` | A.4 | EDGE | §S16 严格说要求 `<pkg>.<Method>:` 前缀，例 `handlers.decodeJSON: decode body: %w`。当前是 `decode body: %w` 缺包前缀。**B1 _summary §S16 cross-cutting note** 已 flag："apikey.go:221 `fmt.Errorf("decode body: %w", ...)` 同样无 `<pkg>.<Method>:` 前缀"——B1 trace 已登记此点为 known issue。**功能上**：`errors.Join(err, errorsdomain.ErrInvalidRequest)` 结合 `%w` wrap，最终 `errors.Is(err, errorsdomain.ErrInvalidRequest)` 仍可匹配（已登记 errmap → 400）。仅 log 中 prefix 缺位置定位。 | LOW | log 中"decode body: <err>"看不出来自哪个 handler；客户端依然 400 INVALID_REQUEST 正确。decodeJSON 是共享 helper，被 chat.go / apikey.go / conversation.go / forge.go / model.go / sandbox.go / answers.go 全部消费——一处修复多处受益。 | 改成 `fmt.Errorf("handlers.decodeJSON: %w", joinInvalidRequest(err))` 或 `fmt.Errorf("transport.decodeJSON: %w", joinInvalidRequest(err))`。1 行变更，0 风险，覆盖所有 handler decodeJSON 路径。 | FIXED (this commit — `handlers.decodeJSON: %w` 前缀) |
| 9 | apikey.go:229-231 | `func joinInvalidRequest(err error) error { return errors.Join(err, errorsdomain.ErrInvalidRequest) }` | A.4 / A.5 | OK | `errors.Join` 是 Go 1.20+ 标准包装方式——把多 err 合成一个，`errors.Is` 能 unwrap 到任一。这里把原 err（json 错）+ sentinel 合并；errmap.lookup 用 `errors.Is(err, ErrInvalidRequest)` 即匹配。**§S16 spec 例 3** 允许"直接返 sentinel（最里层无需 wrap）"——`errors.Join(err, sentinel)` 形式比 spec 表述更新但等价。已登记 errmap.go:44 → 400 INVALID_REQUEST。 | — | — | — | — |

## Sub-checks

A.1 §S3 错误吞没:
  - violations: **not present**
  - 全部 err 路径都走 if-err-return → FromDomainError / Error envelope；无 `_ = err`、无 silent skip、无 `if err != nil { /* nothing */ }`

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: **none in handler scope**
  - 各自 ctx 来源: 全部 r.Context()——Create / Update / Delete 是单步 CRUD（cancel = drop OK）；List 是纯读；Test 内部探测写 test result 由 apikeyapp.Service.Test 内 detached ctx 处理（spec extracts §S9 已注 "前一轮已修"——属 apikey-app audit batch）
  - violations: **N/A: handler 层不做终态写**——所有 detached ctx 责任在 apikeyapp.Service 层

A.3 §S15 ID 生成:
  - ID generation calls: **none in handler**——k.ID 由 apikeyapp.Service.Create 调用 idgenpkg.New("aki") 生成
  - violations: **N/A: handler pure shell 不 mint ID**

A.4 §S16 错误 wrap 格式:
  - violations: **1 EDGE-LOW** site 8 (`fmt.Errorf("decode body: %w", ...)` 缺 `<pkg>.<Method>:` 前缀)
  - 这是共享 helper`decodeJSON` 唯一 wrap 点，影响 7 文件 14+ 调用点；推荐改为 `handlers.decodeJSON: %w` 一处修复全部受益
  - B1 _summary footnote 2 已识别此问题留待本批修复

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined in apikey.go: **0** (handler 不定义 sentinel)
  - apikeydomain sentinels reachable from this handler:
    - ErrNotFound (errmap.go:48 ✓ → 404 API_KEY_NOT_FOUND)
    - ErrNotFoundForProvider (errmap.go:49 ✓ → 404)
    - ErrInvalidProvider (errmap.go:50 ✓ → 400)
    - ErrBaseURLRequired (errmap.go:51 ✓ → 400)
    - ErrAPIFormatRequired (errmap.go:52 ✓ → 400)
    - ErrKeyRequired (errmap.go:53 ✓ → 400)
    - ErrTestFailed (errmap.go:54 ✓ → 422)
    - ErrInvalid (errmap.go:55 ✓ → 401)
  - errorsdomain.ErrInvalidRequest via decodeJSON (errmap.go:44 ✓ → 400)
  - reqctxpkg.ErrMissingUserID via svc layer (errmap.go:185 ✓ → 500)
  - 已登记 errmap: **all 8 apikeydomain sentinels + 2 cross-cutting registered**
  - missing: **none**

## Cross-cutting note

site 8 的 `decodeJSON` 是本包 6+ handler 共享的入口 helper（chat.go / apikey.go / conversation.go / forge.go / model.go / sandbox.go / answers.go 全部经它）。修 §S16 `<pkg>.<Method>:` 前缀**一处见效**，是 handlers-B1 + handlers-B2 跨批最高 ROI 的 1 行修复。
