// BlockRenderer rendering across types — uses backend-driven live data
// from prior chat (07-chat-live seeds a conv with rich block tree).
import { runCase } from "../lib/harness.mjs";
import { backend } from "../lib/backend.mjs";
import { clickConv } from "../lib/helpers.mjs";

async function openSeededConv(page) {
  if (!backend.hasDeepseekKey()) return null;
  const convs = await backend.conversations();
  const list = Array.isArray(convs) ? convs : convs?.items || [];
  // Only pick convs that actually have streamed messages (live-chat-test
  // is seeded by 07-chat-live, blocks-live by 25). composer-test convs
  // are created but never sent to → empty → no blocks to assert against.
  const target = list.find((c) => /live-chat-test|blocks-live/.test(c.title) && c.id);
  if (!target) return null;
  await page.reload({ waitUntil: "domcontentloaded" });
  await page.waitForSelector(".sidebar");
  await clickConv(page, target.title);
  await page.waitForTimeout(800);
  return target;
}

export default [
  ["text block renders msg-body content", async ({ page, expect }) => {
    const t = await openSeededConv(page);
    if (!t) { console.log("    (skipped: no live conv)"); return; }
    const textCount = await page.locator(".blk-text").count();
    expect.gte(textCount, 1, "expect at least 1 text block");
  }],

  ["reasoning block defaults collapsed", async ({ page, expect }) => {
    const t = await openSeededConv(page);
    if (!t) { console.log("    (skipped)"); return; }
    const reasoning = page.locator(".blk-reasoning");
    if (await reasoning.count() === 0) { console.log("    (no reasoning block in seeded data)"); return; }
    const open = await reasoning.first().evaluate((el) => el.classList.contains("is-open"));
    expect.truthy(!open, "reasoning should be collapsed by default");
  }],

  ["reasoning expands on head click", async ({ page, expect }) => {
    const t = await openSeededConv(page);
    if (!t) { console.log("    (skipped)"); return; }
    const reasoning = page.locator(".blk-reasoning").first();
    if (await reasoning.count() === 0) { console.log("    (no reasoning)"); return; }
    await reasoning.locator(".blk-reasoning-head").click();
    await page.waitForTimeout(200);
    const open = await reasoning.evaluate((el) => el.classList.contains("is-open"));
    expect.truthy(open, "reasoning should open after click");
    const body = await reasoning.locator(".blk-reasoning-body").count();
    expect.gte(body, 1, "body should render when expanded");
  }],

  ["tool_call defaults collapsed", async ({ page, expect }) => {
    const t = await openSeededConv(page);
    if (!t) { console.log("    (skipped)"); return; }
    const tool = page.locator(".blk-tool");
    if (await tool.count() === 0) { console.log("    (no tool block)"); return; }
    const open = await tool.first().evaluate((el) => el.classList.contains("is-open"));
    expect.truthy(!open, "tool_call should default collapsed");
  }],

  ["tool_call shows tool name + summary in head", async ({ page, expect }) => {
    const t = await openSeededConv(page);
    if (!t) { console.log("    (skipped)"); return; }
    const tool = page.locator(".blk-tool").first();
    if (await tool.count() === 0) { console.log("    (no tool block)"); return; }
    const nameEl = tool.locator(".blk-tool-name code");
    await nameEl.waitFor({ state: "attached", timeout: 2000 });
    const name = (await nameEl.textContent()).trim();
    expect.truthy(name.length > 0, "tool name should be non-empty");
  }],

  ["expanded tool_call shows Arguments section", async ({ page, expect }) => {
    const t = await openSeededConv(page);
    if (!t) { console.log("    (skipped)"); return; }
    const tool = page.locator(".blk-tool").first();
    if (await tool.count() === 0) { console.log("    (no tool block)"); return; }
    await tool.locator(".blk-tool-head").click();
    await page.waitForTimeout(200);
    const args = await tool.locator(".blk-tool-section:has-text('Arguments')").count();
    expect.equals(args, 1, "Arguments section should render in expanded tool_call");
  }],

  ["day-divider appears once per chat-stream", async ({ page, expect }) => {
    const t = await openSeededConv(page);
    if (!t) { console.log("    (skipped)"); return; }
    const dividers = await page.locator(".day-divider").count();
    expect.equals(dividers, 1, "expect exactly 1 day-divider per stream");
  }],

  ["msg-meta + msg-body present on each message", async ({ page, expect }) => {
    const t = await openSeededConv(page);
    if (!t) { console.log("    (skipped)"); return; }
    const msgs = await page.locator(".msg").count();
    const metas = await page.locator(".msg-meta").count();
    const bodies = await page.locator(".msg-body").count();
    expect.equals(metas, msgs, "each msg must have msg-meta");
    expect.equals(bodies, msgs, "each msg must have msg-body");
  }],
].map(([name, fn]) => () => runCase("14-blocks · " + name, fn));
