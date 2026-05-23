// Chat store — message/block tree built from eventlog SSE.
//
// State shape (per conversation):
//   messages: Map<msgId, Message{id, role, status, blocks: [blockId..], ...}>
//   blocks:   Map<blockId, Block{id, parentId, messageId, type, content, status, ...}>
//   topMsgIds: msgId[] — top-level messages in arrival order (subagent
//              inner messages live nested under message-type blocks).
//
// Algorithms:
//   onMessageStart: insert into messages; if parentBlockId given, the
//     message is nested under that block's children (the parent block
//     should already exist as type="message"). Otherwise it joins
//     topMsgIds.
//   onBlockStart: insert into blocks; attach to either parent
//     message.blocks[] or parent block.children[].
//   onBlockDelta: append delta to block.content; bump version.
//   onBlockStop: set block.status + durationMs; final cleanup.
//   onMessageStop: set message.status + token counts.
//
// 设计目标：tree 增量更新，TextBlock/ToolCallBlock 等 React 组件按 id
// memo，单 delta 只重渲染对应 block，不会重渲染整个对话。

import { create } from "zustand";

function emptyConv() {
  return { messages: new Map(), blocks: new Map(), topMsgIds: [], lastSeq: 0 };
}

// ── rAF delta coalescing ─────────────────────────────────────────────
// Backend streams can fire 30-100 deltas/sec per block. Without
// batching, each delta is one setState → one React render → one full
// reconcile + highlight pass. For long messages with code blocks this
// saturates the main thread and locks the tab.
//
// We buffer deltas and flush them once per animation frame. Visible
// effect: text appears in 16ms steps instead of continuously —
// imperceptible at 60Hz, but caps render rate so the main thread
// stays responsive.
//
// block_stop / message_stop must flush synchronously so terminal state
// is never applied before the deltas that preceded it.
//
// rAF 合并 delta —— 一帧最多 setState 一次；终态事件先冲洗 buffer 再 set。
let pendingDeltas = []; // [{convId, blockId, delta}]
let rafHandle = 0;
let storeApiRef = null; // captured below

function scheduleFlush() {
  if (rafHandle) return;
  if (typeof requestAnimationFrame === "undefined") {
    rafHandle = setTimeout(() => { rafHandle = 0; flushDeltas(); }, 16);
    return;
  }
  rafHandle = requestAnimationFrame(() => { rafHandle = 0; flushDeltas(); });
}

function cancelFlush() {
  if (!rafHandle) return;
  if (typeof cancelAnimationFrame !== "undefined") cancelAnimationFrame(rafHandle);
  else clearTimeout(rafHandle);
  rafHandle = 0;
}

function flushDeltas() {
  const items = pendingDeltas;
  if (items.length === 0) return;
  pendingDeltas = [];

  const byConv = new Map();
  for (const it of items) {
    let m = byConv.get(it.convId);
    if (!m) { m = new Map(); byConv.set(it.convId, m); }
    m.set(it.blockId, (m.get(it.blockId) || "") + it.delta);
  }

  storeApiRef.setState((s) => {
    const convs = { ...s.convs };
    for (const [convId, blockDeltas] of byConv) {
      const conv = convs[convId];
      if (!conv) continue;
      const blocks = new Map(conv.blocks);
      for (const [blockId, delta] of blockDeltas) {
        const cur = blocks.get(blockId);
        if (!cur) continue;
        blocks.set(blockId, {
          ...cur,
          content: cur.content + delta,
          version: cur.version + 1,
        });
      }
      convs[convId] = { ...conv, blocks };
    }
    return { convs };
  });
}

function flushNow() {
  cancelFlush();
  flushDeltas();
}

export const useChatStore = create((set, get) => ({
  convs: {},

  ensureConv(convId) {
    const state = get();
    if (state.convs[convId]) return;
    set((s) => ({ convs: { ...s.convs, [convId]: emptyConv() } }));
  },

  resetConv(convId) {
    set((s) => ({ convs: { ...s.convs, [convId]: emptyConv() } }));
  },

  resetAll() {
    set({ convs: {} });
  },

  // Hydrate from REST history. Backend exposes blocks as a flat list per
  // message with `parentBlockId` (omitted when parent is the message
  // itself). We rebuild the tree: blocks whose parentBlockId targets an
  // existing block become its children; the rest become top-level
  // message blocks. Safe to call on conv switch / 410 recovery.
  //
  // REST 给扁平 block 列表 + 顶层 parentBlockId 省略；按 parentBlockId 重
  // 建树（subagent 嵌套同理）。
  hydrateConv(convId, messages) {
    const conv = emptyConv();

    const installMessage = (m, parentBlockId) => {
      conv.messages.set(m.id, {
        id: m.id,
        role: m.role,
        status: m.status || "completed",
        createdAt: m.createdAt,
        stopReason: m.stopReason || null,
        inputTokens: m.inputTokens ?? null,
        outputTokens: m.outputTokens ?? null,
        model: m.model || null,
        parentBlockId: parentBlockId || null,
        blocks: [],
        attachments: m.attachments || [],
        attrs: m.attrs || null,
      });
      if (!parentBlockId) conv.topMsgIds.push(m.id);

      for (const b of m.blocks || []) {
        const parentId = b.parentBlockId || b.parentId || m.id;
        conv.blocks.set(b.id, {
          id: b.id,
          messageId: m.id,
          parentId,
          type: b.type,
          attrs: b.attrs || null,
          content: b.content || "",
          status: b.status || "completed",
          durationMs: b.durationMs ?? null,
          error: b.error || null,
          children: [],
          version: 0,
        });
      }
      // Wire children after all blocks exist so parent lookups succeed.
      for (const b of m.blocks || []) {
        const block = conv.blocks.get(b.id);
        const parent = conv.blocks.get(block.parentId);
        if (parent) {
          parent.children.push(b.id);
        } else if (conv.messages.has(m.id)) {
          conv.messages.get(m.id).blocks.push(b.id);
        }
      }

      // subagent: nested message lives off a message-type block via
      // attrs.messageId. Recurse if backend embeds it.
      for (const b of m.blocks || []) {
        if (b.type === "message" && b.innerMessage) {
          installMessage(b.innerMessage, b.id);
        }
      }
    };

    for (const m of messages || []) installMessage(m, null);
    set((s) => ({ convs: { ...s.convs, [convId]: conv } }));
  },

  // ── SSE handlers ───────────────────────────────────────────────────
  onMessageStart(convId, e) {
    set((s) => {
      const conv = s.convs[convId] || emptyConv();
      if (conv.messages.has(e.id)) return s; // dedupe
      const messages = new Map(conv.messages);
      const blocks = new Map(conv.blocks);
      const parentBlockId = e.parentBlockId || null;

      messages.set(e.id, {
        id: e.id,
        role: e.role,
        status: "streaming",
        createdAt: new Date().toISOString(),
        stopReason: null,
        inputTokens: null,
        outputTokens: null,
        model: null,
        parentBlockId,
        blocks: [],
        attachments: [],
        attrs: e.attrs || null,
      });

      let topMsgIds = conv.topMsgIds;
      if (parentBlockId && blocks.has(parentBlockId)) {
        // Nest the message-id under the placeholder message block so
        // SubagentBlock can find it.
        const parent = { ...blocks.get(parentBlockId) };
        parent.attrs = { ...(parent.attrs || {}), messageId: e.id };
        blocks.set(parentBlockId, parent);
      } else {
        topMsgIds = [...conv.topMsgIds, e.id];
      }

      return { convs: { ...s.convs, [convId]: { ...conv, messages, blocks, topMsgIds } } };
    });
  },

  onMessageStop(convId, e) {
    flushNow();
    set((s) => {
      const conv = s.convs[convId];
      if (!conv) return s;
      const cur = conv.messages.get(e.id);
      if (!cur) return s;
      const messages = new Map(conv.messages);
      messages.set(e.id, {
        ...cur,
        status: e.status || "completed",
        stopReason: e.stopReason || cur.stopReason,
        inputTokens: e.inputTokens ?? cur.inputTokens,
        outputTokens: e.outputTokens ?? cur.outputTokens,
      });
      return { convs: { ...s.convs, [convId]: { ...conv, messages } } };
    });
  },

  onBlockStart(convId, e) {
    set((s) => {
      const conv = s.convs[convId] || emptyConv();
      if (conv.blocks.has(e.id)) return s;

      const blocks = new Map(conv.blocks);
      const messages = new Map(conv.messages);

      const parentId = e.parentId || e.messageId;
      const messageId = e.messageId;

      blocks.set(e.id, {
        id: e.id,
        messageId,
        parentId,
        type: e.blockType,
        attrs: e.attrs || null,
        content: "",
        status: "streaming",
        durationMs: null,
        error: null,
        children: [],
        version: 0,
      });

      // attach to parent's child list
      if (parentId === messageId && messages.has(messageId)) {
        const msg = { ...messages.get(messageId), blocks: [...messages.get(messageId).blocks, e.id] };
        messages.set(messageId, msg);
      } else if (blocks.has(parentId)) {
        const parent = blocks.get(parentId);
        const updated = { ...parent, children: [...parent.children, e.id] };
        blocks.set(parentId, updated);
      }

      return { convs: { ...s.convs, [convId]: { ...conv, blocks, messages } } };
    });
  },

  onBlockDelta(convId, e) {
    if (!e?.id) return;
    pendingDeltas.push({ convId, blockId: e.id, delta: e.delta || "" });
    scheduleFlush();
  },

  onBlockStop(convId, e) {
    flushNow();
    set((s) => {
      const conv = s.convs[convId];
      if (!conv) return s;
      const cur = conv.blocks.get(e.id);
      if (!cur) return s;
      const blocks = new Map(conv.blocks);
      blocks.set(e.id, {
        ...cur,
        status: e.status || "completed",
        error: e.error || cur.error,
        durationMs: e.durationMs ?? cur.durationMs,
      });
      return { convs: { ...s.convs, [convId]: { ...conv, blocks } } };
    });
  },
}));

// Wire the rAF batcher to the live store handle (created above).
storeApiRef = useChatStore;

// Selectors MUST return stable references between snapshots —
// useSyncExternalStore (under zustand) sees a "new" value otherwise and
// infinite-loops. So we return IDs (which the store mutates by
// allocating new arrays on real changes only) and let consumers map to
// blocks via per-id selectors that already use `===` equality.
//
// selector 必须返稳定引用——zustand 的 useSyncExternalStore 否则会死循环。
// 这里返 ID 数组（store 改变时才换新引用），消费者按需用 selectBlock 拿
// 具体 block。
const EMPTY_IDS = Object.freeze([]);

export function selectTopMessageIds(convId, state) {
  return state.convs[convId]?.topMsgIds || EMPTY_IDS;
}
export function selectBlock(convId, blockId, state) {
  return state.convs[convId]?.blocks.get(blockId) || null;
}
export function selectChildIds(convId, parentId, state) {
  return state.convs[convId]?.blocks.get(parentId)?.children || EMPTY_IDS;
}
