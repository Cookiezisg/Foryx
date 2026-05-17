/**
 * vue-i18n setup.
 *
 * Locales: zh-CN (default) + en. Picked at boot from:
 *   1. active user.language (after users store loads)
 *   2. localStorage `forgify:locale` (last-used fallback)
 *   3. navigator.language (system pref)
 *   4. zh-CN (final default)
 */

import { createI18n } from 'vue-i18n';
import zhCN from './locales/zh-CN.json';
import en from './locales/en.json';

export type SupportedLocale = 'zh-CN' | 'en';

const STORAGE_KEY = 'forgify:locale';

export function getInitialLocale(): SupportedLocale {
  try {
    const stored = localStorage.getItem(STORAGE_KEY) as SupportedLocale | null;
    if (stored === 'zh-CN' || stored === 'en') return stored;
  } catch {
    /* ignore */
  }
  // Sniff browser preference: anything starting with "zh" → zh-CN, else en.
  // 浏览器偏好嗅探：zh* → zh-CN，否则 en。
  const nav = (typeof navigator !== 'undefined' && navigator.language) || '';
  return nav.toLowerCase().startsWith('zh') ? 'zh-CN' : 'en';
}

export function persistLocale(loc: SupportedLocale) {
  try {
    localStorage.setItem(STORAGE_KEY, loc);
  } catch {
    /* ignore */
  }
}

export const i18n = createI18n({
  legacy: false, // composition API
  globalInjection: true,
  locale: getInitialLocale(),
  fallbackLocale: 'en',
  messages: {
    'zh-CN': zhCN,
    en,
  },
});

export function setLocale(loc: SupportedLocale) {
  i18n.global.locale.value = loc;
  persistLocale(loc);
  // Match Accept-Language for SSE EventSource (which can't set headers).
  // 给 SSE 用：EventSource 不能自定义 header,localStorage 同步。
}
