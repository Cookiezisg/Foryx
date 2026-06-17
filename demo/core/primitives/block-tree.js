/* Anselm 原语 G3 — <an-block-tree>。对话块流 + agent transcript 的统一渲染面（chat 核心，也被 agent 详情页复用）。
   声明式：JS 属性 .blocks = [{type,...}] → 渲染一列块；类型决定语汇。无命令式 builder（杀多海洋内联漂移）。
   块型（8）：
     text       —— markdown 正文（轻量内联 md：**粗** `码` 段落）；role="user" → 右对齐浅灰气泡，assistant（缺省）→ 左对齐通栏
     reasoning  —— 思考块（脑图标 + 默认折叠 · 视觉灵魂铁律）
     tool_call  —— 工具组（扳手图标）：running 流光摘要 → settle 收敛摘要。展开见组内逐项；
                   每项 = 项头（图标 + 人性化动词 + mono 名 + danger 徽）+ 平铺三段：输入(JSON→json-tree) / 日志(stderr 平打印) / 输出(JSON→json-tree)——输入输出同构、一致
     tool_result—— 独立输出块（JSON → <an-json-tree>；非 JSON 退化 mono 打印）
     progress   —— 实时 stderr/yield（终端图标 + run 流光头 + live 脉冲点 → done 静态）
     compaction —— 静默压缩标记（无框无线耳语）
     turnEnd    —— max_steps 诚实终态（旗标 + warn 浮卡 + code 徽 + Continue）
     subtree    —— E3 subagent 子树（分叉图标 + 可折叠，默认收起 + 嵌套 <an-block-tree> + 左导轨缩进）
   名称铁律：所有 tool 动词经 anLabel 人性化（run_function → run function），从不裸露 snake_case；tool 图标经 toolIcon(动词) 按工具区分。
   折叠机制：grid-template-rows 0fr→1fr（spring）。运行态：shimmer 流光文字 + 实时脉冲点。
   复用：status-dot · code-editor（inline 高亮）· json-tree · approval-gate(flavor=chat) · icons(toolIcon)。
   交互对外（composed CustomEvent）：an-continue（turnEnd 续跑）· an-decide 由内嵌 approval-gate 自冒泡。
   数据：.blocks 走 JS 属性（数组不经线缆）；设入即重渲染。块可带 open:true 预展开、running:true 跑态。 */
(function () {
  const e = window.anEsc;
  const ic = (name, size, stroke) => (window.icon ? window.icon(name, size, stroke) : "");
  // 名称人性化（run_function → run function）；全 block-tree 统一走它，绝不裸露 snake_case。
  const human = (s) => (window.anLabel ? window.anLabel(s) : String(s == null ? "" : s));

  // 轻量内联 markdown：**粗** · `码` · 段落（\n\n 分段）。先转义、再放行白名单标记——杜绝注入。
  function md(src) {
    let s = e(src);
    s = s.replace(/\*\*([^*]+)\*\*/g, "<b>$1</b>");
    s = s.replace(/`([^`]+)`/g, '<code class="md-code">$1</code>');
    return s.split(/\n{2,}/).map((p) => "<p>" + p.replace(/\n/g, "<br>") + "</p>").join("");
  }

  // danger 三级（S18 自报）→ <an-badge> tone（safe 不显）。
  const DANGER_TONE = { cautious: "warn", dangerous: "danger" };

  class AnBlockTree extends window.AnElement {
    static tag = "an-block-tree";
    static observed = ["nested"];
    static css = `
      :host { display: block; }
      .stream { display: flex; flex-direction: column; min-width: 0; }

      /* 共用：折叠容器内壁（grid-rows 0fr→1fr 的被裁子） + chevron + 行首语义图标 */
      .w { overflow: hidden; min-width: 0; }
      .chev { display: inline-grid; place-items: center; flex: none; transition: transform var(--d-slow) var(--ease-spring); }
      .chev svg { width: var(--icon-sm); height: var(--icon-sm); }
      .lico { display: inline-grid; place-items: center; flex: none; color: var(--ink-3); }
      .lico svg { width: var(--icon-sm); height: var(--icon-sm); }

      /* 共用：运行态流光文字 */
      .shimmer {
        background: linear-gradient(105deg, var(--ink-3) 36%, var(--ink) 50%, var(--ink-3) 64%);
        background-size: 220% 100%; -webkit-background-clip: text; background-clip: text;
        color: transparent; animation: bkShimmer var(--d-breath) linear infinite;
      }
      @keyframes bkShimmer { from { background-position: 130% 0; } to { background-position: -30% 0; } }
      @keyframes bkLineIn { from { opacity: 0; transform: translateY(var(--line-2)); } to { opacity: 1; transform: none; } }

      /* ── text：assistant 通栏 / user 右对齐浅灰气泡 ── */
      .text { font-size: var(--t-body); color: var(--ink); line-height: var(--lh-prose); margin: var(--sp-2) 0; }
      .text p { margin: 0 0 var(--sp-2); }
      .text p:last-child { margin-bottom: 0; }
      .text b { font-weight: 600; }
      .text .md-code { font-family: var(--mono); font-size: var(--t-meta);
        background: var(--island-3); border-radius: var(--r-tag); padding: var(--hairline) var(--gap-tight); }
      .text.user { display: flex; justify-content: flex-end; margin: var(--sp-3) 0; }
      .text.user .bubble { max-width: 80%; min-width: 0; background: var(--island-3); color: var(--ink);
        padding: var(--sp-2) var(--sp-3); border-radius: var(--r-chip); overflow-wrap: anywhere; }
      .text.user .bubble p:last-child { margin-bottom: 0; }

      /* ── reasoning：脑图标 + 默认折叠 ── */
      .reason { margin: var(--sp-3) 0; }
      .reason-sum { min-width: 0; display: inline-flex; align-items: center; gap: var(--gap-tight);
        color: var(--ink-3); font-size: var(--t-body); cursor: pointer; padding: var(--grid) 0; }
      .reason-sum:hover { color: var(--ink-2); }
      .reason.open .reason-sum .chev { transform: rotate(90deg); }
      .reason-body { display: grid; grid-template-rows: 0fr; transition: grid-template-rows var(--d-slow) var(--ease-spring); }
      .reason.open .reason-body { grid-template-rows: 1fr; }
      .reason-text { margin-top: var(--sp-2); color: var(--ink-2); font-size: var(--t-body);
        line-height: var(--lh-prose); white-space: pre-wrap; }

      /* ── tool_call：扳手图标 + 运行流光 → settle 收敛 + 外框工具列表 ── */
      .tg { margin: var(--sp-3) 0; }
      .tg-sum { min-width: 0; display: inline-flex; align-items: center; gap: var(--gap-tight);
        color: var(--ink-3); font-size: var(--t-body); cursor: pointer; padding: var(--grid) 0; }
      .tg-sum:hover { color: var(--ink-2); }
      .tg.run .tg-sum { cursor: default; }
      .tg.run .tg-sum:hover { color: var(--ink-3); }
      .tg.open .tg-sum .chev { transform: rotate(90deg); }
      .tg.run .tg-sum .chev { display: none; }
      .tg-list { display: grid; grid-template-rows: 0fr; transition: grid-template-rows var(--d-slow) var(--ease-spring); }
      .tg.open .tg-list { grid-template-rows: 1fr; }
      /* 描边走 inset box-shadow 而非 border——半透明 border 圆角四角 1px 自叠加成"灰尖"，inset 环均匀无叠加。 */
      .tg-box { margin-top: var(--sp-2); box-shadow: inset 0 0 0 var(--hairline) var(--line);
        border-radius: var(--r-card); padding: var(--sp-3); }

      /* 工具项：项头（图标 + 动词 + 名 + danger 徽）+ 平铺三段（输入 / 日志 / 结果）。
         三段同一"小灰标签 + 内容"语汇、左缘对齐、无盒中盒、无诡异缩进——一眼读懂输入→日志→结果。 */
      .ti { display: flex; flex-direction: column; gap: var(--sp-3); min-width: 0; }
      .ti + .ti { margin-top: var(--sp-3); padding-top: var(--sp-3); border-top: var(--hairline) solid var(--line); }
      .ti-head { display: flex; align-items: center; gap: var(--gap-tight); font-size: var(--t-body); color: var(--ink-3); }
      .ti-head .v { color: var(--ink-2); }
      .ti-head .nm { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--ink); font-family: var(--mono); font-size: var(--t-meta); }
      .ti-head .danger-badge { margin-left: var(--grid); }
      .ti > an-approval-gate { margin: var(--zero); }

      /* 三段共用：小灰标签 + 内容，竖排左缘齐、无边框（容器边界由 tg-box / block-sec 提供） */
      .sec { display: flex; flex-direction: column; gap: var(--grid); min-width: 0; }
      .sec-label { display: inline-flex; align-items: center; gap: var(--gap-tight); font-size: var(--t-meta); color: var(--ink-3); }
      .sec-label .lt { display: inline-flex; align-items: center; gap: var(--gap-tight); margin-left: var(--gap-tight); color: var(--accent); font-weight: 500; }
      .sec an-code-editor, .sec an-json-tree { display: block; }
      .log { min-width: 0; font-family: var(--mono); font-size: var(--t-meta); line-height: var(--lh-prose); color: var(--ink-2); white-space: pre-wrap; overflow-wrap: anywhere; }
      .log .pline { animation: bkLineIn var(--d-slow) var(--ease-spring) both; }

      /* 独立 progress / tool_result 块（不在工具组内时）：同样的平铺段 + 上下留白 */
      .block-sec { margin: var(--sp-3) 0; }

      /* ── compaction：安静耳语（无框无线） ── */
      .compaction { text-align: center; color: var(--ink-3); font-size: var(--t-meta);
        font-family: var(--mono); margin: var(--sp-6) 0; opacity: .8; }

      /* ── turnEnd：旗标 + max_steps 诚实终态 ── */
      .turn-end { display: flex; align-items: center; gap: var(--sp-2); margin: var(--sp-4) 0 var(--grid);
        padding: var(--sp-3); border-radius: var(--r-card);
        border: var(--hairline) solid color-mix(in srgb, var(--warn) 40%, var(--line));
        background: color-mix(in srgb, var(--warn) 5%, transparent); }
      .turn-end .te-ico { color: var(--warn); display: grid; place-items: center; flex: none; }
      .turn-end .te-ico svg { width: var(--icon); height: var(--icon); }
      .turn-end .te-msg { flex: 1; font-size: var(--t-body); color: var(--ink-2); line-height: var(--lh-ui); }
      .turn-end .te-msg b { color: var(--ink); font-weight: 600; }
      .turn-end .te-code { font-family: var(--mono); font-size: var(--t-meta); font-weight: 600;
        padding: var(--hairline) var(--badge-pad-x); border-radius: var(--r-tag);
        color: var(--warn); background: color-mix(in srgb, var(--warn) 12%, transparent); }

      /* ── subtree：E3 subagent 嵌套（可折叠 + 展开后左 1px 导轨） ── */
      .subtree { margin: var(--sp-2) 0; }
      .sub-sum { min-width: 0; display: inline-flex; align-items: center; gap: var(--gap-tight);
        font-family: var(--mono); font-size: var(--t-meta); color: var(--ink-3); cursor: pointer; padding: var(--grid) 0; }
      .sub-sum:hover { color: var(--ink-2); }
      .subtree.open .sub-sum .chev { transform: rotate(90deg); }
      .sub-body { display: grid; grid-template-rows: 0fr; transition: grid-template-rows var(--d-slow) var(--ease-spring); }
      .subtree.open .sub-body { grid-template-rows: 1fr; }
      .sub-inner { margin-top: var(--sp-2); padding-left: var(--sp-3); border-left: var(--hairline) solid var(--line); }
      :host([nested]) .stream { gap: 0; }
    `;

    // .blocks 走 JS 属性（数组不经线缆）；设入即重渲染。
    get blocks() { return this._blocks || []; }
    set blocks(v) { this._blocks = Array.isArray(v) ? v : []; if (this.isConnected) this._render(); }

    render() {
      this._jdata = [];  // 本次渲染收集的 json-tree 载荷（DOM 序），hydrate 据序设入 .data
      const list = this._blocks || [];
      return `<div class="stream">${list.map((b, i) => this.block(b || {}, i)).join("")}</div>`;
    }

    // 注册一个 json-tree 槽：把对象按序存入 _jdata，返回带序号的 <an-json-tree>
    _jsonSlot(payload) {
      const idx = this._jdata.length;
      this._jdata.push(payload);
      return `<an-json-tree root="false" data-jtree="${idx}"></an-json-tree>`;
    }

    block(b, i) {
      switch (b.type) {
        case "text": return this.text(b);
        case "reasoning": return this.reasoning(b, i);
        case "tool_call": return this.toolCall(b, i);
        case "tool_result": return this.toolResult(b);
        case "progress": return this.progress(b);
        case "compaction": return this.compaction(b);
        case "turnEnd": return this.turnEnd(b);
        case "subtree": return this.subtree(b, i);
        default: return "";
      }
    }

    text(b) {
      const body = b.html != null ? b.html : md(b.text || b.content || "");
      // role="user" → 右对齐浅灰气泡；否则 assistant 通栏
      if (b.role === "user") return `<div class="text user"><div class="bubble">${body}</div></div>`;
      return `<div class="text">${body}</div>`;
    }

    reasoning(b, i) {
      const open = b.open ? " open" : "";
      return `<div class="reason${open}" data-i="${i}">
        <div class="reason-sum"><span class="chev">${ic("chevr", 12)}</span><span class="lico">${ic("reasoning", 12)}</span>${e(b.label || "推理")}</div>
        <div class="reason-body"><div class="w"><div class="reason-text">${e(b.text || b.content || "")}</div></div></div>
      </div>`;
    }

    toolCall(b, i) {
      const running = !!b.running;
      const open = b.open && !running ? " open" : "";
      const items = Array.isArray(b.items) ? b.items : [];
      // settled 摘要：作者 summary or 由项动词人性化自动生成（绝不裸 snake_case）
      const auto = items.length
        ? items.map((it) => human(it.verb || it.name || "")).filter(Boolean).join(" · ")
        : "工具调用";
      const sumTxt = running ? (b.status || "正在调用工具…") : (b.summary || auto);
      const sumCls = running ? "shimmer" : "";
      const rows = items.map((it, j) => this.toolItem(it || {}, i, j)).join("");
      return `<div class="tg${running ? " run" : ""}${open}" data-i="${i}">
        <div class="tg-sum"><span class="chev">${ic("chevr", 12)}</span><span class="lico">${ic("tool", 12)}</span><span class="${sumCls}">${e(sumTxt)}</span></div>
        <div class="tg-list"><div class="w"><div class="tg-box">${rows}</div></div></div>
      </div>`;
    }

    // 工具项 = 项头 + 平铺三段（输入 / 日志 / 结果）。输入/日志/结果共用同一段语汇、左缘对齐。
    toolItem(it, i, j) {
      const danger = it.danger;
      const showBadge = danger && danger !== "safe";
      const tone = DANGER_TONE[danger] || "danger";
      const badge = showBadge ? `<an-badge class="danger-badge" tone="${tone}">${e(danger)}</an-badge>` : "";
      const tico = window.toolIcon ? window.toolIcon(it.verb || it.name || "") : "tool";
      const verbText = it.verb ? human(it.verb) : "调用";
      const nm = it.name ? `<span class="nm">${e(it.name)}</span>` : "";

      const secs = [];
      // 输入：danger 行挂 chat 审批门（交互确认即输入面）；否则 args 结构化（JSON → json-tree，与输出同构）
      if (it.gate) {
        secs.push(`<an-approval-gate class="gate" flavor="chat" tool="${e(it.name || it.verb || "")}" danger="${e(danger || "dangerous")}"`
          + (it.summary ? ` summary="${e(it.summary)}"` : "")
          + (it.args != null ? ` args="${e(it.args)}"` : "") + `></an-approval-gate>`);
      } else if (it.args != null) {
        secs.push(`<div class="sec"><div class="sec-label">输入</div>${this._jsonView(it.args)}</div>`);
      }
      // 日志：progress 行平打印（running 显实时脉冲）
      if (it.progress) {
        const live = it.progress.done ? "" : `<span class="lt"><an-status-dot state="run"></an-status-dot>实时</span>`;
        secs.push(`<div class="sec"><div class="sec-label">日志${live}</div><div class="log">${this._logLines(it.progress)}</div></div>`);
      }
      // 输出：result 结构化（JSON → json-tree，与输入同构）
      if (it.result) {
        secs.push(`<div class="sec"><div class="sec-label">输出</div>${this._jsonView(this._payload(it.result))}</div>`);
      }
      return `<div class="ti" data-i="${i}" data-j="${j}">
        <div class="ti-head"><span class="lico">${ic(tico, 12)}</span><span class="v">${e(verbText)}</span>${nm}${badge}</div>
        ${secs.join("")}
      </div>`;
    }

    // ── 段内容共享（工具项内嵌 + 独立块同源，杜绝两套设计） ──
    _logLines(b) {
      const lines = Array.isArray(b.lines) ? b.lines : (b.text != null ? String(b.text).split("\n") : []);
      return lines.map((l) => `<div class="pline">${e(l)}</div>`).join("");
    }
    // 取块结构化载荷：对象 .data / JSON 串 .json / 文本 .text
    _payload(b) {
      if (b == null) return "";
      if (b.data !== undefined) return b.data;
      if (b.json != null) return b.json;
      return b.text != null ? b.text : "";
    }
    // 结构化视图（输入 / 输出同一函数）：对象或 JSON 串 → json-tree（hydrate 据 _jdata 序设入 .data）；非 JSON 退化 mono 打印
    _jsonView(raw) {
      let payload, isJson = false;
      if (raw !== null && typeof raw === "object") { payload = raw; isJson = true; }
      else if (typeof raw === "string") { try { payload = JSON.parse(raw); isJson = true; } catch (_) {} }
      if (isJson) return this._jsonSlot(payload);
      return `<div class="log">${e(raw == null ? "" : raw)}</div>`;
    }

    // 独立 tool_result 块（不在工具组内）：平铺段
    toolResult(b) {
      return `<div class="block-sec sec"><div class="sec-label">${e(b.label || "输出")}</div>${this._jsonView(this._payload(b))}</div>`;
    }

    // 独立 progress 块：平铺段（running 显实时脉冲）
    progress(b) {
      const live = b.done ? "" : `<span class="lt"><an-status-dot state="run"></an-status-dot>实时</span>`;
      return `<div class="block-sec sec"><div class="sec-label">${e(b.label || "日志 · stderr")}${live}</div><div class="log">${this._logLines(b)}</div></div>`;
    }

    compaction(b) {
      return `<div class="compaction">${e(b.text || "· 历史上下文已压缩 · earlier context summarized ·")}</div>`;
    }

    turnEnd(b) {
      const code = e(b.code || "MAX_STEPS_REACHED");
      const msg = b.msg || "已达到单回合步数上限，正常终止（<b>非失败</b>）。";
      const safeMsg = b.text != null ? e(b.text) : msg;
      return `<div class="turn-end">
        <span class="te-ico">${ic("turnend", 16)}</span>
        <span class="te-msg">${safeMsg}</span>
        <span class="te-code">${code}</span>
        <an-button size="sm" icon="enter" data-continue>${e(b.continueLabel || "继续")}</an-button>
      </div>`;
    }

    subtree(b, i) {
      // 可折叠，默认收起（b.open 才展开）；展开见嵌套 <an-block-tree>
      const open = b.open ? " open" : "";
      return `<div class="subtree${open}" data-i="${i}">
        <div class="sub-sum"><span class="chev">${ic("chevr", 12)}</span><span class="lico">${ic("subagent", 12)}</span>${e(b.label || "Subagent")}</div>
        <div class="sub-body"><div class="w"><div class="sub-inner"><an-block-tree nested data-subtree></an-block-tree></div></div></div>
      </div>`;
    }

    hydrate() {
      const list = this._blocks || [];

      // 折叠切换（reasoning / tool_call 组 / tool item / subtree）——running 组不可点
      this.$$(".reason").forEach((el) => {
        el.querySelector(".reason-sum").addEventListener("click", () => el.classList.toggle("open"));
      });
      this.$$(".tg").forEach((el) => {
        const sum = el.querySelector(".tg-sum");
        sum.addEventListener("click", () => { if (!el.classList.contains("run")) el.classList.toggle("open"); });
      });
      this.$$(".subtree").forEach((el) => {
        el.querySelector(".sub-sum").addEventListener("click", () => el.classList.toggle("open"));
      });

      // json-tree 载荷按 DOM 序设入（独立 tool_result + 项内 result 一并覆盖）
      (this._jdata || []).forEach((d, idx) => { const t = this.$(`[data-jtree="${idx}"]`); if (t) t.data = d; });

      // subtree：把子块流设入内嵌 <an-block-tree>.blocks（E3 递归，无论展开与否先就绪）
      const subEls = this.$$("[data-subtree]");
      let si = 0;
      list.forEach((b) => {
        if (!b || b.type !== "subtree") return;
        const inner = subEls[si++]; if (inner) inner.blocks = Array.isArray(b.blocks) ? b.blocks : [];
      });

      // turnEnd 续跑 → composed an-continue
      this.$$("[data-continue]").forEach((b) => {
        b.addEventListener("click", () => this.emit("an-continue", {}));
      });
    }
  }

  window.AnElement.define(AnBlockTree);
})();
