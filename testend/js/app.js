// app.js — Alpine root store, tab registry, and root-level helpers.

// Single source of truth for tabs. Order = render order in the bar.
// Adding a tab is a one-place edit.
//
// 单一事实源。顺序即渲染顺序。新增 tab 只改这里。
const TESTEND_TABS = [
  { id: 'config',    label: 'Config' },
  { id: 'logs',      label: 'Logs' },
  { id: 'sql',       label: 'SQL' },
  { id: 'tests',     label: 'Tests' },
  { id: 'tools',     label: 'Tools' },
  { id: 'catalog',   label: 'Catalog' },
  { id: 'skill',     label: 'Skill' },
  { id: 'mcp',       label: 'MCP' },
  { id: 'sandbox',   label: 'Sandbox' },
  { id: 'mock-llm',  label: 'Mock LLM' },
  { id: 'wire',      label: 'Wire' },
  { id: 'processes', label: 'Processes' },
  { id: 'errors',    label: 'Errors' },
  { id: 'metrics',   label: 'Metrics' },
  { id: 'routes',    label: 'Routes' },
  { id: 'notifs',    label: 'Notifs' },
  { id: 'info',      label: 'Info' },
];

document.addEventListener('alpine:init', () => {
  Alpine.store('app', {
    conversationId: null,
    conversationTitle: '',
    // Mirror of appRoot.activeRightTab so per-tab setInterval polls
    // can guard `if (store.activeRightTab !== 'X') return` from any
    // tab's component scope. appRoot keeps it synced via $watch.
    //
    // 镜像 appRoot.activeRightTab 让 per-tab setInterval guard 能从任意
    // tab 组件读到当前 active tab。appRoot 的 $watch 负责同步。
    activeRightTab: 'config',
  });

  // notifBus — single shared EventSource for /api/v1/notifications.
  //
  // Why: HTTP/1.1 caps concurrent connections per origin at 6. Each
  // EventSource burns one for its lifetime, so previously chat.js
  // and tab-notifications.js each spawned their own /notifications
  // stream — 2 of the 6 slots gone before any fetch. With this bus
  // we open ONE stream lazily on first subscribe and fan it out to
  // all listeners.
  //
  // 单一共享 EventSource。HTTP/1.1 同 origin 最多 6 个并发连接，
  // 之前 chat.js 和 tab-notifications.js 各自开一条 /notifications
  // 占掉 2 个 slot；现在合并为 1 个连接懒启动 + 多 listener 复用。
  Alpine.store('notifBus', {
    _es: null,
    listeners: new Set(),
    connState: 'closed',

    _open() {
      if (this._es) return;
      this._es = new EventSource('/api/v1/notifications');
      this.connState = 'live';
      this._es.addEventListener('notification', e => {
        let n; try { n = JSON.parse(e.data); } catch { return; }
        if (!n || !n.type) return;
        for (const fn of this.listeners) {
          try { fn(n, e.lastEventId); } catch (err) { console.error('notifBus listener', err); }
        }
      });
      this._es.onopen = () => { this.connState = 'live'; };
      this._es.onerror = () => { this.connState = 'error'; };
    },

    // Subscribe registers fn as a listener and returns an unsubscribe.
    // Bus stays open for the lifetime of the page once any tab subscribes
    // — closing it on last unsubscribe risks rapid open/close churn when
    // tabs swap.
    //
    // 订阅 fn 返 unsubscribe。一旦有 tab 订阅，bus 不再关——避免 tab 切换
    // 引发的频繁开关连接抖动。
    subscribe(fn) {
      this._open();
      this.listeners.add(fn);
      return () => { this.listeners.delete(fn); };
    },
  });

  // logBus — same pattern for /dev/logs (consumed by tab-logs and
  // tab-errors). The backend sends named SSE events ("event: log\n"),
  // not unnamed messages — so we addEventListener('log', ...) rather
  // than onmessage. Listener receives the parsed entry object.
  //
  // logBus 同模式：tab-logs / tab-errors 共享一条 /dev/logs。后端发
  // "event: log\n" 命名事件而非匿名 message——用 addEventListener('log')。
  // listener 收到已解析的 entry 对象。
  Alpine.store('logBus', {
    _es: null,
    listeners: new Set(),
    connState: 'closed',

    _open() {
      if (this._es) return;
      this._es = new EventSource('/dev/logs');
      this.connState = 'live';
      this._es.addEventListener('log', e => {
        let entry; try { entry = JSON.parse(e.data); } catch { return; }
        for (const fn of this.listeners) {
          try { fn(entry); } catch (err) { console.error('logBus listener', err); }
        }
      });
      this._es.onopen = () => { this.connState = 'live'; };
      this._es.onerror = () => { this.connState = 'error'; };
    },

    subscribe(fn) {
      this._open();
      this.listeners.add(fn);
      return () => { this.listeners.delete(fn); };
    },
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

    init() {
      // Seed + sync the store mirror used by polling guards.
      // 同步给 polling guard 用的 store 镜像。
      Alpine.store('app').activeRightTab = this.activeRightTab;
      this.$watch('activeRightTab', v => {
        Alpine.store('app').activeRightTab = v;
      });
    },

    selectTab(tab) {
      this.activeRightTab = tab;
    },
  };
}
