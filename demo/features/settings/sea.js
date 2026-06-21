/* Anselm feature — settings 海洋（sea）：左 rail 选类目 → Intent.on('settingsCat') → 渲对应设置页。
   六类：通用 / 模型与 Key / MCP 与市场 / 技能 / 运行时与索引 / 高级。各页 = ocean-header + an-section + 现有原语拼（无右岛）。
   纯展示 mock：字段/按钮接 toast，真后端时换端点。 */
window.FEATURE = window.FEATURE || {};
window.FEATURE.settings = Object.assign(window.FEATURE.settings || {}, {
  sea: (ctx) => {
    const el = window.el;
    const S = window.SETTINGS || {};
    const toast = (t) => window.AnToast && window.AnToast.show({ text: t });
    const page = el("an-page");

    // ── 共用小工具 ──
    const head = (title) => el("an-ocean-header", { crumb: "设置", title: title });
    const field = (label, value, opts) => el("an-field", Object.assign({ label: label, value: value, editable: "" }, opts || {}));
    const dotOf = (s) => ({ ok: "done", ready: "run", degraded: "wait", failed: "err", error: "err" }[s] || "idle");

    // ── ① 通用：全 an-field 五行（label 左 · 值右；名称走统一就地编辑、其余右侧 slot 控件），无分段标题 ──
    function general() {
      const ws = S.workspace || {};
      // 名称：可编辑 an-field —— hover key 右出铅笔 → value 原地变白底框 + ✓✕（统一就地编辑机制）
      const nameF = el("an-field", { label: "名称", value: ws.name, editable: "" });
      nameF.addEventListener("an-field-change", (ev) => toast("已保存名称：" + ev.detail.value));
      // 其余：an-field + 右侧 slot 控件（无 value → slot，仍右对齐、无铅笔）
      const slotField = (label, ctrl, hint) => { const f = el("an-field", hint ? { label: label, hint: hint } : { label: label }); f.append(ctrl); return f; };
      const del = el("an-button", { variant: "danger", outline: "", size: "sm" }); del.textContent = "删除";
      del.addEventListener("click", () => toast("（待单独设计）删除确认流"));

      const list = el("div");
      list.append(
        nameF,
        slotField("语言", dropdownVal("中文", ["中文", "English", "跟随系统"], "set-lang")),
        slotField("主题", dropdownVal("明亮", ["明亮", "暗色", "跟随系统"], "set-theme")),
        slotField("切换工作区", dropdownVal(ws.name, ["Personal", "Work", "Client X"], "ws-switch")),
        slotField("删除此工作区", del, "不可撤销 · 级联清空所有数据"),
      );
      return [head("通用"), list];
    }
    // 下拉值控件（语言/主题/切换工作区同款）：[当前 ⌄] ghost 钮 → AnMenu 列选项（勾当前）；选后更新钮文案
    function dropdownVal(current, options, ns) {
      const btn = el("an-button", { variant: "ghost", size: "sm" });
      const setLabel = (t) => { btn.innerHTML = window.anEsc(t) + '<span style="display:inline-flex; vertical-align:middle; margin-left:var(--gap-tight); color:var(--ink-3);">' + window.icon("chevd", 12) + "</span>"; };
      setLabel(current);
      btn.addEventListener("click", () => window.AnMenu && window.AnMenu.open(btn, {
        align: "end", placement: "bottom", namespace: ns,
        items: options.map((o) => ({ value: o, label: o, icon: o === btn.textContent.trim() ? "check" : undefined })),
        onPick: (v, it) => { setLabel(it.label); toast("已选 " + it.label); },
      }));
      return btn;
    }
    function actBtn(label, icon, on, variant) {
      const b = el("an-button", { slot: "actions", variant: variant || "ghost", size: "sm", icon: icon });
      b.textContent = label; b.addEventListener("click", on); return b;
    }

    // 设置页一次性皮肤：模型与 Key 走【卡片】（异构信息密、靠框分块，退「无边框靠留白」原则）+ provider-pick 浮层。inline 无 :hover，收口到这。
    function ensureSettingsStyle() {
      if (document.getElementById("an-set-style")) return;
      const s = document.createElement("style"); s.id = "an-set-style";
      s.textContent = `
        .an-pp { padding: var(--sp-1); border-radius: var(--r-chip); background: var(--island); box-shadow: inset 0 0 0 var(--hairline) var(--line), var(--shadow-pop); min-width: var(--side-w); max-height: var(--w-content); overflow-y: auto; }
        .an-pp-row { display: flex; align-items: center; gap: var(--gap); width: 100%; height: var(--row); padding: 0 var(--pad-row); border: var(--zero); background: none; cursor: pointer; border-radius: var(--r-btn); color: var(--ink); font-size: var(--t-body); text-align: left; }
        .an-pp-row:hover { background: var(--island-3); }
        .an-pp-ico { flex: none; width: var(--lead); height: var(--lead); display: grid; place-items: center; color: var(--ink); font-size: var(--lead); }
        .an-pp-ico svg { width: 1em; height: 1em; display: block; }
        .an-pp-name { min-width: var(--zero); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
        /* 卡片簇 */
        .mk-list { display: flex; flex-direction: column; gap: var(--sp-2); }
        .mk-card { display: flex; align-items: center; gap: var(--sp-3); padding: var(--sp-3) var(--sp-4); box-shadow: inset 0 0 0 var(--hairline) var(--line); border-radius: var(--r-chip); background: var(--island); }
        .mk-ico { flex: none; width: var(--ctl); height: var(--ctl); display: grid; place-items: center; color: var(--ink); font-size: calc(var(--lead) + var(--sp-1)); }
        .mk-ico.is-managed { color: var(--accent); }
        .mk-ico svg { width: 1em; height: 1em; display: block; }
        .mk-mid { min-width: var(--zero); flex: 1; display: flex; flex-direction: column; gap: calc(var(--grid) / 2); }
        .mk-name { display: flex; align-items: center; gap: var(--gap-tight); min-width: var(--zero); }
        .mk-name .t { font-size: var(--t-body); color: var(--ink); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
        .mk-sub { font-size: var(--t-meta); color: var(--ink-3); font-family: var(--mono); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
        .mk-right { flex: none; display: flex; align-items: center; gap: var(--sp-3); }
        .mk-add { display: flex; align-items: center; justify-content: center; gap: var(--gap-tight); width: 100%; min-height: var(--island-head); box-sizing: border-box; border: var(--hairline) dashed var(--line-strong); border-radius: var(--r-chip); background: transparent; color: var(--ink-3); cursor: pointer; font-size: var(--t-body); transition: background var(--d-fast), border-color var(--d-fast), color var(--d-fast); }
        .mk-add:hover { background: var(--island-2); border-color: var(--ink-3); color: var(--ink-2); }
        .mk-scn { display: flex; flex-direction: column; gap: var(--sp-2); padding: var(--sp-3) var(--sp-4); box-shadow: inset 0 0 0 var(--hairline) var(--line); border-radius: var(--r-chip); background: var(--island); }
        .mk-scn-top { display: flex; align-items: center; gap: var(--sp-2); }
        .mk-scn-cfg { display: flex; align-items: center; gap: var(--sp-3); flex-wrap: wrap; padding-top: var(--sp-2); box-shadow: inset 0 var(--hairline) 0 var(--line); }
        .mk-knob { display: flex; align-items: center; gap: var(--gap-tight); }
        .mk-knob > .k { font-size: var(--t-meta); color: var(--ink-3); }
        .mk-form { display: flex; flex-direction: column; gap: var(--sp-3); padding: var(--sp-4); box-shadow: inset 0 0 0 var(--hairline) var(--accent-line); border-radius: var(--r-chip); background: var(--island); }
      `;
      document.head.appendChild(s);
    }

    // ── ② 模型与 Key —— 三段：① key 列表(含免费档)+虚线建 key ② 默认模型 API→model→config 联动 ③ 搜索 key（见 WRK-034）──
    function models() {
      ensureSettingsStyle();
      const anEsc = window.anEsc, icon = window.icon, AnMenu = window.AnMenu, AnFloating = window.AnFloating;
      const P = S.providers || [];
      const provById = (n) => P.find((p) => p.name === n) || { glyph: "?", label: n, category: "llm" };
      const capsOf = (id) => (S.modelCaps || {})[id] || [];
      const okLlmKeys = () => (S.keys || []).filter((k) => k.status === "ok" && provById(k.provider).category !== "search");
      const chev = () => '<span style="display:inline-flex;vertical-align:middle;margin-left:var(--gap-tight);color:var(--ink-3);">' + icon("chevd", 12) + "</span>";
      const fmtCtx = (n) => n >= 1000000 ? (n / 1000000) + "M" : Math.round(n / 1000) + "K";

      // 供应商真实品牌图标（window.BRAND，lobehub/simple-icons 单色 currentColor）。缺真 logo 的回落：anselm→sparkles(accent)、custom→gear、搜索三家(serper/tavily/bocha)→字母 glyph 各自区分。
      const brandSvg = (p) => (window.BRAND && window.BRAND[p.name])
        || (p.name === "anselm" ? icon("sparkles", 20) : p.name === "custom" ? icon("gear", 20)
          : '<span style="font-size:var(--t-meta);font-weight:600;color:var(--ink-3);">' + anEsc(p.glyph || "?") + "</span>");
      const brandIco = (p) => { const s = el("span"); s.className = "mk-ico" + (p.managed ? " is-managed" : ""); s.innerHTML = brandSvg(p); return s; };
      const brandIcoHtml = (p) => '<span class="an-pp-ico">' + brandSvg(p) + "</span>";

      // 小下拉钮：dd(当前值, 当前label, items 函数, onPick, align)
      const dd = (curValue, curLabel, items, onPick, align) => {
        const btn = el("an-button", { variant: "ghost", size: "sm" });
        let cur = curValue;
        const setL = (t) => { btn.innerHTML = anEsc(t) + chev(); };
        setL(curLabel);
        btn.addEventListener("click", () => AnMenu && AnMenu.open(btn, {
          align: align || "end", placement: "bottom",
          items: items().map((it) => ({ value: it.value, label: it.label, icon: it.value === cur ? "check" : undefined })),
          onPick: (v, it) => { cur = v; setL(it.label); onPick && onPick(v, it); },
        }));
        return btn;
      };

      // ── ① key 行（含免费档 / 搜索 key）──
      const miniQuota = (q) => { const s = el("span"); s.style.cssText = "font-size:var(--t-meta);color:var(--ink-3);font-variant-numeric:tabular-nums;"; s.textContent = "剩 " + (q.limit - q.used) + " / " + q.limit + " · " + q.resetAt + " 重置"; return s; };
      const keyRowEl = (k) => {
        const p = provById(k.provider), isSearch = p.category === "search", isDefault = isSearch && k.id === S.defaultSearchKeyId;
        const card = el("div"); card.className = "mk-card";
        const mid = el("div"); mid.className = "mk-mid";
        const top = el("div"); top.className = "mk-name";
        const nm = el("span"); nm.className = "t"; nm.textContent = p.label + " · " + k.name; top.append(nm);
        if (k.managed) top.append(el("an-badge", { tone: "accent" }, "免费档"));
        if (isDefault) top.append(el("an-badge", { tone: "neutral" }, "默认"));
        const sub = el("span"); sub.className = "mk-sub"; sub.textContent = k.masked;
        mid.append(top, sub);
        const right = el("div"); right.className = "mk-right";
        if (k.managed && k.quota) right.append(miniQuota(k.quota));
        else if (k.status === "error") { const e = el("span"); e.style.cssText = "font-size:var(--t-meta);color:var(--danger);"; e.textContent = k.err || "异常"; right.append(e); }
        else if (!isSearch) { const m = el("span"); m.style.cssText = "font-size:var(--t-meta);color:var(--ink-3);"; m.textContent = capsOf(k.id).length + " 模型"; right.append(m); }
        if (!k.managed) {
          const more = el("an-button", { variant: "icon", size: "sm", icon: "more" });
          const menu = isSearch
            ? [{ value: "default", label: "设为默认搜索" }, { value: "test", label: "测试", icon: "check" }, { value: "del", label: "删除", icon: "trash" }]
            : [{ value: "test", label: "测试", icon: "check" }, { value: "del", label: "删除", icon: "trash" }];
          more.addEventListener("click", () => AnMenu.open(more, { align: "end", items: menu, onPick: (v) => toast({ default: "（mock）" + k.name + " 设为默认搜索", test: "探活 " + k.name + " …", del: "（mock）删 " + k.name }[v]) }));
          right.append(more);
        }
        card.append(brandIco(p), mid, right);
        return card;
      };

      // 建 key 配置卡（选完 provider 出现）：名称 + key +（ollama/custom）baseUrl +（custom）apiFormat + 测试/取消/保存
      const keyConfigForm = (p, onDone) => {
        const card = el("div"); card.className = "mk-form";
        const headRow = el("div"); headRow.style.cssText = "display:flex;align-items:center;gap:var(--sp-2);"; const hn = el("span"); hn.style.cssText = "font-size:var(--t-body);color:var(--ink);font-weight:600;"; hn.textContent = "新建 " + p.label + " Key"; headRow.append(brandIco(p), hn); card.append(headRow);
        const fieldRow = (label, ctrl) => { const f = el("div"); f.style.cssText = "display:flex;align-items:center;gap:var(--sp-3);min-height:var(--ctl);"; const l = el("span"); l.style.cssText = "flex:none;width:calc(var(--lead) * 5);font-size:var(--t-body);color:var(--ink-2);"; l.textContent = label; const wrap = el("div"); wrap.style.cssText = "flex:1;min-width:0;"; wrap.append(ctrl); f.append(l, wrap); return f; };
        card.append(fieldRow("名称", el("an-input", { full: "", placeholder: "显示名（如：个人 key）" })));
        card.append(fieldRow("Key", el("an-input", { full: "", placeholder: p.name === "ollama" ? "Ollama 仍需占位 key" : "粘贴 API Key（仅存一次、不回显）" })));
        if (p.baseReq) card.append(fieldRow("Base URL", el("an-input", { full: "", placeholder: p.base || "http://…" })));
        if (p.apiFormat) card.append(fieldRow("API 格式", dd("openai-compatible", "openai-compatible", () => [{ value: "openai-compatible", label: "openai-compatible" }, { value: "anthropic-compatible", label: "anthropic-compatible" }], null, "start")));
        const foot = el("div"); foot.style.cssText = "display:flex;align-items:center;justify-content:flex-end;gap:var(--sp-2);"; const test = el("an-button", { variant: "ghost", size: "sm" }, "测试"); test.addEventListener("click", () => toast("探活 " + p.label + " …")); const cancel = el("an-button", { variant: "ghost", size: "sm" }, "取消"); cancel.addEventListener("click", onDone); const save = el("an-button", { variant: "primary", size: "sm" }, "保存"); save.addEventListener("click", () => { toast("（mock）已建 " + p.label + " key"); onDone(); }); foot.append(test, cancel, save); card.append(foot);
        return card;
      };

      // 虚线「新建 key」框 → 点开 provider 浮层 → 配置卡（box ↔ form 切换）
      const addKeySlot = (category) => {
        const slot = el("div");
        const showBox = () => {
          slot.innerHTML = "";
          const box = el("button"); box.className = "mk-add";
          box.innerHTML = icon("plus", 16) + "<span>新建" + (category === "search" ? "搜索" : "") + " Key</span>";
          box.addEventListener("click", () => {
            const list = P.filter((p) => p.category === category && p.name !== "anselm");
            const rows = list.map((p) => '<button type="button" class="an-pp-row" data-prov="' + p.name + '">' + brandIcoHtml(p) + '<span class="an-pp-name">' + anEsc(p.label) + "</span></button>").join("");
            const fl = AnFloating.open(box, { namespace: "prov-pick", placement: "bottom", align: "start", className: "an-pp", content: rows });
            fl.el.querySelectorAll(".an-pp-row").forEach((r) => r.addEventListener("click", () => { AnFloating.close("prov-pick"); slot.innerHTML = ""; slot.append(keyConfigForm(provById(r.dataset.prov), showBox)); }));
          });
          slot.append(box);
        };
        showBox();
        return slot;
      };

      // ── ② 默认模型场景行：API → model → config 联动（改任一即重渲右侧）──
      const scenarioRow = (d) => {
        const st = { apiKeyId: d.ref.apiKeyId, modelId: d.ref.modelId, options: Object.assign({}, d.ref.options) };
        const card = el("div"); card.className = "mk-scn";
        const renderInner = () => {
          card.innerHTML = "";
          const top = el("div"); top.className = "mk-scn-top";
          const lbl = el("div"); lbl.style.cssText = "min-width:0;flex:1;display:flex;flex-direction:column;gap:calc(var(--grid)/2);";
          const t1 = el("div"); t1.style.cssText = "font-size:var(--t-body);color:var(--ink);font-weight:500;"; t1.textContent = d.label;
          const t2 = el("div"); t2.style.cssText = "font-size:var(--t-meta);color:var(--ink-3);"; t2.textContent = d.hint; lbl.append(t1, t2);
          const key = (S.keys || []).find((k) => k.id === st.apiKeyId), keyP = key ? provById(key.provider) : {};
          const apiDd = dd(st.apiKeyId, key ? (keyP.label + " · " + key.name) : "选 API", () => okLlmKeys().map((k) => ({ value: k.id, label: provById(k.provider).label + " · " + k.name })), (v) => { st.apiKeyId = v; const ms = capsOf(v); st.modelId = ms[0] ? ms[0].modelId : null; st.options = {}; if (ms[0]) (ms[0].knobs || []).forEach((kn) => st.options[kn.key] = kn.default); renderInner(); });
          const caps = capsOf(st.apiKeyId), curModel = caps.find((m) => m.modelId === st.modelId) || caps[0] || null;
          const modelDd = dd(st.modelId, curModel ? curModel.label : "—", () => caps.map((m) => ({ value: m.modelId, label: m.label })), (v) => { st.modelId = v; const m = caps.find((x) => x.modelId === v); st.options = {}; if (m) (m.knobs || []).forEach((kn) => st.options[kn.key] = kn.default); renderInner(); });
          top.append(lbl, apiDd, modelDd); card.append(top);
          const cfg = el("div"); cfg.className = "mk-scn-cfg";
          if (curModel) {
            (curModel.knobs || []).forEach((kn) => { const w = el("div"); w.className = "mk-knob"; const kl = el("span"); kl.className = "k"; kl.textContent = kn.label; w.append(kl, dd(st.options[kn.key] || kn.default, st.options[kn.key] || kn.default, () => kn.values.map((v) => ({ value: v, label: v })), (v) => { st.options[kn.key] = v; })); cfg.append(w); });
            if (!(curModel.knobs || []).length) { const none = el("span"); none.style.cssText = "font-size:var(--t-meta);color:var(--ink-3);"; none.textContent = "无可调参数"; cfg.append(none); }
            const ctx = el("span"); ctx.style.cssText = "font-size:var(--t-meta);color:var(--ink-3);margin-left:auto;"; ctx.textContent = fmtCtx(curModel.ctx) + " 上下文"; cfg.append(ctx);
          }
          if (cfg.children.length) card.append(cfg);
        };
        renderInner();
        return card;
      };

      // 装配三段
      const keySec = el("an-section", { label: "API Key" });
      const keyList = el("div"); keyList.className = "mk-list";
      (S.keys || []).filter((k) => provById(k.provider).category !== "search").forEach((k) => keyList.append(keyRowEl(k)));
      keyList.append(addKeySlot("llm")); keySec.append(keyList);

      const defSec = el("an-section", { label: "默认模型" });
      const defList = el("div"); defList.className = "mk-list";
      (S.defaults || []).forEach((d) => defList.append(scenarioRow(d))); defSec.append(defList);

      const searchSec = el("an-section", { label: "搜索引擎 Key" });
      const searchList = el("div"); searchList.className = "mk-list";
      (S.keys || []).filter((k) => provById(k.provider).category === "search").forEach((k) => searchList.append(keyRowEl(k)));
      searchList.append(addKeySlot("search")); searchSec.append(searchList);

      return [head("模型与 Key"), keySec, defSec, searchSec];
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
      const engineLabel = { builtin: "内置", ollama: "Ollama", off: "关闭" }[S.embedder] || "内置";
      const engineDd = dropdownVal(engineLabel, ["内置", "Ollama", "关闭"], "embed-engine");
      const emRow = el("div"); emRow.style.cssText = "display:flex; align-items:center; justify-content:space-between; padding:var(--sp-2) var(--zero);";
      const emL = el("div"); emL.style.cssText = "display:flex; flex-direction:column; gap:var(--grid);";
      const emT = el("div"); emT.style.cssText = "font-size:var(--t-body); color:var(--ink);"; emT.textContent = "引擎";
      const emS = el("div"); emS.style.cssText = "font-size:var(--t-meta); color:var(--ink-3);";
      emS.append(el("an-status-dot", { state: "done" }), document.createTextNode(" " + S.embedderStatus));
      emL.append(emT, emS); emRow.append(emL, engineDd);
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
      return [head("运行时与索引"), emSec, rtSec, stSec];
    }

    // ── ⑥ 高级（运行上限，应用级） ──
    function advanced() {
      const kids = [head("高级")];
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
