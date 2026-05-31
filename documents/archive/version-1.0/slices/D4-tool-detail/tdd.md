# D4 · 工具详情（代码视图）— 技术设计文档

**切片**：D4  
**状态**：待 Review

---

## 1. 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 布局 | 独立 Tool Tab 或 Chat+Tool 右侧面板 | Tab 系统统一管理 |
| 代码编辑器 | Monaco Editor（`@monaco-editor/react`）| Python 语法高亮、只读/编辑切换 |
| Diff 查看 | Monaco DiffEditor（同一包的 named export）| side-by-side diff，零新依赖 |
| 参数解析 | Python AST（`forge.ParseFunctionAST`）+ 正则 fallback | 100% 准确，支持泛型 `list[int]` |
| Inline 编辑 | InlineEdit（文本）+ InlineSelect（下拉枚举）| Notion 风格，即改即存 |
| 元数据同步 | `PATCH /meta` → `NormalizeCodeAnnotations` | DB + 代码标注双向一致 |
| 测试历史 | `tool_test_history` 表 | 最多 20 条 |
| 版本历史 | `tool_versions` 表 + `VersionHistoryView` 组件 | 自动快照 + DiffEditor 查看 + 一键恢复 |

---

## 2. 目录结构

```
frontend/src/components/tools/
├── ToolMainView.tsx    # 整体容器：Header(InlineEdit/InlineSelect/TagBar/版本badge)
│                       #   + Tab 切换(code/params/test)
│                       #   + VersionHistoryView(historyMode 切换)
│                       #   + CodeTab(Monaco Editor + diff review + pending change)
│                       #   + ParamsTab + TestTab
│                       #   + InlineEdit / InlineSelect / TagBar (内部组件)
└── ToolCard.tsx        # 工具列表卡片
```
注：所有子组件（CodeTab、ParamsTab、TestTab、InlineEdit、InlineSelect、TagBar、VersionHistoryView、InlineDiff）均定义在 ToolMainView.tsx 内部，非独立文件。

---

## 3. HTTP API 路由

```go
// backend/internal/server/routes.go（复用 D1/D2 已有路由，新增以下）
mux.HandleFunc("GET /api/tools/{id}", s.getTool)
mux.HandleFunc("PUT /api/tools/{id}", s.updateTool)
mux.HandleFunc("GET /api/tools/{id}/test-history", s.listToolTestHistory)
// POST /api/tools/{id}/run 已在 D2 定义
```

---

## 4. 前端组件

### `ToolMainView.tsx`

```tsx
type ToolTab = 'code' | 'params' | 'test'

export function ToolMainView({ toolId }: { toolId: string }) {
    const [tool, setTool] = useState<Tool | null>(null)
    const [tab, setTab] = useState<ToolTab>('code')

    useEffect(() => { GetTool(toolId).then(setTool) }, [toolId])

    if (!tool) return <div className="h-full flex items-center justify-center"><LoadingSpinner /></div>

    const isBuiltin = tool.builtin === true

    return (
        <div className="flex flex-col h-full">
            {/* 顶部信息栏 */}
            <div className="px-4 py-3 border-b border-neutral-800 flex items-start justify-between flex-shrink-0">
                <div>
                    <div className="flex items-center gap-2">
                        <span className="font-semibold">📦 {tool.displayName}</span>
                        {isBuiltin && <span className="text-xs bg-neutral-700 text-neutral-400 px-1.5 py-0.5 rounded">内置</span>}
                        <StatusBadge status={tool.status} />
                    </div>
                    <p className="text-xs text-neutral-500 mt-0.5">{tool.description} · {tool.category}</p>
                </div>
                <div className="flex gap-2">
                    {!isBuiltin && <button onClick={() => ExportTool(tool.id)} className="text-xs px-2 py-1 bg-neutral-700 rounded">导出</button>}
                    {!isBuiltin && <button className="text-xs px-2 py-1 bg-red-900/50 text-red-400 rounded">删除</button>}
                </div>
            </div>

            {/* Tab 切换 */}
            <div className="flex gap-4 px-4 border-b border-neutral-800 flex-shrink-0">
                {(['code', 'params', 'test'] as ToolTab[]).map(t => (
                    <button key={t} onClick={() => setTab(t)}
                        className={`py-2 text-sm ${tab === t ? 'border-b-2 border-blue-500 text-white' : 'text-neutral-400'}`}>
                        {{ code: '代码', params: '参数', test: '测试' }[t]}
                    </button>
                ))}
            </div>

            {/* Tab 内容 */}
            <div className="flex-1 overflow-hidden">
                {tab === 'code'   && <ToolCodeTab tool={tool} readonly={isBuiltin} onSave={setTool} />}
                {tab === 'params' && <ToolParamsTab params={tool.parameters} />}
                {tab === 'test'   && <ToolTestTab tool={tool} readonly={isBuiltin} />}
            </div>
        </div>
    )
}
```

### `ToolCodeTab.tsx`

```tsx
import Editor from '@monaco-editor/react'

export function ToolCodeTab({ tool, readonly, onSave }:
    { tool: Tool; readonly: boolean; onSave: (t: Tool) => void }) {
    const [editing, setEditing] = useState(false)
    const [code, setCode] = useState(tool.code)
    const [error, setError] = useState('')

    const save = async () => {
        try {
            await UpdateTool({ ...tool, code })
            const updated = await GetTool(tool.id)
            onSave(updated)
            setEditing(false)
            setError('')
        } catch (e: any) { setError(e.message) }
    }

    return (
        <div className="h-full flex flex-col p-4">
            {!readonly && (
                <div className="flex justify-end mb-2 gap-2">
                    {editing
                        ? <><button onClick={() => { setEditing(false); setCode(tool.code) }} className="text-sm px-3 py-1">取消</button>
                              <button onClick={save} className="text-sm px-3 py-1 bg-blue-600 rounded">保存</button></>
                        : <button onClick={() => setEditing(true)} className="text-sm px-3 py-1 bg-neutral-700 rounded">编辑</button>
                    }
                </div>
            )}
            {error && <p className="text-red-400 text-xs mb-2">{error}</p>}
            <div className="flex-1">
                <Editor height="100%" language="python" value={code}
                    onChange={v => setCode(v ?? '')}
                    options={{ readOnly: !editing, minimap: { enabled: false }, fontSize: 13 }}
                    theme="vs-dark" />
            </div>
        </div>
    )
}
```

### `ToolParamsTab.tsx`

```tsx
export function ToolParamsTab({ params }: { params: ToolParameter[] }) {
    if (!params.length) return <div className="p-4 text-sm text-neutral-500">无参数</div>
    return (
        <div className="p-4 overflow-y-auto">
            <table className="w-full text-sm">
                <thead>
                    <tr className="text-left text-xs text-neutral-500 border-b border-neutral-800">
                        <th className="pb-2 pr-4">参数名</th>
                        <th className="pb-2 pr-4">类型</th>
                        <th className="pb-2 pr-4">必填</th>
                        <th className="pb-2">说明</th>
                    </tr>
                </thead>
                <tbody>
                    {params.map(p => (
                        <tr key={p.name} className="border-b border-neutral-800/50">
                            <td className="py-2 pr-4 font-mono text-xs text-blue-300">{p.name}</td>
                            <td className="py-2 pr-4 text-neutral-400 text-xs">{p.type}</td>
                            <td className="py-2 pr-4 text-xs">{p.required ? '是' : '否'}</td>
                            <td className="py-2 text-xs text-neutral-400">{p.doc}</td>
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
    )
}
```

### `ToolTestTab.tsx`

```tsx
export function ToolTestTab({ tool, readonly }: { tool: Tool; readonly: boolean }) {
    const [values, setValues] = useState<Record<string, string>>({})
    const [history, setHistory] = useState<ToolTestRecord[]>([])
    const [running, setRunning] = useState(false)

    useEffect(() => { ListToolTestHistory(tool.id).then(setHistory) }, [tool.id])

    const run = async () => {
        setRunning(true)
        await RunTool(tool.id, values)
        const h = await ListToolTestHistory(tool.id)
        setHistory(h)
        setRunning(false)
    }

    return (
        <div className="h-full overflow-y-auto p-4 space-y-4">
            {!readonly && (
                <div className="space-y-3">
                    {tool.parameters.map(p => (
                        <div key={p.name}>
                            <label className="text-xs text-neutral-400 mb-1 block">{p.name} <span className="text-neutral-600">({p.type})</span></label>
                            <input value={values[p.name] ?? ''} onChange={e => setValues(v => ({ ...v, [p.name]: e.target.value }))}
                                className="w-full px-3 py-2 bg-neutral-800 rounded-lg text-sm" />
                        </div>
                    ))}
                    <button onClick={run} disabled={running}
                        className="w-full py-2 bg-blue-600 rounded-lg text-sm disabled:opacity-50">
                        {running ? '运行中...' : '▶ 运行测试'}
                    </button>
                </div>
            )}

            {history.length > 0 && (
                <div>
                    <p className="text-xs text-neutral-500 mb-2">最近测试记录</p>
                    {history.map(r => (
                        <div key={r.id} className="flex items-center gap-2 py-2 border-t border-neutral-800 text-xs">
                            <span>{r.passed ? '✅' : '❌'}</span>
                            <span className="text-neutral-400">{relativeTime(r.createdAt)}</span>
                            <span className="text-neutral-500">{r.durationMs}ms</span>
                            {r.errorMsg && <span className="text-red-400 truncate flex-1">{r.errorMsg}</span>}
                        </div>
                    ))}
                </div>
            )}
        </div>
    )
}
```

---

## 5. 验收测试

```
1. 点击工具 → Tool Tab 或 Chat+Tool 右侧出现 ToolMainView，顶部信息正确
2. 代码 Tab：语法高亮正确；点"编辑"→ 可修改；保存有语法检查
3. 语法错误代码点保存 → 显示错误信息，不保存
4. 参数 Tab：显示从代码签名解析的所有参数
5. 测试 Tab：填参数 → 运行 → 结果显示在历史列表
6. 内置工具：编辑/删除按钮不存在，代码只读
7. 测试历史最多 20 条，按时间倒序
```
