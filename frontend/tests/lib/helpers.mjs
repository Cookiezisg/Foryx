// helpers — page-level convenience functions shared across spec files.
// Each helper assumes the harness has already loaded the app and the
// .sidebar is visible.
//
// helpers —— spec 间共享的页面级辅助。

import { backend } from "./backend.mjs";

// closeAllPanes — clicks every active nav item to close any open panes.
// 关闭所有当前 active 的 pane。
export async function closeAllPanes(page) {
  const actives = await page.locator(".nav-item.is-active").count();
  for (let i = 0; i < actives; i++) {
    await page.locator(".nav-item.is-active").first().click();
    await page.waitForTimeout(150);
  }
}

// openPaneByLabel — open a pane via sidebar click.
export async function openPaneByLabel(page, label) {
  await page.locator(`button.nav-item:has-text("${label}")`).first().click();
  await page.waitForTimeout(300);
}

// openCmdk — open command palette and wait for it.
export async function openCmdk(page) {
  await page.keyboard.press("Meta+k");
  await page.waitForSelector(".cmdk", { timeout: 4000 });
}

// clickConv — click a conversation in sidebar by title substring.
export async function clickConv(page, titleSubstring) {
  await page.locator(`.nav-item .label:has-text("${titleSubstring}")`).first().click();
  await page.waitForTimeout(400);
}

// seed — REST helpers re-exported under one namespace for spec brevity.
export const seed = {
  conv: (title) => backend.createConv(title),
  apiKey: () => backend.addKey("deepseek", backend.deepseekKey(), "DeepSeek (seed)"),
  configureChat: async () => {
    await backend.setModel("chat", "deepseek", "deepseek-v4-flash");
  },
  // ensureDeepSeek — idempotent setup that the live-chat tests need.
  ensureDeepSeek: async () => {
    if (!backend.hasDeepseekKey()) return false;
    const keys = await backend.apiKeys();
    const list = Array.isArray(keys) ? keys : (keys?.items || []);
    let key = list.find((k) => k.provider === "deepseek");
    if (!key) {
      key = await backend.addKey("deepseek", backend.deepseekKey(), "DeepSeek (seed)");
      await backend.testKey(key.id);
      await backend.setModel("chat", "deepseek", "deepseek-v4-flash");
    }
    return true;
  },
};

// waitFor — polling helper. Resolves when fn() returns truthy, rejects
// after timeoutMs.
export async function waitFor(fn, { timeoutMs = 5000, pollMs = 100, label = "condition" } = {}) {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    const v = await fn();
    if (v) return v;
    await new Promise((r) => setTimeout(r, pollMs));
  }
  throw new Error(`waitFor(${label}) timed out after ${timeoutMs}ms`);
}

// dragBy — emulate a mousedown-move-up gesture on an element by (dx, dy).
export async function dragBy(page, locator, dx, dy, steps = 10) {
  const box = await locator.boundingBox();
  if (!box) throw new Error("dragBy: locator has no bounding box");
  const sx = box.x + box.width / 2;
  const sy = box.y + box.height / 2;
  await page.mouse.move(sx, sy);
  await page.mouse.down();
  // PaneResize attaches its window mousemove/mouseup listeners in a
  // useEffect that runs *after* React commits the setDragging(true)
  // state change. Wait one paint+effect cycle before issuing any moves.
  await page.waitForTimeout(80);
  for (let i = 1; i <= steps; i++) {
    await page.mouse.move(sx + (dx * i) / steps, sy + (dy * i) / steps);
    await page.waitForTimeout(15);
  }
  await page.mouse.up();
}

// getDataAttr — read documentElement.dataset.x.
export const getDataAttr = (page, key) =>
  page.evaluate((k) => document.documentElement.dataset[k], key);
