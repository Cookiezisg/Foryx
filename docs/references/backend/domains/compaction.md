---
id: DOC-105
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Compaction Domain — 对话上下文压缩与 Token 治理

> **核心职责**：Compaction 是 Forgify 的 **“内存整理器”**。它负责监控对话线程的 Token 消耗，当上下文接近 LLM 窗口极限时，通过自动摘要技术将历史 Blocks 压缩，确保对话长效不中断。

---

## 1. 物理模型 (Data Anatomy)

Compaction 无独立主表，它直接驱动 `message_blocks` 的状态流转。

### 1.1 `CompactionEvent` (逻辑载体)
由 `Eventlog` 发射的物理记录。
```typescript
interface CompactionPayload {
    coversFromSeq: number; // 压缩起始 Seq
    coversToSeq: number;   // 压缩截止 Seq
    summary: string;       // 产生的摘要文本
    blocksArchived: number; // 被归档的原子块数量
}
```

---

## 2. 核心原理 (Principles)

### 2.1 Anchored FIFO (锚点式先进先出)
Forgify 不简单切除旧消息，而是采用 **“摘要替换”**：
1. **监控**：`ContextCompactor` 在每回合结束后计算累计 Token。
2. **触发**：当消耗 > 阈值（默认 80% 窗口）时，触发压缩。
3. **摘要生成**：调用 **Utility 档** 模型，对 `[fromSeq, toSeq]` 范围的内容生成一段高密度摘要。
4. **状态投影**：将该范围内所有 Block 的 `ContextRole` 标为 `archived`。
5. **插入锚点**：在物理层插入一个 `type="compaction"` 的新 Block，内容为生成的摘要。

### 2.2 Calibration (消耗校准)
- **挑战**：后端通过 `tiktoken` 估算的 Token 数通常与供应商实际计算的有误差。
- **方案**：`Calibrate` 接口接受 LLM 返回的真实 `usage` 数据。系统会根据真实值动态调整 `MaybeCompact` 的触发水位，防止过度压缩。

### 2.3 Non-Destructive Archiving (非破坏性归档)
- **原理**：压缩过程不物理删除 `message_blocks` 中的行。
- **优点**：用户在 UI 侧向上滚动查看历史时，前端依然可以拉取到原始内容；仅在 LLM 生成时，后端根据 `ContextRole` 过滤掉这些行。

---

## 3. 生命周期 (Lifecycle)

1. **监控 (Observing)**：Chat 回合结束，读取消息累计 Tokens。
2. **决策 (Thresholding)**：水位触线 -> 启动压缩协程。
3. **合成 (Summarizing)**：向 LLM 投递压缩指令。
4. **原子广播 (Publishing)**：
   - 插入 `compaction` 块。
   - 更新旧块 `ContextRole`。
   - 通过 `notifications` 发布压缩完成事件。
5. **生效 (Applying)**：下一轮对话请求，旧块被物理过滤，摘要进入上下文。

---

## 4. 跨域集成 (Interactions)

- **Chat**：压缩逻辑的触发宿主。
- **Model**：决定压缩任务分发给哪个低成本模型（Utility scenario）。
- **Eventlog**：通过 `compaction` 块类型在时间线上留下痕迹。

---

## 5. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 备注 |
|---|---|---|---|
| `ErrCompactionFailed`| 500 | `COMPACTION_ERROR` | LLM 生成摘要失败，跳过本次压缩。 |
| `ErrCalibrationMismatch`| - | - | 内部日志：真实消耗与预估误差过大。 |
| `ErrSeqOverlap` | 400 | `INVALID_REQUEST` | 尝试压缩一个已经被压缩过的区间。 |
| `ErrNoActiveModel` | 422 | `MODEL_NOT_CONFIGURED`| 没配 Utility 模型，无法执行压缩。 |
