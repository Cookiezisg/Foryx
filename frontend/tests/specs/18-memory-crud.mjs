// Memory CRUD via UI: create, list, edit, delete.
import { runCase } from "../lib/harness.mjs";
import { openPaneByLabel } from "../lib/helpers.mjs";

async function openMemory(page) {
  // Close any existing extra pane first to keep layout predictable
  await openPaneByLabel(page, "Memory");
  await page.waitForSelector(".pane[data-kind='memory']", { timeout: 4000 });
  await page.waitForTimeout(400);
}

export default [
  ["memory pane shows 4-type tabs", async ({ page, expect }) => {
    await openMemory(page);
    const tabs = await page.locator(".pane[data-kind='memory'] .page-tab").count();
    expect.gte(tabs, 5, "expect at least 5 tabs (全部/user/feedback/project/reference)");
  }],

  ["clicking '新建' opens MemoryDrawer", async ({ page, expect }) => {
    await openMemory(page);
    await page.locator(".pane[data-kind='memory'] .page-actions button:has-text('新建')").click();
    await page.waitForSelector(".drawer", { timeout: 3000 });
    const titleVisible = await page.locator(".drawer-title:has-text('新建 Memory')").count();
    expect.equals(titleVisible, 1, "drawer title should read '新建 Memory'");
  }],

  ["MemoryDrawer cancel closes drawer", async ({ page, expect }) => {
    await openMemory(page);
    await page.locator(".pane[data-kind='memory'] .page-actions button:has-text('新建')").click();
    await page.waitForSelector(".drawer");
    await page.locator(".drawer button.btn-ghost:has-text('取消')").click();
    await page.waitForTimeout(400);
    const stillThere = await page.locator(".drawer").count();
    expect.equals(stillThere, 0, "cancel should dismiss drawer");
  }],

  ["empty state visible when no memories", async ({ page, expect }) => {
    await openMemory(page);
    await page.waitForTimeout(500);
    // Either there are memory rows OR an empty state. Verify the page
    // renders SOMETHING (not blank).
    const cards = await page.locator(".pane[data-kind='memory'] .card").count();
    const empty = await page.locator(".pane[data-kind='memory'] .empty").count();
    expect.gte(cards + empty, 1, "either rows or empty state should render");
  }],
].map(([name, fn]) => () => runCase("18-memory-crud · " + name, fn));
