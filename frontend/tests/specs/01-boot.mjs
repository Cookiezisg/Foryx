// L1 — boot: app loads, sidebar renders, no console errors.
import { runCase } from "../lib/harness.mjs";

export default [
  ["app boots without console errors", async ({ page, shot, expect }) => {
    await expect.visible(page.locator(".sidebar"));
    await expect.visible(page.locator(".workspace-name"));
    const dashOrChat = await page.locator(".pane, .dash").count();
    expect.gte(dashOrChat, 1, "expected dashboard or chat pane to render at boot");
    await shot("boot");
  }],

  ["sidebar has all primary nav items", async ({ page, expect }) => {
    for (const label of ["对话", "锻造", "执行", "文档"]) {
      await expect.visible(page.locator(`button.nav-item:has-text("${label}")`).first());
    }
    for (const label of ["Skills", "MCP", "Memory"]) {
      await expect.visible(page.locator(`button.nav-item:has-text("${label}")`).first());
    }
  }],

  ["sidebar footer shows SSE status dot + user pill", async ({ page, expect }) => {
    await expect.visible(page.locator(".sidebar .user-pill"));
    await expect.visible(page.locator(".sidebar .user-status"));
  }],
].map(([name, fn]) => () => runCase("01-boot · " + name, fn));
