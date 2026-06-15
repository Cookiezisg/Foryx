/* Forgify demo — 组件 flowrun（单一事实源；收掉 chat 海洋的 flowrunStrip / fireStrip / connStrip 三处副本）。
   契约：组件 = 工厂 → handle；自载同名 .css；只读令牌 + 配置(NODE_ICON) + icons；fg- 前缀；不碰任何 feature/别组件内部。
   形态铁律（durable 节点条，逐像素移植 design-lab 验证态）：
     头(frid + live pulse) + 逐节点行 running(shimmer)→ok/okPort/fail/park；三变体共一皮肤
     （flowrun=记忆化 durable / fire=ephemeral trigger 信号 / conn=mcp 连接态机——同骨、只换头文案与节点语义）。
   为何 park 复用 ApprovalGate 而非内联按钮：人在环 :decide 决策面是单一事实源（durable flavor），节点行只是它的宿主；
     StatusDot 供节点状态点、NODE_ICON 供 kind 图标——本组件零自绘状态色、零硬编码图标。
   frid 是 run 引用：点头部 → Intent.select{kind:'run'} 一个前门派发（切轨是别处事，本组件只负责选中意图）。
   API：Flowrun.strip(host, frid, {variant}) → { el, addNode(kind,id) → 节点 handle, finish() }
     variant ∈ {flowrun(默认), fire, conn}（仅换头标签；三者同皮肤、同节点 API）。
     节点 handle：running() · ok(memo) · okPort(port) · fail(msg) · park(autoMs) → Promise<'yes'|'no'>。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);
  const ICO = window.icon || (() => '');
  const NICON = window.NODE_ICON || {};
  const SD = window.StatusDot;
  const esc = s => String(s == null ? '' : s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

  // 头标签按 variant（durable 记忆化 / ephemeral trigger 信号 / mcp 连接）；皮肤共享、只换文案
  const HEAD = { flowrun: 'flowrun', fire: 'trigger fire', conn: 'connect' };

  function strip(host, frid, opts = {}) {
    const h = typeof host === 'string' ? window.qs(host) : host;
    const variant = HEAD[opts.variant] ? opts.variant : 'flowrun';

    const el = window.tag(`div.fg-fr.fg-fr-v-${variant}`);
    el.innerHTML =
      `<div class="fg-fr-head">
         <span class="fg-fr-fl"${frid ? ' role="button" tabindex="0"' : ''}>${esc(HEAD[variant])}</span>
         <span class="fg-fr-pulse" data-pulse></span>
       </div>
       <div class="fg-fr-nodes" data-nodes></div>`;

    // frid 是 run 引用（机器 ID，只作路由不显）：点变体标签 → Intent 一个前门派发 kind:run
    const fid = window.qs('.fg-fr-fl', el);
    if (frid && fid) {
      const go = () => window.Intent && Intent.select({ kind: 'run', id: String(frid) });
      fid.onclick = go;
      fid.onkeydown = e => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); go(); } };
    }

    if (h) h.appendChild(el);
    const nodesBox = window.qs('[data-nodes]', el);

    // 节点状态点：经 StatusDot 折正 flowrun 词汇（running/completed/failed/parked → DOT key）
    const dot = state => (SD ? SD.dot(state, { size: 7 }) : '');

    function addNode(kind, id) {
      const r = window.tag('div.fg-fr-node.enter.future');
      r.innerHTML =
        `<span class="fg-fr-dot" data-dot>${dot('future')}</span>
         <span class="fg-fr-nico">${ICO(NICON[kind] || kind, 15)}</span>
         <span class="fg-fr-nid">${esc(id)}</span>
         <span class="fg-fr-nkind">${esc(kind)}</span>
         <span class="fg-fr-nstatus" data-status>排队</span>`;
      nodesBox.appendChild(r);

      const statusEl = () => window.qs('[data-status]', r);
      const dotEl = () => window.qs('[data-dot]', r);
      // 改态 = 行类名(皮肤) + StatusDot 原位换点 + 右侧文案（三处一致，单点改）
      const setSt = (cls, state, html) => {
        r.className = `fg-fr-node ${cls}`;
        if (SD) SD.setSt(dotEl(), state); else dotEl().innerHTML = dot(state);
        statusEl().innerHTML = html;
      };

      return {
        el: r,
        running() { setSt('running', 'running', '<span class="fg-fr-shimmer">运行中…</span>'); },
        // ok：记忆化语义（memo=true 标「已记忆化」徽，durable record-once 的视觉证）
        ok(memo) { setSt('ok', 'completed', `${ICO('check', 13)} ok${memo ? ' <span class="fg-fr-memo">已记忆化</span>' : ''}`); },
        // okPort：control 节点解析出口（→ port: done/retry…）
        okPort(port) { setSt('ok', 'completed', `${ICO('check', 13)} ok <span class="fg-fr-port">→ port: ${esc(port)}</span>`); },
        fail(msg) { setSt('failed', 'failed', `${ICO('close', 13)} ${esc(msg || 'failed')}`); },
        // park：人在环 :decide。行进 parked 态，内联挂 ApprovalGate(durable)，决议后收口为 ok/parked。
        park(autoMs) {
          r.className = 'fg-fr-node parked';
          if (SD) SD.setSt(dotEl(), 'parked');
          statusEl().textContent = '等决策';
          const gateHost = window.tag('div.fg-fr-gate');
          r.insertAdjacentElement('afterend', gateHost);

          if (!window.ApprovalGate) {
            // 地基缺席兜底（不该发生）：直接判通过，保流程不卡
            return Promise.resolve('yes');
          }
          const gate = window.ApprovalGate.mount(gateHost, {
            flavor: 'durable',
            title: `审批收件箱 · ${id}`,
            prompt: '人工过目本节点产出，决定 flowrun 是否续走下一节点。',
            ddl: '剩 4m',
          });
          return gate.wait(autoMs ? 'yes' : null, autoMs || 1800).then(({ act }) => {
            const yes = act === 'yes';
            gate.settle(yes ? '已通过 · flowrun 续走' : '已驳回 · flowrun 终止');
            if (yes) {
              setSt('ok', 'completed', `${ICO('check', 13)} 已通过`);
            } else {
              r.className = 'fg-fr-node parked';
              if (SD) SD.setSt(dotEl(), 'parked');
              statusEl().textContent = '已驳回';
            }
            return yes ? 'yes' : 'no';
          });
        },
      };
    }

    // finish：熄灭 live pulse（run 终态，无更多节点推进）
    function finish() {
      const p = window.qs('[data-pulse]', el);
      if (p) p.classList.add('off');
    }

    return { el, addNode, finish };
  }

  window.Flowrun = { strip };
})();
