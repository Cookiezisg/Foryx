/* Foryx demo — 组件 right-island（单一事实源；收掉 chat .aside / documents .doc-aside / scheduler .sch-aside / entity-card 四份手搓右岛外壳的副本）。
   契约：组件 = 工厂 → handle；自载同名 .css；只读令牌 + icons；fg- 前缀；不碰任何 feature/别组件内部。
   为何作基座：四海洋各自手搓「从右滑入的抽屉」——同一组 width 0→W 弹簧滑入 + head(图标+标题+关) + 可滚 body——归一为本组件，海洋只填 body。
   幂等以 oceanId 为键：同海洋反复 create 复用同一 <aside data-ocean-right>；外壳 mount 时据该属性清理上个海洋的右岛（见 shell.js）。
   滑入纯 width/margin/opacity（脱离 widget 树外的 transition），故不阻塞、可被外壳整体重排。
   API：RightIsland.create(oceanId, {title, icon, width=372}) → {el, body, head, setHead(html), show(), hide(), toggle(), isOpen()}。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);
  const ICO = window.icon || ((k, n) => '');

  // 幂等取/建本海洋的 <aside>：已挂同 oceanId 的复用，否则新建并 append 到 Shell.body（外壳给的右岛宿主槽）。
  function ensure(oceanId, width) {
    const host = (window.Shell && Shell.body) || document.body;
    let el = host.querySelector(`aside.fg-island[data-ocean-right="${oceanId}"]`);
    if (el) return el;
    el = window.tag('aside.fg-island', { 'data-ocean-right': oceanId },
      `<div class="fg-island-head"><span class="fg-island-ico"></span>` +
      `<span class="fg-island-title"></span>` +
      `<button class="fg-island-x" type="button" title="关闭">${ICO('close', 16)}</button></div>` +
      `<div class="fg-island-body"></div>`);
    el.style.setProperty('--fg-island-w', (width || 372) + 'px');
    host.appendChild(el);
    return el;
  }

  function create(oceanId, opts = {}) {
    const el = ensure(oceanId, opts.width);
    const head = el.querySelector('.fg-island-head');
    const body = el.querySelector('.fg-island-body');
    const icoEl = el.querySelector('.fg-island-ico');
    const titleEl = el.querySelector('.fg-island-title');
    const xBtn = el.querySelector('.fg-island-x');

    if (opts.width) el.style.setProperty('--fg-island-w', opts.width + 'px');
    if (opts.icon != null) icoEl.innerHTML = ICO(opts.icon, 17);
    else if (!icoEl.innerHTML) icoEl.classList.add('fg-island-ico-empty');
    if (opts.title != null) titleEl.textContent = opts.title;

    // 宽度走 JS 内联具体 px（width 过渡到 var() 在部分引擎不触发；字面量 px 才可靠滑入）。reflow 提交 0 基线。
    const W = opts.width || 372;
    const hide = () => { el.classList.remove('show'); el.style.width = ''; };
    const reveal = () => { void el.offsetWidth; el.style.width = W + 'px'; el.classList.add('show'); };
    xBtn.onclick = hide;

    return {
      el,
      head,
      body,
      // 整体替换头部 html（海洋自定义副行/徽章时用；不传则保留默认 图标+标题+关 脚手架）。
      setHead(html) { head.innerHTML = html == null ? '' : html; return head; },
      show() { reveal(); return this; },
      hide() { hide(); return this; },
      toggle() { el.classList.contains('show') ? hide() : reveal(); return this; },
      isOpen() { return el.classList.contains('show'); },
    };
  }

  window.RightIsland = { create, ensure };
})();
