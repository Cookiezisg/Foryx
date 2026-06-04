# R0020 调研存档 — 4 家 LLM provider 配置规格（2026-06-04）

> 数据来源：各家**官方文档**现场抓取（4 个并行调研 agent，2026-06-04）。原生术语**原样保留、绝不归一化**。
> 用途：M1.3 model 重构的事实基础——论证「各家自包含」，并作各家 adapter 实现模板。
> ⚠️ 模型迭代极快：本文件是 **2026-06 快照**，静态部分需随软件更新**人工对账**。

---

## 0. 对比总览（最有杀伤力的一张表）

| 维度 | Anthropic | OpenAI | Gemini | DeepSeek |
|---|---|---|---|---|
| **推理控制** | **双正交**：`thinking`(adaptive/enabled/disabled) + `output_config.effort`(low/medium/high/xhigh/max) | `reasoning_effort`(none/minimal/low/medium/high/xhigh) + 另有 `verbosity` | 2.5=`thinkingBudget`(数字 -1/0/范围) · 3.x=`thinkingLevel`(minimal/low/medium/high)，**互斥(传俩→400)** | `thinking`(enabled/disabled) + `reasoning_effort`(**仅 high/max**) |
| **取值能否归一** | xhigh 仅 Opus4.8/4.7；max 部分 | **none≠minimal 不通用；默认逐版本翻转**(5.5=medium,5.4/5.1=none) | **自家两代形态都不同** | low/medium 被映射吞成 high |
| **1M / 长上下文** | 选模型自带、**无 header**(旧 context-1m header 已失效) | 无开关、随模型 | 无开关、随模型 | 无开关、V4 默认 1M |
| **输出上限参数** | `max_tokens`(**必填**) | `max_completion_tokens`(老 `max_tokens` 废、o 系列不认) | `maxOutputTokens`(可选) | `max_tokens`(可选) |
| **推理吃 output 预算** | adaptive 下 max_tokens 是硬 cap | **是**(含 reasoning，设小→空截断) | **是**(设小→空 MAX_TOKENS) | 含 CoT |
| **`/models` 富贫** | **富**(max_input_tokens/max_tokens/capabilities) | **贫**(仅 id/created/owned_by) | **富**(inputTokenLimit/outputTokenLimit/thinking 布尔) | **贫**(仅 id/object/owned_by) |
| **多轮思考回传** | `signature` 原样回传 | (Responses 体系) | `thoughtSignature`(3.x 函数调用强制) | **`reasoning_content` 必回传、否则 400** |

**核心结论：没有任何两家一样，连 Gemini 自家两代、OpenAI 自家几个版本都不一样 → 任何中立抽象当场碎。**

---

## 1. Anthropic（Messages API，`POST /v1/messages`，header `anthropic-version: 2023-06-01`）

- **推理 = 两个正交旋钮**：
  - `thinking`：`{type:"adaptive"}` / `{type:"enabled", budget_tokens:N}` / `{type:"disabled"}`。`budget_tokens` ≥1024 且 `< max_tokens`。
  - `output_config.effort`：`low|medium|high|xhigh|max`（**就这 5 个**，无 `none`），默认 `high`（≡省略）。作用于**所有 output**（文本+工具+思考），thinking 关也生效。
- **⚠️ 真 bug 来源**：`budget_tokens` 在 **Opus 4.7/4.8 已废弃 → 传了 400**；4.7/4.8 **只认 `thinking.type:"adaptive"`**。我们 M0.6 的 anthropic adapter 正是用 `budget_tokens` 写的 = 旗舰上坏的。
- **1M**：Opus 4.8/4.7/4.6 + Sonnet 4.6 **GA、无 header**；旧 `anthropic-beta: context-1m-2025-08-07` 已失效。其余 200K。
- **max_tokens**：必填；`0` = 缓存预热（不生成）。上限随模型（Opus 128K / Sonnet·Haiku 64K）。
- **`/v1/models` 富**：每个对象带 `max_input_tokens`、`max_tokens`、`capabilities{effort{high/low/max/medium/xhigh 布尔}, thinking{types.adaptive/enabled}, image_input, pdf_input, structured_outputs, ...}`。⚠️ 文档**示例里数值是占位 0**，真实数值要实连一次。
- **最特立独行**：thinking(要不要想) 与 effort(花多大力) 拆成两个正交旋钮；旗舰废弃数字预算、改软性 effort 信号。
- 来源：platform.claude.com /docs/en/{build-with-claude/extended-thinking,adaptive-thinking,effort,context-windows · api/messages,models-list · about-claude/models/overview}

## 2. OpenAI（Chat Completions；Responses API 为官方推荐迁移路径）

- **推理 = 请求参数 `reasoning_effort`**，全集 `none|minimal|low|medium|high|xhigh`，**按模型开子集**：gpt-5.5=`none/low/medium/high/xhigh`(默认 medium)；gpt-5.4/5.1 默认 **`none`**(默认不思考)；原版 gpt-5=`minimal/low/medium/high`。gpt-4.1/4o **不支持**(非推理模型)。
- **wire 路径分裂**：Chat Completions 顶层扁平 `reasoning_effort`；Responses 嵌套 `reasoning.effort`。另有 `verbosity`(low/medium/high) 同样分裂(顶层 vs `text.verbosity`)。
- **坑**：`none`≠`minimal` 且互不通用、跨版本默认翻转 → **不能假设"不传=中等"**。
- **1M**：无开关，随模型(gpt-5.5/5.4=1,050,000；gpt-4.1=1,047,576，口径不同别统一成"1M")。
- **输出上限**：`max_completion_tokens`（老 `max_tokens` 已废、**o 系列不兼容**）；**含 reasoning tokens**，设小→可见输出空。可选、无固定默认。
- **`/v1/models` 贫**：仅 `{id,object,created,owned_by}`，**无任何规格/能力** → 窗口/上限/是否推理/effort 子集**全静态硬编码**，且因迭代快需人工对账。
- **最特立独行**：把"想多久"和"说多长"塞进同一个 `max_completion_tokens` 预算；`none`/`minimal` 两个不通用的零推理档。
- 来源：developers.openai.com /api/docs/{guides/reasoning,guides/latest-model,models/<id>} · /cookbook/examples/gpt-5/* · /api/reference/resources/{models,chat}

## 3. Google Gemini（`generativelanguage.googleapis.com/v1beta`）

- **推理 = `generationConfig.thinkingConfig`，但按代变形**：
  - **2.5**：`thinkingBudget`(int)，`-1`=动态、`0`=关(部分模型不可关)、正数=目标；范围随模型(2.5-pro 128–32768 不可关；flash 0–24576)。
  - **3.x**：`thinkingLevel`(`minimal|low|medium|high`)，**不能关**；默认随模型(pro/flash=high、flash-lite=minimal、3.5-flash=medium)。
  - `includeThoughts`(bool)、`thoughtSignature`(3.x 函数调用/图像**强制**回传)。
  - **⚠️ 2.5 与 3.x 互斥**：同请求传 `thinkingLevel`+`thinkingBudget` → **400**。
- **wire**：REST JSON key 一律 camelCase(`thinkingConfig/thinkingBudget/thinkingLevel/includeThoughts`)。
- **1M**：随模型固定、无开关(主线全 1,048,576)。无 2M。
- **输出上限**：`generationConfig.maxOutputTokens`(可选)；thinking **吃同一预算**，设小→空 `MAX_TOKENS` 响应。
- **`/v1beta/models`(ListModels) 富**：`inputTokenLimit`、`outputTokenLimit`、`thinking`(bool)、`supportedGenerationMethods`、`temperature/topP/topK` 默认。**但 thinkingLevel 枚举/budget 范围/互斥规则不在内** → 仍要静态。
- **最特立独行**：思考旋钮按代际换形(数字↔枚举)且互斥；thinking 挤占 output 预算。
- 来源：ai.google.dev /gemini-api/docs/{thinking,gemini-3,models,models/<id>,long-context,tokens} · /api/{models,generate-content}

## 4. DeepSeek（`api.deepseek.com`，OpenAI-compatible）

- **范式已翻转(2026-04-24 V4 起)**：从「选模型(`deepseek-chat`/`deepseek-reasoner`)」→「**传参数**」。两别名降级为指向 `deepseek-v4-flash` 的 non-thinking/thinking，**2026-07-24 退役**。
- **推理 = 两参数**：`thinking`:`{type:"enabled"|"disabled"}`(默认 enabled)；`reasoning_effort`:**原生仅 `high`/`max`**(默认 high)。OpenAI 生态的 `low/medium`→映射成 `high`、`xhigh`→`max`（DeepSeek 自身只认两档）。
- **⚠️ 多轮工具调用坑**：上一轮 assistant 的 `reasoning_content` **必须原样回传**进上下文、否则 **400**（与几乎所有家"思考只读、回传即丢"相反）。
- **1M**：V4 默认全开、无开关。(V3.1 时代是 128K → 打到 128K 说明是旧端点/私有部署)
- **输出上限**：`max_tokens`(可选)；含 CoT；V4 上限 384K，**默认值官方未公开 → 客户端应显式传**。
- **`/models` 贫**：仅 `{id,object,owned_by}`，无规格 → 窗口/上限/旋钮静态内置。
- **最特立独行**：推理改参数控制(非选模型)；`reasoning_effort` 原生只两档；`reasoning_content` 强制回传。
- 来源：api-docs.deepseek.com /{guides/thinking_mode,api/create-chat-completion,api/list-models,quick_start/pricing,news/news260424}

---

## 5. 对 M1.3 的设计结论（→ 详见 contracts/model.md）

1. **中立抽象死刑**：删 `llm.ThinkingSpec{Mode,Effort,Budget}`。无法表达双正交/代际变形/互斥/两档/none≠minimal。
2. **取值绝不自创**：原名原值（`xhigh`/`max`/`thinkingBudget=-1`…），删旧自创的 `max` 归一档。
3. **动态 vs 静态因家而异**，且**旋钮定义所有家都静态**（`/models` 永不返回"支持哪些 effort 档/范围/互斥/默认"）：
   - 富(Anthropic/Gemini)：窗口/上限/部分能力可从 `/models` 动态拿；
   - 贫(OpenAI/DeepSeek)：仅 id 列表动态，规格全静态。
4. **旋钮粒度 = (provider, model)**：OpenAI 每模型 effort 子集不同、Gemini 2.5≠3.x → `Knobs(modelID)` 而非 `Knobs()`。
5. **跨家共性陷阱**：reasoning 吃 output 预算（OpenAI/Gemini/DeepSeek 都是）→ provider 自填 max_tokens 时要留思考余量。
6. **6 家补充已调研完毕**（见 §7）；custom 泛型不查。

---

## 7. 6 家补充调研（2026-06-04，OpenAI-compat 批 + 聚合 + 本地）

### qwen（DashScope OpenAI-compat，`/compatible-mode/v1`）
- 旋钮（**顶层**，extra_body 是 Python-SDK 假象）：`enable_thinking`(bool) + `thinking_budget`(int，仅 thinking 模式)。
- 默认开：qwen3.5-*、qwen3 开源；默认关：qwen-max/plus/flash/turbo、qwen3-max（须 opt-in）；强制开（no-op）：qwq、qwen3-max-thinking。
- 1M 无开关随模型。`max_tokens` 可选，默认=模型 max output。**/models 无文档**（贫，best-effort id-only）。
- 规格：`qwen3-max` 262144/32768 · `qwen-plus` 1000000/32768 · `qwen-flash` 1000000/32768 · `qwen-turbo` 131072/16384 · `qwen-long` 10000000/32768 · `qwen3.5-plus` 1000000/65536 · `qwen-max` 32768/8192。

### zhipu（bigmodel.cn `/paas/v4`）
- 旋钮：`thinking{type: enabled/disabled}`（+ `clear_thinking` bool 默认 true）。默认 `enabled`（GLM-4.5+）。
- 1M 无开关（`glm-4-long` 1M/4K，主力 200K）。`max_tokens` 可选。**无 /models 端点**（贫）。
- 规格：`glm-5.1` 200K/128K · `glm-5`/`glm-5-turbo` 200K/128K · `glm-4.7`(+flash/flashx) 200K/128K · `glm-4.6` 200K/128K · `glm-4.5`*(air/airx/flash) 128K/96K · `glm-4-long` 1M/4K。坑：`do_sample` 总闸；thinking 默认开。

### moonshot（api.moonshot.cn/v1；platform→kimi.com）
- 旋钮：`thinking{type: enabled/disabled, keep("all"/null 仅 k2.6)}`（**仅 kimi-k2.6/k2.5**）。默认 `enabled`（k2.6）。**无 reasoning_effort**。
- 1M 无（256K 封顶）。`max_tokens` 弃用→`max_completion_tokens`（可选，k2.6 默认 32768、通用 1024）。
- **/models 富**：`context_length` + `supports_reasoning` + `supports_image_in` + `supports_video_in`。
- 规格：`kimi-k2.6` 262144 · `kimi-k2.5` 262144 · `moonshot-v1-8k/32k/128k`(8192/32768/131072)。坑：k2.6 temp 锁 1.0；旧 `kimi-k2-thinking`/`kimi-k2-*-preview` 2026-05-25 下线（chat.md 枚举仍残留，以 /models 为准）。

### doubao（火山方舟 Ark `/api/v3`）
- 旋钮：`thinking{type: enabled/disabled/auto}` + `reasoning_effort`(minimal/low/medium/high/max，默认 medium，**力度档非 token，无 budget_tokens**)。默认随模型（deepseek-v3-2 默认 disabled）。`auto` 几乎只 seed-1-6 支持。
- 1M 无（doubao-seed 256K；1M 仅 Ark 托管的 `deepseek-v4-*` 1024k）。`max_tokens` 可选默认 4096 / `max_completion_tokens`[1,65536]，**二者互斥**。**无 /models**（404）。
- 规格：`doubao-seed-1-6*` 256K/32K · `doubao-seed-1-8` 256K/64K · `doubao-seed-2-0-pro/lite/mini` 256K/128K · `doubao-seed-character` 128K/32K。
- 坑：`model` 字段收带日期 model id（`doubao-seed-1-6-250615`，点号写法报错）或自建 `ep-xxx` endpoint id。

### openrouter（openrouter.ai/api/v1，聚合器）
- 旋钮：统一 `reasoning{effort: xhigh/high/medium/low/minimal/none ⊕ max_tokens(int) 二者互斥; enabled(bool); exclude(bool)}`。
- 1M 无开关（`:extended` 变体 / 选模型）。`max_tokens` 可选（别名 `max_completion_tokens`）。
- **/models 最富（全行业）**：`context_length` + `supported_parameters[]`（每模型支持哪些旋钮，可据此推 Knobs）+ pricing + architecture。id=`vendor/model`。
- 变体后缀：`:free`/`:extended`/`:thinking`/`:nitro`/`:floor`/`:exacto`。

### ollama（本地 `localhost:11434`）
- 旋钮：**顶层** `think`(bool；GPT-OSS 用 "low"/"medium"/"high"；**本地不收 "max"**) + `options.num_ctx`(客户端设窗口) + `options.num_predict`(输出，默认 -1 无限)。
- **/api/tags**（本地模型 id 列表，无能力）+ **POST /api/show**（`capabilities[]` 含 `thinking`/`tools`/`vision`；context 在 `model_info["{arch}.context_length"]` 架构前缀动态 key）。
- 坑：num_ctx 客户端设窗口（云 API 无此概念）；think 双形态（bool vs effort 档）；默认 num_ctx 文档三套口径（4096/VRAM 分档/2048）→ 客户端显式传。

### custom（泛型，不调研）
- 策略：**不发任何 thinking 旋钮**（通用端点不一定支持，发了可能 400）；/models 走 OpenAI-compat 解析（best-effort）；无静态 specs（端点未知）。

> **统一观察**：`thinking{type: enabled/disabled}` 是国产家最常见形态（zhipu/moonshot/doubao + deepseek）；effort 档是 openai/doubao/openrouter；qwen 用 `enable_thinking` 布尔 + budget 数字；ollama 用顶层 `think` 布尔 + 客户端 `num_ctx`。**没有两家完全相同**——再次印证零中立抽象、各家原生自包含。
