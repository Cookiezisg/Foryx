// chat.js — center chat panel: message list, streaming, send/cancel, attachments.
// Phase 6 entity-state SSE model: each `chat.message` event = full Message
// snapshot. Subscribers find by id and replace; no per-token / delta logic.
//
// Block model conversion:
//   user blocks      → [{type:'text'|'attachment', ...}]
//   assistant blocks → items[] with reasoning/tool/text entries (tool_call +
//                      tool_result paired by toolCallId)

document.addEventListener('alpine:init', () => {
  Alpine.data('chatPanel', () => ({
    messages: [],
    input: '',
    streaming: false,
    pendingAtts: [],      // [{id, fileName, mimeType}]
    uploading: false,
    _es: null,

    // Raw-snapshot modal: clicking [📋 raw] on a message stashes the
    // wire payload here + shows the modal. JSON-stringified for display
    // + lazy 'copied' flag for the clipboard-button feedback.
    //
    // Raw-snapshot modal：点消息 [📋 raw] 把 wire 载荷存这里 + 显示 modal。
    // JSON-stringified 给显示用 + lazy 'copied' 标志给剪贴板按钮反馈。
    rawModal: { open: false, json: '', messageId: '', copied: false },

    // Catalog injection indicator: shows a small pill in the header
    // confirming the LLM is being told about available capabilities
    // (catalog block in system prompt). Updates on conv switch +
    // refreshes alongside catalog tab interactions.
    //
    // Catalog 注入指示器：header 小 pill 确认 LLM 收到了能力清单
    // （system prompt 里的 catalog 块）。切换对话 + catalog tab 操作时
    // 跟随刷新。
    catalogFp: '',
    catalogGenerated: '',

    showRaw(m) {
      this.rawModal = {
        open: true,
        json: JSON.stringify(m.raw || m, null, 2),
        messageId: m.id,
        copied: false,
      }
    },

    closeRaw() { this.rawModal.open = false },

    async copyRaw() {
      try {
        await navigator.clipboard.writeText(this.rawModal.json)
        this.rawModal.copied = true
        setTimeout(() => { this.rawModal.copied = false }, 1500)
      } catch {
        // navigator.clipboard requires HTTPS or localhost; testend is
        // localhost so this should work, but fall back to selection
        // if it doesn't.
        // navigator.clipboard 要 HTTPS 或 localhost；testend localhost
        // 应工作，失败时退回选中文本让用户手动 Cmd+C。
        alert('Copy failed; select the JSON text + Cmd+C manually.')
      }
    },

    get conversationId() { return Alpine.store('app').conversationId },
    get title() { return Alpine.store('app').conversationTitle },

    init() {
      this.loadCatalogStatus()
      this.$watch('conversationId', id => {
        this._closeSSE()
        this.messages = []
        this.streaming = false
        this.pendingAtts = []
        if (id) {
          this.loadMessages(id).then(() => this._connectSSE(id))
        }
        this.loadCatalogStatus()
      })
    },

    async loadCatalogStatus() {
      try {
        const r = await fetch('/api/v1/catalog')
        if (!r.ok) { this.catalogFp = ''; return }
        const j = await r.json()
        const cat = j.data
        if (cat) {
          this.catalogFp = (cat.fingerprint || '').slice(0, 8)
          this.catalogGenerated = cat.generatedBy || ''
        } else {
          this.catalogFp = ''
        }
      } catch { this.catalogFp = '' }
    },

    async deleteCurrentConv() {
      const id = this.conversationId
      if (!id) return
      if (!confirm('Delete the current conversation? Removes all messages from the database.')) return
      try {
        await fetch(`/api/v1/conversations/${id}`, { method: 'DELETE' })
        // Sidebar polls; clear local state immediately so the UI is responsive.
        // Sidebar 会轮询；本地立即清让 UI 响应。
        Alpine.store('app').conversationId = ''
        Alpine.store('app').conversationTitle = ''
        this.messages = []
        document.dispatchEvent(new CustomEvent('conv-deleted'))
      } catch (e) {
        alert('delete failed: ' + e)
      }
    },

    // ── loadMessages ──────────────────────────────────────────────────────────

    async loadMessages(id) {
      const r = await fetch(`/api/v1/conversations/${id}/messages?limit=200`)
      if (!r.ok) return
      const j = await r.json()
      const raw = (j.data || []).filter(m => m.status !== 'pending')
      this.messages = raw.map(m => this._messageFromSnapshot(m))
    },

    // ── snapshot → display message ────────────────────────────────────────────

    _messageFromSnapshot(m) {
      return m.role === 'user'
        ? this._userMsgFromBlocks(m)
        : this._assistantMsgFromBlocks(m)
    },

    _userMsgFromBlocks(m) {
      const blocks = []
      for (const b of (m.blocks || [])) {
        try {
          const d = JSON.parse(b.data)
          if (b.type === 'text') {
            blocks.push({ type: 'text', content: d.text })
          } else if (b.type === 'attachment_ref') {
            blocks.push({ type: 'attachment', fileName: d.fileName, mimeType: d.mimeType, id: d.attachmentId })
          }
        } catch {}
      }
      // Keep raw snapshot stashed so the [📋 raw] button can show the
      // verbatim chat.message wire payload (full block data, status,
      // tokens, error fields, timestamps — everything).
      // 留 raw snapshot 让 [📋 raw] 按钮显示 chat.message 原始 wire 载荷
      // （完整 block data / status / tokens / 错误字段 / 时间戳——全部）。
      return { id: m.id, role: 'user', blocks, status: m.status, raw: m }
    },

    _assistantMsgFromBlocks(m) {
      const items = []
      const toolMap = {}  // toolCallId → item

      for (const b of (m.blocks || [])) {
        try {
          const d = JSON.parse(b.data)
          if (b.type === 'reasoning') {
            items.push({ type: 'reasoning', content: d.text, done: true })
          } else if (b.type === 'tool_call') {
            const item = {
              type: 'tool', toolCallId: d.id, toolName: d.name,
              summary: d.summary || '', destructive: d.destructive || false,
              executionGroup: d.executionGroup || 0,
              input: JSON.stringify(d.arguments || {}),
              result: null, ok: null, errorMsg: '', elapsedMs: 0,
            }
            items.push(item)
            toolMap[d.id] = item
          } else if (b.type === 'tool_result') {
            const item = toolMap[d.toolCallId]
            if (item) {
              item.result = d.result
              item.ok = d.ok
              item.errorMsg = d.errorMsg || ''
              item.elapsedMs = d.elapsedMs || 0
            }
          } else if (b.type === 'text') {
            if (d.text) items.push({ type: 'text', content: d.text })
          }
        } catch {}
      }

      // While streaming, the assistant message may end with an in-progress
      // tool_call whose tool_result hasn't arrived yet — that's fine, keep it.
      // Mark trailing reasoning as not-done if no text block follows it.
      // 流式途中 assistant 消息可能以未配对的 tool_call 结尾——保留即可。
      // 末尾 reasoning 若后面没 text block 还在生成，标 done=false。
      let lastReasoning = null
      for (const it of items) {
        if (it.type === 'reasoning') lastReasoning = it
        if (it.type === 'text') lastReasoning = null
      }
      if (lastReasoning && m.status === 'streaming') lastReasoning.done = false

      return {
        id: m.id, role: 'assistant', items, status: m.status,
        stopReason: m.stopReason || '',
        errorCode: m.errorCode || '',
        errorMessage: m.errorMessage || '',
        inputTokens: m.inputTokens || 0,
        outputTokens: m.outputTokens || 0,
        raw: m,  // see _userMsgFromBlocks comment
      }
    },

    // ── Attachment upload ─────────────────────────────────────────────────────

    pickFile() {
      this.$refs.fileInput.click()
    },

    async onFileChange(e) {
      const files = e.target.files
      if (!files || files.length === 0) return
      for (const file of files) {
        await this.uploadAttachment(file)
      }
      e.target.value = ''
    },

    async uploadAttachment(file) {
      this.uploading = true
      try {
        const fd = new FormData()
        fd.append('file', file)
        const r = await fetch('/api/v1/attachments', { method: 'POST', body: fd })
        if (!r.ok) { alert('Upload failed: ' + r.status); return }
        const j = await r.json()
        this.pendingAtts.push({ id: j.data.id, fileName: j.data.fileName, mimeType: j.data.mimeType })
      } finally {
        this.uploading = false
      }
    },

    removeAtt(idx) {
      this.pendingAtts.splice(idx, 1)
    },

    // ── send ──────────────────────────────────────────────────────────────────

    async send() {
      const content = this.input.trim()
      if ((!content && this.pendingAtts.length === 0) || this.streaming) return
      if (this.uploading) return

      let convId = this.conversationId
      if (!convId) {
        const rc = await fetch('/api/v1/conversations', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ title: '' }),
        })
        if (!rc.ok) return
        const jc = await rc.json()
        convId = jc.data.id
        Alpine.store('app').conversationId = convId
        Alpine.store('app').conversationTitle = ''
        document.dispatchEvent(new CustomEvent('conv-created'))
        await new Promise(r => setTimeout(r, 100))
      }

      const attIds = this.pendingAtts.map(a => a.id)
      const attsSnapshot = [...this.pendingAtts]
      this.input = ''
      this.pendingAtts = []

      const r = await fetch(`/api/v1/conversations/${convId}/messages`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ content, attachmentIds: attIds }),
      })
      if (!r.ok) return

      const j = await r.json()

      // Optimistic user message — backend's chat.message snapshot will
      // confirm later (id matches j.data.messageId).
      // 乐观插入用户消息——后端 chat.message 快照（id 与 j.data.messageId 同）会确认。
      const userBlocks = []
      if (content) userBlocks.push({ type: 'text', content })
      for (const a of attsSnapshot) userBlocks.push({ type: 'attachment', fileName: a.fileName, mimeType: a.mimeType, id: a.id })
      this.messages.push({ id: j.data.messageId, role: 'user', blocks: userBlocks, status: 'completed' })

      this.streaming = true
      this._scrollBottom()
    },

    async cancel() {
      const id = this.conversationId
      if (!id) return
      await fetch(`/api/v1/conversations/${id}/stream`, { method: 'DELETE' })
    },

    // ── SSE (Phase 6 entity-state model) ──────────────────────────────────────
    //
    // Three event types:
    //   chat.message  — full Message snapshot; replace by id, append if new.
    //   conversation  — full Conversation snapshot (title updates etc.).
    //   forge         — full Forge snapshot (consumed by tab-tools / tab-sse).

    _connectSSE(id) {
      this._closeSSE()
      const es = new EventSource(`/api/v1/events?conversationId=${id}`)
      this._es = es

      es.addEventListener('chat.message', e => {
        const m = JSON.parse(e.data)
        if (!m || !m.id) return
        const display = this._messageFromSnapshot(m)
        const idx = this.messages.findIndex(x => x.id === m.id)
        if (idx >= 0) {
          this.messages[idx] = display
        } else {
          this.messages.push(display)
        }
        if (m.role === 'assistant' && m.status !== 'streaming') {
          this.streaming = false
        }
        this._scrollBottom()
      })

      es.addEventListener('conversation', e => {
        const c = JSON.parse(e.data)
        if (!c) return
        Alpine.store('app').conversationTitle = c.title || ''
        document.dispatchEvent(new CustomEvent('conv-created'))
      })
    },

    _closeSSE() {
      if (this._es) { this._es.close(); this._es = null }
    },

    _scrollBottom() {
      this.$nextTick(() => {
        const el = this.$el.querySelector('.chat-messages')
        if (el) el.scrollTop = el.scrollHeight
      })
    },

    tryFmt(s) {
      if (s === null || s === undefined) return '…'
      try { return JSON.stringify(JSON.parse(s), null, 2) } catch { return s }
    },

    attIcon(mimeType) {
      if (!mimeType) return '📎'
      if (mimeType.startsWith('image/')) return '🖼'
      if (mimeType === 'application/pdf') return '📄'
      if (mimeType.includes('spreadsheet') || mimeType.includes('excel')) return '📊'
      return '📎'
    },

    handleKeydown(e) {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); this.send() }
    },
  }))
})
