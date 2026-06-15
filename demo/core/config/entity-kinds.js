/* Forgify demo — 实体类型表（单一事实源，收掉 entities.js / sidebar / scheduler NICON 三处漂移副本）。
   9 类：Quadrinity(function/handler/agent/workflow) + 图节点(trigger/control/approval) + 连接(mcp) + 技能(skill)。
   字段：label · icon(icons.js key) · verb(N5 执行动词) · vico(动词图标) · prefix(ID 前缀)。 */
window.ENTITY_KINDS = {
  function: { label: 'Function',   icon: 'function', verb: 'Run',       vico: 'play', prefix: 'fn' },
  handler:  { label: 'Handler',    icon: 'handler',  verb: 'Call',      vico: 'play', prefix: 'hd' },
  agent:    { label: 'Agent',      icon: 'agent',    verb: 'Invoke',    vico: 'play', prefix: 'ag' },
  workflow: { label: 'Workflow',   icon: 'workflow', verb: 'Trigger',   vico: 'zap',  prefix: 'wf' },
  trigger:  { label: 'Trigger',    icon: 'trigger',  verb: 'Fire',      vico: 'zap',  prefix: 'trg' },
  control:  { label: 'Control',    icon: 'control',  verb: 'Probe',     vico: 'play', prefix: 'ctl' },
  approval: { label: 'Approval',   icon: 'shield',   verb: 'Render',    vico: 'play', prefix: 'apr' },
  mcp:      { label: 'MCP server', icon: 'mcp',      verb: 'Reconnect', vico: 'spin', prefix: 'mcp' },
  skill:    { label: 'Skill',      icon: 'skill',    verb: 'Render',    vico: 'play', prefix: 'skl' },
};
// 图节点 5 kind → 图标 key（approval 复用 shield；其余 kind 即 key）
window.NODE_ICON = { trigger: 'trigger', action: 'action', agent: 'agent', control: 'control', approval: 'shield' };
