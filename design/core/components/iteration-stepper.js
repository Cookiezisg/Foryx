/* Foryx demo — 组件 iteration-stepper（单一事实源；收掉 scheduler .sa-iter 逐迭代行的副本）。
   契约：组件 = 工厂 → handle；自载同名 .css；只读令牌 + icons；fg- 前缀；不碰任何 feature/别组件内部。
   为何一行一迭代不合并：durable 引擎 UNIQUE(node,iteration) = n 次循环 n 行 n 结果，合并会抹掉记忆化的物理事实。
   API：IterationStepper.mount(host, {items, onSelect}) → {el}
     items = [{i, status, out}]；status 经折正 ok(✓绿) / fail(✕红) / running(转圈)；out = 该迭代输出文案。
     行属循环迭代（非带 kind/id 的实体），故点选只回 onSelect(item)、不触 Intent。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);
  const ICO = window.icon || ((k, n) => '');

  // 折正任意状态串 → 三态：fail / running / ok（兜底 ok，对齐 scheduler 原 sa-iter 三分支）
  function normSt(s) {
    if (s === 'failed' || s === 'fail' || s === 'err' || s === 'error') return 'fail';
    if (s === 'running' || s === 'run' || s === 'active') return 'running';
    return 'ok';
  }
  // 三态 → 标记图标（✓ / ✕ / 转圈，皆 13px 同原始度量）
  const MARK = { fail: () => ICO('close', 13), running: () => ICO('spin', 13), ok: () => ICO('check', 13) };

  function row(it) {
    const st = normSt(it.status);
    const r = window.tag(`div.fg-itstep-row.${st}`, {
      type: 'button',
      role: 'button',
      tabindex: '0',
    },
      `<span class="fg-itstep-n">iter ${esc(it.i)}</span>` +
      `<span class="fg-itstep-mk">${MARK[st]()}</span>` +
      `<span class="fg-itstep-ot">${esc(it.out)}</span>`);
    return r;
  }

  function mount(host, opts = {}) {
    const h = typeof host === 'string' ? window.qs(host) : host;
    const items = opts.items || [];
    const el = window.tag('div.fg-itstep');
    const rows = items.map(it => {
      const r = row(it);
      // 点选：高亮本行 + 回调（属循环迭代，无实体 kind/id，故不触 Intent）
      r.onclick = () => {
        rows.forEach(x => x.classList.toggle('on', x === r));
        if (typeof opts.onSelect === 'function') opts.onSelect(it);
      };
      el.appendChild(r);
      return r;
    });
    if (h) h.appendChild(el);
    return { el };
  }

  // 最小转义（挡 < & > 防注入；i 是数字、out 是任意文案）
  function esc(s) {
    return String(s == null ? '' : s).replace(/[&<>]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]));
  }

  window.IterationStepper = { mount };
})();
