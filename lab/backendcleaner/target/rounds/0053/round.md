# Round 0053 — attachment sandbox 提取（波次 5 · M5.2 前置子模块 3/3，**attachment 完整收官**）

类型 / 目标：给**不能原生读文档的模型**把 PDF/Office 抽成文本内联——可插 `Extractor` 端口 + `SandboxExtractor`（在 sandbox 跑 python `pdfplumber`/`python-docx`/`openpyxl`/`python-pptx`）+ `ToContentParts` 文档分支（native→file part / 否则→抽文本，token 截断）。attachment 子模块**第三轮 = 收官**（R0051 存储 → R0052 注入 → **R0053 提取**）。整条多模态通路打通。

依赖扫描：
- 上游就绪：sandbox app Service（`Spawn(owner,SpawnOpts{Cmd,Args,Stdin})→ExecutionResult` 同步一次性 + `EnsureEnv(owner,EnvSpec{Runtime,Deps})` 幂等装包，Explore 摸清接口）；R0052 的 `ToContentParts`（接 document 分支）。
- 下游接口（消费者）：chat（M5.2）传 `Capabilities{Vision, NativeDocs}` 进 `ToContentParts`。
- 考古：旧 backend 无独立提取层（attachment 内嵌 chat、仅存路径）；本轮全新建 + 抄 LibreChat 路由式提取思路（OCR>STT>解析）但主线先做 PDF/Office。

设计要点（与既有架构强结合）：
- **何时抽**：`document` kind 且 `caps.NativeDocs=false`（模型无原生文档输入）→ 抽文本内联；原生三家（anthropic/openai/gemini）走 R0052 的 file part、**不抽**。**无独立缓存**——抽出的 text part 由 chat M5.2 落进 message_blocks，天然「抽一次」（不在 R0053 加缓存列/blob）。
- **可插 `Extractor` 端口**：`Extract(ctx, mime, data) (string, error)`，不认的 mime 返 `ErrExtractionUnsupported` → 调用方降级占位。`SandboxExtractor` 是主线实现；音频(Whisper)/视频/扫描 OCR(tesseract) 后补**独立 extractor**，不动主干（用户「主线扎实做、其余可插延后」）。
- **DIP**：`SandboxRunner` 本地端口（`EnsureEnv` + `Spawn`），sandboxapp.Service **结构化满足**（attachment 不 import app/sandbox），照 function/handler 适配器范式。
- **python 投递**：`python -c <内嵌脚本> <mime>`，文件字节走 stdin，脚本按 mime 分派、打印 `{"text"}` 或 `{"error"}`；解析失败脚本退出 0 + JSON {error}（损坏文件优雅降级），仅解释器坏/缺包 spawn `!Ok` 才算 infra 错。
- **共享 env**：固定全机 owner `{kind:attachment, id:"extractor"}` 单 venv（4 包），跨 workspace 复用（不存用户数据、字节流过）；首抽付 pip install、之后 EnsureEnv 幂等快返。
- **token 限额**：`maxExtractedChars=400_000`（~100K token@4 字符，对齐 LibreChat fileTokenLimit），保头部 + 截断标注。
- **Capabilities**：R0052 的 `ToContentParts(att_ids, visionCapable bool)` → `ToContentParts(att_ids, caps Capabilities{Vision, NativeDocs})`（两 flag 调用方按模型能力传，本层不持目录知识）。

强化地基：**sandbox 加 `attachment` owner kind**——domain 常量 `OwnerKindAttachment` + `sandbox_envs.owner_kind` CHECK 加 `'attachment'`（否则 CreateEnv 写脏值被 CHECK 拒）。合法（attachment 是新 env owner），非在 attachment 内绕。

修改后完整逻辑（= domains/attachment.md DOC-307 §4-5-7 as-built）：
- **domain/sandbox**：+ `OwnerKindAttachment`（注释说明 = 共享抽取 env owner）。
- **store/sandbox**：DDL owner_kind CHECK + `'attachment'`。
- **app/attachment/extractor.go**（新）：`Extractor` 端口 + `ErrExtractionUnsupported` + `SandboxRunner` 端口 + `SandboxExtractor`（`extractorOwner` 固定 owner + `extractorDeps` 4 包 + 内嵌 `extractScript` dispatch 4 格式 + EnsureEnv→Spawn→解析）。
- **app/attachment/attachment.go**：+ `Capabilities{Vision,NativeDocs}`；Service + `extractor Extractor`（nil-able）；`New` 加 extractor 形参；`ToContentParts` document 分支 native?file:extract；`extractDocPart`（抽取 + 截断 + 降级占位）+ `maxExtractedChars` + `truncateForLLM`。

删除 / 合并：无（纯增 + R0052 ToContentParts 签名 visionCapable→caps）。

契约变更（→ contract-changes #35）：domains/attachment.md §5 由「待建」整段重写 as-built + §4 ToContentParts 改 Capabilities + §7 sandbox/model；domains/sandbox.md §2.2 owner_kind 枚举加 attachment。**无新 REST / error-code**；DB 仅 sandbox_envs.owner_kind CHECK 多一枚举值（database.md 仅索引该表、无 CHECK 细节，不需改）。

新测试（全离线）：
- ToContentParts 抽取路径 3：`NonNativeDocExtracts`（fake extractor，pdf+NativeDocs=false→text-extracted 标注文本）+ `DocDegradesWhenNoExtractor`（nil extractor→占位 unavailable）+ `ExtractionFailureDegrades`（extractor 报错→could not be extracted 占位）。
- SandboxExtractor 4：`Success`（fake sandbox 验 EnsureEnv deps + Spawn `python -c <script> mime` + stdin=原始字节 + 解析 {text}）+ `UnsupportedMimeShortCircuits`（audio→ErrExtractionUnsupported 且 **不调 EnsureEnv**）+ `PythonErrorWrapped`（{error}→包装错误）+ `NonZeroExitErrors`（!Ok→错误）。
- R0052 既有 4 测改 `Capabilities`（ByKind=Vision+NativeDocs 仍 file part 验证不破）。newSvc 重构为 newSvc/newSvcWith(ext)。

验证：gofmt clean / `go build ./...` exit 0 / vet clean / `go test ./...` **97 包 ok / 0 FAIL**。

是否更干净（自证）：① 提取与渲染解耦（R0052 渲染 / R0053 抽取，各自纯净）；② 端口可插（Extractor + SandboxRunner 双端口，主线一个实现、音视频后补不动主干）；③ 无投机缓存（抽出文本由 chat message 持久化天然「抽一次」，不在本轮造缓存列）；④ 共享全机 env（不 per-attachment 浪费，env 无用户数据故全机共享合理，对齐 runtime 全机共享）；⑤ 降级链完整（mime 不支持 / 解析失败 / 无 extractor / 缺包 各有去处，绝不让回合失败）。

覆盖状态（capability-ledger）：多模态附件「持久化(R0051) + 11 家注入(R0052) + PDF/Office 本地抽取(R0053)」**整条通路打通**；音频/视频/OCR 留 Extractor 端口按需补（dogfood 再上）。

遗留 / 下一步：**attachment 子模块完整收官 🎉**。**M5.2 chat**（attachment 唯一消费者）：拼用户文本 part + 调 `ToContentParts`、model 目录补 `vision`/`nativeDocs` flag 喂 `Capabilities`、消息持久化抽取结果（天然缓存）；M7 装配：`attachment.NewSandboxExtractor(sandboxSvc)` 注入 + handler 注册 + blob.New + boot/ticker `:gc`。音频/视频/OCR extractor 按需补。
