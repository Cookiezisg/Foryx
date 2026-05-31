# dev_routes.go — handlers/dev_routes (187 LOC, dev-only)

Audit scope: §S3 / §S9 / §S15 / §S16 / §S17.

## 文件性质

dev_routes.go 是**纯静态 manifest**——`devRoutes []devRoute` 是手维护的字符串数组，handler `Routes()` 只 `copy + sortRoutes + Success`。没有任何 service 调用、ID 生成、错误路径、ctx 操作。

## Trace 表

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | dev_routes.go:163-170 | `func (h *DevHandler) Routes(w http.ResponseWriter, r *http.Request) { out := make([]devRoute, len(devRoutes)); copy(out, devRoutes); sortRoutes(out); responsehttpapi.Success(w, http.StatusOK, out) }` | A.1 / A.2 / A.4 / A.5 | OK | Handler 仅做内存数据 copy + sort + JSON serialize。无 error path（`Success` 内部 marshal 即便失败也由 stdlib http.ResponseWriter 接住，不通过 handler 错误流）。无 ctx 使用（连 r.Context() 都不读）——纯静态 manifest 端点合理。无 sentinel 消费。 | — | — | — | — |
| 2 | dev_routes.go:172-180 | `func sortRoutes(rs []devRoute) { /* insertion sort */ }` | A.1 | OK | 纯算法实现，无 IO 无错误。`for i := 1; i < len(rs); i++ { for j := i; j > 0 && lessRoute(rs[j], rs[j-1]); j-- { ...swap... } }`——不可能产生错误。 | — | — | — | — |
| 3 | dev_routes.go:182-187 | `func lessRoute(a, b devRoute) bool { ... }` | A.1 | OK | 字符串比较 helper，纯函数无错误。 | — | — | — | — |

## Sub-checks

A.1 §S3 错误吞没:
  - violations: **not present**
  - 全文件无 error 表达式

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: **none**——纯只读 manifest 端点
  - 各自 ctx 来源: handler 入口 r 参数甚至**未使用** `r.Context()`（编译器允许，因为是 manifest read-only）
  - violations: **N/A: 文件不做任何 IO**

A.3 §S15 ID 生成:
  - ID generation calls: **none**
  - violations: **N/A**

A.4 §S16 错误 wrap 格式:
  - violations: **not present**
  - 全文件无 `fmt.Errorf` / `errors.New` / 错误路径

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: **0**
  - 已登记 errmap: N/A
  - missing: **N/A: 文件不消费任何 sentinel**

## 任务提示备注

任务描述里写："dev_routes.go 可能起 fire-and-forget goroutine（live-reload 风格），需 detached ctx 检查"。**实际**：dev_routes.go 是手维护的路由清单（用于 testend Routes tab "what endpoints exist"），**没有**任何 goroutine、live-reload、watcher 逻辑。文件长 187 LOC 但 158 行是 `devRoutes` 数组（manifest 字面量）。fire-and-forget concern 不适用——dev_routes 与 dev_runtime / dev_processes 等 dev handler 不同，是纯静态字符串端点。

**结论**：本文件 0 violation / 0 EDGE。是 B2 五文件中**最简单**的一个。
