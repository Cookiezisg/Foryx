import { useEffect } from "react";
import { Outlet } from "react-router-dom";
import { useUIStore } from "@/stores/ui";
import { useUsersStore } from "@/stores/users";
import { useNotificationsStore } from "@/stores/notifications";
import { useForgeStore } from "@/stores/forge";
import { useCatalogStore } from "@/stores/catalog";
import { TopBar } from "@/layout/TopBar";
import { ConvSidebar } from "@/layout/ConvSidebar";
import { ChatPanel } from "@/layout/ChatPanel";
import { TabNav } from "@/layout/TabNav";
import { UserPicker } from "@/layout/UserPicker";
import { ResizableSplit } from "@/layout/ResizableSplit";

export function App() {
  const ui = useUIStore();
  const users = useUsersStore();
  const notifs = useNotificationsStore();
  const forge = useForgeStore();
  const catalog = useCatalogStore();

  useEffect(() => {
    (async () => {
      await users.refresh();
      notifs.start();
      forge.start();
      catalog.refresh();
    })();
    return () => { notifs.stop(); forge.stop(); };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        ui.openPalette();
      }
    };
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, [ui]);

  const showPicker = users.list.length >= 2 && !users.list.find((u) => u.id === users.activeId);

  return (
    <div className="app-root">
      <TopBar />
      <div className="layout">
        {ui.expanded ? (
          <>
            <aside style={{ width: 40, background: "var(--bg-sidebar)", borderRight: "1px solid var(--border)" }} />
            <aside style={{ width: 40, background: "var(--bg-sidebar)", borderRight: "1px solid var(--border)" }}>
              <TabNav />
            </aside>
            <main className="tab-content"><Outlet /></main>
          </>
        ) : (
          <ResizableSplit
            leftWidth={ui.colConv} minLeft={140} maxLeft={380} onResize={ui.setColConv}
            left={<ConvSidebar />}
            right={
              <ResizableSplit
                leftWidth={ui.colChat} minLeft={320} maxLeft={900} onResize={ui.setColChat}
                left={<ChatPanel />}
                right={
                  <ResizableSplit
                    leftWidth={ui.colNav} minLeft={180} maxLeft={320} onResize={ui.setColNav}
                    left={<TabNav />}
                    right={<main className="tab-content"><Outlet /></main>}
                  />
                }
              />
            }
          />
        )}
      </div>
      {showPicker && <UserPicker />}
      {/* CommandPalette + RawJsonModal + ToastTray land in P2.C */}
    </div>
  );
}
