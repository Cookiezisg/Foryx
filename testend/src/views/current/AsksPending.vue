<script setup lang="ts">
/**
 * AsksPending — every `tool_call` block in the current conversation that:
 *   1. has attrs.toolName === "AskUserQuestion"
 *   2. is still in `streaming` status (means: waiting for an answer)
 *   3. has no sibling `tool_result` child (defensive — answers create the result)
 *
 * Backend wire (`POST /api/v1/conversations/{id}/answers`) takes
 * `{toolCallId, answer:string}`; we serialize the structured answer set
 * as JSON like BlockView does.
 */
import { computed } from 'vue';
import { useConvStore } from '@/stores/conv';
import { useChatStore } from '@/stores/chat';
import { useUIStore } from '@/stores/ui';
import ViewHeader from '@/components/common/ViewHeader.vue';
import EmptyView from '@/components/common/EmptyView.vue';
import { pretty } from '@/utils/format';

const conv = useConvStore();
const chat = useChatStore();
const ui = useUIStore();

interface PendingAsk {
  blockId: string;
  toolCallId: string;
  summary?: string;
  questions: Array<{ question: string; header?: string; multiSelect?: boolean; options: Array<{ label: string; description?: string }> }>;
  raw: unknown;
}

const pending = computed<PendingAsk[]>(() => {
  if (!conv.selectedId) return [];
  const map = chat.blocksByConv[conv.selectedId] ?? {};
  const out: PendingAsk[] = [];
  for (const b of Object.values(map)) {
    if (b.type !== 'tool_call') continue;
    const a = (b.attrs ?? {}) as Record<string, unknown>;
    if (a.toolName !== 'AskUserQuestion') continue;
    if (b.status !== 'streaming') continue;
    const hasResult = Object.values(map).some((x) => x.type === 'tool_result' && x.parentBlockId === b.id);
    if (hasResult) continue;
    try {
      const parsed = JSON.parse(b.content || '{}');
      out.push({
        blockId: b.id,
        toolCallId: (a.toolCallId as string) || b.id,
        summary: a.summary as string | undefined,
        questions: parsed.questions ?? [],
        raw: b,
      });
    } catch {
      // skip malformed
    }
  }
  return out;
});

async function answerWithLabel(p: PendingAsk, qIdx: number, label: string) {
  const answers = p.questions.map((q, i) =>
    i === qIdx ? { question: q.question, answer: label } : { question: q.question, answer: '' },
  );
  await chat.deliverAnswer(p.toolCallId, JSON.stringify({ answers }));
}
</script>

<template>
  <div v-if="!conv.selectedId" class="view-pad">
    <EmptyView title="Asks pending" hint="选个对话先" />
  </div>
  <div v-else class="view">
    <ViewHeader title="Asks pending" :subtitle="`conv ${conv.selectedId} · ${pending.length} waiting`" />
    <div class="scroll">
      <div v-if="pending.length === 0" class="empty">
        <div class="empty-title">No pending asks</div>
        <div class="empty-hint">When the agent calls AskUserQuestion you'll see open prompts here.</div>
      </div>
      <article v-for="p in pending" :key="p.blockId" class="ask-card">
        <header class="ask-head">
          <span class="pill warn">waiting</span>
          <span class="ask-summary">{{ p.summary ?? p.toolCallId }}</span>
          <span class="spacer" />
          <button class="btn ghost sm" @click="ui.showRaw(`AskUserQuestion ${p.toolCallId}`, p.raw)">raw</button>
        </header>

        <div v-for="(q, idx) in p.questions" :key="idx" class="ask-q">
          <div class="ask-q-line">
            <span v-if="q.header" class="pill accent">{{ q.header }}</span>
            <strong>{{ q.question }}</strong>
          </div>
          <div class="ask-options">
            <button
              v-for="opt in q.options"
              :key="opt.label"
              class="btn"
              @click="answerWithLabel(p, idx, opt.label)"
            >
              {{ opt.label }}
              <span v-if="opt.description" class="dim small">— {{ opt.description }}</span>
            </button>
          </div>
        </div>

        <details v-if="p.raw" class="ask-raw">
          <summary class="dim small">raw block content</summary>
          <pre class="code-block mono">{{ pretty(p.raw) }}</pre>
        </details>
      </article>
    </div>
  </div>
</template>

<style scoped>
.view {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.view-pad {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}
.scroll {
  flex: 1;
  overflow: auto;
  padding: var(--sp-3);
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
}
.ask-card {
  border: 1px solid var(--border-1);
  border-radius: var(--radius-md);
  padding: var(--sp-3);
  background: var(--bg-1);
}
.ask-head {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  margin-bottom: var(--sp-2);
}
.ask-summary {
  font-weight: 600;
}
.spacer { flex: 1; }
.ask-q {
  margin-top: var(--sp-2);
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}
.ask-q-line {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
}
.ask-options {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-1);
}
.ask-raw {
  margin-top: var(--sp-2);
}
.empty {
  text-align: center;
  padding: var(--sp-6) 0;
}
.empty-title {
  font-weight: 600;
  margin-bottom: var(--sp-1);
}
</style>
