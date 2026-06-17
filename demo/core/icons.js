/* Anselm demo — 图标语法（共享）。
   图标集 = Lucide（整组 vendored 在 core/vendor/lucide.js，ISC；与 OpenAI Codex 前端同款）。
   本文件：① 领域语义 key → Lucide 名 的单一映射（ALIAS）；② 工具名 → 图标（TOOL_ICON + toolIcon 解析）；③ 把 Lucide 节点构建成 <svg>。
   feature/原语一律 icon('<key>')；领域 key 走 ALIAS，其余直接当 Lucide 名用。未知 key 回退 circle-question-mark 并记 window.ICON_MISSING。 */
(function () {
  // 语义别名：领域 key → Lucide 名（改图标只动这一处）
  var ALIAS = {
    // chrome
    chevr: "chevron-right", chevd: "chevron-down", more: "ellipsis", grip: "grip-vertical",
    close: "x", sliders: "sliders-horizontal", wrap: "text-wrap", expand: "maximize-2",
    // 实体 / 图节点 / 挂载
    function: "square-function", handler: "box", agent: "bot", workflow: "workflow",
    trigger: "zap", control: "git-branch", action: "play", approval: "shield-check", shield: "shield-check",
    mcp: "plug", skill: "book-open", doc: "file-text", entities: "layout-grid",
    chat: "message-square", conversation: "message-square", scheduler: "clock", gear: "settings",
    // 块/对话语义
    reasoning: "brain", tool: "wrench", subagent: "git-fork", turnend: "flag", terminal: "square-terminal",
    // 执行动词 / 动作
    run: "play", enter: "corner-down-left", stop: "square", spin: "loader-circle",
    forge: "hammer", edit: "square-pen", trash: "trash-2", web: "globe",
    iterate: "sparkles", history: "history", diff: "git-compare",   // AI 编辑（sparkles ≠ forge/hammer 重建环境）· 版本历史 · 版本 diff
    // 态占位
    empty: "inbox", error: "triangle-alert",
  };

  // 工具名 → 图标 key（block-tree 等：不同 tool_call 显不同图标）
  var TOOL_ICON = {
    run_function: "play", call_handler: "handler", invoke_agent: "agent", trigger_workflow: "workflow",
    run_shell: "tool", read_file: "doc", write_file: "edit", edit_file: "edit",
    web_search: "web", web_fetch: "web", search_blocks: "search",
  };
  function toolIcon(name) {
    var n = String(name || "").toLowerCase();
    if (TOOL_ICON[n]) return TOOL_ICON[n];
    if (/shell|bash|exec/.test(n)) return "tool";
    if (/search/.test(n)) return "search";
    if (/file|read|write|doc/.test(n)) return "doc";
    if (/web|fetch|http|url/.test(n)) return "web";
    if (/function/.test(n)) return "function";
    if (/handler/.test(n)) return "handler";
    if (/agent/.test(n)) return "agent";
    if (/workflow|trigger/.test(n)) return "workflow";
    if (/create|edit|build|forge/.test(n)) return "forge";
    if (/^mcp/.test(n)) return "mcp";
    return "tool";
  }

  var FALLBACK = "circle-question-mark";
  window.ICON_MISSING = window.ICON_MISSING || {};

  function nodesToSvg(nodes) {
    return nodes.map(function (n) {
      var tag = n[0], attrs = n[1] || {};
      var a = Object.keys(attrs).map(function (k) { return k + '="' + attrs[k] + '"'; }).join(" ");
      return "<" + tag + (a ? " " + a : "") + " />";
    }).join("");
  }

  window.icon = function (name, size, stroke) {
    size = size || 16; stroke = stroke || 1.7;
    var L = window.LUCIDE || {};
    var lname = ALIAS[name] || name;
    var nodes = L[lname];
    if (!nodes) { window.ICON_MISSING[name] = lname; nodes = L[FALLBACK] || []; }
    return '<svg viewBox="0 0 24 24" width="' + size + '" height="' + size +
      '" fill="none" stroke="currentColor" stroke-width="' + stroke +
      '" stroke-linecap="round" stroke-linejoin="round">' + nodesToSvg(nodes) + "</svg>";
  };

  window.toolIcon = toolIcon;
})();
