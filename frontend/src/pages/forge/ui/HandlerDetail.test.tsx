// HandlerDetail — Class/Config/Calls tabs + multi-method picker + diff
// view + VersionRail. pendingV swaps action buttons to Accept/Revert.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("@entities/handler", () => ({
  useHandler: vi.fn(),
  useHandlerVersions: vi.fn(),
  useHandlerConfig: vi.fn(),
}));

vi.mock("@features/forge-review", () => ({
  useForgeReview: vi.fn(),
  useForgeBatchDelete: vi.fn(),
}));

vi.mock("@shared/model", () => ({
  useForgeProgress: (selector: (s: any) => any) => selector({ active: {} }),
}));

vi.mock("@/widgets/entity-rel-meta/EntityRelMeta.tsx", () => ({
  EntityRelMeta: (): null => null,
}));

vi.mock("./RunDrawer.tsx", () => ({
  RunDrawer: ({ open, entity }: { open: any; entity: any }) =>
    open ? <div data-testid="run-drawer">drawer-{entity?.id}</div> : null,
}));

vi.mock("@/widgets/ask-ai-trigger/AskAiTrigger.tsx", () => ({
  AskAiTrigger: ({ entityId }: { entityId: any }) => <div data-testid="ask-ai">ask-{entityId}</div>,
}));

import {
  useHandler, useHandlerVersions, useHandlerConfig,
} from "@entities/handler";
import { useForgeReview } from "@features/forge-review";
import { useToastStore } from "@shared/ui/toastStore";
import { HandlerDetail } from "./HandlerDetail.tsx";

const mockUseHandler = useHandler as any;
const mockUseHandlerVersions = useHandlerVersions as any;
const mockUseHandlerConfig = useHandlerConfig as any;
const mockUseForgeReview = useForgeReview as any;

const HD = { id: "hd_1", name: "SlackHandler", desc: "slack ops", status: "ready" };

const VERSIONS_READY = [
  { id: "hv_1", label: "v1", state: "current",
    methods: [
      { name: "send", sig: "(channel, text)", desc: "send msg", body: "print(channel, text)" },
      { name: "list", sig: "()", desc: "list channels", body: "return []" },
    ],
  },
];

const VERSIONS_PENDING = [
  ...VERSIONS_READY,
  { id: "hv_2", label: "v2", state: "pending",
    methods: [
      { name: "send", sig: "(channel, text, attachments=None)", body: "print(channel, text)" },
      { name: "deleteChannel", sig: "(channel)", body: "pass" },
    ],
  },
];

beforeEach(() => {
  useToastStore.setState({ toasts: [] });
  mockUseHandler.mockReturnValue({ data: HD });
  mockUseHandlerVersions.mockReturnValue({ data: VERSIONS_READY });
  mockUseHandlerConfig.mockReturnValue({ data: {} });
  mockUseForgeReview.mockReturnValue({ accept: vi.fn(), reject: vi.fn() });
});

describe("HandlerDetail", () => {
  it("header_showsNameAndKindChip", () => {
    render(<HandlerDetail forge={HD} onBack={() => {}} />);
    expect(screen.getByText("SlackHandler")).toBeInTheDocument();
    expect(screen.getByText("hd_1")).toBeInTheDocument();
  });

  it("readyState_showsRunButton_andAskAi", () => {
    render(<HandlerDetail forge={HD} onBack={() => {}} />);
    expect(screen.getByText("试调用")).toBeInTheDocument();
    expect(screen.getByTestId("ask-ai")).toBeInTheDocument();
  });

  it("pendingState_showsAcceptAndRevert", () => {
    mockUseHandlerVersions.mockReturnValue({ data: VERSIONS_PENDING });
    render(<HandlerDetail forge={HD} onBack={() => {}} />);
    expect(screen.getAllByText("接受").length).toBeGreaterThan(0);
    expect(screen.getAllByText("还原").length).toBeGreaterThan(0);
  });

  it("backButton_callsOnBack", async () => {
    const onBack = vi.fn();
    render(<HandlerDetail forge={HD} onBack={onBack} />);
    await userEvent.click(screen.getByText(/返回/));
    expect(onBack).toHaveBeenCalled();
  });

  it("classTab_showsMethodList_firstSelectedByDefault", () => {
    render(<HandlerDetail forge={HD} onBack={() => {}} />);
    // "send" is both in picker and method signature — assert at least one
    expect(screen.getAllByText("send").length).toBeGreaterThan(0);
    expect(screen.getByText("list")).toBeInTheDocument();
    expect(screen.getByText("send msg")).toBeInTheDocument();
  });

  it("methodPickerClick_switchesActiveMethod", async () => {
    render(<HandlerDetail forge={HD} onBack={() => {}} />);
    await userEvent.click(screen.getByText("list"));
    expect(screen.getByText("list channels")).toBeInTheDocument();
  });

  it("configTab_emptyConfig_showsEmptyState", async () => {
    render(<HandlerDetail forge={HD} onBack={() => {}} />);
    await userEvent.click(screen.getByText("Config"));
    expect(screen.getByText(/还没有配置项/)).toBeInTheDocument();
  });

  it("configTab_withItems_rendersEachConfigRow", async () => {
    mockUseHandlerConfig.mockReturnValue({
      data: {
        SLACK_TOKEN: { value: "xoxb-...", secret: true },
        DEFAULT_CHANNEL: { value: "#general" },
      },
    });
    render(<HandlerDetail forge={HD} onBack={() => {}} />);
    await userEvent.click(screen.getByText("Config"));
    expect(screen.getByText("SLACK_TOKEN")).toBeInTheDocument();
    expect(screen.getByText("DEFAULT_CHANNEL")).toBeInTheDocument();
    expect(screen.getByText("secret")).toBeInTheDocument();
  });

  it("callsTab_showsPlaceholder", async () => {
    render(<HandlerDetail forge={HD} onBack={() => {}} />);
    await userEvent.click(screen.getByText("Call 历史"));
    // appears as both tab label and panel title
    expect(screen.getAllByText("Call 历史").length).toBeGreaterThanOrEqual(2);
  });

  it("runClick_opensRunDrawer", async () => {
    render(<HandlerDetail forge={HD} onBack={() => {}} />);
    await userEvent.click(screen.getByText("试调用"));
    await waitFor(() => expect(screen.getByTestId("run-drawer")).toBeInTheDocument());
  });

  it("acceptClick_callsAcceptAction", async () => {
    mockUseHandlerVersions.mockReturnValue({ data: VERSIONS_PENDING });
    const accept = vi.fn();
    mockUseForgeReview.mockReturnValue({ accept, reject: vi.fn() });
    render(<HandlerDetail forge={HD} onBack={() => {}} />);
    const headerAccept = screen.getAllByText("接受")[0];
    await userEvent.click(headerAccept);
    expect(accept).toHaveBeenCalled();
  });

  it("rejectClick_callsRejectAction", async () => {
    mockUseHandlerVersions.mockReturnValue({ data: VERSIONS_PENDING });
    const reject = vi.fn();
    mockUseForgeReview.mockReturnValue({ accept: vi.fn(), reject });
    render(<HandlerDetail forge={HD} onBack={() => {}} />);
    const headerRevert = screen.getAllByText("还原")[0];
    await userEvent.click(headerRevert);
    expect(reject).toHaveBeenCalled();
  });

  it("pendingDiff_methodSignatureChange_listedAsModified", () => {
    mockUseHandlerVersions.mockReturnValue({ data: VERSIONS_PENDING });
    const { container } = render(<HandlerDetail forge={HD} onBack={() => {}} />);
    expect(container.textContent).toContain("Diff");
    expect(container.textContent).toContain("处方法变更");
    expect(screen.getByText("新增")).toBeInTheDocument();
    expect(screen.getByText("修改")).toBeInTheDocument();
    expect(screen.getByText("删除")).toBeInTheDocument();
  });
});
