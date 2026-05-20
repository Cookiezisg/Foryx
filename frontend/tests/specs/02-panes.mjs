// L1+L2 — open every pane, verify it renders, close it.
import { runCase } from "../lib/harness.mjs";

const PANES = [
  ["forge",     "锻造"],
  ["execute",   "执行"],
  ["documents", "文档"],
  ["skills",    "Skills"],
  ["mcp",       "MCP"],
  ["memory",    "Memory"],
];

const cases = PANES.flatMap(([kind, label]) => [
  [`open ${kind} pane`, async ({ page, shot, expect }) => {
    await page.locator(`button.nav-item:has-text("${label}")`).first().click();
    await page.waitForSelector(`.pane[data-kind="${kind}"]`, { timeout: 5000 });
    await expect.visible(page.locator(`.pane[data-kind="${kind}"]`));
    await shot(kind);
  }],
  [`close ${kind} pane`, async ({ page, expect }) => {
    await page.locator(`button.nav-item:has-text("${label}")`).first().click();
    await page.waitForTimeout(500);
    const closed = await page.locator(`button.nav-item:has-text("${label}")`).first().click();
    await page.waitForTimeout(400);
    const stillThere = await page.locator(`.pane[data-kind="${kind}"]`).count();
    expect.equals(stillThere, 0, "pane should close after second nav click");
  }],
]);

export default cases.map(([name, fn]) => () => runCase("02-panes · " + name, fn));
