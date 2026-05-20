// Test runner — imports every spec file, runs all cases sequentially,
// prints aggregated table report, exits non-zero on any failure.
//
// 测试 runner —— 顺跑全部 spec，汇总表格报告，失败非 0 退出。

import { report } from "./lib/harness.mjs";
import boot     from "./specs/01-boot.mjs";
import panes    from "./specs/02-panes.mjs";
import keyboard from "./specs/03-keyboard.mjs";
import overlays from "./specs/04-overlays.mjs";
import settings from "./specs/05-settings.mjs";
import chatEmpty from "./specs/06-chat-empty.mjs";
import chatLive from "./specs/07-chat-live.mjs";
import nesting  from "./specs/08-nesting.mjs";

const all = [
  ...boot, ...panes, ...keyboard, ...overlays,
  ...settings, ...chatEmpty, ...chatLive, ...nesting,
];

console.log(`running ${all.length} cases against ${process.env.FRONTEND_URL || "http://localhost:5173"}`);
console.log(`backend: ${process.env.BACKEND_URL || "http://localhost:8742"}`);

const results = [];
for (const c of all) {
  process.stdout.write(`  · running...`);
  const r = await c();
  process.stdout.write(`\r`);
  results.push(r);
}

const ok = report(results);
process.exit(ok ? 0 : 1);
