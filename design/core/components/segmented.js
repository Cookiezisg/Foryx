/* Foryx demo — 组件 segmented（单一事实源；收掉 onboarding .seg / settings .st-seg 两处滑动段控副本）。
   契约：组件 = 工厂函数 → handle；自载同名 .css；只读令牌；fg- 前缀；不碰别的海洋。
   一颗灰胶囊在选项间弹簧滑动（唯一动效）；段宽自适应文案，pill 随选中项 transform+width 跟随。
   API：Segmented.mount(host, options, {value, onPick}) → {el, value(), set(i)}
     options = [{value, label}] 或纯字符串数组（字符串即 value 即 label）。
     value = 初始选中的 value（缺省取第一项）；onPick(value, index) 每次切换回调。
     键盘：←/→ 在选项间移焦并选中（roving，循环）。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  // 归一：字符串项 → {value,label}；保持调用方两种写法都成立
  function norm(o) { return typeof o === 'string' ? { value: o, label: o } : { value: o.value, label: o.label != null ? o.label : o.value }; }

  function mount(host, options, opts = {}) {
    const items = (options || []).map(norm);
    const el = window.tag('div.fg-seg', { role: 'tablist' });
    const pill = window.tag('span.fg-seg-pill');
    el.appendChild(pill);

    let idx = Math.max(0, items.findIndex(it => it.value === opts.value));
    if (idx < 0) idx = 0;

    const btns = items.map((it, i) => {
      const b = window.tag('button.fg-seg-btn', {
        type: 'button',
        role: 'tab',
        'data-v': it.value,
        tabindex: i === idx ? '0' : '-1',
        'aria-selected': i === idx ? 'true' : 'false',
        onclick: () => set(i),
      }, `<span class="fg-seg-l">${esc(it.label)}</span>`);
      el.appendChild(b);
      return b;
    });

    // pill 落位：宽高取自选中按钮、transform 跟随其 offset；首帧无动画(防加载抖)
    function place(animate) {
      const b = btns[idx];
      if (!b) return;
      if (!animate) pill.style.transition = 'none';
      pill.style.width = b.offsetWidth + 'px';
      pill.style.height = b.offsetHeight + 'px';
      pill.style.transform = `translate(${b.offsetLeft}px, ${b.offsetTop}px)`;
      if (!animate) { void pill.offsetWidth; pill.style.transition = ''; }   // 强制回流后恢复过渡
    }

    function set(i, focus) {
      if (i < 0 || i >= items.length || !items.length) return;
      idx = i;
      btns.forEach((b, j) => {
        const on = j === i;
        b.classList.toggle('on', on);
        b.setAttribute('aria-selected', on ? 'true' : 'false');
        b.tabIndex = on ? 0 : -1;
      });
      place(true);
      if (focus) btns[i].focus();
      if (typeof opts.onPick === 'function') opts.onPick(items[i].value, i);
    }

    // 键盘 roving：←/→ 循环移焦并选中（段控选中即焦点，符合 tablist 自动激活语义）
    el.addEventListener('keydown', e => {
      if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
      e.preventDefault();
      const n = items.length;
      set(e.key === 'ArrowRight' ? (idx + 1) % n : (idx - 1 + n) % n, true);
    });

    if (host) host.appendChild(el);

    // 初态：先标 on 再无动画落位（host 已在文档树才有 offset 度量）
    btns.forEach((b, j) => b.classList.toggle('on', j === idx));
    place(false);
    // host 可能尚未布局完成（display:none / 异步插入）→ 下一帧补一次量
    requestAnimationFrame(() => place(false));

    return { el, value: () => (items[idx] || {}).value, set: i => set(i, false) };
  }

  // 最小转义（label 走文本语义；挡 < & 防注入）
  function esc(s) { return String(s == null ? '' : s).replace(/[&<>]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c])); }

  window.Segmented = { mount };
})();
