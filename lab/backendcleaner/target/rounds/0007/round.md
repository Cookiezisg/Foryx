# Round 0007 — pkg/limits（波次 0 · M0.1 末件）

类型 / 目标：迁移 `limits`（用户可调运行上限的唯一来源）—— 搬 + 清 Phase/adhoc 叙述 + 补测试。

依赖扫描：
- 上游：无（零 import，纯 Go）。
- 下游：**12 处横切** —— `infra/settings`、`infra/llm/provider`、`chat/runner`、`scheduler/dispatch_agent`、`subagent/spawn`、`tool/{handler,skill,mcp,agent,function}/search`、`permissions` handler、`cmd/server`。

它是什么：7 组运行上限（agent 步数/output cap/context 比/timeout/tool/workflow/guards）的配置 struct，镜像 settings.json 的 `limits` 段。`Default()` 返高 ceiling 默认。

修改后完整逻辑（给人看的）：
- `Limits` struct（7 组）+ `Default()`（MaxSteps 150 / SearchTopN 10 / LLMIdle 150s / AgentNode 10·硬 50 / 附件 50MB…）。
- 全局 getter 模式：`var current = Default` + `Current()`（消费方唯一读取点）+ `SetProvider(fn)`（启动期换 settings-backed getter，热重载）。`MaxSearchTopN=50` 硬上限常量。
- **保留全局 getter 模式**（用户拍板）：limits 是地基配置常量（非业务依赖）；`SetProvider` 仅 main 启动期、起 server 前调一次，之后只读 → 并发安全；热重载真相在 settings 层（getter 内部读最新快照）；横切 12 处，改 DIP 注入成本高收益低（`Default()` 本身即好的测试默认）。

删除 / 移出（原则 #7）：
- **Phase 阶段叙述**：`in P0 it's limits.Default, in P3 it reads settings.json`、`(P3)`、`P3's main.go calls SetProvider`、`P0 getter` → 改写当前事实。
- **adhoc 文档引用**：`see adhoc-topic-documents/limits-optimization/02`、`= 02 §4`、`decision #2`、`bucket-2` → 删（值/Why 保留，引用删）。
- 零关注点移出（全局 getter 保留，不进 deps-todo）。

契约变更：无对外 API。`Limits` struct 镜像 settings.json schema（M1.9 settings 那轮对齐）；`Current`/`SetProvider`/`Default`/`MaxSearchTopN` 是内部契约（12 下游），签名/数字不动。字段 `PerScenarioOverride`(scenario)/`UnknownModelMaxTokens`(modelcatalog) 对接 M1.3 model，optional 无害暂留。

新测试：搬 2（Default 值 / getter 形状）+ 补 4（Current 默认 / SetProvider 换源 / nil 忽略 / MaxSearchTopN；改全局态用 `defer SetProvider(Default)` 恢复）。

验证：`gofmt -w` 净；`go build -o /dev/null ./...` OK；`go vet` OK；`go test` 绿；残留 grep `P0|P3|adhoc|decision|bucket|§4` 零命中。

是否更干净：逻辑/数字/getter 模式一字不动；清掉 Phase/adhoc 叙述 + 补测试。搬+清范本。

覆盖状态：limits 标 cleaned。**M0.1 纯工具全部完成**（8 件迁移 + 1 件 userpath 判删）。

下一步：M0.2 `infra/db`（modernc sqlite + 迁移；schema 可激进重定）。`modelcaps`/`modelcatalog` 移交 M1.3 model 判定（非通用工具，是 model 数据层）。
