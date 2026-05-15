<script setup lang="ts">
/**
 * ChatPanel — col2.
 *
 * Renders messages of the currently selected conversation.
 * Streaming-aware: blocks update live via the chat store's SSE subscription.
 */
import { computed, ref, nextTick, watch } from 'vue';
import { useConvStore } from '@/stores/conv';
import { useChatStore } from '@/stores/chat';
import MessageView from '@/components/chat/MessageView.vue';
import Composer from '@/components/chat/Composer.vue';
import SystemPromptEditor from '@/components/chat/SystemPromptEditor.vue';

const conv = useConvStore();
const chat = useChatStore();

const messages = computed(() => chat.currentMessages);
const selected = computed(() => conv.list.find((c) => c.id === conv.selectedId));

const scrollEl = ref<HTMLDivElement | null>(null);
const stickyBottom = ref(true);

function checkSticky() {
  const el = scrollEl.value;
  if (!el) return;
  stickyBottom.value = el.scrollTop + el.clientHeight + 40 >= el.scrollHeight;
}

watch(
  () => messages.value.length,
  async () => {
    await nextTick();
    if (stickyBottom.value) scrollToBottom();
  },
);

watch(
  () => conv.selectedId,
  async () => {
    await nextTick();
    scrollToBottom();
  },
);

function scrollToBottom() {
  const el = scrollEl.value;
  if (el) el.scrollTop = el.scrollHeight;
}

const showSysPrompt = ref(false);
</script>

<template>
  <section class="chat-panel">
    <header v-if="selected" class="chat-header">
      <div class="chat-header-left">
        <span class="chat-title ellipsis">{{ selected.title || '(untitled)' }}</span>
        <span class="mono dim xs">{{ selected.id }}</span>
      </div>
      <div class="chat-header-right">
        <button class="btn ghost sm" @click="showSysPrompt = !showSysPrompt">
          ✎ {{ selected.systemPrompt ? 'system prompt (set)' : 'system prompt' }}
        </button>
      </div>
    </header>

    <SystemPromptEditor v-if="showSysPrompt && selected" :conv-id="selected.id" @close="showSysPrompt = false" />

    <div v-if="!selected" class="chat-empty">
      <div class="empty">
        <div class="empty-title">还没选对话</div>
        <div class="empty-hint">从左侧选一个,或 ⌘N 开新对话</div>
      </div>
    </div>

    <template v-else>
      <div class="chat-stream scroll" ref="scrollEl" @scroll="checkSticky">
        <div v-if="chat.loadingMessages && messages.length === 0" class="empty">
          <span class="dim">加载消息中…</span>
        </div>
        <MessageView v-for="m in messages" :key="m.id" :message="m" :blocks="chat.blocksByConv[selected.id] ?? {}" />
      </div>

      <Composer />
    </template>
  </section>
</template>

<style scoped>
.chat-panel {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--bg-0);
  min-width: 0;
}

.chat-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--sp-2) var(--sp-3);
  border-bottom: 1px solid var(--border-1);
  background: var(--bg-1);
  flex-shrink: 0;
  gap: var(--sp-2);
}

.chat-header-left {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.chat-title {
  font-weight: 600;
  font-size: var(--fs-md);
}

.chat-header-right {
  display: flex;
  gap: var(--sp-1);
}

.chat-stream {
  flex: 1;
  overflow-y: auto;
  padding: var(--sp-3);
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
  min-height: 0;
}

.chat-empty {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}
</style>
