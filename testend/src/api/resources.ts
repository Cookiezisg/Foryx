/**
 * Resources API — apikey, model_config, skill, mcp, sandbox, catalog,
 * dev-tool registry. Grouped because each domain only has a few endpoints.
 *
 * All paths verified against `grep mux.HandleFunc` of the live router
 * (see backend/internal/transport/httpapi/handlers/*.go) — keep in sync.
 */

import { deleteEmpty, getJSON, postJSON, postRaw, putJSON, patchJSON } from './client';
import type {
  APIKey,
  ModelConfig,
  Skill,
  MCPServerStatus,
  MCPRegistryEntry,
  SandboxRuntime,
  SandboxEnv,
  Catalog,
  ToolMeta,
} from '@/types/domain';

/* ───────── providers (whitelist) ───────── */
export interface ProviderMeta {
  name: string;
  displayName: string;
  category: 'llm' | 'search' | 'other' | string;
  defaultBaseUrl?: string;
  baseUrlRequired: boolean;
}
export const providerAPI = {
  list: () => getJSON<ProviderMeta[]>('/api/v1/providers'),
};

/* ───────── api keys ───────── */
export const apikeyAPI = {
  list: () => getJSON<APIKey[]>('/api/v1/api-keys'),
  create: (in_: {
    provider: string;
    apiFormat?: string;
    displayName?: string;
    baseUrl?: string;
    key: string;
  }) => postJSON<APIKey>('/api/v1/api-keys', in_),
  update: (id: string, patch: { displayName?: string; baseUrl?: string; key?: string }) =>
    patchJSON<APIKey>(`/api/v1/api-keys/${id}`, patch),
  remove: (id: string) => deleteEmpty(`/api/v1/api-keys/${id}`),
  /** :test action — backend dispatches via `{idAction}` to live-probe the key. */
  test: (id: string) =>
    postJSON<{ ok: boolean; message?: string; latencyMs?: number; modelsFound?: string[] }>(
      `/api/v1/api-keys/${id}:test`,
    ),
};

/* ───────── model configs ───────── */
export const modelAPI = {
  list: () => getJSON<ModelConfig[]>('/api/v1/model-configs'),
  /**
   * PUT (idempotent upsert, returns 200 per N6).
   * Backend body: `{provider, modelId}`. Provider must match an existing
   * api-key on the user's account (lookup is provider→key, not by id).
   */
  upsert: (scenario: string, in_: { provider: string; modelId: string }) =>
    putJSON<ModelConfig>(`/api/v1/model-configs/${scenario}`, in_),
};

/* ───────── skills ───────── */
export const skillAPI = {
  list: () => getJSON<Skill[]>('/api/v1/skills'),
  get: (name: string) => getJSON<Skill>(`/api/v1/skills/${encodeURIComponent(name)}`),
  body: (name: string) =>
    getJSON<{ body: string }>(`/api/v1/skills/${encodeURIComponent(name)}/body`),
  refresh: () => postJSON<{ count: number }>('/api/v1/skills:refresh'),
  invoke: (name: string, args: Record<string, unknown> = {}) =>
    postJSON<{ output: string }>(
      `/api/v1/skills/${encodeURIComponent(name)}:invoke`,
      args,
    ),
};

/* ───────── mcp ───────── */
export const mcpAPI = {
  servers: () => getJSON<MCPServerStatus[]>('/api/v1/mcp-servers'),
  server: (name: string) =>
    getJSON<MCPServerStatus>(`/api/v1/mcp-servers/${encodeURIComponent(name)}`),
  stderr: (name: string) =>
    getJSON<{ stderr: string }>(`/api/v1/mcp-servers/${encodeURIComponent(name)}/stderr`),
  put: (name: string, config: unknown) =>
    putJSON<MCPServerStatus>(`/api/v1/mcp-servers/${encodeURIComponent(name)}`, config),
  remove: (name: string) => deleteEmpty(`/api/v1/mcp-servers/${encodeURIComponent(name)}`),
  reconnect: (name: string) =>
    postJSON<MCPServerStatus>(`/api/v1/mcp-servers/${encodeURIComponent(name)}:reconnect`),
  healthCheck: (name: string) =>
    postJSON<{ ok: boolean; latencyMs?: number; toolCount?: number; message?: string }>(
      `/api/v1/mcp-servers/${encodeURIComponent(name)}:health-check`,
    ),
  registry: () => getJSON<MCPRegistryEntry[]>('/api/v1/mcp-registry'),
  registryEntry: (name: string) =>
    getJSON<MCPRegistryEntry>(`/api/v1/mcp-registry/${encodeURIComponent(name)}`),
  install: (name: string, env: Record<string, string> = {}, args: Record<string, string> = {}) =>
    postJSON<MCPServerStatus>(
      `/api/v1/mcp-registry/${encodeURIComponent(name)}:install`,
      { env, args },
    ),
};

/* ───────── sandbox ───────── */
export const sandboxAPI = {
  runtimes: () => getJSON<SandboxRuntime[]>('/api/v1/sandbox/runtimes'),
  envs: (q: { ownerKind?: string; ownerId?: string } = {}) => {
    const qs = new URLSearchParams();
    if (q.ownerKind) qs.set('ownerKind', q.ownerKind);
    if (q.ownerId) qs.set('ownerId', q.ownerId);
    const suffix = qs.toString() ? `?${qs.toString()}` : '';
    return getJSON<SandboxEnv[]>(`/api/v1/sandbox/envs${suffix}`);
  },
  env: (id: string) => getJSON<SandboxEnv>(`/api/v1/sandbox/envs/${id}`),
  destroyEnv: (id: string) => postJSON<void>(`/api/v1/sandbox/envs/${id}:destroy`),
  destroyRuntime: (id: string) =>
    postJSON<void>(`/api/v1/sandbox/runtimes/${id}:destroy`),
  diskUsage: () =>
    getJSON<{ totalBytes: number; runtimeBytes: number; envBytes: number }>(
      '/api/v1/sandbox/disk-usage',
    ),
  bootstrapStatus: () =>
    getJSON<{ ready: boolean; miseBin?: string; message?: string }>(
      '/api/v1/sandbox/bootstrap-status',
    ),
  /** Generic action endpoint: gc / retry-bootstrap. */
  action: (action: 'gc' | 'retry-bootstrap') =>
    postJSON<unknown>(`/api/v1/sandbox/${action}`),
  /** Per-conversation env list (handler envs scoped to a conv). */
  convEnvs: (convId: string) =>
    getJSON<SandboxEnv[]>(`/api/v1/conversations/${convId}/sandbox-envs`),
  resetConvEnv: (convId: string, kind: string) =>
    postJSON<void>(`/api/v1/conversations/${convId}/sandbox-envs/${kind}:reset`),
  resetAllConvEnvs: (convId: string) =>
    postJSON<void>(`/api/v1/conversations/${convId}/sandbox-envs:reset-all`),
};

/* ───────── catalog ───────── */
export const catalogAPI = {
  get: () => getJSON<Catalog | null>('/api/v1/catalog'),
  refresh: () => postJSON<Catalog>('/api/v1/catalog:refresh'),
};

/* ───────── tool registry (dev) ───────── */
export const toolsAPI = {
  list: () => getJSON<ToolMeta[]>('/dev/tools'),
  invoke: (tool: string, args: string) =>
    postRaw<
      { ok: boolean; output: string; elapsedMs: number; error?: string; data?: unknown },
      { tool: string; args: string }
    >('/dev/invoke', { tool, args }),
};
