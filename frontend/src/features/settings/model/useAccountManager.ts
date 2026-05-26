// Orchestrates account switch and creation for the settings panel.
// Extracted verbatim from AccountRegion in SettingsModal.jsx so the
// component only handles rendering.
//
// 封装切换账户/新建账户编排;AccountRegion 只负责渲染,不再含业务决策。

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useCreateUser } from "@entities/user";
import { useSessionStore } from "@entities/session";
import { useToastStore } from "@shared/ui/toastStore";

export interface AccountManagerState {
  name: string;
  setName: (v: string) => void;
  switchTo: (id: string) => void;
  addAccount: () => Promise<void>;
  isAdding: boolean;
}

export function useAccountManager(): AccountManagerState {
  const { t } = useTranslation("settings");
  const qc = useQueryClient();
  const pushToast = useToastStore((s) => s.pushToast);
  const createUser = useCreateUser();

  const [name, setName] = useState("");

  const switchTo = (id: string) => {
    useSessionStore.getState().setCurrentUser(id);
    qc.invalidateQueries();
    pushToast({ kind: "success", title: t("account.switchedTo", { id }) });
  };

  const addAccount = async () => {
    const username = name.trim();
    if (!username) return;
    try {
      const created = await createUser.mutateAsync({ username });
      switchTo(created.id);
      setName("");
    } catch (e) {
      pushToast({ kind: "error", title: t("account.addFail"), desc: (e as Error).message });
    }
  };

  return {
    name,
    setName,
    switchTo,
    addAccount,
    isAdding: createUser.isPending,
  };
}
