// tab-sql.js — read-only SQL query tab (POST /dev/sql) + schema browser
// (GET /dev/schema). The schema panel lists every user table with row counts;
// clicking a table populates a "SELECT * LIMIT 20" so testers don't have to
// remember table names. Expanding a table reveals its columns.
//
// tab-sql.js — 只读 SQL 查询 tab + schema 浏览器。schema 面板列出每个
// 用户表 + 行数；点表名自动填 "SELECT * LIMIT 20"，免背表名。展开看列。

document.addEventListener('alpine:init', () => {
  Alpine.data('sqlTab', () => ({
    sql: 'SELECT id, conversation_id, parent_block_id, role, status, stop_reason, error_code, attrs, input_tokens, output_tokens, created_at FROM messages ORDER BY created_at DESC LIMIT 20',
    result: null,
    error: null,
    loading: false,
    wrap: false,
    schema: [],          // [{name, rowCount, columns:[{name,type,notNull,pk,default}]}]
    schemaLoading: false,
    schemaErr: '',
    expandedTable: '',
    schemaFilter: '',

    init() { this.loadSchema() },

    shortcuts: [
      {
        label: 'messages',
        sql: 'SELECT id, conversation_id, parent_block_id, role, status, stop_reason, error_code, attrs, input_tokens, output_tokens, created_at FROM messages ORDER BY created_at DESC LIMIT 20',
      },
      {
        label: 'message_blocks',
        sql: 'SELECT id, message_id, parent_block_id, seq, type, status, attrs, content, error, created_at FROM message_blocks ORDER BY created_at DESC, seq ASC LIMIT 50',
      },
      {
        label: 'blocks for conv',
        sql: `SELECT b.id, b.message_id, b.parent_block_id, b.seq, b.type, b.status, substr(b.content,1,80) as content_preview
FROM message_blocks b
JOIN messages m ON m.id = b.message_id
ORDER BY m.created_at ASC, b.seq ASC
LIMIT 100`,
      },
      {
        label: 'conversations',
        sql: "SELECT id, title, auto_titled, created_at, updated_at FROM conversations WHERE deleted_at IS NULL ORDER BY created_at DESC",
      },
      {
        label: 'api_keys',
        sql: "SELECT id, provider, display_name, key_masked, test_status FROM api_keys WHERE deleted_at IS NULL",
      },
      {
        label: 'model_configs',
        sql: "SELECT * FROM model_configs WHERE deleted_at IS NULL",
      },
      {
        label: 'attachments',
        sql: 'SELECT id, user_id, file_name, mime_type, size_bytes, created_at FROM attachments WHERE deleted_at IS NULL ORDER BY created_at DESC LIMIT 20',
      },
      {
        label: 'forges',
        sql: "SELECT id, name, description, version_count, tags FROM forges WHERE deleted_at IS NULL ORDER BY created_at DESC",
      },
      {
        label: 'forge_versions',
        sql: "SELECT id, forge_id, version, status, change_reason FROM forge_versions ORDER BY created_at DESC LIMIT 20",
      },
      {
        label: 'forge_test_cases',
        sql: "SELECT id, forge_id, name, input_data, expected_output FROM forge_test_cases",
      },
      {
        label: 'forge_executions (run)',
        sql: "SELECT id, forge_id, forge_version, ok, elapsed_ms, triggered_by, created_at FROM forge_executions WHERE kind='run' ORDER BY created_at DESC LIMIT 20",
      },
      {
        label: 'forge_executions (test)',
        sql: "SELECT id, forge_id, batch_id, ok, pass, triggered_by, created_at FROM forge_executions WHERE kind='test' ORDER BY created_at DESC LIMIT 20",
      },
      {
        label: 'forge_executions (chat-triggered)',
        sql: "SELECT id, forge_id, kind, ok, conversation_id, message_id, tool_call_id, created_at FROM forge_executions WHERE triggered_by='chat' ORDER BY created_at DESC LIMIT 20",
      },
    ],

    async loadSchema() {
      this.schemaLoading = true; this.schemaErr = ''
      try {
        const r = await fetch('/dev/schema')
        if (!r.ok) { this.schemaErr = `HTTP ${r.status}`; return }
        this.schema = await r.json() || []
      } catch (e) {
        this.schemaErr = String(e)
      } finally {
        this.schemaLoading = false
      }
    },

    filteredSchema() {
      const q = this.schemaFilter.trim().toLowerCase()
      if (!q) return this.schema
      return this.schema.filter(t => t.name.toLowerCase().includes(q))
    },

    selectTable(t) {
      this.sql = `SELECT * FROM ${t.name} ORDER BY rowid DESC LIMIT 20`
      this.run()
    },

    toggleTable(name) {
      this.expandedTable = this.expandedTable === name ? '' : name
    },

    async run() {
      if (!this.sql.trim()) return
      this.loading = true; this.error = null; this.result = null
      try {
        const r = await fetch('/dev/sql', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ sql: this.sql }),
        })
        const data = await r.json()
        if (data.error) this.error = data.error
        else this.result = data
      } catch (e) {
        this.error = e.message
      }
      this.loading = false
    },

    setSQL(sql) { this.sql = sql },

    handleKeydown(e) {
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
        e.preventDefault(); this.run()
      }
    },

    fmt(v) {
      if (v === null || v === undefined) return ''
      if (typeof v === 'string' && v.length > 80) return v.slice(0, 80) + '…'
      return String(v)
    },

    // clickCell: if the cell contains a known ID, jump to a detail query for it.
    clickCell(v) {
      if (!v) return
      const s = String(v)
      let q = null
      if (s.startsWith('msg_')) {
        q = `SELECT b.id, b.parent_block_id, b.seq, b.type, b.status, b.content, b.attrs, b.created_at\nFROM message_blocks b\nWHERE b.message_id = '${s}'\nORDER BY b.seq ASC`
      } else if (s.startsWith('blk_')) {
        q = `SELECT * FROM message_blocks WHERE id = '${s}'`
      } else if (s.startsWith('cv_')) {
        q = `SELECT id, conversation_id, role, status, stop_reason, error_code, error_message, input_tokens, output_tokens, created_at, updated_at\nFROM messages WHERE conversation_id = '${s}' ORDER BY created_at ASC`
      } else if (s.startsWith('att_')) {
        q = `SELECT id, user_id, file_name, mime_type, size_bytes, storage_path, created_at, deleted_at FROM attachments WHERE id = '${s}'`
      } else if (s.startsWith('f_')) {
        q = `SELECT id, name, description, code, version_count, tags FROM forges WHERE id = '${s}'`
      } else if (s.startsWith('fv_')) {
        q = `SELECT id, forge_id, version, status, change_reason, code FROM forge_versions WHERE id = '${s}'`
      } else if (s.startsWith('fe_')) {
        q = `SELECT * FROM forge_executions WHERE id = '${s}'`
      }
      if (q) { this.sql = q; this.run() }
    },
  }))
})
