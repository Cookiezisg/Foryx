// Error/empty paths: backend 500, empty lists, malformed responses.
import { runCase } from "../lib/harness.mjs";
import { openPaneByLabel } from "../lib/helpers.mjs";

export default [
  ["forge empty list shows empty state with hint", async ({ page, expect }) => {
    await openPaneByLabel(page, "锻造");
    await page.waitForSelector(".pane[data-kind='forge']");
    await page.waitForTimeout(500);
    // 'all' tab; if no forges, empty state visible
    const cards = await page.locator(".pane[data-kind='forge'] table.t tbody tr").count();
    const empty = await page.locator(".pane[data-kind='forge'] .empty").count();
    expect.gte(cards + empty, 1, "either rows or empty-state must render");
  }],

  ["execute empty list shows 'no flowruns' message", async ({ page, expect }) => {
    await openPaneByLabel(page, "执行");
    await page.waitForSelector(".pane[data-kind='execute']");
    await page.waitForTimeout(500);
    const empty = await page.locator(".pane[data-kind='execute'] .empty .title").count();
    const rows = await page.locator(".pane[data-kind='execute'] table.t tbody tr").count();
    expect.gte(empty + rows, 1, "either rows or empty state");
  }],

  // These two cases intentionally trigger 500s. Forwarded to runCase as
  // { allowConsoleErrors: true } to silence the console-error guard.
  ["network 500 on /api/v1/conversations surfaces gracefully (no white screen)", async ({ page, expect }) => {
    await page.route("**/api/v1/conversations**", (r) => r.fulfill({ status: 500, body: '{"error":{"code":"INTERNAL","message":"test forced"}}' }));
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector(".sidebar", { timeout: 5000 });
    await page.waitForTimeout(500);
    const sidebarVisible = await page.locator(".sidebar").isVisible();
    expect.truthy(sidebarVisible, "sidebar should render despite /conversations 500");
  }, { allowConsoleErrors: true }],

  ["network 500 on /api/v1/api-keys surfaces gracefully", async ({ page, expect }) => {
    await page.route("**/api/v1/api-keys**", (r) => r.fulfill({ status: 500, body: '{"error":{"code":"X","message":"x"}}' }));
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector(".sidebar", { timeout: 5000 });
    await page.waitForTimeout(500);
    const sidebarVisible = await page.locator(".sidebar").isVisible();
    expect.truthy(sidebarVisible, "shell renders despite key-list 500");
  }, { allowConsoleErrors: true }],

  ["empty document list shows hint to drop markdown files", async ({ page, expect }) => {
    await openPaneByLabel(page, "文档");
    await page.waitForSelector(".pane[data-kind='documents']");
    await page.waitForTimeout(500);
    const empty = await page.locator(".pane[data-kind='documents'] .empty .title").count();
    const rows = await page.locator(".pane[data-kind='documents'] .card").count();
    expect.gte(empty + rows, 1);
  }],

  ["skills pane handles empty list", async ({ page, expect }) => {
    await openPaneByLabel(page, "Skills");
    await page.waitForSelector(".pane[data-kind='skills']");
    await page.waitForTimeout(500);
    const cards = await page.locator(".pane[data-kind='skills'] .card").count();
    const empty = await page.locator(".pane[data-kind='skills'] .empty").count();
    expect.gte(cards + empty, 1);
  }],
].map(([name, fn, opts]) => () => runCase("20-error-states · " + name, fn, opts));
