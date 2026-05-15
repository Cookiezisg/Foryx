/**
 * Format helpers — pure functions for rendering values.
 *
 * Kept stateless + side-effect-free so Vue templates can call freely
 * in render hot paths.
 */

const MIN = 60_000;
const HOUR = 3600_000;
const DAY = 86_400_000;

/** Relative time: "just now", "3m", "2h", "5d", "Apr 23". */
export function timeAgo(ts: string | number | Date | undefined): string {
  if (!ts) return '—';
  const t = new Date(ts).getTime();
  if (Number.isNaN(t)) return '—';
  const diff = Date.now() - t;
  if (diff < 30_000) return 'just now';
  if (diff < MIN) return `${Math.floor(diff / 1000)}s ago`;
  if (diff < HOUR) return `${Math.floor(diff / MIN)}m ago`;
  if (diff < DAY) return `${Math.floor(diff / HOUR)}h ago`;
  if (diff < 7 * DAY) return `${Math.floor(diff / DAY)}d ago`;
  return new Date(t).toLocaleDateString();
}

/** Absolute time: "2026-05-13 18:30". */
export function timestamp(ts: string | number | Date | undefined): string {
  if (!ts) return '—';
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return '—';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

/** Compact bytes: 1.2 KB, 3.4 MB. */
export function bytes(n: number | undefined): string {
  if (n === undefined || n === null || Number.isNaN(n)) return '—';
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  return `${(n / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

/** Duration in ms: 23ms, 1.2s, 1m23s. */
export function duration(ms: number | undefined | null): string {
  if (ms === undefined || ms === null || Number.isNaN(ms)) return '—';
  if (ms < 1000) return `${Math.round(ms)}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(2)}s`;
  const min = Math.floor(ms / 60_000);
  const sec = Math.floor((ms % 60_000) / 1000);
  return `${min}m${sec.toString().padStart(2, '0')}s`;
}

/** Short ID — trim `<prefix>_<16hex>` to `<prefix>_xxxx` for table cells. */
export function shortID(id: string | undefined, keep = 6): string {
  if (!id) return '—';
  const idx = id.indexOf('_');
  if (idx < 0) return id.length > keep + 2 ? `${id.slice(0, keep)}…` : id;
  return `${id.slice(0, idx + 1)}${id.slice(idx + 1, idx + 1 + keep)}…`;
}

/** Pretty-print JSON with 2-space indent. */
export function pretty(value: unknown): string {
  if (value === null || value === undefined) return '';
  if (typeof value === 'string') {
    // try parse; if it's a JSON string, pretty it; otherwise return as-is.
    try {
      const parsed = JSON.parse(value);
      return JSON.stringify(parsed, null, 2);
    } catch {
      return value;
    }
  }
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

/** Truncate string to N chars + ellipsis. */
export function truncate(s: string | undefined, max = 200): string {
  if (!s) return '';
  if (s.length <= max) return s;
  return s.slice(0, max) + '…';
}

/** Status → pill CSS class. */
export function statusClass(status?: string): string {
  if (!status) return '';
  const s = status.toLowerCase();
  if (['ok', 'completed', 'accepted', 'active', 'ready', 'connected'].includes(s)) return 'ok';
  if (['failed', 'error', 'rejected'].includes(s)) return 'err';
  if (['cancelled', 'timeout'].includes(s)) return 'warn';
  if (['pending', 'idle', 'unconfigured', 'partially_configured', 'streaming'].includes(s)) return 'pending';
  if (['paused'].includes(s)) return 'paused';
  if (['running', 'connecting'].includes(s)) return 'info';
  if (['degraded'].includes(s)) return 'warn';
  return '';
}

/** Pluralize: items(0,'thing') = 'no things'; items(1,'thing') = '1 thing'. */
export function plural(n: number, noun: string, plural = `${noun}s`): string {
  if (n === 0) return `no ${plural}`;
  return `${n} ${n === 1 ? noun : plural}`;
}
