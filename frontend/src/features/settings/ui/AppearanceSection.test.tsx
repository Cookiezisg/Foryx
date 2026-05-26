// AppearanceSection — segmented controls write live to settingsStore; swatch
// clicks change accent.

import { beforeEach, describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useSettingsStore } from "@entities/settings/model/settingsStore";
import { AppearanceSection } from "./AppearanceSection.tsx";

beforeEach(() => {
  useSettingsStore.setState({
    theme: "system",
    accent: "claude",
    density: "cozy",
    lang: "zh",
    reasoningDefault: "collapsed",
  });
});

const renderOpen = () => render(<AppearanceSection open onToggle={() => {}} />);

describe("AppearanceSection", () => {
  it("themeSegment_darkClick_writesThemeDark", async () => {
    renderOpen();
    await userEvent.click(screen.getByText("深色"));
    expect(useSettingsStore.getState().theme).toBe("dark");
  });

  it("themeSegment_systemActive_byDefault", () => {
    renderOpen();
    const btn = screen.getByText("跟随系统");
    expect(btn.className).toContain("is-active");
  });

  it("accentSwatch_click_changesAccent", async () => {
    renderOpen();
    const swatches = document.querySelectorAll(".onb-swatch");
    // Second swatch is "blue" (#2383e2)
    await userEvent.click(swatches[1]);
    expect(useSettingsStore.getState().accent).toBe("blue");
  });

  it("accentSwatch_activeSwatch_hasIsActiveClass", () => {
    renderOpen();
    const swatches = document.querySelectorAll(".onb-swatch");
    // Default accent is "claude" — first swatch
    expect(swatches[0].className).toContain("is-active");
    expect(swatches[1].className).not.toContain("is-active");
  });

  it("densitySegment_compactClick_writesDensityCompact", async () => {
    renderOpen();
    await userEvent.click(screen.getByText("紧凑"));
    expect(useSettingsStore.getState().density).toBe("compact");
  });

  it("langSegment_englishClick_writesLangEn", async () => {
    renderOpen();
    await userEvent.click(screen.getByText("English"));
    expect(useSettingsStore.getState().lang).toBe("en");
  });

  it("reasoningSegment_expandedClick_writesReasoningExpanded", async () => {
    renderOpen();
    await userEvent.click(screen.getByText("默认展开"));
    expect(useSettingsStore.getState().reasoningDefault).toBe("expanded");
  });

  it("closed_rendersNoRows", () => {
    render(<AppearanceSection open={false} onToggle={() => {}} />);
    expect(screen.queryByText("主题")).not.toBeInTheDocument();
  });
});
