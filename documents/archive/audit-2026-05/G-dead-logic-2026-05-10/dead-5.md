# dead-5 — tool framework + web/shell/filesystem/search 死逻辑审计

审计范围：
- `internal/app/tool/tool.go` (framework 主文件 + 接口)
- `internal/app/tool/web/` (4 文件，不含 _test.go)
- `internal/app/tool/shell/` (5 文件：shell.go / bash.go / bash_route.go / output.go / kill.go / manager.go)
- `internal/app/tool/filesystem/` (4 文件：filesystem.go / read.go / write.go / edit.go)
- `internal/app/tool/search/` (5 文件：search.go / grep.go / grep_rg.go / grep_stdlib.go / glob.go)

跳过：forge 子包（用户重写中）。不读 _test.go。

---

## 1. HIGH — Bash 描述承诺 sandbox 自动路由 ≠ 实际能装的 runtime

**Location**:
- `internal/app/tool/shell/bash.go:99` — Bash tool description 字符串列出 `pip / python / uv / node / npm / npx / pnpm / cargo / go / gem / bundle / mvn / gradle / composer / dotnet, etc.` 全部"自动 sandbox 路由"
- `internal/app/tool/shell/bash_route.go:60-67` — `runtimeDetectors` 8 个 kind：`python / node / rust / go / ruby / php / java / dotnet`
- `cmd/server/main.go:521-532` — `registerSandboxStack` 仅注册 3 个 installer (`python` / `node` / `uv`，uv 走 python kind) + 2 个 EnvManager (`python` / `node`)

**Claims to do**:
- Bash description 告诉 LLM 这些命令"automatically execute inside a per-conversation isolated environment"
- bash_route.go 的 detectRuntime 真能检测 rust / go / ruby / php / java / dotnet kind 并返回非空字符串

**Reality**:
LLM 在对话里跑 `cargo build` / `go test ./...` / `gem install` / `bundle exec` / `mvn package` / `gradle build` / `composer install` / `dotnet build` 任意一个时，链路是：
1. `detectRuntime` 返 `"rust"` / `"go"` / `"ruby"` / `"php"` / `"java"` / `"dotnet"`
2. `maybeAutoRoute` 进 `t.sandbox.EnsureEnv(EnvSpec{Runtime: {Kind: <kind>}})`
3. `EnsureEnv` (sandbox.go:505) 先调 `EnsureRuntime`，installer map 里无该 kind → 返 `ErrRuntimeNotSupported`
4. 错误链回到 `formatAutoRouteError`（bash.go:259-267）
5. LLM 收到 tool result：

   ```
   Sandbox auto-route could not prepare the runtime for this command. The
   command was NOT executed (running on the system shell would return
   misleading data — e.g. system Python 3.9.6 instead of the conversation's
   isolated 3.12 venv). Please retry, or have the user check the sandbox
   status in testend.

   Reason: shelltool.Bash.maybeAutoRoute: sandbox env install failed
   (rust for cv_xxx): ...

   [sandbox auto-route failed]
   [exit code: -1]
   ```

LLM 看到这条会试图重试 / 报告问题 / 提示用户改 sandbox。但根本问题是：bash description 说会自动路由，runtimeDetectors 配合声称在检测，但 sandbox 注册栈里那 6 个 kind 根本没装；只是 fail-safe 回到"绝不静默落系统 shell"。**LLM 在每次跑 go/cargo/gem/mvn/gradle/composer/dotnet 时都被路由到死路上**。

更糟：因为 bash description 明确承诺"runtime auto-route"，LLM 可能因此**避免**用 Bash 跑那些命令（"系统装了我也不该用"），等于功能性退化。

`envBinDirsForKind` (bash_route.go:317-338) 还有 rust/go/ruby/php cases — 若将来 EnvManager 装了它们，链路会走通。但目前是死的。

**Severity**: HIGH — 用户场景下直接可触发，LLM 收到误导性失败 + bash description 与 runtime 实力不符。

**Fix**: 二选一
- (A) **裁 detectors**：`runtimeDetectors` 砍剩 python + node 两条，让 `cargo build` / `go test` 等不被 detect → 直接落系统 shell（sandbox 不参与，跟 `git status` 同路径）。同步删 bash description 里那串语言。
- (B) **扩 sandbox stack**：补 rust/go/ruby/php/java/dotnet 的 MiseInstaller + EnvManager。但这是 D2 / 后续 phase 的工作，当前不值。

推荐 A。

**Risk**: 不修则每个跑 cargo / mvn / gradle / dotnet 等命令的对话都会撞这堵墙。LLM 不知道为什么命令"装了又装不上"，可能要花掉 1-2 个 turn 试错或问用户。投资人 demo 时撞上这问题观感差。

---

## 2. MED — Tool 接口 3 个静态元数据方法（IsReadOnly / NeedsReadFirst / RequiresWorkspace）框架不读

**Location**:
- `internal/app/tool/tool.go:111` — `IsReadOnly() bool`
- `internal/app/tool/tool.go:118` — `NeedsReadFirst() bool`
- `internal/app/tool/tool.go:125` — `RequiresWorkspace() bool`
- 全 backend 内 `\.IsReadOnly\(\)` / `\.NeedsReadFirst\(\)` / `\.RequiresWorkspace\(\)` 调用：**0 个**（grep 全空）
- 实现方：~30 个 tool 文件，每个写 3 个 boilerplate 方法（约 90 个方法体）

**Claims to do**:
tool.go godoc：
- IsReadOnly：`true → safe to run concurrently with other read-only tools`
- NeedsReadFirst：`Phase 5 Edit/Write` 文件级 read-first 守卫
- RequiresWorkspace：`Phase 5 Bash/Edit/Write` 工作区白名单

**Reality**:
- IsReadOnly 的并发调度被 `execution_group` 字段（LLM 自报）取代。CLAUDE.md §S18 §1 末尾 + tool.go:73-77 都声明"there is no IsConcurrencySafe method anymore. Parallel scheduling is driven by the LLM-supplied execution_group"，但同时仍要求 tool 实现 IsReadOnly。
- NeedsReadFirst 在 Edit (edit.go:127-129) / Write (write.go:107-109) 注释明说 "metadata; actual enforcement in Execute"——真的守卫在 Execute 内查 `state.WasRead(cleaned)`，不靠这个方法。
- RequiresWorkspace 同样"文档性元数据"——PathGuard 在 Execute 内调 `t.pathGuard.Allow(args.FilePath)`，不查这个方法。CLAUDE.md §S18 §8 自己承认："NeedsReadFirst / RequiresWorkspace 当前是文档性元数据——框架不强制，靠每个 tool 在 Execute 内部自查。"

CLAUDE.md §S18 §1 注释里已写 "IsReadOnly: 仅文档/语义参考；不再驱动并发调度"——即文档承认这 3 个方法是文档值。但接口仍强制实现，每个新 tool 必须复制 3 行 boilerplate，加新 tool 的成本里有 ~5% 是为这 3 个无用方法服务。

**Severity**: MED — 不破坏功能，但是 framework 招牌契约里写 9 方法，其中 3 个无效。新 tool 作者必须读到 §S18 才知道"声明 true 但 Execute 没自查 = 元数据撒谎"，框架毫无察觉。

**Fix**: 二选一
- (A) **从接口里删**：3 方法从 Tool interface 移除；同步删 ~30 个 tool 的 boilerplate（每文件 3 行）。新 tool 作者少抄 3 行。
- (B) **降级为可选** + 文档说明仅供未来 scheduler 提示：用 `interface{ IsReadOnly() bool }` type assertion，非必须实现。

推荐 A——既然今天没消费者，且 §S18 都讲清楚 enforcement 在 Execute 自查，删了让接口诚实。等真要做 Phase 4+ scheduler 再加。

**Risk**: 接口腐化的活样本。新人读 9 方法接口会以为这 3 个有意义，复制 boilerplate 时陷入"声明 true 还是 false 才对"的纠结。CLAUDE.md §S18 §8 补丁式一句话告知"仅文档"是正本清源前的临时缓冲。

---

## 3. LOW — PermissionMode 三个常量（AcceptEdits / Plan / Bypass）从未被传入

**Location**:
- `internal/app/tool/tool.go:51-53` — `PermissionModeAcceptEdits` / `PermissionModePlan` / `PermissionModeBypass` 常量
- `internal/app/loop/tools.go:219` — `t.CheckPermissions(argsJSON, toolapp.PermissionModeDefault)` 框架唯一传入点，**永远是 Default**

**Claims to do**:
tool.go:42-46 godoc：`Reserved for Phase 4+ workflow scheduler / acceptEdits UI`。

**Reality**:
- runOneTool 永远只传 PermissionModeDefault；其他 3 个常量从未被任何代码路径写入
- CheckPermissions 的 `mode` 参数在所有 tool 实现里都用 `_` 忽略（grep `func.*CheckPermissions.*PermissionMode\) PermissionResult` 全部）
- 即使 tool 想根据 mode 决定 Allow/Deny/Ask，框架也没传过非 Default 的值

**Severity**: LOW — 是显式 reserved-for-future 的占位符，不是迷惑。但跟下面 #4 配合，说明 PermissionMode 整套机制都是 stub。

**Fix**: 不动。等 Phase 4+ acceptEdits UI 做的时候再激活。godoc 已经标 "reserved" 是诚实的。

**Risk**: 无。已自我标注未来用。

---

## 4. LOW — runOneTool 的 PermissionDeny / PermissionAsk 两 case 永远不进

**Location**:
- `internal/app/loop/tools.go:219-230` — runOneTool 的 switch 三 case
- 全 backend 内 tool 实现里 `return toolapp.PermissionDeny` / `return toolapp.PermissionAsk`：**0 个**（grep 全空，~28 个 tool 实现的 CheckPermissions 全 `return PermissionAllow`）

**Claims to do**:
PermissionDeny case 写 `"permission denied for this call"` 给 LLM；PermissionAsk case 留给 Phase 4+ 用户审批 UI。

**Reality**:
没有 tool 现在会返 Deny 或 Ask。switch 的 PermissionAllow（默认 case）外加未声明的 default 分支才是真路径。两个非 default 的 case 当前死。

**Severity**: LOW — 跟 #3 同源（PermissionMode 整套机制是 stub）。框架接住未来的扩展点。

**Fix**: 不动。tool.go:142-144 godoc 已说 "Forgify Phase 3 一律传 mode=Default，多数 tool 返 Allow；保留位给 Phase 4+ workflow scheduler"。

**Risk**: 无。

---

## 5. LOW — `ToLLMDef`（singular）仅 tool_test.go 用，生产无消费者

**Location**:
- `internal/app/tool/tool.go:165-171` — exported `func ToLLMDef(t Tool) llminfra.ToolDef`
- `internal/app/tool/tool.go:176-182` — `ToLLMDefs` (plural) 内部调 `ToLLMDef(t)` 但只在 for 循环里
- 全 backend 调用：仅 `internal/app/tool/tool_test.go` 用（line 296, 334），生产代码无 grep 命中

**Claims to do**:
godoc：`ToLLMDef converts a Tool to the ToolDef sent to the LLM, automatically injecting "summary" and "destructive" fields into the Parameters schema.`

**Reality**:
- 生产唯一调用方是 `loop/loop.go:96` 用 `toolapp.ToLLMDefs(tools)` 处理整个切片
- `ToLLMDef`（singular）只是 ToLLMDefs 内部循环用 + tests 用作单元单点测试

**Severity**: LOW — CLAUDE.md T4 允许导出符号仅供测试使用，但要求加注释。`ToLLMDef` 的 godoc 没说"prod 不调，仅测试契约"。如果作者删了 ToLLMDef 改 ToLLMDefs 内联实现，会破 tool_test.go 的几条测试，但 deadcode 工具默认不扫测试代码（CLAUDE.md "开发期工具纪律" 提醒过）。

**Fix**: godoc 加一句 "Production callers use ToLLMDefs (plural); singular form preserved for unit-test access."

**Risk**: 低。下次有人想"清理这个为什么导出"删了，会破测试。

---

## 6. LOW — `web/web.go` 包文档说"BYOK → MCP → Bing CN routing"，Bing CN 已删

**Location**:
- `internal/app/tool/web/web.go:4` (英文 doc) — `WebSearch (BYOK → MCP → Bing CN routing)`
- `internal/app/tool/web/web.go:13` (中文 doc) — `WebSearch（BYOK → MCP → Bing CN 路由）`
- `internal/app/tool/web/web.go:67` — `newWebSearch` godoc：`mcpRouter 可空以禁用 MCP 层（如只测 BYOK + Bing CN 路径的单测）`
- 对照 `internal/app/tool/web/search.go:1-30` (英文 + 中文 package-level doc) 明说："We deliberately removed the previous Bing CN HTML scrape 'fallback' — modern Bing/Bing CN renders results via JavaScript, so HTML scraping returns 0 hits regardless of UA."
- search.go:99 tool description 也只列 4 个 BYOK provider + duckduckgo MCP，无 Bing
- 代码里没有 `searchBing` / `bing` 函数（grep 全空）

**Claims to do**:
web.go 包文档把 WebSearch 的 routing 链描述为 3 段：BYOK → MCP → Bing CN。

**Reality**:
- WebSearch.Execute (search.go:221-264) 实际只走 2 段：Tier 1 BYOK + Tier 2 MCP
- 两段都失败时返"install duckduckgo / configure BYOK"提示，不再有 Bing CN 兜底
- web.go 包文档与 search.go 包文档自相矛盾——前者承诺 3 段，后者明说删了第 3 段

**Severity**: LOW — 误导未来读者认为还有 Bing 路径，可能去找代码里的 Bing client。读 search.go 后会发现真相，但浪费时间。

**Fix**: web.go:4 / web.go:13 / web.go:67 三处删 "Bing CN" 字样。

**Risk**: 文档腐化。CLAUDE.md §S14 文档同步纪律一条违反——代码改了 Bing 删除时联动文档没改 web.go。

---

## 7. LOW — `tool.go` 包文档列出 `tool/tasks/` `tool/ux/` 两个从未存在的子包

**Location**:
- `internal/app/tool/tool.go:14-16` (英文 doc) — `plus future tool/filesystem/, tool/shell/, tool/web/, tool/tasks/, tool/ux/ (Phase 5)`
- `internal/app/tool/tool.go:26-27` (中文 doc) — `按 tool 家族分子包：tool/forge/、tool/filesystem/、tool/shell/、tool/web/、tool/tasks/、tool/ux/（§S12 例外位置...）`
- 实际子包：`ask/, mcp/, search/, shell/, skill/, subagent/, todo/, forge/, filesystem/, web/`

**Claims to do**:
godoc 暗示未来会有 `tool/tasks/` 和 `tool/ux/` 子包。

**Reality**:
- "tasks" 实际命名为 `tool/todo/`（todo 系统级命名为 Todo / TodoCreate / TodoList / TodoUpdate / TodoGet）
- "ux" 这个 hat 从未实例化——它本来可能是一组对话级 UX tool 容器，但实际拆为 `tool/ask/`（AskUserQuestion）+ `tool/skill/`（SearchSkills / ActivateSkill）+ `tool/subagent/`（SubagentTool）

**Severity**: LOW — 文档腐化，目前结构与 doc 描述不一致。新加 tool 的人按 doc 找不到 tasks/ux。

**Fix**: tool.go:14-16 + tool.go:26-27 改为列实际子包；不再保留"future"二字。

**Risk**: 文档与代码漂移。CLAUDE.md §S14 违反小例。

---

## 8. EDGE — `BgProcess.ConvID` 字段有写无读

**Location**:
- `internal/app/tool/shell/manager.go:80` — `ConvID string // conversation that started it (informational)`
- `internal/app/tool/shell/bash.go:549` — `proc := &BgProcess{ConvID: convID, ...}`
- `internal/app/tool/shell/manager.go:223` — `Snapshot.ConvID string \`json:"convId,omitempty"\``
- `internal/app/tool/shell/manager.go:245` — snapshot 函数把 `p.ConvID` 复制给 `s.ConvID`
- 全 backend 内读 `\.ConvID`：仅 manager.go:245 自身读自身写

**Claims to do**:
field comment 说 `informational`——意思是 dev 端点 `/dev/bash-processes` 会序列化输出，testend 给人看用。

**Reality**:
- 前端 testend 控制台收到 `processes[].convId` 字符串，但当前 testend 实现（`dev_processes.go`）只把整 array 给 envelope；没看到任何前端代码按 convId 过滤或显示
- 既无 filter API 也无 UI 列说"哪个对话起的"
- 字段是写出来给 JSON 的，理论上 testend 可以用，目前没用

**Severity**: EDGE — 不是死代码，是"写了准备给前端但前端没拿"的 dangling 字段。生产意图明确（informational），实施一半。

**Fix**: 二选一
- (A) testend 加一列显示 ConvID（或加 query 过滤）
- (B) 删字段——backup 路径是 `Bash` 命令本身的 reqctx 已经拿到 convID，未来要 filter 时再加

不动也可（保留 informational 字段是合理的）。

**Risk**: 无。

---

## 9. EDGE — `envBinDirsForKind` 的 rust/go/ruby/php cases 当前不可达

**Location**:
- `internal/app/tool/shell/bash_route.go:329-334` — switch cases for `"rust"` / `"go"` / `"ruby"` / `"php"`

**Claims to do**:
godoc：返这些 kind 的 EnvManager 期望的 bin 目录前置到 PATH。

**Reality**:
- 见 finding #1：rust/go/ruby/php 的 RuntimeInstaller 与 EnvManager 都没注册
- maybeAutoRoute 调链 `EnsureRuntime → installer == nil → ErrRuntimeNotSupported`，**永远不会调到** `envBinDirsForKind`
- envBinDirsForKind 是被 `maybeAutoRoute` 在 `EnsureEnv` 成功**后**调用的（bash.go:351-352）

**Severity**: EDGE — 与 finding #1 同根。修了 #1（裁 detectors）这些 cases 就真死了；不修 #1（扩 sandbox stack）这些 cases 就活了。今天技术上死。

**Fix**: 与 #1 一起处理。如果裁 runtimeDetectors 只剩 python/node，envBinDirsForKind 也只保留 python/node case + default。

**Risk**: 同 #1。

---

## 10. EDGE — `WebSearch.mcpRouter == nil` 路径生产不可达

**Location**:
- `internal/app/tool/web/search.go:242` — `if ctx.Err() == nil && t.mcpRouter != nil { ... }` 生产路径
- `internal/app/tool/web/search_mcp.go:55-57` — `if t.mcpRouter == nil { return nil, ErrMCPSearchUnavailable }` runMCPSearch 内防御
- `cmd/server/main.go:285` — 唯一构造点：`webtool.WebTools(modelService, apikeyService, llmFactory, mcpapp.NewSearchRouter(mcpService), log)` 永远传非 nil router
- web.go:67 newWebSearch godoc：`mcpRouter 可空以禁用 MCP 层（如只测 BYOK + Bing CN 路径的单测）`——为测试保留 nil

**Claims to do**:
mcpRouter nil 时 WebSearch 跳过 Tier 2，直接进 fallback "no backend" 提示。

**Reality**:
- 生产 main.go 永远传非 nil，nil 分支不执行
- `runMCPSearch` 内的 `if t.mcpRouter == nil` 是双重防御（Execute 已 if 过一遍）
- 测试代码（_test.go，本审计跳过）会用 nil 来隔离 BYOK 路径

**Severity**: EDGE — 是合法的测试注入点，CLAUDE.md T6 / T2 鼓励这种 fake-able 设计。tested-only fallback 不算死。

**Fix**: 不动。

**Risk**: 无。

---

## 总结

10 个发现：
- 1 HIGH (#1 sandbox runtime 不匹配 detector)
- 1 MED (#2 Tool 接口 3 个静态元数据死方法)
- 5 LOW (#3-#7 reserved 常量 / unused branches / test-only export / 两处 stale doc)
- 3 EDGE (#8 informational field / #9 与 #1 联动 / #10 测试注入点)

最值得当下修的是 #1（用户场景下 LLM 直接撞，可触发频次高，且与 bash description 公开承诺自相矛盾）+ #6 + #7（两处易修、删字消除文档与代码漂移）。

#2 是更大的设计债——值得在 Phase 4+ scheduler 启动前清掉，把 9 方法接口收成 6 方法（identity 3 + ValidateInput / CheckPermissions / Execute），直到 Phase 4+ 真要 scheduler 时再扩。

#3 #4 是 Phase 4+ 自我标注的 reserved-for-future 占位符——不是死，是"备用插座"，按 §S20 不必现在动。
