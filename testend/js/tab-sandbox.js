// tab-sandbox.js — Sandbox v2 (mise) inventory + admin (D2). Mostly
// read-only — runtimes get installed lazily by service code at first
// use. Two actions: GC (drop unreferenced envs) and retry-bootstrap
// (after a degraded-mode boot the user can fix their setup + retry).

document.addEventListener('alpine:init', () => {
  Alpine.data('sandboxTab', () => ({
    bootstrap: null,            // {ready, error, miseBin, ...}
    runtimes: [],
    envs: [],
    convEnvs: [],
    disk: null,                 // {totalBytes, runtimeBytes, envBytes}
    loading: false,
    actionBusy: '',
    err: '',

    get conversationId() { return Alpine.store('app').conversationId },

    async init() {
      await this.loadAll()
      this.$watch('conversationId', () => this.loadConvEnvs())
    },

    async loadAll() {
      this.loading = true
      this.err = ''
      try {
        const [bs, rt, ev, du] = await Promise.all([
          fetch('/api/v1/sandbox/bootstrap-status').then(r => r.json()).catch(() => null),
          fetch('/api/v1/sandbox/runtimes').then(r => r.json()).catch(() => null),
          fetch('/api/v1/sandbox/envs').then(r => r.json()).catch(() => null),
          fetch('/api/v1/sandbox/disk-usage').then(r => r.json()).catch(() => null),
        ])
        this.bootstrap = bs?.data || null
        this.runtimes = (rt?.data || []).sort((a, b) => a.kind.localeCompare(b.kind))
        this.envs = (ev?.data || []).sort((a, b) => (b.createdAt || '').localeCompare(a.createdAt || ''))
        this.disk = du?.data || null
        await this.loadConvEnvs()
      } catch (e) {
        this.err = String(e)
      } finally {
        this.loading = false
      }
    },

    async loadConvEnvs() {
      if (!this.conversationId) {
        this.convEnvs = []
        return
      }
      try {
        const r = await fetch(`/api/v1/conversations/${this.conversationId}/sandbox-envs`)
        if (!r.ok) return
        const j = await r.json()
        this.convEnvs = j.data || []
      } catch {}
    },

    async destroyEnv(env) {
      if (!confirm(`Destroy env ${env.id}? Removes the venv directory + frees disk.`)) return
      this.actionBusy = env.id
      this.err = ''
      try {
        const r = await fetch(`/api/v1/sandbox/envs/${encodeURIComponent(env.id)}:destroy`, { method: 'POST' })
        if (!r.ok) {
          this.err = `destroy failed HTTP ${r.status}`
          return
        }
        await this.loadAll()
      } finally {
        this.actionBusy = ''
      }
    },

    async gc() {
      if (!confirm('Run sandbox GC? Drops envs not referenced by any forge/skill/mcp.')) return
      this.actionBusy = 'gc'
      this.err = ''
      try {
        const r = await fetch('/api/v1/sandbox:gc', { method: 'POST' })
        if (!r.ok) {
          this.err = `gc failed HTTP ${r.status}`
          return
        }
        const j = await r.json()
        this.err = `gc ok: removed ${j.data?.removed ?? 0} envs, freed ${this.fmtBytes(j.data?.freedBytes ?? 0)}`
        await this.loadAll()
      } finally {
        this.actionBusy = ''
      }
    },

    async retryBootstrap() {
      this.actionBusy = 'bootstrap'
      this.err = ''
      try {
        const r = await fetch('/api/v1/sandbox:retry-bootstrap', { method: 'POST' })
        if (!r.ok) {
          this.err = `retry failed HTTP ${r.status}`
          return
        }
        await this.loadAll()
      } finally {
        this.actionBusy = ''
      }
    },

    fmtBytes(n) {
      if (!n) return '0 B'
      const units = ['B', 'KB', 'MB', 'GB', 'TB']
      let i = 0
      while (n >= 1024 && i < units.length - 1) { n /= 1024; i++ }
      return n.toFixed(i === 0 ? 0 : 1) + ' ' + units[i]
    },

    fmtTime(s) {
      if (!s) return '—'
      try { return new Date(s).toLocaleTimeString() } catch { return s }
    },
  }))
})
