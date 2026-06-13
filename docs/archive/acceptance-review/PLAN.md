---
id: WRK-013
type: working
status: archived
owner: @weilin
created: 2026-06-12
reviewed: 2026-06-12
review-due: 2026-09-12
expires: 2026-09-12
landed-into: ""
audience: [human, ai]
---

# PLAN —— 全产品真机验收 + 体验审查总计划（用户批准版，2026-06-12）

## 总纲

- **两柱**：柱 A = 全功能真机验收（真起后端、真打请求、真跑模型）；柱 B = 六视角×六状态体验审查。共用环境、样本数据、promptdump、金标任务。
- **每个 feature 的验收单元** = 功能本体 × **情况矩阵**（正常/边界/出错/并发/降级）× **涟漪面**（创建→可见性涟漪：搜索/关系/catalog/通知；删除→残留涟漪；修改→生效涟漪）。
- **三列判定**：用户面（HTTP 真调成功且语义对）/ 产品逻辑（状态机、级联、记账全对）/ LLM 面（工具链真驱动得了）。

## 柱 A：逐域功能×情况清单

### A1 Function（锻造执行原型域）

| 功能 | 必验情况 |
|---|---|
| 创建（HTTP 直建 / create_function / :iterate 锻造） | 正常；坏代码无 def；脏 JSON 进 jsonrepair；重名 |
| 编辑 ops 全集 | set_code/inputs/outputs/deps/python_version 逐个；空 ops（env 重建路径）；中途非法 op |
| 版本 | 列表分页；revert；cap 裁剪不裁 active；版本号单调 |
| 运行 | active/指定版本；print→logs 三写真到（chat 进度块+面板终端+落库）；非零退出→failed+stderr 进 error；超时/取消记账；env 被删→自愈重建重试；sandbox 未就绪降级；并发 run |
| 执行记录 | 列表过滤×5+聚合徽标；详情带 logs；列表不带 logs |
| 删除 | 软删；涟漪：搜索索引清、关系边清、agent 挂载它的下次 invoke 表现、workflow capability_check 报缺、执行记录保留（D1） |
| 四调用源记账 | chat/agent/workflow/sensor 各跑一次，TriggeredBy+溯源列各对 |

### A2 Handler（常驻进程域）

A1 全部 + 特有：实例生命周期（首调 spawn/crash 后重生/restart/stop/app 退出优雅关）；config（PUT 合并/掩码回显/DELETE 清/必填缺失拒 spawn/改完自动重启真生效）；方法级超时真触发；yield 流式真到三处；print 到 stdout 炸协议→实例判 crash→自动重生（产品语义真验）；stderr 窗口归属；版本切换实例重启。

### A3 Control / Approval（内联语义域）

CRUD/版本/:iterate；CEL 分支语法校验拒坏表达式；真实战场在 workflow 内：control 真路由（port 选边+emit 字段下游可读）、approval 真渲染模板→parked→决策（yes 走 yes 边/no 停）→超时三政策（reject/approve/fail）真等到超时。

### A4 Agent（LLM 实体域）

CRUD/版本/modelOverride 优先级真生效；挂载三类（fn_、hd_.method、mcp:server/tool）真合成专属工具且真调通；invoke 三入口（chat 的 invoke_agent 嵌套块流、HTTP :invoke、workflow 节点）；transcript 落库+executions 查询；挂载物被删后 invoke 的表现（悬空涟漪）；输出 schema 约束。

### A5 Workflow + Trigger + Flowrun（编排域，最重）

| 功能 | 必验情况 |
|---|---|
| 图编辑 | ops 全集；校验拒：无 trigger/孤儿节点/坏 port/自环；capability_check 缺挂载报告 |
| 活监听 | activate/stage（一次性真验只触发一次）/deactivate（drain 真验）/kill 真杀在途；edit 换入口 trigger 重绑复验 |
| Trigger 四 kind 全真跑 | cron 真等到点；webhook 真 POST（HMAC 验签+坏签 401+超大 body 413）；fsnotify 真改文件；sensor 真轮询（CEL 真/假/probe 报错三态看 activation） |
| firing 政策 | dedup 幂等；overlap skip/buffer_one/allow 真并发验；shed |
| Flowrun 执行 | 线性/分支汇合（active-branch join）/循环（iteration 递增）/并行扇出；payload CEL 寻址；pin 语义真验（运行中改实体在途 run 跑旧版）；replay（completed 抄行不重跑）；**崩溃恢复：执行中途 kill -9，重启 Recover 续跑到完成** |
| 唤回环 | run 失败→通知+attention 亮；replay 成功→熄；approval park→通知（复验） |

### A6 MCP（外部生态域）

registry 浏览/install（requiredEnv 缺失拒+提示）/手动 PUT/import mcp.json；真装真实 server（filesystem 或 fetch）→连接→动态工具真调→calls 记账+聚合+logs；失败态/3 连败 degraded/恢复/reconnect；stderr ring 失败附尾；卸载清理（进程/索引/关系/动态工具消失）。

### A7 Search（全况波）

| 维度 | 必验情况 |
|---|---|
| 词法 | 中文 2 字（LIKE）/3+ 字（trigram）/中英混合/代码符号/FTS 注入字符/空查询 400 |
| 语义 RAG 全链 | builtin：真下载→真嵌入→真混合检索（中文查英文命中）；ollama：真连+切自定义 baseURL/model+错误态可见；off：纯词法；三态互切→向量失效→后台重嵌真完成 |
| 12 实体投影 | 逐实体建→搜到→改→新内容搜到→删→搜不到；conversation 增量；handler 方法/mcp 工具粒度 |
| 同步 | 写后可搜延迟；杀进程丢事件→boot 对账自愈；reindex 202→重建完整 |
| 检索质量 | exact-name 置顶/前缀次之；折叠+matchedChunks；分页 cursor 不重不漏+异查询拒；归档过滤；隔离 |
| LLM 口 | search_blocks 三段精度链各自真触发；8 垂搜+降级链；search_conversations；Retrieve（MaxChars） |

### A8 Chat / Conversation / Memory / Skill / Attachment / Todo（对话域）

真对话全链（流式 block 顺序/工具调用/并行工具批/progress/subagent 嵌套树）；打断 stop；长对话真触发压缩→压缩后继续对话语义不丢；自动标题（utility 缺席静默降级真验）；归档/Send 自动解档/删除取消在途生成；@mention 冻结；附件三路（vision/native PDF/sandbox 提取）；memory 写读忘+索引注入；skill 两路 activate+allowed-tools 免确认真验；todo_write 真写真见 reminder；消息重连重水合。

### A9 平台域

workspace CRUD/最后一个拒删/删除级联逐资产验残留；模型三场景配置→真生效；apikey CRUD/:test 真探活/被引用拒删；limits 每字段 PATCH→对应行为真变；sandbox runtimes 装/删/disk-usage/gc；通知全事件真到达+未读+已读；SSE 三流：重连 replay 不丢 durable、E2 ephemeral 不进 buffer、E3 嵌套。

### A10 跨域涟漪矩阵（收口波）

机械表：{创建/改名/删除} × 12 实体 → {搜索索引、关系图、catalog、通知、挂载方、引用方} 六涟漪面全部如期变化。

## 柱 B：体验审查

- 六视角：Chat 主 LLM / Subagent / Agent 实体 / Utility / 用户 / 前端开发者——各自「UI」独立 dump 独立审。
- 六状态：空态（自举）/正常/规模态（200 实体）/降级态/崩溃恢复态/长程压缩后态。
- 横切六刀：prompt lint、tool_result 形状、安全契约体验、多模型矩阵（后置）、token 成本账单、i18n 接缝。
- W0 资产：promptdump——llmmock 线缆抓包即「模型真实看到的全部」，四 LLM 视角 × 六状态落盘。

## 柱 C：金标 LLM 旅程（12 条，真模型，make evals）

①空 workspace 自举引导配置 ②从零建 function 调通看日志 ③建 handler 配 config 调方法 ④搓三节点 workflow 真触发到 parked ⑤调试埋雷 function 至修好 ⑥装 MCP 调工具 ⑦search_blocks 找积木接线 ⑧回忆历史对话 ⑨写读 memory ⑩激活 skill 干活 ⑪跨压缩边界长任务 ⑫降级态完成主链路。

## 产出物

验收台账（场景即 go test，函数名即台账行）· findings（PR-N 亲验）· testend/ 可重跑验收套件（make testend）· promptdump · 金标套件（make evals）· 终报。

## 波次

W0 环境+座架 → W1 锻造域(A1-A3) → W2 编排域(A5) → W3 集成域(A6+A7) → W4 对话域(A8+A4 的 chat 面) → W5 平台域(A9)+涟漪(A10) → W6 体验静态(柱B) → W7 金标(柱C) → W8 修复收口+终报。每波收口提交，发现随波修。
