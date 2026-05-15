<script setup lang="ts">
/**
 * MessageView — render one message (user / assistant) with its block
 * tree. Tool-call blocks are special-cased to render summary + args +
 * result + nested children (progress / tool_result).
 */
import { computed } from 'vue';
import type { Block, Message } from '@/types/domain';
import BlockView from './BlockView.vue';
import { useUIStore } from '@/stores/ui';
import { duration, timeAgo } from '@/utils/format';

const props = defineProps<{
  message: Message;
  blocks: Record<string, Block>;
}>();

const ui = useUIStore();

/* Children of the message itself = blocks whose parentBlockId === message.id */
const topBlocks = computed(() => {
  const all = Object.values(props.blocks);
  return all
    .filter((b) => b.parentBlockId === props.message.id || (props.message.blocks ?? []).some((mb) => mb.id === b.id))
    .sort((a, b) => a.seq - b.seq);
});

const isUser = computed(() => props.message.role === 'user');
const isAssistant = computed(() => props.message.role === 'assistant');

function showRaw() {
  ui.showRaw(`message ${props.message.id}`, props.message);
}

const stopReasonClass = computed(() => {
  switch (props.message.stopReason) {
    case 'end_turn':
    case 'tool_use':
      return 'ok';
    case 'max_tokens':
      return 'warn';
    case 'cancelled':
    case 'error':
      return 'err';
    default:
      return '';
  }
});
</script>

<template>
  <article class="msg" :class="{ user: isUser, assistant: isAssistant }">
    <header class="msg-head">
      <span class="msg-role">{{ message.role }}</span>
      <span class="pill" :class="message.status">{{ message.status }}</span>
      <span v-if="message.stopReason" class="pill" :class="stopReasonClass">{{ message.stopReason }}</span>
      <span v-if="message.errorCode" class="pill err">{{ message.errorCode }}</span>
      <span v-if="message.inputTokens || message.outputTokens" class="mono dim xs">
        in {{ message.inputTokens ?? 0 }} · out {{ message.outputTokens ?? 0 }}
      </span>
      <span class="spacer" />
      <span class="dim xs">{{ timeAgo(message.createdAt) }}</span>
      <button class="btn ghost sm" @click="showRaw">raw</button>
    </header>

    <div v-if="message.errorMessage" class="msg-error mono">
      {{ message.errorMessage }}
    </div>

    <div class="msg-body">
      <BlockView v-for="b in topBlocks" :key="b.id" :block="b" :all-blocks="blocks" />
      <div v-if="topBlocks.length === 0 && message.status !== 'streaming'" class="dim small">
        (no content)
      </div>
    </div>
  </article>
</template>

<style scoped>
.msg {
  background: var(--bg-1);
  border: 1px solid var(--border-1);
  border-radius: var(--radius-md);
  padding: var(--sp-3);
}

.msg.user {
  background: var(--accent-bg);
  border-color: var(--accent-bg);
}

.msg-head {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  margin-bottom: var(--sp-2);
  flex-wrap: wrap;
}

.msg-role {
  text-transform: uppercase;
  letter-spacing: 0.06em;
  font-size: var(--fs-xs);
  font-weight: 600;
  color: var(--fg-2);
}

.msg-error {
  background: var(--status-err-bg);
  color: var(--status-err);
  padding: var(--sp-2) var(--sp-3);
  border-radius: var(--radius-sm);
  font-size: var(--fs-sm);
  margin-bottom: var(--sp-2);
}

.msg-body {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}
</style>
