<script setup lang="ts">
/**
 * Composer — chat input with conv state machine (2026-05 #6 redesign).
 *
 * Three states, derived from chat store:
 *   - IDLE             — nothing in flight; main input → POST /messages
 *   - AGENT_RUNNING    — assistant streaming (no pending ask); Send shows "■ stop"
 *   - AWAITING_ANSWER  — assistant called AskUserQuestion + waiting; input
 *                        routes to POST /answers; Skip button appears
 *
 * Smart route: user types into the same input no matter the state. The
 * composer figures out whether to send it as a new message or answer.
 * Mirrors Claude Code's "just chat" behaviour.
 */
import { computed, ref } from 'vue';
import { useChatStore } from '@/stores/chat';
import { useConvStore } from '@/stores/conv';
import { useUIStore } from '@/stores/ui';
import { attachmentAPI } from '@/api/misc';
import { bytes } from '@/utils/format';
import type { Attachment } from '@/types/domain';

const chat = useChatStore();
const conv = useConvStore();
const ui = useUIStore();

const text = ref('');
const pending = ref<Attachment[]>([]);
const taRef = ref<HTMLTextAreaElement | null>(null);
const dragOver = ref(false);

const sending = computed(() => chat.sending);
const streaming = computed(() => chat.streaming);
const pendingAsk = computed(() => chat.pendingAsk);

/** AWAITING_ANSWER mode = pendingAsk exists in the active conv. */
const awaitingAnswer = computed(() => pendingAsk.value !== null);

const canSubmit = computed(() => {
  if (sending.value) return false;
  if (awaitingAnswer.value) return text.value.trim().length > 0;
  if (streaming.value) return false;
  return text.value.trim().length > 0;
});

const placeholder = computed(() => {
  if (!conv.selectedId) return '选个对话先';
  if (awaitingAnswer.value) return '你的回答…(或点上方选项 / 按 Skip 让 agent 用默认值继续)';
  return 'Type a message…  ⏎ send, ⇧⏎ newline';
});

const sendLabel = computed(() => (awaitingAnswer.value ? '↑ 答' : '↑ Send'));

async function submit() {
  if (!canSubmit.value) return;
  const content = text.value.trim();
  if (awaitingAnswer.value && pendingAsk.value) {
    text.value = '';
    await chat.deliverAnswer(pendingAsk.value.toolCallId, content);
    return;
  }
  const ids = pending.value.map((a) => a.id);
  text.value = '';
  pending.value = [];
  await chat.send(content, ids);
}

async function skipAsk() {
  if (!pendingAsk.value) return;
  await chat.deliverAnswer(pendingAsk.value.toolCallId, '', true);
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey && !e.metaKey && !e.ctrlKey) {
    e.preventDefault();
    submit();
  }
}

async function onFiles(files: FileList | null) {
  if (!files) return;
  if (awaitingAnswer.value) {
    ui.toast('info', '答题模式不能上传附件,先答完上面的问题');
    return;
  }
  for (let i = 0; i < files.length; i++) {
    try {
      const att = await attachmentAPI.upload(files[i], conv.selectedId ?? undefined);
      pending.value.push(att);
    } catch (e) {
      ui.toast('err', `附件上传失败: ${(e as Error).message}`);
    }
  }
}

function onDrop(e: DragEvent) {
  e.preventDefault();
  dragOver.value = false;
  onFiles(e.dataTransfer?.files ?? null);
}

function removeAtt(id: string) {
  pending.value = pending.value.filter((a) => a.id !== id);
}

function jumpToAskBlock() {
  if (!pendingAsk.value) return;
  const el = document.getElementById(`block-${pendingAsk.value.block.id}`);
  if (el) el.scrollIntoView({ behavior: 'smooth', block: 'center' });
}

function truncate(s: string, n: number): string {
  return s.length > n ? s.slice(0, n) + '…' : s;
}
</script>

<template>
  <div
    class="composer"
    :class="{ 'drag-over': dragOver, awaiting: awaitingAnswer }"
    @dragenter.prevent="dragOver = !awaitingAnswer"
    @dragover.prevent="dragOver = !awaitingAnswer"
    @dragleave.prevent="dragOver = false"
    @drop="onDrop"
  >
    <!-- AWAITING_ANSWER 横幅:点击 Jump 到 ask block -->
    <div v-if="awaitingAnswer && pendingAsk" class="ask-banner">
      <span class="ask-icon">⏳</span>
      <span class="ask-text mono ellipsis">Agent 在等你答:{{ truncate(pendingAsk.question, 80) }}</span>
      <button class="btn ghost sm" @click="jumpToAskBlock" title="滚动到问题位置">↑ Jump</button>
    </div>

    <!-- 附件 chip(IDLE 态用) -->
    <div v-if="pending.length > 0 && !awaitingAnswer" class="att-row">
      <span v-for="a in pending" :key="a.id" class="att-chip">
        <span class="ellipsis" :title="a.filename">{{ a.filename }}</span>
        <span class="dim small">{{ bytes(a.sizeBytes) }}</span>
        <button class="btn ghost icon sm" @click="removeAtt(a.id)">×</button>
      </span>
    </div>

    <div class="composer-row">
      <textarea
        ref="taRef"
        v-model="text"
        :disabled="!conv.selectedId"
        :placeholder="placeholder"
        @keydown="onKeydown"
        rows="2"
      />
      <div class="composer-actions">
        <!-- 附件按钮仅 IDLE 显示 -->
        <label v-if="!awaitingAnswer" class="btn ghost icon" title="attach file">
          📎
          <input type="file" multiple style="display: none" @change="(e) => onFiles((e.target as HTMLInputElement).files)" />
        </label>

        <!-- Skip 按钮仅 AWAITING_ANSWER 显示 -->
        <button v-if="awaitingAnswer" class="btn ghost sm" @click="skipAsk" title="跳过这个问题,让 agent 用默认值继续">
          Skip
        </button>

        <!-- 主动作按钮:streaming 时 Stop,否则 Send/答 -->
        <button v-if="streaming && !awaitingAnswer" class="btn danger sm" @click="chat.cancel">■ stop</button>
        <button
          v-else
          class="btn primary"
          :class="{ 'awaiting-btn': awaitingAnswer }"
          :disabled="!canSubmit"
          @click="submit"
        >
          {{ sendLabel }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.composer {
  border-top: 1px solid var(--border-1);
  background: var(--bg-1);
  padding: var(--sp-2) var(--sp-3);
  flex-shrink: 0;
}

.composer.drag-over {
  background: var(--accent-bg);
}

.composer.awaiting {
  background: linear-gradient(180deg, rgba(250, 204, 21, 0.05) 0%, var(--bg-1) 30%);
}

.ask-banner {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  padding: 6px 8px;
  margin-bottom: var(--sp-1);
  background: rgba(250, 204, 21, 0.12);
  border: 1px solid rgba(250, 204, 21, 0.4);
  border-radius: var(--radius-sm);
  font-size: var(--fs-xs);
}

.ask-icon {
  font-size: var(--fs-md);
}

.ask-text {
  flex: 1;
  min-width: 0;
}

.att-row {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-bottom: var(--sp-1);
}

.att-chip {
  display: flex;
  align-items: center;
  gap: var(--sp-1);
  padding: 2px 6px;
  background: var(--bg-3);
  border-radius: var(--radius-sm);
  font-size: var(--fs-xs);
  max-width: 200px;
}

.composer-row {
  display: flex;
  gap: var(--sp-2);
  align-items: flex-end;
}

textarea {
  flex: 1;
  min-height: 40px;
  max-height: 200px;
  resize: vertical;
  font-family: var(--font-sans);
  font-size: var(--fs-sm);
  line-height: 1.5;
}

textarea:disabled {
  opacity: 0.5;
}

.composer-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}

.awaiting-btn {
  background: rgba(250, 204, 21, 0.8);
  color: #000;
}
</style>
