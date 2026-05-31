# Phase A — 错误处理 + 数据完整性 — 规范摘录（fork 必读）

只看本文件即可判违规，不必再读 CLAUDE.md 全文。摘自 CLAUDE.md §S3 / §S9 / §S15 / §S16 / §S17。

---

## §S3 错误不吞（高风险）

**规则**：`_` 忽略必须带注释说明原因；**严禁用"静默跳过"掩盖失败**——若某功能在当前环境不可用，必须让调用者看到错误或在文档/启动日志里明确说明，而不是用 `_ = err` 藏起来。

**典型违规模式**：

1. `_ = err` / `_ = func()` 没注释为什么忽略
2. `value, _ := func()` — 多返回值丢弃命名 err 之外的 err
3. `if err != nil { return nil }` — 静默把错误转成成功
4. `if err != nil { /* nothing */ }` — 完全忽略
5. `defer X.Close()` 吞错（Close 返 error 但被丢）—— 注意：`defer` 没法直接 return err，需 `defer func() { if err := X.Close(); err != nil { ... } }()` 或 named return
6. silent fallback：upstream 失败后悄悄走 plan B 不告诉调用方
7. 把 error 变 zero value 返回（`return "", err` 路径但没 wrap）

**例外（不算违规）**：

- `_ = err` **带行内注释**说明为什么吞（如：`// _ = err — close-on-shutdown，不可恢复`）
- `defer f.Close()` 在只读路径（Close 返错对调用方无意义）
- panic 路径里的 cleanup

**判定关键**：如果一个 error 会导致**用户看到错误状态 / 数据丢失 / 配置失效**，那就不能吞。如果只是"清理资源失败"且不影响业务，吞 OK。

---

## §S9 detached context 终态写

**规则**：每个跨层调用传 `ctx`。**例外**：终态写入（必须落库的最后一步）必须用 `reqctxpkg.SetUserID(context.Background(), uid)` 创建 detached context——否则上游 cancel 会让终态写失败。

**判定关键**：

- "终态" = 一个用户操作的**结果**必须落库，否则用户看到陈旧 / 错误的状态。
- 例：apikey.Test 探测后写 test result（前一轮已修）；chat 流被 cancel 后写 assistant final message；某操作完成后写 audit log。
- 反例：纯查询、log 写 zap、cache 更新——不算终态。

**典型违规**：

- 终态 `s.repo.Update*(ctx, ...)` 用 `r.Context()`（含 r.Context() 派生的 ctx）
- 一个长跑流程 mid-stream 已经 ctx canceled，但完成时还要写"流被取消"事件——还用旧 ctx 必然 fail

**例外（OK）**：

- 操作中间步骤的写入（不是"必须落库否则丢失"）
- background 任务本身就是 detached（不在 request context）

**判定关键**：识别"终态写"——写完这一步整个用户操作才算落地。如果对应 ctx 是 r.Context() 或派生，可能违规；要看 cancel 风险。

---

## §S15 ID 生成

**规则**：业务 ID 一律 `<prefix>_<16hex>` 格式（前缀按 domain 取：`aki_` apikey / `mc_` model config / `cv_` conversation / `msg_` message / `f_` forge / 等）；8 字节从 `crypto/rand` 取，**`rand.Read` 失败必须 panic**。所有 `newID()` 函数遵守此格式（实现统一在 `pkg/idgen.New(prefix)`）。

**典型违规**：

- 自写 ID 生成不调 `idgen.New`
- 用非 `crypto/rand`（如 `math/rand`、`time.Now().UnixNano()`）
- `rand.Read` 失败时返 zero value / 重试 / log 但继续——必须 panic

**判定关键**：

- 该包是否生成业务 ID？查 `idgen.New(...)` 调用 + 自写 hex generation
- 如果调 `idgen.New`，本身已经 panic on fail（pkg/idgen 实现层管），调用方不再重复检查
- 如果**不**调 `idgen.New`，就要手动看 `crypto/rand.Read` 是否 panic

---

## §S16 错误包装格式

**规则**：上抛错误用 `fmt.Errorf("<pkg>.<Method>: %w", err)`，sentinel 在最里层。例：`apikeystore.List: missing user id in context`。

**禁止**：

- 裸 `errors.New` 套娃丢失原 sentinel：`return errors.New("xxx: " + err.Error())` ❌
- 自创新前缀代替 `%w` 包装：`return fmt.Errorf("xxx: %v", err)` ❌（`%v` 不能 unwrap）
- 不带 `<pkg>.<Method>:` 前缀：`return fmt.Errorf("%w", err)` ❌（无定位上下文）

**正确**：

```go
return fmt.Errorf("apikeystore.List: %w", err)         // ✓ 有前缀 + %w
return fmt.Errorf("apikey.Service.Test: tester: %w", err)  // ✓ 多段前缀
return apikeydomain.ErrNotFound                        // ✓ 直接返 sentinel（最里层无需 wrap）
```

**判定关键**：

- 看 `fmt.Errorf` 调用的格式串：必含 `<pkg>.<Method>:` 前缀 + 必含 `%w`（不是 `%v` / `%s`）
- 看 `errors.New` 调用：是否在拼别人的 err.Error()——如果是，违规
- `errors.Is` 路径：必须能 unwrap 到最里层 sentinel——如果中间某层用 `%v`，链就断了

---

## §S17 errmap 单一事实源

**规则**：每个会到达 handler 的 sentinel 必须登记到 `transport/httpapi/response/errmap.go::errTable`——**包括** `pkg/` 和 `infra/` 中跨层使用的（如 `reqctxpkg.ErrMissingUserID` / `cryptoinfra.ErrUnsupportedVersion`）。未登记的 sentinel 会触发"unmapped domain error" ERROR 日志，污染烟雾报警。

**判定流程（每个包）**：

1. 找该包定义的所有 sentinel：`var Err... = errors.New(...)` / `errors.New(...)` 在 var 声明
2. 找该包定义的 sentinel 是否在某 handler 路径上有可能冒泡到 errmap.FromDomainError——如果是，必须登记
3. 反之，**完全包内 / 跨包但只在 service 层消费、handler 层翻译成别的 sentinel** 的，不需要登记
4. 已登记的查 `errmap.go::errTable` map 里的 key

**典型违规**：

- 新加 sentinel 但没加 errmap.go 行
- sentinel 在 pkg/ 里 + handler 路径上会到 errmap，但 errmap 没有它

**判定关键**：

- 看 var Err...
- handler 是否会调到该 service？看依赖
- handler 是否调用 `responsehttpapi.FromDomainError(w, h.log, err)`？如果是，所有 err 都该有 errmap 行

---

# Sub-check 模板（每个 audit 文件末尾必填）

```
A.1 §S3 错误吞没:
  - violations: [list site#] OR "not present"
A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: [site#]
  - 各自 ctx 来源: [list]
  - violations: [list] OR "not present" OR "N/A: package doesn't do terminal writes (reason)"
A.3 §S15 ID 生成:
  - ID generation calls: [list of idgen.New(...) or self-rand]
  - violations: [list] OR "N/A: package doesn't generate business IDs"
A.4 §S16 错误 wrap 格式:
  - violations: [list] OR "not present"
A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: [list var Err...]
  - 已登记 errmap: [list with errmap.go:line]
  - missing: [list] OR "all registered" OR "N/A: file defines no sentinels"
```

每条必须显式，不许 silence。`N/A` 必须写 reason。

---

# 9 列 Trace 表 schema

```
| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix |
```

- `site#`: 文件内连号
- `file:line`: 例 `apikey.go:178`
- `snippet`: 该行 + 上下 2-3 行 context（用 inline 反引号或简短代码块）
- `category`: A.1 / A.2 / A.3 / A.4 / A.5（多类用斜杠隔开）
- `classification`: `OK` / `POST-FIX OK` / `VIOLATION` / `EDGE`
- `reasoning`: 必填，引用本文件 §SX 子项
- `severity`: `HIGH` / `MED` / `LOW` / `N-A`
- `user_impact`: 一行话，OK 填 `—`
- `suggested_fix`: 短描述，OK 填 `—`

每行末尾追加一个 status 列由修复阶段填（FOUND / FIXED / DEFERRED / WAIVED）——audit 阶段统一填 `FOUND` 待修；OK / POST-FIX 填 `—`。
