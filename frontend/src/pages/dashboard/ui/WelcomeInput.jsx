// WelcomeInput — pill-shaped composer on the welcome page. Enter submits
// (Shift+Enter inserts newline). Empty / whitespace-only is no-op.
//
// WelcomeInput —— 欢迎页输入框;Enter 直接发(Shift+Enter 换行);空内容
// 不触发;parent 拿到 text 后串行新建对话 + 发首条消息。

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Icon } from "../../../components/primitives/Icon.jsx";

export function WelcomeInput({ onSubmit, isSubmitting = false }) {
  const { t } = useTranslation("dashboard");
  const [text, setText] = useState("");

  const submit = () => {
    const trimmed = text.trim();
    if (!trimmed) return;
    onSubmit(trimmed);
    setText("");
  };

  const onKeyDown = (e) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      submit();
    }
  };

  return (
    <div className="wel-input">
      <span className="wel-input-icon"><Icon.Plus size={18} strokeWidth={2} /></span>
      <textarea
        className="wel-input-area"
        placeholder={t("input.placeholder")}
        value={text}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={onKeyDown}
        disabled={isSubmitting}
        rows={1}
      />
      <button type="button" className="wel-input-send" onClick={submit} disabled={isSubmitting || !text.trim()}>
        <Icon.Send size={16} strokeWidth={2} />
      </button>
    </div>
  );
}
