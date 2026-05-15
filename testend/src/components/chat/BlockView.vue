<script setup lang="ts">
/**
 * BlockView — render a single block (recursive for tool_call → progress/result).
 *
 * Block types (per event-log-protocol.md):
 *   text          plain assistant prose
 *   reasoning     chain-of-thought; rendered collapsed by default
 *   tool_call     LLM-initiated tool invocation; children = progress + tool_result
 *   tool_result   final tool output (string)
 *   progress      per-step delta while tool runs
 *   message       sub-message (nested under e.g. Subagent tool_call)
 */
import { computed, ref } from 'vue';
import type { Block } from '@/types/domain';
import { useUIStore } from '@/stores/ui';
import { useChatStore } from '@/stores/chat';
import { duration, pretty, truncate } from '@/utils/format';

const props = defineProps<{
  block: Block;
  allBlocks: Record<string, Block>;
}>();

const ui = useUIStore();
const chat = useChatStore();

const expanded = ref(props.block.type !== 'reasoning'); // collapse reasoning by default

const children = computed(() => {
  const out = Object.values(props.allBlocks).filter((b) => b.parentBlockId === props.block.id);
  return out.sort((a, b) => a.seq - b.seq);
});

const isToolCall = computed(() => props.block.type === 'tool_call');
const isToolResult = computed(() => props.block.type === 'tool_result');
const isProgress = computed(() => props.block.type === 'progress');
const isReasoning = computed(() => props.block.type === 'reasoning');
const isMessage = computed(() => props.block.type === 'message');

const attrs = computed(() => props.block.attrs ?? {});

function showRaw() {
  ui.showRaw(`block ${props.block.id} (${props.block.type})`, props.block);
}

/* AskUserQuestion special-case: when block is a tool_call to "AskUserQuestion"
   and we have args.questions[] + status === 'streaming', render answer form. */
const askArgs = computed(() => {
  if (!isToolCall.value) return null;
  if (attrs.value.toolName !== 'AskUserQuestion') return null;
  try {
    return JSON.parse(props.block.content || '{}') as {
      questions: { question: string; header?: string; multiSelect?: boolean; options: { label: string; description?: string }[] }[];
    };
  } catch {
    return null;
  }
});

const askAnswerable = computed(() => {
  if (!askArgs.value) return false;
  // answerable iff block is still streaming (tool waiting) AND no tool_result child yet.
  if (props.block.status !== 'streaming') return false;
  return !children.value.some((c) => c.type === 'tool_result');
});

const askDraft = ref<Record<number, { label: string; freeText?: string; selectedLabels?: string[] }>>({});

async function submitAsk() {
  if (!askArgs.value) return;
  const answers: { question: string; answer?: string; answers?: string[]; freeText?: string }[] = [];
  for (const [idx, q] of askArgs.value.questions.entries()) {
    const draft = askDraft.value[idx];
    if (!draft) continue;
    if (q.multiSelect) {
      answers.push({ question: q.question, answers: draft.selectedLabels ?? [] });
    } else if (draft.label === '__other__') {
      answers.push({ question: q.question, freeText: draft.freeText ?? '' });
    } else {
      answers.push({ question: q.question, answer: draft.label });
    }
  }
  const toolCallId = (attrs.value.toolCallId as string) || props.block.id;
  // Backend wire (`POST /api/v1/conversations/{id}/answers`) takes
  // `answer: string` — serialize the structured payload as JSON.
  await chat.deliverAnswer(toolCallId, JSON.stringify({ answers }));
}
</script>

<template>
  <div
    class="blk"
    :class="[
      'type-' + block.type,
      'status-' + block.status,
      { 'pending-ask': askAnswerable }
    ]"
    :id="`block-${block.id}`"
  >
    <!-- text / message: plain content -->
    <div v-if="block.type === 'text' || isMessage" class="blk-text mono">
      <pre>{{ block.content }}</pre>
      <span v-if="block.status === 'streaming'" class="cursor">▍</span>
    </div>

    <!-- reasoning: collapsible -->
    <div v-else-if="isReasoning" class="blk-reasoning">
      <button class="blk-toggle" @click="expanded = !expanded">
        <span class="caret">{{ expanded ? '▾' : '▸' }}</span>
        <em class="dim">reasoning ({{ block.content.length }} chars)</em>
      </button>
      <pre v-if="expanded" class="mono">{{ block.content }}</pre>
    </div>

    <!-- tool_call: header + args + children (progress/result) -->
    <div v-else-if="isToolCall" class="blk-toolcall">
      <header class="tc-head">
        <span class="tc-name mono">▸ {{ attrs.toolName || 'tool' }}</span>
        <span v-if="attrs.summary && attrs.summary !== attrs.toolName" class="tc-summary dim">{{ attrs.summary }}</span>
        <span v-if="attrs.destructive" class="pill warn" title="LLM marked this call as potentially destructive">⚠ destructive</span>
        <span v-if="attrs.executionGroup !== undefined && Number(attrs.executionGroup) > 0" class="pill info">grp:{{ attrs.executionGroup }}</span>
        <span class="pill" :class="block.status">{{ block.status }}</span>
        <span v-if="attrs.elapsedMs" class="mono dim xs">{{ duration(Number(attrs.elapsedMs)) }}</span>
        <span class="spacer" />
        <button class="btn ghost sm" @click="expanded = !expanded">{{ expanded ? '−' : '+' }}</button>
        <button class="btn ghost sm" @click="showRaw">raw</button>
      </header>

      <div v-if="expanded" class="tc-body">
        <details v-if="block.content" class="tc-args">
          <summary class="dim small">args</summary>
          <pre class="mono code-block">{{ pretty(block.content) }}</pre>
        </details>

        <div v-if="children.length > 0" class="tc-children">
          <BlockView v-for="c in children" :key="c.id" :block="c" :all-blocks="allBlocks" />
        </div>

        <!-- AskUserQuestion answer form (inline) -->
        <div v-if="askArgs && askAnswerable" class="ask-form">
          <div v-for="(q, idx) in askArgs.questions" :key="idx" class="ask-q">
            <div class="ask-q-line">
              <span v-if="q.header" class="pill accent">{{ q.header }}</span>
              <strong>{{ q.question }}</strong>
            </div>
            <div v-if="q.multiSelect" class="ask-multi">
              <label v-for="opt in q.options" :key="opt.label">
                <input
                  type="checkbox"
                  :value="opt.label"
                  @change="
                    (e) => {
                      askDraft[idx] = askDraft[idx] || { label: '', selectedLabels: [] };
                      const list = askDraft[idx].selectedLabels ?? (askDraft[idx].selectedLabels = []);
                      const cb = e.target as HTMLInputElement;
                      if (cb.checked) list.push(opt.label);
                      else askDraft[idx].selectedLabels = list.filter((x) => x !== opt.label);
                    }
                  "
                />
                {{ opt.label }}
                <span v-if="opt.description" class="dim small">— {{ opt.description }}</span>
              </label>
            </div>
            <div v-else class="ask-single">
              <button
                v-for="opt in q.options"
                :key="opt.label"
                class="btn"
                :class="{ primary: askDraft[idx]?.label === opt.label }"
                @click="askDraft[idx] = { label: opt.label }"
              >
                {{ opt.label }}
                <span v-if="opt.description" class="dim small">{{ opt.description }}</span>
              </button>
              <button class="btn ghost" @click="askDraft[idx] = { label: '__other__', freeText: '' }">
                其他…
              </button>
              <input
                v-if="askDraft[idx]?.label === '__other__'"
                placeholder="输入…"
                v-model="askDraft[idx]!.freeText"
              />
            </div>
          </div>
          <button class="btn primary" @click="submitAsk">投递答案</button>
        </div>
      </div>
    </div>

    <!-- tool_result: indented box -->
    <div v-else-if="isToolResult" class="blk-toolresult">
      <div class="tr-head dim small">▸ tool_result</div>
      <pre class="code-block mono">{{ truncate(block.content, 4000) }}</pre>
    </div>

    <!-- progress: small line -->
    <div v-else-if="isProgress" class="blk-progress dim small">
      <span v-if="attrs.stage" class="pill pending">{{ attrs.stage }}</span>
      <pre class="mono inline">{{ block.content }}</pre>
    </div>

    <!-- unknown -->
    <div v-else class="blk-unknown dim">
      <pre class="mono">[{{ block.type }}] {{ block.content }}</pre>
    </div>

    <div v-if="block.error" class="blk-error mono">⨯ {{ block.error }}</div>
  </div>
</template>

<style scoped>
.blk {
  font-size: var(--fs-sm);
}

.blk-text pre {
  font-family: var(--font-mono);
  margin: 0;
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.6;
}

.cursor {
  display: inline-block;
  animation: pulse 1s ease-in-out infinite;
  color: var(--accent);
}

/* AskUserQuestion pending — soft yellow glow + pulse to draw user's eye.
 * Combined with Composer's awareness banner, makes it impossible to miss.
 * AskUserQuestion 等答时柔和黄光呼吸,跟 Composer 横幅一起让用户必然注意。 */
.blk.pending-ask {
  outline: 2px solid rgba(250, 204, 21, 0.55);
  outline-offset: 4px;
  border-radius: var(--radius-sm);
  animation: ask-glow 1.8s ease-in-out infinite;
}

@keyframes ask-glow {
  0%, 100% {
    outline-color: rgba(250, 204, 21, 0.55);
    box-shadow: 0 0 12px rgba(250, 204, 21, 0.18);
  }
  50% {
    outline-color: rgba(250, 204, 21, 0.85);
    box-shadow: 0 0 18px rgba(250, 204, 21, 0.32);
  }
}

.blk-reasoning {
  border-left: 2px solid var(--border-2);
  padding-left: var(--sp-2);
}

.blk-toggle {
  display: flex;
  align-items: center;
  gap: 4px;
  background: transparent;
  cursor: pointer;
  font-size: var(--fs-xs);
}

.blk-toolcall {
  background: var(--bg-2);
  border: 1px solid var(--border-1);
  border-radius: var(--radius-sm);
  padding: var(--sp-2);
}

.tc-head {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  flex-wrap: wrap;
}

.tc-name {
  font-weight: 600;
}

.tc-summary {
  font-size: var(--fs-sm);
}

.tc-body {
  margin-top: var(--sp-2);
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}

.tc-args summary {
  cursor: pointer;
  padding: 2px 0;
}

.tc-children {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
  padding-left: var(--sp-3);
  border-left: 1px dashed var(--border-2);
}

.blk-toolresult {
  background: var(--bg-2);
  border-left: 2px solid var(--accent);
  padding: var(--sp-2);
  border-radius: var(--radius-sm);
}

.tr-head {
  margin-bottom: 4px;
}

.blk-progress {
  display: flex;
  gap: var(--sp-1);
  align-items: center;
}

.blk-progress pre.inline {
  display: inline;
  margin: 0;
}

.blk-unknown pre {
  background: var(--bg-2);
  padding: var(--sp-1) var(--sp-2);
  border-radius: var(--radius-sm);
}

.blk-error {
  background: var(--status-err-bg);
  color: var(--status-err);
  padding: var(--sp-1) var(--sp-2);
  border-radius: var(--radius-sm);
  margin-top: var(--sp-1);
  font-size: var(--fs-xs);
}

.ask-form {
  background: var(--bg-3);
  border-radius: var(--radius-sm);
  padding: var(--sp-3);
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}

.ask-q {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}

.ask-q-line {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
}

.ask-multi label {
  display: block;
  font-size: var(--fs-sm);
  padding: 2px 0;
}

.ask-single {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  align-items: center;
}
</style>
