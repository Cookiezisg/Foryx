// tab-notifications.js — unified notification feed (M4 of testend rework).
//
// Single time-ordered ring of every entity-state notification the
// backend pushes via /api/v1/notifications SSE PLUS every toast fired
// locally via window.toast.X(). Pre-M4 this tab only mirrored toasts
// — backend SSE events (mcp_server status / catalog regenerate / skill
// rescan / todo CRUD / conversation rename) were silently dropped
// unless some other tab proactively echoed them as a toast. Now both
// sources land here as one feed; per-source + per-type chip filters
// let the dev focus.
//
// Manual test toast triggers (success/error/warn/info/sticky) are kept
// from the prior version — handy for verifying the toast renderer
// without waiting for the app to fire one.
//
// tab-notifications.js — 统一通知 feed。后端 /api/v1/notifications SSE
// 推的 entity 事件 + 本地 window.toast.X() 喊的 toast 全收进同一时间线。
// 之前只镜像 toast，后端 SSE 静默丢；现在两源合一 + source/type 过滤。
// 手动测试 toast 触发按钮保留——不等真事件就能验渲染。

document.addEventListener('alpine:init', () => {
  Alpine.data('notificationsTab', () => ({
    // Unified feed entries: { id, source: 'sse'|'toast', type, ts,
    // summary, data, seq?: string, conversationId?: string }
    // Newest-last (push at end). Cap = 200; oldest dropped.
    //
    // 统一 feed 条目结构。新事件 push 到末尾；上限 200，drop 旧。
    events: [],

    // Filters. sourceFilter ∈ {'all','sse','toast'}; typeFilters is a
    // Set of type strings (empty = all types).
    //
    // 过滤。source ∈ {all/sse/toast}；typeFilters 为空 = 全部 type。
    sourceFilter: 'all',
    typeFilters: new Set(),

    // notifBus subscription disposer; bus connection state mirrored
    // into our local connState so the existing UI indicator stays.
    //
    // notifBus 订阅 disposer；bus 的 connState 镜像到本地保持 UI 指示器不变。
    _unsubNotif: null,
    connState: 'closed',

    // Type vocabularies (for filter chip menus).
    //
    // Type 词表（filter 芯片菜单用）。
    SSE_TYPES: ['conversation', 'todo', 'mcp_server', 'skill', 'catalog', 'sandbox_env'],
    TOAST_TYPES: ['success', 'error', 'warn', 'info'],
    MAX_EVENTS: 200,

    init() {
      this._connectSSE()
      // Mirror toasts: any new id we haven't seen lands as a feed entry.
      // Toasts dismissed don't get a feed entry update — the feed
      // records the moment of firing, not the dismissal.
      //
      // 镜像 toast：未见过的 id 入 feed。dismiss 不更新 feed——记录的是
      // 触发时刻，不是消失。
      this.$watch('$store.toasts.list', list => {
        for (const t of list) {
          const key = 'toast:' + t.id
          if (!this.events.some(e => e.id === key)) {
            this.events.push({
              id: key,
              source: 'toast',
              type: t.type,
              ts: Date.now(),
              summary: t.text,
              data: { text: t.text, sticky: !!t.sticky },
            })
            this._trim()
          }
        }
      })
    },

    _connectSSE() {
      // Subscribe to the shared notifBus rather than open our own
      // EventSource. The bus exposes connState as a string; we mirror
      // it into local state via a tiny watcher (Alpine $watch on a
      // store path works inline).
      //
      // 订共享 notifBus 而不是自己 new EventSource。bus 暴露 connState，
      // 用 $watch 镜像到本地。
      const bus = Alpine.store('notifBus')
      this.connState = bus.connState
      this._unsubNotif = bus.subscribe((n, lastEventId) => {
        this.events.push({
          id: 'sse:' + (lastEventId || (Date.now() + ':' + Math.random())),
          source: 'sse',
          type: n.type,
          ts: Date.now(),
          summary: this._summarizeSSE(n),
          data: n,
          seq: lastEventId || '',
          conversationId: n.conversationId || '',
        })
        this._trim()
      })
      this.$watch('$store.notifBus.connState', v => { this.connState = v })
    },

    _trim() {
      if (this.events.length > this.MAX_EVENTS) {
        this.events.splice(0, this.events.length - this.MAX_EVENTS)
      }
    },

    // _summarizeSSE produces a one-line "key fields" preview per type.
    // Falls back to id + first 60 chars of data JSON when type unknown.
    //
    // _summarizeSSE 给每种 type 提取一行关键字段；type 未知则 fall back。
    _summarizeSSE(n) {
      const d = n.data || {}
      switch (n.type) {
        case 'conversation':
          return (d.title || '(untitled)') + ' · ' + (n.id || '')
        case 'todo': {
          const text = (d.text || d.title || '').slice(0, 60)
          return text + ' [' + (d.status || '?') + ']'
        }
        case 'mcp_server':
          if (d.deleted) return n.id + ' · deleted'
          return n.id + ' · ' + (d.status || '?') +
            (d.lastError ? ' (' + String(d.lastError).slice(0, 40) + ')' : '')
        case 'skill':
          // Skill scan summary varies — dump first 60 chars of data.
          // Skill scan 摘要因实现而异——dump 前 60 字符。
          return JSON.stringify(d).slice(0, 60)
        case 'catalog':
          return 'fp=' + (d.fingerprint || n.id || '').slice(0, 8) +
            ' v' + (d.version != null ? d.version : '?')
        case 'sandbox_env':
          if (d.deleted) return n.id + ' · destroyed'
          return (d.ownerKind || '?') + '/' + (d.ownerId || '?') +
            ' · ' + (d.runtimeKind || '?') +
            ' · ' + (d.status || '?') +
            (d.errorMsg ? ' (' + String(d.errorMsg).slice(0, 40) + ')' : '')
        default:
          return n.id || JSON.stringify(d).slice(0, 60)
      }
    },

    // ── Filter controls ────────────────────────────────────────────────

    setSourceFilter(s) {
      if (s === this.sourceFilter) return
      this.sourceFilter = s
      // Type vocab differs across sources — reset chips so a user-toggled
      // 'success' chip from toasts doesn't leak into sse view.
      // 不同 source 的 type 词表不同——切换时清 chips 防 'success' 漏到 sse 视图。
      this.typeFilters = new Set()
    },

    toggleTypeFilter(t) {
      if (this.typeFilters.has(t)) this.typeFilters.delete(t)
      else this.typeFilters.add(t)
      // Force Alpine re-render — Set mutations don't fire reactivity.
      // 强 Alpine 重渲——Set 改动不触发响应式。
      this.typeFilters = new Set(this.typeFilters)
    },

    knownTypes() {
      if (this.sourceFilter === 'sse') return this.SSE_TYPES
      if (this.sourceFilter === 'toast') return this.TOAST_TYPES
      return [...this.SSE_TYPES, ...this.TOAST_TYPES]
    },

    filteredEvents() {
      let out = this.events
      if (this.sourceFilter !== 'all') {
        out = out.filter(e => e.source === this.sourceFilter)
      }
      if (this.typeFilters.size > 0) {
        out = out.filter(e => this.typeFilters.has(e.type))
      }
      return out
    },

    countSSE() { return this.events.filter(e => e.source === 'sse').length },
    countToast() { return this.events.filter(e => e.source === 'toast').length },

    // ── Per-row helpers ────────────────────────────────────────────────

    sourceIcon(s) { return s === 'sse' ? '🔔' : '💬' },

    typeColor(type) {
      const map = {
        // SSE
        conversation: '#3267d2',
        todo: '#7c3aed',
        mcp_server: '#c97600',
        skill: '#0891b2',
        catalog: '#0891b2',
        sandbox_env: '#2a9d3a',
        // toast
        success: '#2a9d3a',
        error: '#c93434',
        warn: '#c97600',
        info: '#3267d2',
      }
      return map[type] || '#666'
    },

    fmtElapsed(ts) {
      const sec = Math.floor((Date.now() - ts) / 1000)
      if (sec < 1) return 'just now'
      if (sec < 60) return sec + 's ago'
      const m = Math.floor(sec / 60)
      if (m < 60) return m + 'm ago'
      const h = Math.floor(m / 60)
      return h + 'h ago'
    },

    pretty(p) {
      try { return JSON.stringify(p, null, 2) } catch { return String(p) }
    },

    // ── Manual test triggers (kept from prior version) ─────────────────

    fire(type) {
      const samples = {
        success: ['Saved', 'Created', 'Operation completed'],
        error: ['Save failed: connection refused', 'Server returned 500', 'Network error'],
        warn: ['Cache out of date', 'Deprecation: this endpoint will be removed', 'Unusual response shape'],
        info: ['Backend restart in 5s', 'Sync in progress', 'Reconnecting…'],
      }
      const arr = samples[type] || ['Hello']
      window.toast[type](arr[Math.floor(Math.random() * arr.length)])
    },

    fireSticky() {
      window.toast.info('Sticky notification (manual dismiss only)', { sticky: true })
    },

    clearActive() {
      window.Alpine.store('toasts').clear()
    },

    clearHistory() {
      this.events = []
    },
  }))
})
