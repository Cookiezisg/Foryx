<script setup lang="ts">
/**
 * TabNav — col3.
 *
 * Two-level tree:
 *   - 6 collapsible sections
 *   - Each section lists items that route to /<section>/<item>
 *
 * "CURRENT CONV" items are gated on a selected conversation; they
 * render dim if none is selected.
 */
import { computed, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useConvStore } from '@/stores/conv';

defineProps<{ rail?: boolean }>();

const conv = useConvStore();
const route = useRoute();
const router = useRouter();

interface NavItem {
  label: string;
  path: string;
  hint?: string;
}
interface NavSection {
  id: string;
  label: string;
  items: NavItem[];
  requiresConv?: boolean;
}

const sections: NavSection[] = [
  {
    id: 'current',
    label: 'Current conv',
    requiresConv: true,
    items: [
      { label: 'Wire trace', path: '/current/wire', hint: 'LLM call request/events/elapsed' },
      { label: 'Eventlog raw', path: '/current/eventlog', hint: 'this conv\'s SSE events' },
      { label: 'Notifications', path: '/current/notifications', hint: 'notif history for this conv' },
      { label: 'Sub-agents', path: '/current/subagents', hint: 'sub-runs spawned in this conv' },
      { label: 'Tool calls', path: '/current/tools', hint: 'all tool_call blocks' },
      { label: 'Todos', path: '/current/todos', hint: 'conv-scoped todos' },
      { label: 'Asks pending', path: '/current/asks', hint: 'waiting AskUserQuestions' },
      { label: 'Attachments', path: '/current/attachments', hint: 'uploaded files' },
      { label: 'Compaction', path: '/current/compaction', hint: 'summary + block roles (V1.2 §1)' },
    ],
  },
  {
    id: 'forge',
    label: 'Forge',
    items: [
      { label: 'Functions', path: '/forge/functions' },
      { label: 'Handlers', path: '/forge/handlers' },
      { label: 'Workflows', path: '/forge/workflows' },
      { label: 'Tools registry', path: '/forge/tools', hint: 'LLM system tools' },
      { label: 'Collections', path: '/forge/collections', hint: 'YAML test scenarios' },
    ],
  },
  {
    id: 'execute',
    label: 'Execute',
    items: [
      { label: 'Triggers', path: '/execute/triggers' },
      { label: 'FlowRuns', path: '/execute/flowruns' },
      { label: 'Approvals', path: '/execute/approvals' },
      { label: 'Executions', path: '/execute/executions', hint: 'D22 5 表' },
    ],
  },
  {
    id: 'observe',
    label: 'Observe',
    items: [
      { label: 'Live SSE', path: '/observe/live', hint: '3 streams' },
      { label: 'Notif history', path: '/observe/notifications' },
      { label: 'Catalog', path: '/observe/catalog' },
      { label: 'Mock LLM', path: '/observe/mock-llm' },
    ],
  },
  {
    id: 'config',
    label: 'Config',
    items: [
      { label: 'API Keys', path: '/config/apikeys' },
      { label: 'Models', path: '/config/models' },
      { label: 'Skills', path: '/config/skills' },
      { label: 'MCP servers', path: '/config/mcp' },
      { label: 'Sandbox', path: '/config/sandbox' },
      { label: 'Memory', path: '/config/memory', hint: 'cross-conv long-term facts (V1.2 §2)' },
      { label: 'Permissions', path: '/config/permissions', hint: 'settings.json rules + hooks (V1.2 §3)' },
    ],
  },
  {
    id: 'dev',
    label: 'Dev',
    items: [
      { label: 'SQL', path: '/dev/sql' },
      { label: 'Info', path: '/dev/info' },
      { label: 'Routes', path: '/dev/routes' },
      { label: 'Backend logs', path: '/dev/logs' },
      { label: 'Processes', path: '/dev/processes', hint: 'Bash bg' },
      { label: 'Metrics', path: '/dev/metrics' },
      { label: 'Errors', path: '/dev/errors' },
    ],
  },
];

/* Persist collapsed sections in localStorage. */
const LS_KEY = 'forgify-testend:nav-collapsed';
const collapsed = ref<Set<string>>(
  (() => {
    try {
      const raw = localStorage.getItem(LS_KEY);
      return new Set<string>(raw ? (JSON.parse(raw) as string[]) : []);
    } catch {
      return new Set<string>();
    }
  })(),
);

function toggle(id: string) {
  if (collapsed.value.has(id)) collapsed.value.delete(id);
  else collapsed.value.add(id);
  try {
    localStorage.setItem(LS_KEY, JSON.stringify(Array.from(collapsed.value)));
  } catch {
    /* ignore */
  }
  // force reactivity
  collapsed.value = new Set(collapsed.value);
}

const activePath = computed(() => route.path);

function nav(p: string) {
  router.push(p);
}

function isActive(p: string) {
  return activePath.value === p || activePath.value.startsWith(p + '/');
}
</script>

<template>
  <nav class="tabnav" :class="{ rail }">
    <div v-if="!rail" class="tabnav-inner scroll">
      <section
        v-for="s in sections"
        :key="s.id"
        class="section"
        :class="{ disabled: s.requiresConv && !conv.selectedId, collapsed: collapsed.has(s.id) }"
      >
        <button class="section-header" @click="toggle(s.id)">
          <span class="caret">{{ collapsed.has(s.id) ? '▸' : '▾' }}</span>
          <span class="section-label">{{ s.label }}</span>
          <span v-if="s.requiresConv && !conv.selectedId" class="section-note" title="select a conversation">
            no conv
          </span>
        </button>
        <ul v-show="!collapsed.has(s.id)" class="section-items">
          <li
            v-for="it in s.items"
            :key="it.path"
            class="item"
            :class="{ active: isActive(it.path), gated: s.requiresConv && !conv.selectedId }"
            @click="(s.requiresConv && !conv.selectedId) ? null : nav(it.path)"
            :title="it.hint || ''"
          >
            <span>{{ it.label }}</span>
            <span v-if="it.hint" class="item-hint">{{ it.hint }}</span>
          </li>
        </ul>
      </section>
    </div>

    <div v-else class="rail-list">
      <button
        v-for="s in sections"
        :key="s.id"
        class="rail-section"
        :title="s.label"
        @click="nav(s.items[0].path)"
      >
        {{ s.label[0] }}
      </button>
    </div>
  </nav>
</template>

<style scoped>
.tabnav {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--bg-1);
  border-right: 1px solid var(--border-1);
  min-width: 0;
}

.tabnav-inner {
  flex: 1;
  overflow-y: auto;
  padding: var(--sp-2) 0;
}

.section {
  margin-bottom: var(--sp-1);
}

.section.disabled .section-header {
  color: var(--fg-3);
}

.section-header {
  display: flex;
  align-items: center;
  gap: var(--sp-1);
  width: 100%;
  padding: 4px 10px;
  font-size: var(--fs-xs);
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--fg-2);
  font-weight: 600;
  background: transparent;
  cursor: pointer;
}

.section-header:hover {
  color: var(--fg-1);
}

.caret {
  font-size: 10px;
  color: var(--fg-3);
  width: 10px;
}

.section-label {
  flex: 1;
  text-align: left;
}

.section-note {
  font-size: 9px;
  color: var(--fg-3);
  text-transform: none;
  letter-spacing: 0;
}

.section-items {
  list-style: none;
  margin: 0;
  padding: 0;
}

.item {
  display: flex;
  flex-direction: column;
  padding: 5px 10px 5px 24px;
  cursor: pointer;
  border-left: 2px solid transparent;
  font-size: var(--fs-sm);
  color: var(--fg-1);
  user-select: none;
}

.item:hover {
  background: var(--bg-hover);
}

.item.active {
  background: var(--bg-active);
  border-left-color: var(--accent);
  color: var(--accent);
}

.item.gated {
  color: var(--fg-3);
  cursor: not-allowed;
}

.item.gated:hover {
  background: transparent;
}

.item-hint {
  font-size: 10px;
  color: var(--fg-3);
}

.rail-list {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: var(--sp-2) 0;
  gap: 4px;
}

.rail-section {
  width: 28px;
  height: 28px;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--fg-2);
  font-weight: 600;
  font-size: var(--fs-sm);
}

.rail-section:hover {
  background: var(--bg-hover);
}
</style>
