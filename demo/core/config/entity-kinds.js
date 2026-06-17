/* Anselm demo — 实体类型单源（9 kind）。
   一处定义每种实体的 label / 图标语义 key（→ Lucide via icons.js ALIAS）/ 执行动词（对齐后端 N5）/ ID 前缀。
   feature / 原语只读它，禁各自再写一份。图节点 5 kind 的图标走 NODE_ICON。 */
(function () {
  window.ENTITY_KINDS = {
    function: { label: "Function", icon: "function", verb: "Run", prefix: "fn" },
    handler:  { label: "Handler",  icon: "handler",  verb: "Call", prefix: "hd" },
    agent:    { label: "Agent",    icon: "agent",    verb: "Invoke", prefix: "ag" },
    workflow: { label: "Workflow", icon: "workflow", verb: "Trigger", prefix: "wf" },
    trigger:  { label: "Trigger",  icon: "trigger",  verb: "Fire", prefix: "trg" },
    control:  { label: "Control",  icon: "control",  verb: "",     prefix: "ctl" },
    approval: { label: "Approval", icon: "approval", verb: "",     prefix: "apf" },
    mcp:      { label: "MCP",      icon: "mcp",      verb: "",     prefix: "mcp" },
    skill:    { label: "Skill",    icon: "skill",    verb: "",     prefix: "skill" },
  };
  window.NODE_ICON = { trigger: "trigger", action: "action", agent: "agent", control: "control", approval: "approval" };

  // 实体引用（能力挂载 / 关系边）→ 其 kind 图标：与左岛同源，挂载图标即左岛分组图标。
  // why：ID 前缀（S15 <prefix>_<16hex>，mcp 用 mcp:）已编码 kind——单源派生、不让消费方各写一份。
  const PREFIX = {};
  Object.keys(window.ENTITY_KINDS).forEach((k) => (PREFIX[window.ENTITY_KINDS[k].prefix] = k));
  window.kindIconOf = function (ref) {
    if (ref == null) return null;
    const s = String(ref), p = s.indexOf(":") >= 0 ? s.slice(0, s.indexOf(":")) : s.slice(0, s.indexOf("_") >= 0 ? s.indexOf("_") : s.length);
    const kind = PREFIX[p];
    return kind ? window.ENTITY_KINDS[kind].icon : null;
  };
})();
