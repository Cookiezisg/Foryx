/* Anselm 原语 G2 — <an-run-terminal verb vico args lang gate>。执行型实体（function/handler/agent/workflow）的试运行。
   形态 = 一个普通代码块：顶栏左上角是 run 图标钮（点它执行）+ 右上角语言标签；下方可编辑 args（复用 <an-code-editor editable inline>，不重造编辑器）。
   运行后自上而下追加：stdout 逐行 → result 终态摘要（status-dot + 状态词 + out + ms）→ 结构化输出（result 带 json 时挂 <an-json-tree>）。
   gate 闸态：env 未 ready / config 不全 → 只渲染一行盾「不可运行」note，不挂运行链路。
   交互：点 run 钮 emit composed 'an-run'{args}（装配层接后端真执行）；自带 mock：data-trace（JSON {lines,result}）驱动逐行吐字 + 收尾摘要。
   属性：verb（钮文案，默认 运行）| vico（钮图标，默认 play）| args（args 种子串，默认 {}）| lang（args 语言，默认 json）| gate（闸说明文案）。 */
(function () {
  const e = window.anEsc;
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

  class AnRunTerminal extends window.AnElement {
    static tag = "an-run-terminal";
    static observed = ["verb", "vico", "args", "lang", "gate"];
    static css = `
      :host { display: block; min-height: fit-content; }
      /* 代码块外壳：单条细描边圆角块（同 code-editor 语汇）；min-height:fit-content 贴内容、不被外层压短导致末行裁切。
         描边走 inset box-shadow 而非 border——圆角 + 半透明 border 在四角有 1px 叠加变深的"灰尖"（Retina 更明显），inset 环均匀无叠加。 */
      .term { min-height: fit-content; box-shadow: inset 0 0 0 var(--hairline) var(--line); border-radius: var(--r-card); background: var(--island); overflow: hidden; }

      /* 顶栏：左上角 run 钮（图标 + 文案） + 右侧语言标签 */
      .bar { display: flex; align-items: center; gap: var(--gap); padding: var(--sp-2) var(--sp-3) 0; }
      .run {
        display: inline-flex; align-items: center; gap: var(--gap-tight);
        height: var(--ctl-sm); padding: 0 var(--btn-pad-x-sm); border: 0; border-radius: var(--r-tag);
        background: none; color: var(--accent); font-size: var(--t-meta); font-weight: 600; cursor: pointer;
        transition: background var(--d-fast);
      }
      .run:hover { background: var(--accent-soft); }
      .run[disabled] { color: var(--ink-3); pointer-events: none; }
      .run .ico { display: grid; place-items: center; }
      .run .ico svg { width: var(--icon-sm); height: var(--icon-sm); }
      .run .ico.spin svg { animation: anRunSpin calc(var(--d-slow) * 3) linear infinite; }
      @keyframes anRunSpin { to { transform: rotate(360deg); } }
      .grow { flex: 1; }
      .lang { flex: none; white-space: nowrap; color: var(--ink-3); font-size: var(--t-meta); line-height: var(--lh-ui); }

      /* args 代码区：复用 code-editor（inline = 无自带框/栏/行号，纯可编辑高亮码） */
      .args { padding: var(--sp-2) var(--sp-3) var(--sp-3); }
      .args an-code-editor { display: block; }

      /* stdout：默认收起、运行时展开逐行 */
      .out {
        display: none; padding: var(--sp-3) var(--sp-4); border-top: var(--hairline) solid var(--line);
        font-family: var(--mono); font-size: var(--t-meta); line-height: var(--lh-prose); color: var(--ink-2); white-space: pre-wrap;
      }
      .out.show { display: block; }

      /* result 摘要：块流——首行 .res-head [状态点+词 | 毫秒]，次行 .ev 输出独占整行（margin-top 间隔）。
         min-height:fit-content 兜底——嵌套 shadow 里 .res 的内在高度被解析短一截、.ev 末行被 .term overflow:hidden 裁掉，强制贴内容。空输出不占行。 */
      .res {
        display: none; min-height: fit-content;
        padding: var(--sp-3) var(--sp-4); border-top: var(--hairline) solid var(--line); font-size: var(--t-meta);
      }
      .res.show { display: block; }
      .res-head { display: flex; align-items: baseline; gap: var(--gap); min-width: 0; }
      .st { display: inline-flex; align-items: center; gap: var(--gap-tight); font-weight: 500; color: var(--ink-2); min-width: 0; }
      .res.done .st { color: var(--ok); }
      .res.err .st { color: var(--danger); }
      .ms { margin-left: auto; flex: none; font-family: var(--mono); font-size: var(--t-meta); color: var(--ink-3); font-variant-numeric: tabular-nums; }
      .ev { min-width: 0; margin-top: var(--grid); font-family: var(--mono); color: var(--ink); white-space: pre-wrap; overflow-wrap: anywhere; }
      .ev:empty { display: none; margin-top: 0; }

      /* 结构化输出：result 带 json 时挂 json-tree（独立段，顶边线分隔） */
      .json { display: none; padding: var(--sp-2) var(--sp-3); border-top: var(--hairline) solid var(--line); }
      .json.show { display: block; }

      /* 闸态：env/config 未就绪，只一行盾说明（无运行链路） */
      .gate { display: flex; align-items: center; gap: var(--gap); padding: var(--sp-3) var(--sp-4); font-size: var(--t-meta); color: var(--ink-3); }
      .gateico { flex: none; display: grid; place-items: center; color: var(--warn); }
      .gateico svg { width: var(--icon); height: var(--icon); }
    `;

    render() {
      // 闸态：环境/配置未就绪——只渲染一行盾说明，不挂运行链路
      if (this.attr("gate")) {
        return `<div class="term"><div class="gate">`
          + `<span class="gateico">${window.icon("shield")}</span>${e(this.attr("gate"))}</div></div>`;
      }
      const verb = this.attr("verb", "运行");
      const vico = this.attr("vico", "play");
      const lang = this.attr("lang", "json");
      const args = this.attr("args", "{}");
      return `<div class="term">`
        + `<div class="bar">`
        +   `<button type="button" class="run" data-go><span class="ico">${window.icon(vico)}</span>${e(verb)}</button>`
        +   `<span class="grow"></span><span class="lang">${e(lang)}</span>`
        + `</div>`
        + `<div class="args"><an-code-editor editable inline wrap lang="${e(lang)}">${e(args)}</an-code-editor></div>`
        + `<div class="out"></div>`
        + `<div class="res"><div class="res-head"><span class="st"><an-status-dot></an-status-dot><span data-stt></span></span><span class="ms" data-ms></span></div>`
        +   `<span class="ev" data-ev></span></div>`
        + `<an-json-tree class="json" root="false"></an-json-tree>`
        + `</div>`;
    }

    hydrate() {
      const btn = this.$("[data-go]");
      if (!btn) return;  // 闸态无运行钮
      btn.addEventListener("click", () => this.run());
    }

    // 当前 args 串（取自内嵌 code-editor，未就绪回退种子属性）。
    get args() {
      const ed = this.$("an-code-editor");
      return ed ? ed.value : this.attr("args", "{}");
    }

    // 运行：emit 真意图给装配层 + 跑内置 mock 动效（逐行吐 stdout → 终态摘要 → 可选结构化输出）。
    async run() {
      const args = this.args;
      this.emit("an-run", { args });

      const btn = this.$("[data-go]"), ico = this.$(".run .ico");
      const out = this.$(".out"), res = this.$(".res"), tree = this.$(".json");
      const vico = this.attr("vico", "play");
      let trace = {};
      try { trace = JSON.parse(this.attr("data-trace") || "{}"); } catch (_) {}

      // run 态：钮锁 + 图标转 spinner、清旧态、stdout 展开逐行吐字
      btn.setAttribute("disabled", "");
      ico.classList.add("spin"); ico.innerHTML = window.icon("spin");
      res.classList.remove("show", "done", "err"); tree.classList.remove("show");
      out.classList.add("show"); out.textContent = "";

      const lines = trace.lines || ["→ spawn sandbox", "→ exec", "stdout: ok"];
      for (const l of lines) { await sleep(240); out.textContent += l + "\n"; }
      await sleep(240);

      // done 收尾：松钮、图标复位、出终态摘要（StatusDot 经 anState 折正）
      ico.classList.remove("spin"); ico.innerHTML = window.icon(vico); btn.removeAttribute("disabled");
      const r = trace.result || { st: "ok", out: "done", ms: 100 };
      const dot = window.anState(r.st);  // ok→done / error→err …（单一翻译路径）
      this.$("an-status-dot").setAttribute("state", dot);
      this.$("[data-stt]").textContent = r.st;
      this.$("[data-ev]").textContent = r.out == null ? "" : r.out;
      this.$("[data-ms]").textContent = (r.ms == null ? "" : r.ms) + "ms";
      res.className = "res show " + dot;

      // 结构化输出：result 带 json 时挂 json-tree（结构化展示，不裸串）
      if (r.json !== undefined) { tree.data = r.json; tree.classList.add("show"); }
    }
  }

  window.AnElement.define(AnRunTerminal);
})();
