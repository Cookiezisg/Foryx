/* Anselm feature — chat 海洋（sea）：AI 对话运行时主战场。
   布局：中央 = an-page（居中列：an-block-tree transcript）+ 底部固定 an-composer；会话名走 shell 左上角紧凑标题（恒显）；
     右岛 = <an-entity-workspace>（v2，= entities 流的实体面板镜像）：「跟着对话长出来」——对话起步无右岛，主对话首个 tool call 触发某 item 才挂出右岛显该 item；后续触发的 item 进右上角下拉选择器，active 随最新触发而变。
     每 item（5 实体 kind + Todo + Subagent）一套 canonical 完整岛屿（an-tabs 切全量 facet，未触及 facet 显空态）。无 item → 右岛收起、对话全宽。
   契约落地（mock 演示）：Send=202+SSE → 这里以脚本回放模拟流式回合（每对话同时只一个在途回合 → generating 时 composer 切「停止」）；
     DB 行是真相 → 对话流 blocks（messages 流）+ 右岛实体面板（entities 流）双写：同一回合步既 push tool_call 块到 transcript，又驱动右岛对应 item 出现/置 active/切 facet/流式填充（edit 立即生效、可 revert，无草稿/采用门）。
   脚本解释器消费 data.js 的 turn 步：push/patch/stream（文本逐 token）/progressStream（终端逐行）/island{item,facet,op}（驱动右岛：ensure+setActive+focus+op 流式，流式数据取自 facet 种子单源）/islandTodo{item,items}（Todo item 看板）/islandStatus{item,status}（item 态机→picker 点）/gate（人在环门，只在对话流渲）。
   串接：composer an-send→追加 user 块 + 跑回复回合 · an-stop→停 · block-tree an-continue→续跑 · an-ref→Intent.select；rail 选会话→Intent.on('conversation')→loadConvo。 */
window.FEATURE = window.FEATURE || {};
window.FEATURE.chat = Object.assign(window.FEATURE.chat || {}, {
  sea: (ctx) => {
    const el = window.el;
    const KIND = window.ENTITY_KINDS || {};
    const CONVOS = window.CHAT_CONVOS || {};
    const toast = (t) => window.AnToast && window.AnToast.show({ text: t });

    // ── 持久骨架（切会话只更新内容，不重建）──
    // chat 无文章式大标题——会话名一上来即显于左上角紧凑标题（shell.setHeadTitle，恒 collapsed）；transcript 直接顶到头栏下。
    const page = el("an-page");
    const tree = el("an-block-tree");
    page.append(tree);
    const composer = el("an-composer");
    composer.mentions = window.CHAT_MENTIONS || [];
    const root = el("div", { class: "chat-sea" });
    root.style.cssText = "flex:1; min-height:0; display:flex; flex-direction:column; position:relative;";   // relative：承载空态居中浮层
    root.append(page, composer);

    // ── 会话/回合态 ──
    let cur = null;          // 当前会话
    let landing = null;      // 空态居中浮层（New-chat 落地：问候 + 居中药丸 composer）
    let blocks = [];         // live transcript（耐久态）
    let timers = [];         // 脚本步定时器（切会话全清）
    let gateListener = null; // 等待中的 an-decide 监听
    let ws = null;           // 右岛实体工作台（an-entity-workspace），无 item 则 null
    let islShell = null;     // 承载 ws 的 headless an-right-island（皮肤壳）
    let wsMounted = false;   // 右岛是否已挂（autoplay 下首个触发才挂=「跟着对话长出来」）

    const setBlocks = (b) => { blocks = b; tree.blocks = blocks; requestAnimationFrame(() => page.scrollToBottom()); };
    const pushBlock = (b) => setBlocks(blocks.concat([b]));
    const patchLast = (b) => setBlocks(blocks.slice(0, -1).concat([b]));

    function clearTurn() {
      timers.forEach(clearTimeout); timers = [];
      if (gateListener) { tree.removeEventListener("an-decide", gateListener); gateListener = null; }
    }
    const after = (ms, fn) => { timers.push(setTimeout(fn, ms)); };

    // ── 脚本解释器：instant 步（push/patch）+ 流式步，对齐后端 Open→Delta*→Close ──
    function applyStep(s) {
      if (!s) return;
      if (s.push) pushBlock(s.push);
      if (s.patch) patchLast(s.patch);
    }
    // 文本/推理逐 token 流出（先 push 空块=Open 整渲一次 → 每帧 pokeText 就地增量=Delta → 末帧落 blocks[i]=Close 快照）
    function streamBlock(spec, done) {
      const b = Object.assign({}, spec, { text: "" });
      blocks = blocks.concat([b]); tree.blocks = blocks;
      const i = blocks.length - 1, full = spec.text || "";
      const toks = full.match(/\s+|\S+/g) || (full ? [full] : []);
      const tps = spec.tps || 26;   // tokens/sec
      let acc = "", k = 0;
      const step = () => {
        if (!cur) return;
        if (k >= toks.length) { blocks[i].text = full; tree.pokeText(i, full); page.scrollToBottom(); if (done) done(); return; }
        acc += toks[k++]; blocks[i].text = acc; tree.pokeText(i, acc); page.scrollToBottom();
        timers.push(setTimeout(step, Math.round(1000 / tps)));
      };
      step();
    }
    // progress 终端式 live 流：push 空 progress 块（Open）→ pokeLog 逐行追加（Delta，实时脉冲）→ done:true 落定（Close）
    function streamLog(spec, done) {
      const lines = Array.isArray(spec.lines) ? spec.lines : [];
      blocks = blocks.concat([{ type: "progress", label: spec.label, done: false, lines: [] }]); tree.blocks = blocks;
      const i = blocks.length - 1, lps = spec.lps || 6;
      let k = 0;
      const step = () => {
        if (!cur) return;
        if (k >= lines.length) { blocks[i].done = true; blocks[i].lines = lines; tree.blocks = blocks; page.scrollToBottom(); if (done) done(); return; }
        k++; blocks[i].lines = lines.slice(0, k); tree.pokeLog(i, blocks[i].lines); page.scrollToBottom();
        timers.push(setTimeout(step, Math.round(1000 / lps)));
      };
      step();
    }

    // ── 右岛驱动（= entities 流镜像）：focus 切实体 tab + 子视图，再按 op 把流式喂进该 view 的 live 原语 ──
    // 代码逐字流入（create→an-code-editor.value / 镜像 build 流 arg delta，close 快照才是重连真相）
    function streamCode(ed, full, cps, done) {
      if (!ed) { if (done) done(); return; }
      const chunk = 2; let n = 0;
      const step = () => {
        if (!cur) return;
        if (n >= full.length) { ed.value = full; if (done) done(); return; }
        n = Math.min(full.length, n + chunk); ed.value = full.slice(0, n);
        timers.push(setTimeout(step, Math.round(1000 / (cps / chunk))));
      };
      step();
    }
    // 编辑逐字流入（edit→an-version-diff.after 累积，每设触发 LCS 重算红绿；before=旧 active 版本源、立即生效无审批门）
    function streamDiff(diff, before, full, cps, done) {
      if (!diff) { if (done) done(); return; }
      if (before != null) diff.before = before;
      diff.after = "";
      const chunk = 3; let n = 0;
      const step = () => {
        if (!cur) return;
        if (n >= full.length) { diff.after = full; if (done) done(); return; }
        n = Math.min(full.length, n + chunk); diff.after = full.slice(0, n);
        timers.push(setTimeout(step, Math.round(1000 / (cps / chunk))));
      };
      step();
    }
    // flowrun 逐节点点亮（trigger 产出 durable flowrun；镜像 ephemeral Signal 无 open→close，纯终态 tick）
    function streamGantt(g, nodes, lps, done) {
      if (!g || !nodes.length) { if (done) done(); return; }
      const seed = nodes.map((n) => Object.assign({}, n, { status: "future", wPct: 0, iters: null, parked: false }));
      g.nodes = seed.map((n) => Object.assign({}, n));
      let k = 0;
      const step = () => {
        if (!cur) return;
        if (k >= nodes.length) { g.nodes = nodes.map((n) => Object.assign({}, n)); if (done) done(); return; }
        g.nodes = seed.map((s, i) => (i <= k ? Object.assign({}, nodes[i]) : s));
        k++; timers.push(setTimeout(step, Math.round(1000 / lps)));
      };
      step();
    }
    // agent 轨迹逐块流入（invoke→嵌套 an-block-tree 的 ReAct；与对话流 subtree 同一轨迹两处呈现）
    function streamTrace(bt, list, done) {
      if (!bt || !list.length) { if (done) done(); return; }
      bt.blocks = []; let acc = [], k = 0;
      const step = () => {
        if (!cur) return;
        if (k >= list.length) { bt.blocks = list.slice(); if (done) done(); return; }
        acc = acc.concat([list[k++]]); bt.blocks = acc.slice();
        timers.push(setTimeout(step, 520));
      };
      step();
    }
    // 首次触发才挂右岛（autoplay「跟着对话长出来」）；幂等
    function mountIsland() { if (!wsMounted) { if (ctx.shell) ctx.shell.setRight(islShell); wsMounted = true; } }
    function driveIsland(drive, done) {
      if (!ws || !drive || !drive.item) { if (done) done(); return; }
      mountIsland();
      ws.ensure(drive.item);          // item 入岛 + 进 picker（首个 ensure 即出现点）
      const target = ws.focus(drive.item, drive.facet);   // active 跟随 + 切 facet + 拿 live 元素
      const op = drive.op;
      if (!op) { if (done) done(); return; }   // 仅切视图（静态切换）
      const fs = (ws.facetSpec && ws.facetSpec(drive.item, drive.facet)) || {};   // 流式数据单源 = facet 种子
      if (op === "create") return streamCode(target, drive.code != null ? drive.code : (fs.code || ""), drive.cps || 150, done);
      if (op === "edit") return streamDiff(target, drive.before != null ? drive.before : fs.before, drive.after != null ? drive.after : (fs.after || ""), drive.cps || 150, done);
      if (op === "run") {
        if (target) {
          if (drive.args != null) target.setAttribute("args", drive.args);
          if (drive.trace) target.setAttribute("data-trace", JSON.stringify(drive.trace));
          if (target.run) target.run();
        }
        const ln = (drive.trace && drive.trace.lines ? drive.trace.lines.length : 3);
        after((ln + 2) * 260, done); return;
      }
      if (op === "flowrun") return streamGantt(target, drive.nodes || fs.nodes || [], drive.lps || 3, done);
      if (op === "trace") return streamTrace(target, drive.blocks || fs.blocks || [], done);
      if (done) done();
    }
    // todo_write 步：Todo 成独立 item，ensure + 置 active（跟随）+ 整表喂看板
    function driveTodo(d, done) {
      if (!ws || !d || !d.item) { if (done) done(); return; }
      mountIsland();
      ws.ensure(d.item); ws.setActive(d.item); ws.setTodo(d.item, d.items || []);
      if (done) done();
    }

    function runStep(s, done) {
      if (!s) { if (done) done(); return; }
      if (s.stream) { after(s.ms || 250, () => streamBlock(s.stream, done)); return; }
      if (s.progressStream) { after(s.ms || 250, () => streamLog(s.progressStream, done)); return; }
      // island 步：与对话流块同步推进——先落 push/patch（messages 真相），再驱动右岛（entities 镜像）
      if (s.island) { after(s.ms || 250, () => { applyStep(s); driveIsland(s.island, done); }); return; }
      if (s.islandTodo) { after(s.ms || 250, () => { applyStep(s); driveTodo(s.islandTodo, done); }); return; }
      if (s.islandStatus) { after(s.ms || 250, () => { applyStep(s); if (ws) ws.setItemStatus(s.islandStatus.item, s.islandStatus.status); if (done) done(); }); return; }
      after(s.ms || 300, () => { applyStep(s); if (done) done(); });
    }
    function runTurn(steps, i) {
      if (!cur || !steps || i >= steps.length) { composer.removeAttribute("generating"); return; }
      const s = steps[i], next = () => runTurn(steps, i + 1);
      if (s.gate) {   // 人在环门（只在对话流 block-tree 渲，右岛不重复门）：等 an-decide（approve/accept→onApprove · deny/decline→onDeny）；3.5s 无人动自动放行（自演）
        gateListener = (ev) => { clearTurn(); const a = ev.detail.action; const no = a === "deny" || a === "decline"; runStep(no ? s.gate.onDeny : s.gate.onApprove, next); };
        tree.addEventListener("an-decide", gateListener);
        after(3500, () => { if (gateListener) { tree.removeEventListener("an-decide", gateListener); gateListener = null; } runStep(s.gate.onApprove, next); });
        return;
      }
      runStep(s, next);
    }
    function startTurn(steps) { if (!steps || !steps.length) return; composer.setAttribute("generating", ""); runTurn(steps, 0); }

    // 用户发送后的脚本回复回合（逐 token 流式；演示真实经 messages SSE Delta 帧）
    const replyTurn = () => [
      { ms: 400, stream: { type: "reasoning", open: true, label: "推理", text: "理解用户的追加请求，给出简短回应。", tps: 42 } },
      { ms: 450, stream: { type: "text", text: "收到 👍 我会按这个调整。\n\n（演示：真实回合经 messages SSE 逐 token 流式产出，此处为脚本流式回放。）", tps: 24 } },
    ];

    // ── 右岛实体工作台（v2）：据 c.items（本对话所有可能 item）建 headless 岛；
    //   已完成对话（无 autoplay）载入即全 ensure + 停最后 active；autoplay 先不挂、首个 island 步触发才「长出来」。──
    function buildWorkspace(c) {
      ws = null; islShell = null; wsMounted = false;
      const items = c.items || [];
      if (!items.length) { if (ctx.shell) ctx.shell.setRight(null); return; }
      islShell = el("an-right-island", { headless: true });
      ws = el("an-entity-workspace");
      ws.model = { items };
      ws.addEventListener("an-revert", (e) => toast("已 revert · " + (e.detail.name || "") + " 回退到上一版本（revert_* 移 active 指针）"));
      islShell.append(ws);
      if (!c.autoplay) {
        if (ctx.shell) ctx.shell.setRight(islShell); wsMounted = true;   // 载入即显（先连上 ws 再 ensure/setActive）
        items.forEach((it) => ws.ensure(it.id));
        ws.setActive(c.activeItem || items[items.length - 1].id);
      } else if (ctx.shell) ctx.shell.setRight(null);   // 跟着对话长出来：首个 island 步才挂
    }

    // ── 空态（New-chat 居中落地，ChatGPT 式）：root 内绝对浮层，垂直居中 = 问候打字机（不加粗，prefix + 5s 轮播）+ 居中药丸 composer。无 chips、无左上角标题、右岛收起。──
    function showEmpty() {
      clearTurn(); cur = null;
      composer.removeAttribute("generating"); composer.clear && composer.clear();
      composer.style.transition = ""; composer.style.transform = "";
      if (ctx.shell) {
        ctx.shell.setRight(null);
        ctx.shell.setHeadTitle && ctx.shell.setHeadTitle(null);   // 左上角不显示 New Chat
        ctx.shell.setHeadMenu && ctx.shell.setHeadMenu(null);
      }
      page.replaceChildren();   // 清空 transcript（landing 浮层覆盖之）
      if (landing) landing.remove();
      landing = el("div", { class: "chat-landing" });
      landing.style.cssText = "position:absolute; inset:0; z-index:2; display:flex; flex-direction:column; align-items:center; justify-content:center; gap:var(--sp-16); padding:var(--sp-6);";
      const greet = el("div");   // 排版尺度容器（typewriter 自带 ink/ink-2 色，故不内联皮肤色）；字体不加粗
      greet.style.cssText = "font-size:var(--t-h2); font-weight:400; line-height:var(--lh-tight); text-align:center; max-width:var(--w-content);";
      const tw = el("an-typewriter", { prefix: window.greetOf() + ", ", pause: "5000" });
      tw.phrases = window.CHAT_GREET_PHRASES || [];
      greet.append(tw);
      composer.setAttribute("pill", "");
      composer.style.width = "100%";   // 居中浮层 align-items:center 会 shrink-fit composer → 强制满宽，让药丸到 .bar 的 --w-content（宽药丸、非小圆）
      landing.append(greet, composer);   // 把 composer 移进居中浮层（药丸态）
      root.append(landing);
    }
    // 取消落地态、composer 复位到底部（导航进会话用，无动画）
    function restoreComposer() {
      if (landing) { landing.remove(); landing = null; }
      composer.removeAttribute("pill"); composer.style.transition = ""; composer.style.transform = ""; composer.style.width = "";
      if (composer.parentNode !== root) root.append(composer);
    }
    // 首条消息：composer 从居中药丸 FLIP 滑到底部（First-Last-Invert-Play），transcript 浮现
    function slideToChat() {
      const first = composer.getBoundingClientRect();
      composer.removeAttribute("pill"); composer.style.width = "";
      if (tree.parentNode !== page) page.replaceChildren(tree);
      if (landing) { landing.remove(); landing = null; }
      if (composer.parentNode !== root) root.append(composer);
      if (ctx.shell) { ctx.shell.setHeadTitle && ctx.shell.setHeadTitle("新对话"); ctx.shell.setHeadCollapsed && ctx.shell.setHeadCollapsed(true); }
      const last = composer.getBoundingClientRect();
      composer.style.transition = "none";
      composer.style.transform = "translate(" + (first.left - last.left) + "px, " + (first.top - last.top) + "px)";
      composer.getBoundingClientRect();   // 强制 reflow 锁定 invert 起点
      requestAnimationFrame(() => {
        composer.style.transition = "transform var(--d-slow) var(--ease-spring)";
        composer.style.transform = "translate(0px, 0px)";
      });
      const done = () => { composer.style.transition = ""; composer.style.transform = ""; composer.removeEventListener("transitionend", done); };
      composer.addEventListener("transitionend", done);
    }

    // ── 切会话 ──
    function loadConvo(id) {
      clearTurn();
      if (id == null) return showEmpty();   // 无选中 / 点 New → New-chat 居中空态
      const c = CONVOS[id] || CONVOS[window.CHAT_DEFAULT]; if (!c) return;
      cur = c;
      restoreComposer();
      if (tree.parentNode !== page) page.replaceChildren(tree);   // 从空态回来 → 恢复 transcript
      // 会话名 → 左上角紧凑标题（chat 恒 collapsed=一上来即显）；标题点回顶，⌄ 开对话动作菜单
      if (ctx.shell && ctx.shell.setHeadTitle) {
        ctx.shell.setHeadTitle(c.title || "对话", () => page.scrollToTop(true));
        ctx.shell.setHeadCollapsed(true);
        ctx.shell.setHeadMenu && ctx.shell.setHeadMenu((a) => window.AnMenu.open(a, {
          align: "end", placement: "bottom", namespace: "chat-head-menu",
          items: [
            { value: "rename", label: "重命名", icon: "edit" },
            { value: "pin", label: "置顶", icon: "history" },
            { value: "archive", label: "归档", icon: "enter" },
            { value: "delete", label: "删除", icon: "trash", danger: true },
          ],
          onPick: (v) => toast(({ rename: "已重命名", pin: "已置顶", archive: "已归档", delete: "已删除" }[v]) + "「" + (c.title || "") + "」"),
        }));
      }
      composer.removeAttribute("generating");
      composer.attachments = [];
      // 右岛实体工作台
      buildWorkspace(c);
      // 初始 transcript + 自动播放脚本回合
      setBlocks((c.blocks || []).slice());
      if (c.autoplay && c.turn) after(700, () => startTurn(c.turn));
    }

    // ── 串接 ──
    composer.addEventListener("an-send", (ev) => {
      if (composer.hasAttribute("generating")) return;
      const d = ev.detail || {};
      if (!cur) {   // 空态首条消息 → composer 从居中药丸 FLIP 滑到底部 + 起新对话
        cur = { id: "__new__", title: "新对话", kind: "chat" };
        blocks = [];
        slideToChat();
      }
      pushBlock({ type: "text", role: "user", html: d.html || window.anEsc(d.text || "") });
      startTurn(replyTurn());
    });
    // :cancel → 回合终态快照 stopReason=cancelled（对齐后端 message_stop status=cancelled），非裸文本
    composer.addEventListener("an-stop", () => { clearTurn(); composer.removeAttribute("generating"); pushBlock({ type: "turnEnd", stopReason: "cancelled" }); });
    tree.addEventListener("an-continue", () => { if (composer.hasAttribute("generating")) return; startTurn([{ ms: 300, stream: { type: "text", text: "继续——本回合接着上一步推进（演示，逐 token 流式）。", tps: 24 } }]); });
    // ref-pill 点击（transcript 内 @提及 / 实体引用）→ 统一前门 Intent.select
    root.addEventListener("an-ref", (ev) => { const d = ev.detail || {}; if (d.kind && d.id) ctx.Intent.select({ kind: "entity", id: d.id, source: "chat" }); });

    ctx.Intent.on("conversation", (sel) => { if (!root.isConnected) return; if (sel && sel.id) loadConvo(sel.id); else showEmpty(); });
    showEmpty();   // 默认进海洋 = New-chat 空态（选会话 / 点 chip 才进对话）
    return root;
  },
});
