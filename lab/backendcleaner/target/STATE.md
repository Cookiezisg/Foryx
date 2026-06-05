# backendcleaner — STATE（单一状态源）

> 进度的**唯一**事实源。原 CONCLUSIONS.md / backlog.json 已删；结论并入 SPEC/criteria，轮次索引在 ROUNDS.md，跨模块待办在 deps-todo.md。

## 当前

- **阶段**：Phase 2 逐模块 — 波次 0 全部完成；**波次 1（叶子业务域）进行中：M1.1 workspace ✅ · M1.2 apikey ✅ · M1.3 model ✅ · M1.4 relation ✅ · M1.5 catalog ✅ · M1.6 mention ✅ · M1.7 memory ✅ · M1.8 sandbox ✅**。
- **分支**：`main`（backend-new 平行重写不需要分支）。
- **策略**：`backend-new/` 平行重建 → 覆盖回 `backend/` → 调前端/testend 兼容。

## 已定的关键决策

- 全量重写，**无任何保留**（含本地 SQLite 数据 → schema 可激进重定）。
- **全局命名 `user_id` → `workspace_id`**（本地单机隔离单元=工作区；ctx/middleware/物理列/实体一律 workspace）。从波次 0 reqctx 起生效。
- 契约**可改**：每改对外 API/SSE/error 都 take note 到 `contract-changes.md`；前端/testend 也是 AI 写的，覆盖后一并兼容。
- 架构按 `module-template.md` 统一、**按需取层**；`go.mod` 空起按需生长、版本对齐现有。
- 重写单元 = 垂直切片；顺序见 `order.md`，判据见 `criteria.md`。
- **去 GORM**：自研 `pkg/orm`（链式、类型安全、自动 workspace 双向隔离 + 软删 + 时间戳）+ `glebarez/go-sqlite`（database/sql driver）。R0008 ✅。
- **domain 去 GORM 化（贯穿所有业务模块）**：domain 实体剥 `import gorm` + gorm tag + `TableName` + `gorm.DeletedAt` → 纯 struct + 轻量 `db:"col,..."` tag（无 import）；store 基于 `pkg/orm` 重写。
- **错误体系强化（贯穿所有模块）**：domain 错误升级为结构化 `Error{Kind,Code,Message,Details,cause}`（Is by Code）；错误码契约内聚 domain；各 domain `errors.New(msg)`→`New(kind,code,msg)`；transport errmap 塌缩成 `statusForKind`（M0.7，零 domain import）。R0012 ✅。
- **SSE 三流统一协议（流式树）**：`eventlog/forge/notifications` → `messages`/`entities`(全实体流式总线)/`notifications`；统一信封 `Envelope{seq,scope,id,frame}` + 四动词 frame(open/delta/close/signal) + **通用 Node{Type,Content}**（词表下放各业务、domain 不持）；**id 升信封层**；frame 按可丢性分级(delta/tick=ephemeral 不入 buffer，open/close/signal=durable，close 带快照)；infra **单一 `Bus`×3 实例**。设计蓝本 = `stream-protocol.md`（已拍板 2026-06-03）。
- **workspace（原 user 正名，M1.1）**：本地隔离单元=workspace。**一切 workspace 隔离（业务表持 workspace_id、orm 自动隔离；应用资源 mcp/skills/settings/memory **按 workspace 分桶** `~/.forgify/workspaces/<wsID>/`——R0024 翻转原"不分桶"决策）**——workspace=完整隔离单元。Name 自由展示名 + 全机唯一（去 slug/GetByUsername/EnsureExists）；Language 是第一个 workspace 偏好（不预建 preferences 容器，YAGNI）。orm 顺手补 `ErrConflict`（UNIQUE 翻译）+ handler 地基首建。R0018 ✅。
- **apikey（收窄，M1.2）**：大幅收窄为「加密保险箱 + 哑探针 + 按 id 发钥匙」。**选 key 下放**（LLM→model 的 api_key_id 显式 / 搜索→未来搜索配置，防乱烧钱）；**哑探针**（tester 只判 HTTP 200 + 存 `test_response` 原始返回，砍解析器）；**解析下放**（`models_found→test_response`，model 靠 `ProbeReader` 解析 + 静态目录兜底——Claude 无 list-models 端点故静态目录是其唯一来源、应**可更新推送**）。`KeyProvider` 收窄 2 法全按 id；**首个吃 orm 自动隔离的业务表**。`modelcatalog`/capabilities 移交 M1.3。R0019 ✅。
- **model（重写，M1.3）**：model 退化成「聚合+展示薄层」(无 store/无 Repository)；删中立 `ThinkingSpec`、各家原生旋钮经 `Request.Options` 自包含；模型知识(窗口/上限/旋钮)下沉各家 `infra/llm` provider(`DescribeModels`+静态目录)，弃跨家 `pkg/modelcatalog`；默认模型搬 workspace 3 列(`ModelPicker` 由 workspace 实现)；override 改运行时优雅报错(删删除时保护)；`Resolve` 收口 override 优先；`Knob` 容器统一/内容原生。R0020 ✅。
- **relation（横切，M1.4）**：实体血缘网（有向边图）。**边类型收成 4 动词** `create/edit/equip/link`（两端类型在 from_kind/to_kind 列，kind 只需动词，删 from/to 冗余编码 + DB CHECK 恒 4 值；中央枚举保留=全局拓扑契约，与删中立抽象不矛盾）；**8 节点**(Quadrinity+document+conversation+skill+mcp)；**`KindForID` 收编自 idgen**(8 条前缀，补 agent + 定 `sk_`/`mcp_` 规矩——skill/mcp 归一 id 体系是波次 3，前缀此刻定死使 document 现在就能 wikilink tag)；**显示名读时内存 hydrate**(按 kind 批量 `Namer.NamesByIDs`，不落库/改名即新/无 reader port/孤立节点不显示)；**无删除时引用保护**(对齐 model override 弱引用，best-effort sync)。R0021 ✅。
- **catalog（收窄，M1.5）**：能力概览「实体名录」——只报「名字+描述」按类型分组，告诉 LLM「有哪些实体」。**砍 InvokeTool**（调用是搜索工具/调用层的事）+ 砍花活(handler 方法列表/mcp 合成/Kind/Active) + 砍预留(`Generator`/`GeneratedBy`/`Granularity`/`Category`/`activate_tools`)；**两段式**：概览→`search_*`(波次 2)精确定位，故 id 不进菜单、name 不要求唯一；document 例外(name=文档名/desc=路径)；无 store 派生现查。R0022 ✅。
- **mention（纯契约，M1.6）**：@ 引用快照的 domain 契约——5 种可 @ 类型(四件套+document) + `MentionInput` + `Reference`(含 Content) + `Resolver` 接口 + `IsValidMentionType`。**Freeze-on-Send**(发送瞬间抓内容快照注入、定格)；**纯 domain 无 app/store/handler/error**——resolver 实现(波次 3)、chat 注册表+统一 `<mentions>` 渲染+错误处理(波次 5)。conversation/skill/mcp 不可 @。R0023 ✅。
- **notification（新模块，R0024）**：通知从"内存广播"升格为**持久化实体**——`Notification{ID,Type,Payload,ReadAt}` 存 DB(workspace 隔离)、前端通知中心(列表/badge/标已读)、关机重开仍在。scope=`notification:noti_x`(锚通知实体)，**workspace 是 Bus 分流轴非 scope**(前端按当前 workspace 订阅防多窗口串台)，事件类型在 `node.type`=`<域>.<动作>`，后端只发 type+payload 前端自渲文案。`Emitter` 端口给 producer。连带 stream 清理(删 KindWorkspace/ListReader/list.go)+ **R0018 翻转**(一切 workspace 隔离/~/.forgify 分桶)。业界调研 4 agent 指导 memory(下一步)走文件式。R0024 ✅。
- **memory（文件式，M1.7）**：从重型 SQLite CRUD 改为**按 workspace 的文件式 markdown**(`~/.forgify/workspaces/<wsID>/memories/<name>.md`，frontmatter description/pinned/source + 正文，文件名即 name、**无 mem_ id**)。两段式注入(pinned 全文常驻 + 非 pinned 目录 `read_memory` 按需)；天然去重(注入目录让 LLM 自判 update 否则新建，无向量 pipeline)；发通知用 `notification.Emitter`；用户可直接编辑文件。**backend-new 首个文件式 store**(手写 frontmatter/原子写/slug 防穿越，skills 波次 3 复用)。砍热度/Type 四分类/Metadata/向量/reflection/decay(业界调研判过度)。R0025 ✅。
- **sandbox（三 runtime，M1.8）**：Python+Node+Docker 隔离运行时（GitHub MCP registry 98 调研:Python+Node+remote 覆盖 92%、缺口 7 Docker-only）。**三 runtime 统一双接口**（image=docker 的 runtime、容器=env，零特例共用 manifest/锁/Ensure 流程）；`EnvBin/EnvDir`→`ResolveExec`（spawn 层不持 runtime 知识，docker 返回 `docker run --rm -i` 包装）；**两表系统级不分桶**（orm `meta.ws==nil` 自动跳隔离，runtime 全机共享——相对 memory/skills 分桶的合理例外）；去 GORM+硬删（无 deleted 列）；`docker.go` 新写（探测 daemon+pull+docker run+`-e` env，**不代装** docker，ErrDocker* 从"残留"转正预留）；notifications pkg→`notification.Emitter`（`sandbox.env_status_changed`/`env_deleted`）；路由 hacky(`POST /sandbox/{action}` 前导冒号)→RESTful+N5(`DELETE /runtimes|envs/{id}`、`POST /sandbox:gc`)。**骨架照搬**（旧实现本质复杂度无脂肪、双接口正交故 docker 无缝插入）+重写烂文档（MiseSpec/BootstrapOK 虚构字段、错误码全旧）。Docker 精细化（stop/孤儿/stdio e2e）留 M3.6、注册+base+fetch-mise 留 M7。R0026 ✅。

## 模块进度（编号见 order.md）

状态：⬜ pending ｜ 🔧 doing ｜ ✅ done ｜ ⏭️ 判定删除/合并

- **Phase 1 骨架** ✅：`backend-new/` + 空 go.mod + health server + smoke。
- **波次0 地基**：M0.1 pkg ✅（**reqctx/idgen/pagination ✅** R0001；**tokencount ✅** R0002；**pathguard ✅** R0003；**userpath ⏭️删** R0004；**wikilink ✅** R0005；**jsonrepair ✅** R0006；**limits ✅** R0007；modelcaps/modelcatalog 移交 M1.3）· M0.2 数据库层 ✅（**pkg/orm R0008 · db 网关 R0009**；业务表 DDL 分散各模块）· M0.3 ✅（**logger R0010 · crypto R0011**）· M0.4 ✅：**errors R0012** · **stream 统一协议 R0013**（单一 domain/stream：信封+四动词Frame+通用 Node{Type,Content}+Bridge/ListReader；词表下放业务）· M0.5 ✅ infra **stream bus（单一 Bus）R0014**（实例化三次=三流；frame 分级；D2 全量推；infra/chat extractor 移交 M5.2）· M0.6 llm ✅（11 家 provider）· **M0.7 transport ✅ R0017**（response N1+errmap 塌缩+SSE marshal · middleware workspace · router 框架；完整 New→M7）· **波次 0 收官 ✅**
- **波次1 叶子域**：M1.1 workspace(原 user) **✅ R0018** · M1.2 apikey **✅ R0019** · M1.3 model **✅ R0020** · M1.4 relation **✅ R0021** · M1.5 catalog **✅ R0022** · M1.6 mention **✅ R0023** · notification(基础) **✅ R0024** · M1.7 memory **✅ R0025** · M1.8 sandbox **✅ R0026** · M1.9 permissions/hooks ⬜ · M1.10 document ⬜ · M1.11 todo ⬜(待判定)
- **波次2 tool+原语**：tool ⬜ · loop ⬜ · tool/filesystem·search·web·toolset ⬜
- **波次3 Quadrinity**：function·handler·subagent·agent·skill·mcp + tool 适配器组 ⬜
- **波次4 编排核心**：workflow ⬜ · flowrun ⬜ · scheduler 🔴⬜ · trigger ⬜ · tool/workflow ⬜
- **波次5 对话**：conversation ⬜ · chat ⬜ · contextmgr ⬜ · tool/permissionsgate ⬜
- **波次6 顶层编排**：askai ⬜ · ask+tool/ask ⬜(强残留嫌疑)
- **波次7 wiring**：cmd/server 装配 ⬜ · cmd/desktop+工具 ⬜

## 下一步

- **波次 1（下一轮）**：M1.9 permissions / hooks。
- M1.1 遗留 → M7：boot 默认 workspace（`Count==0→Create`）+ `WorkspaceResolver` 注入 `IdentifyWorkspace` + `~/.forgify/` 共享资源布局落地（不分桶）。
