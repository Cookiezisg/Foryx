# Round 0052 — attachment 多 provider 注入（波次 5 · M5.2 前置子模块 2/3）

类型 / 目标：把上传的附件**渲染进 11 家 LLM provider 的官方多模态 wire**——中立 `ContentPart`（text/image_url/file）各家自渲 + `attachment.ToContentParts` 桥（att_ids → []ContentPart）。attachment 子模块第二轮（R0051 存储 → **R0052 注入** → R0053 sandbox 提取）。

用户指令链：「各家分开做」（不收敛共享 wire）→「逐个做」→「**各家做的时候，挨个去查官方文档对齐。对齐了再做。然后都做完**」。

依赖扫描：
- 上游就绪：infra/llm（11 家 provider 客户端，R0013-16 已各自查官网重构、每家自包含）；中立 `LLMMessage.Parts []ContentPart` 已存在（part.Type 约定 `text`/`image_url`）；attachment app（R0051 Service + BlobStore 端口 + Repository.GetBatch）。
- 下游接口（消费者）：chat（M5.2）调 `ToContentParts(att_ids, visionCapable)` 拼用户文本 part → loop/provider。
- 考古：旧 backend 多模态注入基本只 OpenAI `image_url` 一家格式（LibreChat 同病）；本轮**11 家全覆盖、逐家对官方文档**。

官方文档对齐（11 家逐一查证，这是用户指令的交付物）：
- **原生内联 PDF 仅 3 家**：anthropic（`document` 块 + base64 source）/ openai（`file` part，`file_data` data-URL，与官方逐字一致）/ gemini（`inlineData` mimeType=application/pdf，50MB/1000 页）。
- **image 两大阵营**：① data-URL 阵营（openai 兼容：`image_url.url` 放完整 data-URL）= deepseek/qwen/zhipu/doubao/moonshot/openrouter/custom；② 解析阵营 = anthropic（拆 media_type+base64 源块）、gemini（拆 mimeType+data inlineData）、ollama（原生 `images[]` 裸 base64、剥 data-URL 前缀）。
- **moonshot/Kimi**：图仅接 base64（data-URL）或 file-id，无 URL；`content` 字段须由 `string` 改 `json.RawMessage` 才能装 parts 数组。
- **PDF 降级**：deepseek/qwen/zhipu/doubao/moonshot/openrouter/custom/ollama **无原生内联文档** → file part 静默跳过（降级），靠 R0053 sandbox 抽文本补。

修改后完整逻辑（= domains/attachment.md DOC-307 §4 as-built）：
- **infra/llm/llm.go**：`ContentPart` 扩字段 `MediaType`/`Data`/`Filename` + 常量 `PartText`/`PartImageURL`/`PartFile`（`image_url`/`file` 沿用既有 switch 线缆名）。
- **11 家渲染器**（每家自包含、不共享基座）：
  - anthropic：`buildAnthropicUserMsg` image→`{type:image, source:base64}`（`extractMediaType`/`extractBase64Data` 拆 data-URL）、file→`document` 块（part.MediaType+Data）。
  - openai：`oaiContentPart` 加 `file` + `oaiFile{filename,file_data}`，file_data=`data:<mime>;base64,<data>`。
  - gemini：`geminiUserParts` image→`geminiImagePart`、file→`inlineData{mimeType,data}`。
  - deepseek/qwen/doubao/openrouter：各加 `xContentPart`+`xImageURL` + `buildXUserMsg`（text + image_url data-URL，未知 part 跳过），user case 改调它。
  - moonshot：`content` string→`json.RawMessage`（+ `moonshotJSONString` 包系统/工具/assistant）+ `buildmoonshotUserMsg` + content-part struct。
  - zhipu/custom：本已渲 text+image_url，`default` 由报错改**跳过**（让 file 优雅降级）。
  - ollama：`ollamaMessage` 加 `Images []string` + `buildOllamaUserMsg`（text→content、image→`images[]` 裸 base64、剥 data-URL 前缀 `ollamaStripDataURL`）。
- **app/attachment**：`ToContentParts(ctx, att_ids, visionCapable) → []llm.ContentPart`——image→image_url data-URL part（仅 visionCapable，否则降级文字提示）、text→文件内容内联 text part（带文件名）、document→file part（MediaType+base64+文件名）、audio/video/other→文字占位（R0053）；缺失/不可读 id 告警跳过；**parts 按 att_ids 保序**（`GetBatch` 的 `WHERE id IN` 不保序、按 id 建索引重排）。`visionCapable` 由调用方传入（本层不持模型目录知识）。

删除 / 合并：无（纯增 + zhipu/custom 的 default 报错→跳过）。

契约变更（→ contract-changes #34）：domains/attachment.md §4 由「待建」整段重写为 as-built（11 家渲染表 + ToContentParts 签名 + 降级语义）+ §3/§7 修正。**无新 REST / DB / error-code**——ContentPart 是 infra/llm 内部 wire，非对外契约。

新实现要点：ContentPart 扩字段 + 3 常量；11 家各自渲染器（3 原生 PDF / 7 openai 兼容 image_url / ollama images[]）；moonshot content 升 RawMessage；ToContentParts 桥（保序 + vision 门控 + 降级）。

新测试（全离线）：
- `multimodal_test.go`：`TestMultimodalRendering`（11 家一条 text+image+PDF 消息过 `BuildRequest` → string-contains 断言对齐结果：anthropic document+image 源/openai file+image_url/gemini inlineData/7 家 image_url data-URL 且 file 降级跳过/ollama images[] 裸 base64 且无 data-URL）+ `TestMultimodalTextOnlyUnchanged`（9 家 no-Parts 回归）。
- attachment app：`ToContentParts_ByKind`（image→image_url / text→inline / pdf→file base64，保序）+ `NonVisionDegradesImage`（图降级文字提示）+ `SkipsMissingPreservingOrder`（陈旧 id 跳过保序）+ `EmptyIDs`。

验证：gofmt clean / `go build ./...`（整仓）exit 0 / vet clean（修 ollama `+=` loop→strings.Builder）/ `go test`（llm 全套 + attachment app）全绿。

是否更干净（自证）：① 不造共享 wire 基座——每家自包含延续 R0014-16「重复 < 错抽象」（11 个 buildXUserMsg ~25 行各自，但零错抽象、改一家不波及别家）；② **降级即跳过**而非报错（part switch 遇不支持类型静默略过，新 part 类型不会 400 某家）；③ ToContentParts 把"附件→parts"收在 attachment 自身（自包含的公共面），vision 门控用形参（调用方按模型能力传，本层不耦合模型目录）；④ 逐家对官方文档=不靠猜（离线测只能验形状、对齐靠文档）。

覆盖状态（capability-ledger）：多模态附件「11 家 provider 注入 + vision 门控 + PDF 原生/降级」能力落地；提取（R0053）随后。

遗留 / 下一步：**R0053 sandbox 提取**（PDF/Office python `pdfplumber`/`python-docx` 在 sandbox 跑、抽文本补降级家 + token 限额；音频 Whisper/视频/OCR 经可插 `Extractor` 端口留插槽延后）。**M5.2 接线**：model 目录补 `vision` flag（ToContentParts 的 `visionCapable` 实参来源）；chat 拼用户文本 part + 调 ToContentParts。
