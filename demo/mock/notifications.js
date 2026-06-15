/* Forgify demo — 通知示意数据层（DTO 镜像后端 references/notifications）。绝不连后端。
   后端事实：notifications 流唯一 durable actionable = workflow.approval_pending（→ /flowruns/{id}/approvals/{node}:decide）；
   其余 ~15 类均 FYI 生命周期（function/handler/agent/workflow/skill/mcp/document/conversation/memory/sandbox 增删改）。
   后端无人类可读文案/严重级——文案 + 状态点由前端按 type+payload 自渲（此处用示意数据）。
   字段（camelCase 线缆）：id · type · title · st(5 态) · time · unread · refKind/refId(被提及实体→Intent.select) · refLabel。
   actionable(approval_pending) 额外带 prompt(停泊 flowrun_nodes 行的 markdown 摘要) + ddl。
   消费者：features/notifications（侧栏 Inbox）。键 = 分区（needs / groups[label] / read）。 */
(function () {
  window.MOCK_NOTIFICATIONS = {
    // ① 待决：仅 workflow.approval_pending（唯一 actionable）。prompt 真身 = 停泊行 markdown，此处示意。
    needs: [
      { id: 'ntf_a1f3', type: 'workflow.approval_pending', title: '竞品动态日报流程 · 发布前过目', time: '14:05',
        refKind: 'workflow', refId: '竞品监控流', refLabel: '竞品监控流',
        prompt: '即将向 #marketing 发布今日 3 条竞品动态摘要，确认内容无误后批准。\n\n· 来源：fetch_news（function）\n· 条数：3\n· 去向：Slack #marketing', ddl: '自动驳回 22h' },
      { id: 'ntf_a2c8', type: 'workflow.approval_pending', title: '账单对账流程 · 金额超阈值确认', time: '昨天',
        refKind: 'workflow', refId: '账单对账流', refLabel: '账单对账流',
        prompt: '本月对账差额 ¥1,280 超过自动通过阈值 ¥1,000，需人工确认后继续入账。', ddl: '自动驳回 6h' },
    ],

    // ② 时间线：FYI 生命周期，按日期分组（newest-first，对齐后端 List）。st: done/err/idle/run/wait；unread 靠字色明暗。
    groups: [
      { label: '今天', items: [
        { id: 'ntf_b1', type: 'handler.crashed',        title: 'Webhook 入库 handler 崩溃（已自动重启）', st: 'err',  time: '13:40', unread: true,  refKind: 'handler',  refId: 'hd_webhook',  refLabel: 'Webhook 入库' },
        { id: 'ntf_b2', type: 'workflow.run_failed',     title: '账单对账流 运行失败 · extract 超时',        st: 'err',  time: '11:22', unread: true,  refKind: 'run',      refId: 'fr_5b3c10',   refLabel: '账单对账流' },
        { id: 'ntf_b3', type: 'function.edited',         title: 'PDF 提取 function 已发布新版本 v5',          st: 'done', time: '10:08', unread: true,  refKind: 'function', refId: 'fn_process_invoice', refLabel: 'process_invoice' },
        { id: 'ntf_b4', type: 'conversation.compacted',  title: '「Researcher 调优」会话已压缩',              st: 'idle', time: '09:30', refKind: 'conversation', refId: 'cv_researcher', refLabel: 'Researcher 调优' },
      ] },
      { label: '昨天', items: [
        { id: 'ntf_c1', type: 'agent.updated',    title: 'Researcher agent 配置已更新',  st: 'idle', time: '周二', refKind: 'agent', refId: 'ag_researcher', refLabel: 'Researcher' },
        { id: 'ntf_c2', type: 'mcp.reconnected',  title: 'Notion 连接器已重新连上',      st: 'done', time: '周二', refKind: 'mcp',   refId: 'mcp_notion',   refLabel: 'Notion' },
      ] },
      { label: '过去 7 天', items: [
        { id: 'ntf_d1', type: 'skill.created',    title: '新增技能「网页摘要」',         st: 'done', time: 'Jun 9', refKind: 'skill', refId: 'skl_web_summary', refLabel: '网页摘要' },
        { id: 'ntf_d2', type: 'document.moved',   title: '「上线清单」已移动到 归档/',   st: 'idle', time: 'Jun 8', refKind: 'document', refId: 'doc_launch', refLabel: '上线清单' },
      ] },
      { label: '更早', items: [
        { id: 'ntf_e1', type: 'workflow.lifecycle_changed', title: '研报抓取流 已激活', st: 'idle', time: 'May 28', refKind: 'workflow', refId: '研报抓取流', refLabel: '研报抓取流' },
      ] },
    ],

    // ③ 已读：标记已读沉底（折叠）
    read: [
      { id: 'ntf_r1', type: 'function.created', title: '新建 fetch_news function', st: 'done', time: 'Jun 7', refKind: 'function', refId: 'fn_fetch_news', refLabel: 'fetch_news' },
      { id: 'ntf_r2', type: 'memory.updated',   title: '记忆「项目偏好」已更新',    st: 'idle', time: 'Jun 6', refKind: 'document', refId: 'doc_memory', refLabel: '项目偏好' },
    ],
  };
})();
