import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Dashboard } from "./Dashboard.jsx";
import { apiFetch } from "../../api/client.js";

const createMutateAsync = vi.fn().mockResolvedValue({ id: "cv_n" });

vi.mock("../../api/flowruns.js", () => ({
  useFlowRuns: () => ({ data: [] }),
}));
vi.mock("../../api/conversations.js", () => ({
  useConversations:       () => ({ data: [] }),
  useCreateConversation:  () => ({ mutateAsync: createMutateAsync }),
}));
vi.mock("../../api/client.js", () => ({
  apiFetch: vi.fn().mockResolvedValue({}),
}));
vi.mock("../../hooks/useDisplayName.js", () => ({
  useDisplayName: () => ["", vi.fn()],
}));

const mockOpenPane = vi.fn();
const mockSetActiveConv = vi.fn();

function renderDash() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <Dashboard onOpenPane={mockOpenPane} onSetActiveConv={mockSetActiveConv} />
    </QueryClientProvider>
  );
}

beforeEach(() => {
  localStorage.clear();
  createMutateAsync.mockClear();
  apiFetch.mockClear();
  mockOpenPane.mockClear();
  mockSetActiveConv.mockClear();
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
    expect(apiFetch).toHaveBeenCalledWith(
      "/conversations/cv_n/messages",
      expect.objectContaining({ method: "POST", body: { content: "hello forge" } })
    );
    expect(mockOpenPane).toHaveBeenCalledWith("chat");
    expect(mockSetActiveConv).toHaveBeenCalledWith("cv_n");
  });

  it("hides the context strip when there's nothing of interest", () => {
    renderDash();
    expect(document.querySelector(".wel-strip")).toBeNull();
  });
});
