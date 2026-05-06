// sidebar.js — conversation list panel.

document.addEventListener('alpine:init', () => {
  Alpine.data('sidebar', () => ({
    conversations: [],
    loading: false,

    get selected() { return Alpine.store('app').conversationId },

    async init() {
      await this.load()
      // Auto-select the most recent conversation so the input is immediately usable.
      if (!Alpine.store('app').conversationId && this.conversations.length > 0) {
        const c = this.conversations[0]
        this.select(c.id, c.title)
      }
      // Refresh list periodically to catch auto-title updates.
      setInterval(() => this.load(), 8000)
      // Re-load when a new conversation is created (e.g., from chat send or title update).
      document.addEventListener('conv-created', () => this.load())
      // TE-6: re-load after the chat tab deletes the current conv.
      // TE-6：chat tab 删当前对话后刷新。
      document.addEventListener('conv-deleted', () => this.load())
    },

    async load() {
      const r = await fetch('/api/v1/conversations?limit=50')
      if (r.ok) {
        const j = await r.json()
        this.conversations = j.data || []
      }
    },

    async create() {
      const r = await fetch('/api/v1/conversations', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: '' }),
      })
      if (r.ok) {
        const j = await r.json()
        await this.load()
        this.select(j.data.id, j.data.title)
      }
    },

    select(id, title) {
      Alpine.store('app').conversationId = id
      Alpine.store('app').conversationTitle = title || '(untitled)'
    },

    formatTime(ts) {
      if (!ts) return ''
      const d = new Date(ts)
      const now = new Date()
      if (d.toDateString() === now.toDateString()) {
        return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
      }
      return d.toLocaleDateString([], { month: 'short', day: 'numeric' })
    },
  }))
})
