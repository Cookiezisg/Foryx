// Theme / accent / density matrix — every combination should mount
// without console errors and the data-attrs should propagate to <html>.
import { runCase } from "../lib/harness.mjs";
import { getDataAttr } from "../lib/helpers.mjs";

const THEMES = ["light", "dark"];
const ACCENTS = ["claude", "blue", "ink", "green", "purple"];
const DENSITIES = ["compact", "cozy", "comfortable"];

async function applySettings(page, { theme, accent, density }) {
  // Open settings popover
  await page.locator(".sidebar .user-pill button.icon-btn[title*='主题']").click();
  await page.waitForSelector(".settings-pop", { timeout: 3000 });
  // theme button
  await page.locator(`.settings-pop-row:has-text("主题") button:has-text("${themeLabel(theme)}")`).click();
  await page.waitForTimeout(150);
  // accent swatch
  await page.locator(`.settings-pop-swatch[title="${accent}"]`).click();
  await page.waitForTimeout(150);
  // density
  await page.locator(`.settings-pop-row:has-text("密度") button:has-text("${densityLabel(density)}")`).click();
  await page.waitForTimeout(150);
  // dismiss popover
  await page.mouse.click(800, 400);
  await page.waitForTimeout(200);
}
const themeLabel = (k) => ({ system: "系统", light: "明", dark: "暗" }[k]);
const densityLabel = (k) => ({ compact: "紧凑", cozy: "适中", comfortable: "舒展" }[k]);

const cases = [];

// Sample a representative subset rather than full 30 combos for speed.
// (Full matrix can be enabled via FULL_THEME_MATRIX env if user wants.)
const sample = process.env.FULL_THEME_MATRIX
  ? THEMES.flatMap((t) => ACCENTS.flatMap((a) => DENSITIES.map((d) => [t, a, d])))
  : [
      ["light", "claude", "cozy"],
      ["dark", "claude", "cozy"],
      ["light", "blue", "compact"],
      ["dark", "ink", "comfortable"],
      ["light", "green", "cozy"],
      ["dark", "purple", "compact"],
    ];

for (const [theme, accent, density] of sample) {
  cases.push([
    `theme=${theme} accent=${accent} density=${density} applies cleanly`,
    async ({ page, shot, expect }) => {
      await applySettings(page, { theme, accent, density });
      expect.equals(await getDataAttr(page, "theme"), theme, "theme dataset");
      expect.equals(await getDataAttr(page, "accent"), accent, "accent dataset");
      expect.equals(await getDataAttr(page, "density"), density, "density dataset");
      await shot(`${theme}-${accent}-${density}`);
    },
  ]);
}

// Bonus: apply → reload → settings preserved.
cases.push([
  "dark / purple / comfortable survives reload",
  async ({ page, expect }) => {
    await applySettings(page, { theme: "dark", accent: "purple", density: "comfortable" });
    await page.reload({ waitUntil: "domcontentloaded" });
    await page.waitForSelector(".sidebar");
    await page.waitForTimeout(400);
    expect.equals(await getDataAttr(page, "theme"), "dark");
    expect.equals(await getDataAttr(page, "accent"), "purple");
    expect.equals(await getDataAttr(page, "density"), "comfortable");
  },
]);

export default cases.map(([name, fn]) => () => runCase("09-themes · " + name, fn));
