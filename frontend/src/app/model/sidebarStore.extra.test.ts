// sidebarStore — additional coverage for setRecentExpanded + setArchivedExpanded.

import { describe, expect, it, beforeEach } from "vitest";
import { useSidebarStore } from "./sidebarStore";

beforeEach(() => {
  localStorage.clear();
  useSidebarStore.setState({ collapsed: false, toolsExpanded: true, recentExpanded: true, archivedExpanded: false });
});

describe("sidebarStore — setRecentExpanded + setArchivedExpanded", () => {
  it("setRecentExpanded_boolean_updatesState", () => {
    useSidebarStore.getState().setRecentExpanded(false);
    expect(useSidebarStore.getState().recentExpanded).toBe(false);
    expect(localStorage.getItem("sidebar.recentExpanded")).toBe("0");
  });

  it("setRecentExpanded_function_toggles", () => {
    useSidebarStore.getState().setRecentExpanded((p) => !p);
    expect(useSidebarStore.getState().recentExpanded).toBe(false);
  });

  it("setArchivedExpanded_boolean_updatesState", () => {
    useSidebarStore.getState().setArchivedExpanded(true);
    expect(useSidebarStore.getState().archivedExpanded).toBe(true);
    expect(localStorage.getItem("sidebar.archivedExpanded")).toBe("1");
  });

  it("setArchivedExpanded_function_toggles", () => {
    useSidebarStore.getState().setArchivedExpanded((p) => !p);
    expect(useSidebarStore.getState().archivedExpanded).toBe(true);
  });

  it("setToolsExpanded_function_toggles", () => {
    useSidebarStore.getState().setToolsExpanded((p) => !p);
    expect(useSidebarStore.getState().toolsExpanded).toBe(false);
  });
});
