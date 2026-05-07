// app.js — Alpine root store, shared utilities, tab registry,
// and the responsive tabs-bar manager.

// Single source of truth for tabs. Order = priority: leftmost
// (Config) is shown first, rightmost (Info) is the first to spill
// into the More-▾ dropdown when the panel narrows.
//
// 单一事实源。顺序即优先级：最左 Config 总是显示，最右 Info 在面板
// 变窄时第一个被折叠到 More 下拉。
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

// appRoot — top-level x-data on <body>. Owns activeRightTab plus the
// responsive tabs manager (visibleCount auto-shrinks/grows with
// container width via ResizeObserver + offsetWidth measurements).
//
// appRoot 是 body 顶层 x-data。owns activeRightTab 与响应式 tabs
// 管理器（visibleCount 跟随容器宽度自动伸缩，靠 ResizeObserver +
// offsetWidth 测量驱动）。
function appRoot() {
  return {
    activeRightTab: 'config',
    moreTabsOpen: false,

    // All tabs, in priority order. Templates iterate this directly
    // (so adding a tab is a one-place edit in TESTEND_TABS above).
    // 所有 tab 按优先级。模板直接 iterate（新增 tab 只改 TESTEND_TABS）。
    allTabs: TESTEND_TABS,

    // Number of tabs currently visible in the bar. Initialized to
    // 'all' so first paint shows everything; ResizeObserver + the
    // first measurement after $nextTick narrow it as needed.
    // 当前 bar 内可见 tab 数。初值 'all' 让首屏全显，首次测量后收缩。
    visibleCount: TESTEND_TABS.length,

    // Cached per-tab pixel widths from the most recent measurement.
    // Recomputed only when the tab list itself changes (rare); the
    // resize-driven recalc just re-uses these.
    // 每个 tab 的最新像素宽度。tab 列表变化才重测；resize 重算复用此缓存。
    _tabWidths: [],
    _moreBtnWidth: 80, // estimate until first measurement
    _ro: null,         // ResizeObserver handle

    selectTab(tab) {
      this.activeRightTab = tab;
      this.moreTabsOpen = false;
    },

    // Tabs spilled into the More dropdown right now.
    // 当前折进 More 下拉的 tab。
    get hiddenTabs() {
      return this.allTabs.slice(this.visibleCount);
    },
    get visibleTabs() {
      return this.allTabs.slice(0, this.visibleCount);
    },
    get hasOverflow() {
      return this.visibleCount < this.allTabs.length;
    },

    isMoreTabActive() {
      return this.hiddenTabs.some(t => t.id === this.activeRightTab);
    },
    activeMoreLabel() {
      const t = this.hiddenTabs.find(t => t.id === this.activeRightTab);
      return t ? t.label : '';
    },

    // initTabsManager — wires ResizeObserver to a tabs-bar element and
    // measures initial widths. Called via x-init on the .tabs-bar
    // element (passes itself as $el).
    //
    // initTabsManager 把 ResizeObserver 接到 tabs-bar 元素 + 测初始宽度。
    // 通过 .tabs-bar 上的 x-init 调用。
    initTabsManager(barEl) {
      // First-paint measurement: render all tabs visible (visibleCount
      // is already TESTEND_TABS.length), wait for layout, then measure
      // each .tab-btn's natural width. After this we have ground truth
      // and can compute visibleCount for any container size.
      //
      // 首屏测量：所有 tab 都先渲染，layout 后测每个 .tab-btn 的自然宽度。
      // 之后任意容器宽度都能算出 visibleCount。
      const measure = () => {
        const tabs = barEl.querySelectorAll('[data-tab-btn]');
        if (tabs.length === 0) return;
        this._tabWidths = Array.from(tabs).map(el => {
          // Include any margin/border via getBoundingClientRect width
          // + 1px buffer for sub-pixel rounding.
          // 用 getBoundingClientRect 拿宽度 + 1px 防亚像素舍入。
          return Math.ceil(el.getBoundingClientRect().width) + 1;
        });
        const moreBtn = barEl.querySelector('[data-more-btn]');
        if (moreBtn) {
          this._moreBtnWidth = Math.ceil(moreBtn.getBoundingClientRect().width) + 8;
        }
        this._recalc(barEl);
      };

      // Initial measure after Alpine renders the all-visible state.
      // Alpine 渲染完全可见态后第一次测量。
      requestAnimationFrame(() => requestAnimationFrame(measure));

      // ResizeObserver for ongoing recalc when the container width
      // changes (drag handle resize, window resize). Throttled to
      // animation frames so rapid resizes don't thrash.
      // ResizeObserver 持续监听容器尺寸变化（拖动 / 窗口）。throttle 到帧。
      let pending = false;
      this._ro = new ResizeObserver(() => {
        if (pending) return;
        pending = true;
        requestAnimationFrame(() => {
          pending = false;
          this._recalc(barEl);
        });
      });
      this._ro.observe(barEl);
    },

    // _recalc — given measured widths, decide how many tabs fit in the
    // current container width, leaving room for More-button when there
    // IS overflow. The "all fit" case skips the More button entirely.
    //
    // _recalc：用已测宽度算当前容器能装几个 tab；有溢出才给 More 按钮留位。
    // 全装下时直接全显，不留 More 位。
    _recalc(barEl) {
      if (this._tabWidths.length === 0) return;
      const containerWidth = barEl.clientWidth;
      const totalWidth = this._tabWidths.reduce((a, b) => a + b, 0);

      // Fast path: every tab fits → show all, no More button.
      // 快路径：全装下 → 全显，不要 More。
      if (totalWidth <= containerWidth) {
        this.visibleCount = this._tabWidths.length;
        return;
      }

      // Slow path: some tabs overflow. Reserve space for More button,
      // accumulate widths, count how many fit.
      // 慢路径：溢出。给 More 留位，累加宽度看能装几个。
      const budget = containerWidth - this._moreBtnWidth;
      let acc = 0;
      let count = 0;
      for (const w of this._tabWidths) {
        if (acc + w > budget) break;
        acc += w;
        count++;
      }
      // Always show at least 1 tab even on absurdly narrow panels.
      // 极窄时至少保留 1 个 tab。
      this.visibleCount = Math.max(1, count);
    },
  };
}
