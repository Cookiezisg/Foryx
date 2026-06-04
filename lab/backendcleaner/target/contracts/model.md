# model 模块设计蓝本（M1.3）

> 范围：本轮 = **model 模块新建** + **连带重构 `infra/llm` 11 家 adapter** + **修 apikey anthropic 探针** + **workspace 加 3 列默认**。
> 事实依据：`rounds/0020/research-llm-knobs.md`（4 家官方调研，2026-06-04）。
> 一句话方针：**模型知识下沉各家 provider；model 退化成「聚合+展示」薄层；删中立抽象、全归各家原生。**

---

## 1. 定位（从「知识中枢」退化成「聚合+展示」薄层）

旧 model = 选择(ModelConfig 表) + 知识(pkg/modelcatalog 跨家大表) + 编译(中立 ThinkingSpec)。**三件全拆**：

| 旧职责 | 新归属 |
|---|---|
| 选择·默认（按 scenario 选 key+model） | **workspace 三列**（与 language 并列） |
| 选择·override（单实体临时换） | **各实体自己**的 `*ModelRef` 字段（agent/conversation/node；本轮不建消费者，仅留类型） |
| 知识（窗口/上限/旋钮/1M/effort） | **各家 provider 自包含**（`infra/llm/<家>.go`） |
| 编译（options→参数） | **消失**——provider 自读 Options、自填 max_tokens |

**model 模块只剩**：① 解析 apikey 探针存的 `test_response` → 聚合「能用哪些模型、每个怎么配」喂前端；② `ModelRef` 类型 + 校验 + override/默认 resolve 收口；③ scenario 白名单。**无 store、无数据库实体、无编译。**

---

## 2. 三层职责 + 数据流

### 层 A — `infra/llm` 各家 provider（自包含，全用原生术语）
`Provider` 接口扩两法（纯解析+静态、**不碰网络**，`providerClient` 不受影响）：
```
type Provider interface {
    Name() string
    DefaultBaseURL() string
    BuildRequest(ctx, req Request) (*http.Request, error)   // 已有：读 req.Options 编码自家旋钮
    ParseStream(ctx, resp, req) iter.Seq[StreamEvent]        // 已有
    DescribeModels(rawProbe string) ([]ModelInfo, error)     // 新：解析自家 /models 原始返回；富家出规格、贫家出 id+静态补
    Knobs(modelID string) []Knob                             // 新：该模型的旋钮声明（原名原值/默认/互斥），静态内置
}
```
包级入口供 model 调用（不暴露 Provider 实例）：`llm.DescribeModels(provider, raw)` · `llm.Knobs(provider, modelID)`。

### 层 B — `app/model`（聚合，无 store）
`CapabilityService.List(ctx)`：经 `apikeydomain.ProbeReader.ListProbed` 取所有 ProbedKey → 按 provider 调 `llm.DescribeModels` → 聚合去重 → `[]CapabilityView`（含每模型 Knobs）。`ModelRef` 校验（modelId 非空 + key 经 `KeyProvider.ResolveCredentialsByID` 存在）。

### 层 C — caller（chat/agent/loop，**本轮不写**，波次 2/3/5）
`Resolve(scenario, override) → ModelRef`（override 优先、否则 picker、都无 → 优雅报错）→ apikey 换 creds → `llm.Request{ModelID, Key, BaseURL, Options: ref.Options}` → provider 自办。

```
前端「我能用哪些模型」 ← handler ← app/model.CapabilityService
                                        ↓ ListProbed
                              apikey(原始 test_response)
                                        ↓ DescribeModels(raw)（按 provider 分发）
                              infra/llm 各家解析器 + 静态规格
发请求 ： workspace.Pick(scenario) ─或─ 实体.override → Resolve → ModelRef
        → apikey.ResolveCredentialsByID → llm.Request{Options} → provider 自读自填
```

---

## 3. 关键类型与契约改动

### 3.1 `infra/llm` 改动（连带，核心）
- **删 `ThinkingSpec`**；`Request` 改：
  - `Options map[string]string` 成为**唯一**旋钮承载（原生 key→原生 value，如 `{"reasoning_effort":"high"}`、`{"thinking":"enabled"}`、`{"thinkingLevel":"high"}`、`{"effort":"max"}`）。
  - `MaxTokens int` 语义改为**可选覆盖**；`0` → **provider 自填**（provider 自知规格，注释「caller 从 catalog 派生」作废）。
- **新增类型**（住 llm 包，model import 它，保持 llm 零业务依赖）：
  ```
  type ModelInfo struct { ID, DisplayName string; ContextWindow, MaxOutput int; Knobs []Knob; /* +能力布尔按需 */ }
  type Knob struct { Key, Label, Type, Default string; Values []string /* enum 原生取值 */; /* +互斥/适用提示按需 */ }
  ```
  > `Knob` 是**描述容器**（容器统一、内容全原生），非语义模子——前端据此通用渲染、不为每家写死。
- **11 家 adapter 重写**旋钮编码（按 research 真实规格），并各实现 `DescribeModels/Knobs` + 静态规格兜底。
  - **anthropic**：`thinking{adaptive/enabled/disabled}` + `output_config.effort`；**修 budget_tokens→400 bug**（4.7/4.8 用 adaptive）；`/v1/models` 富解析 + 1M 无 header。
  - **openai**：`reasoning_effort`(+`verbosity`)，每模型 effort 子集；`max_completion_tokens`；`/models` 贫→静态表。
  - **gemini**：`thinkingBudget`(2.5)/`thinkingLevel`(3.x) 代际分流 + 互斥防护；ListModels 富解析。
  - **deepseek**：`thinking`+`reasoning_effort`(high/max)；`reasoning_content` 回传；`/models` 贫→静态表。
  - **7 家待查**(qwen/zhipu/moonshot/doubao/openrouter/ollama/custom)：实现谁查谁。

### 3.2 `domain/model`（薄）
- `ModelRef{APIKeyID, ModelID string; Options map[string]string}`（去 GORM tag、纯 struct）。被 workspace 默认 + 未来 override import（与 agent/conversation→model 同性质，无环）。
- `Scenario` 常量（dialogue/utility/agent）+ `IsValidScenario`/`ListScenarios`。
- `ModelPicker` 接口保留（`Pick(ctx, scenario) (ModelRef, error)`），由 **workspace.Service** 实现。
- `Resolve(scenario, override *ModelRef, picker ModelPicker)` 纯函数（override 优先）。
- 错误(S20)：`MODEL_SCENARIO_INVALID`、`MODEL_NOT_CONFIGURED`(优雅报错用)、`MODEL_REF_INVALID`(apiKeyId+modelId 必填)。**无 Repository**。

### 3.3 `workspace`（搬家）
- domain：加 3 列 `DefaultDialogue/DefaultUtility/DefaultAgent *modeldomain.ModelRef`（JSON 存）；workspace import modeldomain。
- store：DDL 加 3 列（TEXT JSON，nullable）。
- app：`SetDefault(scenario, ModelRef)` / `GetDefault(scenario)`；实现 `modeldomain.ModelPicker.Pick`。

### 3.4 `apikey`（修 M1.2 一处）
- anthropic 探针 `TestMethodAnthropicPing` → 改打 `/v1/models`（x-api-key + anthropic-version 头），既测连通又存富 models 返回。新增/改 `TestMethodAnthropicModels`。

### 3.5 handler
- `GET /api/v1/model-capabilities`（重写：源改 app/model.CapabilityService + Knobs，去旧 modelsFound 链）。
- `GET /api/v1/scenarios`（白名单，exempt auth，照搬）。
- `model-configs` 端点**删除**（默认搬 workspace → 走 workspace 端点；override 走各实体）。

---

## 4. 删除清单
- `pkg/modelcatalog`（37 行跨家大表）**不迁、弃**。
- `llm.ThinkingSpec` 删。
- `model_configs` 表 / `domain/model.Repository` / `infra/store/model` / `app/model` 旧 CRUD+Picker **不迁**。
- `infra/store/modelcapoverride` 空目录、`pkg/modelcaps`(不存在) — 确认无残留（doc-fix：order/STATE 旧旗标作废）。

---

## 5. 契约变更（→ contract-changes #5）
- `model-configs` 端点删除；默认模型选择并入 workspace（workspace 响应+ 3 字段 `defaultDialogue/defaultUtility/defaultAgent`）。
- `/model-capabilities` 返回结构变（每模型带 `knobs[]`，原生 key/values；去 `modelsFound` 旧链）。
- `:test` 已在 M1.2 去 modelsFound；本轮 anthropic 探针端点内部改（对外 envelope 不变）。
- error code：`MODEL_*` 前缀（SCENARIO_INVALID/NOT_CONFIGURED/REF_INVALID）。

---

## 6. 验证
`gofmt`/`go build ./...`/`go vet`/`go test ./... -race` 全绿；llm 各家旋钮编码 + DescribeModels/Knobs 单测；model capability 聚合测试（fake ProbeReader）；workspace picker 测试。

## 7. 是否更干净
旧：选择+知识+编译三合一、跨家中立大表、自创档名、旗舰上 budget_tokens 报错。
新：知识归各家原生自包含、model 仅聚合展示无 store、删中立抽象、修真 bug。✅
