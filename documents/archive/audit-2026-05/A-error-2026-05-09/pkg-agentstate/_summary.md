# Package audit summary: internal/pkg/agentstate

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: 本包是**纯内存运行时状态**——sync.Map / Mutex-protected fields / atomic.Pointer 都不返 error。1 LOW 在 skill.go::extractPrimaryArg（site#6）静默吞 json.Unmarshal 错误——属 fail-safe 设计但缺 telemetry，用户视角等价于"skill 配置错的误报"。
- **§S9 detached ctx 终态写**: N/A — 本包是 in-memory state，**没有 DB/网络/事件落地**。任务 prompt 关注的"AgentState 并发安全"已通过 sync.Map (SeenFiles) + sync.Mutex (cwd/subTokens) + atomic.Pointer (activeSkill) 三层并发原语守住。**真正的 §S9 终态写责任在调用方**——如 app/subagent.Service 在 sub-run 终结时既调 AddSubagentTokens（本包）又写 DB（subagentstore），那个 DB 写才是 §S9 关注点（该包审时再查）。
- **§S15 ID 生成**: N/A — 本包接收 RunID / TypeName 作为参数，不生成 ID。
- **§S16 错误 wrap 格式**: N/A — 本包 0 个 error 返回值。
- **§S17 errmap 单一事实源**: N/A — 本包 0 个 sentinel。

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| agentstate.go | 131 | 9 | 8 | 0 | 0 | 1 |
| skill.go | 211 | 8 | 7 | 0 | 1 | 0 |
| **TOTAL** | **342** | **17** | **15** | **0** | **1** | **1** |

## Severity breakdown

| Severity | Count | Status |
|---|---|---|
| HIGH | 0 | — |
| MED | 0 | — |
| LOW | 1 | skill.go site#6 §S3 — extractPrimaryArg 静默吞 unmarshal err |

**Net: 1 LOW violation**.

## Cross-cutting

### 并发安全总评（任务 prompt 关注点）

AgentState 使用三种并发原语：

| 字段 | 原语 | 适用理由 |
|---|---|---|
| SeenFiles | sync.Map | reads vastly outnumber writes (every Edit/Write check vs occasional Read mark)；key-by-key 隔离 |
| cwd | sync.Mutex | 单一字符串，读写都需互斥；互斥成本 << 字段大小 |
| subTokens (slice) | sync.Mutex | append 必须串行化（slice 头扩容竞态）；godoc 行 36 显式 "mutex 串化 slice 增长" |
| activeSkill | atomic.Pointer | 读 >> 写 + 指针不可变（Activate 后不再改 Skill struct） |

每种原语的选择都有 godoc 内注释解释——concurrency-design 显式而非 "随便选一个"。

**潜在 race**:
- `SubagentTokenLog()`（agentstate.go:85-91）持锁拷贝 slice 后返——返回值在 caller 处 lock-free 安全
- `IsToolPreApprovedBySkill`（skill.go:78-89）Load 后再 Range AllowedTools——AllowedTools 来自 skilldomain.Skill 的 Frontmatter（Activate 后不变，per godoc 行 47-52 "concurrent SetActiveSkill can swap underneath but won't mutate the existing skill struct"）
- `MarkRead` + `WasRead` 不需要原子读改写——MarkRead 是 sync.Map.Store（原子），WasRead 是 Load（原子），无需更高层同步

并发模型设计正确。

### Fail-safe vs Fail-silent 区分

skill.go 有两处看起来像吞错的 site：

| Site | 错误源 | 处理方式 | 是否 fail-safe? | 是否合规? |
|---|---|---|---|---|
| #5 | 畸形 pattern（skill 文件 author bug） | 返 false → 拒授权 | ✅ | ✅ godoc 显式说明 |
| #6 | 畸形 LLM args（runtime 异常） | 返 "" → 拒授权 | ✅ | ⚠️ 缺 telemetry — LOW |

判断依据：
- **错误源是已知/期望发生** → fail-safe 即可（site#5：author 写错 skill 是预期 case）
- **错误源是非期望/异常** → fail-safe + telemetry（site#6：LLM 不应产生畸形 args，发生 = 信号）

**S20 当场修风险**: 触发"留下次"风险。修复需要：
- (a) 结构性硬约束: AgentState 没有 logger 字段——加 logger 是侵入性改动；可以通过传入 logger 参数或返 (string, error) 修复，需要改 callsite。
- 故 LOW + FOUND 待批量修——但需要在 fix-batch 设计层评估"侵入 vs 注释化"两条路

### §S9 责任在调用方（重要）

任务 prompt 提"agentstate：SeenFiles map + AddSubagentTokens 并发安全？"——**并发安全已守**（mutex/sync.Map/atomic 三层），但 §S9 检查需要重新框定：

- 本包是**内存 state**（不写 DB）→ 没有"终态落库"概念
- AddSubagentTokens 看似终态写但只写到内存 slice → 不是 §S9 范畴
- 真正 §S9 责任：调用方（app/subagent.Service / app/chat.Service）在写 DB 时是否用 detached ctx——该 audit 各自包时验

**误判风险**：若把 AddSubagentTokens 看成"终态写需要 detached ctx"，会产生 false-positive 让调用方传错 ctx。本包**不要求** caller 传 ctx（方法签名无 ctx 参数）——这是设计上规避了 §S9 责任，正确选择。

### Skill ActiveSkill 旁路 (skill.md §9.5)

`activeSkill` 是 atomic.Pointer 而非 RWMutex 的设计选择写在 skill.go:200-209——godoc 引用 skill.md §9.5 的设计文档。这是**设计层决策**正确进入代码注释，符合 §S11 注释规范"决策叙事去 design doc，注释只留结论"；本注释主体是结论 + 三个理由（reads >> writes / 指针不可变 / race 良性），未变成长篇心路历程，符合长度上限。

## Recommended fix priorities

1. **LOW** — skill.go site#6: extractPrimaryArg 的 json.Unmarshal err 至少留 telemetry。两条路：
   - **Option A** (轻): 内联注释明确"LLM 畸形 args 是 unusual case，期望发生频率为零；fail-safe 是有意"——把未表达的设计意图显式化，不改代码。
   - **Option B** (重): 改函数签名为 `(string, error)` 让调用方处理；或在 IsToolPreApprovedBySkill 注入 logger 在调用 extractPrimaryArg 后判断 args 长度 vs 解析结果记录 anomaly。
   
   Option A 投入小但仍有"LLM 行为漂移看不到"的盲点；Option B 才真治本。fix-batch 阶段决定哪条路。

## Out-of-scope notes

1. **任务 prompt 提到的 "token log"** ——本包提供 in-memory 累积器（SubagentTokenEntry slice）；token log 持久化是 app/subagent 层的职责（写入 messages 表的 attrs JSON）。本包 audit 仅覆盖内存层并发安全。
2. **AgentState 的生命周期** ——godoc 行 9 说"stamped by chat/runner.go::processTask once per agent task"——一个 agent task 一个 AgentState 实例，对话结束 GC。这避免了 long-lived state 的内存累积——属设计正确，不是 audit issue。
3. **wildcardMatch 边界 cases** ——godoc 行 158-166 给出 6 个例子覆盖 leading / trailing / embedded `*`、anchor 边界。本包 audit 不验测试覆盖率（不读 _test.go），但 godoc 例子足够清晰，假定测试已覆盖。
