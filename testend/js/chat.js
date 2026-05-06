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

    // Per-block expansion state (TE-16): keyed by block.id (reasoning) or
    // tool callId (tool steps). Default-folded keeps DOM size constant
    // regardless of how long a reasoning / tool args / tool result is —
    // the user opts in to render large bodies. Survives streaming updates
    // because keys are stable across snapshots.
    //
    // 按 block 的展开状态（TE-16）：reasoning 用 block.id，tool 用 callId
    // 作 key。默认折叠让 DOM 大小恒定，用户主动展开。stream 更新不会重置
    // （key 跨 snapshot 稳定）。
    expanded: {},

    toggleExpand(key) {
      this.expanded[key] = !this.expanded[key]
    },

    // SSE rAF batch (TE-16): coalesce burst chat.message snapshots into
    // one DOM render per frame. Without this, even with backend throttle
    // every event reaches Alpine reactive immediately → N renders per
    // burst → DOM thrashing on long messages. With this, multiple events
    // in the same frame collapse to a single Alpine update; same-id
    // snapshots, only the latest survives (intermediate frames discarded
    // — the user couldn't have perceived them anyway).
    //
    // SSE rAF 批处理（TE-16）：把同一帧内的多个 chat.message 快照合并成
    // 一次 DOM 渲染。无此机制时即使后端节流，每个 event 仍即时触发 Alpine
    // 反应式，长消息上 DOM 抖动严重。同 id 后到的覆盖前到的（用户看不到
    // 中间帧）。
    _pendingMsgs: new Map(),
    _pendingShouldScroll: false,
    _rafToken: 0,
    // Smart scroll (TE-18): only auto-stick to bottom when the user is
    // already there. If they've scrolled up to read history, new messages
    // bump newMsgCount but don't yank the viewport — instead a floating
    // pill ("↓ N new") appears at the bottom-center of the chat.
    //
    // 智能滚动：仅当用户已在底部时新消息自动滚底。用户已上滚阅读历史时，
    // 新消息累加 newMsgCount + 浮 pill 提示，不抢走视口。
    _userScrolledUp: false,
    newMsgCount: 0,
    // _suppressFlush guards the send() critical section: while a send is
    // in-flight the SSE handler still buffers snapshots into _pendingMsgs
    // but we don't drain them, so the optimistic user-row (tempId) gets
    // its real messageId stamped FIRST. Otherwise the rAF could fire
    // between fetch-out and fetch-in, push the assistant snapshot before
    // the user row exists, and the optimistic user push lands at the end
    // → "AI on top, user on bottom" bug.
    //
    // _suppressFlush 在 send() 临界区为真：SSE 仍写 _pendingMsgs 但不
    // 抽干，让乐观 user 行（tempId）拿到真 ID 后再处理 SSE。否则 rAF
    // 可能在 fetch 一来一回之间 push 了 assistant，乐观 user 落到尾巴
    // → "AI 在上 user 在下"。
    _suppressFlush: false,

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

    // Per-conversation system prompt editor (TE-13). Loads on conv switch
    // via GET /api/v1/conversations/{id}; edits saved via PATCH. Empty
    // string disables the override (chat layer falls back to defaults).
    //
    // 按对话的 system prompt 编辑器（TE-13）。切换对话时 GET 加载；
    // 编辑后 PATCH 保存。空字符串关闭覆盖（chat 层走默认）。
    systemPrompt: '',
    systemPromptDraft: '',
    systemPromptOpen: false,
    systemPromptSaving: false,
    systemPromptSavedAt: 0,

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
        toast.warn('Copy failed — select the JSON and Cmd+C manually.')
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
        this.systemPrompt = ''
        this.systemPromptDraft = ''
        if (id) {
          this.loadMessages(id).then(() => this._connectSSE(id))
          this.loadConvMeta(id)
        }
        this.loadCatalogStatus()
      })
      // ESC closes the raw-snapshot modal. Wire here once instead of once
      // per modal instance — single global keydown listener is cheap and
      // matches how every other modal (toasts, confirms, etc.) on the
      // platform handles ESC.
      // ESC 关 raw modal。全局监听一次即可，比按 modal 实例挂监听更轻。
      document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && this.rawModal.open) {
          this.closeRaw();
        }
      });
    },

    async loadConvMeta(id) {
      try {
        const r = await fetch(`/api/v1/conversations/${id}`)
        if (!r.ok) return
        const j = await r.json()
        this.systemPrompt = (j.data?.systemPrompt) || ''
        this.systemPromptDraft = this.systemPrompt
      } catch {}
    },

    async saveSystemPrompt() {
      const id = this.conversationId
      if (!id) return
      this.systemPromptSaving = true
      try {
        const r = await fetch(`/api/v1/conversations/${id}`, {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ systemPrompt: this.systemPromptDraft }),
        })
        if (!r.ok) {
          const j = await r.json().catch(() => ({}))
          toast.error('Save failed: ' + (j.error?.message || r.status))
          return
        }
        const j = await r.json()
        this.systemPrompt = (j.data?.systemPrompt) || ''
        this.systemPromptDraft = this.systemPrompt
        this.systemPromptSavedAt = Date.now()
      } catch (e) {
        toast.error('Save failed: ' + e)
      } finally {
        this.systemPromptSaving = false
      }
    },

    revertSystemPrompt() { this.systemPromptDraft = this.systemPrompt },
    clearSystemPrompt() { this.systemPromptDraft = '' },

    get systemPromptDirty() { return this.systemPromptDraft !== this.systemPrompt },
    get systemPromptJustSaved() { return Date.now() - this.systemPromptSavedAt < 2000 },

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
        toast.error('Delete failed: ' + e)
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
            // expandKey = block.id so per-block expand state survives streaming
            // updates (block.id is stable across snapshots).
            // expandKey = block.id 让按块展开状态跨流式更新存活。
            items.push({ type: 'reasoning', content: d.text, done: true, expandKey: 'r:' + b.id })
          } else if (b.type === 'tool_call') {
            const item = {
              type: 'tool', toolCallId: d.id, toolName: d.name,
              summary: d.summary || '', destructive: d.destructive || false,
              executionGroup: d.executionGroup || 0,
              input: JSON.stringify(d.arguments || {}),
              result: null, ok: null, errorMsg: '', elapsedMs: 0,
              expandKey: 't:' + d.id,
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
        if (!r.ok) { toast.error('Upload failed: HTTP ' + r.status); return }
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

      // Build user blocks now (we have content + attsSnapshot before fetch).
      // user blocks 提前组装——content + attsSnapshot fetch 前就有。
      const userBlocks = []
      if (content) userBlocks.push({ type: 'text', content })
      for (const a of attsSnapshot) userBlocks.push({ type: 'attachment', fileName: a.fileName, mimeType: a.mimeType, id: a.id })

      // (1) Optimistic-insert user row IMMEDIATELY with a tempId so the
      //     order is locked: user → (assistant later). Anchor for the SSE
      //     flush to merge against.
      // (1) 立刻乐观插入 user（tempId）——锁定顺序：user → 后续 assistant。
      const tempId = '__pending__' + Date.now() + '_' + Math.random().toString(36).slice(2, 8)
      this.messages.push({ id: tempId, role: 'user', blocks: userBlocks, status: 'sending' })
      this._scrollBottom()

      // (2) Suppress SSE flushes for the duration of the fetch. Any user/
      //     assistant snapshots that arrive will sit in _pendingMsgs until
      //     we've stamped the real messageId onto our tempId row.
      // (2) fetch 期间冻结 rAF flush，让 SSE 入 _pendingMsgs 但暂不 apply。
      this._suppressFlush = true

      let r
      try {
        r = await fetch(`/api/v1/conversations/${convId}/messages`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ content, attachmentIds: attIds }),
        })
      } catch (e) {
        // Network failure — drop the optimistic row + unfreeze SSE.
        // 网络失败——丢乐观行 + 解冻 SSE。
        this._dropPendingRow(tempId)
        this._suppressFlush = false
        this._flushPending()
        return
      }

      if (!r.ok) {
        this._dropPendingRow(tempId)
        this._suppressFlush = false
        this._flushPending()
        return
      }

      const j = await r.json()

      // (3) Stamp real id onto the optimistic row WITHOUT re-pushing —
      //     same array index, just id swap. Now SSE's user snapshot will
      //     find this row by id and replace it (no duplicate push).
      // (3) 把真 id 打到乐观行（不重 push，原位换 id）。SSE user snapshot
      //     按 id 找到此行 replace 即可，不会重复 push。
      const idx = this.messages.findIndex(m => m.id === tempId)
      if (idx >= 0) {
        this.messages[idx] = { ...this.messages[idx], id: j.data.messageId, status: 'completed' }
      }

      // (4) Unfreeze + flush any snapshots that piled up during fetch.
      //     The user row is now keyed by real id, so SSE's user snapshot
      //     replaces in-place; SSE's assistant snapshot pushes after.
      // (4) 解冻 + 抽干积压。user 行已是真 id，user snapshot 原位 replace；
      //     assistant snapshot push 在后。
      this._suppressFlush = false
      this._flushPending()

      this.streaming = true
      // Sending a new message = user is engaged + wants to see the
      // assistant reply. Reset any prior "scrolled up" state so the
      // upcoming stream auto-scrolls.
      // 发新消息 = 用户在场，想看回复——清旧 scroll-up 状态让新流自动滚。
      this.scrollBottomNow()
    },

    _dropPendingRow(tempId) {
      const idx = this.messages.findIndex(m => m.id === tempId)
      if (idx >= 0) this.messages.splice(idx, 1)
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
        // Stash latest snapshot per message id; same id overwrites — only
        // the freshest state ever reaches the DOM. Schedule a rAF flush
        // if not already pending.
        // 按 id 存最新快照（同 id 覆盖），rAF 排队后下一帧统一 apply。
        this._pendingMsgs.set(m.id, m)
        this._pendingShouldScroll = true
        this._scheduleFlush()
      })

      es.addEventListener('conversation', e => {
        const c = JSON.parse(e.data)
        if (!c) return
        Alpine.store('app').conversationTitle = c.title || ''
        document.dispatchEvent(new CustomEvent('conv-created'))
      })
    },

    _scheduleFlush() {
      if (this._rafToken) return
      // While send() is mid-flight, leave snapshots in _pendingMsgs
      // un-flushed. send() will call _flushPending() itself once it has
      // stamped the optimistic user-row with its real id. See
      // _suppressFlush comment + send() for why.
      // send() 进行中先不 flush，等乐观 user 行拿到真 id 后由 send() 调
      // _flushPending() 抽干。
      if (this._suppressFlush) return
      this._rafToken = requestAnimationFrame(() => {
        this._rafToken = 0
        this._flushPending()
      })
    },

    _flushPending() {
      if (this._pendingMsgs.size === 0) return
      for (const [id, snapshot] of this._pendingMsgs) {
        const display = this._messageFromSnapshot(snapshot)
        const idx = this.messages.findIndex(x => x.id === id)
        if (idx >= 0) {
          this.messages[idx] = display
        } else {
          this.messages.push(display)
        }
        if (snapshot.role === 'assistant' && snapshot.status !== 'streaming') {
          this.streaming = false
        }
      }
      const incoming = this._pendingMsgs.size
      this._pendingMsgs.clear()
      if (this._pendingShouldScroll) {
        this._pendingShouldScroll = false
        // Smart scroll: only stick to bottom when the user is already there.
        // Otherwise increment the new-message counter; the floating pill
        // shows + offers a one-click jump to bottom.
        // 智能滚动：用户在底部时跟，否则加计数 + 浮 pill 提示。
        if (this._userScrolledUp) {
          this.newMsgCount += incoming
        } else {
          this._scrollBottom()
        }
      }
    },

    _closeSSE() {
      if (this._es) { this._es.close(); this._es = null }
      // Cancel any pending rAF + drop unflushed snapshots — they belong to
      // the conversation we're leaving. The next conv's loadMessages()
      // will hydrate the canonical state.
      // 取消 rAF + 丢未 flush 快照（属于即将离开的对话；新对话 loadMessages
      // 会拿权威态）。
      if (this._rafToken) {
        cancelAnimationFrame(this._rafToken)
        this._rafToken = 0
      }
      this._pendingMsgs.clear()
      this._pendingShouldScroll = false
    },

    _scrollBottom() {
      this.$nextTick(() => {
        const el = this.$el.querySelector('.chat-messages')
        if (el) el.scrollTop = el.scrollHeight
      })
    },

    // Force scroll to bottom and clear the new-message pill. Used when the
    // user explicitly clicks "↓ N new" or sends a new message.
    // 强制滚底 + 清 pill。用户点 pill 或发新消息时调。
    scrollBottomNow() {
      this._userScrolledUp = false;
      this.newMsgCount = 0;
      this._scrollBottom();
    },

    // Bound to .chat-messages @scroll. Threshold is 60px from bottom — gives
    // some hysteresis so a click "go to bottom" doesn't immediately flip
    // the flag back on if the layout settles 1-2px short.
    // 监听 chat-messages 滚动事件。距离底部 60px 内算"在底部"，留余量
    // 避免 layout 误差让 flag 反复抖动。
    onChatScroll(e) {
      const el = e.target;
      const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
      const isAtBottom = distanceFromBottom < 60;
      if (isAtBottom) {
        this._userScrolledUp = false;
        this.newMsgCount = 0;
      } else {
        this._userScrolledUp = true;
      }
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
