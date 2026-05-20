// Multi-pane: max 2, opening 3rd evicts the oldest, both pane chromes ok.
import { runCase } from "../lib/harness.mjs";
import { openPaneByLabel } from "../lib/helpers.mjs";

export default [
  ["1 → 2 panes side-by-side", async ({ page, expect }) => {
    await openPaneByLabel(page, "锻造");
    await page.waitForSelector(".pane[data-kind='forge']");
    const count = await page.locator(".pane-wrap").count();
    expect.equals(count, 2, "expect 2 pane wrappers (chat + forge)");
  }],

  ["opening 3rd pane evicts the first (chat) and keeps later 2", async ({ page, expect }) => {
    await openPaneByLabel(page, "锻造");
    await page.waitForSelector(".pane[data-kind='forge']");
    await openPaneByLabel(page, "执行");
    await page.waitForSelector(".pane[data-kind='execute']");
    await page.waitForTimeout(400);
    const chat = await page.locator(".pane[data-kind='chat']").count();
    const forge = await page.locator(".pane[data-kind='forge']").count();
    const execute = await page.locator(".pane[data-kind='execute']").count();
    expect.equals(chat, 0, "chat (oldest) should be evicted");
    expect.equals(forge, 1, "forge should remain");
    expect.equals(execute, 1, "execute should be open");
  }],

  ["closing all panes reveals Dashboard", async ({ page, expect }) => {
    await page.locator("button.nav-item:has-text('对话')").first().click();  // close chat
    await page.waitForTimeout(400);
    const panes = await page.locator(".pane").count();
    expect.equals(panes, 0, "no panes should be open");
    const dash = await page.locator(".dash").count();
    expect.equals(dash, 1, "dashboard should render when no panes");
  }],

  ["pane-bar shows correct icon + breadcrumb per kind", async ({ page, expect }) => {
    await page.locator("button.nav-item:has-text('对话')").first().click();  // close chat first
    await page.waitForTimeout(300);
    await openPaneByLabel(page, "锻造");
    await page.waitForSelector(".pane[data-kind='forge'] .pane-bar");
    const crumbText = await page.locator(".pane[data-kind='forge'] .pane-crumbs .cur").textContent();
    expect.equals(crumbText, "锻造", "forge crumb should read 锻造");
  }],

  ["chat pane has NO .pane-bar (own header)", async ({ page, expect }) => {
    const chatBar = await page.locator(".pane[data-kind='chat'] .pane-bar").count();
    expect.equals(chatBar, 0, "chat pane intentionally has no pane-bar");
  }],
].map(([name, fn]) => () => runCase("11-multi-pane · " + name, fn));
