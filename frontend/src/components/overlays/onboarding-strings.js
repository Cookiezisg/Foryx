// Onboarding UI constants (accent swatches, provider hints, default models).
// Copy lives in i18n/locales/{zh,en}/onboarding.json.
//
// 引导界面常量(色板、厂商标注、默认模型)。文案已迁入 i18n namespace。

export const ACCENTS = [
  ["claude", "#d97757"],
  ["blue", "#2383e2"],
  ["ink", "#37352f"],
  ["green", "#0f7b6c"],
  ["purple", "#6940a5"],
];

// LLM provider chips (abbr + brand color). Keyed by backend provider `name`.
export const LLM_HINTS = {
  deepseek: { abbr: "DS", color: "#4D6BFE" },
  openai: { abbr: "OA", color: "#10A37F" },
  anthropic: { abbr: "AN", color: "#D97757" },
  google: { abbr: "GO", color: "#4285F4" },
  qwen: { abbr: "QW", color: "#615CED" },
  zhipu: { abbr: "ZP", color: "#3870E0" },
  moonshot: { abbr: "MS", color: "#37352F" },
  ollama: { abbr: "OL", color: "#6b6459" },
};

export const SEARCH_HINTS = {
  bocha: { abbr: "BC", color: "#1f9d55" },
  brave: { abbr: "BR", color: "#fb542b" },
  serper: { abbr: "SE", color: "#5436da" },
  tavily: { abbr: "TV", color: "#0f7b6c" },
};

// Fallback model id used ONLY when :test returns no modelsFound (e.g.
// Anthropic ping). Must be a real, runnable id.
export const PROVIDER_DEFAULT_MODEL = {
  anthropic: "claude-sonnet-4-6",
};

