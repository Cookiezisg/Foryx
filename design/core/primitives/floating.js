/* Foryx 原语 — Floating。统一锚定弹层:定位、外点关闭、Escape 栈。
   API: FyFloating.open(anchor, { content, align?, placement?, namespace?, className? })。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  var active = {};

  function cleanClass(s) {
    return String(s || '').split(/\s+/).filter(function (x) { return /^[A-Za-z0-9_-]+$/.test(x); }).join(' ');
  }

  function tokenPx(name, fallback) {
    var v = parseFloat(getComputedStyle(document.documentElement).getPropertyValue(name));
    return Number.isFinite(v) ? v : fallback;
  }

  function close(namespace) {
    var h = active[namespace || 'global'];
    if (!h) return;
    h.destroy();
  }

  function place(anchor, el, o) {
    var gap = tokenPx('--sp-2', 8);
    var pad = tokenPx('--sp-2', 8);
    var r = anchor.getBoundingClientRect();
    var ew = el.offsetWidth;
    var eh = el.offsetHeight;
    var align = o.align || 'start';
    var placement = o.placement || 'bottom';
    var left = align === 'end' ? r.right - ew : r.left;
    var top = placement === 'top' ? r.top - eh - gap : r.bottom + gap;

    left = Math.max(pad, Math.min(left, window.innerWidth - ew - pad));
    if (top + eh > window.innerHeight - pad) top = r.top - eh - gap;
    if (top < pad) top = pad;

    el.style.left = left + 'px';
    el.style.top = top + 'px';
  }

  function open(anchor, o) {
    o = o || {};
    var namespace = o.namespace || 'global';
    close(namespace);
    var cls = 'fy-floating ' + cleanClass(o.className);
    var el = window.el('<div class="' + cls + '" role="presentation">' + (o.content || '') + '</div>');
    document.body.appendChild(el);
    place(anchor, el, o);

    function outside(e) {
      if (el.contains(e.target) || anchor.contains(e.target)) return;
      close(namespace);
    }
    function key(e) {
      if (e.key === 'Escape') close(namespace);
    }
    function resize() { place(anchor, el, o); }
    function destroy() {
      document.removeEventListener('pointerdown', outside, true);
      document.removeEventListener('keydown', key, true);
      window.removeEventListener('resize', resize);
      window.removeEventListener('scroll', resize, true);
      if (el.parentNode) el.parentNode.removeChild(el);
      if (active[namespace] && active[namespace].el === el) delete active[namespace];
      if (o.onClose) o.onClose();
    }

    active[namespace] = { el: el, destroy: destroy };
    setTimeout(function () { document.addEventListener('pointerdown', outside, true); }, 0);
    document.addEventListener('keydown', key, true);
    window.addEventListener('resize', resize);
    window.addEventListener('scroll', resize, true);
    return active[namespace];
  }

  window.FyFloating = { open: open, close: close };
})();
