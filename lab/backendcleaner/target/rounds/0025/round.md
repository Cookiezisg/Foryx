# Round 0025 — memory（波次 1 · M1.7）文件式重设计

类型 / 目标：M1.7 memory 重新设计——从重型 SQLite CRUD 改为**按 workspace 的文件式 markdown**。设计经业界调研（4 并行 agent，见 R0024）+ 多轮讨论敲定。

## 核心方针（一句话）
**memory = 每个 workspace 的 markdown 文件(`~/.forgify/workspaces/<wsID>/memories/*.md`)，两段式注入(pinned 全文 + 目录按需读)，LLM 工具自管，改动发 notification，用户可直接编辑。**

## 关键设计决策（经讨论拍板）
1. **文件式而非 SQLite**：memory 是"workspace 资源"(和 skills 同类、可编辑文本)，非"业务实体"。每条 = `<name>.md`(frontmatter `description/pinned/source` + 正文)。文件名即 name(slug)、**无 mem_ id**。用户可直接看/编辑/git。
2. **workspace 分桶**：`~/.forgify/workspaces/<wsID>/memories/`，完全隔离(R0024 翻转"不分桶"后的落地)。
3. **两段式注入**(沿用 catalog 心智)：pinned 全文常驻 + 非 pinned 只列 name+description(`read_memory(name)` 按需加载)。砍热度排序。
4. **天然去重**：system prompt 已注入目录 → LLM 写前看得见现有 → 自然 update 而非重复(无 Mem0 向量 pipeline)。
5. **发通知用 notification.Emitter**(R0024 建)：增删改 → `Emit("memory.created/updated/deleted", {name})`。
6. **业界调研结论**(R0024)：简单 ≥ 复杂(benchmark)、原文>抽取、文件式趋势——砍向量/图/热度/type/reflection/decay。

## 考古发现（旧 SQLite 版的过度设计）
- 9 字段(AccessCount/AccessedAt 热度、Type 四分类、Metadata)——单机过度。
- 文档虚构："Catalog 选热点记忆"(memory 自有 ForSystemPrompt)、"Relation 建 memory 边"(relation 无)。

## 新实现
- **domain/memory**：`Memory{Name,Description,Content,Pinned,Source,UpdatedAt}` + 文件 `Repository` + `SystemPromptProvider` + slug 校验 + 4 错误。**无 id**。
- **infra/fs/memory**：**backend-new 第一个文件式 store**——按 ctx workspace 算路径、扫目录、手写极简 frontmatter 解析/渲染、原子写(temp+rename)、slug 防穿越(不用 pathguard)。skills(波次3)复用。
- **app/memory**：Service(Upsert/Get/List/Delete/Pin/Unpin + ForSystemPrompt 两段式 + 调 notification.Emitter)。
- **handler**：GET/PUT/DELETE /memories[/{name}] + pin/unpin。

## 测试
store 7(temp dir)：往返/NotFound/空/pinned 过滤/slug 防穿越/delete/**workspace 隔离**/frontmatter 往返。app 5(fake repo+emitter)：Upsert create→update 通知/validate/ForSystemPrompt 两段(非 pinned content 不泄漏)/空/delete 通知。

## 验证
`gofmt -l` 干净 · `go build ./...` 0 · `go vet ./...` 0 · `go test ./... -race` 全 ok。

## 契约
domains/memory.md 整篇重写(文件式)；**database.md 删 memories 表 + mem_ 前缀**；api.md +Memory §5.4(6 端点)；error-codes memory 3 行重写(删 NameConflict + 加 InvalidSource/InvalidInput)；events.md memory 行 payload {name}(去 id)。

## 遗留 / 下一步
- **M1.8 sandbox**（波次 1 续）。
- 工具 `read/write/forget_memory` 包 app → 波次 2/3；chat 注入(ForSystemPrompt) → 波次 5；`~/.forgify` base + `notification.Emitter` 注入 → M7。
- skills(波次3) 复用 infra/fs 文件 store 范式。
