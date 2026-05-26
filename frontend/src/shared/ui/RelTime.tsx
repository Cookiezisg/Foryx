// RelTime — bilingual relative time with absolute time in title. Re-renders
// every 30s so "just now" → "1m ago" updates without page reload.
//
// RelTime —— 双语相对时间；title 属性给绝对时间；每 30s 重渲染一次。

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

interface RelTimeProps {
  ts?: string | number | Date;
  prefix?: string;
}

export function RelTime({ ts, prefix = "" }: RelTimeProps) {
  const { t, i18n } = useTranslation("misc");
  const [, tick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => tick((n) => n + 1), 30_000);
    return () => clearInterval(id);
  }, []);

  if (!ts) return null;
  const d = typeof ts === "string" || typeof ts === "number" ? new Date(ts) : ts;
  if (isNaN(d.getTime())) return null;

  const locale = i18n.language === "zh" ? "zh-CN" : "en-US";

  function format(date: Date) {
    const diff = (Date.now() - date.getTime()) / 1000;
    if (diff < 5) return t("relTime.justNow");
    if (diff < 60) return t("relTime.secondsAgo", { n: Math.floor(diff) });
    if (diff < 3600) return t("relTime.minutesAgo", { n: Math.floor(diff / 60) });
    if (diff < 86400) return t("relTime.hoursAgo", { n: Math.floor(diff / 3600) });
    if (diff < 86400 * 30) return t("relTime.daysAgo", { n: Math.floor(diff / 86400) });
    return date.toLocaleDateString(locale, { month: "short", day: "numeric" });
  }

  function absolute(date: Date) {
    return date.toLocaleString(locale, {
      year: "numeric", month: "2-digit", day: "2-digit",
      hour: "2-digit", minute: "2-digit", second: "2-digit",
    });
  }

  return <time title={absolute(d)}>{prefix}{format(d)}</time>;
}
