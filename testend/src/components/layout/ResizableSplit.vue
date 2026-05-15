<script setup lang="ts">
/**
 * Horizontal resizable split — fixed-width left pane + flex right pane
 * with a 4px draggable divider.
 *
 * Why not CSS grid? grid resize requires JS anyway for clamping + emit;
 * this is the simplest path that exposes a clean prop+emit contract.
 */
import { ref, computed } from 'vue';

const props = defineProps<{
  leftWidth: number;
  minLeft?: number;
  maxLeft?: number;
}>();

const emit = defineEmits<{ resize: [number] }>();

const dragging = ref(false);
const startX = ref(0);
const startW = ref(0);

function onMouseDown(e: MouseEvent) {
  dragging.value = true;
  startX.value = e.clientX;
  startW.value = props.leftWidth;
  document.body.style.cursor = 'col-resize';
  document.body.style.userSelect = 'none';
  document.addEventListener('mousemove', onMove);
  document.addEventListener('mouseup', onUp);
}

function onMove(e: MouseEvent) {
  if (!dragging.value) return;
  const delta = e.clientX - startX.value;
  const target = clamp(startW.value + delta, props.minLeft ?? 80, props.maxLeft ?? 1200);
  emit('resize', target);
}

function onUp() {
  dragging.value = false;
  document.body.style.cursor = '';
  document.body.style.userSelect = '';
  document.removeEventListener('mousemove', onMove);
  document.removeEventListener('mouseup', onUp);
}

function clamp(v: number, lo: number, hi: number): number {
  return Math.max(lo, Math.min(hi, v));
}

const leftStyle = computed(() => ({
  width: `${props.leftWidth}px`,
  flex: '0 0 auto',
}));
</script>

<template>
  <div class="rsplit">
    <div class="rsplit-left" :style="leftStyle">
      <slot name="left" />
    </div>
    <div class="rsplit-divider" :class="{ active: dragging }" @mousedown="onMouseDown" />
    <div class="rsplit-right">
      <slot name="right" />
    </div>
  </div>
</template>

<style scoped>
.rsplit {
  display: flex;
  flex: 1;
  min-width: 0;
  min-height: 0;
  width: 100%;
  height: 100%;
}

.rsplit-left {
  min-width: 0;
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.rsplit-right {
  flex: 1;
  min-width: 0;
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.rsplit-divider {
  flex: 0 0 auto;
  width: 4px;
  background: var(--border-1);
  cursor: col-resize;
  transition: background var(--transition-fast);
  position: relative;
}

.rsplit-divider:hover,
.rsplit-divider.active {
  background: var(--accent);
}
</style>
