---
# Round 0030 — tool（波次 2 · M2.1）基础接口瘦身

类型 / 目标：M2.1 tool 基础接口重写——9 方法瘦成 5、删整套权限模式机制、`destructive`→`danger` 三级、**保留懒加载**。波次 2 起点（loop / 所有工具适配器 / chat 的根）。

## 核心方针（一句话）
**tool = 统一工具契约 + framework 三字段注入/剥离 + resident/lazy 懒加载；M1.9 权限解散 + M1.5 catalog 两个决定落到工具层 → 大幅瘦身。**

## 考古发现
- 9 方法里 **4 个死/解散**：`CheckPermissions`+`PermissionMode`（loop 固定传 default、真门控在已解散的 chat `interceptor.gate`）、`IsReadOnly`（仅已解散的 permissionsgate 用）、`NeedsReadFirst`/`RequiresWorkspace`（全仓 0 消费者）。
- `destructive` boolean：唯一实质消费者是 chat `interceptor.gate.Evaluate`（已随 M1.9 解散）。
- `Toolset.Lazy`+`activate_tools`：**不是** catalog 替代的（我一度判错并向用户纠正）——catalog 管**实体**报菜名（实体=搜索的数据、不是工具），懒加载管**工具自己**的巨大 description 烧 token，两者**正交**，**保留**。

## 关键决策（用户拍板）
1. **9 → 5 方法**：`Name`/`Description`/`Parameters`/`ValidateInput`/`Execute`。删 `CheckPermissions`/`IsReadOnly`/`NeedsReadFirst`/`RequiresWorkspace` + `PermissionMode`/`PermissionResult`。
2. **`danger` 三级**（`destructive` boolean → 枚举）：`safe`（静默）/`cautious`（标记不阻塞）/`dangerous`（阻塞等用户同意）。**纯信任**——工具不设静态下限，危险全由 LLM 逐次自报。
3. **三字段强制注入/剥离**：`summary`（必填）+ `danger`（必填三级）+ `execution_group`；`StripStandardFields` jsonrepair 兜 LLM 4-8% 畸形 JSON、`danger` 缺/坏 → `safe`。
4. **Toolset 懒加载保留**：resident/lazy；巨大 description 工具收起、prompt 报类名、`activate_tools` 拉入。
5. **与 M1.9 不冲突**：M1.9 砍的是**配置门控**（预配规则/模式/registry）；`danger` 是 LLM 逐次自报 + 一次确认，零配置。

## 新实现（`app/tool`，无 domain / store / handler）
- `tool.go`：`Tool` 5 方法接口 + `DangerLevel` 三级 + `IsValidDanger`。
- `fields.go`：`StandardFields` + `injectStandardFields`（注入 3 字段 + summary/danger 必填 + 冲突 panic）+ `StripStandardFields`（jsonrepair + danger 缺/坏→safe + 负 exec→0）+ `ToLLMDef`/`ToLLMDefs`。
- `toolset.go`：`Toolset{Resident,Lazy}` + `All()`。

## 测试（全离线）
14 个：inject（加 3 字段 / summary+danger 必填首位 / 3 冲突 panic / 非对象 panic / 无 properties）· strip（取 3 / 缺省 / 坏 danger→safe / 负 exec→0 / jsonrepair 修畸形）· ToLLMDef（注入 + 原 schema 不变）/ 批量 · IsValidDanger · Toolset.All。

## 验证
`gofmt -l` 干净 · `go build ./...` 0 · `go vet` 0 · `go test -race` ok（2.2s）。

## 契约
CLAUDE.md S18（9→5 方法 + summary/`danger`/execution_group + 无中央门控）；contract-changes #10（工具 schema `danger` 枚举 / `destructive` 改名 / 删权限机制 → 前端确认气泡 + LLM prompt）。**无 domains 文档**（tool 无契约文档）。

## 遗留 / 跨波次接线
- **`danger` 确认流 + `execution_group` 并行批** → loop M2.2。
- **`activate_tools` 工具 + 激活状态 + lazy 类 prompt + `host.Tools(ctx)` 组装** → chat M5.2。
- **15 个工具适配器**（filesystem/search/web/shell/function/handler/agent/skill/mcp/subagent/memory/document/todo/workflow/ask）→ 波次 2.3 / 3。
- workflow/subagent 的 `host.Tools` 用固定预过滤切片（无 lazy）→ 各自波次。

## 波次 2 起步
M2.1 tool ✅ → 下一 **M2.2 loop**（ReAct 引擎，接 danger 闸门 / execution_group 批 / todo `SystemReminder` 注入）。
