import { useEffect, useRef, useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { getJSON, postJSON } from "@/api/devClient";
import { qk } from "@/hooks/queryKeys";
import { useConvStore } from "@/stores/conv";
import { useChatStore, type BlockNode } from "@/stores/chat";
import { subscribe } from "@/api/sse";
import type { Message } from "@frontend/entities/conversation/model/types";

function StatusBadge({ status }: { status: string }) {
  const kind =
    status === "completed" || status === "ok" ? "success" :
    status === "streaming" || status === "running" ? "streaming" :
    status === "error" || status === "cancelled" ? "error" : "";
  return <span className={`pill ${kind}`}>{status}</span>;
}

// P2.B placeholder block renderer — replaced by @/ui/BlockView in P2.C.
function BlockPlaceholder({ block }: { block: BlockNode }) {
  return (
    <div style={{ borderLeft: "2px solid var(--border-soft)", paddingLeft: 8, margin: "4px 0", fontSize: 12 }}>
      <div className="muted mono" style={{ fontSize: 10 }}>
        {block.type} <StatusBadge status={block.status} />
      </div>
      <div style={{ whiteSpace: "pre-wrap" }}>{block.content.slice(0, 240)}</div>
      {block.children?.map((c) => <BlockPlaceholder key={c.id} block={c} />)}
    </div>
  );
}

export function ChatPanel() {
  const { activeId } = useConvStore();
  const chat = useChatStore();
  const qc = useQueryClient();
  const [draft, setDraft] = useState("");
  const scrollRef = useRef<HTMLDivElement>(null);

  // Load existing messages on conv change.
  useQuery({
    queryKey: qk.messages(activeId ?? ""),
    queryFn: async () => {
      if (!activeId) return [] as Message[];
      const m = await getJSON<Message[]>(`/api/v1/conversations/${activeId}/messages`);
      chat.setMessages(activeId, m);
      return m;
    },
    enabled: !!activeId,
  });

  // SSE eventlog → chat store
  useEffect(() => {
    return subscribe("eventlog", (e) => {
      const d = e.data as Record<string, unknown>;
      const convId = d.conversationId as string | undefined;
      if (!convId) return;
      chat.ensureConv(convId);
      if (e.event === "message_start") {
        chat.onMessageStart(convId, d as Partial<Message> & { id: string });
      } else if (e.event === "message_stop") {
        chat.onMessageStop(convId, d.id as string, d as Partial<Message>);
      } else if (e.event === "block_start") {
        chat.onBlockStart(convId, d as Parameters<typeof chat.onBlockStart>[1]);
      } else if (e.event === "block_delta") {
        chat.onBlockDelta(convId, d.id as string, d.delta as string);
      } else if (e.event === "block_stop") {
        chat.onBlockStop(convId, d.id as string, d as Parameters<typeof chat.onBlockStop>[2]);
      }
    });
  }, [chat]);

  const send = useMutation({
    mutationFn: ({ content }: { content: string }) =>
      postJSON(`/api/v1/conversations/${activeId}/messages:send`, { content }),
    onSuccess: () => { setDraft(""); qc.invalidateQueries({ queryKey: qk.conversations() }); },
  });

  const messages = activeId ? (chat.byConv[activeId]?.messages ?? []) : [];

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: "smooth" });
  }, [messages.length]);

  if (!activeId) return <div className="empty">pick a conversation</div>;
  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
      <div ref={scrollRef} style={{ flex: 1, overflowY: "auto", padding: 8 }}>
        {messages.map((m) => (
          <div key={m.id} style={{ marginBottom: 8 }}>
            <div style={{ display: "flex", gap: 6, fontSize: 11, color: "var(--fg-muted)", padding: "4px 8px" }}>
              <strong>{m.role}</strong> <StatusBadge status={m.status} />
              {m.inputTokens != null && <span>in {m.inputTokens}</span>}
              {m.outputTokens != null && <span>out {m.outputTokens}</span>}
            </div>
            {m.blocks?.map((b) => <BlockPlaceholder key={b.id} block={b} />)}
          </div>
        ))}
      </div>
      <div style={{ borderTop: "1px solid var(--border)", padding: 8 }}>
        <textarea value={draft} onChange={(e) => setDraft(e.target.value)} placeholder="发消息…(⌘+Enter)"
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
              send.mutate({ content: draft });
            }
          }}
          style={{
            width: "100%", minHeight: 48, padding: 6,
            border: "1px solid var(--border)", borderRadius: 4,
            background: "var(--bg-paper)", color: "var(--fg-body)",
            fontFamily: "var(--mono)", fontSize: 12, resize: "vertical",
          }} />
        <div style={{ display: "flex", justifyContent: "flex-end", marginTop: 4 }}>
          <button onClick={() => send.mutate({ content: draft })} disabled={!draft.trim() || send.isPending}
            style={{
              padding: "4px 12px", background: "var(--accent)",
              color: "var(--accent-fg)", border: "none", borderRadius: 4,
              cursor: "pointer", fontSize: 12,
            }}>send</button>
        </div>
      </div>
    </div>
  );
}
