/**
 * Backend API envelope + pagination types.
 *
 * Every HTTP response (other than 204 / SSE) follows one of:
 *   success:  { data: T,  nextCursor?: string, hasMore?: boolean }
 *   failure:  { error: { code, message, details? } }
 */

export interface Envelope<T> {
  data?: T;
  error?: ApiError;
  nextCursor?: string;
  hasMore?: boolean;
}

export interface ApiError {
  code: string;
  message: string;
  details?: unknown;
}

export interface Page<T> {
  items: T[];
  nextCursor: string;
  hasMore: boolean;
}

/** Generic ID prefixes used across the trinity backend (see §S15 for full list). */
export type IDPrefix =
  | 'cv' // conversation
  | 'msg' // message
  | 'blk' // block
  | 'att' // attachment
  | 'fn' // function
  | 'fnv' // function version
  | 'fne' // function execution (D22)
  | 'hd' // handler
  | 'hdv' // handler version
  | 'hcl' // handler call (D22)
  | 'wf' // workflow
  | 'wfv' // workflow version
  | 'fr' // flowrun
  | 'frn' // flowrun node
  | 'mcl' // mcp call (D22)
  | 'ske' // skill execution (D22)
  | 'aki' // apikey
  | 'mc' // model config
  | 'td' // todo
  | 'sar' // subagent run
  | 'smm' // subagent message
  | 'fnenv' // function venv
  | 'hdenv' // handler venv
  | 'bsh' // background bash shell
  | 'mem'; // memory (V1.2 §2 final-sweep)
