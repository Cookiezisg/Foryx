// tab-tools.js — Tools tab: System tool invocation + full User Tools management.
// User Tools: CRUD, Run, Tests (create/run/generate), Versions (history/revert/pending).

document.addEventListener('alpine:init', () => {
  Alpine.data('toolsTab', () => ({
    section: 'system',          // 'system' | 'user'
    userDetailTab: 'run',       // 'run' | 'tests' | 'versions' | 'executions'

    // ── System Tools ──────────────────────────────────────────────────────────
    sysTools: [],
    sysSelected: '',
    sysArgs: '{}',
    sysResult: null,
    sysLoading: false,

    // ── User Tools list ───────────────────────────────────────────────────────
    userTools: [],
    userSearch: '',
    userSelected: null,

    // ── Run tab ───────────────────────────────────────────────────────────────
    userInput: '{}',
    userResult: null,
    userLoading: false,

    // ── Create / Edit forms ───────────────────────────────────────────────────
    showCreateForm: false,
    showEditForm: false,
    createForm: { name: '', description: '', code: '', tags: '' },
    editForm:   { name: '', description: '', code: '', tags: '' },
    crudLoading: false,
    crudError: '',

    // ── Import ────────────────────────────────────────────────────────────────
    showImport: false,
    importJson: '',

    // ── Tests tab ─────────────────────────────────────────────────────────────
    testCases: [],
    testResults: {},      // tcId → {ok, pass, output, errorMsg, elapsedMs}
    testRunning: {},      // tcId → bool
    testAllLoading: false,
    testAllSummary: null,
    showAddCase: false,
    addCaseForm: { name: '', inputData: '{}', expectedOutput: '' },
    addCaseLoading: false,
    generating: false,
    generateLog: [],      // [{name, ok}]

    // ── Versions tab ──────────────────────────────────────────────────────────
    versions: [],
    versionsLoading: false,
    pending: null,
    pendingLoading: false,

    // ── Executions tab (TE-10) ────────────────────────────────────────────────
    // Unified history of run/test executions for the selected forge. Sourced
    // from GET /api/v1/forges/{id}/executions which returns the forge_executions
    // table newest-first.
    //
    // Executions tab：选中 forge 的 run/test 统一历史。
    executions: [],
    executionsLoading: false,
    executionFilter: 'all',     // 'all' | 'run' | 'test'

    // ─────────────────────────────────────────────────────────────────────────

    init() {
      this.loadSysTools()
      this.loadUserTools()
    },

    // ── System Tools ─────────────────────────────────────────────────────────

    async loadSysTools() {
      try {
        const r = await fetch('/dev/tools')
        if (r.ok) {
          this.sysTools = await r.json()
          if (this.sysTools.length && !this.sysSelected) {
            this.sysSelected = this.sysTools[0].name
          }
        }
      } catch { /* server not up yet */ }
    },

    get sysDesc() {
      const t = this.sysTools.find(t => t.name === this.sysSelected)
      return t ? t.desc : ''
    },

    async invokeSystem() {
      if (!this.sysSelected || this.sysLoading) return
      this.sysLoading = true; this.sysResult = null
      try {
        const r = await fetch('/dev/invoke', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ tool: this.sysSelected, args: this.sysArgs }),
        })
        this.sysResult = await r.json()
      } catch (e) {
        this.sysResult = { ok: false, error: e.message, output: '', elapsedMs: 0 }
      }
      this.sysLoading = false
    },

    sysPretty() {
      if (!this.sysResult) return ''
      try { return JSON.stringify(JSON.parse(this.sysResult.output), null, 2) }
      catch { return this.sysResult.output }
    },

    handleSysKeydown(e) {
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); this.invokeSystem() }
    },

    // ── User Tools list ───────────────────────────────────────────────────────

    async loadUserTools() {
      try {
        const r = await fetch('/api/v1/forges?limit=200')
        if (r.ok) { const j = await r.json(); this.userTools = j.data || [] }
      } catch { /* server not up yet */ }
    },

    get filteredUserTools() {
      const q = this.userSearch.trim().toLowerCase()
      if (!q) return this.userTools
      return this.userTools.filter(t =>
        t.name.toLowerCase().includes(q) || (t.description || '').toLowerCase().includes(q)
      )
    },

    async selectUser(t) {
      this.userSelected = t
      this.userInput = '{}'
      this.userResult = null
      this.showEditForm = false
      this.showAddCase = false
      this.testCases = []; this.testResults = {}; this.testAllSummary = null; this.generateLog = []
      this.versions = []; this.pending = null
      this.executions = []
      if (this.userDetailTab === 'tests') await this.loadTestCases()
      if (this.userDetailTab === 'versions') await this.loadVersions()
      if (this.userDetailTab === 'executions') await this.loadExecutions()
    },

    async switchDetailTab(tab) {
      this.userDetailTab = tab
      if (!this.userSelected) return
      if (tab === 'tests' && this.testCases.length === 0) await this.loadTestCases()
      if (tab === 'versions' && this.versions.length === 0) await this.loadVersions()
      if (tab === 'executions' && this.executions.length === 0) await this.loadExecutions()
    },

    async loadExecutions() {
      if (!this.userSelected) return
      this.executionsLoading = true
      try {
        const r = await fetch(`/api/v1/forges/${this.userSelected.id}/executions?limit=50`)
        if (r.ok) { const j = await r.json(); this.executions = j.data || [] }
      } finally { this.executionsLoading = false }
    },

    get filteredExecutions() {
      if (this.executionFilter === 'all') return this.executions
      return this.executions.filter(e => e.kind === this.executionFilter)
    },

    fmtExecTime(s) {
      if (!s) return '—'
      try { return new Date(s).toLocaleString() } catch { return s }
    },

    execStatusColor(e) {
      if (e.kind === 'test') {
        if (e.pass === true) return 'var(--green)'
        if (e.pass === false) return 'var(--err)'
      }
      return e.ok ? 'var(--green)' : 'var(--err)'
    },

    execStatusLabel(e) {
      if (e.kind === 'test') return e.pass === true ? 'pass' : (e.pass === false ? 'fail' : '?')
      return e.ok ? 'ok' : 'fail'
    },

    // ── CRUD ─────────────────────────────────────────────────────────────────

    openCreate() {
      this.showCreateForm = true; this.showImport = false
      this.createForm = { name: '', description: '', code: '', tags: '' }
      this.crudError = ''
    },

    async createTool() {
      if (this.crudLoading) return
      this.crudLoading = true; this.crudError = ''
      try {
        const tags = this.createForm.tags
          ? this.createForm.tags.split(',').map(s => s.trim()).filter(Boolean) : []
        const r = await fetch('/api/v1/forges', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            name: this.createForm.name,
            description: this.createForm.description,
            code: this.createForm.code,
            tags,
          }),
        })
        const j = await r.json()
        if (!r.ok) { this.crudError = j.error?.message || 'Create failed'; return }
        this.userTools.unshift(j.data)
        this.showCreateForm = false
        await this.selectUser(j.data)
      } catch (e) { this.crudError = e.message }
      finally { this.crudLoading = false }
    },

    openEdit() {
      if (!this.userSelected) return
      const t = this.userSelected
      const tags = Array.isArray(t.tags) ? t.tags.join(', ') : (t.tags || '')
      this.editForm = { name: t.name, description: t.description || '', code: t.code || '', tags }
      this.showEditForm = true; this.crudError = ''
    },

    async saveTool() {
      if (!this.userSelected || this.crudLoading) return
      this.crudLoading = true; this.crudError = ''
      try {
        const tags = this.editForm.tags
          ? this.editForm.tags.split(',').map(s => s.trim()).filter(Boolean) : []
        const r = await fetch(`/api/v1/forges/${this.userSelected.id}`, {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            name: this.editForm.name,
            description: this.editForm.description,
            code: this.editForm.code,
            tags,
          }),
        })
        const j = await r.json()
        if (!r.ok) { this.crudError = j.error?.message || 'Save failed'; return }
        const updated = j.data
        const idx = this.userTools.findIndex(t => t.id === updated.id)
        if (idx >= 0) this.userTools[idx] = updated
        this.userSelected = updated
        this.showEditForm = false
      } catch (e) { this.crudError = e.message }
      finally { this.crudLoading = false }
    },

    async deleteTool() {
      if (!this.userSelected || !confirm(`Delete "${this.userSelected.name}"?`)) return
      const id = this.userSelected.id
      await fetch(`/api/v1/forges/${id}`, { method: 'DELETE' })
      this.userTools = this.userTools.filter(t => t.id !== id)
      this.userSelected = null
    },

    async exportTool() {
      if (!this.userSelected) return
      const r = await fetch(`/api/v1/forges/${this.userSelected.id}:export`, { method: 'POST' })
      if (!r.ok) return
      const blob = await r.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url; a.download = `${this.userSelected.name}.json`
      a.click(); URL.revokeObjectURL(url)
    },

    openImport() {
      this.showImport = true; this.showCreateForm = false
      this.importJson = ''; this.crudError = ''
    },

    async importTool() {
      if (this.crudLoading) return
      this.crudLoading = true; this.crudError = ''
      try {
        let payload
        try { payload = JSON.parse(this.importJson) }
        catch { this.crudError = 'Invalid JSON'; this.crudLoading = false; return }
        const r = await fetch('/api/v1/forges:import', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        const j = await r.json()
        if (!r.ok) { this.crudError = j.error?.message || 'Import failed'; return }
        this.userTools.unshift(j.data)
        this.showImport = false
        await this.selectUser(j.data)
      } catch (e) { this.crudError = e.message }
      finally { this.crudLoading = false }
    },

    // ── Run tab ───────────────────────────────────────────────────────────────

    async runUser() {
      if (!this.userSelected || this.userLoading) return
      this.userLoading = true; this.userResult = null
      try {
        let input = {}
        try { input = JSON.parse(this.userInput) } catch { /* bad JSON */ }
        const r = await fetch(`/api/v1/forges/${this.userSelected.id}:run`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ input }),
        })
        const j = await r.json()
        this.userResult = j.data ?? j
      } catch (e) {
        this.userResult = { ok: false, errorMsg: e.message }
      }
      this.userLoading = false
    },

    userResultPretty() {
      if (!this.userResult) return ''
      try { return JSON.stringify(this.userResult, null, 2) } catch { return String(this.userResult) }
    },

    handleUserKeydown(e) {
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); this.runUser() }
    },

    // ── Tests tab ─────────────────────────────────────────────────────────────

    async loadTestCases() {
      if (!this.userSelected) return
      const r = await fetch(`/api/v1/forges/${this.userSelected.id}/test-cases`)
      if (r.ok) { const j = await r.json(); this.testCases = j.data || [] }
    },

    testIcon(tcId) {
      if (this.testRunning[tcId]) return '●'
      const res = this.testResults[tcId]
      if (!res) return '—'
      if (!res.ok) return '!'
      return res.pass === true ? '✓' : res.pass === false ? '✗' : '—'
    },

    testIconColor(tcId) {
      if (this.testRunning[tcId]) return 'color:var(--accent)'
      const res = this.testResults[tcId]
      if (!res) return 'color:var(--text-mute)'
      if (!res.ok) return 'color:var(--red)'
      return res.pass === true ? 'color:var(--green)' : res.pass === false ? 'color:var(--red)' : 'color:var(--text-mute)'
    },

    async runOneTest(tcId) {
      if (!this.userSelected) return
      this.testRunning = { ...this.testRunning, [tcId]: true }
      try {
        const r = await fetch(
          `/api/v1/forges/${this.userSelected.id}/test-cases/${tcId}:run`,
          { method: 'POST' }
        )
        const j = await r.json()
        this.testResults = { ...this.testResults, [tcId]: j.data ?? j }
      } finally {
        const running = { ...this.testRunning }; delete running[tcId]
        this.testRunning = running
      }
    },

    async runAllTests() {
      if (!this.userSelected || this.testAllLoading) return
      this.testAllLoading = true; this.testAllSummary = null
      try {
        const r = await fetch(`/api/v1/forges/${this.userSelected.id}:test`, { method: 'POST' })
        const j = await r.json()
        const data = j.data ?? j
        this.testAllSummary = { total: data.total, passed: data.passed, failed: data.failed }
        for (const res of (data.results || [])) {
          this.testResults = { ...this.testResults, [res.testCaseId]: res }
        }
      } finally { this.testAllLoading = false }
    },

    async generateTestCases() {
      if (!this.userSelected || this.generating) return
      this.generating = true; this.generateLog = []
      try {
        const r = await fetch(
          `/api/v1/forges/${this.userSelected.id}:generate-test-cases?count=5`,
          { method: 'POST' }
        )
        const json = await r.json()
        if (!r.ok) {
          this.generateLog = [{ name: `Error: ${json.error?.message || 'failed'}`, ok: false }]
        } else {
          // Backend returns envelope { data: { notSupported, reason, testCases } }
          // 后端返回 envelope { data: { notSupported, reason, testCases } }
          const result = json.data || {}
          if (result.notSupported) {
            this.generateLog = [{ name: `Not supported: ${result.reason}`, ok: false }]
          } else {
            const cases = result.testCases || []
            this.testCases = [...this.testCases, ...cases.map(tc => ({
              id: tc.id, name: tc.name,
              inputData: tc.inputData, expectedOutput: tc.expectedOutput,
            }))]
            this.generateLog = cases.map(tc => ({ name: tc.name, ok: true }))
          }
        }
      } catch (e) {
        this.generateLog = [{ name: `Error: ${e.message}`, ok: false }]
      }
      this.generating = false
    },

    async addTestCase() {
      if (!this.userSelected || this.addCaseLoading) return
      this.addCaseLoading = true
      try {
        const r = await fetch(`/api/v1/forges/${this.userSelected.id}/test-cases`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            name: this.addCaseForm.name,
            inputData: this.addCaseForm.inputData,
            expectedOutput: this.addCaseForm.expectedOutput || '',
          }),
        })
        if (r.ok) {
          const j = await r.json()
          this.testCases = [...this.testCases, j.data]
          this.showAddCase = false
          this.addCaseForm = { name: '', inputData: '{}', expectedOutput: '' }
        }
      } finally { this.addCaseLoading = false }
    },

    async deleteTestCase(tcId) {
      if (!this.userSelected) return
      await fetch(`/api/v1/forges/${this.userSelected.id}/test-cases/${tcId}`, { method: 'DELETE' })
      this.testCases = this.testCases.filter(tc => tc.id !== tcId)
      const results = { ...this.testResults }; delete results[tcId]
      this.testResults = results
    },

    get testSummaryLabel() {
      const total = this.testCases.length
      if (!total) return ''
      const passed = Object.values(this.testResults).filter(r => r.pass === true).length
      return `  ${passed}/${total}`
    },

    // ── Versions tab ──────────────────────────────────────────────────────────

    async loadVersions() {
      if (!this.userSelected) return
      this.versionsLoading = true
      try {
        const [vr, pr] = await Promise.all([
          fetch(`/api/v1/forges/${this.userSelected.id}/versions`),
          fetch(`/api/v1/forges/${this.userSelected.id}/pending`),
        ])
        if (vr.ok) { const j = await vr.json(); this.versions = j.data || [] }
        this.pending = pr.ok ? ((await pr.json()).data ?? null) : null
      } finally { this.versionsLoading = false }
    },

    async revertToVersion(version) {
      if (!this.userSelected || !confirm(`Revert to v${version}?`)) return
      const r = await fetch(`/api/v1/forges/${this.userSelected.id}:revert`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ version }),
      })
      if (r.ok) {
        const j = await r.json(); const updated = j.data
        const idx = this.userTools.findIndex(t => t.id === updated.id)
        if (idx >= 0) this.userTools[idx] = updated
        this.userSelected = updated
        await this.loadVersions()
      }
    },

    async acceptPending() {
      if (!this.userSelected || !this.pending) return
      this.pendingLoading = true
      try {
        const r = await fetch(`/api/v1/forges/${this.userSelected.id}/pending:accept`, { method: 'POST' })
        if (r.ok) {
          const j = await r.json(); this.userSelected = j.data
          this.pending = null; await this.loadVersions()
        }
      } finally { this.pendingLoading = false }
    },

    async rejectPending() {
      if (!this.userSelected || !this.pending) return
      this.pendingLoading = true
      try {
        await fetch(`/api/v1/forges/${this.userSelected.id}/pending:reject`, { method: 'POST' })
        this.pending = null
      } finally { this.pendingLoading = false }
    },

    fmtTime(ts) {
      if (!ts) return ''
      const diff = Math.floor((new Date() - new Date(ts)) / 1000)
      if (diff < 60) return `${diff}s ago`
      if (diff < 3600) return `${Math.floor(diff/60)}m ago`
      if (diff < 86400) return `${Math.floor(diff/3600)}h ago`
      return `${Math.floor(diff/86400)}d ago`
    },
  }))
})
