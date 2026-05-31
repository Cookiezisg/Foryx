# Chat System Prompt 重写 + Prompt 面一致性

> 状态:✅ 已实现(2026-05-27)。`make verify` PASS(34 prompts 0 violations);常驻 ~5016 token。实现计划见 [`plans/2026-05-27-chat-prompt-redesign.md`](plans/2026-05-27-chat-prompt-redesign.md)。本轮改动未 commit(用户并行任务),留工作区由用户统一提交。
> 日期:2026-05-27
> 前置:能力披露层重构(`capability-disclosure-design.md`)已落地,常驻 ~5.1k。本设计治「对话效果」(system prompt 内容质量)+ 整个 prompt 面的一致性/正确性,**不动 token 架构**。

---

## 0. 背景

token 重构解决了「塞太多」;但 system prompt 内容本身在「对话效果」上,对照 best practice(Claude Code 真本 + Anthropic prompt docs + 本 agent 自己的 system prompt 精神)有 gap。一次**全 prompt 面审计**还逮到 1 个 live bug + 一批格式/自洽问题:

- 🔴 **live bug**:`multi_agent_forging` 段教 LLM `Subagent(type=...)`,但工具参数实际是 `subagent_type` → agent 照做会发出错误调用。
- 🟡 结构:`capabilities` 段**双 H2 撞**(runner 的 `## Your library` + catalog 的 `## Available capabilities` 同级);`multi_agent_forging` **每轮无条件注入** ~380t 主题花名段。
- 🟡 格式不统一:`memory` 用 `──── banner ────`;catalog header 渲染内部词 `(N, PerServer)`;section 名 `user_systemPrompt` 是 camelCase;`activate_tools` 描述重复 capabilities 的类别列表;`skill` label 误导。
- 🟡 best practice:`base` 仅一句、无行为框架;全程无操作原则、无 acting-with-care。

**设计取向**(融会贯通,不机械对齐模板):高密度操作原则句(非 bullet 堆)、**删产品介绍废话**(部署/数据归属/流式/自我定位 —— 对 AI 解决问题零价值)、信任 agent 判断不框死、harness 该讲的讲清(工具/能力/按需加载)。

---

## 1. 改后完整 chat system prompt(英文 const;`{}` 为运行时注入)

```text
<identity>
You are Forgify. You turn the user's needs into reusable capabilities — Functions (logic),
Handlers (stateful services), Workflows (DAGs over them) — and run them.
</identity>

<how_to_work>
- Reuse first: search_* the user's library and extend an existing Function/Handler/Workflow before forging a new one. Build the smallest fit — Function for logic, Handler when it needs state, Workflow to orchestrate ≥2 steps.
- Verify before claiming: run what you forge (run_function / call_handler / trigger_workflow dryRun); report the real result, with the actual error on failure — never claim untested success.
- Inspect before changing (get_* / read_document); if reality contradicts what the user described, surface the mismatch instead of plowing ahead.
- Before an irreversible or outward action — deleting a forge, running a Handler that writes external state, force-reverting, an external MCP write — set destructive=true and confirm when a wrong move is costly.
- Ask (AskUserQuestion) when the request is ambiguous or config is missing; don't guess, and don't interrogate over safe defaults.
- Be concise: lead with the result, skip the play-by-play, match the user's language.
- Run independent subtasks in parallel — same execution_group, or fan out with Subagent; keep coupled or side-effecting work sequential.
</how_to_work>

<tools>
Common tools are always loaded; pull the rest on demand with activate_tools(category):
function / handler / workflow — create · edit · delete · revert · run/call/trigger · inspect;
document (manage docs) · mcp (external servers) · skill (execution logs).
Three standard fields on every call: summary (one line: what + why), destructive (true if irreversible),
execution_group (int; same group runs in parallel, groups run in order).
Prefer Read/Edit/Grep/Glob over Bash cat/sed/grep. Search before you act; call by a real id, never a guess.
</tools>

<capabilities>          ← 动态:工具组计数 + 资产菜单(buildCapabilitiesSection,单一 H2)
## Your library
### function [run_function]
- normalize_address: clean & standardize an address
- … (30 total; search_function for more)
### handler [call_handler] · ### workflow [trigger_workflow] · ### skill [activate_skill] · ### mcp [call_mcp_tool]
</capabilities>

<memory>               ← 动态:markdown(无 banner)
## Pinned
### {name} (type={t})
{content}
## Index (read_memory(name) to load)
- {name}: {one-line}
</memory>

<environment>          ← 动态:替代裸 locale_hint
{date} · {modelId} · reply language: {locale}
</environment>
```

**SystemPromptSections 新顺序**:`identity → how_to_work → tools → capabilities → memory → documents → user_system_prompt → environment`。
(删 `multi_agent_forging`;`tool_conventions` 并入 `tools`;`locale_hint` 并入 `environment`。)

---

## 2. Subagent 工具描述改写(承接被删的 multi_agent_forging,通用化 + 修 bug)

`internal/app/tool/subagent/agent.go` 的 `Description()` 改为(用对参数名 `subagent_type`,用途不框死):

```text
Run a focused subtask in an isolated subagent — its own context window and a curated toolset, so your
context stays clean. Use it for independent research/exploration, or to fan out several independent
subtasks in parallel (e.g. forging several modules at once). Returns the subagent's final message.
Set subagent_type in the schema (Explore / Plan / general-purpose).
```

`subagent_type` 已是 enum(能力披露重构 T11 改);本段只是把"怎么用 Subagent"从 system prompt 花名段挪进工具自身,并改掉 `type=` 的错引用。

---

## 3. 审计修复清单(随重写一并做)

| 修复 | 位置 |
|---|---|
| 🔴 `Subagent(type=...)` → `subagent_type`(删花名段 + 工具描述用对名) | `runner.go` / `subagent/agent.go` |
| 删 `multi_agent_forging` 主题花名 section(内容移入 §2 + `how_to_work` 一句) | `runner.go` |
| `capabilities` 双 H2 合并 —— 删 catalog 的 `## Available capabilities`,只留 runner 的 `## Your library`,`### source [tool]` 直接挂其下 | `mechanical.go` |
| `memory` 段 `──── banner ────` → markdown `##` | `memory.go` |
| 删 catalog group header 的 `(N, Granularity)` 内部词,改为 `### source [tool]` | `mechanical.go` |
| section 名 `user_systemPrompt` → `user_system_prompt` | `runner.go` |
| `activate_tools` 描述去掉 6 类别枚举(capabilities 段已列),改为"load a lazy group before using its tools" | `toolset/activate.go` |
| `skill` categoryLabel "skill execution history" → "skill execution logs" | `runner.go` categoryLabels |

---

## 4. 涉及文件

- `internal/app/chat/runner.go` — 重写 prompt const(identity/how_to_work/tools)、删 multi_agent_forging、合并 tool_conventions→tools、locale_hint→environment、section 重命名、`*Text()` exporters 同步
- `internal/app/tool/subagent/agent.go` — Description 改写
- `internal/app/catalog/mechanical.go` — 删 `## Available capabilities` + `(N, Granularity)`
- `internal/app/memory/memory.go`(`ForSystemPrompt`)— banner → markdown
- `internal/app/tool/toolset/activate.go` — 去重描述
- 测试:`runner_test.go`、`mechanical_test.go`、`subagent` 测试、token guard

---

## 5. 非目标(本次不做)

- **64 工具 Description voice 统一**(审计 A5:电报体/口语/结构化三种 register)—— 刚精简过,边际低,标「未来」。
- `documents` 段双 XML 嵌套(审计 A3)—— 是 Anthropic doc-block 模式,intentional,保留。
- search 工具复数命名不一(审计 A7)—— 改工具名有风险,保留。

---

## 6. 测试 + §S14 文档同步

- `runner_test`:各新 section(identity/how_to_work/tools/environment)存在 + 关键词;`multi_agent_forging` 段已不存在;section 名全 snake_case。
- Subagent:Description 含 `subagent_type`(不含 `type=`);grep 确认全仓无 `Subagent(type=` 残留。
- `mechanical_test`:catalog 输出无 `## Available capabilities`、无 `Granularity` 字样、单一 H2。
- token guard(`TestResidentContext_UnderBudget`)仍过(净变化 ~+200t / -380t multi_agent,应更省或持平)。
- §S14:`capability-disclosure-design.md`(§4.1 system prompt 结构更新)、`service-design-documents/chat.md`、`CLAUDE.md` §S18(若 prompt 段模式描述需更新)、`progress-record.md` dev log。
