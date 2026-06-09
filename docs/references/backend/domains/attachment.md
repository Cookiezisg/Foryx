---
id: DOC-307
type: reference
status: active
owner: @weilin
created: 2026-06-09
review-due: 2026-09-01
audience: [human, ai]
---
# Attachment Domain — 多模态附件（CAS 存储 + 多 provider 注入 + sandbox 提取）

> **核心职责**：用户上传到聊天回合的文件（图/PDF/Office/文本/音视频）的**持久化 + 进 LLM** 的完整通路。
> 抄 LibreChat 最完整的流水线骨架，加 3 个 Forgify 升级：**① CAS 内容寻址存储**（dedup+完整性）
> **② sandbox 当本地提取引擎**（不依赖云 OCR/API key，离线）**③ 中立 ContentPart + 各 provider 渲染器**（多家）。
> **无 RAG**（对齐 document 域决定）：大文档 = 抽文本 + token 限额截断 + 直接注入，非向量检索。
>
> **三轮交付**：**R0051 存储核心 ✅** → **R0052 多 provider 注入 ✅（11 家各自渲染 + ToContentParts 桥，本文档 as-built）** → R0053 sandbox 提取（PDF/Office；音频/视频/OCR 留插槽）。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `Attachment` 元数据行（att_，as-built R0051）
```go
type Attachment struct {
    ID          string     `db:"id,pk"`              // att_<16hex>
    WorkspaceID string     `db:"workspace_id,ws"`    // orm 自动隔离
    SHA256      string     `db:"sha256"`             // 内容寻址键 → CAS blob
    Filename    string     `db:"filename"`           // 显示名（blob 按 sha 寻址、非按名）
    MimeType    string     `db:"mime_type"`
    SizeBytes   int64      `db:"size_bytes"`
    Kind        string     `db:"kind"`               // image|document|text|audio|video|other
    CreatedAt   time.Time  `db:"created_at,created"`
    DeletedAt   *time.Time `db:"deleted_at,deleted"` // D1 软删
}
```
**字节绝不进 SQLite**：行只存元数据；blob 在文件系统。`sha256` **不唯一**——多行可共享一个 blob（dedup）。

### 1.2 CAS blob 存储（infra/fs/blob）
```
~/.forgify/workspaces/<wsID>/blobs/<sha[:2]>/<sha>
```
- 按 SHA-256 内容寻址、两字符分片；workspace id 取自 ctx（隔离），同 memory/skill 文件式 store 的缝。
- **Put 内容寻址 dedup**：blob 已存在则 no-op（相同字节哈希到同一路径）；原子 temp+rename。
- sha 进路径前校验为 64 位 hex（防穿越）。

---

## 2. 存储原理 (Storage Principles)

- **上传与发送解耦**：`POST /attachments → att_id`，消息引用 id（ChatGPT/Claude.ai/OpenAI/Anthropic 通用范式）。
- **blob 存盘、元数据进 DB**：字节绝不塞 SQLite（业界铁律；LibreChat/Open WebUI 同）。
- **内容寻址 dedup**：同一文件重传 = 一份 blob、多条 att_ 行。
- **软删 + GC**：删 = 软删行（留墓碑，D1）；`GC` 扫孤儿——blob 的 sha 无任何活跃行引用才删（**按 sha refcount**，dedup-aware）。
- **上限**：单文件 `MaxBytes = 50 MB`（对齐 OpenAI 单文件；Claude.ai 30 MB）。

---

## 3. 模态分类 (Kind)
`KindFromMIME(mime, filename)` 按 mime（剥 `; charset`）分类，application/octet-stream 用扩展名兜底：

| Kind | 触发 | 进 LLM 方式（R0052/R0053）|
|---|---|---|
| `image` | image/* · png/jpg/gif/webp/heic | vision 块（`image_url` data-URL，或 anthropic base64 源 / gemini inlineData / ollama 裸 base64）|
| `document` | application/pdf · docx/xlsx/pptx/odt/epub | PDF 原生透传(capable 模型) 或 sandbox 抽文本 |
| `text` | text/* · json/xml/yaml/csv · 代码扩展名 | 内联文本（原生读）|
| `audio` | audio/* | **R0053 留插槽**：sandbox Whisper 转写（延后）|
| `video` | video/* | **不做**（抽帧重、价值低，Claude 也不做）|
| `other` | 其余 | 不透明（仅存储下载）|

---

## 4. LLM 注入（R0052，as-built ✅）

中立 `ContentPart`（`text` / `image_url` / `file`，挂 `llminfra.LLMMessage.Parts`）→ **11 家 provider 客户端各自渲染成自家官方 wire**。**不共享 wire 基座**——每家自包含（R0014-16「重复 < 错抽象」原则的延续）；逐家查官方文档对齐后实现。`Parts` 非空时各家只渲 `Parts`、忽略 `Content`（chat M5.2 把用户文本作为首个 text part 拼入）。

| part | 原生支持（内联）| openai 兼容家（data-URL）| 特殊 wire |
|---|---|---|---|
| **image** | 各家 vision | deepseek/qwen/zhipu/doubao/moonshot/openrouter/custom：`image_url` data-URL | anthropic：base64 源块；gemini：`inlineData`；ollama：原生 `images[]` 裸 base64（剥 data-URL 前缀）|
| **file (PDF)** | **仅** anthropic `document` 块 / openai `file` / gemini `inlineData` 三家原生内联 | 跳过（降级）| ollama / 中文家：跳过 |

- **降级即跳过**：无法内联某 part 类型的家在 part switch 静默跳过（**不报错**）；PDF 对非原生家 = 留给 R0053 sandbox 抽文本补。
- **moonshot** 的 `content` 字段由 `string` 改 `json.RawMessage`（容纳 parts 数组；Kimi 图仅接 base64 / file-id，正是 data-URL）。

**`attachment.ToContentParts(ctx, att_ids, visionCapable) → []llm.ContentPart`**（app 层桥，one user turn）：
- `image` → `image_url`（data-URL）part，**仅 visionCapable 时**；否则降级为文字提示（模型无视觉）。
- `text` → 文件内容内联成 text part（带文件名标注）。
- `document`(pdf) → `file` part（MediaType + base64 + 文件名）；原生三家渲、其余跳过靠 R0053。
- `audio`/`video`/`other` → 文字占位（真抽取 = R0053）。
- 缺失 / blob 不可读的 id 告警跳过（陈旧 id 不让回合失败）；parts 按 `att_ids` 保序（`GetBatch` 的 `WHERE id IN` 不保序，按 id 建索引重排）。
- `visionCapable` **由调用方（chat M5.2）按解析后模型能力传入**——本层不持模型目录知识；model 目录的 `vision` flag 由 M5.2 接线时补（R0052 不碰目录）。

## 5. 提取流水线（R0053，待建）—— sandbox 当本地引擎
- 路由优先级（抄 LibreChat）：OCR > STT > 文本解析 > 兜底。
- **主线**：PDF 文本 `pdfplumber` / Office `python-docx`·`openpyxl`·`python-pptx`，**全在 Forgify sandbox 跑 python**（离线、无云 OCR、无 API key）。
- **token 限额**：`fileTokenLimit`（默认 100K），全量提取、构造 prompt 时截断、保头部。
- **可插 `Extractor` 端口**：音频(Whisper)/视频/扫描 OCR(tesseract) 都是往此端口插一个 extractor，不动主干——按需补。

---

## 6. HTTP 端点（as-built R0051）

| Method | Path | 说明 |
|---|---|---|
| POST | `/api/v1/attachments` | multipart 上传（单 `file` 字段）→ 校验+算 sha+CAS 存+返 att_ |
| GET | `/api/v1/attachments/{id}` | 元数据 |
| GET | `/api/v1/attachments/{id}/content` | 原始字节（按存储 mime + Content-Disposition inline）|
| DELETE | `/api/v1/attachments/{id}` | 软删（blob 由 GC 回收）|

---

## 7. 跨域集成 (Interactions)
- **chat（M5.2）**：消息引用 `att_ids` → `ToContentParts(att_ids, visionCapable)` → 拼用户文本 part → 进 loop/provider；`visionCapable` 由 chat 按解析后模型能力传入。
- **sandbox（R0053）**：提取引擎（python 提取脚本）。
- **model（M5.2 接线）**：vision capability flag（R0052 桥已留 `visionCapable` 形参，目录 flag 待 M5.2 补）。
- **无 RAG**：大文档抽文本 + token 限额 + 直接注入（对齐 document 域无向量检索）。
- **GC**：boot 或 ticker 定期 `:gc`（M7 接线）。

---

## 8. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 备注 |
|---|---|---|---|
| `ErrNotFound` | 404 | `ATTACHMENT_NOT_FOUND` | id 不存在 / 已软删 / 跨 workspace |
| `ErrTooLarge` | 413 | `ATTACHMENT_TOO_LARGE` | 超 50 MB |
| `ErrEmpty` | 422 | `ATTACHMENT_EMPTY` | 空文件 |
| (handler) | 400/413 | `ATTACHMENT_BAD_UPLOAD` | multipart 缺 `file` 字段 / 读取失败 |
