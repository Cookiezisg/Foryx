---
id: DOC-122
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Skill Domain — 预制能力模板与执行审计

> **核心地位**：Skill 是 Forgify 的 **“预制件”**。它不属于用户实时锻造的实体，而是通过 `.md` 文件定义的原子化能力包。系统通过扫描特定的文件目录，自动加载这些具备高度可复制性的技能。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `Skill` 逻辑定义
Skill 没有主数据库表，其主权在文件系统。
- **文件源**：`~/.forgify/skills/*.md`。
- **物理标识**：文件名（不含后缀）。
- **Frontmatter**：
  ```yaml
  ---
  name: "code_reviewer"
  description: "专业代码审查技能"
  tools: ["fn_lint", "mcp:google_maps"]
  ---
  ```

### 1.2 `SkillExecution` (执行审计表)
虽然定义在文件，但执行过程必须入库审计（D22）。
```go
type Execution struct {
    ID             string         `gorm:"primaryKey;type:text" json:"id"` // ske_<16hex>
    UserID         string         `gorm:"not null;index" json:"-"`
    SkillName      string         `gorm:"not null;index" json:"skillName"`
    
    // 执行环境
    ConversationID string         `gorm:"index" json:"conversationId"`
    ForkDepth      int            `gorm:"not null;default:0" json:"forkDepth"`
    Substitutions  map[string]any `gorm:"serializer:json" json:"substitutions"` // 动态参数替换
    
    // 标准 D22 字段
    Status         string         `json:"status"` // ok|failed|cancelled
    Input          string         `json:"input"`
    Output         string         `json:"output"`
    ElapsedMs      int64          `json:"elapsedMs"`
    CreatedAt      time.Time      `json:"createdAt"`
}
```

---

## 2. 核心原理 (Principles)

### 2.1 FS-Scanning (物理热重载)
系统不预加载所有技能，而是采用 **“按需扫描 + 热重载”**：
1. **启动扫描**：后端启动时遍历 `skills/` 目录。
2. **校验解析**：解析 Markdown 顶部的 YAML 块，并验证其引用的工具（fn/hd）是否存在。
3. **实时失效**：用户通过 `skills:refresh` API 强制刷新缓存，无需重启后端即可应用新技能。

### 2.2 Template-Injection (指令注入)
Skill 的核心是一段 Markdown 文本。
- **动态占位符**：支持 `$USER_ID`, `$PROMPT` 等标记。
- **注入逻辑**：当 LLM 调用 `activate_skill` 时，系统将对应的 MD 文本动态拼入 **System Prompt** 的 `capabilities` 段，从而瞬间赋予 LLM 全新的领域知识。

### 2.3 Fork-Isolation (分叉隔离)
Skill 运行通常会产生一个新的子任务上下文。
- **递归深度控制**：通过 `ForkDepth` 物理限制 Skill 调用 Skill。
- **环境对齐**：子任务会承袭父对话的 `model_override` 设置，确保能力一致性。

---

## 3. 生命周期 (Lifecycle)

1. **载入 (Loading)**：从磁盘读取 `.md`，解析 Schema。
2. **发现 (Discovery)**：出现在 `GET /api/v1/skills` 或 `GET /api/v1/catalog` 中。
3. **激活 (Activation)**：LLM 调工具，将技能文本“热插拔”进系统提示词。
4. **归档 (Journaling)**：执行详情记入 `skill_executions` 表。

---

## 4. 跨域集成 (Interactions)

- **Catalog**：作为 Catalog 的静态源之一。
- **Chat**：LLM 通过 `activate_skill` 进行按需动态学习。
- **Relation**：建立 `skill_uses_function` 等隐式依赖。

---

## 5. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 场景 |
|---|---|---|---|
| `ErrSkillNotFound` | 404 | `SKILL_NOT_FOUND` | 文件被删了。 |
| `ErrInvalidFrontmatter`| 422 | `SKILL_INVALID_METADATA` | YAML 格式坏了。 |
| `ErrBodyTooLarge` | 422 | `SKILL_BODY_TOO_LARGE` | 单个 MD 超过 512KB。 |
| `ErrExecutionNotFound`| 404 | `SKILL_EXECUTION_NOT_FOUND` | 查不到审计记录。 |
| `ErrNameConflict` | 409 | `SKILL_NAME_CONFLICT` | 文件名重叠。 |
| `ErrInvalidName` | 400 | `SKILL_INVALID_NAME` | 文件名含有非法字符。 |
| `ErrRecursionAttempt` | 400 | `SKILL_RECURSION_DENIED` | 尝试 Skill 调 Skill（深度限制）。 |
