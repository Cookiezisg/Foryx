/* Anselm demo — <an-sidebar>：左岛内容（固定 chrome，≈ Flutter NavigationRail + Drawer）。
   职责：岛皮肤 + 红绿灯/收起/搜索 顶栏 + 导航（Notion 式：未选只图标、选中=图标+标签药丸）+ #sidebody 海洋侧栏宿主 +
        底部 工作区(头像→设置轴) + 通知(铃铛→通知轴) + 左下角 peek。不碰布局宽度（归 <an-shell>）。
   契约：set model({ws, nav:[{id,label,icon}]}) → 渲染；setRail(node) 注入海洋侧栏；setActive(kind) 高亮（kind=海洋 id | 'avatar' | 'bell'）；setUnread(b)；peek(text)。
   事件：an-toggle-left（收起/展开）· an-nav{id}（切海洋）· an-axis{which:'avatar'|'bell'}（切轴）。 */
(function () {
  class AnSidebar extends window.AnElement {
    static tag = "an-sidebar";
    static css = `
      :host { position: relative; display: flex; flex-direction: column; height: 100%; padding: var(--sp-3); overflow: hidden;
        background: var(--island); border: var(--hairline) solid var(--line); border-radius: var(--r-chip); box-shadow: var(--shadow-float); }

      .top { display: flex; align-items: center; gap: var(--sp-1); height: var(--row); margin-bottom: var(--sp-2); }
      .lights { display: flex; gap: var(--sp-2); padding-left: calc(var(--grid) / 2); }
      .lights i { width: var(--icon-sm); height: var(--icon-sm); border-radius: var(--r-pill); }
      .lights .r { background: var(--tl-close); } .lights .y { background: var(--tl-min); } .lights .g { background: var(--tl-max); }
      .grow { flex: 1; }

      /* 导航：未选只图标、选中=图标+标签灰药丸（图标恒显，只截断选中标签） */
      .nav { display: flex; gap: calc(var(--grid) / 2); margin-bottom: var(--sp-3); min-width: 0; }
      .navbtn { flex: none; display: flex; align-items: center; gap: 0; height: var(--row); padding: 0 var(--gap-tight); border-radius: var(--r-btn);
        color: var(--ink-2); font-size: var(--t-body); font-weight: 500; background: transparent;
        transition: background var(--d-fast), color var(--d-fast), gap var(--d-slow) var(--ease-spring), padding var(--d-slow) var(--ease-spring); }
      .navbtn:hover { background: var(--island-3); color: var(--ink); }
      .navbtn .ico { flex: none; display: grid; place-items: center; }
      .navbtn .lbl { max-width: var(--zero); opacity: 0; overflow: hidden; white-space: nowrap; text-overflow: ellipsis;
        transition: max-width var(--d-slow) var(--ease-spring), opacity var(--d-mid); }
      .navbtn.on { gap: var(--gap); padding: 0 var(--btn-pad-x-sm); background: var(--island-4); color: var(--ink); }
      .navbtn.on .lbl { max-width: var(--w-content); opacity: 1; }

      #sidebody { flex: 1; min-height: 0; display: flex; flex-direction: column; overflow-y: auto; scrollbar-width: none; }
      #sidebody::-webkit-scrollbar { width: var(--zero); height: var(--zero); }
      .soon { padding: var(--sp-6) var(--sp-3); color: var(--ink-3); font-size: var(--t-meta); }

      .foot { display: flex; align-items: center; gap: calc(var(--grid) / 2); margin-top: var(--gap-tight); }
      .ws { flex: 1; min-width: 0; display: flex; align-items: center; gap: var(--btn-pad-x-sm); height: var(--island-head); padding: 0 var(--gap-tight);
        border-radius: var(--r-btn); color: var(--ink); transition: background var(--d-fast); }
      .ws:hover { background: var(--island-3); }
      .ws.on { background: var(--island-4); }
      .ava { flex: none; width: var(--ctl); height: var(--ctl); border-radius: var(--r-pill); display: grid; place-items: center;
        background: var(--island-4); color: var(--ink-2); font-size: var(--t-meta); font-weight: 600; }
      .wsname { flex: 1; min-width: 0; text-align: left; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: var(--t-body); font-weight: 600; }
      .bell { flex: none; position: relative; width: var(--ctl-sm); height: var(--ctl-sm); display: grid; place-items: center;
        color: var(--ink-3); border-radius: var(--r-btn); transition: color var(--d-fast), background var(--d-fast); }
      .bell:hover { color: var(--ink); background: var(--island-3); }
      .bell.on { background: var(--island-4); color: var(--ink); }
      .bell .dot { position: absolute; top: var(--grid); right: var(--grid); width: var(--dot); height: var(--dot); border-radius: var(--r-pill);
        background: var(--accent); border: var(--line-2) solid var(--island); }

      .peek { position: absolute; left: var(--sp-2); right: var(--sp-2); bottom: calc(var(--island-head) + var(--sp-3)); z-index: 6;
        display: flex; align-items: center; gap: var(--sp-2); height: var(--tab-h); padding: 0 var(--sp-2) 0 var(--sp-3); border-radius: var(--r-chip);
        background: var(--island); border: var(--hairline) solid var(--line); box-shadow: var(--shadow-pop);
        opacity: 0; transform: translateY(var(--sp-1)); transition: opacity var(--d-mid), transform var(--d-mid) var(--ease-spring); }
      .peek.in { opacity: 1; transform: none; }
      .pd { flex: none; width: var(--dot); height: var(--dot); border-radius: var(--r-pill); background: var(--warn); animation: peekpulse var(--d-breath) ease infinite; }
      .pt { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: var(--t-meta); color: var(--ink); cursor: pointer; }
      .pgo { flex: none; font-size: var(--t-meta); font-weight: 500; color: var(--ink-2); padding: 0 var(--grid); cursor: pointer; }
      .pgo:hover { color: var(--ink); }
      .px { flex: none; width: var(--trail); height: var(--trail); display: grid; place-items: center; color: var(--ink-3); border-radius: var(--r-tag); }
      .px:hover { color: var(--ink); background: var(--island-3); }
      @keyframes peekpulse { 50% { opacity: .3; } }
    `;

    render() {
      const m = this._model || { ws: "Personal", nav: [] };
      const ava = (m.ws || "?").trim().split(/\s+/).slice(0, 2).map((w) => w[0]).join("").toUpperCase();
      const nav = (m.nav || []).map((n) =>
        `<button class="navbtn" data-id="${window.anEsc(n.id)}"><span class="ico">${window.icon(n.icon, 18)}</span><span class="lbl">${window.anEsc(n.label)}</span></button>`
      ).join("");
      return `
        <div class="top">
          <div class="lights"><i class="r"></i><i class="y"></i><i class="g"></i></div>
          <span class="grow"></span>
          <an-button class="collapse" variant="icon" icon="panel-left" title="收起侧栏"></an-button>
          <an-button class="search" variant="icon" icon="search" title="搜索"></an-button>
        </div>
        <div class="nav">${nav}</div>
        <div id="sidebody"></div>
        <div class="foot">
          <button class="ws" title="工作区 / 设置"><span class="ava">${window.anEsc(ava)}</span><span class="wsname">${window.anEsc(m.ws || "")}</span></button>
          <button class="bell" title="通知">${window.icon("bell", 18)}<span class="dot" hidden></span></button>
        </div>`;
    }

    hydrate() {
      this.$(".collapse").addEventListener("click", () => this.emit("an-toggle-left"));
      this.$$(".navbtn").forEach((b) => b.addEventListener("click", () => this.emit("an-nav", { id: b.dataset.id })));
      this.$(".ws").addEventListener("click", () => this.emit("an-axis", { which: "avatar" }));
      this.$(".bell").addEventListener("click", () => this.emit("an-axis", { which: "bell" }));
    }

    set model(v) { this._model = v; if (this.isConnected) this._render(); }
    get model() { return this._model; }

    setRail(node) {
      const host = this.$("#sidebody"); if (!host) return;
      host.replaceChildren(node || Object.assign(document.createElement("div"), { className: "soon", textContent: "侧栏设计中…" }));
    }
    setActive(kind) {
      this.$$(".navbtn").forEach((b) => b.classList.toggle("on", b.dataset.id === kind));
      this.$(".ws").classList.toggle("on", kind === "avatar");
      this.$(".bell").classList.toggle("on", kind === "bell");
    }
    setUnread(has) { const d = this.$(".bell .dot"); if (d) d.hidden = !has; }

    peek(text) {
      this._peekDismiss();
      this.setUnread(true);
      const p = document.createElement("div"); p.className = "peek";
      p.innerHTML = `<span class="pd"></span><span class="pt">${window.anEsc(text)}</span><button class="pgo">查看</button><button class="px" title="忽略">${window.icon("close", 13)}</button>`;
      this.shadowRoot.appendChild(p);
      requestAnimationFrame(() => p.classList.add("in"));
      const go = () => { this._peekDismiss(); this.emit("an-axis", { which: "bell" }); };
      p.querySelector(".pgo").addEventListener("click", go);
      p.querySelector(".pt").addEventListener("click", go);
      p.querySelector(".px").addEventListener("click", (e) => { e.stopPropagation(); this._peekDismiss(); });
      this._peek = p;
    }
    _peekDismiss() { const p = this._peek; if (p) { p.classList.remove("in"); setTimeout(() => p.remove(), 220); this._peek = null; } }
  }
  window.AnElement.define(AnSidebar);
})();
