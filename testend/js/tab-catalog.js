// tab-catalog.js — Capability Catalog viewer (D8). Shows the cached
// catalog the chat runner injects into every system prompt: Summary
// text, per-source Coverage IDs, fingerprint + version, when each
// source was last polled, and which path generated the Summary
// (LLM vs mechanical-fallback). Force-refresh button calls
// POST /api/v1/catalog:refresh which bypasses the 1s polling cadence.

document.addEventListener('alpine:init', () => {
  Alpine.data('catalogTab', () => ({
    catalog: null,
    loading: false,
    refreshing: false,
    err: '',

    async init() {
      await this.load()
    },

    async load() {
      this.loading = true
      this.err = ''
      try {
        const r = await fetch('/api/v1/catalog')
        if (!r.ok) {
          this.err = `HTTP ${r.status}`
          return
        }
        const j = await r.json()
        this.catalog = j.data
      } catch (e) {
        this.err = String(e)
      } finally {
        this.loading = false
      }
    },

    async refresh() {
      this.refreshing = true
      this.err = ''
      try {
        const r = await fetch('/api/v1/catalog:refresh', { method: 'POST' })
        if (!r.ok) {
          this.err = `HTTP ${r.status}`
          return
        }
        const j = await r.json()
        this.catalog = j.data
      } catch (e) {
        this.err = String(e)
      } finally {
        this.refreshing = false
      }
    },

    // Per-source Coverage entries. Returns [{source, ids: [...]}] sorted by source.
    coverageRows() {
      if (!this.catalog || !this.catalog.coverage) return []
      return Object.entries(this.catalog.coverage)
        .sort(([a], [b]) => a < b ? -1 : 1)
        .map(([source, ids]) => ({ source, ids: ids || [] }))
    },

    // Per-source last-polled timestamps. Returns [{source, at}] sorted.
    sourceTimestamps() {
      if (!this.catalog || !this.catalog.sourcesAt) return []
      return Object.entries(this.catalog.sourcesAt)
        .sort(([a], [b]) => a < b ? -1 : 1)
        .map(([source, at]) => ({ source, at }))
    },

    fmtTime(s) {
      if (!s) return '—'
      try {
        const d = new Date(s)
        return d.toLocaleTimeString() + ' (' + d.toLocaleDateString() + ')'
      } catch { return s }
    },

    fpShort(fp) {
      if (!fp) return '—'
      return fp.length > 12 ? fp.slice(0, 12) + '…' : fp
    },
  }))
})
