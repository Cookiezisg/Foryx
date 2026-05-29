// ModelCapOverrideEditor — opens on trigger; save / restore call their
// respective mutations; fields are prefilled from `current`.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";

const mockSetOverride = vi.fn();
const mockClearOverride = vi.fn();

vi.mock("@entities/model-config", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@entities/model-config")>();
  return {
    ...actual,
    useSetModelCapabilityOverride: () => ({ mutate: mockSetOverride, isPending: false }),
    useClearModelCapabilityOverride: () => ({ mutate: mockClearOverride, isPending: false }),
  };
});

import { ModelCapOverrideEditor } from "./ModelCapOverrideEditor.tsx";

function wrap({ children }: { children: any }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

const baseCap = {
  provider: "deepseek",
  modelId: "deepseek-v4-flash",
  thinkingShape: "effort" as const,
  effortValues: ["low", "medium", "high"],
  budgetMin: 0,
  budgetMax: 100000,
  contextWindow: 128000,
  maxOutput: 8000,
  contextMode: "full",
};

beforeEach(() => {
  mockSetOverride.mockReset();
  mockClearOverride.mockReset();
});

describe("ModelCapOverrideEditor", () => {
  it("collapsed_showsTriggerLink", () => {
    render(
      <ModelCapOverrideEditor provider="deepseek" modelId="deepseek-v4-flash" current={baseCap} />,
      { wrapper: wrap },
    );
    expect(screen.getByText("能力不对？覆盖")).toBeInTheDocument();
    expect(screen.queryByText("思考控制形态")).not.toBeInTheDocument();
  });

  it("clickTrigger_opensEditor", () => {
    render(
      <ModelCapOverrideEditor provider="deepseek" modelId="deepseek-v4-flash" current={baseCap} />,
      { wrapper: wrap },
    );
    fireEvent.click(screen.getByText("能力不对？覆盖"));
    expect(screen.getByText("思考控制形态")).toBeInTheDocument();
    expect(screen.getByText("上下文窗口 (tokens)")).toBeInTheDocument();
    expect(screen.getByText("最大输出 (tokens)")).toBeInTheDocument();
  });

  it("prefillsFromCurrent", () => {
    render(
      <ModelCapOverrideEditor provider="deepseek" modelId="deepseek-v4-flash" current={baseCap} />,
      { wrapper: wrap },
    );
    fireEvent.click(screen.getByText("能力不对？覆盖"));
    const windowInput = screen.getByRole("spinbutton", { name: "上下文窗口 (tokens)" });
    const outputInput = screen.getByRole("spinbutton", { name: "最大输出 (tokens)" });
    expect((windowInput as HTMLInputElement).value).toBe("128000");
    expect((outputInput as HTMLInputElement).value).toBe("8000");
  });

  it("save_callsSetOverrideWithBody", () => {
    render(
      <ModelCapOverrideEditor provider="deepseek" modelId="deepseek-v4-flash" current={baseCap} />,
      { wrapper: wrap },
    );
    fireEvent.click(screen.getByText("能力不对？覆盖"));
    const windowInput = screen.getByRole("spinbutton", { name: "上下文窗口 (tokens)" });
    fireEvent.change(windowInput, { target: { value: "200000" } });
    fireEvent.click(screen.getByText("保存"));
    expect(mockSetOverride).toHaveBeenCalledWith({
      provider: "deepseek",
      modelId: "deepseek-v4-flash",
      thinkingShape: "effort",
      contextWindow: 200000,
      maxOutput: 8000,
    });
  });

  it("restore_callsClearOverride", () => {
    render(
      <ModelCapOverrideEditor provider="deepseek" modelId="deepseek-v4-flash" current={baseCap} />,
      { wrapper: wrap },
    );
    fireEvent.click(screen.getByText("能力不对？覆盖"));
    fireEvent.click(screen.getByText("恢复默认"));
    expect(mockClearOverride).toHaveBeenCalledWith({
      provider: "deepseek",
      modelId: "deepseek-v4-flash",
    });
  });

  it("noCurrent_saveSendsOnlyShape", () => {
    render(
      <ModelCapOverrideEditor provider="openai" modelId="gpt-4o" current={undefined} />,
      { wrapper: wrap },
    );
    fireEvent.click(screen.getByText("能力不对？覆盖"));
    fireEvent.click(screen.getByText("保存"));
    expect(mockSetOverride).toHaveBeenCalledWith({
      provider: "openai",
      modelId: "gpt-4o",
      thinkingShape: "none",
    });
  });
});
