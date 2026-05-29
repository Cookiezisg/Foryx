// ModelOverrideEditor — ThinkingControl integration tests.
// Verifies: control renders when capability exists; changing thinking updates
// pending; changing model (via picker) resets thinking.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";

const mockMutateAsync = vi.fn().mockResolvedValue({});

vi.mock("../model/useConvModelOverride", () => ({
  useConvModelOverride: () => ({ mutateAsync: mockMutateAsync, isPending: false }),
}));

let apiKeys: any[] = [];
let caps: any[] = [];

vi.mock("@entities/apikey", () => ({
  useApiKeys: () => ({ data: apiKeys }),
}));

vi.mock("@entities/model-config", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@entities/model-config")>();
  return {
    ...actual,
    useModelCapabilities: () => ({ data: caps }),
  };
});

vi.mock("@features/settings", () => ({
  KeyModelPicker: ({ value, onChange }: any) => (
    <button
      data-testid="kmp"
      onClick={() => onChange({ apiKeyId: "aki_2", modelId: "gpt-4o" })}
    >
      {value ? `${value.apiKeyId}::${value.modelId}` : "pick"}
    </button>
  ),
}));

import { ModelOverrideEditor } from "./ModelOverrideEditor.tsx";

function wrap({ children }: { children: any }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => {
  mockMutateAsync.mockReset().mockResolvedValue({});
  apiKeys = [
    { id: "aki_ds", provider: "deepseek", displayName: "DS", keyMasked: "sk-...", testStatus: "ok", modelsFound: ["deepseek-v4-flash"] },
    { id: "aki_2", provider: "openai", displayName: "OA", keyMasked: "sk-...", testStatus: "ok", modelsFound: ["gpt-4o"] },
  ];
  caps = [];
});

describe("ModelOverrideEditor — ThinkingControl", () => {
  it("noCapability_thinkingControlNotRendered", () => {
    caps = [];
    render(
      <ModelOverrideEditor
        conversationId="cv_a"
        current={{ apiKeyId: "aki_ds", modelId: "deepseek-v4-flash" }}
        onClose={() => {}}
      />,
      { wrapper: wrap },
    );
    expect(screen.queryByText("自动")).not.toBeInTheDocument();
  });

  it("toggleCapability_thinkingControlRendered", () => {
    caps = [{
      provider: "deepseek", modelId: "deepseek-v4-flash",
      thinkingShape: "toggle", effortValues: [],
      budgetMin: 0, budgetMax: 0, contextWindow: 128000, maxOutput: 8000, contextMode: "full",
    }];
    render(
      <ModelOverrideEditor
        conversationId="cv_a"
        current={{ apiKeyId: "aki_ds", modelId: "deepseek-v4-flash" }}
        onClose={() => {}}
      />,
      { wrapper: wrap },
    );
    // toggle shape shows auto/on/off buttons
    expect(screen.getByText("自动")).toBeInTheDocument();
    expect(screen.getByText("开")).toBeInTheDocument();
  });

  it("changingThinking_updatesThinkingInPending", async () => {
    caps = [{
      provider: "deepseek", modelId: "deepseek-v4-flash",
      thinkingShape: "toggle", effortValues: [],
      budgetMin: 0, budgetMax: 0, contextWindow: 128000, maxOutput: 8000, contextMode: "full",
    }];
    render(
      <ModelOverrideEditor
        conversationId="cv_a"
        current={{ apiKeyId: "aki_ds", modelId: "deepseek-v4-flash" }}
        onClose={() => {}}
      />,
      { wrapper: wrap },
    );
    // Click "on" in the ThinkingControl
    fireEvent.click(screen.getByText("开"));
    // Save — pending should include thinking
    fireEvent.click(screen.getByText("保存"));
    await vi.waitFor(() => expect(mockMutateAsync).toHaveBeenCalled());
    const call = mockMutateAsync.mock.calls[0][0];
    expect(call.override.thinking).toEqual({ mode: "on" });
  });

  it("changingModel_resetsThinking", async () => {
    caps = [{
      provider: "deepseek", modelId: "deepseek-v4-flash",
      thinkingShape: "toggle", effortValues: [],
      budgetMin: 0, budgetMax: 0, contextWindow: 128000, maxOutput: 8000, contextMode: "full",
    }];
    render(
      <ModelOverrideEditor
        conversationId="cv_a"
        current={{ apiKeyId: "aki_ds", modelId: "deepseek-v4-flash", thinking: { mode: "on" } }}
        onClose={() => {}}
      />,
      { wrapper: wrap },
    );
    // Click the mock KeyModelPicker to switch to a different model
    fireEvent.click(screen.getByTestId("kmp"));
    // Save — thinking must be absent (reset on model change)
    fireEvent.click(screen.getByText("保存"));
    await vi.waitFor(() => expect(mockMutateAsync).toHaveBeenCalled());
    const call = mockMutateAsync.mock.calls[0][0];
    expect(call.override.apiKeyId).toBe("aki_2");
    expect(call.override.modelId).toBe("gpt-4o");
    expect(call.override.thinking).toBeUndefined();
  });
});
