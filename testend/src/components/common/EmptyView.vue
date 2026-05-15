<script setup lang="ts">
defineProps<{ title: string; hint?: string }>();
</script>

<template>
  <div class="view-pad">
    <div class="empty">
      <div class="empty-title">{{ title }}</div>
      <div v-if="hint" class="empty-hint">{{ hint }}</div>
    </div>
  </div>
</template>

<style scoped>
.view-pad {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}
</style>
