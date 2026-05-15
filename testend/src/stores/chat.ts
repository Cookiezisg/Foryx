/**
 * Chat store — message stream for the currently selected conversation.
 *
 * Subscribes to the eventlog SSE on selection, demuxes events by
 * conversationId, and merges them into an in-memory message+block map
 * that the ChatPanel renders.
 *
 * Event model (see event-log-protocol.md):
 *   message_start  →  add new message, status="streaming"
 *   block_start    →  add new block under message (parentId could be
 *                     message id [top-level] or another block id [nested
 *                     under e.g. a tool_call])
 *   block_delta    →  append delta to block.content
 *   block_stop     →  flip block.status to terminal
 *   message_stop   →  flip message.status to terminal + stamp tokens
 */

import { defineStore } from 'pinia';
import { computed, ref, watch } from 'vue';
import { convAPI } from '@/api/conversations';
import { subscribe } from '@/api/sse';
import type { Block, Message } from '@/types/domain';
import { useConvStore } from './conv';
import { useUIStore } from './ui';

/**
 * Wire envelope per `domain/eventlog/eventlog.go` — exactly the JSON of the
 * Go struct for each event. Event type drives interpretation of `id`:
 *   - message_start / message_stop:  id = message id
 *   - block_start / block_delta / block_stop: id = block id
 *
 * The SSE seq lives in the outer StreamEvent (from `ev.lastEventId`), not
 * in the payload.
 */
interface StreamEnvelope {
  conversationId: string;
  id: string;
  /** block_start only */
  parentId?: string;
  /** block_start only — convenience: top-level message the block belongs to */
  messageId?: string;
  /** block_start only — one of 6 BlockTypes */
  blockType?: string;
  /** message_stop / block_stop terminal value */
  status?: string;
  /** block_delta only */
  delta?: string;
  attrs?: Record<string, unknown>;
  /** block_stop only (when status=error) */
  error?: string;
  /** message_stop only */
  stopReason?: string;
  errorCode?: string;
  errorMessage?: string;
  inputTokens?: number;
  outputTokens?: number;
  /** message_start only */
  role?: 'user' | 'assistant' | 'system';
  parentBlockId?: string;
}

export const useChatStore = defineStore('chat', () => {
  const conv = useConvStore();
  const ui = useUIStore();

  /** convId → messages (ordered) */
  const messagesByConv = ref<Record<string, Message[]>>({});
  /** convId → blockId → Block (flat for fast lookup; render order via message.blocks) */
  const blocksByConv = ref<Record<string, Record<string, Block>>>({});
  /** Raw eventlog SSE events (last RAW_MAX, all convs) — feeds CURRENT › Eventlog raw view. */
  const rawEvents = ref<Array<{ event: string; seq: number; convId: string; data: unknown; at: number }>>([]);

  const loadingMessages = ref(false);
  const sending = ref(false);
  const streaming = computed(() => {
    const m = currentMessages.value;
    return m.some((x) => x.status === 'streaming' || x.status === 'pending');
  });

  const currentMessages = computed<Message[]>(() => {
    if (!conv.selectedId) return [];
    return messagesByConv.value[conv.selectedId] ?? [];
  });

  /**
   * pendingAsk — the single open AskUserQuestion tool_call in the current
   * conversation, or null. A tool_call block counts as "pending ask" when:
   *  - block.type === "tool_call"
   *  - block.attrs.toolName === "AskUserQuestion"
   *  - block.status === "streaming" (still waiting for answer)
   *  - no sibling tool_result child (the answer hasn't closed it)
   *
   * Composer reads this to switch to AWAITING_ANSWER mode (route Send to
   * POST /answers, show banner / skip button).
   *
   * pendingAsk — 当前 conv 唯一未答的 AskUserQuestion tool_call(无则 null)。
   * Composer 看它切到"答题模式"。
   */
  const pendingAsk = computed<{ block: Block; toolCallId: string; question: string; options: string[] } | null>(() => {
    if (!conv.selectedId) return null;
    const blkMap = blocksByConv.value[conv.selectedId] ?? {};
    const blocks = Object.values(blkMap);
    for (const b of blocks) {
      if (b.type !== 'tool_call') continue;
      const a = (b.attrs ?? {}) as Record<string, unknown>;
      // Tool name: SSE event uses attrs.toolName; legacy DB store may store
      // attrs.tool. Check both for robustness.
      // SSE 用 attrs.toolName;旧存档可能是 attrs.tool。兼容查两个。
      const toolName = (a.toolName as string) || (a.tool as string) || '';
      if (toolName !== 'AskUserQuestion') continue;
      if (b.status !== 'streaming') continue;
      // No tool_result child means still pending.
      const hasResult = blocks.some(x => x.type === 'tool_result' && x.parentBlockId === b.id);
      if (hasResult) continue;
      // Parse question + options from block.content (LLM-emitted args JSON).
      let question = '';
      let options: string[] = [];
      try {
        const parsed = JSON.parse(b.content || '{}');
        question = String(parsed.question ?? '');
        if (Array.isArray(parsed.options)) {
          options = parsed.options.map(String);
        }
      } catch {
        // Malformed args — surface block id at least
        question = `(unparseable question for ${b.id})`;
      }
      const toolCallId = (a.toolCallId as string) || b.id;
      return { block: b, toolCallId, question, options };
    }
    return null;
  });

  /* ─────── load REST history when conv changes ─────── */

  async function loadMessages(convId: string) {
    loadingMessages.value = true;
    try {
      const page = await convAPI.messages(convId, 200);
      const msgs = page.items.sort((a, b) => a.createdAt.localeCompare(b.createdAt));
      // 2026-05: backend Block.Attrs / Message.Attrs is now map[string]any
      // via GORM serializer:json — REST returns objects (matching SSE wire).
      // No client-side parsing workaround needed.
      messagesByConv.value[convId] = msgs;
      const blkMap: Record<string, Block> = {};
      for (const m of msgs) {
        if (m.blocks) {
          for (const b of m.blocks) blkMap[b.id] = b;
        }
      }
      blocksByConv.value[convId] = blkMap;
    } catch (e) {
      ui.toast('err', `加载消息失败: ${(e as Error).message}`);
    } finally {
      loadingMessages.value = false;
    }
  }

  watch(
    () => conv.selectedId,
    (id) => {
      if (id) loadMessages(id);
    },
    { immediate: true },
  );

  /* ─────── SSE subscription (per-user → client-side demux by convId) ─────── */

  let unsub: (() => void) | null = null;

  function startSSE() {
    if (unsub) return;
    unsub = subscribe('eventlog', (ev) => {
      const data = ev.data as StreamEnvelope;
      if (!data || !data.conversationId) return;
      // Raw capture (ring buffer) for the "Eventlog raw" view.
      rawEvents.value.unshift({
        event: ev.event,
        seq: ev.id,
        convId: data.conversationId,
        data,
        at: ev.receivedAt,
      });
      if (rawEvents.value.length > 2000) rawEvents.value.length = 2000;
      onEvent(ev.event, data, ev.id);
    });
  }

  function stopSSE() {
    if (unsub) {
      unsub();
      unsub = null;
    }
  }

  function ensureMsgs(convId: string): Message[] {
    if (!messagesByConv.value[convId]) {
      messagesByConv.value[convId] = [];
      blocksByConv.value[convId] = {};
    }
    return messagesByConv.value[convId];
  }

  function ensureBlocks(convId: string): Record<string, Block> {
    if (!blocksByConv.value[convId]) blocksByConv.value[convId] = {};
    return blocksByConv.value[convId];
  }

  function onEvent(name: string, env: StreamEnvelope, seq: number) {
    const convId = env.conversationId;
    const msgs = ensureMsgs(convId);
    const blkMap = ensureBlocks(convId);

    switch (name) {
      case 'message_start': {
        if (!env.id) return;
        const m: Message = {
          id: env.id,
          conversationId: convId,
          role: env.role ?? 'assistant',
          status: 'streaming',
          createdAt: new Date().toISOString(),
          blocks: [],
          attrs: env.attrs,
        };
        if (env.parentBlockId) {
          // Nested message (subagent run); attach it under the tool_call block
          // by stamping parentBlockId on attrs for UI threading.
          m.attrs = { ...(m.attrs ?? {}), parentBlockId: env.parentBlockId };
        }
        if (!msgs.find((x) => x.id === m.id)) {
          msgs.push(m);
          conv.touchUpdated(convId);
        }
        break;
      }
      case 'message_stop': {
        if (!env.id) return;
        const m = msgs.find((x) => x.id === env.id);
        if (!m) return;
        m.status = (env.status as Message['status']) ?? 'completed';
        m.stopReason = env.stopReason;
        m.errorCode = env.errorCode;
        m.errorMessage = env.errorMessage;
        m.inputTokens = env.inputTokens;
        m.outputTokens = env.outputTokens;
        break;
      }
      case 'block_start': {
        if (!env.id || !env.parentId) return;
        const b: Block = {
          id: env.id,
          messageId: env.messageId ?? '',
          parentBlockId: env.parentId,
          type: (env.blockType as Block['type']) ?? 'text',
          status: 'streaming',
          content: '',
          attrs: env.attrs,
          seq,
          createdAt: new Date().toISOString(),
        };
        // Attach to parent: parentId can be a message ID (top-level block)
        // or a block ID (nested, e.g. progress under tool_call). Backend
        // also supplies messageId for convenience.
        const m = msgs.find((x) => x.id === env.parentId);
        if (m) {
          if (!m.blocks) m.blocks = [];
          m.blocks.push(b);
        }
        // Nested case: also add to parent block's children array.
        const parentBlk = blkMap[env.parentId];
        if (parentBlk) {
          if (!parentBlk.children) parentBlk.children = [];
          parentBlk.children.push(b);
          if (!b.messageId) b.messageId = parentBlk.messageId;
        }
        blkMap[env.id] = b;
        break;
      }
      case 'block_delta': {
        if (!env.id) return;
        const b = blkMap[env.id];
        if (!b) return;
        if (env.delta !== undefined) b.content += env.delta;
        if (env.attrs) b.attrs = { ...(b.attrs ?? {}), ...env.attrs };
        break;
      }
      case 'block_stop': {
        if (!env.id) return;
        const b = blkMap[env.id];
        if (!b) return;
        b.status = (env.status as Block['status']) ?? 'completed';
        if (env.error) b.error = env.error;
        if (env.attrs) b.attrs = { ...(b.attrs ?? {}), ...env.attrs };
        break;
      }
    }
  }

  /* ─────── outbound ─────── */

  async function send(content: string, attachmentIds: string[] = []) {
    if (!conv.selectedId) return;
    sending.value = true;
    try {
      await convAPI.sendMessage(conv.selectedId, content, attachmentIds);
      // No optimistic render — backend emits message_start + block events
      // for the user message within ~100ms via the eventlog SSE that App.vue
      // subscribed at mount, and the demuxer will append the canonical row.
      conv.touchUpdated(conv.selectedId);
    } catch (e) {
      ui.toast('err', `发送失败: ${(e as Error).message}`);
    } finally {
      sending.value = false;
    }
  }

  async function cancel() {
    if (!conv.selectedId) return;
    try {
      await convAPI.cancel(conv.selectedId);
    } catch (e) {
      ui.toast('err', `取消失败: ${(e as Error).message}`);
    }
  }

  async function deliverAnswer(toolCallId: string, answer: string, skipped = false) {
    if (!conv.selectedId) return;
    try {
      await convAPI.deliverAnswer(conv.selectedId, toolCallId, answer, skipped);
    } catch (e) {
      ui.toast('err', `投递答案失败: ${(e as Error).message}`);
    }
  }

  return {
    messagesByConv,
    blocksByConv,
    rawEvents,
    currentMessages,
    pendingAsk,
    loadingMessages,
    sending,
    streaming,
    loadMessages,
    startSSE,
    stopSSE,
    send,
    cancel,
    deliverAnswer,
  };
});
