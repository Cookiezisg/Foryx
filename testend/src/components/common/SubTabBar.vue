<script setup lang="ts">
defineProps<{
  modelValue: string;
  tabs: { id: string; label: string; badge?: string | number }[];
}>();

defineEmits<{ 'update:modelValue': [string] }>();
</script>

<template>
  <div class="subtabs">
    <button
      v-for="t in tabs"
      :key="t.id"
      class="subtab"
      :class="{ active: modelValue === t.id }"
      @click="$emit('update:modelValue', t.id)"
    >
      {{ t.label }}
      <span v-if="t.badge !== undefined" class="pill small">{{ t.badge }}</span>
    </button>
  </div>
</template>

<style scoped>
.subtab {
  background: transparent;
}
</style>
