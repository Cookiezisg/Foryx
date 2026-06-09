---
id: DOC-115
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-04
review-due: 2026-09-01
audience: [human, ai]
---
# Model Domain — 场景分派规则 + 能力聚合（无存储薄层）

> **核心职责**：model 域只做两件事——**分派**（按 scenario 解析「该用哪把 key、哪个 model、哪些原生旋钮」）与**展示**（把各 key 的探测档案聚合成「我能用哪些模型、每个怎么配」的目录）。它**不持有任何存储**：默认选择落在 workspace 行的列，override 是各实体（agent / conversation / node）的字段。模型知识（窗口 / 上限 / 旋钮）下沉各家 `infra/llm` provider 自包含。这是一次有意的收窄——model 提供规则与类型，存储与解读各归其位。

---

## 1. 物理模型 (Data Anatomy)

model 域**无表**。它只定义一个共享值类型与一组常量。

### 1.1 `ModelRef` — 可复用模型选择
workspace 默认列、各实体 override 共用同一形状。`Provider` 由 `APIKeyID` 引用的 api_key 隐含，不冗余存。
```go
type ModelRef struct {
    APIKeyID string            `json:"apiKeyId"`
    ModelID  string            `json:"modelId"`
    Options  map[string]string `json:"options,omitempty"` // 原生旋钮 k-v，如 {"reasoning_effort":"high"}
}
```
- `Validate()`：已设的选择必须同时带 `apiKeyId` 与 `modelId`，否则 `ErrRefInvalid`。
- `IsZero()`：三字段皆空表示「未设」——caller 用它判断该不该回落默认。
- **`Options` 是原生旋钮值**：key/取值是各家 provider 自己的 wire 词表，model 不解释、不归一，原样透传给 `infra/llm`。

### 1.2 `Scenario` — 默认模型槽白名单
workspace 级默认模型的三个固定槽，永不扩张：
```go
const (
    ScenarioDialogue = "dialogue" // 核心用户交互
    ScenarioUtility  = "utility"  // 后台活（摘要、命名、改写）
    ScenarioAgent    = "agent"    // workflow / agent 步骤
)
```
`IsValidScenario(s)` 守白名单；`ListScenarios()` 按规范顺序返回三者，供 `GET /api/v1/scenarios` 渲染。

---

## 2. 分派规则 (Resolve)

### 2.1 Override-then-Default（局部优先）
`Resolve` 收口了每个用 LLM 的 caller 本要各自重复的「先看 override 再看默认」分支：
```go
func Resolve(ctx, scenario string, override *ModelRef, picker ModelPicker) (ModelRef, error)
```
1. **Override 胜出**：`override != nil && !override.IsZero()` → 直接返回它（顺带 `Validate`）。
2. **Scenario 默认**：否则校验 scenario 合法，再问 `picker.Pick(ctx, scenario)`。

### 2.2 `ModelPicker` — 默认选择的端口
默认选择住在 workspace 列，故 picker **由 `app/workspace` 实现**、装配时注入；model 域只持端口、不依赖 workspace。
```go
type ModelPicker interface {
    Pick(ctx context.Context, scenario string) (ModelRef, error) // 无默认时返 ErrNotConfigured
}
```
- 该 scenario 在当前 workspace 没配默认模型 → `ErrNotConfigured`（caller 提示「去配置模型」，而非晦涩失败）。

### 2.3 Key-Model 闭包
`ModelRef` 永远带 `apiKeyId`：下游 `infra/llm` 按此显式 id 向 apikey 取凭证（`KeyProvider.ResolveCredentialsByID`），杜绝跨 provider 误用（如拿 OpenAI 的 key 调 Gemini 模型）。

---

## 3. 能力聚合 (Capability Surface)

`GET /api/v1/model-capabilities` 由 `app/model` 的 `CapabilityService` 服务，**无 store**——它读 apikey 的探测档案，逐 key 解析成模型目录。

### 3.1 聚合管道
```go
type CapabilityService struct { probes apikeydomain.ProbeReader; … }
```
1. `probes.ListProbed(ctx)` 取当前 workspace 全部 `ProbedKey{ID, DisplayName, Provider, TestStatus, TestResponse}`。
2. 跳过 `TestStatus != ok` 的 key（capabilities 反映**真正可达**的，而非仅录入过的）。
3. 每把活 key：`llm.DescribeModels(provider, testResponse)` → `[]ModelInfo`（单把 key 档案解析不出只 warn 跳过，不让整个目录变空）。
4. 摊平成 `CapabilityView`，每个 model 归属到提供它的 key。

### 3.2 `CapabilityView`（线缆形状）
```go
type CapabilityView struct {
    APIKeyID, KeyName, Provider, ModelID, DisplayName string
    ContextWindow, MaxOutput                          int
    Vision, NativeDocs                                bool       // M7 model-caps：原生图片 / 内联 PDF（与 ctx/out 同源 spec 表）
    Knobs                                             []llm.Knob // 复用 llm 描述符，不另造同形结构
}
```
> **M7 model-caps**：`ModelInfo`/`CapabilityView` 加 `Vision`/`NativeDocs`，住各 provider 静态 spec 表（provider 自描述，与 ctx/out 同源；gemini 解析时按 `gemini-*` 前缀判）。bootstrap 一个 `ModelInfoLookup` 同时供 chat 的 `Bundle.Caps`（附件原生渲染门控）+ contextmgr 的 `WindowResolver`（压缩预算）。现表：anthropic/gemini = vision+docs，openai/kimi-k2.x = vision；deepseek/qwen/zhipu/doubao 列出的文本旗舰 = 否（vision 在独立 SKU，未入目录）。

### 3.3 模型知识下沉 provider 自包含
**不存在跨家 `pkg/modelcatalog`**。窗口 / 上限 / 旋钮由各家 `infra/llm` provider 经 `DescribeModels(rawProbe)` 自描述：
- **富家**（gemini / moonshot / openrouter）：从 `/models` 载荷直接读规格。
- **贫家**（openai / deepseek / qwen / anthropic …）：`/models` 仅返回 id，规格 + 旋钮从各 provider 自家**静态目录**补，随软件更新维护。
- **ollama**：本地模型，窗口由每请求 `num_ctx` 设定，无固定规格。

### 3.4 旋钮契约 `Knob`
「容器统一、内容全原生」的可渲染描述符——前端据此通用渲染，无需懂任何一家：
```go
type Knob struct {
    Key     string   // native param name, e.g. "reasoning_effort"
    Label   string   // display label
    Type    string   // "enum" | "bool" | "int"
    Values  []string // native enum values, e.g. ["high","max"]（仅 enum）
    Default string
}
```
各家原生旋钮（key / 取值全是各家 wire 词表）：

| Provider | 原生旋钮 |
|---|---|
| openai | `reasoning_effort`（none/low/medium/high/xhigh 因型而异）+ `verbosity`(low/medium/high) |
| anthropic | `thinking`(adaptive/enabled/disabled 因型而异) + `effort`(low/medium/high/xhigh/max) |
| gemini | Gemini-3：`thinkingLevel`(minimal/low/medium/high 枚举，不可关)；Gemini-2.5：`thinkingBudget`(整数，-1 动态 / 0 关) |
| deepseek | `thinking`(enabled/disabled) + `reasoning_effort`(high/max) |
| qwen | `enable_thinking`(bool) + `thinking_budget`(整数) |
| ollama | `think`(bool) + `num_ctx`(整数) |

---

## 4. 跨域集成 (Interactions)

- **APIKey**：经 `ProbeReader` 供探测档案（原始返回）；经 `KeyProvider` 按 id 发凭证。**apikey 持原始数据，model 持解读。**
- **Workspace**：实现 `ModelPicker`，默认选择存其行的 `default_dialogue/utility/agent` 列。
- **Chat / Compaction / Auto-title**：分别消费 dialogue / utility 默认（+ 对话 override）。
- **Workflow / Agent**：消费 agent 默认（+ 节点 / agent override）。
- **infra/llm**：`DescribeModels`(解析) + 实际生成时按 `ModelRef` 构建请求。

---

## 5. 错误字典 (Sentinels)

全部经 `errorsdomain.New(kind, code, msg)` 构造（§S20），transport 直接读 Kind/Code。

| Sentinel | HTTP | Wire Code | 场景 |
|---|---|---|---|
| `ErrScenarioInvalid` | 400 | `MODEL_SCENARIO_INVALID` | 传入非 dialogue/utility/agent 的 scenario |
| `ErrNotConfigured` | 422 | `MODEL_NOT_CONFIGURED` | 该 workspace 在此 scenario 下无默认模型——提示去配置 |
| `ErrRefInvalid` | 400 | `MODEL_REF_INVALID` | ModelRef 缺 `apiKeyId` 或 `modelId` |
