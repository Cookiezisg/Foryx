<script setup lang="ts">
/**
 * SystemPromptEditor — inline editor in the chat header.
 */
import { ref, watch } from 'vue';
import { useConvStore } from '@/stores/conv';

const props = defineProps<{ convId: string }>();
const emit = defineEmits<{ close: [] }>();

const conv = useConvStore();
const draft = ref('');
const dirty = ref(false);

watch(
  () => props.convId,
  () => {
    const c = conv.list.find((x) => x.id === props.convId);
    draft.value = c?.systemPrompt ?? '';
    dirty.value = false;
  },
  { immediate: true },
);

watch(draft, () => (dirty.value = true));

async function save() {
  await conv.setSystemPrompt(props.convId, draft.value);
  dirty.value = false;
}

function reset() {
  const c = conv.list.find((x) => x.id === props.convId);
  draft.value = c?.systemPrompt ?? '';
  dirty.value = false;
}
</script>

<template>
  <div class="sys-editor">
    <header class="sys-head">
      <strong class="dim small">SYSTEM PROMPT</strong>
      <span v-if="dirty" class="pill warn">modified</span>
      <span class="spacer" />
      <button class="btn ghost sm" :disabled="!dirty" @click="reset">还原</button>
      <button class="btn primary sm" :disabled="!dirty" @click="save">保存</button>
      <button class="btn ghost sm" @click="emit('close')">关闭</button>
    </header>
    <textarea v-model="draft" rows="4" placeholder="optional system prompt prepended to every chat turn" />
  </div>
</template>

<style scoped>
.sys-editor {
  background: var(--bg-2);
  border-bottom: 1px solid var(--border-1);
  padding: var(--sp-2) var(--sp-3);
}

.sys-head {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  margin-bottom: var(--sp-1);
}

textarea {
  width: 100%;
  font-family: var(--font-mono);
  font-size: var(--fs-sm);
  resize: vertical;
}
</style>
