/* Anselm feature — documents 海洋（sea）：打开文档的编辑面 + 右岛信息；支持多文档切换。
   左岛点行 → Intent.select({kind:document}) → 本海洋 Intent.on('document') → loadDoc 重渲（页头 + 编辑器 + 右岛）。
   主面 = an-page（居中阅读列）：ocean-header（path crumb + 可改名标题）+ an-doc-editor（块编辑 + 斜杠 + @ + 悬卡）。
   右岛 = 大纲 ToC · 反链/出链 · 元信息 · AI 编辑入口。缺内容的节点生成占位。 */
window.FEATURE = window.FEATURE || {};
window.FEATURE.documents = Object.assign(window.FEATURE.documents || {}, {
  sea: (ctx) => {
    const el = (tag, attrs, ...kids) => {
      const n = document.createElement(tag);
      if (attrs) for (const k in attrs) { const v = attrs[k]; if (v == null || v === false) continue; if (k === "prop") Object.assign(n, v); else n.setAttribute(k, v === true ? "" : v); }
      kids.flat().forEach((c) => { if (c == null) return; n.append(c.nodeType ? c : document.createTextNode(String(c))); });
      return n;
    };
    const DOCS = window.DOCS || {};
    const treeLabel = (id) => { let f = null; (function walk(ns) { (ns || []).forEach((n) => { if (n.id === id) f = n.label; if (n.children) walk(n.children); }); })(window.DOC_TREE || []); return f; };
    const stub = (id) => { const t = treeLabel(id) || id; return { id, title: t, path: "/" + t, blocks: [{ type: "callout", tone: "info", text: "这篇文档还没有正文（demo 仅核心文档有内容）。按 / 选块、@ 提及开始写。" }], backlinks: [], outlinks: [], outline: [], meta: [["path", "/" + t], ["状态", "空文档"]], history: [] }; };
    const rowsCard = (title, icon, rows) => el("an-info-card", { title, icon }, ...rows.map((r) => el("an-row", r)));

    const page = el("an-page");
    const island = el("an-right-island", { title: "文档信息", icon: "doc" });

    function buildIsland(D, editor) {
      island.innerHTML = "";
      // 大纲 = an-outline（导引线 + 层级缩进 + 当前节高亮 + 点击滚到标题）
      const tocCard = el("an-info-card", { title: "大纲", icon: "list" });
      const toc = el("an-outline"); toc.items = D.outline || [];
      toc.addEventListener("an-outline-pick", (ev) => editor && editor.scrollToHeading(ev.detail.index));
      tocCard.append(toc);
      island.append(tocCard);
      island.append(rowsCard("反链 · 被引用", "history", (D.backlinks || []).length ? (D.backlinks).map((b) => ({ icon: b.icon, label: b.label, meta: b.meta, hint: b.hint })) : [{ label: "暂无反链", passive: true }]));
      island.append(rowsCard("出链 · @ 提及", "enter", (D.outlinks || []).length ? (D.outlinks).map((o) => ({ icon: o.icon, label: o.label, meta: o.meta, passive: true })) : [{ label: "暂无出链", passive: true }]));
      const metaCard = el("an-info-card", { title: "元信息", icon: "shield-check" });
      const metaKv = el("an-kv"); metaKv.setAttribute("wrap", ""); metaKv.rows = D.meta || []; metaCard.append(metaKv);
      island.append(metaCard);
    }

    function loadDoc(id) {
      const D = DOCS[id] || stub(id);
      const segs = (D.path || "").split("/").filter(Boolean);
      page.innerHTML = "";
      const oh = el("an-ocean-header", { crumb: "Documents | " + segs.slice(0, -1).join(" | "), title: D.title || "未命名", editable: true });
      // 页属性条 → 标题下的 meta 槽，全 an-badge（状态=tone+dot，其余=neutral「name value」）——非正文、不混进 markdown 块、零自画
      (D.props || []).forEach((p) => {
        if (p.badge != null) oh.append(el("an-badge", { slot: "meta", tone: p.tone || "neutral", dot: p.dot }, p.badge));
        else oh.append(el("an-badge", { slot: "meta" }, p.name + " " + p.value));
      });
      page.append(oh);
      const editor = el("an-doc-editor");
      editor.mentions = window.DOC_MENTIONS || [];
      editor.blocks = D.blocks || [];
      page.append(editor);
      buildIsland(D, editor);
    }

    ctx.Intent.on("document", (sel) => { if (page.isConnected && sel && sel.id) loadDoc(sel.id); });
    loadDoc(window.DOC_DEFAULT || "doc_prd");
    if (ctx.shell) ctx.shell.setRight(island);
    return page;
  },
});
