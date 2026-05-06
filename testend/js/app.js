// app.js — Alpine root store, shared utilities, tab registry.

// Tab registry — central definition shared between the visible tabs-bar
// and the More-▾ dropdown overflow menu. Add a new tab in one place,
// not two. Order here is the order rendered.
//
// Tab 注册表——主 bar 和 More 下拉共享一份。新增 tab 只改这里，不用两边
// 改。这里的顺序就是渲染顺序。
const TESTEND_CORE_TABS = [
  { id: 'config', label: 'Config' },
  { id: 'sse',    label: 'SSE' },
  { id: 'logs',   label: 'Logs' },
  { id: 'sql',    label: 'SQL' },
  { id: 'tests',  label: 'Tests' },
  { id: 'tools',  label: 'Tools' },
];

// Spill into "More ▾" — domain panels (catalog/subagent/skill/mcp/sandbox)
// + dev-only diagnostics (mock-llm/wire/processes/info). Keeps the
// always-visible bar tight (6 items) regardless of how narrow the right
// panel gets.
//
// 折进 "More ▾" 的 tab —— 各 domain 面板 + dev 诊断面板。让主 bar 保持紧凑
// （6 个固定项），不论右栏被拖多窄。
const TESTEND_MORE_TABS = [
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

function appRoot() {
  return {
    activeRightTab: 'config',
    moreTabsOpen: false,

    // Expose the tab registries to templates.
    coreTabs: TESTEND_CORE_TABS,
    moreTabs: TESTEND_MORE_TABS,

    selectTab(tab) {
      this.activeRightTab = tab;
      this.moreTabsOpen = false;
    },

    isMoreTabActive() {
      return TESTEND_MORE_TABS.some(t => t.id === this.activeRightTab);
    },

    activeMoreLabel() {
      const t = TESTEND_MORE_TABS.find(t => t.id === this.activeRightTab);
      return t ? t.label : '';
    },
  };
}
