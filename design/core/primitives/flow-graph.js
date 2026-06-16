/* Foryx 原语 — FlowGraph。Workflow 图是可选择的真实图面,不是节点/边列表。
   API: FyFlowGraph.mount(host,{ graph, selected?, onNode? }) → {el,select}。 */
(function () {
  if (window.cssNextTo) cssNextTo(document.currentScript);

  var NODE_W = 164;
  var NODE_H = 58;
  var GAP_X = 74;
  var GAP_Y = 44;
  var PAD = 28;

  function normalizeGraph(graph) {
    graph = graph || {};
    var nodes = (graph.nodes || []).map(function (node, i) {
      var row = node.row || 0;
      return Object.assign({
        id: node.id || node.name,
        x: node.x == null ? PAD + i * (NODE_W + GAP_X) : node.x,
        y: node.y == null ? PAD + row * (NODE_H + GAP_Y) : node.y,
        status: node.status || 'ready',
      }, node);
    });
    var maxX = nodes.reduce(function (m, n) { return Math.max(m, n.x + NODE_W + PAD); }, NODE_W + PAD * 2);
    var maxY = nodes.reduce(function (m, n) { return Math.max(m, n.y + NODE_H + PAD); }, NODE_H + PAD * 2);
    return { nodes: nodes, edges: graph.edges || [], width: graph.width || maxX, height: graph.height || maxY };
  }

  function nodeById(nodes) {
    return nodes.reduce(function (map, n) { map[n.id || n.name] = n; return map; }, {});
  }

  function edgePath(a, b) {
    var x1 = a.x + NODE_W;
    var y1 = a.y + NODE_H / 2;
    var x2 = b.x;
    var y2 = b.y + NODE_H / 2;
    var mid = x1 + Math.max(GAP_X / 2, (x2 - x1) / 2);
    return 'M' + x1 + ',' + y1 + ' C' + mid + ',' + y1 + ' ' + mid + ',' + y2 + ' ' + (x2 - 8) + ',' + y2;
  }

  function kindInitial(kind) {
    return ({ trigger: 'T', Function: 'F', Handler: 'H', Agent: 'A', Workflow: 'W' }[kind]) || String(kind || 'N').charAt(0).toUpperCase();
  }

  function svg(graph, selected) {
    var g = normalizeGraph(graph);
    var byId = nodeById(g.nodes);
    var edges = g.edges.map(function (edge) {
      var a = byId[edge.from];
      var b = byId[edge.to];
      if (!a || !b) return '';
      return '<path class="fy-flow-edge' + (edge.hot ? ' hot' : '') + '" d="' + edgePath(a, b) + '" marker-end="url(#fy-flow-arrow)"></path>'
        + (edge.port ? '<text class="fy-flow-port" x="' + ((a.x + b.x + NODE_W) / 2) + '" y="' + (Math.min(a.y, b.y) + NODE_H / 2 - 8) + '">' + window.esc(edge.port) + '</text>' : '');
    }).join('');
    var nodes = g.nodes.map(function (node) {
      var id = node.id || node.name;
      return '<g class="fy-flow-node ' + window.esc(node.status || 'ready') + (selected === id ? ' on' : '') + '" data-node="' + window.esc(id) + '" transform="translate(' + node.x + ',' + node.y + ')" tabindex="0" role="button">'
        + '<rect class="fy-flow-card" width="' + NODE_W + '" height="' + NODE_H + '" rx="12"></rect>'
        + '<circle class="fy-flow-kind" cx="22" cy="29" r="12"></circle>'
        + '<text class="fy-flow-kind-text" x="22" y="33" text-anchor="middle">' + window.esc(kindInitial(node.kind)) + '</text>'
        + '<text class="fy-flow-title" x="44" y="25">' + window.esc(node.label || id) + '</text>'
        + '<text class="fy-flow-sub" x="44" y="44">' + window.esc(node.name || node.kind || '') + '</text>'
        + '<circle class="fy-flow-state" cx="148" cy="18" r="4"></circle>'
        + '</g>';
    }).join('');
    return '<svg class="fy-flow-svg" viewBox="0 0 ' + g.width + ' ' + g.height + '" preserveAspectRatio="xMidYMid meet" role="img">'
      + '<defs><marker id="fy-flow-arrow" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><path class="fy-flow-arrow" d="M0,0 L8,4 L0,8 z"></path></marker></defs>'
      + '<g class="fy-flow-edges">' + edges + '</g><g class="fy-flow-nodes">' + nodes + '</g></svg>';
  }

  function mount(host, o) {
    o = o || {};
    var selected = o.selected || null;
    var el = window.tag('div.fy-flow-graph');

    function wire() {
      window.qsa('.fy-flow-node', el).forEach(function (node) {
        function activate() { select(node.dataset.node); }
        node.addEventListener('click', activate);
        node.addEventListener('keydown', function (ev) {
          if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); activate(); }
        });
      });
    }
    function draw() {
      el.innerHTML = svg(o.graph || {}, selected);
      wire();
    }
    function select(id) {
      selected = id;
      draw();
      if (o.onNode) o.onNode(id);
    }

    draw();
    if (host) host.appendChild(el);
    return { el: el, select: select };
  }

  window.FyFlowGraph = { mount: mount };
})();
