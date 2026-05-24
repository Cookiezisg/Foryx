import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Dashboard } from "./Dashboard.jsx";
import { useUIStore } from "../../store/ui.js";

const createMutateAsync = vi.fn().mockResolvedValue({ id: "cv_n" });

vi.mock("../../api/flowruns.js", () => ({
  useFlowRuns: () => ({ data: [] }),
}));
vi.mock("../../api/conversations.js", () => ({
  useConversations:       () => ({ data: [] }),
  useCreateConversation:  () => ({ mutateAsync: createMutateAsync }),
}));

function renderDash() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Dashboard />
    </QueryClientProvider>
  );
}

beforeEach(() => {
  localStorage.clear();
  useUIStore.setState({ openPanes: [], activeConv: null });
  createMutateAsync.mockClear();
  global.fetch = vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({}),
  });
});

describe("Dashboard", () => {
  it("renders a non-empty greeting", () => {
    renderDash();
    const greet = document.querySelector(".wel-greet");
    expect(greet).toBeTruthy();
    expect(greet.textContent.trim().length).toBeGreaterThan(0);
  });

  it("renders the input with correct placeholder", () => {
    renderDash();
    expect(screen.getByPlaceholderText("Ask Forgify… or forge something")).toBeInTheDocument();
  });

  it("Enter creates conv, sends first message, switches to chat pane", async () => {
    renderDash();
    const input = screen.getByPlaceholderText("Ask Forgify… or forge something");
    fireEvent.change(input, { target: { value: "hello forge" } });
    await act(async () => {
      fireEvent.keyDown(input, { key: "Enter" });
      await Promise.resolve(); await Promise.resolve();
    });
    expect(createMutateAsync).toHaveBeenCalled();
    expect(global.fetch).toHaveBeenCalledWith(
      expect.stringContaining("/conversations/cv_n/messages"),
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ text: "hello forge" }),
      })
    );
    expect(useUIStore.getState().openPanes).toContain("chat");
    expect(useUIStore.getState().activeConv).toBe("cv_n");
  });

  it("hides the context strip when there's nothing of interest", () => {
    renderDash();
    expect(document.querySelector(".wel-strip")).toBeNull();
  });
});
