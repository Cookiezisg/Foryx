<script setup lang="ts">
/**
 * TopBar — sticky title strip with build + port + catalog snapshot + expand toggle.
 */
import { onMounted, ref, computed } from 'vue';
import { devAPI } from '@/api/dev';
import { useUIStore } from '@/stores/ui';
import { useCatalogStore } from '@/stores/catalog';
import { status as sseStatus } from '@/api/sse';

const ui = useUIStore();
const catalog = useCatalogStore();

const info = ref<{ port?: number; buildSHA?: string; goVersion?: string; gitCommit?: string }>({});

onMounted(async () => {
  try {
    const i = await devAPI.info();
    info.value = i as typeof info.value;
  } catch {
    /* dev info optional; topbar still renders */
  }
});

/* re-render every 2s so SSE pills update */
const tick = ref(0);
setInterval(() => (tick.value += 1), 2000);

const sse = computed(() => {
  void tick.value;
  return {
    eventlog: sseStatus('eventlog'),
    notifications: sseStatus('notifications'),
    forge: sseStatus('forge'),
  };
});

const catalogFp = computed(() => catalog.current?.fingerprint?.slice(0, 8) ?? '—');
const catalogBy = computed(() => catalog.current?.generatedBy ?? '');
</script>

<template>
  <header class="topbar">
    <div class="title">
      <span class="brand">FORGIFY</span>
      <span class="brand-sub">tester</span>
    </div>

    <div class="meta">
      <span class="meta-item mono">port:{{ info.port ?? '?' }}</span>
      <span v-if="info.gitCommit" class="meta-item mono" :title="info.gitCommit">
        git:{{ info.gitCommit.slice(0, 7) }}
      </span>
      <span class="meta-item mono" :title="`catalog ${catalogBy} ${catalog.current?.generatedAt ?? ''}`">
        cat:{{ catalogFp }}
        <span v-if="catalogBy === 'mechanical-fallback'" class="catalog-fallback">·mech</span>
      </span>
    </div>

    <div class="spacer" />

    <div class="sse-row">
      <span
        v-for="s in [
          { id: 'eventlog', state: sse.eventlog, label: 'EL' },
          { id: 'notifications', state: sse.notifications, label: 'NF' },
          { id: 'forge', state: sse.forge, label: 'FG' },
        ]"
        :key="s.id"
        class="sse-pill"
        :class="{ on: s.state.connected, dim: !s.state.listenerCount }"
        :title="`${s.id}: ${s.state.connected ? 'connected' : 'disconnected'} (lastEventId=${s.state.lastEventId}, listeners=${s.state.listenerCount})`"
      >
        <span class="sse-dot" />
        {{ s.label }}
      </span>
    </div>

    <button class="btn ghost icon" @click="ui.toggleExpanded()" :title="ui.expanded ? 'restore 4-column' : 'expand col4 (hide conv + chat)'">
      <span v-if="ui.expanded">⛶</span>
      <span v-else>⤢</span>
    </button>

    <button class="btn ghost icon" @click="ui.openPalette()" title="⌘K command palette">⌘K</button>
  </header>
</template>

<style scoped>
.topbar {
  height: var(--topbar-h);
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  padding: 0 var(--sp-3);
  background: var(--bg-1);
  border-bottom: 1px solid var(--border-1);
  font-size: var(--fs-sm);
}

.title {
  display: flex;
  align-items: baseline;
  gap: 6px;
}

.brand {
  font-weight: 700;
  letter-spacing: 0.06em;
  color: var(--fg-1);
}

.brand-sub {
  color: var(--fg-3);
  font-size: var(--fs-xs);
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

.meta {
  display: flex;
  gap: var(--sp-2);
  color: var(--fg-2);
  font-family: var(--font-mono);
  font-size: var(--fs-xs);
}

.meta-item {
  padding: 2px 6px;
  border-radius: var(--radius-sm);
  background: var(--bg-2);
}

.catalog-fallback {
  color: var(--status-warn);
  margin-left: 2px;
}

.spacer {
  flex: 1;
}

.sse-row {
  display: flex;
  gap: 4px;
}

.sse-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  background: var(--bg-2);
  border: 1px solid var(--border-1);
  border-radius: var(--radius-sm);
  font-family: var(--font-mono);
  font-size: var(--fs-xs);
  color: var(--fg-2);
}

.sse-pill.on {
  color: var(--status-ok);
  border-color: var(--status-ok-bg);
}

.sse-pill.dim {
  opacity: 0.5;
}

.sse-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
}

.btn {
  background: transparent;
}
</style>
