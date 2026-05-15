<script setup lang="ts">
/**
 * Cmd+K palette — fuzzy nav across conversations + tab-nav items.
 *
 * Sources:
 *   - conversations (by title + id)
 *   - all tabnav items (label + section)
 *   - hardcoded "new conversation" action
 */
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useConvStore } from '@/stores/conv';
import { useUIStore } from '@/stores/ui';

const router = useRouter();
const conv = useConvStore();
const ui = useUIStore();

const q = ref('');
const inputRef = ref<HTMLInputElement | null>(null);
onMounted(() => inputRef.value?.focus());

interface Item {
  id: string;
  label: string;
  sub: string;
  action: () => void;
  score?: number;
}

const allItems = computed<Item[]>(() => {
  const out: Item[] = [];

  out.push({
    id: 'action:new-conv',
    label: '＋ 新对话',
    sub: 'create conversation',
    action: async () => {
      await conv.create('');
      ui.closePalette();
    },
  });

  for (const c of conv.list) {
    out.push({
      id: `conv:${c.id}`,
      label: c.title || '(untitled)',
      sub: `conversation · ${c.id}`,
      action: () => {
        conv.select(c.id);
        ui.closePalette();
      },
    });
  }

  const navItems: { sec: string; label: string; path: string }[] = [
    /* current */
    { sec: 'current', label: 'Wire trace', path: '/current/wire' },
    { sec: 'current', label: 'Eventlog raw', path: '/current/eventlog' },
    { sec: 'current', label: 'Notifications', path: '/current/notifications' },
    { sec: 'current', label: 'Sub-agents', path: '/current/subagents' },
    { sec: 'current', label: 'Tool calls', path: '/current/tools' },
    { sec: 'current', label: 'Todos', path: '/current/todos' },
    { sec: 'current', label: 'Asks pending', path: '/current/asks' },
    { sec: 'current', label: 'Attachments', path: '/current/attachments' },
    /* forge */
    { sec: 'forge', label: 'Functions', path: '/forge/functions' },
    { sec: 'forge', label: 'Handlers', path: '/forge/handlers' },
    { sec: 'forge', label: 'Workflows', path: '/forge/workflows' },
    { sec: 'forge', label: 'Tools registry', path: '/forge/tools' },
    { sec: 'forge', label: 'Collections', path: '/forge/collections' },
    /* execute */
    { sec: 'execute', label: 'Triggers', path: '/execute/triggers' },
    { sec: 'execute', label: 'FlowRuns', path: '/execute/flowruns' },
    { sec: 'execute', label: 'Approvals', path: '/execute/approvals' },
    { sec: 'execute', label: 'Executions', path: '/execute/executions' },
    /* observe */
    { sec: 'observe', label: 'Live SSE', path: '/observe/live' },
    { sec: 'observe', label: 'Notif history', path: '/observe/notifications' },
    { sec: 'observe', label: 'Catalog', path: '/observe/catalog' },
    { sec: 'observe', label: 'Mock LLM', path: '/observe/mock-llm' },
    /* config */
    { sec: 'config', label: 'API Keys', path: '/config/apikeys' },
    { sec: 'config', label: 'Models', path: '/config/models' },
    { sec: 'config', label: 'Skills', path: '/config/skills' },
    { sec: 'config', label: 'MCP servers', path: '/config/mcp' },
    { sec: 'config', label: 'Sandbox', path: '/config/sandbox' },
    /* dev */
    { sec: 'dev', label: 'SQL', path: '/dev/sql' },
    { sec: 'dev', label: 'Info', path: '/dev/info' },
    { sec: 'dev', label: 'Routes', path: '/dev/routes' },
    { sec: 'dev', label: 'Backend logs', path: '/dev/logs' },
    { sec: 'dev', label: 'Processes', path: '/dev/processes' },
    { sec: 'dev', label: 'Metrics', path: '/dev/metrics' },
    { sec: 'dev', label: 'Errors', path: '/dev/errors' },
  ];

  for (const n of navItems) {
    out.push({
      id: `nav:${n.path}`,
      label: n.label,
      sub: n.sec,
      action: () => {
        router.push(n.path);
        ui.closePalette();
      },
    });
  }
  return out;
});

const filtered = computed(() => {
  const query = q.value.trim().toLowerCase();
  if (!query) return allItems.value.slice(0, 30);
  // Simple substring scoring — label hit > sub hit; sort by score desc.
  const scored: Item[] = [];
  for (const item of allItems.value) {
    const labelL = item.label.toLowerCase();
    const subL = item.sub.toLowerCase();
    let score = 0;
    if (labelL.startsWith(query)) score += 100;
    else if (labelL.includes(query)) score += 60;
    if (subL.includes(query)) score += 20;
    if (item.id.toLowerCase().includes(query)) score += 30;
    if (score > 0) scored.push({ ...item, score });
  }
  scored.sort((a, b) => (b.score ?? 0) - (a.score ?? 0));
  return scored.slice(0, 30);
});

const sel = ref(0);

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'ArrowDown') {
    e.preventDefault();
    sel.value = Math.min(sel.value + 1, filtered.value.length - 1);
  } else if (e.key === 'ArrowUp') {
    e.preventDefault();
    sel.value = Math.max(sel.value - 1, 0);
  } else if (e.key === 'Enter') {
    e.preventDefault();
    const it = filtered.value[sel.value];
    if (it) it.action();
  }
}

function pick(i: number) {
  const it = filtered.value[i];
  if (it) it.action();
}
</script>

<template>
  <div class="palette-backdrop" @click.self="ui.closePalette()">
    <div class="palette">
      <input
        ref="inputRef"
        v-model="q"
        @keydown="onKeydown"
        @input="sel = 0"
        placeholder="跳到… (对话标题 / view / 'sql' / 'workflows' / ...)"
        autocomplete="off"
        spellcheck="false"
      />
      <div class="palette-results scroll">
        <div
          v-for="(it, idx) in filtered"
          :key="it.id"
          class="row"
          :class="{ active: idx === sel }"
          @click="pick(idx)"
          @mouseenter="sel = idx"
        >
          <span class="row-label">{{ it.label }}</span>
          <span class="row-sub dim small">{{ it.sub }}</span>
        </div>
        <div v-if="filtered.length === 0" class="empty dim">no matches</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.palette-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  z-index: 250;
  padding-top: 10vh;
  display: flex;
  justify-content: center;
}

.palette {
  background: var(--bg-1);
  border: 1px solid var(--border-2);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-modal);
  width: min(640px, 90vw);
  max-height: 60vh;
  display: flex;
  flex-direction: column;
}

.palette input {
  border: none;
  border-bottom: 1px solid var(--border-1);
  border-radius: 0;
  padding: var(--sp-3);
  font-size: var(--fs-lg);
}

.palette-results {
  overflow-y: auto;
  max-height: 50vh;
}

.row {
  padding: var(--sp-2) var(--sp-3);
  display: flex;
  flex-direction: column;
  gap: 2px;
  cursor: pointer;
  border-left: 2px solid transparent;
}

.row.active {
  background: var(--bg-active);
  border-left-color: var(--accent);
}

.row-label {
  font-size: var(--fs-md);
}
</style>
