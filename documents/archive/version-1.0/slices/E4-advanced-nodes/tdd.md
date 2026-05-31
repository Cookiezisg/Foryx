# E4 · 高级节点类型 — 技术设计文档

**切片**：E4  
**状态**：待 Review

---

## 1. 节点组件

```
frontend/src/components/workflow/nodes/
├── LoopNode.tsx
├── ParallelNode.tsx
└── SubworkflowNode.tsx

frontend/src/components/workflow/panels/
├── LoopConfigPanel.tsx
├── ParallelConfigPanel.tsx
└── SubworkflowConfigPanel.tsx
```

---

## 2. 循环节点 (Loop)

### `LoopNode.tsx`

```tsx
export function LoopNode({ data, selected }: NodeProps) {
    return (
        <BaseNode data={data} selected={selected}>
            <Handle type="target" position={Position.Left} />
            <div>
                <p className="text-sm font-medium">🔄 循环</p>
                <p className="text-xs text-neutral-400">遍历 {data.config?.items_expr || '未配置'}</p>
            </div>
            {/* loop_body：进入循环体 */}
            <Handle type="source" position={Position.Bottom} id="loop_body" />
            {/* loop_done：循环结束后继续 */}
            <Handle type="source" position={Position.Right} id="loop_done" />
        </BaseNode>
    )
}
```

### 循环节点在 Go 执行层的处理

循环节点有两个特殊出线 handle：
- `loop_body`：连到循环体第一个节点
- `loop_done`：连到循环结束后的下游节点

执行层（F1 负责）识别这个模式，将 `loop_body` 路径视为子图，反复执行。

---

## 3. 并行节点 (Parallel)

### `ParallelNode.tsx`

```tsx
export function ParallelNode({ data, selected }: NodeProps) {
    const branches = data.config?.branch_count ?? 2
    return (
        <BaseNode data={data} selected={selected}>
            <Handle type="target" position={Position.Left} />
            <div>
                <p className="text-sm font-medium">⟰ 并行</p>
                <p className="text-xs text-neutral-400">{branches} 个分支</p>
            </div>
            {Array.from({ length: branches }, (_, i) => (
                <Handle key={i} type="source" position={Position.Right}
                    id={`branch_${i}`}
                    style={{ top: `${(i + 1) * 100 / (branches + 1)}%` }} />
            ))}
        </BaseNode>
    )
}
```

### 汇合节点 (Join)

并行节点总是与一个 Join 节点配对（隐式创建）：
```tsx
export function JoinNode({ data, selected }: NodeProps) {
    return (
        <BaseNode data={data} selected={selected}>
            {/* 多个 target handle */}
            <Handle type="target" position={Position.Left} id="join_all" />
            <p className="text-sm">⟰ 汇合</p>
            <Handle type="source" position={Position.Right} />
        </BaseNode>
    )
}
```

---

## 4. 子工作流节点 (Subworkflow)

```tsx
export function SubworkflowNode({ data, selected }: NodeProps) {
    return (
        <BaseNode data={data} selected={selected}>
            <Handle type="target" position={Position.Left} />
            <div>
                <p className="text-sm font-medium">⊞ 子工作流</p>
                <p className="text-xs text-neutral-400">{data.config?.workflow_name || '未选择'}</p>
            </div>
            <Handle type="source" position={Position.Right} />
        </BaseNode>
    )
}
```

---

## 5. 验收测试

```
1. 循环节点：配置 items_expr="{{email_list}}" → 连接到子流程 → 运行时每个邮件执行一次
2. 并行节点：2个分支各自完成 → 汇合后继续
3. 子工作流节点：选择已有工作流 → 传参 → 运行时调用子工作流
4. 循环节点超出最大迭代次数（100次）时停止并报错
```
