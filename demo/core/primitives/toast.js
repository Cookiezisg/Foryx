/* Anselm 原语 F3 — Toast（横切件·命令式模块，非 custom element）。
   why：瞬时反馈须挂 <body> 顶层逃出 shell 裁剪、固定屏角（非 anchored，故不走 AnFloating），
   且多条要向上堆叠成栈、各自自动消隐——这是浮层的“无锚定时栈”形态，与 AnFloating（锚定单实例）正交，
   故同 Floating/Menu 走 light-DOM + 一次性注入 <style>（token-only，类名前缀 an-toast-）。
   解剖：固定右下角栈容器（最新在底、向上堆叠）；单条 = tone 色条 + 文案 + 可选 action 按钮 + 关闭。
   tone ∈ ok | warn | danger（缺省中性）；进出动画 = --d-mid + --ease-spring（位移 + 淡入）。
   API：AnToast.show({ text, tone?, action?:{label,onClick}, ms? }) → { close }。ms 缺省 4000；ms<=0 不自动消隐。 */
(function () {
  // 一次性注入皮肤（token-only；右下角固定栈，单条为浮岛 + 左侧 tone 色条）。
  var STYLE_ID = "an-toast-style";
  function ensureStyle() {
    if (document.getElementById(STYLE_ID)) return;
    var s = document.createElement("style");
    s.id = STYLE_ID;
    s.textContent = `
      .an-toast-stack {
        position: fixed; right: var(--sp-6); bottom: var(--sp-6); z-index: 60;
        display: flex; flex-direction: column-reverse; gap: var(--sp-2);
        pointer-events: none;
      }
      .an-toast {
        pointer-events: auto;
        display: grid; grid-template-columns: auto minmax(0, 1fr) auto auto; align-items: center;
        column-gap: var(--gap); min-width: var(--w-block); max-width: var(--island-w);
        padding: var(--sp-2) var(--sp-3); padding-left: var(--sp-2);
        border: var(--hairline) solid var(--line); border-radius: var(--r-chip);
        background: var(--island); box-shadow: var(--shadow-pop);
        color: var(--ink-2); font-size: var(--t-body); line-height: var(--lh-ui);
        opacity: 0; transform: translateY(var(--sp-2));
        transition: opacity var(--d-mid) var(--ease-spring), transform var(--d-mid) var(--ease-spring);
      }
      .an-toast.is-in { opacity: 1; transform: translateY(var(--zero)); }
      .an-toast-bar {
        width: var(--grid); align-self: stretch; border-radius: var(--r-pill); background: var(--ink-3);
      }
      .an-toast.is-ok     .an-toast-bar { background: var(--ok); }
      .an-toast.is-warn   .an-toast-bar { background: var(--warn); }
      .an-toast.is-danger .an-toast-bar { background: var(--danger); }
      .an-toast-text { min-width: 0; color: var(--ink); }
      .an-toast-action {
        display: inline-flex; align-items: center; height: var(--ctl-sm); padding: 0 var(--btn-pad-x-sm);
        border-radius: var(--r-btn); border: 0; background: none; cursor: pointer; white-space: nowrap;
        color: var(--accent); font-size: var(--t-meta); font-weight: 600;
        transition: background var(--d-fast);
      }
      .an-toast-action:hover { background: var(--accent-soft); }
      .an-toast-close {
        display: grid; place-items: center; width: var(--ctl-sm); height: var(--ctl-sm);
        border-radius: var(--r-btn); border: 0; background: none; cursor: pointer; color: var(--ink-3);
        transition: background var(--d-fast), color var(--d-fast);
      }
      .an-toast-close:hover { background: var(--island-3); color: var(--ink); }
      .an-toast-close svg { width: var(--icon-sm); height: var(--icon-sm); }
    `;
    document.head.appendChild(s);
  }

  // tone → 左侧色条类（中性缺省无后缀）。
  var TONE_CLASS = { ok: "is-ok", warn: "is-warn", danger: "is-danger" };

  // 单节点构建：light-DOM 子树根（demo 无 window.el → 局部内建，同 floating.js）。
  function node(htmlStr) {
    var t = document.createElement("template");
    t.innerHTML = String(htmlStr).trim();
    return t.content.firstElementChild;
  }

  var stackEl = null;
  function ensureStack() {
    if (stackEl && stackEl.parentNode) return stackEl;
    stackEl = node('<div class="an-toast-stack" role="region" aria-live="polite"></div>');
    document.body.appendChild(stackEl);
    return stackEl;
  }

  function show(o) {
    o = o || {};
    ensureStyle();
    var stack = ensureStack();
    var e = window.anEsc;
    var toneCls = TONE_CLASS[o.tone] ? " " + TONE_CLASS[o.tone] : "";

    var actionHtml = (o.action && o.action.label)
      ? '<button type="button" class="an-toast-action">' + e(o.action.label) + "</button>"
      : "";
    var el = node(
      '<div class="an-toast' + toneCls + '" role="status">'
        + '<span class="an-toast-bar" aria-hidden="true"></span>'
        + '<span class="an-toast-text">' + e(o.text || "") + "</span>"
        + actionHtml
        + '<button type="button" class="an-toast-close" aria-label="Close">' + window.icon("close") + "</button>"
      + "</div>"
    );
    stack.appendChild(el);
    // 推迟一帧加 is-in，让进入过渡真实触发（初值 → 终值）。
    requestAnimationFrame(function () { el.classList.add("is-in"); });

    var timer = null;
    function close() {
      if (timer) { clearTimeout(timer); timer = null; }
      if (!el.parentNode) return;
      el.classList.remove("is-in");
      // 等淡出过渡结束再摘除；栈空则连容器一并撤掉。
      var done = false;
      function remove() {
        if (done) return; done = true;
        if (el.parentNode) el.parentNode.removeChild(el);
        if (stackEl && stackEl.children.length === 0 && stackEl.parentNode) {
          stackEl.parentNode.removeChild(stackEl);
          stackEl = null;
        }
      }
      el.addEventListener("transitionend", remove, { once: true });
      // 兜底：transitionend 偶失（无头/被抢占）时按时长强制摘除。
      var fallback = parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--d-mid"));
      setTimeout(remove, (Number.isFinite(fallback) ? fallback : 240) + 60);
    }

    var actBtn = el.querySelector(".an-toast-action");
    if (actBtn) {
      actBtn.addEventListener("click", function () {
        if (o.action && o.action.onClick) o.action.onClick();
        close();
      });
    }
    el.querySelector(".an-toast-close").addEventListener("click", close);

    var ms = o.ms == null ? 4000 : o.ms;
    if (ms > 0) timer = setTimeout(close, ms);

    return { el: el, close: close };
  }

  window.AnToast = { show: show };
})();
