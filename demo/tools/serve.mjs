#!/usr/bin/env node
/* Anselm demo — 静态预览服务（no-store）。
   why：python http.server 会缓存子资源（CSS/JS），改文件后预览看到旧版 → 调试踩坑。
   本服务对所有响应发 Cache-Control: no-store，编辑即时生效。无依赖、纯 Node。
   用法：node demo/tools/serve.mjs  （由 .claude/launch.json 的 demo 配置拉起，端口 4192）。 */
import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import { join, extname, normalize } from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = join(fileURLToPath(import.meta.url), "..", "..");   // demo/
const PORT = 4192;
const MIME = {
  ".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8",
  ".mjs": "text/javascript; charset=utf-8", ".css": "text/css; charset=utf-8",
  ".json": "application/json; charset=utf-8", ".svg": "image/svg+xml", ".map": "application/json",
  ".woff2": "font/woff2", ".png": "image/png", ".jpg": "image/jpeg",
};

createServer(async (req, res) => {
  let p = decodeURIComponent((req.url || "/").split("?")[0]);
  if (p === "/" || p.endsWith("/")) p += "app.html";
  const file = normalize(join(ROOT, p));
  if (!file.startsWith(ROOT)) { res.writeHead(403); return res.end("forbidden"); }
  try {
    const body = await readFile(file);
    res.writeHead(200, { "Content-Type": MIME[extname(file)] || "application/octet-stream", "Cache-Control": "no-store, must-revalidate" });
    res.end(body);
  } catch {
    res.writeHead(404, { "Cache-Control": "no-store" });
    res.end("not found: " + p);
  }
}).listen(PORT, "127.0.0.1", () => console.log("demo no-cache server → http://127.0.0.1:" + PORT));
