/**
 * vue-router (hash) — URL ↔ col4 nav state.
 *
 * Hash mode chosen because the backend serves the index.html at
 * `/dev/` for any sub-path (catch-all). Hash routing keeps that simple
 * and survives the backend cache-bust strategy.
 *
 * Top-level groups mirror the col3 sidebar:
 *   /current/:item   — when conv selected, scoped to that conv
 *   /forge/:item     — functions / handlers / workflows / tools / collections
 *   /execute/:item   — triggers / flowruns / approvals / executions
 *   /observe/:item   — live / notifications / catalog / mock-llm
 *   /config/:item    — apikey / model / skills / mcp / sandbox
 *   /dev/:item       — sql / info / routes / logs / processes / metrics / errors
 */

import { createRouter, createWebHashHistory, type RouteRecordRaw } from 'vue-router';

const routes: RouteRecordRaw[] = [
  { path: '/', redirect: '/forge/functions' },

  /* current-conversation views (col3 top section) */
  { path: '/current/wire', component: () => import('@/views/current/WireTrace.vue') },
  { path: '/current/eventlog', component: () => import('@/views/current/EventlogRaw.vue') },
  { path: '/current/notifications', component: () => import('@/views/current/Notifications.vue') },
  { path: '/current/subagents', component: () => import('@/views/current/SubAgents.vue') },
  { path: '/current/tools', component: () => import('@/views/current/ToolCalls.vue') },
  { path: '/current/todos', component: () => import('@/views/current/Todos.vue') },
  { path: '/current/asks', component: () => import('@/views/current/AsksPending.vue') },
  { path: '/current/attachments', component: () => import('@/views/current/Attachments.vue') },
  { path: '/current/compaction', component: () => import('@/views/current/Compaction.vue') },

  /* forge */
  { path: '/forge/functions', component: () => import('@/views/forge/Functions.vue') },
  { path: '/forge/functions/:id', component: () => import('@/views/forge/FunctionDetail.vue'), props: true },
  { path: '/forge/handlers', component: () => import('@/views/forge/Handlers.vue') },
  { path: '/forge/handlers/:id', component: () => import('@/views/forge/HandlerDetail.vue'), props: true },
  { path: '/forge/workflows', component: () => import('@/views/forge/Workflows.vue') },
  { path: '/forge/workflows/:id', component: () => import('@/views/forge/WorkflowDetail.vue'), props: true },
  { path: '/forge/tools', component: () => import('@/views/forge/ToolsRegistry.vue') },
  { path: '/forge/collections', component: () => import('@/views/forge/TestCollections.vue') },

  /* execute */
  { path: '/execute/triggers', component: () => import('@/views/execute/Triggers.vue') },
  { path: '/execute/flowruns', component: () => import('@/views/execute/FlowRuns.vue') },
  { path: '/execute/flowruns/:id', component: () => import('@/views/execute/FlowRunDetail.vue'), props: true },
  { path: '/execute/approvals', component: () => import('@/views/execute/ApprovalsQueue.vue') },
  { path: '/execute/executions', component: () => import('@/views/execute/Executions.vue') },

  /* observe */
  { path: '/observe/live', component: () => import('@/views/observe/LiveSSE.vue') },
  { path: '/observe/notifications', component: () => import('@/views/observe/NotificationHistory.vue') },
  { path: '/observe/catalog', component: () => import('@/views/observe/Catalog.vue') },
  { path: '/observe/usage', component: () => import('@/views/observe/Usage.vue') },
  { path: '/observe/mock-llm', component: () => import('@/views/observe/MockLLM.vue') },

  /* config */
  { path: '/config/apikeys', component: () => import('@/views/config/ApiKeys.vue') },
  { path: '/config/models', component: () => import('@/views/config/ModelConfigs.vue') },
  { path: '/config/skills', component: () => import('@/views/config/Skills.vue') },
  { path: '/config/mcp', component: () => import('@/views/config/MCPServers.vue') },
  { path: '/config/sandbox', component: () => import('@/views/config/Sandbox.vue') },
  { path: '/config/memory', component: () => import('@/views/config/Memory.vue') },
  { path: '/config/documents', component: () => import('@/views/config/Documents.vue') },
  { path: '/config/permissions', component: () => import('@/views/config/Permissions.vue') },
  { path: '/config/llm-health', component: () => import('@/views/config/LLMHealth.vue') },
  { path: '/config/profile', component: () => import('@/views/config/Profile.vue') },

  /* dev */
  { path: '/dev/sql', component: () => import('@/views/dev/SQL.vue') },
  { path: '/dev/info', component: () => import('@/views/dev/Info.vue') },
  { path: '/dev/routes', component: () => import('@/views/dev/Routes.vue') },
  { path: '/dev/logs', component: () => import('@/views/dev/BackendLogs.vue') },
  { path: '/dev/processes', component: () => import('@/views/dev/Processes.vue') },
  { path: '/dev/metrics', component: () => import('@/views/dev/Metrics.vue') },
  { path: '/dev/errors', component: () => import('@/views/dev/Errors.vue') },
  { path: '/dev/prompts', component: () => import('@/views/dev/Prompts.vue') },

  /* catch-all */
  { path: '/:pathMatch(.*)*', redirect: '/forge/functions' },
];

export const router = createRouter({
  history: createWebHashHistory(),
  routes,
  scrollBehavior() {
    return { top: 0 };
  },
});
