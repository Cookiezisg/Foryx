// format.js — globally-available formatting helpers used across all tabs.
// Loaded BEFORE Alpine in tester.html so individual tab x-data components
// can call window.fmt* without imports. Keeps token / ms / byte / time
// formatting consistent everywhere.
//
// format.js — 全局格式化工具，所有 tab 共享。早于 Alpine 加载，让每个 tab
// 直接调 window.fmt*，token / 毫秒 / 字节 / 时间渲染保持一致。

(function () {
  'use strict';

  // Bytes → human-readable (B / KB / MB / GB / TB).
  // Bytes → 人类可读单位。
  window.fmtBytes = function (n) {
    if (n == null || isNaN(n)) return '—';
    if (n === 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let i = 0;
    let v = n;
    while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
    return (i === 0 ? v.toFixed(0) : v.toFixed(1)) + ' ' + units[i];
  };

  // Milliseconds → "423ms" / "1.2s" / "3m 4s" / "1h 23m".
  // 毫秒 → 友好时长。
  window.fmtMs = function (ms) {
    if (ms == null || isNaN(ms)) return '—';
    if (ms < 1000) return Math.round(ms) + 'ms';
    const s = ms / 1000;
    if (s < 60) return s.toFixed(s < 10 ? 1 : 0) + 's';
    const m = Math.floor(s / 60);
    const rest = Math.round(s % 60);
    if (m < 60) return m + 'm ' + rest + 's';
    const h = Math.floor(m / 60);
    return h + 'h ' + (m % 60) + 'm';
  };

  // Seconds (uptime) → "5d 3h 12m 4s".
  // 秒（uptime）→ 多段时长。
  window.fmtUptime = function (seconds) {
    if (seconds == null || isNaN(seconds)) return '—';
    const d = Math.floor(seconds / 86400);
    const h = Math.floor((seconds % 86400) / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    const s = Math.floor(seconds % 60);
    const parts = [];
    if (d) parts.push(d + 'd');
    if (h) parts.push(h + 'h');
    if (m) parts.push(m + 'm');
    parts.push(s + 's');
    return parts.join(' ');
  };

  // ISO timestamp → locale time. Returns '—' on falsy / parse fail so callers
  // can chain without null checks.
  // ISO 时间 → 本地时间字符串。失败返 '—' 让调用方无须判空。
  window.fmtTime = function (isoOrEpoch) {
    if (!isoOrEpoch) return '—';
    try {
      return new Date(isoOrEpoch).toLocaleTimeString();
    } catch {
      return String(isoOrEpoch);
    }
  };

  // Same as fmtTime but date+time.
  // 同 fmtTime 但带日期。
  window.fmtDateTime = function (isoOrEpoch) {
    if (!isoOrEpoch) return '—';
    try {
      return new Date(isoOrEpoch).toLocaleString();
    } catch {
      return String(isoOrEpoch);
    }
  };

  // Number with thousands-separator ("114831" → "114,831"). Uses tabular
  // figures via CSS so columns stay aligned.
  // 数字加千分位。CSS 走 tabular-nums 让列对齐。
  window.fmtNum = function (n) {
    if (n == null || isNaN(n)) return '—';
    return Number(n).toLocaleString();
  };

  // Truncate string with ellipsis. Mostly for ID-style strings where the
  // suffix carries info too — keeps a short head + tail when long.
  // 字符串截断省略号。ID 类字符串保留前后缀。
  window.fmtTrunc = function (s, max) {
    if (!s) return '';
    s = String(s);
    if (s.length <= max) return s;
    const head = Math.ceil((max - 3) / 2);
    const tail = Math.floor((max - 3) / 2);
    return s.slice(0, head) + '…' + s.slice(-tail);
  };
})();
