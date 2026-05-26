import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Sidebar } from "./Sidebar.tsx";
import { usePaneStore, useSidebarStore, useOverlayStore } from "@app/model";

vi.mock("@entities/conversation", () => ({
  useConversations:        () => ({ data: [] as any[] }),
  useCreateConversation:   () => ({ mutateAsync: vi.fn().mockResolvedValue({ id: "cv_new" }) }),
  useUpdateConversation:   () => ({ mutate: vi.fn() }),
  useDeleteConversation:   () => ({ mutate: vi.fn() }),
}));
// useDisplayName now derives from the backend active user (useUsers + settings);
// mock it here so the Sidebar unit test doesn't need a real users query.
vi.mock("@entities/user", async (importOriginal) => {
  const actual = await importOriginal() as Record<string, unknown>;
  return { ...actual, useDisplayName: () => ["Weilin", vi.fn()] };
});

function SidebarConnected(extraProps: Record<string, any> = {}) {
  const openPanes = usePaneStore((s) => s.openPanes);
  const activeConv = usePaneStore((s) => s.activeConv);
  const togglePane = usePaneStore((s) => s.togglePane);
  const openPane = usePaneStore((s) => s.openPane);
  const setActiveConv = usePaneStore((s) => s.setActiveConv);
  const collapsed = useSidebarStore((s) => s.collapsed);
  const toolsExpanded = useSidebarStore((s) => s.toolsExpanded);
  const recentExpanded = useSidebarStore((s) => s.recentExpanded);
  const archivedExpanded = useSidebarStore((s) => s.archivedExpanded);
  const setCollapsed = useSidebarStore((s) => s.setCollapsed);
  const setToolsExpanded = useSidebarStore((s) => s.setToolsExpanded);
  const setRecentExpanded = useSidebarStore((s) => s.setRecentExpanded);
  const setArchivedExpanded = useSidebarStore((s) => s.setArchivedExpanded);
  const setCmdkOpen = useOverlayStore((s) => s.setCmdkOpen);
  const setNotifsOpen = useOverlayStore((s) => s.setNotifsOpen);
  const setSettingsOpen = useOverlayStore((s) => s.setSettingsOpen);
  return (
    <Sidebar
      openPanes={openPanes} activeConv={activeConv}
      collapsed={collapsed} toolsExpanded={toolsExpanded}
      recentExpanded={recentExpanded} archivedExpanded={archivedExpanded}
      onTogglePane={togglePane} onOpenPane={openPane} onSetActiveConv={setActiveConv}
      onSetCollapsed={setCollapsed} onSetToolsExpanded={setToolsExpanded}
      onSetRecentExpanded={setRecentExpanded} onSetArchivedExpanded={setArchivedExpanded}
      sseHealth={{ overall: "ok", eventlog: "ok", notifs: "ok", forge: "ok", unread: 0, clearUnread: vi.fn() }}
      onOpenCmdk={() => setCmdkOpen(true)} onOpenNotifs={() => setNotifsOpen(true)}
      onOpenSettings={() => setSettingsOpen(true)}
      {...extraProps}
    />
  );
}

function renderSidebar(extraProps: Record<string, any> = {}) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <SidebarConnected {...extraProps} />
    </QueryClientProvider>
  );
}

beforeEach(() => {
  localStorage.clear();
  usePaneStore.setState({ openPanes: [] });
  useSidebarStore.setState({ collapsed: false, toolsExpanded: true, recentExpanded: true });
  useOverlayStore.setState({ cmdkOpen: false, notifsOpen: false, settingsOpen: false });
});

describe("Sidebar", () => {
  it("renders Forgify logo + name when expanded", () => {
    renderSidebar();
    expect(screen.getByText("Forgify")).toBeInTheDocument();
  });

  it("renders all 4 workbenches + 4 tools", () => {
    renderSidebar();
    for (const label of ["对话", "工坊", "执行", "文档", "洞察", "Skills", "MCP", "Memory"]) {
      expect(screen.getByText(label)).toBeInTheDocument();
    }
  });

  it("primary 新对话 button calls create-conv and switches to chat pane", async () => {
    renderSidebar();
    await act(async () => {
      fireEvent.click(screen.getByText("新对话"));
    });
    expect(usePaneStore.getState().openPanes).toContain("chat");
    expect(usePaneStore.getState().activeConv).toBe("cv_new");
  });

  it("toggle collapses sidebar (state + localStorage)", () => {
    renderSidebar();
    fireEvent.click(screen.getByLabelText(/toggle sidebar/i));
    expect(useSidebarStore.getState().collapsed).toBe(true);
    expect(localStorage.getItem("sidebar.collapsed")).toBe("1");
  });

  it("hides Forgify name + recent section in collapsed mode", () => {
    useSidebarStore.setState({ collapsed: true });
    renderSidebar();
    expect(screen.queryByText("Forgify")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "最近" })).not.toBeInTheDocument();
  });

  it("collapses tools section on click and persists state", () => {
    renderSidebar();
    fireEvent.click(screen.getByRole("button", { name: "工具" }));
    expect(useSidebarStore.getState().toolsExpanded).toBe(false);
    expect(localStorage.getItem("sidebar.toolsExpanded")).toBe("0");
    expect(screen.queryByText("洞察")).not.toBeInTheDocument();
  });

  it("footer avatar click opens NotificationsDrawer", () => {
    renderSidebar();
    const slot = screen.getByTitle(/通知/i);
    fireEvent.click(slot);
    expect(useOverlayStore.getState().notifsOpen).toBe(true);
  });

  it("footer gear opens settings modal", () => {
    renderSidebar();
    fireEvent.click(screen.getByLabelText("settings"));
    expect(useOverlayStore.getState().settingsOpen).toBe(true);
  });

  it("shows initial from displayName in avatar", () => {
    renderSidebar();
    expect(screen.getByText("W")).toBeInTheDocument();
  });
});
