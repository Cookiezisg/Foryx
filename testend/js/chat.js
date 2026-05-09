// chat.js — center chat panel: message list, streaming, send/cancel,
// attachments. Event-log protocol (post-cleanup model):
//
//   /api/v1/eventlog?conversationId=X    per-conv content stream
//     5 events: message_start / message_stop / block_start / block_delta / block_stop
//     6 block types: text / reasoning / tool_call / tool_result / progress / message
//
//   /api/v1/notifications                  global entity updates
//     1 envelope: {type, id, data, conversationId?}
//     handles type==="conversation" (autoTitle rename) here; others
//     consumed by other tabs.
//
// Display data shape (preserved from prior chat.js for renderer compatibility):
//   user message      → { id, role:'user', blocks:[{type:'text'|'attachment',...}], status, attachments }
//   assistant message → { id, role:'assistant', items:[reasoning|tool|text], status, ...tokens }
//
// Block.Content is now raw text (no JSON wrapper); Block.Attrs JSON
// carries metadata (tool name, progress stage, attachment refs). User
// attachments live in Message.Attrs JSON, not as blocks.
//
// chat.js — 中央对话面板：消息列表、流式、发/取消、附件。事件日志协议
// （清理后模型）。Block.Content 是裸文本（无 JSON 包装）；Block.Attrs
// JSON 携元数据。用户附件在 Message.Attrs JSON，不是 block。

document.addEventListener('alpine:init', () => {
  Alpine.data('chatPanel', () => ({
    messages: [],
    // Sub-messages (subagent runs) keyed by their id. Stored as display
    // models (same shape as top-level assistant messages) so SSE block
    // handlers can mutate them via _findMsg. The chat panel never
    // renders these top-level — they're attached to their parent's
    // 'subagent' item via item.subMsg pointer.
    //
    // Sub-messages（subagent run），按 id 存。形态与顶层 assistant 一致让
    // SSE block handler 经 _findMsg 直接 mutate。Chat panel 不顶层渲染——
    // 经 item.subMsg 指针挂在父 subagent 项上。
    subMessagesById: {},
    input: '',
    streaming: false,
    pendingAtts: [],      // [{id, fileName, mimeType}]
    uploading: false,
    askDraft: {},         // toolCallId → draft answer text (for in-flight AskUserQuestion)
    _es: null,            // /api/v1/eventlog SSE
    _unsubNotif: null,    // notifBus subscription disposer

    // Per-block expansion state — keyed by block.id (reasoning) or
    // tool-call id (tool steps). Default-folded keeps DOM bounded
    // regardless of how long bodies are.
    //
    // 按 block 展开状态——reasoning 用 block.id，tool 用 callId。默认
    // 折叠让 DOM 有界。
    expanded: {},

    toggleExpand(key) {
      this.expanded[key] = !this.expanded[key]
    },

    // Smart scroll: only auto-stick to bottom when the user is already
    // there. If scrolled up, new messages bump newMsgCount + show pill.
    //
    // 智能滚动：用户在底部时自动滚；上滚阅读历史时累加 newMsgCount + 浮 pill。
    _userScrolledUp: false,
    newMsgCount: 0,

    // Raw-snapshot modal: clicking [📋 raw] stashes the message JSON
    // here for inspection.
    //
    // Raw 模态：点 [📋 raw] 把 message JSON 存这里供检视。
    rawModal: { open: false, json: '', messageId: '', copied: false },

    // Catalog injection indicator (small pill in header).
    catalogFp: '',
    catalogGenerated: '',

    // Per-conversation system prompt editor.
    systemPrompt: '',
    systemPromptDraft: '',
    systemPromptOpen: false,
    systemPromptSaving: false,
    systemPromptSavedAt: 0,

    // ── Helpers: raw modal ──────────────────────────────────────────────────

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
        toast.warn('Copy failed — select the JSON and Cmd+C manually.')
      }
    },

    get conversationId() { return Alpine.store('app').conversationId },
    get title() { return Alpine.store('app').conversationTitle },

    init() {
      this.loadCatalogStatus()
      this._connectNotifications()  // global notifications run for the panel's lifetime
      this.$watch('conversationId', id => {
        this._closeEventLog()
        this.messages = []
        this.subMessagesById = {}
        this.streaming = false
        this.pendingAtts = []
        this.systemPrompt = ''
        this.systemPromptDraft = ''
        // Drop expand state from the previous conversation — block IDs
        // never collide across convs, so leftover entries are harmless,
        // but they accumulate forever in heavy sessions.
        // 清上一对话的 expand state——block ID 跨 conv 不冲突，旧条目无害，
        // 但久用累积。
        this.expanded = {}
        if (id) {
          this.loadMessages(id).then(() => this._connectEventLog(id))
          this.loadConvMeta(id)
        }
        this.loadCatalogStatus()
      })
      document.addEventListener('keydown', (e) => {
        if (e.key === 'Escape' && this.rawModal.open) {
          this.closeRaw();
        }
      });
    },

    // ── Conversation metadata + system prompt edit ──────────────────────────

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
      if (!confirm('Delete the current conversation?')) return
      try {
        await fetch(`/api/v1/conversations/${id}`, { method: 'DELETE' })
        Alpine.store('app').conversationId = ''
        Alpine.store('app').conversationTitle = ''
        this.messages = []
        document.dispatchEvent(new CustomEvent('conv-deleted'))
      } catch (e) {
        toast.error('Delete failed: ' + e)
      }
    },

    // ── loadMessages: REST hydrate from /messages ───────────────────────────

    async loadMessages(id) {
      const r = await fetch(`/api/v1/conversations/${id}/messages?limit=200`)
      if (!r.ok) return
      const j = await r.json()
      const all = (j.data || []).filter(m => m.status !== 'pending')

      // Backend returns ALL messages including subagent sub-messages
      // (those with non-empty parentBlockId). Split: convert + index
      // sub-messages first so the top-level renderer can attach them
      // via item.subMsg when it processes the 'message' block.
      //
      // 后端返全部消息（含 subagent sub，parentBlockId 非空）。先转 sub +
      // 入 map，让顶层渲染器遇到 'message' block 时经 item.subMsg 挂上。
      this.subMessagesById = {}
      const topLevel = []
      for (const m of all) {
        if (m.parentBlockId) {
          // Sub-messages are always assistant role (subagent runs).
          // Sub-message 永远 assistant 角色（subagent run）。
          this.subMessagesById[m.id] = this._assistantMsgFromBlocks(m)
        } else {
          topLevel.push(m)
        }
      }
      this.messages = topLevel.map(m => this._messageFromSnapshot(m))
    },

    // ── Snapshot → display message (new Block model) ────────────────────────
    //
    // Block.Content is raw text (no JSON wrapper). Tool name lives in
    // Block.Attrs JSON {tool:name}. Tool args are Block.Content as JSON
    // string. Attachments live in Message.Attrs JSON {attachments:[...]}.
    //
    // Block.Content 是裸文本。Tool name 在 Block.Attrs JSON。Args 是
    // Block.Content（JSON 串）。附件在 Message.Attrs JSON。

    _parseAttrs(s) {
      if (!s) return {}
      try { return JSON.parse(s) || {} } catch { return {} }
    },

    _messageFromSnapshot(m) {
      return m.role === 'user'
        ? this._userMsgFromBlocks(m)
        : this._assistantMsgFromBlocks(m)
    },

    _userMsgFromBlocks(m) {
      const blocks = []
      for (const b of (m.blocks || [])) {
        if (b.type === 'text') {
          blocks.push({ type: 'text', content: b.content || '' })
        }
      }
      // Attachments from Message.Attrs JSON.
      // 附件来自 Message.Attrs JSON。
      const attrs = this._parseAttrs(m.attrs)
      const attachments = Array.isArray(attrs.attachments) ? attrs.attachments : []
      for (const a of attachments) {
        blocks.push({
          type: 'attachment',
          fileName: a.fileName || '',
          mimeType: a.mimeType || '',
          id: a.attachmentId,
        })
      }
      return { id: m.id, role: 'user', blocks, status: m.status, raw: m }
    },

    _assistantMsgFromBlocks(m) {
      const items = []
      const toolMap = {}  // toolCallId → item

      for (const b of (m.blocks || [])) {
        const battrs = this._parseAttrs(b.attrs)
        switch (b.type) {
          case 'reasoning':
            items.push({
              type: 'reasoning',
              content: b.content || '',
              done: b.status !== 'streaming',
              expandKey: 'r:' + b.id,
            })
            break

          case 'text':
            if (b.content) items.push({ type: 'text', content: b.content })
            break

          case 'tool_call': {
            // Block.ID is the LLM tool-call ID (we reuse it as block id).
            // Tool name in Attrs.tool. Args are Content (raw JSON string).
            // Block.ID 是 LLM tool-call ID。Tool name 在 Attrs.tool；
            // Args 是 Content (裸 JSON)。
            const item = {
              type: 'tool',
              toolCallId: b.id,
              toolName: battrs.tool || '',
              summary: '', destructive: false, executionGroup: 0,
              input: b.content || '',
              result: null, ok: null, errorMsg: '', elapsedMs: 0,
              expandKey: 't:' + b.id,
            }
            items.push(item)
            toolMap[b.id] = item
            break
          }

          case 'tool_result': {
            // Parent block ID = the tool_call's ID = LLM tool-call ID.
            // Pair into the existing tool item.
            // Parent block ID = tool_call ID = LLM tool-call ID。配对到
            // 已有 tool item。
            const parentID = b.parentBlockId
            const item = toolMap[parentID]
            if (item) {
              item.result = b.content || ''
              item.ok = b.status !== 'error'
              item.errorMsg = b.error || ''
            }
            break
          }

          case 'progress': {
            // Progress block (sandbox install / network fetch logs etc).
            // Parented under a tool_call — attach to that tool item if found,
            // else render as standalone progress item.
            //
            // Progress block（沙箱装包 / 网络拉块日志等）。挂在 tool_call
            // 下——挂得上就附到该 tool item，挂不上就独立显示。
            const parentID = b.parentBlockId
            const item = toolMap[parentID]
            if (item) {
              item.progress = (item.progress || '') + (b.content || '')
            } else {
              items.push({
                type: 'progress',
                stage: battrs.stage || '',
                content: b.content || '',
                done: b.status !== 'streaming',
              })
            }
            break
          }

          case 'message': {
            // Subagent placeholder. attrs.messageId points to the sub
            // message — the sub-message snapshot lives in
            // subMessagesById (populated in loadMessages BEFORE this
            // function is called). Render as expandable pill; on
            // expand the template dumps the sub-msg as JSON.
            //
            // Subagent 占位。attrs.messageId 指向 sub message——sub 快照在
            // subMessagesById（loadMessages 在调本函数前已先入 map）。
            // 渲染为可展开 pill；展开时模板 dump sub-msg JSON。
            const subId = battrs.messageId || ''
            items.push({
              type: 'subagent',
              subMessageId: subId,
              subType: battrs.type || 'subagent',
              subMsg: subId ? (this.subMessagesById[subId] || null) : null,
              done: b.status !== 'streaming',
              expandKey: 's:' + b.id,
            })
            break
          }
        }
      }

      return {
        id: m.id, role: 'assistant', items, status: m.status,
        stopReason: m.stopReason || '',
        errorCode: m.errorCode || '',
        errorMessage: m.errorMessage || '',
        inputTokens: m.inputTokens || 0,
        outputTokens: m.outputTokens || 0,
        raw: m,
      }
    },

    // ── Attachment upload ───────────────────────────────────────────────────

    pickFile() { this.$refs.fileInput.click() },

    async onFileChange(e) {
      const files = e.target.files
      if (!files || files.length === 0) return
      for (const file of files) await this.uploadAttachment(file)
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

    removeAtt(idx) { this.pendingAtts.splice(idx, 1) },

    // ── send ────────────────────────────────────────────────────────────────

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
        // wait for $watch to fire and connect SSE
        await new Promise(r => setTimeout(r, 100))
      }

      const attIds = this.pendingAtts.map(a => a.id)
      const attsSnapshot = [...this.pendingAtts]
      const optimisticContent = content
      this.input = ''
      this.pendingAtts = []

      // Optimistic-insert user row immediately. SSE will replace by id
      // when the user message arrives (~50ms later).
      //
      // 立刻乐观插入 user 行。SSE 收到 user message（~50ms 后）按 id 替换。
      const tempId = '__pending__' + Date.now() + '_' + Math.random().toString(36).slice(2, 8)
      const userBlocks = []
      if (optimisticContent) userBlocks.push({ type: 'text', content: optimisticContent })
      for (const a of attsSnapshot) userBlocks.push({
        type: 'attachment', fileName: a.fileName, mimeType: a.mimeType, id: a.id,
      })
      this.messages.push({ id: tempId, role: 'user', blocks: userBlocks, status: 'sending' })
      this._scrollBottom()

      let r
      try {
        r = await fetch(`/api/v1/conversations/${convId}/messages`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ content: optimisticContent, attachmentIds: attIds }),
        })
      } catch (e) {
        this._dropPendingRow(tempId)
        toast.error('Send failed: ' + e)
        return
      }

      if (!r.ok) {
        this._dropPendingRow(tempId)
        toast.error('Send failed: HTTP ' + r.status)
        return
      }

      const j = await r.json()
      // Stamp real id onto the optimistic row so SSE's user message_start
      // (which arrives shortly with the same id) finds it and updates in place.
      //
      // 把真 id 打到乐观行——SSE 的 user message_start 用同 id 找到此行
      // 原位更新。
      const idx = this.messages.findIndex(m => m.id === tempId)
      if (idx >= 0) {
        this.messages[idx] = { ...this.messages[idx], id: j.data.messageId, status: 'completed' }
      }

      this.streaming = true
      this.scrollBottomNow()
    },

    _dropPendingRow(tempId) {
      const idx = this.messages.findIndex(m => m.id === tempId)
      if (idx >= 0) this.messages.splice(idx, 1)
    },

    async cancel() {
      const id = this.conversationId
      if (!id) return
      try {
        const r = await fetch(`/api/v1/conversations/${id}/stream`, { method: 'DELETE' })
        if (r.status === 204 || r.ok) {
          toast.info('Stream cancelled')
        } else {
          toast.error('Cancel failed: HTTP ' + r.status)
        }
      } catch (e) {
        toast.error('Cancel failed: ' + e)
      }
      // Backend will emit a final message_stop with status=cancelled and
      // _onMessageStop will flip streaming=false. This is a UX safety
      // net for the rare case the cancel signal races / drops.
      // 后端会推 message_stop status=cancelled，_onMessageStop 翻 streaming=
      // false。这里是 UX 兜底，cancel 信号丢了也能解锁输入框。
      setTimeout(() => { this.streaming = false }, 1000)
    },

    // ── SSE: event-log protocol (per-conversation) ──────────────────────────

    _connectEventLog(id) {
      this._closeEventLog()

      // Per-block-id index for fast lookup during deltas. Each entry is
      // { msgId, item } where item is the message item (e.g. tool item)
      // OR { msgId, blockId } for blocks rendered as message-level items.
      // Maintained by the event handlers.
      //
      // Per-block-id 索引让 delta 查找快。条目 { msgId, item } 或
      // { msgId, blockId }，由事件 handler 维护。
      this._blockIndex = new Map()

      const es = new EventSource(`/api/v1/eventlog?conversationId=${id}`)
      this._es = es

      es.addEventListener('message_start', e => this._onMessageStart(JSON.parse(e.data)))
      es.addEventListener('message_stop',  e => this._onMessageStop(JSON.parse(e.data)))
      es.addEventListener('block_start',   e => this._onBlockStart(JSON.parse(e.data)))
      es.addEventListener('block_delta',   e => this._onBlockDelta(JSON.parse(e.data)))
      es.addEventListener('block_stop',    e => this._onBlockStop(JSON.parse(e.data)))

      // Disconnect / 410 Gone fallback: EventSource auto-reconnects, but
      // its stale Last-Event-ID can land before the server's replay
      // buffer (4096) — backend returns 410 SEQ_TOO_OLD and we'd miss
      // events. Debounce a few error ticks then full-refetch via REST.
      // browsers fire onerror frequently on transient drops, so we wait
      // 3 seconds of unbroken errors before the heavy rehydrate.
      //
      // 断线 / 410 Gone 兜底：EventSource 自动重连但旧 Last-Event-ID 可能落
      // 在 server replay buffer (4096) 后——后端返 410 SEQ_TOO_OLD 我们就漏。
      // 错误 tick 累积 3 秒后走 REST 全态 refetch。瞬时 drop 不触发。
      let errSince = 0
      es.onerror = () => {
        if (!errSince) errSince = Date.now()
        if (Date.now() - errSince < 3000) return
        // Connection has been broken for 3s+. Re-hydrate via REST then
        // reopen SSE.
        // 已断 3s+，REST 重新拿全态再重开 SSE。
        if (this.conversationId !== id) return // user already switched
        this._closeEventLog()
        this.loadMessages(id).then(() => {
          if (this.conversationId === id) this._connectEventLog(id)
        })
      }
      es.onopen = () => { errSince = 0 }
    },

    _closeEventLog() {
      if (this._es) { this._es.close(); this._es = null }
      this._blockIndex = null
    },

    _findMsg(id) {
      return this.messages.find(m => m.id === id) || this.subMessagesById[id]
    },

    _onMessageStart(ev) {
      // ev = { conversationId, id, parentBlockId?, role, attrs }
      //
      // User messages are owned by send()'s optimistic insert + REST POST;
      // the SSE stream's user message_start is purely informational. Skip
      // it to avoid a duplicate-key row when SSE delivers before send() has
      // stamped tempId → real id (Alpine x-for with two same-:key rows
      // can render only the empty SSE stub, swallowing the typed text).
      //
      // user 消息由 send() 乐观插入 + REST POST 全管；SSE 的 user
      // message_start 仅信息性。跳过避免 SSE 早到产生重复 key 行
      // （Alpine x-for 见两行同 key 时可能只渲染空 stub，吞掉用户输入）。
      if (ev.role === 'user') return
      if (this._findMsg(ev.id)) return
      const stub = { id: ev.id, role: 'assistant', items: [], status: 'streaming',
        stopReason: '', errorCode: '', errorMessage: '',
        inputTokens: 0, outputTokens: 0, raw: ev }
      // Sub-message routing: parentBlockId set → it's a subagent run.
      // Index in subMessagesById (NOT pushed to top-level), then attach
      // to the parent's 'subagent' item via blockIndex lookup so the
      // pill's subMsg pointer becomes live and the template renders.
      //
      // Sub-message 路由：parentBlockId 非空 = subagent run。入
      // subMessagesById（不顶层 push），经 blockIndex 找父 'subagent' 项挂
      // subMsg 指针让模板可渲染。
      if (ev.parentBlockId) {
        this.subMessagesById[ev.id] = stub
        const parent = this._blockIndex && this._blockIndex.get(ev.parentBlockId)
        if (parent && parent.kind === 'message' && parent.ref) {
          parent.ref.subMsg = stub
        }
        return
      }
      this.messages.push(stub)
      if (!this._userScrolledUp) this._scrollBottom()
      else this.newMsgCount++
    },

    _onMessageStop(ev) {
      // ev = { conversationId, id, status, stopReason?, errorCode?, errorMessage?, inputTokens?, outputTokens? }
      const m = this._findMsg(ev.id)
      if (!m) return
      m.status = ev.status
      m.stopReason = ev.stopReason || ''
      m.errorCode = ev.errorCode || ''
      m.errorMessage = ev.errorMessage || ''
      m.inputTokens = ev.inputTokens || 0
      m.outputTokens = ev.outputTokens || 0
      if (m.role === 'assistant' && ev.status !== 'streaming') {
        this.streaming = false
      }
    },

    _onBlockStart(ev) {
      // ev = { conversationId, id, parentId, messageId, blockType, attrs }
      const m = this._findMsg(ev.messageId)
      if (!m) return
      // User message blocks are entirely owned by send()'s optimistic insert
      // (typed text + attachments). SSE block events for user are redundant
      // and would duplicate the bubble. Skip.
      // user 消息的 blocks 完全由 send() 乐观插入管（输入文本 + 附件）；
      // SSE 的 user block 事件冗余，会让气泡重复。跳过。
      if (m.role === 'user') return
      const battrs = ev.attrs || {}

      switch (ev.blockType) {
        case 'reasoning':
        case 'text': {
          const item = ev.blockType === 'reasoning'
            ? { type: 'reasoning', content: '', done: false, expandKey: 'r:' + ev.id }
            : { type: 'text', content: '' }
          m.items.push(item)
          this._blockIndex.set(ev.id, { msg: m, ref: item, kind: ev.blockType })
          break
        }

        case 'tool_call': {
          const item = {
            type: 'tool',
            toolCallId: ev.id,
            toolName: battrs.tool || '',
            summary: '', destructive: false, executionGroup: 0,
            input: '',
            result: null, ok: null, errorMsg: '', elapsedMs: 0,
            expandKey: 't:' + ev.id,
          }
          m.items.push(item)
          this._blockIndex.set(ev.id, { msg: m, ref: item, kind: 'tool_call' })
          break
        }

        case 'tool_result': {
          // Find parent tool item; create result placeholder via blockIndex.
          // 找父 tool item；blockIndex 加一条占位等 delta。
          const parent = this._blockIndex.get(ev.parentId)
          this._blockIndex.set(ev.id, {
            msg: m, ref: parent ? parent.ref : null, kind: 'tool_result',
            buf: '',
          })
          break
        }

        case 'progress': {
          // Progress block parented under a tool_call — append to the
          // tool item's `progress` field. Untracked parent → standalone.
          //
          // Progress 挂 tool_call 下——追加到 tool item 的 progress 字段；
          // 父没找到 → 独立展示。
          const parent = this._blockIndex.get(ev.parentId)
          if (parent && parent.kind === 'tool_call') {
            this._blockIndex.set(ev.id, { msg: m, ref: parent.ref, kind: 'progress-attached' })
          } else {
            const item = {
              type: 'progress',
              stage: battrs.stage || '',
              content: '',
              done: false,
            }
            m.items.push(item)
            this._blockIndex.set(ev.id, { msg: m, ref: item, kind: 'progress' })
          }
          break
        }

        case 'message': {
          // Subagent placeholder.
          const item = {
            type: 'subagent',
            subMessageId: battrs.messageId || '',
            subType: battrs.type || 'subagent',
            done: false,
            expandKey: 's:' + ev.id,
          }
          m.items.push(item)
          this._blockIndex.set(ev.id, { msg: m, ref: item, kind: 'message' })
          break
        }
      }
    },

    _onBlockDelta(ev) {
      // ev = { conversationId, id, delta }
      const idx = this._blockIndex.get(ev.id)
      if (!idx) return
      switch (idx.kind) {
        case 'text':
        case 'reasoning':
          idx.ref.content = (idx.ref.content || '') + ev.delta
          break

        case 'tool_call':
          idx.ref.input = (idx.ref.input || '') + ev.delta
          break

        case 'tool_result': {
          // Buffer until block_stop so we can finalize ok/error then.
          // 缓存到 block_stop 时一并 finalize ok/error。
          idx.buf = (idx.buf || '') + ev.delta
          if (idx.ref) {
            idx.ref.result = idx.buf
          }
          break
        }

        case 'progress-attached':
          idx.ref.progress = (idx.ref.progress || '') + ev.delta
          break

        case 'progress':
          idx.ref.content = (idx.ref.content || '') + ev.delta
          break
      }
      if (!this._userScrolledUp) this._scrollBottom()
    },

    _onBlockStop(ev) {
      // ev = { conversationId, id, status, error? }
      const idx = this._blockIndex.get(ev.id)
      if (!idx) return
      switch (idx.kind) {
        case 'reasoning':
          idx.ref.done = true
          break

        case 'tool_result':
          if (idx.ref) {
            idx.ref.ok = ev.status !== 'error'
            idx.ref.errorMsg = ev.error || ''
          }
          break

        case 'progress':
        case 'progress-attached':
          if (idx.ref && idx.kind === 'progress') idx.ref.done = true
          break

        case 'message':
          idx.ref.done = true
          break
      }
    },

    // ── SSE: notifications (global) ─────────────────────────────────────────

    _connectNotifications() {
      // Subscribe to the shared notifBus instead of opening our own
      // EventSource. The bus owns the underlying /api/v1/notifications
      // connection (see app.js Alpine.store('notifBus')) and fans events
      // to every listener — saves an HTTP/1.1 connection slot, since
      // tab-notifications.js subscribes to the same bus.
      //
      // 订共享 notifBus 而不是自己开 EventSource。bus 持有 /api/v1/notifications
      // 真实连接（见 app.js）并把事件分发给所有 listener——省一个
      // HTTP/1.1 connection slot（tab-notifications 也订同一 bus）。
      this._unsubNotif = Alpine.store('notifBus').subscribe(n => {
        switch (n.type) {
          case 'conversation':
            // autoTitle / rename — update title if it's the active conv.
            // autoTitle / 改名——本对话则更新标题。
            if (n.id === this.conversationId && n.data && n.data.title) {
              Alpine.store('app').conversationTitle = n.data.title
              document.dispatchEvent(new CustomEvent('conv-created'))
            }
            break
          case 'catalog':
            // Catalog regenerated (skill / forge / mcp registry changes
            // ripple here). The chat header pill renders fingerprint +
            // generator — refresh it without waiting for a conv switch.
            //
            // Catalog 重新生成（skill / forge / mcp 改动会 ripple 到这）。
            // chat 头部 pill 显示 fingerprint + generator——直接刷新，不等切对话。
            this.loadCatalogStatus()
            break
          // type==="todo" / "skill" / "mcp_server" → 由 tab-notifications + 各模块自己 tab 处理，chat 头部不需要。
          // todo / skill / mcp_server handled by tab-notifications and their own tabs; chat header doesn't need them.
        }
      })
    },

    _closeNotifications() {
      if (this._unsubNotif) { this._unsubNotif(); this._unsubNotif = null }
    },

    // ── Scroll ──────────────────────────────────────────────────────────────

    _scrollBottom() {
      this.$nextTick(() => {
        const el = this.$el.querySelector('.chat-messages')
        if (el) el.scrollTop = el.scrollHeight
      })
    },

    scrollBottomNow() {
      this._userScrolledUp = false;
      this.newMsgCount = 0;
      this._scrollBottom();
    },

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

    // ── Format helpers ──────────────────────────────────────────────────────

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

    // ── AskUserQuestion interactive prompt ──────────────────────────────────

    // askQuestion / askOptions parse the tool item's input (JSON args sent
    // by the LLM) so the template can render the question text + suggested
    // options without re-parsing on every render.
    //
    // askQuestion / askOptions 解 tool item 的 input（LLM 发的 args JSON），
    // 让模板渲染时不必反复解析。
    askQuestion(item) {
      try { return (JSON.parse(item.input || '{}').question) || '' } catch { return '' }
    },
    askOptions(item) {
      try {
        const opts = JSON.parse(item.input || '{}').options
        return Array.isArray(opts) ? opts : []
      } catch { return [] }
    },

    handleAskKeydown(e, item) {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault()
        this.submitAnswer(item, this.askDraft[item.toolCallId] || '')
      }
    },

    // submitAnswer posts the user's answer to /answers, unblocking the
    // AskUserQuestion tool's Wait. The SSE stream then delivers tool_result
    // (with the same answer text) and the assistant continues.
    //
    // submitAnswer 把答案 POST 到 /answers 解锁 AskUserQuestion.Wait。SSE
    // 接着推 tool_result（同答案文本），assistant 继续。
    async submitAnswer(item, answer) {
      const text = (answer || '').trim()
      if (!text) return
      const convId = this.conversationId
      if (!convId) return
      try {
        const r = await fetch(`/api/v1/conversations/${convId}/answers`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ toolCallId: item.toolCallId, answer: text }),
        })
        if (!r.ok && r.status !== 204) {
          toast.error('Send answer failed: HTTP ' + r.status)
          return
        }
        // Optimistic: clear draft. Tool_result block_stop will arrive via
        // SSE shortly and update item.result/ok normally.
        // 乐观：清草稿。SSE 很快推 tool_result block_stop 正常更新 item.result/ok。
        delete this.askDraft[item.toolCallId]
      } catch (e) {
        toast.error('Send answer failed: ' + e)
      }
    },
  }))
})
