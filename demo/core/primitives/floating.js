/* Anselm 原语 F1 — Floating（浮层簇·命令式模块，非 custom element）。
   why：弹层须挂到 <body> 顶层逃出 shell 的 overflow/transform 裁剪，且要跨任意 anchor 复用，
   故不做 Shadow DOM 原语，而是 light-DOM 浮层节点 + 一次性注入 <style>（token-only，类名前缀 an-float-）。
   API：AnFloating.open(anchor, { content, align?, placement?, namespace?, className?, onClose? }) → { el, destroy } · AnFloating.close(namespace?)。
   行为：外点关闭 + Escape 栈 + 视口回避（夹住左缘/翻转上下）+ resize/scroll 重定位。 */
(function () {
  // 一次性注入皮肤（token-only；弹层是浮岛，无横向分割线）。
  var STYLE_ID = "an-float-style";
  function ensureStyle() {
    if (document.getElementById(STYLE_ID)) return;
    var s = document.createElement("style");
    s.id = STYLE_ID;
    s.textContent = `
      .an-float {
        position: fixed; z-index: 40; max-height: calc(100vh - var(--sp-8));
        overflow-x: hidden; overflow-y: auto; scrollbar-width: none; -ms-overflow-style: none;
      }
      .an-float::-webkit-scrollbar { width: var(--zero); height: var(--zero); }
    `;
    document.head.appendChild(s);
  }

  // 单节点构建：light-DOM 浮层根（design 用 window.el，demo 无此 helper → 局部内建）。
  function node(htmlStr) {
    var t = document.createElement("template");
    t.innerHTML = String(htmlStr).trim();
    return t.content.firstElementChild;
  }

  // class 白名单清洗：调用方传的额外类名只放字母数字/下划线/连字符，杜绝注入。
  function cleanClass(s) {
    return String(s || "").split(/\s+/).filter(function (x) { return /^[A-Za-z0-9_-]+$/.test(x); }).join(" ");
  }

  // 读 token 的 px 数值（视口回避用 --sp-2 作 gap/pad）。
  function tokenPx(name, fallback) {
    var v = parseFloat(getComputedStyle(document.documentElement).getPropertyValue(name));
    return Number.isFinite(v) ? v : fallback;
  }

  var active = {};

  function close(namespace) {
    var h = active[namespace || "global"];
    if (h) h.destroy();
  }

  function place(anchor, el, o) {
    var gap = tokenPx("--sp-2", 8);
    var pad = tokenPx("--sp-2", 8);
    var r = anchor.getBoundingClientRect();
    var ew = el.offsetWidth, eh = el.offsetHeight;
    var align = o.align || "start";
    var placement = o.placement || "bottom";
    var left = align === "end" ? r.right - ew : r.left;
    var top = placement === "top" ? r.top - eh - gap : r.bottom + gap;

    // 视口回避：夹住左右缘；底部溢出则翻到 anchor 上方；顶部再溢出则贴边。
    left = Math.max(pad, Math.min(left, window.innerWidth - ew - pad));
    if (top + eh > window.innerHeight - pad) top = r.top - eh - gap;
    if (top < pad) top = pad;

    el.style.left = left + "px";
    el.style.top = top + "px";
  }

  function open(anchor, o) {
    o = o || {};
    ensureStyle();
    var namespace = o.namespace || "global";
    close(namespace);

    var cls = "an-float " + cleanClass(o.className);
    var el = node('<div class="' + cls.trim() + '" role="presentation">' + (o.content || "") + "</div>");
    document.body.appendChild(el);
    place(anchor, el, o);

    function outside(e) {
      if (el.contains(e.target) || anchor.contains(e.target)) return;
      close(namespace);
    }
    function key(e) { if (e.key === "Escape") close(namespace); }
    function resize() { place(anchor, el, o); }
    function destroy() {
      document.removeEventListener("pointerdown", outside, true);
      document.removeEventListener("keydown", key, true);
      window.removeEventListener("resize", resize);
      window.removeEventListener("scroll", resize, true);
      if (el.parentNode) el.parentNode.removeChild(el);
      if (active[namespace] && active[namespace].el === el) delete active[namespace];
      if (o.onClose) o.onClose();
    }

    active[namespace] = { el: el, destroy: destroy };
    // 推迟一帧再挂外点监听，避开触发本次 open 的同一次 pointerdown。
    setTimeout(function () { document.addEventListener("pointerdown", outside, true); }, 0);
    document.addEventListener("keydown", key, true);
    window.addEventListener("resize", resize);
    window.addEventListener("scroll", resize, true);
    return active[namespace];
  }

  window.AnFloating = { open: open, close: close };
})();
