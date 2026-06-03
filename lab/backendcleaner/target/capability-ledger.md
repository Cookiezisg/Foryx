# 能力对账全集（"不漏"的机械保证）

> 基准是**产品能力**，不是现有 API 数量——现有契约含 AI 瞎改的冗余/错误端点，不能当全集。
> 覆盖回去前，本表两部分都必须逐项闭环（✅ 已在 backend-new 实现 / ⏭️ 判定不要 / 🔒 boundary-kept）。
> 端点级明细在进入对应模块那一轮，从 `docs/references/backend/api.md` 提取后补到该模块 round 记录。

## A. 产品能力清单（意图级，稳定基准）

| 能力域 | 能力 | backend-new |
|---|---|---|
| 基础对话 | apikey BYOK + 测试 / model 场景策略 / conversation CRUD+导出 / chat 流式+工具+标题 / compaction | ⬜ |
| Quadrinity | function 版本+run+沙箱 / handler 版本+call+实例 / agent 版本+invoke+执行历史 / workflow 版本+trigger+5node / forge :iterate+:triage | ⬜ |
| Durable 执行 | flowrun 触发/重放/失败/trace/审批 / trigger cron·fsnotify·webhook 注册 | ⬜ |
| 知识与记忆 | document 树 CRUD + LLM-ranked attach（**无 RAG**） / memory 跨对话事实 | ⬜ |
| 集成与系统 | MCP 注册+调用 / SSE 三流(eventlog·notifications·forge) / permissions·sandbox / user 初始化 | ⬜ |
| 灰色（判定后定） | ask·askai / todo / answers·scenarios·prompts·capabilities·metrics·usage 端点 | ⬜ |

## B. 现有物理全集快照（覆盖前逐项核对，确保不漏）

> 每项最终状态：✅ 在 backend-new 有对应 / ⏭️ 判定删除·合并 / 🔒 boundary-kept。

- **domain（27）**：agent apikey catalog chat conversation crypto document errors eventlog flowrun forge⚠️ function handler mcp memory mention model notifications permissions relation sandbox skill subagent todo⚠️ trigger user workflow
- **app（27 + tool 子包 17）**：agent apikey ask⚠️ askai catalog chat contextmgr conversation document function handler hooks loop mcp memory model relation sandbox scheduler🔴 skill subagent todo⚠️ tool trigger user workflow ／ tool/{agent ask⚠️ document filesystem function handler mcp memory permissionsgate search shell skill subagent todo toolset web workflow}
- **infra（14）**：chat crypto db eventlog forge⚠️ handler llm logger mcp notifications sandbox settings store trigger ／ store(22): agent apikey approval chat conversation document flowrun flowrunevent function handler mcpcalls⚠️ mcphealth⚠️ memory model modelcapoverride⚠️ relation sandbox skillexec todo trigger user workflow ／ trigger(cron fsnotify polling⚠️ webhook)
- **pkg（20）**：agentstate envfix eventlog forge⚠️ idgen installprogress jsonrepair limits llmclient llmcost llmparse modelcaps⚠️ modelcatalog notifications pagination pathguard reqctx tokencount userpath wikilink
- **transport handlers（36）**：agent answers⚠️ apikey capabilities⚠️ catalog chat context_stats⚠️ conversation dev*⚠️(×5) document eventlog flowrun forge function handler health mcp memory metrics⚠️ model notifications permissions prompts⚠️ providers registrar relation sandbox scenarios⚠️ skills usage⚠️ users util workflow
- **cmd（7）**：coverage-matrix desktop doc-lint doc-matrix lintprompts resources server

⚠️ = `criteria.md` 灰色/残留候选，进入对应波次时判定。🔴 = 重灾区。
