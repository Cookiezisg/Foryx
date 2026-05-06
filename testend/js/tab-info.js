// tab-info.js — Backend metadata + ~/.forgify tree (TE-9). One-stop
// 'where is everything' answer for testers: backend version,
// uptime, listening port, the absolute paths each subsystem uses,
// and the contents of ~/.forgify/ (the only filesystem-state
// directory the app touches outside of its data dir).

document.addEventListener('alpine:init', () => {
  Alpine.data('infoTab', () => ({
    info: null,
    home: null,
    loading: false,
    err: '',
    expanded: {},

    async init() {
      await this.load()
      // Poll uptime every 5s so it ticks live without spamming.
      this._poll = setInterval(() => this.refreshUptime(), 5000)
    },

    destroy() {
      if (this._poll) clearInterval(this._poll)
    },

    async load() {
      this.loading = true
      this.err = ''
      try {
        const [iRes, hRes] = await Promise.all([
          fetch('/dev/info'),
          fetch('/dev/forgify-home'),
        ])
        if (iRes.ok) {
          const j = await iRes.json()
          this.info = j.data
        } else {
          this.err = `info HTTP ${iRes.status}`
        }
        if (hRes.ok) {
          const j = await hRes.json()
          this.home = j.data
        }
      } catch (e) {
        this.err = String(e)
      } finally {
        this.loading = false
      }
    },

    async refreshUptime() {
      try {
        const r = await fetch('/dev/info')
        if (r.ok) {
          const j = await r.json()
          if (this.info) this.info.uptimeSeconds = j.data?.uptimeSeconds ?? this.info.uptimeSeconds
        }
      } catch {}
    },

    fmtUptime(s) {
      if (s == null) return '—'
      const d = Math.floor(s / 86400)
      const h = Math.floor((s % 86400) / 3600)
      const m = Math.floor((s % 3600) / 60)
      const sec = s % 60
      const parts = []
      if (d) parts.push(d + 'd')
      if (h) parts.push(h + 'h')
      if (m) parts.push(m + 'm')
      parts.push(sec + 's')
      return parts.join(' ')
    },

    fmtBytes(n) {
      if (!n) return '0 B'
      const u = ['B', 'KB', 'MB', 'GB']
      let i = 0
      while (n >= 1024 && i < u.length - 1) { n /= 1024; i++ }
      return n.toFixed(i === 0 ? 0 : 1) + ' ' + u[i]
    },

    fmtTime(s) {
      if (!s) return '—'
      try { return new Date(s).toLocaleString() } catch { return s }
    },

    toggle(path) {
      this.expanded[path] = !this.expanded[path]
    },

    // Flatten the home tree into a depth-tagged list for easy rendering.
    // Folders that aren't expanded skip their children. Returns
    // [{entry, depth}, ...]. Avoids needing recursive Alpine templates
    // (which work but Alpine 3's `<template x-for>` recursion is fragile).
    //
    // 把 home tree 扁平化为带深度的列表方便渲染。未展开的文件夹跳子项。
    // 避免递归 Alpine 模板（Alpine 3 的 template-x-for 递归脆弱）。
    flatHome() {
      if (!this.home || !this.home.entries) return []
      const out = []
      const walk = (entries, depth) => {
        for (const e of entries) {
          out.push({ entry: e, depth })
          if (e.isDir && this.expanded[e.path] && e.children?.length) {
            walk(e.children, depth + 1)
          }
        }
      }
      walk(this.home.entries, 0)
      return out
    },
  }))
})
