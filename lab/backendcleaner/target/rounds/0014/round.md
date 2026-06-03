# Round 0014 — infra/stream 单一 Bus（波次 0 · M0.5）

类型 / 目标：实现三条 SSE 流的进程内底座——单一 `Bus`（实例化三次 = E1 三流），把旧三抄同构 Bridge 收敛成一份。按 `stream-protocol.md` §5。

考古发现：旧 `infra/{eventlog,forge,notifications}` Bridge 95% 同构三抄，只差元素类型 / buffer 大小 / notif 多 2 方法（PublishEphemeral + List）。

设计落地（3 源 + 3 测试）：
- `Bus{spaces map[ws]*workspaceState, bufSize}` + `New(bufSize)`，实例化三次（messages 大 buffer / entities·notif 小）。
- **frame 分级（核心）**：`Publish` 按 `frame.Durable()` 分流——durable（open/close/非 ephemeral signal）分配 seq + 入环形 buffer + 阻塞保序扇出（不丢）；ephemeral（delta/tick）seq 0 + 不入环 + 满则丢（不卡 producer）。**旧的单独 `PublishEphemeral` 方法消失**，内化进 Publish。
- 按 workspace 分流（`RequireWorkspaceID`，原 RequireUserID）。
- `Subscribe`：fromSeq>0 replay 环内 durable 帧；缺口被淘汰 → `ErrSeqTooOld`；cancel 幂等、先 `close(done)` 解锁阻塞中的 Publish 再争锁。
- `List`（`ListReader`，notif 用）：读环内 durable，分页 hasMore。
- 文件拆分：`bus.go`(结构+构造+Publish) / `subscribe.go` / `list.go`。
- **D2 拍板：v1 workspace 全量推**（前端过滤）；scope 级订阅留扩展点（buffer 已按 workspace）。

测试（`-race` 绿）：durable seq 递增 / ephemeral seq0 不入环且不递增 seq / 缺 workspace / 校验 / workspace 隔离 / 实时扇出 / replay fromSeq / ErrSeqTooOld / cancel 幂等 / List 分页 + 翻页 / 空 ws / List 缺 workspace。

验证：`gofmt -l` 空 / `go build ./...` / `go vet` / `go test -race` 全绿。

是否更干净：✅ 旧三抄（3×~180 行）→ 单一 Bus（3 文件 ~140 行核心），并发正确性收一处；frame 分级把 ephemeral 从「单独方法」变成「Publish 内化判断」。

覆盖状态：infra/stream 完成。`infra/chat`(extractor，依赖 chatdomain) 移交 M5.2；三流 Bus 实例化 + 注入 + SSE 线缆 marshal = M0.7/cmd → deps-todo（R0014 节）。

下一步：M0.6 `infra/llm`。
