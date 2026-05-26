// ChatPage — gate on api-keys, conv switch hydration, send/cancel wiring.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";

const mockSend = vi.fn();
const mockCancel = vi.fn();

vi.mock("@entities/conversation", async (importOriginal) => {
  const actual = await importOriginal() as Record<string, unknown>;
  return {
    ...actual,
    useConversation: () => ({ data: { id: "cv_x", title: "Test Conv" } }),
    useConversationMessages: () => ({ data: [] as any[], isLoading: false }),
  };
});

vi.mock("@features/send-message", () => ({
  useSendMessageFlow: () => ({ submit: mockSend, cancelStream: mockCancel, isPending: false }),
  Composer: ({ onSend, onCancel, isStreaming }: { onSend: any; onCancel: any; isStreaming: any }) => (
    createElement("div", { "data-testid": "composer" },
      createElement("button", { onClick: () => onSend({ content: "hi" }) }, "send"),
      createElement("button", { onClick: () => onCancel() }, "cancel"),
      createElement("span", null, isStreaming ? "streaming" : "idle"),
    )
  ),
}));

vi.mock("@entities/apikey", () => ({
  useApiKeys: () => ({ data: [{ id: "aki_1" }], isLoading: false }),
}));
vi.mock("@entities/model-config", () => ({
  // Default: chat scenario configured so NoModelGate doesn't swallow the
  // existing ChatPage tests. Tests that exercise the gate can override.
  useModelConfigs: () => ({
    data: [{ scenario: "chat", provider: "openai", modelId: "gpt-4o" }],
    isLoading: false,
  }),
}));

vi.mock("./ui/ChatHeader.tsx", () => ({
  ChatHeader: ({ conv }: { conv: any }) => <div data-testid="header">{conv?.title}</div>,
}));

vi.mock("./ui/MessageView.tsx", () => ({
  MessageView: ({ msgId }: { msgId: any }) => <div data-testid={`msg-${msgId}`}>{msgId}</div>,
}));

import { useToastStore } from "@shared/ui/toastStore";
import { useChatStore } from "@entities/conversation";
import { ChatPage } from "./ChatPage.tsx";

function wrap({ children }: { children: any }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

const mockSetActiveConv = vi.fn();

beforeEach(() => {
  useToastStore.setState({ toasts: [] });
  useChatStore.setState({ convs: {}, hydratedConvs: new Set() });
  mockSend.mockReset();
  mockCancel.mockReset();
  mockSetActiveConv.mockReset();
});

describe("ChatPage", () => {
  it("noActiveConv_rendersPlaceholder", () => {
    render(<ChatPage activeConv={null} onSetActiveConv={mockSetActiveConv} />, { wrapper: wrap });
    expect(screen.getByText(/还没选中对话/)).toBeInTheDocument();
  });

  it("withActiveConv_rendersHeader", () => {
    render(<ChatPage activeConv="cv_x" onSetActiveConv={mockSetActiveConv} />, { wrapper: wrap });
    expect(screen.getByTestId("header")).toBeInTheDocument();
  });

  it("composer_sendButtonClick_callsSendMutation", async () => {
    render(<ChatPage activeConv="cv_x" onSetActiveConv={mockSetActiveConv} />, { wrapper: wrap });
    await userEvent.click(screen.getByText("send"));
    expect(mockSend).toHaveBeenCalledWith({ content: "hi" });
  });

  it("composer_cancelButtonClick_callsCancelMutation", async () => {
    render(<ChatPage activeConv="cv_x" onSetActiveConv={mockSetActiveConv} />, { wrapper: wrap });
    await userEvent.click(screen.getByText("cancel"));
    expect(mockCancel).toHaveBeenCalled();
  });

  it("hydratesConv_onActiveConvChange", async () => {
    render(<ChatPage activeConv="cv_x" onSetActiveConv={mockSetActiveConv} />, { wrapper: wrap });
    await waitFor(() => {
      expect(useChatStore.getState().hydratedConvs.has("cv_x")).toBe(true);
    });
  });

  it("withStreamingMessage_composerShowsStreamingState", async () => {
    useChatStore.setState({
      convs: {
        cv_x: {
          messages: new Map([["m_1", { id: "m_1", status: "streaming", blocks: [] }]]) as any,
          blocks: new Map(),
          topMsgIds: ["m_1"],
          lastSeq: 0,
        },
      },
      hydratedConvs: new Set(["cv_x"]),
    });
    render(<ChatPage activeConv="cv_x" onSetActiveConv={mockSetActiveConv} />, { wrapper: wrap });
    expect(screen.getByText("streaming")).toBeInTheDocument();
  });
});
