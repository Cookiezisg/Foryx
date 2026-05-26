// ChatListItem — one row in the sidebar "最近" list. Pill matches .sb-item
// (999px radius / 6px margin / shared hover+active bg). Status dot appears
// ONLY when streaming (accent pulse) or awaiting approval (warn); idle rows
// are flush text. Hover reveals the ⋯ menu (pin / rename / archive / delete).
//
// ChatListItem —— "最近"列表行;药丸与导航项一致。idle 标题齐平无点,
// streaming/待批准 才显状态点;hover 浮出 ⋯ 操作菜单。

import { useTranslation } from "react-i18next";
import { useToastStore } from "@shared/ui/toastStore";
import { Icon } from "@shared/ui/Icon";
import { ActionMenu } from "../action-menu/ActionMenu.tsx";
import { useUpdateConversation, useDeleteConversation } from "@entities/conversation";

interface ChatListItemProps {
  conv: any;
  openPanes: string[];
  activeConv: string | null;
  onSetActiveConv: (id: string | null) => void;
  onOpenPane: (pane: string) => void;
}

export function ChatListItem({ conv, openPanes, activeConv, onSetActiveConv, onOpenPane }: ChatListItemProps) {
  const { t } = useTranslation("sidebar");

  const isStreaming = conv.status === "streaming";
  const isApproval = conv.status === "approval";
  const isActive = openPanes.includes("chat") && activeConv === conv.id;

  return (
    <div className={"cv" + (isActive ? " is-active" : "")}>
      <button
        type="button"
        className="cv-open"
        title={conv.title || t("conv.untitled")}
        onClick={() => {
          onSetActiveConv(conv.id);
          if (!openPanes.includes("chat")) onOpenPane("chat");
        }}
      >
        {(isStreaming || isApproval) && (
          <span className={"cv-dot" + (isStreaming ? " is-streaming" : " is-approval")} />
        )}
        <span className={"cv-title" + (conv.title ? "" : " untitled")}>
          {conv.title || t("conv.untitled")}
        </span>
      </button>
      <ConvMenu conv={conv} activeConv={activeConv} onSetActiveConv={onSetActiveConv} />
    </div>
  );
}

function ConvMenu({ conv, activeConv, onSetActiveConv }: { conv: any; activeConv: any; onSetActiveConv: (id: string | null) => void }) {
  const { t } = useTranslation("sidebar");
  const update = useUpdateConversation(conv.id);
  const del = useDeleteConversation();
  const pushToast = useToastStore((s) => s.pushToast);

  const togglePin = () => {
    update.mutate(
      { pinned: !conv.pinned },
      { onError: (e) => pushToast({ kind: "error", title: t("conv.opFail"), desc: e.message }) }
    );
  };
  const toggleArchive = () => {
    update.mutate(
      { archived: !conv.archived },
      {
        onSuccess: () =>
          pushToast({ kind: "success", title: conv.archived ? t("conv.unarchiveSuccess") : t("conv.archiveSuccess") }),
        onError: (e) => pushToast({ kind: "error", title: t("conv.opFail"), desc: e.message }),
      }
    );
  };
  const rename = () => {
    const next = prompt(t("conv.renamePrompt"), conv.title || "");
    if (!next || next === conv.title) return;
    update.mutate(
      { title: next },
      { onError: (e) => pushToast({ kind: "error", title: t("conv.renameFail"), desc: e.message }) }
    );
  };
  const onDelete = () => {
    if (!confirm(t("conv.deleteConfirm", { title: conv.title || conv.id }))) return;
    del.mutate(conv.id, {
      onSuccess: () => {
        if (activeConv === conv.id) onSetActiveConv(null);
        pushToast({ kind: "success", title: t("conv.deleteSuccess") });
      },
      onError: (e) => pushToast({ kind: "error", title: t("conv.deleteFail"), desc: e.message }),
    });
  };

  return (
    <ActionMenu
      placement="bottom-end"
      renderTrigger={({ ref, ...rest }) => (
        <button ref={ref} className="cv-more" title={t("conv.moreActions")} {...rest}>
          <Icon.MoreHorizontal />
        </button>
      )}
      items={[
        { label: conv.pinned ? t("conv.unpin") : t("conv.pin"), icon: Icon.Pin, onClick: togglePin },
        { label: t("common:rename"), icon: Icon.Edit, onClick: rename },
        { label: conv.archived ? t("conv.unarchive") : t("conv.archive"), icon: Icon.Folder, onClick: toggleArchive },
        "divider",
        { label: t("common:delete"), icon: Icon.Trash, danger: true, onClick: onDelete },
      ]}
    />
  );
}
