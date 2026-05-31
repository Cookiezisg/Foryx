# dev_info.go — handlers/dev_info (161 LOC, dev-only)

Audit scope: §S3 / §S9 / §S15 / §S16 / §S17.

## Trace 表

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | dev_info.go:33 | `home, _ := os.UserHomeDir()` | A.1 | EDGE | §S3 "value, _ := func() — 多返回值丢弃命名 err 之外的 err" 典型违规模式 #2。**但**：dev/info 端点是元数据快照，UserHomeDir 失败时 `home==""` 在 JSON 中显示为空字符串——dev 测试者一眼能看出"home 没拿到"，且后续不依赖此值（只放进 response map）。dev-only + 显式向 UI 暴露 = 不算掩盖失败。**§S3 例外判定**：错误"对调用方无意义"——home 显示空、UI 自然反馈、tester 自查环境。**但严格说**应加注释 `_` 含义，否则下一个 audit 又来 flag。 | LOW | dev-only。home 在 response 显示空字符串时 tester 看 forgifyHome / mcpConfigPath 等字段（即便 home 为空仍工作，后者来自 h.forgifyHome 注入）。 | 加 inline 注释：`home, _ := os.UserHomeDir() // dev-info: 拿不到就空字符串，UI 直接显示无歧义` 或显式忽略：`home, errHome := os.UserHomeDir(); if errHome != nil { home = "" }` 配 inline 注释。 | FIXED-doc (this commit — 加 dev-only graceful inline 注释) |
| 2 | dev_info.go:78-94 | `tree, err := walkHomeTree(root, "", 0, 4, &count, 500); if err != nil { if os.IsNotExist(err) { ... return } ; responsehttpapi.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "walk failed: "+err.Error(), nil) ; return }` | A.4 | EDGE | §S16 严格说要求 `<pkg>.<Method>:` 前缀 + `%w`。这里 handler 用 `responsehttpapi.Error` 直传 string 字段（不走 sentinel + errmap），把 err 拼进 message。**两点**：(1) 不走 errmap 路径所以 §S17 不约束；(2) §S16 wrap 规则 spec 重点在"上抛错误"（layer-to-layer），而 handler 直接写 envelope 是 terminal——但**该 string 拼接相当于 fallback 路径**，原 err 非 sentinel 非 unwrap-able。**风险**：err 文本可能含磁盘路径等内部细节，errmap.go:233-237 的 "internal error" 隐藏路径在这里被跳过——**dev-only** 端点暴露内部细节是设计选择（tester 需要看到原因）。 | LOW | dev-only：tester 看 raw error 是设计目的；prod 不暴露此端点（用 dev-mode flag 控制）。无信息泄漏风险。 | 加 inline 注释钉住"dev-only 故意暴露 error.Error()" 含义；或如果想严格走 errmap，定义 dev sentinel `ErrDevWalkFailed` 但 boilerplate > 价值。**EDGE-LOW**——保留现状 + 1 行注释。 | WAIVED (dev-only 故意暴露原 err；prod 经 dev-mode flag 不暴露此端点；新建 sentinel + errmap 行 boilerplate > 价值) |
| 3 | dev_info.go:115-117 | `entries, err := os.ReadDir(path); if err != nil { return nil, err }` | A.4 | OK | 内部辅助函数 `walkHomeTree`，err 直接 raw return 不 wrap——这在**最内层**（直接调 stdlib）合规。§S16 `<pkg>.<Method>:` 前缀建议只对**跨层上抛**有意义；纯内部递归 helper 直接 return 是可接受的（最终调用方 site 2 已用 `os.IsNotExist` 判断）。 | — | — | — | — |
| 4 | dev_info.go:127-130 | `info, err := ent.Info(); if err != nil { continue }` | A.1 | EDGE | §S3 违规模式 #4 "if err != nil { /* nothing */ }"——entry.Info() 失败完全 silent skip。**但**：(a) walkHomeTree 是 dev-only tree 构建；(b) 单个 entry 失败 ≠ 整树失败，跳过显示其他子节点是合理 graceful；(c) 失败原因典型是 "permission denied" / 文件刚被删——tester 也无法行动。**改进**：dev 应该 log 一行（zap）让 tester 知道有 entry 跳过；dev_info handler 当前没有 logger 字段。 | LOW | tester 看不到为什么某文件没出现在树中。dev 调试体验略差，prod 不暴露此端点所以零用户影响。 | 给 DevHandler 加 `log *zap.Logger` 字段（如已存在则用），改为 `if err != nil { h.log.Warn("walkHomeTree: skip entry", zap.String("path", full), zap.Error(err)); continue }`。或加 inline `// dev-only: 跳过损坏 entry 让 walk 继续`。 | FIXED-doc (this commit — 加 dev-only graceful inline 注释) |
| 5 | dev_info.go:138-148 | `if info.IsDir() && depth < maxDepth { children, _ := walkHomeTree(full, relChild, depth+1, maxDepth, count, maxEntries); e.Children = children; ... }` | A.1 | EDGE | §S3 违规模式 #2 ——丢弃 walkHomeTree 的 error。**注意**：递归调用本就可能失败（permission denied 进入子目录、entry 被删），同 site 4 推理：dev-only graceful skip。**但 site 4 已 EDGE-LOW**——这里同类型，更明显（直接 `_`）。 | LOW | 同 site 4。 | 改为 `children, errC := walkHomeTree(...); if errC != nil { h.log.Warn(...); }; e.Children = children`。 | FIXED-doc (this commit — 加 dev-only graceful inline 注释) |
| 6 | dev_info.go:159 | `_ = fs.ErrNotExist` | A.1 | EDGE | "// Suppress unused fs import on platforms where ReadDir doesn't surface fs.PathError directly." —— **这是 §S3 例外**：`_ = err` 带行内注释说明含义。但 `fs.ErrNotExist` 不是 err 而是 sentinel 常量，被丢弃只是让 import 看起来用了。**实际**：当前文件用 `os.IsNotExist(err)` (site 2:85) 而非 `errors.Is(err, fs.ErrNotExist)`，所以 `fs` import 确实没用 —— 这行是 keep-import 工具调用占位。**真正修法**：删 `import "io/fs"` + 删该行，或用 `errors.Is(err, fs.ErrNotExist)` 替代 `os.IsNotExist(err)`。 | LOW | dev-only 编译开销极小；keep-import 占位非合理设计但运行无害。 | 删 `io/fs` import + 删 line 159。本是清理性修复，不算 violation。 | FIXED (this commit — 删 io/fs import + 删 placeholder line) |

## Sub-checks

A.1 §S3 错误吞没:
  - violations: **5 EDGE-LOW** sites 1, 4, 5, 6, （site 2 走的是包装 string 而非吞，归 A.4）
  - sites 1/4/5 都是 dev-only graceful 路径——home 拿不到、entry stat 失败、子目录 walk 失败——按设计 skip。**严格说 §S3 例外条款要求"行内注释"——3 处缺注释**。
  - site 6 是 keep-import 占位（带注释但不规范，应直接删 import）

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: **none**——本文件全是只读/枚举操作（os.ReadDir / ent.Info / 等），不写任何持久状态
  - 各自 ctx 来源: r.Context() 仅在 handler 入口隐式被 stdlib http.Server 管理，未传入 walkHomeTree（递归 helper 不接 ctx 是合理的——dev-only，遍历 ~500 entry 上限）
  - violations: **N/A: 文件不做终态写**

A.3 §S15 ID 生成:
  - ID generation calls: **none**
  - violations: **N/A: dev_info 不 mint 业务 ID**

A.4 §S16 错误 wrap 格式:
  - violations: **1 EDGE-LOW** site 2——`responsehttpapi.Error(... "walk failed: "+err.Error(), nil)` 是 dev-only 故意暴露原文设计
  - 内部 helper site 3 直接 return raw err，最内层合规

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: **0**
  - 已登记 errmap: N/A
  - missing: N/A——dev_info 端点完全不走 sentinel + errmap 路径（直接 `responsehttpapi.Error` 与 `responsehttpapi.Success`）；dev-only graceful 设计
