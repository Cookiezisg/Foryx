# 02 — 前端「高级能力」设置区(settings.json backing + UI + 注入)

> **定位**:把 [`01`](./01-optimize-decisions.md) 里标"可配"的限制做成**用户可调**,落实原则 #3——**存** `settings.json`,**调节入口在前端 Settings 界面底部新增「高级能力」区**,不让用户碰裸文件。
> 默认值 = `01` 的高 ceiling;留空/缺省 = 代码默认。
> **注入时序**:`DefaultLimits` + `func() Limits` getter **在 P0 就落地**(先只返回默认值),P0–P2 一上来就读 getter 写最终形态;P3 只把 getter 的**数据源**从默认改成 `settings.json` + 加读写端点 + 前端 UI(详 [`03`](./03-implementation-plan.md))。

---

## §1 数据落点:`settings.json` 加 `limits` 段

后端已有 `infra/settings`(读 `~/.forgify/settings.json`,100ms debounce + 5s 轮询热重载,现存 permission modes / hooks)。**复用它**,加一个 `limits` 对象:

```jsonc
// ~/.forgify/settings.json
{
  "permissions": { /* 现存 */ },
  "hooks": [ /* 现存 */ ],
  "limits": {                          // ← 新增;整段可缺省,缺省即代码默认
    "agent": {
      "maxSteps": 150,                 // ReAct 步数 ceiling(0 = 无限,靠中断)
      "maxTurnDurationSec": 1800,      // 单轮墙钟(0 = 无限)
      "subagentTimeoutSec": 600,
      "subagentMaxTurns": 30
    },
    "output": {
      "unknownModelMaxTokens": 64000,  // 未知模型输出兜底(modelcaps fallback)
      "perScenarioOverride": {}        // (P3 可选){ dialogue|utility|agent: int };空=用模型真值
    },
    "context": { "softRatio": 0.70, "hardRatio": 0.85 },
    "timeout": {
      "llmIdleSec": 150,               // 流式 LLM「无 token 多久」算死连接(非总墙钟;每 token 重置)
      "mcpCallSec": 180,               // mcp 工具调用:超时=把控制权还给 agent(高默认)
      "bashDefaultTimeoutSec": 120     // Bash 默认超时:同上(单次调用仍可传更大值,硬上限 600s 为常量)
      // 注:workflow 节点墙钟超时已删——节点靠 run-level ctx + stop,不在此配
    },
    "tools": {
      "searchTopN": 10,                // 所有 search_* 统一默认返回数(硬上限 50 为常量)
      "readDefaultLines": 2000,
      "bashOutputCapKB": 256
    },
    "workflow": {                      // 无人值守例外:这里 cap 仍 load-bearing
      "agentNodeMaxTurns": 10,
      "agentNodeMaxTurnsHard": 50
    },
    "guards": {                        // (P3 可选)桶 2 的"机制必需但值可放开"
      "attachmentMaxMB": 50,
      "httpNodeRespMaxMB": 10,
      "webhookBodyMaxMB": 10
    }
  }
}
```

**只暴露有意义的 ~14 个 knob,不是 150 个**——其余写死项(crypto/idgen/SSE buffer/枚举/防护下限)不进 UI。

---

## §2 后端:读 + 写 + 注入

### 读 / 写
- `infra/settings` 增 `Limits()` 返回当前快照(zero-value → 默认 填充);整段缺省返回 `DefaultLimits`。
- 增**写路径**:`Settings.UpdateLimits(patch)` —— **read-modify-write**:先读现有 `settings.json`(含用户手改的 permissions/hooks),**只替换 `limits` 段**,原子写回(`tmp+rename`,0600),**绝不整文件覆盖**(否则冲掉用户的 hooks)。写后立即更新内存快照(不等轮询)。
- 新端点(N 系列 envelope,camelCase):
  - `GET /api/v1/settings/limits` → `{data: Limits}`(含每项"是否用默认"标记,供 UI 显示)
  - `PUT /api/v1/settings/limits` → 200,body 为完整 `Limits`(N6 upsert 语义);非法值 400。

### 注入(DIP getter,沿用现有 `CapabilityResolver func` 注入范式)
`main.go` 把 `func() Limits` getter 注入各消费方,**热重载即时生效**。**getter 本身 P0 就装上**(先返回 `DefaultLimits`),所以下表的注入点在 P0–P2 各自阶段就接好;P3 只让 getter 改读 settings:

| 消费方 | 取哪些 |
|---|---|
| `app/chat` / `app/loop` | `agent.maxSteps` / `maxTurnDurationSec` |
| `app/subagent` | `agent.subagentTimeoutSec` / `subagentMaxTurns` |
| `infra/llm`(transport / anthropic / modelcaps) | `timeout.llmIdleSec`、`output.unknownModelMaxTokens` |
| `app/contextmgr` | `context.softRatio` / `hardRatio`(已有 `SetThresholds` 钩子) |
| `app/scheduler` | `workflow.agentNodeMaxTurns*`(**节点墙钟超时已删,不再注入 nodeTimeout**) |
| `app/mcp` | `timeout.mcpCallSec` |
| `app/tool/{function,handler,filesystem,shell}` | `tools.searchTopN` / `readDefaultLines` / `bashOutputCapKB`、`timeout.bashDefaultTimeoutSec` |

> getter 而非值快照——热重载后下一次 turn 立刻读到新值,无需重启。

---

## §3 前端:Settings 界面底部「高级能力」区

### FSD 落点
- **entities/settings**:新增 `api/`(`useLimits` / `useUpdateLimits`,走 `shared/api/httpClient`)+ `model/types.ts` 加 `Limits` 类型。(现 `entities/settings/model/settingsStore.ts` 管 theme/lang/density 等**前端偏好**;`limits` 是**后端行为配置**,经 API 往返,故走 `api/`。)
- **features/settings/ui**:新增 `AdvancedCapabilitiesSection.tsx`,挂在 `SettingsModal` **最底部**。
- `shared/api/queryKeys.ts`:加 `qk.settingsLimits()`。

### UX
- 默认**折叠**的「高级能力 / Advanced」disclosure,顶部一句警示文案(本地单用户、改这些会影响 agent 行为/成本)。
- 按 §1 分组渲染:`Agent` / `输出` / `上下文` / `超时` / `工具` / `工作流`(+ P3 可选 `防护`)。每项:label + 数字输入(或滑块)+ 当前是否默认的标记 + 单项「恢复默认」。
- 区底「全部恢复默认」按钮(PUT 一个空/默认 `Limits`)。
- 组件零业务决策(= S6/前端铁律):只调 `useUpdateLimits` 拿意图级 API。
- **i18n**:新增 `settings.advanced.*` 键(zh/en 全量),不硬编码中文。

### 视觉
沿用现有 `frontend/src/styles` 的 `.set-sec` / `.onb-grid` 等既有 class(参照 `ModelDefaultsSection.tsx` 的卡片/网格),不引入新体系。

---

## §4 默认值表(= 代码默认 = settings 缺省)

| 组 | knob | 默认 | 旧硬编码 |
|---|---|---|---|
| agent | maxSteps | **150**(0=无限) | 20 |
| agent | maxTurnDurationSec | **1800** | 600 |
| agent | subagentTimeoutSec | **600** | 300 |
| agent | subagentMaxTurns | **30** | 25/30 |
| output | unknownModelMaxTokens | **64000** | 8192/8096 |
| output | perScenarioOverride(P3 可选) | **{}**(用模型真值) | — |
| context | soft / hard | **0.70 / 0.85** | 0.70 / 0.85(不变) |
| timeout | llmIdleSec(死连接网) | **150** | (总墙钟 120,删) |
| timeout | mcpCallSec(还控制权给 agent) | **180** | 30 |
| timeout | bashDefaultTimeoutSec(同上) | **120**(硬顶 600 常量) | 120(默认不变,改可配) |
| tools | searchTopN(统一所有 search_*) | **10**(硬顶 50 常量) | 3/5·5/20·3/10 各异 |
| tools | readDefaultLines | **2000** | 2000(不变) |
| tools | bashOutputCapKB | **256** | 256(不变) |
| workflow | agentNodeMaxTurns / Hard | **10 / 50** | 10 / 50(不变,例外) |
| guards(P3 可选) | attachment/httpResp/webhook MB | **50 / 10 / 10** | 同(可放开) |

> 不变项也进 UI,是为了"一处可调"的完整性;它们的默认即现值。**workflow 节点墙钟超时不在表里**——已删,节点靠 ctx + stop。

---

## §5 §F1 文档同步(实现时)

- 新 endpoint → `frontend-prd.md §17` + `service-contract-documents/api-design.md` + `entity-types.md`(`Limits` 类型)。
- DIP / 注入 → `frontend-contract-documents/cross-cutting.md`。
- 新 feature/section → `frontend-design-documents/feature-settings.md` + `fsd-layers.md` slice 行。
- settings.json schema → `service-design-documents/`(settings 相关)+ `desktop-packaging-notes.md` 若涉及。
