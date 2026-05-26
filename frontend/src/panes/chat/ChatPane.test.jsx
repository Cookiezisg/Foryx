// ChatPane — gate on api-keys, conv switch hydration, send/cancel wiring.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createElement } from "react";

const mockSend = vi.fn();
const mockCancel = vi.fn();

vi.mock("../../api/conversations.js", () => ({
  useConversation: () => ({ data: { id: "cv_x", title: "Test Conv" } }),
  useConversationMessages: () => ({ data: [], isLoading: false }),
}));

vi.mock("../../features/send-message/index.ts", () => ({
  useSendMessageFlow: () => ({ submit: mockSend, cancelStream: mockCancel, isPending: false }),
}));

vi.mock("../../api/config.js", () => ({
  useApiKeys: () => ({ data: [{ id: "aki_1" }], isLoading: false }),
  // Default: chat scenario configured so NoModelGate doesn't swallow the
  // existing ChatPane tests. Tests that exercise the gate can override.
  useModelConfigs: () => ({
    data: [{ scenario: "chat", provider: "openai", modelId: "gpt-4o" }],
    isLoading: false,
  }),
}));

vi.mock("./ChatHeader.jsx", () => ({
  ChatHeader: ({ conv }) => <div data-testid="header">{conv?.title}</div>,
}));

vi.mock("./MessageView.jsx", () => ({
  MessageView: ({ msgId }) => <div data-testid={`msg-${msgId}`}>{msgId}</div>,
}));

vi.mock("./Composer.jsx", () => ({
  Composer: ({ onSend, onCancel, isStreaming }) => (
    <div data-testid="composer">
      <button onClick={() => onSend({ content: "hi" })}>send</button>
      <button onClick={() => onCancel()}>cancel</button>
      <span>{isStreaming ? "streaming" : "idle"}</span>
    </div>
  ),
}));

import { useUIStore } from "../../store/ui.js";
import { useChatStore } from "../../store/chat.js";
import { ChatPane } from "./ChatPane.jsx";

function wrap({ children }) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return createElement(QueryClientProvider, { client }, children);
}

beforeEach(() => {
  useUIStore.setState({ activeConv: "cv_x", toasts: [] });
  useChatStore.setState({ convs: {}, hydratedConvs: new Set() });
  mockSend.mockReset();
  mockCancel.mockReset();
});

describe("ChatPane", () => {
  it("noActiveConv_rendersPlaceholder", () => {
    useUIStore.setState({ activeConv: null });
    render(<ChatPane />, { wrapper: wrap });
    expect(screen.getByText(/还没选中对话/)).toBeInTheDocument();
  });

  it("withActiveConv_rendersHeader", () => {
    render(<ChatPane />, { wrapper: wrap });
    expect(screen.getByTestId("header")).toBeInTheDocument();
  });

  it("composer_sendButtonClick_callsSendMutation", async () => {
    render(<ChatPane />, { wrapper: wrap });
    await userEvent.click(screen.getByText("send"));
    expect(mockSend).toHaveBeenCalledWith({ content: "hi" });
  });

  it("composer_cancelButtonClick_callsCancelMutation", async () => {
    render(<ChatPane />, { wrapper: wrap });
    await userEvent.click(screen.getByText("cancel"));
    expect(mockCancel).toHaveBeenCalled();
  });

  it("hydratesConv_onActiveConvChange", async () => {
    render(<ChatPane />, { wrapper: wrap });
    await waitFor(() => {
      expect(useChatStore.getState().hydratedConvs.has("cv_x")).toBe(true);
    });
  });

  it("withStreamingMessage_composerShowsStreamingState", async () => {
    useChatStore.setState({
      convs: {
        cv_x: {
          messages: new Map([["m_1", { id: "m_1", status: "streaming", blocks: [] }]]),
          blocks: new Map(),
          topMsgIds: ["m_1"],
          lastSeq: 0,
        },
      },
      hydratedConvs: new Set(["cv_x"]),
    });
    render(<ChatPane />, { wrapper: wrap });
    expect(screen.getByText("streaming")).toBeInTheDocument();
  });
});
