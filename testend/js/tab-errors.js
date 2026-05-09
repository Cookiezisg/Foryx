// tab-errors.js — error/warn-only view of the backend log stream. Subscribes
// to the same /dev/logs SSE as the Logs tab but pre-filters to ERROR (and
// optionally WARN+) level so testers can spot regressions without scrolling
// past INFO/DEBUG noise. Independent connection so the Logs tab's autoScroll
// + general filter stay decoupled.
//
// tab-errors.js — 后端 log 流的 ERROR/WARN-only 视图。订阅同一 /dev/logs SSE，
// 但前端预过滤到 ERROR（或 WARN+）让测试者无需翻 INFO/DEBUG 噪音找退步。
// 独立连接：跟 Logs tab 的 autoScroll + 通用 filter 解耦。

document.addEventListener('alpine:init', () => {
  Alpine.data('errorsTab', () => ({
    entries: [],
    levelFilter: 'all', // 'all' | 'error' | 'warn+'
    textFilter: '',
    connected: false,
    _unsub: null,

    init() {
      this._connect();
    },

    _connect() {
      // Subscribe to the shared logBus rather than opening a second
      // /dev/logs EventSource — tab-logs already keeps the bus open.
      // 订共享 logBus 而不是开第二条 /dev/logs——tab-logs 已让 bus 保持开启。
      const bus = Alpine.store('logBus');
      this.connected = bus.connState === 'live';
      this.$watch('$store.logBus.connState', v => { this.connected = v === 'live'; });
      this._unsub = bus.subscribe(entry => {
        // Server-side log entries don't always carry a level for raw stdout
        // captures; default to 'info' so the row at least renders. Only
        // ERROR/WARN/INFO/DEBUG are styled — anything else falls through to
        // the .errors-row--debug muted style.
        // 兼容无 level 的条目，默认 info；未识别 level 走 muted 样式。
        const level = (entry.level || 'info').toLowerCase();
        const msg = entry.msg || entry.message || JSON.stringify(entry);
        // Pre-filter at SSE ingest to keep memory tight: keep only WARN+
        // levels in this tab regardless of UI filter (Logs tab has the full
        // firehose). 'fatal' / 'panic' included.
        // 入口预筛只留 WARN+，省内存（Logs tab 才是完整流）。
        if (!['warn', 'error', 'fatal', 'panic'].includes(level)) return;
        const fields = this._fieldsStr(entry);
        this.entries.unshift({
          ts: this._shortTime(entry.time),
          level: this._normalizeLevel(level),
          msg,
          fields,
        });
        if (this.entries.length > 500) this.entries.length = 500;
      });
    },

    destroy() {
      if (this._unsub) { this._unsub(); this._unsub = null; }
    },

    get filteredEntries() {
      const q = this.textFilter.trim().toLowerCase();
      return this.entries.filter(e => {
        if (this.levelFilter === 'error' && e.level !== 'error') return false;
        if (this.levelFilter === 'warn+' && !['warn', 'error'].includes(e.level)) return false;
        if (q && !(e.msg + ' ' + e.fields).toLowerCase().includes(q)) return false;
        return true;
      });
    },

    _normalizeLevel(l) {
      // Squash fatal/panic into 'error' for styling purposes; the original
      // text is preserved in entry.msg if needed.
      // fatal/panic 折成 'error' 给样式用；原文在 msg 里保留。
      if (l === 'fatal' || l === 'panic') return 'error';
      return l;
    },

    _shortTime(iso) {
      if (!iso) return '—';
      return String(iso).slice(11, 19);
    },

    _fieldsStr(entry) {
      // Drop noisy fields the Logs tab's filter would already cover.
      // 跳掉噪音字段。
      const skip = new Set(['level', 'msg', 'message', 'time', 'ts', 'caller', 'logger']);
      const parts = [];
      for (const [k, v] of Object.entries(entry)) {
        if (skip.has(k)) continue;
        if (v == null) continue;
        const s = typeof v === 'object' ? JSON.stringify(v) : String(v);
        parts.push(`${k}=${s}`);
      }
      return parts.join(' · ');
    },
  }));
});
