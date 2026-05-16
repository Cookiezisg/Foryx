/**
 * Domain types mirroring the backend's Go structs.
 *
 * Kept loose (most fields optional) because:
 *   1. Versioning — backend evolves faster than this file.
 *   2. Read-mostly — UI doesn't construct entities, just renders.
 *
 * For any field this UI hard-relies on, mark it non-optional.
 */

import type { IDPrefix } from './api';

/** A `<prefix>_<16hex>` ID; just an alias for clarity. */
export type ID<_P extends IDPrefix = IDPrefix> = string;

export interface Conversation {
  id: ID<'cv'>;
  /** Hidden by backend (json:"-"); local-only field. */
  userId?: string;
  title: string;
  autoTitled?: boolean;
  systemPrompt?: string;
  /** V1.2 §1 final-sweep — running compaction summary; empty when no
   *  compaction has run yet. */
  summary?: string;
  /** seq of the last block covered by `summary`; 0 if never compacted. */
  summaryCoversUpToSeq?: number;
  createdAt: string;
  updatedAt: string;
}

export interface Message {
  id: ID<'msg'>;
  conversationId: ID<'cv'>;
  role: 'user' | 'assistant' | 'system';
  status: 'pending' | 'streaming' | 'completed' | 'error' | 'cancelled';
  stopReason?: string;
  errorCode?: string;
  errorMessage?: string;
  inputTokens?: number;
  outputTokens?: number;
  createdAt: string;
  updatedAt?: string;
  attrs?: Record<string, unknown>;
  blocks?: Block[];
}

export type BlockType =
  | 'text'
  | 'reasoning'
  | 'tool_call'
  | 'tool_result'
  | 'progress'
  | 'message'
  | 'compaction';

export type ContextRole = 'hot' | 'warm' | 'cold' | 'archived';

export interface Block {
  id: ID<'blk'>;
  messageId: ID<'msg'>;
  parentBlockId?: string;
  type: BlockType;
  status: 'streaming' | 'completed' | 'error' | 'cancelled';
  content: string;
  /** Projection role decided by app/contextmgr (V1.2 §1 final-sweep).
   *  Defaults to 'hot' on fresh blocks; demote/archive as conversation grows. */
  contextRole?: ContextRole;
  attrs?: Record<string, unknown> & {
    toolName?: string;
    toolCallId?: string;
    summary?: string;
    destructive?: boolean;
    executionGroup?: number;
    elapsedMs?: number;
    stage?: string;
    error?: string;
    /* compaction block payload */
    coversFromSeq?: number;
    coversToSeq?: number;
    blocksArchived?: number;
    generatedBy?: string;
  };
  error?: string;
  seq: number;
  createdAt: string;
  /** Local-only: children blocks nested under this one (e.g. progress under
   *  tool_call). Built from SSE block_start events; never persisted. */
  children?: Block[];
}

/* ──────────────── Trinity entities ──────────────── */

export interface Function {
  id: ID<'fn'>;
  userId: string;
  name: string;
  description: string;
  tags: string[];
  activeVersionId?: ID<'fnv'>;
  createdAt: string;
  updatedAt: string;
  // computed
  pending?: FunctionVersion;
  envStatus?: string;
  envError?: string;
  envSyncedAt?: string;
  envSyncStage?: string;
  envSyncDetail?: string;
}

export interface FunctionVersion {
  id: ID<'fnv'>;
  functionId: ID<'fn'>;
  status: 'pending' | 'accepted' | 'rejected';
  version?: number;
  code: string;
  parameters: ParameterSpec[];
  returnSchema?: Record<string, unknown>;
  dependencies: string[];
  pythonVersion: string;
  envId?: ID<'fnenv'>;
  envStatus?: string;
  envError?: string;
  envSyncedAt?: string;
  changeReason?: string;
  createdAt: string;
  updatedAt: string;
}

export interface ParameterSpec {
  name: string;
  type: string;
  required?: boolean;
  default?: unknown;
  description?: string;
  enum?: string[];
}

export interface Handler {
  id: ID<'hd'>;
  userId: string;
  name: string;
  description: string;
  tags: string[];
  activeVersionId?: ID<'hdv'>;
  createdAt: string;
  updatedAt: string;
  // computed
  pending?: HandlerVersion;
  configState?: 'unconfigured' | 'partially_configured' | 'ready';
  envStatus?: string;
  envError?: string;
  envSyncedAt?: string;
  envSyncStage?: string;
  envSyncDetail?: string;
  liveInstances?: number;
}

export interface HandlerVersion {
  id: ID<'hdv'>;
  handlerId: ID<'hd'>;
  status: 'pending' | 'accepted' | 'rejected';
  version?: number;
  imports: string;
  initBody: string;
  shutdownBody: string;
  methods: MethodSpec[];
  initArgsSchema: InitArgSpec[];
  dependencies: string[];
  pythonVersion: string;
  envId?: ID<'hdenv'>;
  envStatus?: 'pending' | 'syncing' | 'ready' | 'failed' | 'evicted';
  envError?: string;
  changeReason?: string;
  createdAt: string;
  updatedAt: string;
}

export interface MethodSpec {
  name: string;
  args: ArgSpec[];
  body: string;
  streaming?: boolean;
  returnSchema?: Record<string, unknown>;
  description?: string;
}

export interface ArgSpec {
  name: string;
  type: string;
  required?: boolean;
  default?: unknown;
  description?: string;
}

export interface InitArgSpec {
  name: string;
  type: string;
  required?: boolean;
  sensitive?: boolean;
  default?: unknown;
  description?: string;
}

export interface Workflow {
  id: ID<'wf'>;
  userId: string;
  name: string;
  description: string;
  tags: string[];
  enabled: boolean;
  concurrency: string;
  needsAttention: boolean;
  attentionReason?: string;
  activeVersionId?: ID<'wfv'>;
  createdAt: string;
  updatedAt: string;
  // computed
  pending?: WorkflowVersion;
  liveRuns?: number;
  lastFiredAt?: string;
  nextFireAt?: string;
}

export interface WorkflowVersion {
  id: ID<'wfv'>;
  workflowId: ID<'wf'>;
  status: 'pending' | 'accepted' | 'rejected';
  version?: number;
  graph: string; // raw JSON
  graphParsed?: Graph;
  changeReason?: string;
  createdAt: string;
  updatedAt: string;
}

export interface Graph {
  name: string;
  description?: string;
  tags?: string[];
  variables?: VariableSpec[];
  nodes: NodeSpec[];
  edges: EdgeSpec[];
}

export interface NodeSpec {
  id: string;
  type: NodeType;
  position?: { x: number; y: number };
  config?: Record<string, unknown>;
  retry?: RetryConfig;
  onError?: 'stop' | 'continue' | 'branch';
  timeout?: number;
  notes?: string;
}

export type NodeType =
  | 'trigger'
  | 'function'
  | 'handler'
  | 'mcp'
  | 'skill'
  | 'llm'
  | 'http'
  | 'condition'
  | 'loop'
  | 'parallel'
  | 'approval'
  | 'wait'
  | 'variable';

export interface RetryConfig {
  maxAttempts: number;
  backoff?: 'fixed' | 'linear' | 'exponential';
  delay?: number;
}

export interface VariableSpec {
  name: string;
  type: string;
  default?: unknown;
  description?: string;
}

export interface EdgeSpec {
  id: string;
  from: string;
  to: string;
}

/* ──────────────── FlowRun + Trigger ──────────────── */

export interface FlowRun {
  id: ID<'fr'>;
  userId: string;
  workflowId: ID<'wf'>;
  versionId: ID<'wfv'>;
  triggerKind: 'cron' | 'fsnotify' | 'webhook' | 'manual';
  triggerInput?: Record<string, unknown>;
  status: 'running' | 'paused' | 'completed' | 'failed' | 'cancelled';
  startedAt: string;
  endedAt?: string;
  elapsedMs?: number;
  output?: unknown;
  errorCode?: string;
  errorMessage?: string;
  pausedState?: PausedState;
  createdAt: string;
}

export interface PausedState {
  nodeId: string;
  variables?: Record<string, unknown>;
  outputs?: Record<string, Record<string, unknown>>;
  position?: string[];
  pausedAt: string;
}

export interface FlowRunNode {
  id: ID<'frn'>;
  userId: string;
  flowrunId: ID<'fr'>;
  nodeId: string;
  nodeType: NodeType;
  status: 'pending' | 'running' | 'ok' | 'failed' | 'cancelled' | 'timeout' | 'skipped';
  triggeredBy: 'chat' | 'workflow' | 'http' | 'test';
  input?: unknown;
  output?: unknown;
  errorCode?: string;
  errorMessage?: string;
  elapsedMs: number;
  startedAt: string;
  endedAt: string;
  conversationId?: string;
  messageId?: string;
  toolCallId?: string;
  flowrunNodeId?: string;
  attempts: number;
  createdAt: string;
}

export interface TriggerState {
  workflowId: ID<'wf'>;
  nodeId: string;
  kind: 'cron' | 'fsnotify' | 'webhook' | 'manual';
  status: 'active' | 'idle' | 'error';
  lastFiredAt?: string;
  nextFireAt?: string;
  lastError?: string;
}

/* ──────────────── D22 execution log shapes ──────────────── */

export interface ExecutionRow {
  id: string;
  userId: string;
  status: 'ok' | 'failed' | 'cancelled' | 'timeout';
  triggeredBy: 'chat' | 'workflow' | 'http' | 'test';
  input?: unknown;
  output?: unknown;
  errorCode?: string;
  errorMessage?: string;
  elapsedMs: number;
  startedAt: string;
  endedAt: string;
  conversationId?: string;
  messageId?: string;
  toolCallId?: string;
  flowrunId?: string;
  flowrunNodeId?: string;
  // kind-specific (each table adds 1-4 extra fields)
  [key: string]: unknown;
}

/* ──────────────── Resources ──────────────── */

export interface APIKey {
  id: ID<'aki'>;
  userId: string;
  provider: string;
  apiFormat?: string;
  displayName?: string;
  baseUrl?: string;
  keyMasked: string;
  /** Live-probe result from POST `/api-keys/{id}:test` action. */
  testStatus?: 'ok' | 'error' | 'untested' | '';
  testError?: string;
  lastTestedAt?: string;
  modelsFound?: string[];
  createdAt: string;
  updatedAt: string;
}

export interface ModelConfig {
  id: ID<'mc'>;
  userId: string;
  scenario: string;
  provider: string;
  modelId: string;
  createdAt: string;
  updatedAt: string;
}

export interface Skill {
  name: string;
  description: string;
  bodyPath: string;
  dirPath: string;
  hash?: string;
  frontmatter: {
    name: string;
    description: string;
    allowed_tools?: string[];
    argument_hint?: string;
    context?: 'inline' | 'fork';
    agent?: string;
    effort?: 'low' | 'medium' | 'high';
    [k: string]: unknown;
  };
}

export interface MCPServerStatus {
  name: string;
  status: 'disconnected' | 'connecting' | 'connected' | 'degraded' | 'error';
  connectedAt?: string;
  lastError?: string;
  lastErrorAt?: string;
  lastSuccessAt?: string;
  consecutiveFailures: number;
  totalCalls: number;
  totalFailures: number;
  tools: MCPToolDef[];
}

export interface MCPToolDef {
  name: string;
  description: string;
  inputSchema?: Record<string, unknown>;
}

export interface MCPRegistryEntry {
  name: string;
  description: string;
  runtime: string;
  installCmd: { command: string; args?: string[] };
  requiredArgs?: { name: string; description: string; type: string }[];
  requiredEnv?: { name: string; description: string }[];
  category?: string;
  tier?: number;
}

export interface SandboxRuntime {
  id: string;
  kind: string;
  version: string;
  path: string;
  sizeBytes?: number;
  installedAt: string;
}

export interface SandboxEnv {
  id: string;
  runtimeId: string;
  ownerKind: string;
  ownerId: string;
  path: string;
  status: string;
  createdAt: string;
  lastUsedAt?: string;
}

/* ──────────────── Subagent ──────────────── */

export interface SubagentRun {
  id: ID<'sar'>;
  conversationId: ID<'cv'>;
  parentBlockId: string;
  type: string;
  prompt: string;
  status: 'running' | 'completed' | 'error' | 'cancelled' | 'maxturns';
  errorCode?: string;
  errorMessage?: string;
  startedAt: string;
  endedAt?: string;
  inputTokens?: number;
  outputTokens?: number;
  result?: string;
}

/* ──────────────── Todo + Ask ──────────────── */

export interface Todo {
  id: ID<'td'>;
  conversationId: ID<'cv'>;
  subject: string;
  description?: string;
  activeForm?: string;
  status: 'pending' | 'in_progress' | 'completed' | 'deleted';
  owner?: string;
  blockedBy?: string[];
  metadata?: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
}

export interface PendingAsk {
  toolCallId: string;
  conversationId: string;
  question: string;
  options?: { label: string; value?: string; description?: string }[];
  multiSelect?: boolean;
  askedAt: string;
}

/* ──────────────── Catalog ──────────────── */

export interface Catalog {
  fingerprint: string;
  version: number;
  summary: string;
  coverage: Record<string, string[]>;
  generatedBy: 'llm' | 'mechanical-fallback';
  generatedAt: string;
  /** Per-source last-scan timestamps. Keys: function / handler / workflow / mcp / skill. */
  sourcesAt?: Record<string, string>;
}

/* ──────────────── Tool registry ──────────────── */

export interface ToolMeta {
  name: string;
  desc: string;
}

/* ──────────────── Attachment ──────────────── */

export interface Attachment {
  id: ID<'att'>;
  conversationId?: ID<'cv'>;
  filename: string;
  mimeType: string;
  sizeBytes: number;
  createdAt: string;
}

/* ──────────────── Memory ────────────────
 * Cross-conversation long-term facts (V1.2 §2 final-sweep).
 * type: user / feedback / project / reference.
 * source: "user" (created in UI) / "ai" (written by write_memory tool).
 *
 * Memory ——跨对话长期事实（V1.2 §2 final-sweep）。
 * type：user / feedback / project / reference。source：UI 创建 / AI 工具写。
 */

export interface Memory {
  id: ID<'mem'>;
  name: string;
  type: 'user' | 'feedback' | 'project' | 'reference';
  description: string;
  content: string;
  pinned: boolean;
  source: 'user' | 'ai';
  metadata?: Record<string, unknown>;
  accessedAt?: string;
  accessCount?: number;
  createdAt: string;
  updatedAt: string;
}
