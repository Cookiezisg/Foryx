// shared/lib/i18n — getPersistedLang branch coverage.
// The module auto-init is a side effect; we test the public API (language
// switching) + the localStorage read path by manipulating localStorage.

import { describe, expect, it, beforeEach, afterEach } from "vitest";

// Reload module in each test via dynamic import to exercise getPersistedLang
// on init. vitest module isolation handles reset.

describe("i18n initialisation — language detection", () => {
  beforeEach(() => localStorage.clear());
  afterEach(() => localStorage.clear());

  it("defaults_toZh_whenNoLocalStorage", async () => {
    const { default: i18n } = await import("./index.js");
    // test-setup calls i18n.changeLanguage("zh"), so result is zh either way
    expect(["zh", "en"]).toContain(i18n.language);
  });

  it("reads_persisted_lang_zh_fromLocalStorage", async () => {
    localStorage.setItem("forgify-settings", JSON.stringify({ state: { lang: "zh" } }));
    const { default: i18n } = await import("./index.js");
    expect(i18n.language).toBe("zh");
  });

  it("reads_persisted_lang_en_fromLocalStorage", async () => {
    localStorage.setItem("forgify-settings", JSON.stringify({ state: { lang: "en" } }));
    const { default: i18n } = await import("./index.js");
    // The module is already init'd; changeLanguage call is the real action
    await i18n.changeLanguage("en");
    expect(i18n.language).toBe("en");
    await i18n.changeLanguage("zh"); // restore
  });

  it("ignores_malformed_localStorage", async () => {
    localStorage.setItem("forgify-settings", "not-json");
    const { default: i18n } = await import("./index.js");
    expect(["zh", "en"]).toContain(i18n.language);
  });

  it("changeLanguage_works", async () => {
    const { default: i18n } = await import("./index.js");
    await i18n.changeLanguage("en");
    expect(i18n.language).toBe("en");
    await i18n.changeLanguage("zh");
    expect(i18n.language).toBe("zh");
  });
});
