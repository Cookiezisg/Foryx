// ChatListItem — one row in the sidebar conversation list.
// status dot reflects conversation state (streaming pulses, approval =
// warn color), title is left-aligned and truncates. Hover shows ActionMenu.
//
// ChatListItem —— sidebar 对话项；hover 显示 ActionMenu，每个动作都接
// 真后端（pin/archive 经 PATCH，rename 经 PATCH+prompt，delete 经 DELETE
// + 确认）。

import { useUIStore } from "../../store/ui.js";
import { Icon } from "../primitives/Icon.jsx";
import { ActionMenu } from "../shared/ActionMenu.jsx";
import { useUpdateConversation, useDeleteConversation } from "../../api/conversations.js";

export function ChatListItem({ conv }) {
  const activeConv = useUIStore((s) => s.activeConv);
  const setActiveConv = useUIStore((s) => s.setActiveConv);
  const openPane = useUIStore((s) => s.openPane);
  const openPanes = useUIStore((s) => s.openPanes);

  const isStreaming = conv.status === "streaming";
  const isApproval = conv.status === "approval";
  const isActive = openPanes.includes("chat") && activeConv === conv.id;

  return (
    <div className={"nav-item-wrap" + (isActive ? " is-active" : "")}>
      <button
        className={"nav-item" + (isActive ? " is-active" : "") + (isStreaming ? " is-streaming" : "")}
        title={conv.title || "(无标题)"}
        onClick={() => {
          setActiveConv(conv.id);
          if (!openPanes.includes("chat")) openPane("chat");
        }}
      >
        <span
          className={"dot" + (isStreaming ? " is-streaming" : "")}
          style={isApproval ? { background: "var(--status-warn)" } : undefined}
        />
        <span className="label">{conv.title || "(无标题)"}</span>
        {isApproval && (
          <span
            className="badge"
            style={{
              background: "color-mix(in srgb, var(--status-warn) 16%, transparent)",
              color: "var(--status-warn)",
            }}
          >!</span>
        )}
      </button>
      <ConvMenu conv={conv} />
    </div>
  );
}

function ConvMenu({ conv }) {
  const update = useUpdateConversation(conv.id);
  const del = useDeleteConversation();
  const pushToast = useUIStore((s) => s.pushToast);
  const activeConv = useUIStore((s) => s.activeConv);
  const setActiveConv = useUIStore((s) => s.setActiveConv);

  const togglePin = () => {
    update.mutate(
      { pinned: !conv.pinned },
      { onError: (e) => pushToast({ kind: "error", title: "操作失败", desc: e.message }) }
    );
  };
  const toggleArchive = () => {
    update.mutate(
      { archived: !conv.archived },
      {
        onSuccess: () =>
          pushToast({ kind: "success", title: conv.archived ? "已取消归档" : "已归档" }),
        onError: (e) => pushToast({ kind: "error", title: "操作失败", desc: e.message }),
      }
    );
  };
  const rename = () => {
    const next = prompt("新名字", conv.title || "");
    if (!next || next === conv.title) return;
    update.mutate(
      { title: next },
      { onError: (e) => pushToast({ kind: "error", title: "重命名失败", desc: e.message }) }
    );
  };
  const onDelete = () => {
    if (!confirm(`删除 "${conv.title || conv.id}"?这一步不可撤销。`)) return;
    del.mutate(conv.id, {
      onSuccess: () => {
        if (activeConv === conv.id) setActiveConv(null);
        pushToast({ kind: "success", title: "已删除" });
      },
      onError: (e) => pushToast({ kind: "error", title: "删除失败", desc: e.message }),
    });
  };

  return (
    <ActionMenu
      placement="bottom-end"
      renderTrigger={({ ref, ...rest }) => (
        <button ref={ref} className="rel-more-btn" title="对话操作" {...rest}>
          <Icon.MoreHorizontal />
        </button>
      )}
      items={[
        { label: conv.pinned ? "取消置顶" : "置顶", icon: Icon.Pin, onClick: togglePin },
        { label: "重命名", icon: Icon.Edit, onClick: rename },
        { label: conv.archived ? "取消归档" : "归档", icon: Icon.Folder, onClick: toggleArchive },
        "divider",
        { label: "删除", icon: Icon.Trash, danger: true, onClick: onDelete },
      ]}
    />
  );
}
