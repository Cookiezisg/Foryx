import { create } from "zustand";
import type { Block, Message, MessageRole } from "@frontend/entities/conversation/model/types";

// Local extension: SSE delivers blocks that we render as a nested tree.
// Block.children in the frontend type is string[] (IDs); here we materialise
// the actual children for in-memory rendering in the testend chat view.
//
// 本地扩展:frontend Block.children 是 string[](ID);这里在内存里存渲染树。
export type BlockNode = Omit<Block, "children"> & { children: BlockNode[] };

// Message augmented with materialised BlockNode tree — used by ChatPanel.
// Exported so layout components can annotate their local variables correctly.
export type MessageNode = Omit<Message, "blocks"> & { blocks: BlockNode[] };

type ChatMessage = MessageNode;

interface ConvState {
  messages: ChatMessage[];
}

interface State {
  byConv: Record<string, ConvState>;
  ensureConv: (convId: string) => void;
  setMessages: (convId: string, messages: Message[]) => void;
  onMessageStart: (convId: string, msg: Partial<Message> & { id: string }) => void;
  onMessageStop: (convId: string, msgId: string, patch: Partial<Message>) => void;
  onBlockStart: (convId: string, blk: Partial<Block> & { id: string; messageId: string; parentId?: string }) => void;
  onBlockDelta: (convId: string, blkId: string, delta: string) => void;
  onBlockStop: (convId: string, blkId: string, patch: Partial<Block>) => void;
  reset: (convId: string) => void;
}

// Converts an incoming partial Block (SSE or REST) to a BlockNode.
// Drops children: string[] from Block, fills in required defaults, replaces with BlockNode[].
function toNode(blk: Partial<Block> & { id: string }): BlockNode {
  const { children: _drop, ...rest } = blk as Block;
  void _drop;
  // Spread rest last so incoming fields win over defaults; children always [].
  const defaults: Omit<BlockNode, "id"> = {
    messageId: "", parentId: "", type: "text", attrs: null,
    content: "", status: "streaming", durationMs: null,
    error: null, version: 0, children: [],
  };
  return { ...defaults, ...rest, children: [] };
}

function messageToChat(m: Message): ChatMessage {
  const { blocks, ...rest } = m;
  return { ...rest, blocks: blocks.map(toNode) };
}

export const useChatStore = create<State>((set, get) => ({
  byConv: {},
  ensureConv: (convId) => {
    if (!get().byConv[convId]) {
      set((s) => ({ byConv: { ...s.byConv, [convId]: { messages: [] } } }));
    }
  },
  setMessages: (convId, messages) =>
    set((s) => ({ byConv: { ...s.byConv, [convId]: { messages: messages.map(messageToChat) } } })),
  onMessageStart: (convId, msg) =>
    set((s) => {
      const cur = s.byConv[convId] ?? { messages: [] };
      const { blocks: _drop, ...msgRest } = msg as Message;
      void _drop;
      const defaults: Omit<ChatMessage, "id"> = {
        conversationId: convId,
        role: "assistant" as MessageRole,
        status: "streaming",
        parentBlockId: null,
        attachments: [],
        createdAt: new Date().toISOString(),
        blocks: [],
      };
      const next: ChatMessage = { ...defaults, ...msgRest, blocks: [] };
      return { byConv: { ...s.byConv, [convId]: { messages: [...cur.messages, next] } } };
    }),
  onMessageStop: (convId, msgId, patch) =>
    set((s) => {
      const cur = s.byConv[convId];
      if (!cur) return s;
      const { blocks: _drop, ...patchRest } = patch as Message;
      void _drop;
      return {
        byConv: {
          ...s.byConv,
          [convId]: {
            messages: cur.messages.map((m) => (m.id === msgId ? { ...m, ...patchRest } : m)),
          },
        },
      };
    }),
  onBlockStart: (convId, blk) =>
    set((s) => {
      const cur = s.byConv[convId];
      if (!cur) return s;
      const next = toNode(blk);
      const messages = cur.messages.map((m) => {
        if (m.id !== blk.messageId) return m;
        if (!blk.parentId || blk.parentId === m.id) {
          return { ...m, blocks: [...m.blocks, next] };
        }
        return { ...m, blocks: nestBlock(m.blocks, blk.parentId, next) };
      });
      return { byConv: { ...s.byConv, [convId]: { messages } } };
    }),
  onBlockDelta: (convId, blkId, delta) =>
    set((s) => {
      const cur = s.byConv[convId];
      if (!cur) return s;
      return {
        byConv: {
          ...s.byConv,
          [convId]: {
            messages: cur.messages.map((m) => ({
              ...m,
              blocks: appendDelta(m.blocks, blkId, delta),
            })),
          },
        },
      };
    }),
  onBlockStop: (convId, blkId, patch) =>
    set((s) => {
      const cur = s.byConv[convId];
      if (!cur) return s;
      return {
        byConv: {
          ...s.byConv,
          [convId]: {
            messages: cur.messages.map((m) => ({
              ...m,
              blocks: patchBlock(m.blocks, blkId, patch),
            })),
          },
        },
      };
    }),
  reset: (convId) =>
    set((s) => {
      const { [convId]: _, ...rest } = s.byConv;
      void _;
      return { byConv: rest };
    }),
}));

function nestBlock(blocks: BlockNode[], parentId: string, child: BlockNode): BlockNode[] {
  return blocks.map((b) => {
    if (b.id === parentId) return { ...b, children: [...b.children, child] };
    if (b.children.length > 0) return { ...b, children: nestBlock(b.children, parentId, child) };
    return b;
  });
}

function appendDelta(blocks: BlockNode[], id: string, delta: string): BlockNode[] {
  return blocks.map((b) => {
    if (b.id === id) return { ...b, content: b.content + delta };
    if (b.children.length > 0) return { ...b, children: appendDelta(b.children, id, delta) };
    return b;
  });
}

function patchBlock(blocks: BlockNode[], id: string, patch: Partial<Block>): BlockNode[] {
  // Strip children: string[] from the Block patch to avoid overwriting BlockNode children.
  const { children: _drop, ...safePatch } = patch as Block;
  void _drop;
  return blocks.map((b) => {
    if (b.id === id) return { ...b, ...safePatch };
    if (b.children.length > 0) return { ...b, children: patchBlock(b.children, id, patch) };
    return b;
  });
}
