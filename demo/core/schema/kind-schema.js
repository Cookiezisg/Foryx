/* Anselm demo — KIND_SCHEMA：9 类实体页的声明式 schema（L3 强制层）。
   why：加一种实体 = 加一段 schema，不动核心组件。字段型 ∈ text|kv|code|json|rows|card；段 layout:'grid' → 块进响应式 2 列；块 type:'card'（含 title/icon/fields[]）= InfoCard 块，span:'full' 跨行。
   每类按后端 domains/<kind>.md 契约扎根（字段/段落投影自真后端，详见 entities-schema-spec 工作流的 grounding）。 */
(function () {
  window.KIND_SCHEMA = {
    function: { sections: [
  { label: "概览", variant: "plain", fields: [
    { key: "description", label: "说明", type: "text", editable: true },
    { key: "meta", type: "kv" },
  ]},
  { label: "代码", variant: "plain", fields: [
    { key: "code", type: "code", lang: "python" },
  ]},
  { label: "输入 / 输出", variant: "plain", layout: "grid", fields: [
    { type: "card", title: "输入", icon: "enter", fields: [{ key: "inputs", type: "kv" }] },
    { type: "card", title: "输出", icon: "send", fields: [{ key: "outputs", type: "kv" }] },
  ]},
  { label: "环境", variant: "plain", layout: "grid", fields: [
    { type: "card", title: "依赖", icon: "box", fields: [{ key: "dependencies", type: "rows" }] },
    { type: "card", title: "venv 状态", icon: "shield-check", fields: [{ key: "env", type: "kv" }] },
  ]},
  { label: "运行历史", variant: "plain", fields: [
    { key: "runs", type: "rows" },
  ]},
] },
    handler: { sections: [
  { label: "概览", variant: "plain", fields: [
    { key: "description", label: "说明", type: "text", editable: true },
    { key: "meta", type: "kv" },
  ]},
  { label: "常驻状态", variant: "plain", layout: "grid", fields: [
    { type: "card", title: "运行时", icon: "bot", fields: [{ key: "runtime", type: "kv" }] },
    { type: "card", title: "init 配置完整度", icon: "shield-check", fields: [{ key: "configState", type: "kv" }, { key: "missingConfig", type: "rows" }] },
  ]},
  { label: "init 参数", variant: "plain", fields: [
    { key: "initArgs", type: "kv" },
  ]},
  { label: "方法", variant: "plain", fields: [
    { key: "methods", type: "rows" },
    { key: "code", type: "code", lang: "python" },
  ]},
  { label: "调用记录", variant: "plain", fields: [
    { key: "calls", type: "rows" },
  ]},
] },
    agent: { sections: [
  { label: "概览", variant: "plain", fields: [
    { key: "description", label: "说明", type: "text", editable: true },
    { key: "meta", type: "kv" },
  ]},
  { label: "提示词", variant: "plain", fields: [
    { key: "prompt", type: "code", lang: "markdown" },
  ]},
  { label: "能力挂载", variant: "plain", layout: "grid", fields: [
    { type: "card", title: "工具挂载", icon: "plug", fields: [{ key: "tools", type: "rows" }] },
    { type: "card", title: "技能", icon: "book-open", fields: [{ key: "skill", type: "text", wrap: true }] },
    { type: "card", title: "知识", icon: "file-text", fields: [{ key: "knowledge", type: "rows" }] },
    { type: "card", title: "模型覆盖", icon: "bot", fields: [{ key: "modelOverride", type: "kv" }] },
  ]},
  { label: "挂载健康", variant: "plain", fields: [
    { key: "mountHealth", type: "rows" },
  ]},
  { label: "输入 / 输出", variant: "plain", layout: "grid", fields: [
    { type: "card", title: "输入", icon: "enter", fields: [{ key: "inputs", type: "kv" }] },
    { type: "card", title: "输出", icon: "send", fields: [{ key: "outputs", type: "kv" }] },
  ]},
  { label: "运行历史", variant: "plain", fields: [
    { key: "executions", type: "rows" },
  ]},
] },
    workflow: { sections: [
  { label: "概览", variant: "plain", fields: [
    { key: "description", label: "说明", type: "text", editable: true },
    { key: "meta", type: "kv" },
  ]},
  { label: "运行治理", variant: "plain", layout: "grid", fields: [
    { type: "card", title: "生命周期", icon: "clock", fields: [{ key: "lifecycle", type: "kv" }] },
    { type: "card", title: "并发策略", icon: "git-branch", fields: [{ key: "concurrency", type: "kv" }] },
  ]},
  { label: "告警", variant: "plain", fields: [
    { key: "attention", type: "rows" },
  ]},
  { label: "编排图", variant: "plain", fields: [
    { key: "graph", type: "graph" },
  ]},
  { label: "运行历史", variant: "plain", fields: [
    { key: "flowruns", type: "rows" },
  ]},
] },
    control: { sections: [
  { label: "概览", variant: "plain", fields: [
    { key: "description", label: "说明", type: "text", editable: true },
    { key: "meta", type: "kv" },
  ]},
  { label: "输入字段", variant: "plain", fields: [
    { key: "inputs", type: "kv" },
  ]},
  { label: "路由分支", variant: "plain", fields: [
    { key: "branches", type: "rows" },
  ]},
  { label: "分支详情", variant: "plain", layout: "grid", fields: [
    { type: "card", title: "When（CEL）", icon: "git-branch", fields: [
      { key: "when", type: "code", lang: "cel" },
    ]},
    { type: "card", title: "Emit（重写）", icon: "send", fields: [
      { key: "emit", type: "json" },
    ]},
  ]},
]},
    approval: { sections: [
  { label: "概览", variant: "plain", fields: [
    { key: "description", label: "说明", type: "text", editable: true },
    { key: "meta", type: "kv" },
  ]},
  { label: "提示模板", variant: "plain", fields: [
    { key: "template", type: "code", lang: "markdown" },
  ]},
  { label: "输入 / 决策规则", variant: "plain", layout: "grid", fields: [
    { type: "card", title: "输入", icon: "enter", fields: [{ key: "inputs", type: "kv" }] },
    { type: "card", title: "决策规则", icon: "shield-check", fields: [{ key: "decision", type: "kv" }] },
  ]},
  { label: "出口", variant: "plain", fields: [
    { key: "ports", type: "rows" },
  ]},
] },
    trigger: { sections: [
  { label: "概览", variant: "plain", fields: [
    { key: "description", label: "说明", type: "text", editable: true },
    { key: "meta", type: "kv" },
  ]},
  { label: "源配置", variant: "plain", fields: [
    { key: "sourceMeta", type: "kv" },
    { key: "config", type: "json" },
  ]},
  { label: "去重 / 输出", variant: "plain", layout: "grid", fields: [
    { type: "card", title: "去重键", icon: "git-branch", fields: [{ key: "dedup", type: "kv" }] },
    { type: "card", title: "输出", icon: "send", fields: [{ key: "outputs", type: "kv" }] },
  ]},
  { label: "Activations（触发判定审计）", variant: "plain", fields: [
    { key: "activations", type: "rows" },
  ]},
  { label: "Firings 收件箱（触发后未执行排查）", variant: "plain", fields: [
    { key: "firings", type: "rows" },
  ]},
] },
    mcp: { sections: [
  { label: "概览", variant: "plain", fields: [
    { key: "description", label: "说明", type: "text", editable: true },
    { key: "meta", type: "kv" },
  ]},
  { label: "连接", variant: "plain", layout: "grid", fields: [
    { type: "card", title: "状态", icon: "plug", fields: [
      { key: "status", label: "连接态", type: "text" },
      { key: "lastError", label: "最近错误", type: "text" },
    ]},
    { type: "card", title: "传输", icon: "git-branch", fields: [
      { key: "transport", type: "kv" },
    ]},
  ]},
  { label: "工具", variant: "plain", fields: [
    { key: "tools", type: "rows" },
  ]},
  { label: "调用记录", variant: "plain", fields: [
    { key: "calls", type: "rows" },
  ]},
  { label: "stderr 输出", variant: "plain", fields: [
    { key: "stderr", type: "code", lang: "text" },
  ]},
]},
    skill: { sections: [
  { label: "概览", variant: "plain", fields: [
    { key: "description", label: "说明", type: "text", editable: true },
    { key: "meta", type: "kv" },
  ]},
  { label: "Frontmatter", variant: "plain", fields: [
    { key: "frontmatter", type: "json" },
  ]},
  { label: "正文（指令）", variant: "plain", fields: [
    { key: "body", type: "code", lang: "markdown" },
  ]},
  { label: "激活", variant: "plain", layout: "grid", fields: [
    { type: "card", title: "inline 注入", icon: "enter", fields: [{ key: "inline", type: "rows" }] },
    { type: "card", title: "fork 派发", icon: "git-branch", fields: [{ key: "fork", type: "rows" }] },
  ]},
  { label: "allowed-tools 预授权", variant: "plain", fields: [
    { key: "allowedTools", type: "rows" },
  ]},
] },
  };
})();
