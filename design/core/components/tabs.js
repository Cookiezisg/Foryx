/* Foryx demo — 组件 tabs（单一事实源；收掉 eo-tabs/eo-tab 文字下划线页签各处副本）。
   契约：组件 = 工厂函数 → handle；自载同名 .css；只读令牌；fg- 前缀；不碰别的海洋。
   文字下划线（非胶囊）：active = 墨色变深 + 一条弹簧滑动的短下划线；可选计数。
   API：Tabs.mount(host, items, opts) → {el, select(key)}
     items = [{key, label, cnt?, render(body, ctx)}]；opts = {onSelect(key, item)}
   懒渲染：切到某 tab 才调它的 render(body)；render 拿到的 body 是该 tab 专属容器。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  function mount(host, items, opts = {}) {
    items = items || [];
    const el = window.tag('div.fg-tabs');
    const nav = window.tag('div.fg-tabs-nav');
    const body = window.tag('div.fg-tabs-body');

    // 每个 tab 一条专属 body：切走只隐藏不销毁，避免重渲染丢态
    const panes = items.map(() => null);
    const btns = items.map((it, i) => {
      const b = window.tag('button.fg-tab', {
        type: 'button',
        'data-key': it.key,
        onclick: () => select(it.key),
      }, `<span class="fg-tab-l">${esc(it.label)}</span>${it.cnt != null ? `<span class="fg-tab-cnt">${it.cnt}</span>` : ''}`);
      nav.appendChild(b);
      return b;
    });

    function select(key) {
      const i = items.findIndex(it => it.key === key);
      if (i < 0) return;
      btns.forEach((b, j) => b.classList.toggle('on', j === i));
      // 懒渲染一次该 pane；其余隐藏（不卸载）
      panes.forEach((p, j) => { if (p) p.hidden = j !== i; });
      if (!panes[i]) {
        const pane = window.tag('div.fg-tab-pane');
        body.appendChild(pane);
        panes[i] = pane;
        if (typeof items[i].render === 'function') items[i].render(pane, items[i]);
      } else {
        panes[i].hidden = false;
      }
      if (typeof opts.onSelect === 'function') opts.onSelect(key, items[i]);
    }

    el.appendChild(nav);
    el.appendChild(body);
    if (host) host.appendChild(el);
    if (items.length) select(items[0].key);

    return { el, select };
  }

  // 最小转义（label 走 textContent 语义；这里只挡 < & 防注入）
  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]));
  }

  window.Tabs = { mount };
})();
