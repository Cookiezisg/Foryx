/* Foryx demo — 组件 ref-pill（单一事实源；收掉 chat 海洋的 refPill 模板 + data-ent→openEntity 各处副本）。
   契约：实体提及药丸 = 类型图标 + 文案，可点 → 经 Intent.select 一个前门派发（不碰任何海洋的 openEntity）。
   API：RefPill.html(kind,label,ref) → html（带 data-ref/data-kind）· RefPill.wire(root) 委托 .fg-ref 点击。
   kind 优先取 ENTITY_KINDS[kind].icon（9 类实体）；未登记则把 kind 当 icons.js key 兜底（doc/search 等纯提及）。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);
  const K = window.ENTITY_KINDS || {};
  const esc = s => String(s == null ? '' : s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

  // 实体提及药丸：类型图标 + 文案；ref 非空才挂可点意图坐标（data-ref/data-kind）
  function html(kind, label, ref) {
    const ico = (K[kind] && K[kind].icon) || kind;
    const data = ref ? ` data-ref="${esc(ref)}" data-kind="${esc(kind)}"` : '';
    return `<span class="fg-ref"${data}><span class="fg-ref-ico">${window.icon(ico, 12)}</span>${esc(label)}</span>`;
  }

  // 委托点击：一个监听管整片 root 内所有药丸 → Intent.select({kind,id})（id 即 data-ref）
  function wire(root) {
    if (!root) return;
    root.addEventListener('click', e => {
      const p = e.target.closest && e.target.closest('.fg-ref[data-ref]');
      if (!p || !root.contains(p)) return;
      window.Intent && Intent.select({ kind: p.dataset.kind, id: p.dataset.ref });
    });
  }

  window.RefPill = { html, wire };
})();
