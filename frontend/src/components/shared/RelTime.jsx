// RelTime — Chinese relative time with absolute time in title. Re-renders
// every 30s so "刚刚" → "1 分钟前" updates without page reload.
//
// RelTime —— 中文相对时间；title 属性给绝对时间；每 30s 重渲染一次。

import { useEffect, useState } from "react";

function format(d) {
  const diff = (Date.now() - d.getTime()) / 1000;
  if (diff < 5) return "刚刚";
  if (diff < 60) return Math.floor(diff) + " 秒前";
  if (diff < 3600) return Math.floor(diff / 60) + " 分钟前";
  if (diff < 86400) return Math.floor(diff / 3600) + " 小时前";
  if (diff < 86400 * 30) return Math.floor(diff / 86400) + " 天前";
  return d.toLocaleDateString("zh-CN", { month: "short", day: "numeric" });
}

function absolute(d) {
  return d.toLocaleString("zh-CN", {
    year: "numeric", month: "2-digit", day: "2-digit",
    hour: "2-digit", minute: "2-digit", second: "2-digit",
  });
}

export function RelTime({ ts, prefix = "" }) {
  const [, tick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => tick((n) => n + 1), 30_000);
    return () => clearInterval(id);
  }, []);

  if (!ts) return null;
  const d = typeof ts === "string" || typeof ts === "number" ? new Date(ts) : ts;
  if (isNaN(d.getTime())) return null;

  return <time title={absolute(d)}>{prefix}{format(d)}</time>;
}
