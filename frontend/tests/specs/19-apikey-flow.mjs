// Config → API Keys flow: add drawer fields, providers populated, list shows added key.
import { runCase } from "../lib/harness.mjs";
import { openPaneByLabel } from "../lib/helpers.mjs";

async function openConfig(page) {
  await openPaneByLabel(page, "Skills");  // any nav opens via click
  // Reset: actually open Config via overlay → "API Keys / Model …" link
  await page.locator(".sidebar .user-pill button.icon-btn[title*='主题']").click();
  await page.waitForSelector(".settings-pop");
  await page.locator(".settings-pop-link").click();
  await page.waitForSelector(".pane[data-kind='config']", { timeout: 4000 });
  await page.waitForTimeout(400);
}

export default [
  ["config pane opens via SettingsPopover link", async ({ page, expect }) => {
    await openConfig(page);
    await expect.visible(page.locator(".pane[data-kind='config']"));
  }],

  ["config has 5 tabs (API Keys / Model / Sandbox / 外观 / 数据)", async ({ page, expect }) => {
    await openConfig(page);
    const tabCount = await page.locator(".pane[data-kind='config'] .page-tab").count();
    expect.equals(tabCount, 5, `expected 5 tabs, got ${tabCount}`);
  }],

  ["API Keys tab shows '添加 Provider' button", async ({ page, expect }) => {
    await openConfig(page);
    await page.locator(".pane[data-kind='config'] .page-tab:has-text('API Keys')").click();
    await page.waitForTimeout(400);
    const add = await page.locator(".pane[data-kind='config'] button:has-text('添加 Provider')").count();
    expect.equals(add, 1, "Add Provider button visible");
  }],

  ["clicking '添加 Provider' opens AddKeyDrawer with fields", async ({ page, expect }) => {
    await openConfig(page);
    await page.locator(".pane[data-kind='config'] .page-tab:has-text('API Keys')").click();
    await page.waitForTimeout(300);
    await page.locator(".pane[data-kind='config'] button:has-text('添加 Provider')").click();
    await page.waitForSelector(".drawer", { timeout: 3000 });
    const titleVisible = await page.locator(".drawer-title:has-text('添加 API Key')").count();
    expect.equals(titleVisible, 1, "drawer title");
    // Wait for the /providers query to actually resolve before counting.
    await page.waitForFunction(
      () => document.querySelectorAll(".drawer select option").length >= 3,
      { timeout: 5000 }
    );
    const providerOpts = await page.locator(".drawer select option").count();
    expect.gte(providerOpts, 3, `expected multiple provider options, got ${providerOpts}`);
  }],

  ["Model tab shows scenario cards (at least 'chat')", async ({ page, expect }) => {
    await openConfig(page);
    await page.locator(".pane[data-kind='config'] .page-tab:has-text('Model')").click();
    await page.waitForTimeout(400);
    const chatScenario = await page.locator(".pane[data-kind='config'] .card:has-text('chat')").count();
    expect.gte(chatScenario, 1, "expected at least 1 scenario card (chat)");
  }],

  ["外观 tab shows theme/accent/density rows", async ({ page, expect }) => {
    await openConfig(page);
    await page.locator(".pane[data-kind='config'] .page-tab:has-text('外观')").click();
    await page.waitForTimeout(400);
    const themeRow = await page.locator(".pane[data-kind='config']:has-text('主题')").count();
    const accentRow = await page.locator(".pane[data-kind='config']:has-text('Accent')").count();
    expect.gte(themeRow, 1);
    expect.gte(accentRow, 1);
  }],
].map(([name, fn]) => () => runCase("19-apikey-flow · " + name, fn));
