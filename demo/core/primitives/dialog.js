/* Anselm 原语 F4 — Dialog（横切件·命令式模块，非 custom element）。
   why：modal 须挂 <body> 顶层逃裁剪、视口居中 + 半透遮罩盖全屏、抢占焦点（陷阱 + Escape 关 + 背景滚动锁）——
   这是“全屏阻断浮层”形态，与 AnFloating（锚定）/ AnToast（屏角非阻断）正交，故同走 light-DOM + 一次性注入 <style>（token-only，类名前缀 an-dialog-）。
   分工：模块只管【居中壳 + 遮罩 + 生命周期】（焦点陷阱 / Escape / 滚动锁 / 进出动画）；
   内容与底栏由调用方用已有原语（<an-button> 等）拼好传入——模块不替业务排版。
   解剖：遮罩 → 居中卡（头：title + 关闭；体：content；底：actions 右对齐按钮组）。
   API：AnDialog.open({ title, content(节点|串), actions:[{label,variant,onClick}] }) → { close }。
        action.onClick 返回 false 可阻止关闭；variant 透传给 <an-button>（primary/danger/ghost…）。 */
(function () {
  // 一次性注入皮肤（token-only；遮罩 + 居中卡 + 进出动画）。
  var STYLE_ID = "an-dialog-style";
  function ensureStyle() {
    if (document.getElementById(STYLE_ID)) return;
    var s = document.createElement("style");
    s.id = STYLE_ID;
    s.textContent = `
      .an-dialog-mask {
        position: fixed; inset: var(--zero); z-index: 80;
        display: grid; place-items: center; padding: var(--sp-6);
        background: var(--scrim);
        opacity: 0; transition: opacity var(--d-mid) var(--ease-out);
      }
      .an-dialog-mask.is-in { opacity: 1; }
      .an-dialog {
        display: flex; flex-direction: column; width: 100%; max-width: var(--w-content);
        max-height: calc(100vh - var(--sp-12));
        border-radius: var(--r-island);
        background: var(--island); box-shadow: inset 0 0 0 var(--hairline) var(--line), var(--shadow-win);
        transform: translateY(var(--sp-2)) scale(.98);
        transition: transform var(--d-mid) var(--ease-spring);
      }
      .an-dialog-mask.is-in .an-dialog { transform: translateY(var(--zero)) scale(1); }
      .an-dialog-head {
        display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center;
        column-gap: var(--gap); height: var(--island-head); padding: 0 var(--sp-4);
        border-bottom: var(--hairline) solid var(--line);
      }
      .an-dialog-title {
        min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
        color: var(--ink); font-size: var(--t-strong); font-weight: 600; line-height: var(--lh-tight);
      }
      .an-dialog-close {
        display: grid; place-items: center; width: var(--ctl); height: var(--ctl);
        border-radius: var(--r-btn); border: 0; background: none; cursor: pointer; color: var(--ink-3);
        transition: background var(--d-fast), color var(--d-fast);
      }
      .an-dialog-close:hover { background: var(--island-3); color: var(--ink); }
      .an-dialog-close svg { width: var(--icon); height: var(--icon); }
      .an-dialog-body {
        min-height: 0; overflow-y: auto; padding: var(--sp-4);
        color: var(--ink-2); font-size: var(--t-body); line-height: var(--lh-prose);
      }
      .an-dialog-foot {
        display: flex; align-items: center; justify-content: flex-end; gap: var(--sp-2);
        padding: var(--sp-3) var(--sp-4); border-top: var(--hairline) solid var(--line);
      }
      .an-dialog-foot:empty { display: none; }
    `;
    document.head.appendChild(s);
  }

  // 单节点构建：light-DOM 子树根（demo 无 window.el → 局部内建，同 floating.js）。
  function node(htmlStr) {
    var t = document.createElement("template");
    t.innerHTML = String(htmlStr).trim();
    return t.content.firstElementChild;
  }

  // 焦点陷阱可聚焦元素选择器（穿透 light-DOM；<an-button> 内部焦点由其 shadow 自管，这里锚到宿主）。
  var FOCUSABLE = 'a[href], button:not([disabled]), an-button:not([disabled]), input:not([disabled]), [tabindex]:not([tabindex="-1"])';

  var active = null;   // 单实例 modal 栈（demo 不叠 modal）

  function open(o) {
    o = o || {};
    ensureStyle();
    if (active) active.close();
    var e = window.anEsc;
    var prevFocus = document.activeElement;

    var mask = node('<div class="an-dialog-mask" role="presentation"></div>');
    var card = node(
      '<div class="an-dialog" role="dialog" aria-modal="true">'
        + '<div class="an-dialog-head">'
          + '<span class="an-dialog-title">' + e(o.title || "") + "</span>"
          + '<button type="button" class="an-dialog-close" aria-label="Close">' + window.icon("close") + "</button>"
        + "</div>"
        + '<div class="an-dialog-body"></div>'
        + '<div class="an-dialog-foot"></div>'
      + "</div>"
    );
    mask.appendChild(card);

    // 内容契约：Node 直接挂（调用方自建富内容）；【串走 textContent 转义】——确认弹窗常嵌实体名等不可信文本，innerHTML 会 XSS。要富 HTML 就传 Node。
    var bodyEl = card.querySelector(".an-dialog-body");
    if (o.content instanceof Node) bodyEl.appendChild(o.content);
    else if (o.content != null) bodyEl.textContent = String(o.content);

    // 底栏：用已有 <an-button> 原语拼，variant 透传；onClick 返回 false 阻止关闭。
    var footEl = card.querySelector(".an-dialog-foot");
    (o.actions || []).forEach(function (a) {
      a = a || {};
      var btn = document.createElement("an-button");
      if (a.variant) btn.setAttribute("variant", a.variant);
      btn.textContent = a.label || "";
      btn.addEventListener("click", function () {
        var keep = a.onClick ? a.onClick() : undefined;
        if (keep !== false) close();
      });
      footEl.appendChild(btn);
    });

    // 滚动锁：记录并冻结 body overflow，关闭时还原。
    var prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    document.body.appendChild(mask);
    requestAnimationFrame(function () { mask.classList.add("is-in"); });

    function focusables() {
      return [].slice.call(card.querySelectorAll(FOCUSABLE));
    }
    function trap(ev) {
      if (ev.key !== "Tab") return;
      var list = focusables();
      if (!list.length) { ev.preventDefault(); return; }
      var first = list[0], last = list[list.length - 1];
      var cur = document.activeElement;
      if (ev.shiftKey && (cur === first || !card.contains(cur))) { ev.preventDefault(); last.focus(); }
      else if (!ev.shiftKey && cur === last) { ev.preventDefault(); first.focus(); }
    }
    function key(ev) {
      if (ev.key === "Escape") { ev.stopPropagation(); close(); return; }
      trap(ev);
    }
    function onMaskDown(ev) {
      // 点遮罩空白处关闭；点卡内不关。
      if (ev.target === mask) close();
    }

    var closed = false;
    function close() {
      if (closed) return; closed = true;
      document.removeEventListener("keydown", key, true);
      mask.removeEventListener("pointerdown", onMaskDown);
      document.body.style.overflow = prevOverflow;
      mask.classList.remove("is-in");
      var done = false;
      function remove() {
        if (done) return; done = true;
        if (mask.parentNode) mask.parentNode.removeChild(mask);
        if (active === handle) active = null;
        if (prevFocus && prevFocus.focus) { try { prevFocus.focus(); } catch (_) {} }
        if (o.onClose) o.onClose();
      }
      mask.addEventListener("transitionend", remove, { once: true });
      var d = parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--d-mid"));
      setTimeout(remove, (Number.isFinite(d) ? d : 240) + 60);
    }

    document.addEventListener("keydown", key, true);
    mask.addEventListener("pointerdown", onMaskDown);
    // 初始焦点落第一个可聚焦元素，否则锚到关闭键，保证陷阱有锚。
    var initial = focusables()[0] || card.querySelector(".an-dialog-close");
    if (initial && initial.focus) initial.focus();

    var handle = { el: card, mask: mask, close: close };
    active = handle;
    return handle;
  }

  function close() { if (active) active.close(); }

  window.AnDialog = { open: open, close: close };
})();
