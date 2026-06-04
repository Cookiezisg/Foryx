# Round 0020 — model（波次 1 · M1.3）+ 连带 infra/llm 11 家重构 + apikey 探针 + workspace 默认

类型 / 目标：M1.3 model 模块重写，**大幅收窄为「聚合+展示薄层」**，并连带**重构 infra/llm 全部 11 家 adapter 的旋钮模型**。设计经多轮讨论 + 10 家官方调研（2026-06-04）敲定。设计蓝本 `contracts/model.md`，调研存档 `rounds/0020/research-llm-knobs.md`。

## 核心方针（一句话）
**模型知识下沉各家 provider 自包含；model 退化成「聚合+展示」薄层；删中立抽象、全归各家原生。**

## 关键设计决策（经讨论拍板）
1. **删中立抽象 `ThinkingSpec`**：4+6 家调研证明没有两家旋钮相同（连 Gemini 自家 2.5↔3.x、OpenAI 自家版本都不同），任何 `{Mode,Effort,Budget}` 归一都碎。`Request.Options map[string]string`（原生 key→原生值）成为**唯一**旋钮载体。
2. **取值绝不自创**：原名原值（`xhigh`/`max`/`thinkingBudget=-1`/`enable_thinking`…），删旧自创的 `max` 归一档与各家 `xxxMapEffort/clampEffort`。
3. **`Knob{key,label,type,values,default}` 描述容器**：容器统一、内容全原生；前端据此通用渲染。是「自描述信封」非「语义模子」。
4. **模型知识下沉 provider**：窗口/上限/旋钮/wire 编码全归各家 `infra/llm/<家>.go`（`Provider.DescribeModels(rawProbe)` 解析自家 /models + 静态目录）。**provider 自填 max_tokens**（`Request.MaxTokens` 改可选，0→provider 默认），删 M0.6「caller 从 catalog 派生」临时接缝。跨家 `pkg/modelcatalog` **弃用不迁**。
5. **动态 vs 静态因家而异**：富 /models（anthropic 现有 /v1/models、gemini ListModels、moonshot、openrouter）解析规格；贫（openai/deepseek/qwen/zhipu/doubao 仅 id 或无端点）用静态目录。**旋钮定义所有家都静态**（/models 永不返回"支持哪些档/范围/互斥"）。
6. **默认选择搬 workspace**：3 列 `default_{dialogue,utility,agent}`（`*ModelRef` JSON，orm `db:"...,json"`），与 language 并列。`ModelPicker` 由 `app/workspace.Pick` 实现。
7. **override 弃删除时保护、改运行时优雅报错**：删 key 只查 workspace 三默认；各实体 override 引用是弱引用，key 没了运行时报错（`MODEL_NOT_CONFIGURED`）。
8. **Resolve 收口**：`model.Resolve(scenario, override, picker)` 把「override 优先否则默认」从十几个 caller 收成一处（caller 在波次 2/3/5）。

## 调研（官方文档 2026-06-04，见 research-llm-knobs.md）
4 家深查（anthropic/openai/gemini/deepseek）+ 6 家补查（qwen/zhipu/moonshot/doubao/openrouter/ollama）。重大纠偏：**Anthropic 现有富 `/v1/models`**（推翻"无端点"旧断言）；**Anthropic `budget_tokens` 在 Opus 4.7/4.8 已废→400**（M0.6 adapter 旗舰上是坏的，本轮修）；**DeepSeek V4 起 thinking 改请求参数**（非选模型）。

## 删 / 移交
- **弃用不迁**：`pkg/modelcatalog`（37 行跨家大表）、`llm.ThinkingSpec`、`model_configs` 表 / `domain/model.Repository` / `infra/store/model` / 旧 app CRUD+Picker。
- **确认无残留**：`infra/store/modelcapoverride` 空目录、`pkg/modelcaps` 不存在 → backend-new 从零写未迁（**order/STATE 旧旗标作废**，doc-fix）。
- **doc-fix**：`pkg/limits/limits.go` 注释提 `modelcatalog`（已弃）→ 改述。

## 新实现
- **infra/llm 地基**：删 `ThinkingSpec`；`Request` 改 `Options` 唯一承载 + `MaxTokens` 可选；新增 `ModelInfo`/`Knob` 类型 + `Provider.DescribeModels`；`models_common.go`（`modelSpec`/`matchSpec`/`describeFromSpecs`/`decodeOpenAICompatModelIDs`/`enumKnob`/`boolKnob`/`intKnob` 共享地基）。
- **11 家 adapter**：各自 BuildRequest 读 Options 原生 key + `DescribeModels` + 静态目录/富解析。anthropic（thinking+output_config.effort，修 budget_tokens bug、删 1M header）；openai（reasoning_effort+verbosity）；gemini（thinkingBudget 2.5/thinkingLevel 3.x 代际，富 ListModels）；deepseek/qwen/zhipu/doubao/moonshot（OpenAI-compat 模板）；openrouter（富 /models + supported_parameters 推 knob）；ollama（think+num_ctx，/api/tags）；custom（不发旋钮）。
- **domain/model**（薄）：`ModelRef`+`Scenario`+`IsValidScenario/ListScenarios`+`ModelPicker` 接口+`Resolve`+`MODEL_*` 3 sentinel。**无 Repository/无 store**。
- **app/model**：`CapabilityService`（经 `apikey.ProbeReader.ListProbed`→`llm.DescribeModels`→聚合 `[]CapabilityView`，含 apiKeyId/keyName 归属 + Knobs）。
- **workspace**：domain 加 3 列 `*ModelRef`(json) + `DefaultFor/SetDefaultFor`；store DDL +3 列；app `Pick`(实现 ModelPicker) + `SetDefault`；handler `PUT /workspaces/{id}/default-models/{scenario}`。
- **apikey**：anthropic 探针 `anthropic_ping`→`anthropic_models`（打 /v1/models）；`ProbedKey` +id+displayName（model 归属）。
- **handler**：`GET /model-capabilities`（聚合，去 modelsFound）+ `GET /scenarios`（白名单 exempt）。

## 测试
model domain 8（Resolve override/fallback/zero/invalid-scenario/not-configured + Validate + scenario）· model app 2（capability 聚合 + 跳过非 OK/解析失败 + 空）· workspace +4（SetDefault↔Pick 往返 / not-configured / ref-invalid / scenario-invalid）· 11 家 llm adapter test 全迁移（ThinkingSpec→Options 原生，删归一化函数测试）。

## 验证
`gofmt -l` 干净 · `go build ./...` 0 · `go vet ./...` 0 · `go test ./... -race` 全 ok。

## 工程方式
4+6 家调研用 10 个并行 agent；11 家 adapter 重构「自己写 5 特殊家(anthropic/gemini/openrouter/ollama/custom) + agent 改 6 模板家」；11 家 test 迁移用 2 个并行 agent；契约文档同步用 1 agent。

## 是否更干净
旧：选+知识+编译三合一、跨家中立大表、自创档名、旗舰 budget_tokens 报错、modelsFound 双存储。
新：知识各家原生自包含、model 仅聚合无 store、删中立抽象、修真 bug、单一数据源（apikey 存原始 / model 解析）。✅

## 契约（→ contract-changes #5）
删 `model-configs` 端点 + 表；新增 `default-models` 端点 + workspace 3 字段/列；`model-capabilities` 重写(knobs 原生)、去 modelsFound；`MODEL_*` 3 error；apikey anthropic 探针内部改(envelope 不变)。

## 遗留 / 下一步
- **M1.4 relation**（波次 1 续）。
- override 消费者（chat/agent/node 读 override + 调 Resolve + 各自 RefScanner 扫 override）→ 波次 2/3/5 各自实现。
- 静态目录数值随软件更新人工对账（OpenAI 迭代快）。
