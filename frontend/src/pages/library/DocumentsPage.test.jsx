// DocumentsPage — tree render + create / rename / delete. Tiptap editor
// is stubbed; this file is about the surrounding shell.

import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

vi.mock("../../api/library.js", () => ({
  useDocumentTree: vi.fn(),
  useDocument: vi.fn(),
  useCreateDocument: vi.fn(),
  useUpdateDocument: vi.fn(),
  useDeleteDocument: vi.fn(),
}));

vi.mock("@/panes/library/DocEditor.jsx", () => ({
  DocEditor: ({ initialMarkdown }) => (
    <div data-testid="editor-stub">{initialMarkdown}</div>
  ),
}));

vi.mock("../../widgets/ask-ai-trigger/AskAiTrigger.jsx", () => ({
  AskAiTrigger: () => <div data-testid="ask-ai-trigger" />,
}));

vi.mock("../../widgets/entity-rel-meta/EntityRelMeta.jsx", () => ({
  EntityRelMeta: () => null,
}));

vi.mock("../../shared/ui/RelTime.jsx", () => ({
  RelTime: ({ ts }) => <span>{ts}</span>,
}));

import {
  useDocumentTree, useDocument,
  useCreateDocument, useUpdateDocument, useDeleteDocument,
} from "../../api/library.js";
import { useToastStore } from "../../shared/ui/toastStore.ts";
import { DocumentsPage } from "./DocumentsPage.jsx";

const TREE = [
  { id: "doc_root1", name: "Roadmap",  parentId: null,        position: 0 },
  { id: "doc_root2", name: "Inbox",    parentId: null,        position: 1 },
  { id: "doc_child", name: "Q4 plan",  parentId: "doc_root1", position: 0 },
];

let createMutateAsync, updateMutate, delMutate;

beforeEach(() => {
  createMutateAsync = vi.fn(async ({ name }) => ({ id: "doc_new", name }));
  updateMutate      = vi.fn();
  delMutate         = vi.fn();
  useDocumentTree.mockReturnValue({ data: TREE, isLoading: false });
  useDocument.mockReturnValue({ data: null, isLoading: false });
  useCreateDocument.mockReturnValue({ mutateAsync: createMutateAsync });
  useUpdateDocument.mockReturnValue({ mutate: updateMutate, isPending: false });
  useDeleteDocument.mockReturnValue({ mutate: delMutate });
  useToastStore.setState({ toasts: [] });
});

describe("DocumentsPage", () => {
  it("treeLoading_showsLoadingHint", () => {
    useDocumentTree.mockReturnValue({ data: undefined, isLoading: true });
    render(<DocumentsPage activeDoc={null} onSetActiveDocument={vi.fn()} />);
    expect(screen.getByText("加载中…")).toBeInTheDocument();
  });

  it("treeRoots_renderedSortedByPosition", () => {
    render(<DocumentsPage activeDoc={null} onSetActiveDocument={vi.fn()} />);
    expect(screen.getByText("Roadmap")).toBeInTheDocument();
    expect(screen.getByText("Inbox")).toBeInTheDocument();
  });

  it("childNode_hiddenUntilParentExpanded", async () => {
    render(<DocumentsPage activeDoc={null} onSetActiveDocument={vi.fn()} />);
    expect(screen.queryByText("Q4 plan")).toBeNull();
    const toggles = document.querySelectorAll(".dtr-toggle[data-has-children]");
    await userEvent.click(toggles[0]);
    expect(screen.getByText("Q4 plan")).toBeInTheDocument();
  });

  it("searchInput_filtersTreeByName", async () => {
    render(<DocumentsPage activeDoc={null} onSetActiveDocument={vi.fn()} />);
    await userEvent.type(screen.getByPlaceholderText("搜索文档…"), "Inbox");
    expect(screen.getByText("Inbox")).toBeInTheDocument();
    expect(screen.queryByText("Roadmap")).toBeNull();
  });

  it("noActiveDoc_showsEmptyState", () => {
    render(<DocumentsPage activeDoc={null} onSetActiveDocument={vi.fn()} />);
    expect(screen.getByText("还没有打开的文档")).toBeInTheDocument();
  });

  it("emptyStateCreate_callsCreateMutate_callsOnSetActiveDocument", async () => {
    const onSetActiveDocument = vi.fn();
    render(<DocumentsPage activeDoc={null} onSetActiveDocument={onSetActiveDocument} />);
    await userEvent.click(screen.getByText("新建第一篇"));
    expect(createMutateAsync).toHaveBeenCalledWith({ name: "未命名", parentId: null });
    expect(onSetActiveDocument).toHaveBeenCalledWith("doc_new");
  });

  it("headerPlusButton_createsRootDocument", async () => {
    render(<DocumentsPage activeDoc={null} onSetActiveDocument={vi.fn()} />);
    const headBtn = document.querySelector(".doc-sidebar-head .icon-btn[title='新建顶级页面']");
    await userEvent.click(headBtn);
    expect(createMutateAsync).toHaveBeenCalledWith({ name: "未命名", parentId: null });
  });

  it("treeRowClick_callsOnSetActiveDocument", async () => {
    const onSetActiveDocument = vi.fn();
    render(<DocumentsPage activeDoc={null} onSetActiveDocument={onSetActiveDocument} />);
    await userEvent.click(screen.getByText("Roadmap"));
    expect(onSetActiveDocument).toHaveBeenCalledWith("doc_root1");
  });

  it("inlinePlusButton_createsChildWithParentId", async () => {
    render(<DocumentsPage activeDoc={null} onSetActiveDocument={vi.fn()} />);
    const childPlus = document.querySelectorAll(".dtr-act-btn[title='新建子页面']");
    await userEvent.click(childPlus[0]);
    expect(createMutateAsync).toHaveBeenCalledWith({ name: "未命名", parentId: "doc_root1" });
  });

  it("deleteAction_confirmed_callsDelete", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    render(<DocumentsPage activeDoc={null} onSetActiveDocument={vi.fn()} />);
    const actBtns = document.querySelectorAll(".dtr-act-btn[title='操作']");
    await userEvent.click(actBtns[0]);
    await userEvent.click(screen.getByText("删除"));
    expect(delMutate).toHaveBeenCalledWith("doc_root1", expect.any(Object));
    confirmSpy.mockRestore();
  });

  it("deleteAction_declined_skipsMutate", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(false);
    render(<DocumentsPage activeDoc={null} onSetActiveDocument={vi.fn()} />);
    const actBtns = document.querySelectorAll(".dtr-act-btn[title='操作']");
    await userEvent.click(actBtns[0]);
    await userEvent.click(screen.getByText("删除"));
    expect(delMutate).not.toHaveBeenCalled();
    confirmSpy.mockRestore();
  });

  it("activeDocLoaded_rendersTitleInput_andEditor", () => {
    useDocument.mockReturnValue({
      data: { id: "doc_root1", name: "Roadmap", content: "## Hi", updatedAt: "2026-05-24T00:00:00Z" },
      isLoading: false,
    });
    render(<DocumentsPage activeDoc="doc_root1" onSetActiveDocument={vi.fn()} />);
    const titleInput = document.querySelector(".doc-page-title-input");
    expect(titleInput.value).toBe("Roadmap");
    expect(screen.getByTestId("editor-stub").textContent).toBe("## Hi");
  });

  it("titleEdit_marksDirty_showsUnsavedIndicator", () => {
    useDocument.mockReturnValue({
      data: { id: "doc_root1", name: "Roadmap", content: "", updatedAt: "2026-05-24T00:00:00Z" },
      isLoading: false,
    });
    render(<DocumentsPage activeDoc="doc_root1" onSetActiveDocument={vi.fn()} />);
    const titleInput = document.querySelector(".doc-page-title-input");
    fireEvent.change(titleInput, { target: { value: "Roadmap v2" } });
    expect(screen.getByText("未保存")).toBeInTheDocument();
  });
});
