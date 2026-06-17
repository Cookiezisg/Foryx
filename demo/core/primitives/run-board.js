/* Anselm 原语 G6 — <an-run-board>。单 workflow 的运行看板：左 = 每次 run 列表（trigger 多次 → 多条 flowrun），右 = 选中 run 的逐节点甘特。
   runs（[{id,status,when,trigger,replay,selected,gantt:[…]}]）经 JS 属性注入；左列每条 run 直接复用 an-row（dot=状态点·label=run id[mono]·hint=trigger·when·meta=↻replay·emphatic 选中），不另造行模版。
   本件真正专属的只有：左列表 ↔ 右 an-node-gantt 的【同步选中一个块】+ 看板 2 列外壳——点行切右甘特 + emit an-run-pick{id}（消费方据此切运行图 + 节点调试）；内嵌 an-node-gantt 的 an-node-pick 经 composed 冒泡至消费方。 */
(function () {
  const e = window.anEsc;
  const DOT = { running: "run", completed: "done", failed: "err", parked: "wait", cancelled: "idle" };

  class AnRunBoard extends window.AnElement {
    static tag = "an-run-board";
    static observed = [];
    static css = `
      :host { display: block; }
      .board { display: grid; grid-template-columns: var(--run-list-w) 1fr; align-items: stretch;
        border: var(--hairline) solid var(--line); border-radius: var(--r-card); background: var(--island); overflow: hidden; }
      .runs { display: flex; flex-direction: column; min-width: 0; border-right: var(--hairline) solid var(--line); }
      .gpane { display: flex; flex-direction: column; min-width: 0; }
      .rhead, .ghead { flex: none; height: var(--ctl); display: flex; align-items: center; padding: 0 var(--sp-3);
        font-size: var(--t-meta); font-weight: 600; color: var(--ink-3); border-bottom: var(--hairline) solid var(--line); }
      .rlist { min-height: 0; overflow-y: auto; padding: var(--grid); }
      an-node-gantt { display: block; padding: var(--sp-3); }
    `;

    set runs(v) { this._runs = Array.isArray(v) ? v : []; this._sel = (this._runs.find((r) => r.selected) || this._runs[0] || {}).id; if (this.isConnected) this._render(); }
    get runs() { return this._runs || []; }
    set selectedId(v) { if (v != null) { this._sel = v; if (this.isConnected) this._select(v, true); } }
    get selectedId() { return this._sel; }

    render() {
      const runs = this._runs || [];
      const items = runs.map((r) => {
        const hint = [r.trigger, r.when].filter(Boolean).join(" · ");
        return `<an-row class="run" data-id="${e(r.id)}" dot="${DOT[r.status] || "idle"}" mono emphatic label="${e(r.id)}"`
          + (hint ? ` hint="${e(hint)}"` : "")
          + (r.replay ? ` meta="↻${e(String(r.replay))}"` : "")
          + (r.id === this._sel ? " selected" : "") + "></an-row>";
      }).join("");
      return `<div class="board">`
        + `<div class="runs"><div class="rhead">运行 · ${runs.length} 次</div><div class="rlist">${items}</div></div>`
        + `<div class="gpane"><div class="ghead">节点甘特 · 本次 run 内逐节点时段</div><an-node-gantt></an-node-gantt></div>`
        + `</div>`;
    }
    hydrate() {
      const g = this.$("an-node-gantt");
      const cur = (this._runs || []).find((r) => r.id === this._sel);
      if (g) g.nodes = (cur && cur.gantt) || [];
      this.$$(".run").forEach((row) => row.addEventListener("an-select", () => this._select(row.dataset.id, false)));
    }
    // 切 run：高亮 an-row + 右甘特随切；silent=true 为外部同步（不回派事件、避免环）
    _select(id, silent) {
      this._sel = id;
      this.$$(".run").forEach((row) => row.toggleAttribute("selected", row.dataset.id === id));
      const run = (this._runs || []).find((r) => r.id === id) || {};
      const g = this.$("an-node-gantt"); if (g) g.nodes = run.gantt || [];
      if (!silent) this.emit("an-run-pick", { id });
    }
  }
  window.AnElement.define(AnRunBoard);
})();
