// SettingsModal — centered overlay with single-open accordion sections.
// Account region always visible; 4 sections mutually exclusive.
//
// SettingsModal —— 居中浮层；账号区常驻；4 个 accordion 区段互斥单展。

import { useEffect, useRef, useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { useTranslation } from "react-i18next";
import { Icon } from "../primitives/Icon.jsx";
import { useOverlayStore } from "@app/model";
import { useSessionStore } from "@entities/session";
import { useUsers } from "../../api/users.js";
import { scaleIn } from "../../motion/tokens.js";
import { ApiKeysSection } from "../config/ApiKeysSection.jsx";
import { SearchSection } from "../config/SearchSection.jsx";
import { AppearanceSection } from "../config/AppearanceSection.jsx";
import { SystemSection } from "../config/SystemSection.jsx";
import { useAccountManager } from "../../features/settings/index.js";

export function SettingsModal() {
  const { t } = useTranslation("settings");
  // TODO(4b): pages props 化后移除 feature-tmp→app 过渡反向引用
  const open = useOverlayStore((s) => s.settingsOpen);
  const setOpen = useOverlayStore((s) => s.setSettingsOpen);
  const [openSection, setOpenSection] = useState("keys");

  const toggle = (key) => setOpenSection((p) => (p === key ? null : key));

  useEffect(() => {
    if (!open) return;
    const onKey = (e) => { if (e.key === "Escape") setOpen(false); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, setOpen]);

  return (
    <AnimatePresence>
      {open && (
        <motion.div
          className="set-scrim"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.18 }}
          onClick={() => setOpen(false)}
        >
          <motion.div
            className="set-modal"
            {...scaleIn}
            role="dialog"
            aria-modal="true"
            aria-label={t("modal.ariaLabel")}
            onClick={(e) => e.stopPropagation()}
          >
            <div className="set-head">
              <span className="set-head-title">{t("modal.title")}</span>
              <button className="set-x" onClick={() => setOpen(false)}>
                <Icon.X />
              </button>
            </div>
            <div className="set-body">
              <AccountRegion />
              <ApiKeysSection
                open={openSection === "keys"}
                onToggle={() => toggle("keys")}
              />
              <SearchSection
                open={openSection === "search"}
                onToggle={() => toggle("search")}
              />
              <AppearanceSection
                open={openSection === "appearance"}
                onToggle={() => toggle("appearance")}
              />
              <SystemSection
                open={openSection === "system"}
                onToggle={() => toggle("system")}
              />
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}

function AccountRegion() {
  const { t } = useTranslation("settings");
  const { data: users = [] } = useUsers();
  const currentUserId = useSessionStore((s) => s.currentUserId);
  const [mode, setMode] = useState("view");

  const { name, setName, switchTo: switchToAccount, addAccount } = useAccountManager();

  const active = users.find((u) => u.id === currentUserId) || users[0];

  const switchTo = (id) => {
    switchToAccount(id);
    setMode("view");
  };

  const initials = (active?.displayName || active?.username || "?")
    .slice(0, 1).toUpperCase();

  return (
    <div className="set-acct">
      <div
        className="set-ava"
        style={{ background: active?.avatarColor || "var(--accent)" }}
      >
        {initials}
      </div>
      <div className="set-acct-info">
        {mode === "view" ? (
          <>
            <div className="set-acct-name">
              {active?.displayName || active?.username || t("account.noAccount")}
            </div>
            <div className="set-acct-sub">{t("account.localWorkspaceCount", { count: users.length })}</div>
          </>
        ) : (
          <div className="set-acct-list">
            {users.map((u) => (
              <button
                key={u.id}
                className={"set-acct-row" + (u.id === active?.id ? " is-active" : "")}
                onClick={() => switchTo(u.id)}
              >
                <span
                  className="set-ava set-ava-sm"
                  style={{ background: u.avatarColor || "var(--accent)" }}
                >
                  {(u.displayName || u.username || "?").slice(0, 1).toUpperCase()}
                </span>
                <span className="set-acct-row-name">{u.displayName || u.username}</span>
                {u.id === active?.id && <Icon.Check />}
              </button>
            ))}
            <div className="set-acct-add">
              <input
                className="set-acct-add-input"
                placeholder={t("account.newWorkspace")}
                value={name}
                onChange={(e) => setName(e.target.value)}
                onKeyDown={(e) => { if (e.key === "Enter") addAccount(); }}
              />
              <button className="btn btn-xs btn-accent" onClick={addAccount}>
                <Icon.Plus /> {t("common:add")}
              </button>
            </div>
          </div>
        )}
      </div>
      <button
        className="set-pill-btn"
        onClick={() => setMode((m) => (m === "switch" ? "view" : "switch"))}
      >
        {t("account.switchOrNew")}
      </button>
    </div>
  );
}
