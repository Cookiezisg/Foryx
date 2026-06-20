/* Anselm feature — settings 海洋（sea）：左 rail 选类目 → Intent.on('settingsCat') → 渲对应设置页。
   六类：通用 / 模型与 Key / MCP 与市场 / 技能 / 运行时与索引 / 高级。各页 = ocean-header + an-section + 现有原语拼（无右岛）。
   纯展示 mock：字段/按钮接 toast，真后端时换端点。机器级页（运行时/高级）页头标「应用级」徽。 */
window.FEATURE = window.FEATURE || {};
window.FEATURE.settings = Object.assign(window.FEATURE.settings || {}, {
  sea: (ctx) => {
    const el = window.el;
    const S = window.SETTINGS || {};
    const toast = (t) => window.AnToast && window.AnToast.show({ text: t });
    const page = el("an-page");

    // ── 共用小工具 ──
    const head = (title, metaBadge) => {
      const h = el("an-ocean-header", { crumb: "设置", title: title });
      if (metaBadge) h.append(el("an-badge", { slot: "meta", tone: "neutral" }, metaBadge));
      return h;
    };
    const field = (label, value, opts) => el("an-field", Object.assign({ label: label, value: value, editable: "" }, opts || {}));
    const dotOf = (s) => ({ ok: "done", ready: "run", degraded: "wait", failed: "err", error: "err" }[s] || "idle");

    // ── ① 通用 ──
    function general() {
      const ws = S.workspace || {};
      const wsSec = el("an-section", { label: "工作区" });
      wsSec.append(
        field("名称", ws.name),
        field("头像色", ws.color),
        field("语言", ws.language, { hint: "同时决定 AI 回复语言" }),
      );
      const prefSec = el("an-section", { label: "偏好 · 仅本机" });
      prefSec.append(segRow("主题", ["明亮", "暗色"], "明亮"), segRow("紧凑度", ["标准", "紧凑"], "紧凑"));
      const dataSec = el("an-section", { label: "数据" });
      const locRow = el("an-row", { icon: "box", label: "数据位置", hint: "~/.anselm" });
      locRow.append(actBtn("打开目录", "enter", () => toast("已打开数据目录")));
      const delRow = el("an-row", { icon: "trash", label: "删除此工作区", hint: "不可撤销 · 级联清空" });
      delRow.append(actBtn("删除", "trash", () => toast("（mock）删除工作区需二次确认"), "danger"));
      dataSec.append(locRow, delRow);
      return [head("通用"), wsSec, prefSec, dataSec];
    }
    function segRow(label, items, val) {
      const seg = el("an-segmented"); seg.items = items; seg.value = val;
      const r = el("div"); r.style.cssText = "display:flex; align-items:center; justify-content:space-between; padding:var(--sp-2) var(--zero);";
      const t = el("div"); t.style.cssText = "font-size:var(--t-body); color:var(--ink);"; t.textContent = label;
      r.append(t, seg); return r;
    }
    function actBtn(label, icon, on, variant) {
      const b = el("an-button", { slot: "actions", variant: variant || "ghost", size: "sm", icon: icon });
      b.textContent = label; b.addEventListener("click", on); return b;
    }

    // ── ② 模型与 Key ──
    function models() {
      const ft = S.freeTier || {};
      // 免费档卡
      const ftSec = el("an-section", { label: "免费档" });
      const card = el("an-info-card", { title: ft.label + " · " + ft.model, icon: "sparkles" });
      const body = el("div"); body.style.cssText = "display:flex; flex-direction:column; gap:var(--sp-3);";
      body.append(quotaGauge(ft.quotaUsed, ft.quotaLimit, ft.resetAt));
      const note = el("div"); note.style.cssText = "font-size:var(--t-meta); color:var(--ink-3); line-height:var(--lh-ui);";
      note.textContent = "经我们代理 + 第三方 DeepSeek，不享本地隐私保证。仅只读 · 不可编辑。";
      const enableRow = el("div"); enableRow.style.cssText = "display:flex; align-items:center; justify-content:space-between;";
      const seg = el("an-segmented"); seg.items = ["关闭", "启用"]; seg.value = ft.enabled ? "启用" : "关闭";
      seg.addEventListener("an-segment", () => toast("（mock）启用免费档会先弹隐私同意"));
      const el2 = el("div"); el2.style.cssText = "font-size:var(--t-body); color:var(--ink);"; el2.textContent = "启用（首用弹隐私同意）";
      enableRow.append(el2, seg);
      body.append(note, enableRow);
      card.append(body);
      ftSec.append(card);

      // 默认模型（三场景）
      const defSec = el("an-section", { label: "默认模型" });
      (S.defaults || []).forEach((d) => defSec.append(field(d.scenario, d.model, { hint: d.hint, editor: "dropdown" })));

      // API Key
      const keySec = el("an-section", { label: "API Key" });
      keySec.append(addBtn("添加 Key", () => toast("（mock）添加 Key：选 provider → 填 key")));
      (S.keys || []).forEach((k) => {
        const r = el("an-row", { dot: dotOf(k.status), label: k.provider + " · " + k.name, hint: k.masked,
          meta: k.status === "error" ? (k.err || "异常") : ("✓ " + k.models + " 模型") });
        r.append(actBtn("测试", "check", () => toast("探活 " + k.name + " …")));
        r.append(actBtn("删除", "trash", () => toast("（mock）删 " + k.name), "danger"));
        keySec.append(r);
      });

      // 搜索引擎 Key
      const searchSec = el("an-section", { label: "搜索引擎 Key" });
      searchSec.append(field("WebSearch 用 Key", S.searchKey, { editor: "dropdown" }));
      return [head("模型与 Key"), ftSec, defSec, keySec, searchSec];
    }
    function quotaGauge(used, limit, resetAt) {
      const pct = Math.max(2, Math.round((used / limit) * 100));
      const wrap = el("div"); wrap.style.cssText = "display:flex; flex-direction:column; gap:var(--grid);";
      const track = el("div"); track.style.cssText = "height:var(--sp-2); border-radius:var(--r-pill); background:var(--island-3); overflow:hidden;";
      const fill = el("div"); fill.style.cssText = "height:100%; width:" + pct + "%; background:var(--accent); border-radius:var(--r-pill);";
      track.append(fill);
      const lbl = el("div"); lbl.style.cssText = "font-size:var(--t-meta); color:var(--ink-3); font-variant-numeric:tabular-nums;";
      lbl.textContent = "剩 " + (limit - used) + " / " + limit + " · " + resetAt + " 重置";
      wrap.append(track, lbl); return wrap;
    }
    function addBtn(label, on) {
      const b = el("an-button", { slot: "actions", variant: "primary", size: "sm", icon: "plus" });
      b.textContent = label; b.addEventListener("click", on); return b;
    }

    // ── ③ MCP 与市场 ──
    function mcp() {
      const oh = head("MCP 与市场");
      const tabs = el("an-tabs"); tabs.items = [{ value: "market", label: "市场" }, { value: "installed", label: "已装 " + (S.mcpInstalled || []).length }];
      tabs.value = "market";
      const body = el("div");
      const renderTab = (v) => { body.replaceChildren(v === "installed" ? mcpInstalled() : mcpMarket()); };
      tabs.addEventListener("an-tab", (e) => renderTab(e.detail.value));
      renderTab("market");
      return [oh, tabs, body];
    }
    function mcpMarket() {
      const grid = el("div");
      grid.style.cssText = "display:grid; grid-template-columns:repeat(auto-fill, minmax(var(--w-block), 1fr)); gap:var(--sp-3); margin-top:var(--sp-3);";
      const authTone = { token: "neutral", oauth: "accent", byo: "warn", "oauth-url": "accent", local: "neutral" };
      (S.mcpMarket || []).forEach((m) => {
        const card = el("an-info-card", { title: m.name, icon: "mcp", meta: m.desc });
        card.append(el("an-badge", { tone: authTone[m.auth] || "neutral" }, m.authLabel));
        card.append(actBtn("安装", "plus", () => toast(installMsg(m))));
        grid.append(card);
      });
      return grid;
    }
    function installMsg(m) {
      return ({ oauth: "装 " + m.name + " → 弹浏览器授权", byo: "装 " + m.name + " → 先填 client_id/secret + redirect URI",
        "oauth-url": "装 " + m.name + " → 先填实例 URL → 浏览器授权", token: "装 " + m.name + " → 填 token",
        local: "装 " + m.name + " → 先在 Figma 开 Dev Mode" }[m.auth]) || ("安装 " + m.name);
    }
    function mcpInstalled() {
      const sec = el("an-section", { label: "已连接的服务器" });
      (S.mcpInstalled || []).forEach((s) => {
        const r = el("an-row", { dot: dotOf(s.status), label: s.name, hint: s.source + " · " + s.tools + " 工具",
          meta: { ready: "就绪", degraded: "降级", failed: s.err || "失败" }[s.status] });
        r.append(actBtn("重连", "history", () => toast("重连 " + s.name)));
        r.append(actBtn("日志", "enter", () => toast("查看 " + s.name + " 调用日志")));
        r.append(actBtn("删除", "trash", () => toast("（mock）删 " + s.name), "danger"));
        sec.append(r);
      });
      return sec;
    }

    // ── ④ 技能 ──
    function skills() {
      const sec = el("an-section", { label: "技能库" });
      sec.append(addBtn("新建技能", () => toast("（mock）新建 SKILL.md")));
      (S.skills || []).forEach((sk) => {
        const r = el("an-row", { icon: "doc", label: sk.name, hint: sk.desc });
        r.append(el("an-badge", { slot: "meta", tone: sk.source === "ai" ? "accent" : "neutral" }, sk.source === "ai" ? "AI 生成" : "手建"));
        r.append(actBtn("编辑", "edit", () => toast("编辑 " + sk.name)));
        r.append(actBtn("删除", "trash", () => toast("（mock）删 " + sk.name), "danger"));
        sec.append(r);
      });
      return [head("技能"), sec];
    }

    // ── ⑤ 运行时与索引（应用级） ──
    function runtime() {
      // 嵌入引擎
      const emSec = el("an-section", { label: "嵌入引擎 · 语义搜索" });
      const seg = el("an-segmented"); seg.items = [{ value: "builtin", label: "内置" }, { value: "ollama", label: "Ollama" }, { value: "off", label: "关闭" }];
      seg.value = S.embedder;
      seg.addEventListener("an-segment", (e) => toast("切到 " + e.detail.value));
      const emRow = el("div"); emRow.style.cssText = "display:flex; align-items:center; justify-content:space-between; padding:var(--sp-2) var(--zero);";
      const emL = el("div"); emL.style.cssText = "display:flex; flex-direction:column; gap:var(--grid);";
      const emT = el("div"); emT.style.cssText = "font-size:var(--t-body); color:var(--ink);"; emT.textContent = "引擎";
      const emS = el("div"); emS.style.cssText = "font-size:var(--t-meta); color:var(--ink-3);";
      emS.append(el("an-status-dot", { state: "done" }), document.createTextNode(" " + S.embedderStatus));
      emL.append(emT, emS); emRow.append(emL, seg);
      emSec.append(emRow);
      const reidx = el("an-row", { icon: "search", label: "重新索引", hint: "清空当前工作区并重建" });
      reidx.append(actBtn("重建", "history", () => toast("开始重新索引 …")));
      emSec.append(reidx);

      // 运行时
      const rtSec = el("an-section", { label: "已装运行时" });
      rtSec.append(addBtn("安装运行时", () => toast("（mock）选 kind + 版本安装")));
      (S.runtimes || []).forEach((rt) => {
        const r = el("an-row", { icon: "box", label: rt.kind + " " + rt.version, meta: rt.size });
        r.append(actBtn("删除", "trash", () => toast("（mock）删 " + rt.kind), "danger"));
        rtSec.append(r);
      });

      // 存储
      const stSec = el("an-section", { label: "存储" });
      const disk = el("an-row", { icon: "box", label: "磁盘占用", meta: S.diskUsage });
      disk.append(actBtn("清理缓存 (GC)", "trash", () => toast("清理 30 天未用的环境 …")));
      const boot = el("an-row", { dot: "done", label: "运行时引导", hint: "沙箱已就绪", meta: "正常" });
      stSec.append(disk, boot);
      return [head("运行时与索引", "应用级 · 所有工作区共享"), emSec, rtSec, stSec];
    }

    // ── ⑥ 高级（运行上限，应用级） ──
    function advanced() {
      const kids = [head("高级", "应用级 · 所有工作区共享")];
      (S.limits || []).forEach(([group, rows]) => {
        const sec = el("an-section", { label: "运行上限 · " + group });
        const kv = el("an-kv"); kv.rows = rows.map((r) => ({ key: r[0], value: r[1], editable: true }));
        sec.append(kv); kids.push(sec);
      });
      const note = el("div"); note.style.cssText = "color:var(--ink-3); font-size:var(--t-meta); padding:var(--zero) var(--sp-1);";
      note.textContent = "改动即时热生效、无需重启。";
      kids[1] && kids[1].prepend(note);
      return kids;
    }

    // ── 路由 ──
    const PAGES = { general: general, models: models, mcp: mcp, skills: skills, runtime: runtime, advanced: advanced };
    function render(id) {
      if (ctx.shell) { ctx.shell.setRight(null); ctx.shell.setHeadTitle && ctx.shell.setHeadTitle(null); ctx.shell.setHeadModel && ctx.shell.setHeadModel(null); ctx.shell.setHeadMenu && ctx.shell.setHeadMenu(null); }
      page.replaceChildren.apply(page, (PAGES[id] || general)());
    }
    ctx.Intent.on("settingsCat", (sel) => { if (page.isConnected && sel && sel.id) render(sel.id); });
    render("general");
    return page;
  },
});
