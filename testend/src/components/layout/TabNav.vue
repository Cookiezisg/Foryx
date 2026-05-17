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
import { useI18n } from 'vue-i18n';
import { useConvStore } from '@/stores/conv';

defineProps<{ rail?: boolean }>();

const { t } = useI18n();
const conv = useConvStore();
const route = useRoute();
const router = useRouter();

interface NavItem {
  labelKey: string;
  path: string;
  hint?: string;
}
interface NavSection {
  id: string;
  labelKey: string;
  items: NavItem[];
  requiresConv?: boolean;
}

const sections = computed<NavSection[]>(() => [
  {
    id: 'current',
    labelKey: 'nav.sectionCurrent',
    requiresConv: true,
    items: [
      { labelKey: 'nav.currentWire', path: '/current/wire' },
      { labelKey: 'nav.currentEventlog', path: '/current/eventlog' },
      { labelKey: 'nav.currentNotifications', path: '/current/notifications' },
      { labelKey: 'nav.currentSubagents', path: '/current/subagents' },
      { labelKey: 'nav.currentTools', path: '/current/tools' },
      { labelKey: 'nav.currentTodos', path: '/current/todos' },
      { labelKey: 'nav.currentAsks', path: '/current/asks' },
      { labelKey: 'nav.currentAttachments', path: '/current/attachments' },
      { labelKey: 'nav.currentCompaction', path: '/current/compaction' },
    ],
  },
  {
    id: 'forge',
    labelKey: 'nav.sectionForge',
    items: [
      { labelKey: 'nav.forgeFunctions', path: '/forge/functions' },
      { labelKey: 'nav.forgeHandlers', path: '/forge/handlers' },
      { labelKey: 'nav.forgeWorkflows', path: '/forge/workflows' },
      { labelKey: 'nav.forgeTools', path: '/forge/tools' },
      { labelKey: 'nav.forgeCollections', path: '/forge/collections' },
    ],
  },
  {
    id: 'execute',
    labelKey: 'nav.sectionExecute',
    items: [
      { labelKey: 'nav.executeTriggers', path: '/execute/triggers' },
      { labelKey: 'nav.executeFlowruns', path: '/execute/flowruns' },
      { labelKey: 'nav.executeApprovals', path: '/execute/approvals' },
      { labelKey: 'nav.executeExecutions', path: '/execute/executions' },
    ],
  },
  {
    id: 'observe',
    labelKey: 'nav.sectionObserve',
    items: [
      { labelKey: 'nav.observeLive', path: '/observe/live' },
      { labelKey: 'nav.observeNotifications', path: '/observe/notifications' },
      { labelKey: 'nav.observeCatalog', path: '/observe/catalog' },
      { labelKey: 'nav.observeUsage', path: '/observe/usage' },
      { labelKey: 'nav.observeMockLLM', path: '/observe/mock-llm' },
    ],
  },
  {
    id: 'config',
    labelKey: 'nav.sectionConfig',
    items: [
      { labelKey: 'nav.configApiKeys', path: '/config/apikeys' },
      { labelKey: 'nav.configModels', path: '/config/models' },
      { labelKey: 'nav.configSkills', path: '/config/skills' },
      { labelKey: 'nav.configMCP', path: '/config/mcp' },
      { labelKey: 'nav.configSandbox', path: '/config/sandbox' },
      { labelKey: 'nav.configMemory', path: '/config/memory' },
      { labelKey: 'nav.configDocuments', path: '/config/documents' },
      { labelKey: 'nav.configPermissions', path: '/config/permissions' },
      { labelKey: 'nav.configLLMHealth', path: '/config/llm-health' },
      { labelKey: 'nav.configProfile', path: '/config/profile' },
    ],
  },
  {
    id: 'dev',
    labelKey: 'nav.sectionDev',
    items: [
      { labelKey: 'nav.devSQL', path: '/dev/sql' },
      { labelKey: 'nav.devInfo', path: '/dev/info' },
      { labelKey: 'nav.devRoutes', path: '/dev/routes' },
      { labelKey: 'nav.devLogs', path: '/dev/logs' },
      { labelKey: 'nav.devProcesses', path: '/dev/processes' },
      { labelKey: 'nav.devMetrics', path: '/dev/metrics' },
      { labelKey: 'nav.devErrors', path: '/dev/errors' },
      { labelKey: 'nav.devPrompts', path: '/dev/prompts' },
    ],
  },
]);

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
          <span class="section-label">{{ t(s.labelKey) }}</span>
          <span v-if="s.requiresConv && !conv.selectedId" class="section-note" :title="t('nav.noConv')">
            {{ t('nav.noConv') }}
          </span>
        </button>
        <ul v-show="!collapsed.has(s.id)" class="section-items">
          <li
            v-for="it in s.items"
            :key="it.path"
            class="item"
            :class="{ active: isActive(it.path), gated: s.requiresConv && !conv.selectedId }"
            @click="(s.requiresConv && !conv.selectedId) ? null : nav(it.path)"
          >
            <span>{{ t(it.labelKey) }}</span>
          </li>
        </ul>
      </section>
    </div>

    <div v-else class="rail-list">
      <button
        v-for="s in sections"
        :key="s.id"
        class="rail-section"
        :title="t(s.labelKey)"
        @click="nav(s.items[0].path)"
      >
        {{ t(s.labelKey)[0] }}
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
