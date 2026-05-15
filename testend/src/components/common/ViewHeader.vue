<script setup lang="ts">
defineProps<{
  title: string;
  subtitle?: string;
}>();
</script>

<template>
  <header class="view-header">
    <div class="view-header-text">
      <h2 class="view-title">{{ title }}</h2>
      <p v-if="subtitle" class="view-subtitle dim">{{ subtitle }}</p>
    </div>
    <div class="view-header-actions">
      <slot name="actions" />
    </div>
  </header>
</template>

<style scoped>
.view-header {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  padding: var(--sp-3) var(--sp-4);
  border-bottom: 1px solid var(--border-1);
  background: var(--bg-1);
  flex-shrink: 0;
}

.view-header-text {
  flex: 1;
  min-width: 0;
}

.view-title {
  margin: 0;
  font-size: var(--fs-lg);
  font-weight: 600;
}

.view-subtitle {
  margin: 0;
  font-size: var(--fs-xs);
}

.view-header-actions {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  flex-shrink: 0;
}
</style>
