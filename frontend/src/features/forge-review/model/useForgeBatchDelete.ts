// Orchestrates batch delete for ForgeList — confirm → mutate each selected
// item → clearSel + toast. Verbatim confirm copy from original ForgeList.
//
// 封装 ForgeList 批量删编排；confirm 文案逐字保留；clearSel + toast 行为不变。

import { useTranslation } from "react-i18next";
import { useDeleteFunction } from "@entities/function";
import { useDeleteHandler } from "@entities/handler";
import { useDeleteWorkflow } from "@entities/workflow";

type ForgeKind = "function" | "handler" | "workflow";

interface ForgeItem {
  id: string;
  kind: ForgeKind;
  name?: string;
}

// batchDelete(items, clearSel) — shows confirm dialog (verbatim copy from
// ForgeList), then calls the correct delete mutation for each item by kind.
//
// batchDelete 逐项按 kind 路由删除；confirm 与原 ForgeList 一致。
export function useForgeBatchDelete() {
  const { t } = useTranslation(["forge", "common"]);

  const deleteFn = useDeleteFunction();
  const deleteHd = useDeleteHandler();
  const deleteWf = useDeleteWorkflow();

  const batchDelete = (items: ForgeItem[], clearSel: () => void) => {
    if (!confirm(t("forge:list.batch.deleteConfirm", { count: items.length }))) return;
    items.forEach((f) => {
      const m =
        f.kind === "function" ? deleteFn
        : f.kind === "handler" ? deleteHd
        :                        deleteWf;
      m.mutate(f.id);
    });
    clearSel();
  };

  return { batchDelete };
}
