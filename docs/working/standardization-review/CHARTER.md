---
id: WRK-020
type: working
status: active
owner: @weilin
created: 2026-06-13
reviewed: 2026-06-13
review-due: 2026-09-13
expires: 2026-09-13
landed-into: ""
audience: [human, ai]
---

# 标准化宪章 —— 后端十轴唯一标准（第 1 轮产物，2026-06-13）

> **北极星**：前端消费**任意**接口只需**一套心智模型**——envelope 一个样、分页一个样、错误一个样、SSE 帧一个样、命名/资源-动作语法各一套确定性规约;后端内部同类实现也一个模板。
> **侦察基础**:14 切片穷举全后端 = **682 原子项 / 224 轴观察**;归出 **59 偏差(12 high)**,其中 **33 待改 / 26 有意分化(byDesign,仅落档)** + **24 项待裁决**。本文是"那部标准",改代码是其下游(第 2/3 轮)。

## 十轴标准（canonical 浓缩,字面依据 N/D/E/S 宪法）

### 1. envelope（顶层零偏离,争议在 data 内层）
- 成功只经四 helper:`Success`→`{data}` / `Created`=201 / `Paged`→`{data:[],nextCursor?,hasMore}` / `NoContent`=204。失败只经 `Error`→`{error:{code,message,details}}`,统一过 `FromDomainError`(Kind→status)。**此两层已全后端一致、设为不可动锚点。**
- **data 内层铁律**:GET 单实体 / PATCH / List item **必须裸实体**,不裹 `{<entity>:...}`。**Create 待裁决**(MD1)。复合读(`/flowruns/{id}`→`{flowrun,nodes}`)允许具名多 key、key=复数 camelCase 资源名。
- 分页坐标**恒在 envelope 顶层**,绝不埋进 data(唯一偏离:`/agents/{id}/executions` 把整个结果埋 data)。
- LLM tool JSON 不走 envelope(合理),但工具间须统一:集合 `{count,<entityPlural>:[]}`、分页直出 `Search*Result`(MD7)。

### 2. pagination（N4）
- 请求侧:所有 List 统一 `?cursor=&limit=` 经 `ParsePage`(limit 默认 50、钳 [1,200]、非法→400);**禁 handler 自解析**(search 现违,见 MD2)。
- 响应侧:统一 `Paged`,`data` 纯数组、`nextCursor/hasMore` 顶层。携聚合页:聚合进 `data` 子对象、分页仍顶层(MD2)。
- 有界集合(skills/documents 树/mcp-servers/workspaces/memories/relations 图)byDesign 不分页,返 `Success(array)`,不伪装游标。

### 3. action-grammar（N5）
- URL:实体级 `POST /{resource}/{id}:{action}` / 集合级 `POST /{resource}:{action}`;占位统一 `{idAction}`/`{nameAction}`。
- 动词全集(语义唯一禁同义):执行 `:run/:call/:invoke/:trigger/:fire`;版本 `:edit/:revert/:activate/:deactivate`;AI `:iterate/:triage`;生命周期 `:restart/:stage/:kill/:replay/:decide`;诊断 `:test`。
- **响应按语义类**(降前端心智):创建→见 MD1;执行/异步→`202 {data:{<执行ref>}}`(键名待 MD3);AI 对话→`202 {data:{conversationId}}`(已统一);状态变更→**统一返实体后置快照**,禁 `{staged:true}`/`{killed:N}` 裸键(MD4);无返回→204。
- 子路径 vs 冒号(`/stream` cancel、`/read`、`/pin`):待 MD5。

### 4. naming（N3）
- 线缆全 camelCase / DB 全 snake_case(domain struct 双 tag 桥接)——**已达标**。
- 路径占位 camelCase:单主键 `{id}`、跨资源 `{<entity>Id}`、唯一名 `{name}`、动作 `{idAction}`;**禁 PascalCase `{conversationID}`**(现 1 处)。
- SSE type 统一 `<domain>.<action>`、action 过去式;领域特殊事件(lifecycle_changed 等)保留但须登记 events.md。
- LLM tool 名:实体工具 `<verb>_<entity>` snake_case + 执行动词与 HTTP `:action` 对齐;内置容器工具(Bash/Read/Subagent...)PascalCase 为对齐外部生态的有意例外。

### 5. errors（S20/N1）
- 两段式:① 构造一律 `errorspkg.New(Kind,Code,Msg)`,Code=`<ENTITY>_<REASON>`;禁标准库 `errors.New` 造命名 sentinel。② 出口 transport 只调 `FromDomainError`,`statusForKind` 是 Kind→status 唯一来源。
- **铁律:transport 禁止裸调 `Error(w,status,"RAW_CODE",...)`**——校验/路由错误也须 sentinel 化经 FromDomainError,使 code 进 error-codes.md + 被唯一性守卫覆盖(现 19 处违、8 码漏登,见 MD-err)。

### 6. sse（E1/E2/E3）
- 仅三流、共用 `StreamHandler`、workspace 级不过滤。统一 Envelope `{seq,scope:{kind,id},id,frame}`。四动词 `Open/Delta/Close/Signal`。Node `{type,content}` 协议不枚举类型(producer 定词表,events.md 作 reference 登记,MD-sse2)。
- **durable/ephemeral 硬规则**:Open/Close 恒 durable、Delta 恒 ephemeral、Signal 由 `Ephemeral` 定。"DB 行才是真相、流只为实时呈现"的点状广播 MUST 置 `Ephemeral:true`——**flowrun tick / trigger fire 现未置、被当 durable 占 replay 环(high 偏差,MD-sse1)**。
- 重连 Last-Event-ID/`?fromSeq`→int64 seq;嵌套仅 messages 流 `parentId`。

### 7. identity（S15）
- ID=`<prefix>_<16hex>`,唯一入口 `idgenpkg.New`,**每 prefix 必登记 database.md**(现漏一批运行时/infra PK)。
- 路径身份键二分(byDesign):row-id 实体 `/{资源}/{id}`、slug 实体 `/{资源}/{name}`(skill/memory/mcp);Log 单读独立顶层资源 + `{id}`(路径变量名统一 `{id}`,MD-id4)。
- 文档勘误:`noti_`(非 ntf_)、`aki_`(非 key_)、`se_/sr_` 登记、`fnenv_/hdenv_` 标注为 owner_id 非表 PK(MD8——代码即真相,改文档)。

### 8. db（D1/D2/D3）
- 四角色表形:① 身份/配置表(workspace_id + created/updated/deleted 三列 + name partial-UNIQUE + 游标索引);② 版本表(只增、UNIQUE(entity_id,version)、无 deleted_at);③ Log 表(只增、无 deleted_at、5 溯源列、status CHECK、幂等 UNIQUE);④ 派生/系统表(search/sandbox/relations,D1/D2 各有显式豁免机制)。
- D2 对外口径重述:"业务表 orm 自动隔离 + 三类有意豁免(search 手写谓词/sandbox owner-id/workspaces 自身即边界)"(MD-db)。

### 9. entity-skeleton
- 实体 = 单一 domain 结构体、字段双 tag(db snake / json camel)、私密 `json:"-"`、骨架字段四族(身份/状态/载荷/时间戳)定序。
- Create 返裸实体(版本实体的 wrapper 待 MD1)。Log 表同骨架(仅专属外键/列不同),Status 统一 `[ok,failed,cancelled,timeout]`、TriggeredBy 统一 `[chat,agent,workflow,manual]`。命名 `Execution` vs handler `Call` 待 MD-skel。
- DTO 能复用 domain 类型就 type-alias(不在 app 重声明);7 个 `search_<entity>` 的 slim struct 抽 domain 级 `EntitySlim` 去重。

### 10. internal-shape
- 状态码样板已宪法化(201/202/204/200 各对应 helper)。
- **Service 构造器统一 `NewService(...)`**(现 13 用 NewService / 12 用裸 New,纯历史随意,MD-ctor)。
- tool Execute 返回 `(string,error)`:结构数据→`ToJSON`、内容正文→prose(document list/search 落 prose 侧待议 MD7)。S18 五方法 + 框架字段已 100% 一致。
- 生命周期 `publish(ctx,action,id,extra)` + `notifySearch`;后置注入统一 `Set<Cap>` nil-tolerant(注入机制统一、注入内容随域分化=byDesign)。

## 决策台账（24 项）—— ✅ 已批准定稿（2026-06-13）

> **批准结果**:A 组 **MD1 裸实体+内嵌 currentVersion / MD3 单产物统一 `{id}` / MD4 统一返实体后置快照 / MD5 逐个判+写进 N5** 经用户确认取推荐;**MD2/MD6/MD7/MD8 + B 组 7 项**默认按推荐生效。下表"推荐"列即**已批准的 canonical**,第 2/3 轮据此对标与归一。

### A. 宏观契约决策（定义前端契约/有产品取舍,需拍板）

| # | 决策 | 推荐 |
|---|---|---|
| **MD1** | 版本实体 Create 响应:`{entity,version}` wrapper vs 裸实体+内嵌 currentVersion | **裸实体 + currentVersion 字段**(create 与 get 同形,前端一套解构;trigger 顺带补齐) |
| **MD2** | 携聚合列表的分页/聚合位置 | **分页恒顶层(Paged)、aggregates 进 data 子对象**;search limit 改走 ParsePage |
| **MD3** | 执行/异步动作返回新 id 的键名 | **单产物统一 `{id}`**(messageId/flowrunId→id),多值动作(:fire)保扁平 camelCase 多键 |
| **MD4** | 状态变更动作(:stage/:kill/:replay...)返回 | **统一返受影响实体后置快照**,禁 `{staged:true}`/`{killed:N}` 裸键 |
| **MD5** | 子资源动作语法(`/stream` cancel、`/read`、`/pin`) | **逐个判 + 写进 N5**:真子资源(stream/read/content)走 CRUD、纯动作(pin/unpin/reindex)走 `:action` |
| **MD6** | `:invoke`/`:call`/`:run`/`:trigger` 是否归一 | **保留专名 + api.md 立"实体→执行动词"对照表**(语义自描述 > 强行归一) |
| **MD7** | LLM tool JSON 是否纳入统一(含 document list/search prose→JSON) | **纳入**:集合工具 `{count,<plural>:[]}`、分页直出 Search*Result、document 收敛 ToJSON |
| **MD8** | ID 前缀代码↔文档冲突(noti_/aki_/se_/sr_) | **代码即真相,改文档**(零迁移;未上线如在意命名密度可反向,见各项) |

### B. 机械/文档决策（代码即真相或多数派明确,默认按推荐执行,你可否决）

| # | 决策 | 推荐 |
|---|---|---|
| MD-err | transport 19 处裸 `Error` + 8 漏登码 | sentinel 化经 FromDomainError + 补登 error-codes.md + 加 AST 守卫禁裸调 |
| MD-sse1 | flowrun tick / trigger fire 未置 ephemeral | `Signal` 加 `ephemeral bool` 形参,两处置 true(chat interaction 是正面模板) |
| MD-sse2 | events.md 是否登记 node.type 词表 | reference 文档登记当前全集 + 标"非穷举"(domain 仍不枚举) |
| MD-ctor | Service 构造器 New vs NewService | 统一 `NewService`(13 vs 12 多数派 + 装配语境自描述) |
| MD-skel | 执行日志实体名 Execution vs handler Call | 保语义名、**统一 wire 形状/包络字段** |
| MD-id4 | Log 单读路径变量 execId/callId/actId | 统一 `{id}`(不上线缆、前端零感知) |
| MD-db | D1/D2 对外口径措辞 | 重述为"软删=隐藏+内容日志永久审计"/"业务表自动隔离+三类显式豁免" |

## 下一步

1. **你裁决 A 组 8 个宏观契约决策**(B 组默认按推荐,你可点否决项)→ �provided 定稿。
2. **第 2 轮:叶子穷尽符合性**——以定稿宪章当尺子,逐端点/事件/码/字段对标,出全量偏差矩阵(33 待改逐条 file:line + 修法)。
3. **第 3 轮+:归一化执行波 S1..Sn**——按矩阵逐波改齐,每波黑盒+verify+文档 1:1 同步、独立提交。
