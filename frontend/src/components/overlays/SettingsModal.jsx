// SettingsModal — centered overlay with single-open accordion sections.
// Account region always visible; 4 sections mutually exclusive.
//
// SettingsModal —— 居中浮层；账号区常驻；4 个 accordion 区段互斥单展。

import { useEffect, useRef, useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { useQueryClient } from "@tanstack/react-query";
import { Icon } from "../primitives/Icon.jsx";
import { useUIStore } from "../../store/ui.js";
import { useSettings } from "../../store/settings.js";
import { useUsers, useCreateUser } from "../../api/users.js";
import { useDisplayName } from "../../hooks/useDisplayName.js";
import { scaleIn } from "../../motion/tokens.js";
import { ApiKeysSection } from "../config/ApiKeysSection.jsx";
import { SearchSection } from "../config/SearchSection.jsx";
import { AppearanceSection } from "../config/AppearanceSection.jsx";
import { SystemSection } from "../config/SystemSection.jsx";

export function SettingsModal() {
  const open = useUIStore((s) => s.settingsOpen);
  const setOpen = useUIStore((s) => s.setSettingsOpen);
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
        <>
          <motion.div
            className="set-scrim"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.18 }}
            onClick={() => setOpen(false)}
          />
          <motion.div
            className="set-modal"
            {...scaleIn}
            role="dialog"
            aria-modal="true"
            aria-label="设置"
          >
            <div className="set-head">
              <span className="set-head-title">设置</span>
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
        </>
      )}
    </AnimatePresence>
  );
}

function AccountRegion() {
  const { data: users = [] } = useUsers();
  const createUser = useCreateUser();
  const qc = useQueryClient();
  const settings = useSettings();
  const pushToast = useUIStore((s) => s.pushToast);
  const [displayName, setDisplayName] = useDisplayName();
  const [draft, setDraft] = useState(displayName);
  const [mode, setMode] = useState("view");
  const [name, setName] = useState("");

  useEffect(() => { setDraft(displayName); }, [displayName]);

  const active = users.find((u) => u.id === settings.activeUserId) || users[0];

  const switchTo = (id) => {
    settings.set({ activeUserId: id });
    setMode("view");
    qc.invalidateQueries();
    pushToast({ kind: "success", title: "已切到 " + id });
  };

  const addAccount = async () => {
    const username = name.trim();
    if (!username) return;
    try {
      const created = await createUser.mutateAsync({ username });
      switchTo(created.id);
      setName("");
    } catch (e) {
      pushToast({ kind: "error", title: "添加失败", desc: e.message });
    }
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
              {active?.displayName || active?.username || "无账号"}
            </div>
            <div className="set-acct-sub">本地工作空间 · 共 {users.length} 个</div>
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
                placeholder="新名字"
                value={name}
                onChange={(e) => setName(e.target.value)}
                onKeyDown={(e) => { if (e.key === "Enter") addAccount(); }}
              />
              <button className="btn btn-xs btn-accent" onClick={addAccount}>
                <Icon.Plus /> 添加
              </button>
            </div>
          </div>
        )}
      </div>
      <button
        className="set-pill-btn"
        onClick={() => setMode((m) => (m === "switch" ? "view" : "switch"))}
      >
        切换 / 新建
      </button>
    </div>
  );
}
