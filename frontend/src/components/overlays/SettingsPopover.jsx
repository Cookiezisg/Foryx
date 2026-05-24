// SettingsPopover — quick theme/accent/density tweaks + local-account
// switcher anchored to the sidebar settings button. Click outside to
// close. Heavier settings live in Config pane.
//
// SettingsPopover —— sidebar 设置按钮锚定的快捷面板；顶部账号区，
// 切换 user 触发 invalidateQueries 让所有 per-user 数据刷新。

import { useEffect, useRef, useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { useQueryClient } from "@tanstack/react-query";
import { Icon } from "../primitives/Icon.jsx";
import { useUIStore } from "../../store/ui.js";
import { useSettings } from "../../store/settings.js";
import { useUsers, useCreateUser } from "../../api/users.js";
import { scaleIn } from "../../motion/tokens.js";

const THEMES = [["system", "系统"], ["light", "明"], ["dark", "暗"]];
const DENSITIES = [["compact", "紧凑"], ["cozy", "适中"], ["comfortable", "舒展"]];
const ACCENTS = [
  ["claude", "#d97757"], ["blue", "#2383e2"],
  ["ink", "#37352f"], ["green", "#0f7b6c"], ["purple", "#6940a5"],
];
const LANGS = [["zh", "中文"], ["en", "English"]];

export function SettingsPopover() {
  const open = useUIStore((s) => s.settingsPopOpen);
  const setOpen = useUIStore((s) => s.setSettingsPopOpen);
  const openPane = useUIStore((s) => s.openPane);
  const pushToast = useUIStore((s) => s.pushToast);
  const settings = useSettings();
  const ref = useRef(null);

  useEffect(() => {
    if (!open) return;
    const onClick = (e) => {
      if (ref.current && !ref.current.contains(e.target)) setOpen(false);
    };
    setTimeout(() => window.addEventListener("click", onClick), 0);
    return () => window.removeEventListener("click", onClick);
  }, [open, setOpen]);

  return (
    <AnimatePresence>
      {open && (
        <motion.div
          ref={ref}
          className="settings-pop"
          {...scaleIn}
          onClick={(e) => e.stopPropagation()}
          style={{ position: "fixed", left: 16, bottom: 60, zIndex: 90 }}
        >
          <AccountSection
            settings={settings}
            onClose={() => setOpen(false)}
            pushToast={pushToast}
          />

          <div className="settings-pop-row">
            <span>主题</span>
            <div style={{ display: "flex", gap: 4 }}>
              {THEMES.map(([v, l]) => (
                <button
                  key={v}
                  className={"btn btn-xs" + (settings.theme === v ? " btn-primary" : " btn-ghost")}
                  onClick={() => settings.set({ theme: v })}
                >
                  {l}
                </button>
              ))}
            </div>
          </div>

          <div className="settings-pop-row">
            <span>色调</span>
            <div className="settings-pop-swatches">
              {ACCENTS.map(([k, c]) => (
                <button
                  key={k}
                  className={"settings-pop-swatch" + (settings.accent === k ? " is-active" : "")}
                  style={{ background: c }}
                  onClick={() => settings.set({ accent: k })}
                  title={k}
                />
              ))}
            </div>
          </div>

          <div className="settings-pop-row">
            <span>密度</span>
            <div style={{ display: "flex", gap: 4 }}>
              {DENSITIES.map(([v, l]) => (
                <button
                  key={v}
                  className={"btn btn-xs" + (settings.density === v ? " btn-primary" : " btn-ghost")}
                  onClick={() => settings.set({ density: v })}
                >
                  {l}
                </button>
              ))}
            </div>
          </div>

          <div className="settings-pop-row">
            <span>语言</span>
            <div style={{ display: "flex", gap: 4 }}>
              {LANGS.map(([v, l]) => (
                <button
                  key={v}
                  className={"btn btn-xs" + (settings.lang === v ? " btn-primary" : " btn-ghost")}
                  onClick={() => settings.set({ lang: v })}
                >
                  {l}
                </button>
              ))}
            </div>
          </div>

          <div style={{ borderTop: "1px solid var(--border-soft)", paddingTop: 8, display: "flex", flexDirection: "column", gap: 4 }}>
            <button
              className="settings-pop-link"
              onClick={() => { setOpen(false); openPane("config"); }}
            >
              <Icon.KeyRound /> 完整设置
            </button>
          </div>
        </motion.div>
      )}
    </AnimatePresence>
  );
}

// AccountSection — current avatar + name + "切换" reveals user list +
// add-new-account input. Switching writes settings.activeUserId and
// invalidates ALL queries so per-user data refreshes immediately.
//
// AccountSection —— 当前 avatar + 切换 + 列表 + 添加输入。切换写
// activeUserId + 全 invalidate。
function AccountSection({ settings, onClose, pushToast }) {
  const { data: users = [] } = useUsers();
  const createUser = useCreateUser();
  const qc = useQueryClient();
  const [mode, setMode] = useState("view");
  const [name, setName] = useState("");

  // Resolve active user: settings.activeUserId wins; fallback to first.
  const active = users.find((u) => u.id === settings.activeUserId) || users[0];
  const switchTo = (id) => {
    settings.set({ activeUserId: id });
    setMode("view");
    // Invalidate every query so the new user's data loads fresh.
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

  return (
    <div className="settings-pop-account">
      <div className="settings-pop-account-head">
        <div
          className="settings-pop-account-avatar"
          style={{ background: active?.avatarColor || "#4f46e5" }}
        >
          {(active?.displayName || active?.username || "?").slice(0, 1).toUpperCase()}
        </div>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ fontSize: 13, fontWeight: 600, color: "var(--fg-strong)" }}>
            {active?.displayName || active?.username || "无账号"}
          </div>
          <div style={{ fontSize: 10, color: "var(--fg-faint)" }}>
            本地 · {users.length} 个
          </div>
        </div>
        <button
          className="btn btn-xs btn-ghost"
          onClick={() => setMode((m) => (m === "switch" ? "view" : "switch"))}
        >
          切换
        </button>
      </div>
      {mode === "switch" && (
        <div className="settings-pop-account-list">
          {users.map((u) => (
            <button
              key={u.id}
              className={"settings-pop-account-row" + (u.id === active?.id ? " is-active" : "")}
              onClick={() => switchTo(u.id)}
            >
              <span
                className="settings-pop-account-avatar small"
                style={{ background: u.avatarColor || "#4f46e5" }}
              >
                {(u.displayName || u.username || "?").slice(0, 1).toUpperCase()}
              </span>
              <span style={{ flex: 1 }}>{u.displayName || u.username}</span>
              {u.id === active?.id && <Icon.Check />}
            </button>
          ))}
          <div className="settings-pop-account-add">
            <input
              className="cfg-input"
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
  );
}
