<script setup lang="ts">
import { computed } from 'vue';
import { statusClass } from '@/utils/format';

const props = defineProps<{
  kind?: 'ok' | 'warn' | 'err' | 'info' | 'pending' | 'paused' | 'accent';
  status?: string;
}>();

const cls = computed(() => props.kind ?? statusClass(props.status));
</script>

<template>
  <span class="pill" :class="cls">
    <slot />
  </span>
</template>
