# E3 · 基础节点类型 — 技术设计文档

**切片**：E3  
**状态**：待 Review

---

## 1. 节点组件结构

每个节点类型 = React Flow 自定义节点 + 对应的配置面板组件。

```
frontend/src/components/workflow/nodes/
├── TriggerNode.tsx        # 触发节点（4 种触发类型共用）
├── ToolNode.tsx           # 工具节点
├── ConditionNode.tsx      # 条件节点（带多出线 handle）
├── VariableNode.tsx       # 变量节点
└── ApprovalNode.tsx       # 人工确认节点

frontend/src/components/workflow/panels/
├── TriggerConfigPanel.tsx
├── ToolConfigPanel.tsx
├── ConditionConfigPanel.tsx
├── VariableConfigPanel.tsx
└── ApprovalConfigPanel.tsx
```

---

## 2. 节点组件示例

### `ToolNode.tsx`

```tsx
import { Handle, Position, NodeProps } from 'reactflow'
import { BaseNode } from './BaseNode'

export function ToolNode({ data, selected }: NodeProps) {
    const missing = !data.config?.tool_name
    return (
        <BaseNode data={data} selected={selected}>
            <Handle type="target" position={Position.Left} />
            <div className="flex items-center gap-2">
                <span>{missing ? '⚠️' : '📦'}</span>
                <div>
                    <p className="text-sm font-medium">
                        {missing ? '[缺失] 未配置工具' : data.config.tool_name}
                    </p>
                    {missing && <p className="text-xs text-yellow-400">点击配置工具</p>}
                </div>
            </div>
            <Handle type="source" position={Position.Right} />
        </BaseNode>
    )
}
```

### `ConditionNode.tsx`

```tsx
export function ConditionNode({ data, selected }: NodeProps) {
    const branches = data.config?.branches ?? ['yes', 'no']
    return (
        <BaseNode data={data} selected={selected}>
            <Handle type="target" position={Position.Left} />
            <div>
                <p className="text-sm font-medium">⑆ 条件</p>
                <p className="text-xs text-neutral-400 mt-1 max-w-[140px] truncate">
                    {data.config?.expression || '未配置条件'}
                </p>
            </div>
            {/* 每个分支一个 source handle */}
            {branches.map((b: string, i: number) => (
                <Handle key={b} type="source" position={Position.Right}
                    id={b} style={{ top: `${(i + 1) * 100 / (branches.length + 1)}%` }}
                />
            ))}
        </BaseNode>
    )
}
```

---

## 3. 配置面板示例

### `ToolConfigPanel.tsx`

```tsx
export function ToolConfigPanel({ node, onChange }: { node: Node; onChange: (n: Node) => void }) {
    const [tools, setTools] = useState<Tool[]>([])
    const config = node.data.config ?? {}

    useEffect(() => { ListTools('', '').then(setTools) }, [])

    const update = (key: string, value: any) => {
        onChange({ ...node, data: { ...node.data, config: { ...config, [key]: value } } })
    }

    return (
        <div className="p-4 space-y-3">
            <h3 className="text-sm font-semibold">工具节点配置</h3>

            {/* 工具选择 */}
            <div>
                <label className="text-xs text-neutral-400 mb-1 block">选择工具</label>
                <select value={config.tool_name ?? ''} onChange={e => update('tool_name', e.target.value)}
                    className="w-full px-3 py-2 bg-neutral-800 rounded text-sm">
                    <option value="">选择工具...</option>
                    {tools.map(t => <option key={t.id} value={t.name}>{t.displayName}</option>)}
                </select>
            </div>

            {/* 参数绑定 */}
            {config.tool_name && tools.find(t => t.name === config.tool_name)?.parameters.map(p => (
                <div key={p.name}>
                    <label className="text-xs text-neutral-400 mb-1 block">
                        {p.name} <span className="text-neutral-600">({p.type})</span>
                    </label>
                    <input
                        value={config.params?.[p.name] ?? ''}
                        onChange={e => update('params', { ...config.params, [p.name]: e.target.value })}
                        placeholder={`值或 {{node_id.result.field}}`}
                        className="w-full px-3 py-2 bg-neutral-800 rounded text-sm font-mono"
                    />
                </div>
            ))}
        </div>
    )
}
```

---

## 4. 触发节点的 Cron 可视化编辑

```tsx
// TriggerConfigPanel.tsx 内的 Cron 编辑器
const CRON_PRESETS = [
    { label: '每天 09:00', cron: '0 9 * * *' },
    { label: '每小时', cron: '0 * * * *' },
    { label: '每周一 09:00', cron: '0 9 * * 1' },
    { label: '自定义...', cron: '' },
]

function CronEditor({ value, onChange }: { value: string; onChange: (c: string) => void }) {
    const preset = CRON_PRESETS.find(p => p.cron === value)
    return (
        <div className="space-y-2">
            <div className="flex gap-2 flex-wrap">
                {CRON_PRESETS.map(p => (
                    <button key={p.label} onClick={() => p.cron && onChange(p.cron)}
                        className={`text-xs px-2 py-1 rounded ${value === p.cron ? 'bg-blue-600' : 'bg-neutral-700'}`}>
                        {p.label}
                    </button>
                ))}
            </div>
            <input value={value} onChange={e => onChange(e.target.value)}
                placeholder="0 9 * * *"
                className="w-full px-3 py-2 bg-neutral-800 rounded text-sm font-mono" />
            {value && <p className="text-xs text-neutral-400">{describeCron(value)}</p>}
        </div>
    )
}
```

---

## 5. 验收测试

```
1. 触发节点：选"定时触发" → Cron 编辑器出现 → 选预设"每天 09:00" → cron 字段更新
2. 工具节点：选 gmail_read → 参数列表出现 → 填写值
3. 工具节点：不选工具 → 节点显示黄色 ⚠️
4. 条件节点：填写表达式 → 节点显示两个出线 handle
5. 人工确认节点：配置标题和消息 → 正确保存
6. 所有节点配置后保存到 FlowDefinition
```
