---
id: WRK-034
type: working
status: active
owner: @weilin
created: 2026-06-21
reviewed: 2026-06-21
review-due: 2026-09-19
audience: [human, ai]
landed-into:
---

# 「模型与 Key」设置页重做 —— 后端对齐 + 三段式设计

> 来源：3 路并行后端调研 + 逐路反核验（API Key 域 / 默认模型选择 / 搜索供应商），全部对到 `backend/internal/…:行`。
> 用法：demo「模型与 Key」页（`demo/features/settings/{data,sea}.js`）重做的设计依据 + 真前端对接契约。
> **旧设计已否决**：免费档单独卡 + 静态默认模型行 + 散 key 段。后端实际是「key 列表(含免费档) + ModelRef 联动 + 搜索 key 同套」。

## 后端事实（设计据此，已核验到代码行）

### Provider 目录 — `GET /api/v1/providers`（豁免 workspace header）
`ProviderMeta = {name, displayName, defaultBaseUrl, baseUrlRequired, managed, category}`。共 17 项（`app/apikey/providers.go:54-73`）：
- **13 LLM**（category=llm）：openai · anthropic · google(Google Gemini) · deepseek · **anselm("Anselm Free (DeepSeek)", managed:true)** · openrouter · qwen(通义千问) · zhipu(智谱 GLM) · moonshot(Moonshot Kimi) · doubao(字节豆包) · ollama(baseUrlRequired) · custom(baseUrlRequired) · mock。
- **4 search**（category=search）：brave(Brave Search) · serper(Serper.dev) · tavily(Tavily) · bocha(博查 Bocha)。
- `managed:true` 仅 anselm；`baseUrlRequired:true` 仅 ollama/custom。

### Key — 一张表 `api_keys`，LLM/搜索共用
- 写字段：`provider, displayName, key, baseUrl, apiFormat`（`app/apikey/apikey.go:72-78`）。校验：key 对所有 provider 必填；ollama/custom 必填 baseUrl；custom 必选 apiFormat ∈ {openai-compatible, anthropic-compatible}。无 org/header/azure。
- 回显仅 `keyMasked`（永不回明文）。`test_status` ∈ {ok, error, pending} → 状态点。
- 端点（`transport/http/handlers/apikey.go:31-41`）：`POST /api-keys` · `GET /api-keys?cursor&limit&provider=`（仅 provider 过滤、**无 category**）· `PATCH/DELETE /{id}` · `POST /{id}:test`（→ `{ok,message,latencyMs}`，失败 422 `API_KEY_TEST_FAILED`+`{latencyMs,reason}`）。
- 删除被引用 → 422 `API_KEY_IN_USE`，`details.references[{kind,id,name}]`，kind ∈ {scenario_default, search_default, agent_override}。

### 免费档（anselm）— key 列表里的一行
- provider=anselm、displayName="Anselm Free (DeepSeek)"、`managed:true`、**不可改不可删**（改 422 `API_KEY_IMMUTABLE`）、自动开通（per-workspace）。
- 唯一模型 `deepseek-v4-flash`、**零 knobs**（网关剥 thinking，`infra/llm/anselm.go:45-58`）。
- 配额：`GET /api/v1/freetier/quota` → `{limit, used, remaining, resetAt, available}`（未开通 404 `FREETIER_NOT_PROVISIONED`）。

### 默认模型 — ModelRef + 三场景
- `ModelRef = {apiKeyId, modelId, options}`（`domain/model/model.go:24-28`），provider 由 apiKeyId 隐含。options = 原生 `map[string]string`，**后端不校验、前端唯一守门人**。
- 三场景（封闭集，`model.go:52-56`）：`dialogue`(对话+subagent) · `utility`(标题·摘要·triage·压缩·搜索精选) · `agent`(Agent 实体)。存 workspace 三 JSON 列。
- 端点：`PUT /workspaces/{id}/default-models/{scenario}` body `{apiKeyId, modelId, options}`；`DELETE` 清。
- 模型列表 + 每模型可配项：`GET /api/v1/model-capabilities` → `CapabilityView[{apiKeyId, keyName, provider, modelId, displayName, contextWindow, maxOutput, vision, nativeDocs, knobs}]`（`app/model/capability.go:25-36`）。加 key 时 probe 存档 + 静态目录解析，**非运行时探测**；目录外的 model id 无 knobs（用户仍可手填 modelId）。
- **Knob = `{key, label, type(enum|int|bool), values, default}`**（`infra/llm/provider.go:178-184`），原生、**per-model**（换 model 换一套）：
  - Anthropic Opus4.8：thinking(adaptive/disabled) + effort(low/medium/high/xhigh/max, 默认 high)
  - OpenAI GPT-5：reasoning_effort + verbosity(low/medium/high)
  - DeepSeek：thinking(enabled/disabled) + reasoning_effort(high/max)
  - Gemini-3：thinkingLevel；Qwen：enable_thinking + budget；其余各异
  - **anselm 免费档：零 knobs**
- **1M ≠ 开关**：是 model 固有 `contextWindow`（只读，如 Opus4.8=1000000）。thinking 才是 knob。

### 搜索 Key — 同 ① 一套
- 同表同端点同 `:test`（`search_ping`，真发 count=1 探测）。4 家：brave/serper/tavily/bocha。
- 默认搜索 key：`PUT /workspaces/{id}/default-search` body `{apiKeyId}`；读 `workspace.defaultSearchKeyId`（""=未配）。**后端不校验 category**（`app/workspace/workspace.go:367-386`）——前端唯一守门人，只让选 search key。
- 列表过滤坑：`GET /api-keys` 无 `?category=`，列「搜索 key」须前端按 provider∈{brave,serper,tavily,bocha} 筛。

## 三段式设计（demo 落地）

### ① API Key（含免费档）
- **列表**：每行 = provider 图标 + 名 + displayName + keyMasked + 状态点(test_status) + 模型数。
  - 免费档行：managed 徽 + 配额条（剩 X/Y · 重置），无编辑/删除。
- **新建**：列表底一个**虚线框 + 居中加号**（与 key 行同高）→ 点开**大下拉**列 13 LLM provider（图标+名）→ 选中 → 下方展开**配置区**：key 输入(必)、baseUrl(ollama/custom 显)、apiFormat 二选(custom 显) + **测试** + 保存。

### ② 默认模型（三场景联动）
- 三行：对话 / 工具 / Agent。每行右侧 = **API → 模型 → 配置** 联动：
  - 选 API（用户已配的 ok key）→ 模型下拉（该 key 在 capabilities 里的 model）→ 配置（选中 model 的 knobs 动态渲染：enum→下拉、bool→开关、int→输入；各取 Knob.default）。
  - 上下文窗口（如「1M 上下文」）作**只读信息**展示，非开关。
  - 免费档 key 选中 → 模型 deepseek-v4-flash、配置区空（零 knobs）。

### ③ 搜索引擎 Key
- **列表**：已配搜索 key（同 ① 行式）。**新建**：虚线框+加号 → 大下拉列 4 搜索 provider（图标+名）→ 配置区(key + 可选 baseUrl) + 测试。
- 「默认搜索 key」选择（在已配搜索 key 里选）。

## demo mock 数据形（`demo/features/settings/data.js`）
- `providers`：17 项 ProviderMeta + 前端图标映射（图标后端不给、前端配）。
- `keys`：已配 key 列表（含 anselm managed 行 + quota）。
- `modelCaps`：几把 key × model 的 CapabilityView（带 knobs）。
- `defaults`：三场景当前 ModelRef；`defaultSearchKeyId`。
