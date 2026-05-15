/**
 * /dev/* endpoints — backend dev console helpers.
 *
 * Wire-format mix (intentional): the TE-9/TE-21 family uses the standard
 * `{data}` envelope via responsehttpapi.Success; the older SQL / schema /
 * collections / tools / invoke endpoints emit raw JSON via writeDevJSON.
 * Pick getJSON vs getRaw based on which helper the handler uses.
 */

import { getJSON, getRaw, postJSON, postRaw } from './client';

export const devAPI = {
  /* ── envelope-wrapped (responsehttpapi.Success) ────────────────────── */

  info: () =>
    getJSON<{
      port: number;
      home: string;
      forgifyHome: string;
      integrationDir: string;
      collectionsDir: string;
      mcpConfigPath: string;
      skillsDir: string;
      catalogCachePath: string;
      buildID?: string;
      goVersion?: string;
      startedAt?: string;
      tableCounts?: Record<string, number>;
    }>('/dev/info'),

  runtime: () =>
    getJSON<{
      uptimeSec: number;
      numGoroutine: number;
      memAllocBytes: number;
      memSysBytes: number;
      numGC: number;
      dbSizeBytes?: number;
    }>('/dev/runtime'),

  routes: () =>
    getJSON<{ method: string; path: string; handler: string }[]>('/dev/routes'),

  forgifyHome: () =>
    getJSON<{
      path: string;
      mcpJson?: string;
      skillsDir?: string;
      catalogJson?: string;
      tree?: Array<{ name: string; size: number; isDir: boolean; modified: string }>;
    }>('/dev/forgify-home'),

  bashProcesses: () =>
    getJSON<{
      processes: Array<{
        id: string;
        command: string;
        cwd: string;
        startedAt: string;
        status: string;
        exitCode?: number;
      }>;
    }>('/dev/bash-processes'),

  mockLLMPush: (scripts: unknown[]) =>
    postJSON<{ pushed: number }>('/dev/mock-llm/scripts', { scripts }),

  mockLLMQueue: () =>
    getJSON<{ scripts: unknown[]; count: number }>('/dev/mock-llm/queue'),

  mockLLMClear: () => fetch('/dev/mock-llm/scripts', { method: 'DELETE' }),

  mockLLMLastPrompt: () =>
    getJSON<{ messages: unknown[]; tools?: unknown[]; capturedAt?: string }>(
      '/dev/mock-llm/last-prompt',
    ),

  llmTrace: () =>
    getJSON<
      Array<{
        traceId: string;
        conversationId?: string;
        scenario?: string;
        model?: string;
        startedAt: string;
        elapsedMs: number;
        request?: unknown;
        events?: unknown[];
        finalText?: string;
        error?: string;
      }>
    >('/dev/llm-trace'),

  /* ── raw JSON (no envelope; writeDevJSON) ──────────────────────────── */

  sql: (sql: string) =>
    postRaw<{ columns: string[]; rows: unknown[][]; error?: string }, { sql: string }>(
      '/dev/sql',
      { sql },
    ),

  schema: () =>
    getRaw<
      Array<{
        name: string;
        rowCount: number;
        columns: Array<{
          name: string;
          type: string;
          notNull: boolean;
          pk: boolean;
          default?: string;
        }>;
      }>
    >('/dev/schema'),

  collections: () =>
    getRaw<
      Array<{
        name: string;
        description: string;
        steps: Array<{
          name: string;
          method: string;
          path: string;
          body?: Record<string, unknown>;
          expect?: { status: number };
          capture?: Record<string, string>;
        }>;
      }>
    >('/dev/collections'),

  tools: () =>
    getRaw<Array<{ name: string; desc: string }>>('/dev/tools'),

  invoke: (tool: string, args: string) =>
    postRaw<
      { ok: boolean; output: string; elapsedMs: number; error?: string },
      { tool: string; args: string }
    >('/dev/invoke', { tool, args }),
};
