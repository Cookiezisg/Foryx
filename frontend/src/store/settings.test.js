// store/settings — session-state defaults and persistence.
// activeUserId has migrated to entities/session.
// Preference fields (theme/accent/density/lang/reasoningDefault) are in
// entities/settings/model/settingsStore.test.ts; helpers (resolveTheme,
// applyTheme) are tested there as well.

import { afterEach, beforeEach, describe, expect, it } from "vitest";

// Mirrors the private detectLang() in entities/settings; used to assert
// default lang in this environment.
function detectLang() {
  if (typeof navigator === "undefined") return "zh";
  const l = (navigator.language || "").toLowerCase();
  return l.startsWith("zh") ? "zh" : "en";
}

beforeEach(() => {
  localStorage.clear();
  vi.resetModules();
});

afterEach(() => {
  delete document.documentElement.dataset.theme;
  delete document.documentElement.dataset.accent;
  delete document.documentElement.dataset.density;
  delete document.documentElement.dataset.lang;
});

describe("useSettings (session store)", () => {
  it("useSettings_defaults_matchSpec", async () => {
    const { useSettings } = await import("./settings.js");
    const s = useSettings.getState();
    expect(s.onboarded).toBe(false);
    expect(s.leftPct).toBe(50);
  });

  it("set_mergesPartialPatch", async () => {
    const { useSettings } = await import("./settings.js");
    useSettings.getState().set({ onboarded: true });
    const s = useSettings.getState();
    expect(s.onboarded).toBe(true);
    expect(s.leftPct).toBe(50);
  });

  it("reset_restoresDefaults", async () => {
    const { useSettings } = await import("./settings.js");
    useSettings.getState().set({ onboarded: true, leftPct: 70 });
    useSettings.getState().reset();
    const s = useSettings.getState();
    expect(s.onboarded).toBe(false);
    expect(s.leftPct).toBe(50);
  });

  it("persist_writesToLocalStorage", async () => {
    const { useSettings } = await import("./settings.js");
    useSettings.getState().set({ onboarded: true });
    const stored = localStorage.getItem("forgify-ui");
    expect(stored).toBeTruthy();
    expect(JSON.parse(stored).state.onboarded).toBe(true);
  });
});

describe("useSettingsStore (preference store)", () => {
  it("defaults_matchSpec", async () => {
    const { useSettingsStore } = await import("../entities/settings/model/settingsStore.ts");
    const s = useSettingsStore.getState();
    expect(s.theme).toBe("system");
    expect(s.accent).toBe("claude");
    expect(s.density).toBe("cozy");
    expect(s.lang).toBe(detectLang());
    expect(s.reasoningDefault).toBe("collapsed");
  });

  it("set_mergesPartialPatch", async () => {
    const { useSettingsStore } = await import("../entities/settings/model/settingsStore.ts");
    useSettingsStore.getState().set({ theme: "dark", accent: "blue" });
    const s = useSettingsStore.getState();
    expect(s.theme).toBe("dark");
    expect(s.accent).toBe("blue");
    expect(s.density).toBe("cozy");
  });

  it("reset_restoresDefaults", async () => {
    const { useSettingsStore } = await import("../entities/settings/model/settingsStore.ts");
    useSettingsStore.getState().set({ theme: "dark", lang: "en" });
    useSettingsStore.getState().reset();
    const s = useSettingsStore.getState();
    expect(s.theme).toBe("system");
    expect(s.lang).toBe(detectLang());
  });

  it("persist_writesToLocalStorage", async () => {
    const { useSettingsStore } = await import("../entities/settings/model/settingsStore.ts");
    useSettingsStore.getState().set({ accent: "green" });
    const stored = localStorage.getItem("forgify-settings");
    expect(stored).toBeTruthy();
    expect(JSON.parse(stored).state.accent).toBe("green");
  });
});

describe("resolveTheme", () => {
  it("resolveTheme_lightOrDark_passesThrough", async () => {
    const { resolveTheme } = await import("../entities/settings/model/settingsStore.ts");
    expect(resolveTheme("light")).toBe("light");
    expect(resolveTheme("dark")).toBe("dark");
  });

  it("resolveTheme_system_collapsesViaMediaQuery", async () => {
    window.matchMedia = vi.fn().mockReturnValue({ matches: true });
    const { resolveTheme } = await import("../entities/settings/model/settingsStore.ts");
    expect(resolveTheme("system")).toBe("dark");
    window.matchMedia = vi.fn().mockReturnValue({ matches: false });
    expect(resolveTheme("system")).toBe("light");
  });
});

describe("detectLang (device language detection)", () => {
  afterEach(() => vi.unstubAllGlobals());
  it("zh-CN_returns_zh", () => {
    vi.stubGlobal("navigator", { language: "zh-CN" });
    expect(detectLang()).toBe("zh");
  });
  it("en-US_returns_en", () => {
    vi.stubGlobal("navigator", { language: "en-US" });
    expect(detectLang()).toBe("en");
  });
  it("fr-FR_returns_en", () => {
    vi.stubGlobal("navigator", { language: "fr-FR" });
    expect(detectLang()).toBe("en");
  });
});

describe("applyTheme", () => {
  it("applyTheme_writesDatasetAttrs", async () => {
    const { applyTheme } = await import("../entities/settings/model/settingsStore.ts");
    applyTheme({ theme: "dark", accent: "blue", density: "compact", lang: "en" });
    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(document.documentElement.dataset.accent).toBe("blue");
    expect(document.documentElement.dataset.density).toBe("compact");
    expect(document.documentElement.dataset.lang).toBe("en");
  });

  it("applyTheme_systemTheme_resolvesBeforeWriting", async () => {
    window.matchMedia = vi.fn().mockReturnValue({ matches: true });
    const { applyTheme } = await import("../entities/settings/model/settingsStore.ts");
    applyTheme({ theme: "system", accent: "blue", density: "cozy", lang: "zh" });
    expect(document.documentElement.dataset.theme).toBe("dark");
  });
});
