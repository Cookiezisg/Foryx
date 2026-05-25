// Test setup — stubs the browser APIs jsdom doesn't ship but our code
// touches. Loaded automatically by vitest before each test file.

import "@testing-library/jest-dom/vitest";
import { afterEach, vi } from "vitest";
import { cleanup } from "@testing-library/react";
import i18n from "./i18n/index.js";

afterEach(() => cleanup());

i18n.changeLanguage("zh"); // resources eager 已在内存,同步生效;t() 立即返回中文

if (typeof window !== "undefined") {
  if (!window.matchMedia) {
    window.matchMedia = (q) => ({
      matches: false,
      media: q,
      onchange: null,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    });
  }

  if (!window.IntersectionObserver) {
    window.IntersectionObserver = class {
      observe() {} unobserve() {} disconnect() {} takeRecords() { return []; }
    };
  }

  if (!window.ResizeObserver) {
    window.ResizeObserver = class {
      observe() {} unobserve() {} disconnect() {}
    };
  }

  // requestAnimationFrame is in jsdom v22+ but we override to allow
  // tests to drive frame timing deterministically via vi.useFakeTimers.
  if (typeof window.requestAnimationFrame !== "function") {
    window.requestAnimationFrame = (cb) => setTimeout(() => cb(performance.now()), 16);
    window.cancelAnimationFrame = (id) => clearTimeout(id);
  }

  // jsdom doesn't implement scrollIntoView; Select uses it to keep the
  // highlighted option visible.
  if (!Element.prototype.scrollIntoView) {
    Element.prototype.scrollIntoView = () => {};
  }
}

// MockEventSource — replaces global EventSource. Each instance is
// registered so a test can grab the active one and dispatch events
// (`.emit("block_delta", {...})`) or simulate disconnect.
export class MockEventSource extends EventTarget {
  static instances = [];
  static reset() { MockEventSource.instances = []; }

  constructor(url) {
    super();
    this.url = url;
    this.readyState = 0; // CONNECTING
    this.listeners = {};
    MockEventSource.instances.push(this);
    queueMicrotask(() => {
      this.readyState = 1; // OPEN
      this.dispatchEvent(new Event("open"));
    });
  }

  addEventListener(type, listener) {
    super.addEventListener(type, listener);
    (this.listeners[type] ||= []).push(listener);
  }

  emit(type, data, lastEventId = "") {
    const evt = new MessageEvent(type, {
      data: typeof data === "string" ? data : JSON.stringify(data),
      lastEventId: String(lastEventId),
    });
    this.dispatchEvent(evt);
  }

  close() {
    this.readyState = 2; // CLOSED
  }
}
MockEventSource.CONNECTING = 0;
MockEventSource.OPEN = 1;
MockEventSource.CLOSED = 2;

if (typeof globalThis.EventSource === "undefined") {
  globalThis.EventSource = MockEventSource;
}
