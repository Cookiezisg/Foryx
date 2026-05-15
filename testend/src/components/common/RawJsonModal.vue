<script setup lang="ts">
import { useUIStore } from '@/stores/ui';
import { pretty } from '@/utils/format';

const ui = useUIStore();

function copy() {
  const text = pretty(ui.rawJson.data);
  navigator.clipboard?.writeText(text).then(
    () => ui.toast('ok', '已复制到剪贴板'),
    () => ui.toast('err', '复制失败'),
  );
}
</script>

<template>
  <div class="modal-backdrop" @click.self="ui.closeRaw()">
    <div class="modal">
      <header class="modal-head">
        <strong class="mono">{{ ui.rawJson.title }}</strong>
        <div class="spacer" />
        <button class="btn ghost sm" @click="copy">📋 copy</button>
        <button class="btn ghost sm" @click="ui.closeRaw()">×</button>
      </header>
      <pre class="modal-body mono">{{ pretty(ui.rawJson.data) }}</pre>
    </div>
  </div>
</template>

<style scoped>
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.6);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 200;
  padding: var(--sp-5);
}

.modal {
  background: var(--bg-1);
  border: 1px solid var(--border-2);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-modal);
  min-width: 480px;
  max-width: min(960px, 90vw);
  max-height: 80vh;
  display: flex;
  flex-direction: column;
}

.modal-head {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  padding: var(--sp-2) var(--sp-3);
  border-bottom: 1px solid var(--border-1);
}

.modal-body {
  flex: 1;
  overflow: auto;
  margin: 0;
  padding: var(--sp-3);
  font-size: var(--fs-sm);
  background: var(--bg-0);
}
</style>
