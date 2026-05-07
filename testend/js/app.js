// app.js — Alpine root store, tab registry, and root-level helpers.

// Single source of truth for tabs. Order = render order in the bar.
// Adding a tab is a one-place edit.
//
// 单一事实源。顺序即渲染顺序。新增 tab 只改这里。
const TESTEND_TABS = [
  { id: 'config',    label: 'Config' },
  { id: 'sse',       label: 'SSE' },
  { id: 'logs',      label: 'Logs' },
  { id: 'sql',       label: 'SQL' },
  { id: 'tests',     label: 'Tests' },
  { id: 'tools',     label: 'Tools' },
  { id: 'catalog',   label: 'Catalog' },
  { id: 'subagent',  label: 'Subagent' },
  { id: 'skill',     label: 'Skill' },
  { id: 'mcp',       label: 'MCP' },
  { id: 'sandbox',   label: 'Sandbox' },
  { id: 'mock-llm',  label: 'Mock LLM' },
  { id: 'wire',      label: 'Wire' },
  { id: 'processes', label: 'Processes' },
  { id: 'info',      label: 'Info' },
];

document.addEventListener('alpine:init', () => {
  Alpine.store('app', {
    conversationId: null,
    conversationTitle: '',
  });
});

// appRoot — top-level x-data on <body>. Owns activeRightTab and exposes
// the tab list to the template. Tabs use CSS flex-wrap to flow into
// multiple rows when the panel narrows; no JS measurement / dropdown.
//
// appRoot 是 body 顶层 x-data。owns activeRightTab + 暴露 tab 列表给模板。
// 窄 panel 用 CSS flex-wrap 多行排列，无 JS 测量 / dropdown。
function appRoot() {
  return {
    activeRightTab: 'config',
    allTabs: TESTEND_TABS,

    selectTab(tab) {
      this.activeRightTab = tab;
    },
  };
}
