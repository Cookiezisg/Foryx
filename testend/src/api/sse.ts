/**
 * SSE channel manager — wraps `EventSource` for the three per-user backend streams.
 *
 *   - /api/v1/eventlog       chat-side events (5 events × 6 block types)
 *   - /api/v1/notifications  entity state changes (open vocab `type`)
 *   - /api/v1/forge          trinity forging progress (4 events × 3 kinds)
 *
 * The backend keys all three by user_id (D-redo-3), no query params.
 * We expose a fan-out subscribe API so multiple views can listen without
 * each opening its own EventSource (saves the browser's per-origin
 * connection budget).
 */

export type StreamID = 'eventlog' | 'notifications' | 'forge';

export interface StreamEvent<T = unknown> {
  /** The SSE `event:` line — always `event` for eventlog/forge; the
   *  notifications stream uses the literal "notification" (see
   *  events-design.md §11.3). */
  event: string;
  /** Monotonic per-user sequence (also reflected in the SSE `id:` line so
   *  Last-Event-ID resume works). */
  id: number;
  /** Parsed JSON payload. */
  data: T;
  /** Wall-clock when the client received it. */
  receivedAt: number;
}

type Listener = (e: StreamEvent) => void;

interface Channel {
  url: string;
  es: EventSource | null;
  lastEventId: number;
  listeners: Set<Listener>;
  connected: boolean;
  connectedAt?: number;
  lastError?: string;
}

const URLS: Record<StreamID, string> = {
  eventlog: '/api/v1/eventlog',
  notifications: '/api/v1/notifications',
  forge: '/api/v1/forge',
};

const channels: Record<StreamID, Channel> = {
  eventlog: blankChannel(URLS.eventlog),
  notifications: blankChannel(URLS.notifications),
  forge: blankChannel(URLS.forge),
};

function blankChannel(url: string): Channel {
  return {
    url,
    es: null,
    lastEventId: 0,
    listeners: new Set(),
    connected: false,
  };
}

function connect(stream: StreamID) {
  const ch = channels[stream];
  if (ch.es) return; // already up
  // Backend reads `Last-Event-ID` HEADER for resume — EventSource sets that
  // automatically on auto-reconnect after network error. On manual reconnect
  // we close + reopen with no header, accepting that gap (frontend can
  // re-fetch state via REST if needed).
  const es = new EventSource(ch.url, { withCredentials: false });
  ch.es = es;
  ch.connected = false;

  es.onopen = () => {
    ch.connected = true;
    ch.connectedAt = Date.now();
    ch.lastError = undefined;
  };

  es.onerror = () => {
    ch.connected = false;
    ch.lastError = 'connection error / 410 SEQ_TOO_OLD likely; reconnecting…';
    // browser auto-reconnects on transient errors; for 410 we reset to 0
    // and reconnect manually after a tick.
    if (ch.es) {
      ch.es.close();
      ch.es = null;
    }
    setTimeout(() => {
      if (ch.listeners.size > 0) {
        // Force resync from scratch — events before lastEventId are lost
        // but the user can manually re-fetch via REST.
        ch.lastEventId = 0;
        connect(stream);
      }
    }, 1000);
  };

  // Generic handler for unnamed messages (rare).
  es.onmessage = (ev) => fanOut(stream, 'message', ev);

  // Specific event names: eventlog/forge fire `event:<eventName>`, notif
  // fires `event:notification`. We don't care which here — we just demux
  // on payload.
  for (const name of ['notification', 'message_start', 'message_stop',
    'block_start', 'block_delta', 'block_stop',
    'forge_started', 'forge_op_applied', 'forge_env_attempt', 'forge_completed']) {
    es.addEventListener(name, (ev) => fanOut(stream, name, ev as MessageEvent));
  }
}

function fanOut(stream: StreamID, eventName: string, ev: MessageEvent) {
  const ch = channels[stream];
  let parsed: unknown = ev.data;
  try {
    parsed = JSON.parse(ev.data as string);
  } catch {
    /* keep raw */
  }
  const id = ev.lastEventId ? Number(ev.lastEventId) : 0;
  if (id > ch.lastEventId) ch.lastEventId = id;
  const wrapped: StreamEvent = {
    event: eventName,
    id,
    data: parsed,
    receivedAt: Date.now(),
  };
  for (const fn of ch.listeners) {
    try {
      fn(wrapped);
    } catch (e) {
      console.error(`[sse:${stream}] listener threw`, e);
    }
  }
}

/**
 * subscribe to a stream — returns an unsubscribe function.
 * Opens the EventSource lazily on first subscriber and closes when the
 * last one leaves.
 */
export function subscribe(stream: StreamID, fn: Listener): () => void {
  const ch = channels[stream];
  ch.listeners.add(fn);
  if (!ch.es) connect(stream);
  return () => {
    ch.listeners.delete(fn);
    if (ch.listeners.size === 0 && ch.es) {
      ch.es.close();
      ch.es = null;
      ch.connected = false;
    }
  };
}

/** Snapshot of stream state, for UI status pills. */
export function status(stream: StreamID) {
  const ch = channels[stream];
  return {
    connected: ch.connected,
    connectedAt: ch.connectedAt,
    listenerCount: ch.listeners.size,
    lastEventId: ch.lastEventId,
    lastError: ch.lastError,
  };
}

/** Force a resync — closes + reopens with lastEventId=0. */
export function reconnect(stream: StreamID) {
  const ch = channels[stream];
  if (ch.es) {
    ch.es.close();
    ch.es = null;
  }
  ch.lastEventId = 0;
  if (ch.listeners.size > 0) connect(stream);
}
