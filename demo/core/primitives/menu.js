/* Anselm 原语 F2 — Menu（浮层簇·命令式模块，非 custom element）。
   why：菜单 = 内容是一组行的 Floating，须挂 body 顶层逃裁剪、跨 anchor 复用，故同 Floating 走 light-DOM + 一次性注入 <style>（token-only，类名前缀 an-menu-）。
   解剖：菜单行 = leading icon/check + label + meta，复用 Row 密度（--lead | 1fr | auto，--row 行高）；分组只用 label + 留白（无分割线）。
   行/项支持 checked / meta / danger / disabled。
   API：AnMenu.html({ items, compact? }) / AnMenu.open(anchor, opts) / AnMenu.attach(anchor, opts)。 */
(function () {
  // 一次性注入皮肤（token-only；菜单复用 Row 密度，无分割线）。
  var STYLE_ID = "an-menu-style";
  function ensureStyle() {
    if (document.getElementById(STYLE_ID)) return;
    var s = document.createElement("style");
    s.id = STYLE_ID;
    s.textContent = `
      .an-menu {
        min-width: calc(var(--side-w) - var(--sp-4));
        padding: var(--sp-1); border: var(--hairline) solid var(--line); border-radius: var(--r-chip);
        background: var(--island); box-shadow: var(--shadow-pop);
      }
      .an-menu-compact { min-width: calc(var(--side-w) - var(--sp-8)); }
      .an-menu-label {
        padding: var(--sp-2) var(--pad-row) var(--sp-1) calc(var(--pad-row) + var(--lead) + var(--gap));
        color: var(--ink-3); font-size: var(--t-meta); font-weight: 600; line-height: var(--lh-ui);
      }
      .an-menu-item {
        display: grid; grid-template-columns: var(--lead) minmax(0, 1fr) auto; align-items: center;
        column-gap: var(--gap); width: 100%; height: var(--row); padding: 0 var(--pad-row);
        border: var(--zero); background: none; cursor: pointer;
        border-radius: var(--r-btn); color: var(--ink-2); font-size: var(--t-body); text-align: left;
        transition: background var(--d-fast), color var(--d-fast);
      }
      .an-menu-item:hover { background: var(--island-3); color: var(--ink); }
      .an-menu-item.is-danger { color: var(--danger); }
      .an-menu-item.is-danger:hover { background: var(--danger-soft); }
      .an-menu-item.is-disabled { opacity: .4; pointer-events: none; }
      .an-menu-lead { width: var(--lead); height: var(--lead); display: grid; place-items: center; color: var(--ink-3); }
      .an-menu-lead svg { display: block; width: var(--icon); height: var(--icon); }
      .an-menu-text { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
      .an-menu-meta { color: var(--ink-3); font-size: var(--t-meta); white-space: nowrap; font-variant-numeric: tabular-nums; }
    `;
    document.head.appendChild(s);
  }

  function itemHtml(it, idx) {
    it = it || {};
    var e = window.anEsc;
    if (it.type === "label") {
      return '<div class="an-menu-label">' + e(it.label || "") + "</div>";
    }
    var cls = "an-menu-item"
      + (it.danger ? " is-danger" : "")
      + (it.disabled ? " is-disabled" : "");
    var lead = it.icon ? window.icon(it.icon) : (it.checked ? window.icon("check") : "");   // 勾选态由前导 check 图标承载
    var meta = it.meta ? '<span class="an-menu-meta">' + e(it.meta) + "</span>" : "";
    var attrs = ' data-index="' + idx + '"'   // 选中走 data-index → 闭包 items[idx]，故无需 data-value
      + (it.disabled ? ' aria-disabled="true"' : "");
    return '<button type="button" class="' + cls + '"' + attrs + ">"
      + '<span class="an-menu-lead">' + lead + "</span>"
      + '<span class="an-menu-text">' + e(it.label || "") + "</span>"
      + meta + "</button>";
  }

  function html(o) {
    o = o || {};
    var cls = "an-menu" + (o.compact ? " an-menu-compact" : "");
    return '<div class="' + cls + '" role="menu">' + (o.items || []).map(itemHtml).join("") + "</div>";
  }

  function open(anchor, o) {
    o = o || {};
    ensureStyle();
    var h = window.AnFloating.open(anchor, {
      content: html(o),
      align: o.align || "end",
      placement: o.placement || "bottom",
      namespace: o.namespace || "menu",
      onClose: o.onClose,   // 转发地基 onClose（消费者据此复位态，免自建 body MutationObserver）
    });
    h.el.querySelectorAll(".an-menu-item").forEach(function (b) {
      b.addEventListener("click", function () {
        var it = (o.items || [])[Number(b.dataset.index)];
        if (!it || it.disabled) return;
        if (o.onPick) o.onPick(it.value, it);
        if (!it.keepOpen) window.AnFloating.close(o.namespace || "menu");
      });
    });
    return h;
  }

  function attach(anchor, o) {
    anchor.addEventListener("click", function (e) {
      e.preventDefault();
      e.stopPropagation();
      open(anchor, o);
    });
  }

  window.AnMenu = { html: html, open: open, attach: attach };
})();
