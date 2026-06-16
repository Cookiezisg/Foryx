/* Foryx demo — 组件 run-graph（单一事实源；收掉 scheduler 海洋的 runGraphSVG/nodeSVG/fwdD/loopD + .rn-* 皮肤）。
   契约：组件 = 工厂 → handle；自载同名 .css；只读令牌 + 配置(NODE_ICON) + icons；fg- 前缀；不碰任何 feature/别组件内部。
   形态铁律（Conducted Keynote 活运行图，逐像素移植 design-lab 验证态）：
     keynote 纯净白卡（164×60，只靠图标分 kind、颜色只在最小标记里）+ 前进/回边 bezier + 单束 accent wireFlow 导电边 + 彗星，
     5 态(completed/failed/parked/running/future) + f0/f1/f2 焦点景深 + loop 的重影栈/×N 迭代徽 + taken/ghost/future 三档边。
   为何 def 带实例 uid：marker/filter 同 svg 仅声明一次，多图同屏时 id 不撞。
   API：RunGraph.render(host, {nodes,edges,loopbacks,state,taken,ghost,live,iters,ports,vb,onNode})
        → {el, setState(map), setLive(edge), select(id), redraw()}
     nodes = [{id,kind,ref,x,y}]（{x,y}=左上，与 entities graphSVG 同契约）· edges/loopbacks = [[a,b]]
     state = {id:'completed|failed|parked|running|future'} · taken/ghost = 'a>b' 边键集 · live = 当前导电边 'a>b'|null
     iters = {id:n}（>1 出重影栈+×N） · ports = {id:'done'|'retry'…}（control 出口标）· vb = [w,h]
     onNode(id) = 节点点击回调（feature 接 Intent；缺省时组件自落 Intent.select{kind:'node',id}）。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  const esc = s => String(s == null ? '' : s).replace(/[&<>]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]));
  const ICO = window.icon || ((k, n) => '');
  const NICON = window.NODE_ICON || {};
  const NW = 164, NH = 60;
  let uidSeq = 0;

  // 前进边：源右心 → 目标左心，中点对称三次贝塞尔；末端留 7px 给箭头
  const fwdD = (s, t) => { const x1 = s.x + NW, y1 = s.y + NH / 2, x2 = t.x, y2 = t.y + NH / 2, mx = (x1 + x2) / 2; return `M${x1},${y1} C${mx},${y1} ${mx},${y2} ${x2 - 7},${y2}`; };
  // 回边：从源顶心拱到目标顶心（上拱 46px → 读作「返回弧」而非前进）
  const loopD = (s, t) => { const x1 = s.x + NW / 2, y1 = s.y, x2 = t.x + NW / 2, y2 = t.y, cy = Math.min(y1, y2) - 46; return `M${x1},${y1} C${x1},${cy} ${x2},${cy} ${x2},${y2}`; };

  // 单节点卡（focus tier 据态分 f0 前沿 / f1 已跑 / f2 未来；loop 出重影栈 + ×N 徽 + control 出 port 标）
  function nodeSVG(nd, ctx) {
    const st = ctx.state[nd.id] || 'future';
    const tier = (st === 'running' || st === 'parked' || st === 'failed') ? 'f0' : (st === 'future' ? 'f2' : 'f1');
    const its = ctx.iters[nd.id] || 1;
    const stack = its > 1 ? `<g class="fg-rng-stack"><rect x="6" y="6" width="${NW}" height="${NH}" rx="12" opacity=".25"/><rect x="3" y="3" width="${NW}" height="${NH}" rx="12" opacity=".5"/></g>` : '';
    const rR = st === 'future' ? 3 : (st === 'completed' ? 3.5 : 4);
    const refW = Math.min(112, (nd.ref || '').length * 6.6 + 10);
    const refPill = nd.ref ? `<rect class="fg-rng-refbg" x="42" y="34" width="${refW}" height="13" rx="5"/><text class="fg-rng-ref" x="46" y="44">${esc(nd.ref)}</text>` : '';
    const port = ctx.ports[nd.id] ? `<g class="fg-rng-port"><rect class="fg-rng-port-bg" x="${NW - 64}" y="${NH - 18}" width="58" height="14" rx="4"/><text class="fg-rng-port-tx" x="${NW - 60}" y="${NH - 8}">→ ${esc(ctx.ports[nd.id])}</text></g>` : '';
    const iter = its > 1 ? `<g class="fg-rng-iter${ctx.iterFailed && ctx.iterFailed[nd.id] ? ' failed' : ''}"><rect class="fg-rng-iter-bg" x="8" y="${NH - 18}" width="26" height="14" rx="7"/><text class="fg-rng-iter-tx" x="21" y="${NH - 8}" text-anchor="middle">×${its}</text></g>` : '';
    return `<g class="fg-rng-node ${nd.kind} ${st} ${tier}" data-id="${esc(nd.id)}" transform="translate(${nd.x},${nd.y})">
      ${stack}
      <rect class="fg-rng-card" width="${NW}" height="${NH}" rx="12" filter="url(#${ctx.lift})"/>
      <g class="fg-rng-ic" transform="translate(14,21)">${ICO(NICON[nd.kind] || nd.kind, 18)}</g>
      <text class="fg-rng-id" x="44" y="27">${esc(nd.id)}</text>
      ${refPill}
      <circle class="fg-rng-rp" cx="${NW - 15}" cy="15" r="${rR}"/>
      ${port}${iter}</g>`;
  }

  // 全图 svg：def(箭头×2 + lift 阴影，各带实例 uid) + 边层(taken实/future淡/ghost虚 + 回边 + live 导电+彗星) + 节点层
  function graphSVG(ctx) {
    const by = Object.fromEntries(ctx.nodes.map(x => [x.id, x]));
    let edges = '';
    ctx.edges.forEach(([a, b]) => {
      const key = a + '>' + b, cls = ctx.taken.includes(key) ? 'taken' : (ctx.ghost.includes(key) ? 'ghost' : 'future');
      const marker = cls === 'ghost' ? '' : (cls === 'future' ? ` marker-end="url(#${ctx.arrowFut})"` : ` marker-end="url(#${ctx.arrow})"`);
      if (by[a] && by[b]) edges += `<path class="fg-rng-edge ${cls}" d="${fwdD(by[a], by[b])}"${marker}/>`;
    });
    (ctx.loopbacks || []).forEach(([a, b]) => { if (by[a] && by[b]) edges += `<path class="fg-rng-loopback" d="${loopD(by[a], by[b])}" marker-end="url(#${ctx.arrow})"/>`; });
    if (ctx.live) {
      const [a, b] = ctx.live.split('>');
      const isLoop = (ctx.loopbacks || []).some(([x, y]) => x === a && y === b);
      if (by[a] && by[b]) {
        const d = isLoop ? loopD(by[a], by[b]) : fwdD(by[a], by[b]);
        edges += `<path class="fg-rng-edge live" d="${d}"/><circle class="fg-rng-comet" r="3"><animateMotion dur="0.9s" repeatCount="indefinite" path="${d}"/></circle>`;
      }
    }
    return `<svg viewBox="0 0 ${ctx.vb[0]} ${ctx.vb[1]}" preserveAspectRatio="xMidYMid meet" role="img">
      <defs>
        <marker class="fg-rng-mk" id="${ctx.arrow}" markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto"><path d="M0,0 L7,3.5 L0,7 z"/></marker>
        <marker class="fg-rng-mk-fut" id="${ctx.arrowFut}" markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto"><path d="M0,0 L7,3.5 L0,7 z"/></marker>
        <filter id="${ctx.lift}" x="-20%" y="-25%" width="140%" height="170%"><feDropShadow dx="0" dy="1" stdDeviation="2" flood-color="#000000" flood-opacity="0.08"/></filter>
      </defs>
      <g class="fg-rng-edges">${edges}</g>
      ${ctx.nodes.map(nd => nodeSVG(nd, ctx)).join('')}</svg>`;
  }

  function render(host, opts = {}) {
    const h = typeof host === 'string' ? window.qs(host) : host;
    const uid = 'rng' + (++uidSeq);
    const ctx = {
      nodes: opts.nodes || [],
      edges: opts.edges || [],
      loopbacks: opts.loopbacks || [],
      state: { ...(opts.state || {}) },
      taken: [...(opts.taken || [])],
      ghost: [...(opts.ghost || [])],
      live: opts.live || null,
      iters: opts.iters || {},
      ports: opts.ports || {},
      iterFailed: opts.iterFailed || {},
      vb: opts.vb || [1012, 340],
      onNode: typeof opts.onNode === 'function' ? opts.onNode : null,
      // def 实例化 id（防多图同屏撞 marker/filter）
      arrow: uid + '-arrow', arrowFut: uid + '-arrowFut', lift: uid + '-lift',
    };

    const el = window.tag('div.fg-rng');

    function wire() {
      el.querySelectorAll('.fg-rng-node').forEach(g => g.onclick = () => select(g.dataset.id));
    }
    function redraw() {
      el.innerHTML = graphSVG(ctx);
      wire();
      return handle;
    }
    // 点节点 → feature 回调（接 Intent）；缺省回退到统一意图通道（属图节点，kind=node）
    function select(id) {
      if (ctx.onNode) ctx.onNode(id);
      else if (window.Intent && window.Intent.select) window.Intent.select({ kind: 'node', id, source: 'run-graph' });
      return handle;
    }
    // 原位改态：合并节点态 map（不重传则保留），重分类边后整图重画
    function setState(map) {
      if (map) Object.assign(ctx.state, map);
      return redraw();
    }
    // 切换当前导电边（传 null 灭灯），整图重画换 wireFlow + 彗星
    function setLive(edge) {
      ctx.live = edge || null;
      return redraw();
    }

    const handle = { el, setState, setLive, select, redraw };
    redraw();
    if (h) h.appendChild(el);
    return handle;
  }

  window.RunGraph = { render };
})();
