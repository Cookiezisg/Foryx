/* Anselm 原语 🧩 — <an-composer>。chat 输入条 = 输入框原语家族的富成员（演变型）：单行 contenteditable + @ 提及内联药丸 + 附件 chip + send/stop。
   演变输入：1 行时是圆边药丸（radius=box 高/2，JS _updateRadius 据高度算），随换行增高 radius 渐变到 --r-card（阈值后恒为普通输入框圆角）；发送钮极简 icon、空输入时藏（_syncHasInput）。
   归属：contenteditable 物理刚需（@ ref-pill 是内联子元素 + radius 高度演变都要富容器），an-input(textarea·纯字符串值) 不能替代——故演变逻辑内化于本件，本件即 chat 的输入框原语。
   why 新建：现有 an-input[multiline] 只是裸 textarea，缺 @picker/附件/send 态——是 chat 海洋唯一必须新建的件。
   复用底座：AnMention（@ → 边打边滤 picker → 内联 an-ref-pill，与 doc-editor 同源）· an-button（工具钮/send/stop）· an-ref-pill（提及药丸）· icons。
   能力：① 多行自增高 contenteditable（max 6 行后内滚）；② 「@」起会话 or 工具栏 @ 钮开 picker → 内联插药丸；③ 附件 chip 行（可删）；
        ④ Enter 发送 / Shift+Enter 换行；⑤ generating 态 send↔stop 互换（纯 CSS，不重渲、不抹输入）。
   数据：.mentions=[{kind,id,label,desc}]（@picker 池）· .attachments=[{name,icon?}]（附件 chip）· generating 属性切发送/停止态。
   交互对外（composed）：an-send{text,html,refs,attachments} · an-stop · an-attach（附件钮，宿主可挂真选文件；demo 自插占位 chip）。 */
(function () {
  const e = window.anEsc;

  class AnComposer extends window.AnElement {
    static tag = "an-composer";
    static observed = [];   // generating 走 :host([generating]) CSS 实时响应、不触发重渲（保住 editable 输入不被抹）
    static css = `
      :host { display: block; }
      .bar { max-width: var(--w-content); margin: 0 auto; padding: var(--sp-2) var(--sp-6) var(--sp-4); }
      /* 输入盒：圆角面 + inset 描边环（半透 border 圆角会叠灰尖，用 inset 均匀）；聚焦叠 accent 光环 */
      /* border-radius 由 JS _updateRadius() 据高度演变（1 行=药丸 → 增高渐变到 --r-card）；transition 让换行平滑 */
      .box {
        background: var(--island);
        box-shadow: inset 0 0 0 var(--hairline) var(--line);
        transition: box-shadow var(--d-fast), border-radius var(--d-slow) var(--ease-spring);
      }
      .box:focus-within { box-shadow: inset 0 0 0 var(--hairline) var(--accent-line), 0 0 0 var(--focus-ring) var(--accent-soft); }
      /* pill attr 降级为「浮起」修饰（landing 居中态用）——只加阴影，radius 全权交演变、不再强制圆 */
      :host([pill]) .box { box-shadow: inset 0 0 0 var(--hairline) var(--line), var(--shadow-float); }

      /* 附件 chip 行（空则整行塌陷） */
      .chips { display: flex; flex-wrap: wrap; gap: var(--gap-tight); padding: var(--sp-2) var(--sp-3) 0; }
      .chips:empty { display: none; }
      .chip {
        display: inline-flex; align-items: center; gap: var(--gap-tight); height: var(--badge-h);
        padding: 0 var(--grid) 0 var(--badge-pad-x); border-radius: var(--r-tag);
        background: var(--island-3); color: var(--ink-2); font-size: var(--t-meta);
      }
      .chip .ci { display: grid; place-items: center; color: var(--ink-3); }
      .chip .ci svg { width: var(--icon-sm); height: var(--icon-sm); }
      .chip .x { display: grid; place-items: center; width: var(--icon); height: var(--icon); border-radius: var(--r-tag); color: var(--ink-3); cursor: pointer; }
      .chip .x:hover { background: var(--island-4); color: var(--ink); }
      .chip .x svg { width: var(--icon-sm); height: var(--icon-sm); }

      /* 单行：[lead][edit][tail] 一行、垂直居中。多行：edit 占整行在上、钮组下移成一行（左 lead · 右 tail）——ChatGPT 式 reflow，靠 grid-template-areas 切换（multiline attr 由高度判定）。 */
      .row { display: grid; grid-template-columns: auto minmax(var(--zero), 1fr) auto; grid-template-areas: "lead edit tail";
        align-items: center; column-gap: var(--grid); row-gap: var(--sp-2); padding: var(--sp-2); }
      :host([multiline]) .row { grid-template-areas: "edit edit edit" "lead . tail"; }
      /* 多行：文字左缘对齐下方 @ 图标左缘（图标在 icon 钮内居中、内缩 (ctl-icon)/2，故文字也内缩同值，非裸值） */
      :host([multiline]) .edit { padding-left: calc((var(--ctl) - var(--icon)) / 2); padding-right: calc((var(--ctl) - var(--icon)) / 2); }
      .row .lead { grid-area: lead; } .row .tail { grid-area: tail; justify-self: end; }
      .row .lead, .row .tail { display: inline-flex; align-items: center; gap: var(--grid); }
      /* 模型切换钮：模型名 + 尾 chevron（ghost 文字钮，与 @/附件 同 lead 行） */
      .t-model .m-name { vertical-align: middle; }
      .t-model .m-chev { display: inline-flex; vertical-align: middle; margin-left: var(--gap-tight); color: var(--ink-3); }
      .t-model .m-chev svg { display: block; }
      /* contenteditable 编辑区：flex 独吞中段、多行自增、超 6 行内滚（无 native gutter）；空态占位 */
      .edit {
        grid-area: edit; min-width: var(--zero); outline: none; padding: var(--zero);
        font-size: var(--t-body); line-height: var(--lh-ui); color: var(--ink);
        min-height: calc(var(--t-body) * var(--lh-ui)); max-height: calc(var(--row) * 6);
        overflow-y: auto; overflow-wrap: anywhere; scrollbar-width: none; -ms-overflow-style: none;
      }
      .edit::-webkit-scrollbar { width: var(--zero); }
      .edit:empty::before { content: attr(data-ph); color: var(--ink-3); pointer-events: none; }
      an-ref-pill { margin: 0 var(--grid); vertical-align: baseline; }

      /* 发送钮：空输入且非 generating 时藏（单行只剩左两钮）；有输入才现 */
      :host(:not([has-input]):not([generating])) .t-send { display: none; }
      /* generating：send↔stop 互换（纯 CSS） */
      :host(:not([generating])) .t-stop { display: none; }
      :host([generating]) .t-send { display: none; }
    `;

    set mentions(v) { this._mentions = Array.isArray(v) ? v : []; }
    get mentions() { return this._mentions || []; }
    set attachments(v) { this._atts = Array.isArray(v) ? v : []; if (this.isConnected) this._renderChips(); }
    get attachments() { return this._atts || []; }
    // 焦点入编辑区（feature 切会话后聚焦）
    focus() { const ed = this.$(".edit"); if (ed) ed.focus(); }
    // 清空输入（feature 发送后 / 切会话）：回单行药丸 + 藏发送钮
    clear() { const ed = this.$(".edit"); if (ed) ed.innerHTML = ""; this._atts = []; this._renderChips(); this.removeAttribute("has-input"); this._updateRadius(); }

    // has-input：有文字 or @ 内联药丸即「有输入」→ 切宿主属性（纯 CSS 显隐发送钮，不重渲、不抹 editable）
    _syncHasInput() {
      const ed = this.$(".edit"); if (!ed) return;
      this.toggleAttribute("has-input", !!(ed.textContent.trim() || ed.querySelector("an-ref-pill")));
    }
    // 演变 radius：据 .box 实测高度 lerp（1 行=minH/2 全圆药丸 → 增高渐变到 --r-card → 阈值后恒 r-card）。JS 计算 px 写 inline（lint 仅管 CSS 源）。
    _updateRadius() {
      const box = this.$(".box"); if (!box) return;
      const cs = getComputedStyle(this);
      const rCard = parseFloat(cs.getPropertyValue("--r-card")) || 16;
      const h = box.offsetHeight; if (!h) return;
      const minH = this._minH || (this._minH = h);            // connect 时单行 box 高 = 药丸基线
      this.toggleAttribute("multiline", h - minH > 6);         // 换行后 = 多行（贴底对齐）；单行 = 居中
      const targetH = minH + (parseFloat(cs.getPropertyValue("--row")) || 32) * 2;   // +2 行即达 box 阈值
      const t = Math.min(1, Math.max(0, (h - minH) / (targetH - minH)));
      box.style.borderRadius = ((minH / 2) + (rCard - minH / 2) * t) + "px";
    }

    // 模型/API 切换：数据属性注入优先（feature 设 composer.models），兜底 window.CHAT_MODELS——composer 对数据无硬依赖、可复用。
    set models(v) { this._models = v; if (this.isConnected) this._syncModelBtn(); }
    get models() { return this._models || window.CHAT_MODELS || { providers: [], current: {} }; }
    _syncModelBtn() {
      const span = this.$(".t-model .m-name"); if (!span) return;
      const data = this.models, cur = data.current || {};
      let label = "选择模型";
      (data.providers || []).forEach((p) => (p.models || []).forEach((m) => { if (m.id === cur.model) label = m.label; }));
      span.textContent = label;
    }
    _openModelMenu(anchor) {
      window.AnModelPicker.open(anchor, {
        models: this.models, namespace: "composer-model", placement: "top",
        onPick: (modelId, keyId, info) => this._pickModel(modelId, keyId, info),
      });
    }
    _pickModel(modelId, keyId, info) {
      this.models.current = { model: modelId, key: keyId };   // 单源选中态就地更新（钮文案 + 派事件供 feature 落地副作用）
      this._syncModelBtn();
      this.emit("an-model-change", { model: modelId, key: keyId, modelLabel: info && info.modelLabel, keyLabel: info && info.keyLabel });
    }

    render() {
      // 单行：左 @/附件 + edit + 右 发送/停止（发送极简 icon 钮、空输入时藏）。无占位文字（Enter 发送 / Shift+Enter 换行靠键位约定）。
      const ph = e(this.attr("placeholder", ""));
      return `<div class="bar"><div class="box">
        <div class="chips"></div>
        <div class="row">
          <span class="lead">
            <an-button class="t-at" variant="icon" icon="at-sign">提及</an-button>
            <an-button class="t-att" variant="icon" icon="paperclip">附件</an-button>
            <an-button class="t-model" variant="ghost" size="sm" aria-haspopup="menu"><span class="m-name"></span><span class="m-chev">${window.icon("chevd", 12)}</span></an-button>
          </span>
          <div class="edit" contenteditable="true" spellcheck="false" data-ph="${ph}"></div>
          <span class="tail">
            <an-button class="t-send" variant="icon" size="sm" icon="arrow-up" aria-label="发送"></an-button>
            <an-button class="t-stop" variant="danger" size="sm" icon="stop" aria-label="停止"></an-button>
          </span>
        </div>
      </div></div>`;
    }

    hydrate() {
      const ed = this.$(".edit");
      this._renderChips();
      // 布局后首算：锁单行 box 高（radius 演变基线 minH）+ 同步发送钮显隐
      requestAnimationFrame(() => { this._updateRadius(); this._syncHasInput(); });
      // 点 box 空白处（非钮/chip/药丸/edit 本体）→ 聚焦编辑区（edit 居中后比 box 矮，留白处也要能聚焦）
      const box = this.$(".box");
      if (box) box.addEventListener("mousedown", (ev) => { if (ev.target.closest("an-button, an-ref-pill, .chip, .edit")) return; ev.preventDefault(); ed.focus(); });

      // 模型/API 切换钮：填当前模型名 + 点开两栏选择器（AnModelPicker）
      this._syncModelBtn();
      const mb = this.$(".t-model");
      if (mb) mb.addEventListener("click", () => this._openModelMenu(mb));

      // @ 提及（复用地基 AnMention：「@」起会话 + 工具栏钮 pick；shadow 内取 shadowRoot 选区）
      this._mention = window.AnMention.attach(ed, {
        mentions: () => this._mentions || [],
        namespace: "composer-at",
        getSelection: () => (this.shadowRoot.getSelection ? this.shadowRoot.getSelection() : window.getSelection()),
      });
      this.$(".t-at").addEventListener("click", () => this._mention.pick(ed));

      // 附件：派 an-attach 供宿主挂真选文件；demo 自插占位 chip（无宿主也自演）
      this.$(".t-att").addEventListener("click", () => {
        this.emit("an-attach", {});
        this._addChip({ name: ["spec.md", "screenshot.png", "data.csv"][(this._atts || []).length % 3], icon: "doc" });
      });

      // Enter 发送 / Shift+Enter 换行；空态保持 :empty（清掉残留 <br>）
      ed.addEventListener("keydown", (ev) => {
        if (ev.key === "Enter" && !ev.shiftKey) { ev.preventDefault(); this._send(); }
      });
      ed.addEventListener("input", () => { if (!ed.textContent.trim() && !ed.querySelector("an-ref-pill")) ed.innerHTML = ""; this._syncHasInput(); this._updateRadius(); });

      this.$(".t-send").addEventListener("click", () => this._send());
      this.$(".t-stop").addEventListener("click", () => this.emit("an-stop", {}));
    }

    // 附件 chip 行（命令式重绘，不动 editable）
    _addChip(att) { this._atts = (this._atts || []).concat([att]); this._renderChips(); }
    _renderChips() {
      const box = this.$(".chips"); if (!box) return;
      box.innerHTML = (this._atts || []).map((a, i) =>
        `<span class="chip"><span class="ci">${window.icon(a.icon || "doc", 12)}</span>${e(a.name || "附件")}<span class="x" data-x="${i}">${window.icon("close", 12)}</span></span>`
      ).join("");
      box.querySelectorAll(".x").forEach((x) => x.addEventListener("click", () => {
        this._atts.splice(Number(x.dataset.x), 1); this._renderChips();
      }));
    }

    // 提取纯文本（ref-pill → @label）；收集引用 + 附件，派 an-send，清空
    _plainText(ed) {
      let out = "";
      ed.childNodes.forEach((n) => {
        if (n.nodeType === 3) out += n.textContent;
        else if (n.nodeName && n.nodeName.toLowerCase() === "an-ref-pill") out += "@" + (n.getAttribute("label") || "");
        else if (n.tagName === "BR") out += "\n";
        else out += n.textContent || "";
      });
      return out;
    }
    _send() {
      if (this.has("generating")) return;
      const ed = this.$(".edit");
      const text = this._plainText(ed);
      const atts = (this._atts || []).slice();
      if (!text.trim() && !atts.length) return;
      const refs = this.$$("an-ref-pill").map((p) => ({ kind: p.getAttribute("kind"), id: p.getAttribute("id"), label: p.getAttribute("label") }));
      this.emit("an-send", { text: text, html: ed.innerHTML, refs: refs, attachments: atts });
      ed.innerHTML = ""; this._atts = []; this._renderChips(); this.removeAttribute("has-input"); this._updateRadius();
    }
  }
  window.AnElement.define(AnComposer);
})();
