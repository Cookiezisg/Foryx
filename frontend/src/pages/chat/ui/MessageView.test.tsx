// MessageView — meta row + body + attachments.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { useChatStore } from "@entities/conversation";

vi.mock("./BlockRenderer.tsx", () => ({
  BlockList: ({ blockIds }: { blockIds: any }) => <div data-testid="blocklist">{(blockIds || []).join(",")}</div>,
}));

vi.mock("../../../shared/ui/RelTime.tsx", () => ({
  RelTime: ({ ts }: { ts: any }) => <span data-testid="reltime">{ts}</span>,
}));

import { MessageView } from "./MessageView.tsx";

const CV = "cv_mv";

function seedMessage(msg: any) {
  useChatStore.setState({
    convs: {
      [CV]: {
        messages: new Map([[msg.id, msg]]),
        blocks: new Map(),
        topMsgIds: [msg.id],
        lastSeq: 0,
      },
    },
    hydratedConvs: new Set([CV]),
  });
}

beforeEach(() => {
  useChatStore.setState({ convs: {}, hydratedConvs: new Set() });
});

describe("MessageView", () => {
  it("noMessage_rendersNothing", () => {
    const { container } = render(<MessageView convId={CV} msgId="missing" />);
    expect(container.firstChild).toBeNull();
  });

  it("userRole_showsUserAvatarYAndLabel你", () => {
    seedMessage({ id: "m_1", role: "user", status: "completed", blocks: [], createdAt: "2026-01-01" });
    render(<MessageView convId={CV} msgId="m_1" />);
    expect(screen.getByText("你")).toBeInTheDocument();
  });

  it("assistantRole_showsProviderShortNameFromModel", () => {
    seedMessage({
      id: "m_2", role: "assistant", status: "completed", blocks: [],
      createdAt: "2026-01-01", model: "anthropic-claude-3",
    });
    render(<MessageView convId={CV} msgId="m_2" />);
    expect(screen.getByText("anthropic-claude-3")).toBeInTheDocument();
  });

  it("streamingStatus_showsStreamingBadge", () => {
    seedMessage({ id: "m_3", role: "assistant", status: "streaming", blocks: [], createdAt: "2026-01-01" });
    render(<MessageView convId={CV} msgId="m_3" />);
    expect(screen.getByText("在写")).toBeInTheDocument();
  });

  it("errorStatus_showsErrorBadge", () => {
    seedMessage({ id: "m_4", role: "assistant", status: "error", blocks: [], createdAt: "2026-01-01" });
    render(<MessageView convId={CV} msgId="m_4" />);
    expect(screen.getByText("出错了")).toBeInTheDocument();
  });

  it("tokens_renderedInMetaRow", () => {
    seedMessage({
      id: "m_5", role: "assistant", status: "completed", blocks: [], createdAt: "2026-01-01",
      inputTokens: 100, outputTokens: 200,
    });
    render(<MessageView convId={CV} msgId="m_5" />);
    expect(screen.getByText("100")).toBeInTheDocument();
    expect(screen.getByText("200")).toBeInTheDocument();
  });

  it("attachments_renderedAsPills", () => {
    seedMessage({
      id: "m_6", role: "user", status: "completed", createdAt: "2026-01-01",
      blocks: [],
      attachments: [{ fileName: "a.txt", sizeBytes: 1024 }, { fileName: "img.png", mimeType: "image/png" }],
    });
    render(<MessageView convId={CV} msgId="m_6" />);
    expect(screen.getByText("a.txt")).toBeInTheDocument();
    expect(screen.getByText("img.png")).toBeInTheDocument();
  });
});
