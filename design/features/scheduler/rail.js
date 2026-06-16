/* Foryx demo — 运行海洋侧栏内容：每个 workflow 一行的派生运行状态一览（薄）。
   点行 → Intent.select({kind:'workflow'}) → sea.js 切驾驶舱。状态点走组件 StatusDot。依赖 mock/workflows.js。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);
  const W = () => window.MOCK_WORKFLOWS || {};

  function render(host) {
    const names = Object.keys(W());
    host.innerHTML = `
      <div class="sch-filter">${icon('search', 16)}<input type="text" placeholder="Filter workflows…"></div>
      <div class="sch-list">${names.map(nm => { const w = W()[nm]; return `<div class="sch-wf wf-${w.st}" data-id="${nm}">${StatusDot.dot(w.st)}<span class="sch-wf-t">${nm}</span><span class="sch-wf-meta">${w.meta || ''}</span></div>`; }).join('')}</div>`;

    host.querySelectorAll('.sch-wf').forEach(it => it.onclick = () => {
      host.querySelectorAll('.sch-wf').forEach(x => x.classList.remove('on')); it.classList.add('on');
      Intent.select({ kind: 'workflow', id: it.dataset.id });
    });
    const fin = host.querySelector('.sch-filter input');
    fin.oninput = () => { const q = fin.value.trim().toLowerCase(); host.querySelectorAll('.sch-wf').forEach(w => w.style.display = w.querySelector('.sch-wf-t').textContent.toLowerCase().includes(q) ? '' : 'none'); };
    const first = host.querySelector('.sch-wf'); if (first) first.classList.add('on');   // 默认高亮首个（与 sea 默认开同步）
  }
  SideBar.register('scheduler', render);
})();
