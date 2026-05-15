<script setup lang="ts">
import { useUIStore } from '@/stores/ui';
const ui = useUIStore();
</script>

<template>
  <div class="toasts">
    <div v-for="t in ui.toasts" :key="t.id" class="toast" :class="t.kind" @click="ui.dismissToast(t.id)">
      <span class="toast-dot" />
      <span class="toast-msg">{{ t.message }}</span>
    </div>
  </div>
</template>

<style scoped>
.toasts {
  position: fixed;
  bottom: var(--sp-4);
  right: var(--sp-4);
  z-index: 300;
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
  pointer-events: none;
}

.toast {
  pointer-events: auto;
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  padding: var(--sp-2) var(--sp-3);
  background: var(--bg-2);
  border: 1px solid var(--border-2);
  border-radius: var(--radius-sm);
  box-shadow: var(--shadow-2);
  font-size: var(--fs-sm);
  cursor: pointer;
  min-width: 280px;
}

.toast.ok {
  border-color: var(--status-ok-bg);
}

.toast.err {
  border-color: var(--status-err-bg);
}

.toast-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--status-pending);
  flex-shrink: 0;
}

.toast.ok .toast-dot {
  background: var(--status-ok);
}

.toast.err .toast-dot {
  background: var(--status-err);
}

.toast.info .toast-dot {
  background: var(--status-info);
}
</style>
