/* Anselm 原语 F3 — ModelPicker（浮层簇·命令式模块，非 custom element）。
   why：模型/API 切换 = 「选模型 + 选该模型的 key」二级选择。不走 AnMenu 嵌套子菜单——嵌套浮层的 hover 桥接 / 点外关闭 / Escape 栈风险大、且要改 AnFloating+AnMenu 地基（殃及全局所有菜单）；改用【单浮层·两栏】：左栏模型（按 provider 分组），右栏 = 当前 hover 模型的 keys；hover 左栏可用模型 → 右栏即列其 keys，点 key → 选定「模型+key」。单 Floating 弹层、零嵌套，复用 AnFloating 的定位 / 点外 / Escape。
   API：AnModelPicker.open(anchor, { models, onPick(modelId,keyId,{modelLabel,keyLabel}), namespace?, placement? }) → { el, destroy }。
   models = { current:{model,key}, providers:[{ provider, models:[{ id,label,available,badge?,keys:[{id,label,masked}] }] }] }；available:false 的模型置灰、不可 hover 展开。 */
(function () {
  // 一次性注入皮肤（token-only；复用 Row 密度与 Menu 同源观感，类名前缀 an-mp-）。
  var STYLE_ID = "an-mp-style";
  function ensureStyle() {
    if (document.getElementById(STYLE_ID)) return;
    var s = document.createElement("style");
    s.id = STYLE_ID;
    s.textContent = `
      .an-mp { padding: var(--sp-1); border-radius: var(--r-chip); background: var(--island); box-shadow: inset 0 0 0 var(--hairline) var(--line), var(--shadow-pop); }
      .an-mp-grid { display: grid; grid-template-columns: auto auto; align-items: stretch; }
      .an-mp-col { min-width: calc(var(--side-w) - var(--sp-8)); padding: var(--sp-1); }
      .an-mp-col + .an-mp-col { box-shadow: inset var(--hairline) 0 0 var(--line); }
      .an-mp-label { padding: var(--sp-2) var(--pad-row) var(--sp-1) calc(var(--pad-row) + var(--lead) + var(--gap)); color: var(--ink-3); font-size: var(--t-meta); font-weight: 600; line-height: var(--lh-ui); }
      .an-mp-row { display: grid; grid-template-columns: var(--lead) minmax(0, 1fr) auto; align-items: center; column-gap: var(--gap); width: 100%; height: var(--row); padding: 0 var(--pad-row); border: var(--zero); background: none; cursor: pointer; border-radius: var(--r-btn); color: var(--ink-2); font-size: var(--t-body); text-align: left; transition: background var(--d-fast), color var(--d-fast); }
      .an-mp-row:hover, .an-mp-row.is-active { background: var(--island-3); color: var(--ink); }
      .an-mp-row.is-disabled { opacity: .4; cursor: default; }
      .an-mp-lead { width: var(--lead); height: var(--lead); display: grid; place-items: center; color: var(--accent); }
      .an-mp-lead svg { display: block; width: var(--icon); height: var(--icon); }
      .an-mp-text { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
      .an-mp-meta { color: var(--ink-3); font-size: var(--t-meta); white-space: nowrap; font-variant-numeric: tabular-nums; }
      .an-mp-empty { padding: var(--sp-3) var(--pad-row); color: var(--ink-3); font-size: var(--t-meta); }
    `;
    document.head.appendChild(s);
  }

  // 模型行（左栏）：勾选态（=当前模型）走前导 check；badge 走尾 meta；不可用置灰。
  function modelRow(m, curModel) {
    var e = window.anEsc;
    var cls = "an-mp-row an-mp-model" + (m.available ? "" : " is-disabled");
    var lead = (m.id === curModel) ? window.icon("check") : "";
    var meta = m.badge ? '<span class="an-mp-meta">' + e(m.badge) + "</span>" : "";
    return '<button type="button" class="' + cls + '" data-model="' + e(m.id) + '"' + (m.available ? "" : ' aria-disabled="true"') + ">"
      + '<span class="an-mp-lead">' + lead + "</span>"
      + '<span class="an-mp-text">' + e(m.label || "") + "</span>" + meta + "</button>";
  }

  // key 行（右栏）：勾选态 = 该 key 且其属当前模型；masked 串走尾 meta。
  function keyRow(k, cur, modelIsCur) {
    var e = window.anEsc;
    var lead = (modelIsCur && k.id === cur.key) ? window.icon("check") : "";
    var meta = k.masked ? '<span class="an-mp-meta">' + e(k.masked) + "</span>" : "";
    return '<button type="button" class="an-mp-row an-mp-key" data-key="' + e(k.id) + '">'
      + '<span class="an-mp-lead">' + lead + "</span>"
      + '<span class="an-mp-text">' + e(k.label || "") + "</span>" + meta + "</button>";
  }

  function keysHtml(m, cur) {
    if (!m || !m.available || !(m.keys || []).length) return '<div class="an-mp-empty">无可用 Key</div>';
    var modelIsCur = m.id === cur.model;
    return '<div class="an-mp-label">API Key</div>' + m.keys.map(function (k) { return keyRow(k, cur, modelIsCur); }).join("");
  }

  function findModel(models, id) {
    var found = null;
    (models.providers || []).forEach(function (p) { (p.models || []).forEach(function (m) { if (m.id === id) found = m; }); });
    return found;
  }

  function open(anchor, o) {
    o = o || {};
    ensureStyle();
    var models = o.models || { providers: [], current: {} };
    var cur = models.current || {};
    var e = window.anEsc;
    var left = (models.providers || []).map(function (p) {
      return '<div class="an-mp-label">' + e(p.provider || "") + "</div>" + (p.models || []).map(function (m) { return modelRow(m, cur.model); }).join("");
    }).join("");
    var content = '<div class="an-mp"><div class="an-mp-grid">'
      + '<div class="an-mp-col an-mp-models">' + left + "</div>"
      + '<div class="an-mp-col an-mp-keys">' + keysHtml(findModel(models, cur.model), cur) + "</div>"
      + "</div></div>";
    var ns = o.namespace || "model-picker";
    var h = window.AnFloating.open(anchor, { content: content, align: "start", placement: o.placement || "top", namespace: ns, onClose: o.onClose });

    var keysCol = h.el.querySelector(".an-mp-keys");
    var activeModel = cur.model;
    function renderKeys(id) { activeModel = id; keysCol.innerHTML = keysHtml(findModel(models, id), cur); }
    // 初始高亮当前模型行：JS 遍历比对 data-model（getAttribute 已解码，与原值匹配）——【勿】把 cur.model 原值拼进 querySelector：含特殊字符会 SyntaxError 崩，且 data-model 经 e() 写入与裸值不等值永不命中
    h.el.querySelectorAll(".an-mp-model").forEach(function (b) { if (b.getAttribute("data-model") === (cur.model || "")) b.classList.add("is-active"); });
    // hover 左栏可用模型 → 右栏列其 keys + 行高亮（横向移入右栏选 key，不经过其它模型行）
    h.el.querySelectorAll(".an-mp-model").forEach(function (b) {
      if (b.classList.contains("is-disabled")) return;
      b.addEventListener("mouseenter", function () {
        h.el.querySelectorAll(".an-mp-model.is-active").forEach(function (x) { x.classList.remove("is-active"); });
        b.classList.add("is-active");
        renderKeys(b.dataset.model);
      });
    });
    // 点右栏 key（委托）→ 选定 activeModel + key、关弹层、回调
    keysCol.addEventListener("click", function (ev) {
      var kb = ev.target.closest(".an-mp-key"); if (!kb) return;
      var m = findModel(models, activeModel); if (!m) return;
      var k = (m.keys || []).find(function (x) { return x.id === kb.dataset.key; }); if (!k) return;
      if (o.onPick) o.onPick(m.id, k.id, { modelLabel: m.label, keyLabel: k.label });
      window.AnFloating.close(ns);
    });
    return h;
  }

  window.AnModelPicker = { open: open };
})();
