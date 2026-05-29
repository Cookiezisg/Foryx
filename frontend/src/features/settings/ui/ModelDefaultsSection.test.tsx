// ModelDefaultsSection — 3 expandable scenario cards; provider grid +
// (key, model) cascade picker per card. Includes ThinkingControl integration.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";

const mockUpsertModel = vi.fn();
const mockSetOverride = vi.fn();
const mockClearOverride = vi.fn();
let apiKeys: any[] = [];
let modelConfigs: any[] = [];
let providers: any[] = [];
let modelCapabilities: any[] = [];

vi.mock("@entities/model-config", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@entities/model-config")>();
  return {
    ...actual,
    useModelConfigs: () => ({ data: modelConfigs }),
    useUpsertModelConfig: () => ({ mutate: mockUpsertModel, mutateAsync: mockUpsertModel, isPending: false }),
    useProviders: () => ({ data: providers }),
    useModelCapabilities: () => ({ data: modelCapabilities }),
    useSetModelCapabilityOverride: () => ({ mutate: mockSetOverride, isPending: false }),
    useClearModelCapabilityOverride: () => ({ mutate: mockClearOverride, isPending: false }),
  };
});

vi.mock("@entities/apikey", () => ({
  useApiKeys: () => ({ data: apiKeys }),
}));

import { ModelDefaultsSection } from "./ModelDefaultsSection.tsx";

function wrap({ children }: { children: any }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

// Real-world dev DB shape: one verified DeepSeek key with 2 models, all 3
// scenarios pointing at flash. Used by most tests.
function seedStandardKeys() {
  apiKeys = [{
    id: "aki_ds", provider: "deepseek",
    displayName: "deepseek (onboarding)",
    keyMasked: "sk-9e1b...a366",
    testStatus: "ok",
    modelsFound: ["deepseek-v4-flash", "deepseek-v4-pro"],
  }];
  providers = [{ name: "deepseek", displayName: "DeepSeek", category: "llm" }];
  modelConfigs = [
    { scenario: "dialogue", apiKeyId: "aki_ds", modelId: "deepseek-v4-flash" },
    { scenario: "utility",  apiKeyId: "aki_ds", modelId: "deepseek-v4-flash" },
    { scenario: "agent",    apiKeyId: "aki_ds", modelId: "deepseek-v4-flash" },
  ];
}

beforeEach(() => {
  mockUpsertModel.mockReset().mockResolvedValue({});
  mockSetOverride.mockReset();
  mockClearOverride.mockReset();
  apiKeys = [];
  modelConfigs = [];
  providers = [];
  modelCapabilities = [];
});

describe("ModelDefaultsSection", () => {
  it("open_rendersThreeScenarioRows", () => {
    seedStandardKeys();
    render(<ModelDefaultsSection open={true} onToggle={() => {}} />, { wrapper: wrap });
    expect(screen.getByText("主对话")).toBeInTheDocument();
    expect(screen.getByText("辅助任务")).toBeInTheDocument();
    expect(screen.getByText("Agent")).toBeInTheDocument();
  });

  it("closed_doesNotRenderRows", () => {
    seedStandardKeys();
    render(<ModelDefaultsSection open={false} onToggle={() => {}} />, { wrapper: wrap });
    expect(screen.queryByText("主对话")).not.toBeInTheDocument();
  });

  it("noKeys_showsEmptyPlaceholderInsteadOfCards", () => {
    render(<ModelDefaultsSection open={true} onToggle={() => {}} />, { wrapper: wrap });
    expect(screen.getByText("尚未配 API Key")).toBeInTheDocument();
    // No scenario cards when there's nothing to pick.
    expect(screen.queryByText("主对话")).not.toBeInTheDocument();
  });

  it("titleAndSubtitle_visible", () => {
    render(<ModelDefaultsSection open={true} onToggle={() => {}} />, { wrapper: wrap });
    expect(screen.getByText("模型默认")).toBeInTheDocument();
    expect(screen.getByText("各场景独立配置")).toBeInTheDocument();
  });

  it("eachRowSummary_showsCurrentModelId", () => {
    seedStandardKeys();
    render(<ModelDefaultsSection open={true} onToggle={() => {}} />, { wrapper: wrap });
    // Every row's collapsed/expanded summary shows the modelId; none shows pro.
    for (const label of ["主对话", "辅助任务", "Agent"]) {
      const row = screen.getByText(label).closest(".set-mc") as HTMLElement;
      expect(row).not.toBeNull();
      expect(row.textContent).toContain("deepseek-v4-flash");
      expect(row.textContent).not.toContain("deepseek-v4-pro");
    }
  });

  it("dialogueOpenByDefault_otherRowsCollapsed", () => {
    seedStandardKeys();
    render(<ModelDefaultsSection open={true} onToggle={() => {}} />, { wrapper: wrap });
    const dialogue = screen.getByText("主对话").closest(".set-mc") as HTMLElement;
    const utility = screen.getByText("辅助任务").closest(".set-mc") as HTMLElement;
    expect(dialogue.className).toContain("is-open");
    expect(utility.className).not.toContain("is-open");
  });

  it("clickProvider_callsUpsertWithFirstKeyAndFirstModel", () => {
    apiKeys = [
      { id: "aki_ds", provider: "deepseek", displayName: "DS", keyMasked: "sk-...1",
        testStatus: "ok", modelsFound: ["deepseek-v4-flash", "deepseek-v4-pro"] },
      { id: "aki_oa", provider: "openai", displayName: "OA", keyMasked: "sk-...2",
        testStatus: "ok", modelsFound: ["gpt-4o-mini", "gpt-4o"] },
    ];
    providers = [
      { name: "deepseek", displayName: "DeepSeek", category: "llm" },
      { name: "openai", displayName: "OpenAI", category: "llm" },
    ];
    modelConfigs = [{ scenario: "dialogue", apiKeyId: "aki_ds", modelId: "deepseek-v4-flash" }];
    render(<ModelDefaultsSection open={true} onToggle={() => {}} />, { wrapper: wrap });
    // Switch dialogue from deepseek to openai by clicking the OpenAI provider chip.
    const openaiChip = screen.getByText("OpenAI");
    fireEvent.click(openaiChip);
    expect(mockUpsertModel).toHaveBeenCalledWith({
      scenario: "dialogue", apiKeyId: "aki_oa", modelId: "gpt-4o-mini",
    });
  });

  it("effortCapability_rendersThinkingControl", () => {
    seedStandardKeys();
    modelCapabilities = [{
      provider: "deepseek", modelId: "deepseek-v4-flash",
      thinkingShape: "effort", effortValues: ["low", "medium", "high"],
      budgetMin: 0, budgetMax: 0, contextWindow: 128000, maxOutput: 8000, contextMode: "full",
    }];
    render(<ModelDefaultsSection open={true} onToggle={() => {}} />, { wrapper: wrap });
    // The expanded dialogue card should show the thinking effort label.
    expect(screen.getByText("思考强度")).toBeInTheDocument();
  });

  it("effortCapability_changingThinking_callsUpsertWithThinking", () => {
    seedStandardKeys();
    modelCapabilities = [{
      provider: "deepseek", modelId: "deepseek-v4-flash",
      thinkingShape: "effort", effortValues: ["low", "medium", "high"],
      budgetMin: 0, budgetMax: 0, contextWindow: 128000, maxOutput: 8000, contextMode: "full",
    }];
    render(<ModelDefaultsSection open={true} onToggle={() => {}} />, { wrapper: wrap });
    // Open the Select for thinking effort.
    const trigger = screen.getByRole("button", { name: "思考强度" });
    fireEvent.click(trigger);
    fireEvent.click(screen.getByText("high"));
    expect(mockUpsertModel).toHaveBeenCalledWith({
      scenario: "dialogue",
      apiKeyId: "aki_ds",
      modelId: "deepseek-v4-flash",
      thinking: { mode: "on", effort: "high" },
    });
  });

  it("expandedCard_withModel_showsCapOverrideTrigger", () => {
    seedStandardKeys();
    render(<ModelDefaultsSection open={true} onToggle={() => {}} />, { wrapper: wrap });
    // Dialogue card is open by default; the override trigger link must be visible.
    expect(screen.getByText("能力不对？覆盖")).toBeInTheDocument();
  });

  it("changingModel_callsUpsertWithoutThinking", () => {
    // When model is switched via cascade the upsert must omit thinking so
    // any previous budget/effort setting does not pollute the new model.
    apiKeys = [{
      id: "aki_ds", provider: "deepseek", displayName: "DS", keyMasked: "sk-...1",
      testStatus: "ok", modelsFound: ["deepseek-v4-flash", "deepseek-v4-pro"],
    }];
    providers = [{ name: "deepseek", displayName: "DeepSeek", category: "llm" }];
    modelConfigs = [{
      scenario: "dialogue", apiKeyId: "aki_ds", modelId: "deepseek-v4-flash",
      thinking: { mode: "on", effort: "high" },
    }];
    modelCapabilities = [{
      provider: "deepseek", modelId: "deepseek-v4-flash",
      thinkingShape: "effort", effortValues: ["low", "medium", "high"],
      budgetMin: 0, budgetMax: 0, contextWindow: 128000, maxOutput: 8000, contextMode: "full",
    }];
    render(<ModelDefaultsSection open={true} onToggle={() => {}} />, { wrapper: wrap });
    // Open model Select and pick the other model.
    const modelTrigger = screen.getByRole("button", { name: "模型" });
    fireEvent.click(modelTrigger);
    fireEvent.click(screen.getByText("deepseek-v4-pro"));
    // Must NOT include thinking.
    expect(mockUpsertModel).toHaveBeenCalledWith({
      scenario: "dialogue",
      apiKeyId: "aki_ds",
      modelId: "deepseek-v4-pro",
    });
    const call = mockUpsertModel.mock.calls[0][0];
    expect(call).not.toHaveProperty("thinking");
  });
});
