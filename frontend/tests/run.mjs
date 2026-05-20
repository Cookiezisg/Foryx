// Test runner — imports every spec, runs all sequentially, aggregates results.
//
// Sequential by design: many specs mutate shared backend state (create
// convs, add keys), so parallel runs would race. ~3-5 min wall time
// for the full ~100-case suite.
//
// 顺跑全部 spec；并行会撞共享后端状态。

import { report } from "./lib/harness.mjs";
import boot         from "./specs/01-boot.mjs";
import panes        from "./specs/02-panes.mjs";
import keyboard     from "./specs/03-keyboard.mjs";
import overlays     from "./specs/04-overlays.mjs";
import settings     from "./specs/05-settings.mjs";
import chatEmpty    from "./specs/06-chat-empty.mjs";
import chatLive     from "./specs/07-chat-live.mjs";
import nesting      from "./specs/08-nesting.mjs";
import themes       from "./specs/09-themes.mjs";
import paneResize   from "./specs/10-pane-resize.mjs";
import multiPane    from "./specs/11-multi-pane.mjs";
import cmdkDeep     from "./specs/12-cmdk-deep.mjs";
import composer     from "./specs/13-composer.mjs";
import blocks       from "./specs/14-blocks.mjs";
import actionMenus  from "./specs/15-action-menus.mjs";
import overlaysDeep from "./specs/16-overlays-deep.mjs";
import convCrud     from "./specs/17-conversations-crud.mjs";
import memoryCrud   from "./specs/18-memory-crud.mjs";
import apikeyFlow   from "./specs/19-apikey-flow.mjs";
import errorStates  from "./specs/20-error-states.mjs";
import rapid        from "./specs/21-rapid-interaction.mjs";
import localStore   from "./specs/22-localstorage.mjs";
import multiStream  from "./specs/23-multi-conv-stream.mjs";
import sse          from "./specs/24-sse-reconnect.mjs";
import blocksLive   from "./specs/25-blocks-live.mjs";
import a11y         from "./specs/26-keyboard-a11y.mjs";
import viewports    from "./specs/27-viewports.mjs";

const all = [
  ...boot, ...panes, ...keyboard, ...overlays,
  ...settings, ...chatEmpty, ...chatLive, ...nesting,
  ...themes, ...paneResize, ...multiPane, ...cmdkDeep,
  ...composer, ...blocks, ...actionMenus, ...overlaysDeep,
  ...convCrud, ...memoryCrud, ...apikeyFlow,
  ...errorStates, ...rapid, ...localStore,
  ...multiStream, ...sse, ...blocksLive,
  ...a11y, ...viewports,
];

console.log(`running ${all.length} cases against ${process.env.FRONTEND_URL || "http://localhost:5173"}`);
console.log(`backend: ${process.env.BACKEND_URL || "http://localhost:8742"}`);

const results = [];
let i = 0;
for (const c of all) {
  i++;
  process.stdout.write(`  [${i}/${all.length}] running...`);
  const r = await c();
  process.stdout.write(`\r`);
  results.push(r);
}

const ok = report(results);
process.exit(ok ? 0 : 1);
