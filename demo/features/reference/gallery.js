/* Anselm — reference 能力画廊（独立页 reference.html 的渲染器）。
   why 独立页：能力参考是设计/调试工具，与 app 海洋分离、单独管理；与正式 app 同源加载全部原语 + 同一 catalog（features/reference/catalog.js）。
   组织：左分类导航 = 分页器，点一类显一页（不再一长卷）；每页 = 页头（类名 + N 原语·M 态）+ 该类各原语展位。
   build(spec) 递归实体化（tag/attrs/props/text/html/children）；交互浮层由 demo 触发钮收口。 */
(function () {
  const el = (window.AnRef || {}).el;
  const CAT = window.REF_CATALOG || [];

  // ref-pill 等会调 Intent.select——独立页无路由，给个轻量 stub（toast 回显），避免点击报错。
  window.Intent = window.Intent || { select: (r) => window.AnToast && window.AnToast.show({ text: "ref → " + ((r && (r.id || r.kind)) || "") }) };

  // 交互浮层（toast/dialog/menu）：纯数据 catalog 表达不了 onclick，由渲染器收口已知浮层 → 触发钮。
  function demoTrigger(spec) {
    const btn = el("an-button", { icon: spec.icon || "play" }, spec.text || "打开");
    btn.addEventListener("click", () => {
      if (spec.demo === "toast") window.AnToast && window.AnToast.show({ text: spec.toast || "已保存 · flowrun fne_5e1a 运行完成" });
      else if (spec.demo === "dialog") window.AnDialog && window.AnDialog.open({ title: spec.title || "确认删除", content: spec.content || "此操作不可撤销，确定删除该实体？", actions: [{ label: "取消" }, { label: "删除", variant: "danger" }] });
      else if (spec.demo === "menu") window.AnMenu && window.AnMenu.open(btn, { items: spec.items || [{ value: "edit", label: "AI 编辑" }, { value: "dup", label: "复制" }, { value: "del", label: "删除" }] });
    });
    return btn;
  }

  // 声明 → 活体：tag + attrs(setAttribute) + props(JS 属性，如 .rows/.data/.graph) + text/html + children(递归)
  function build(spec) {
    if (spec == null) return null;
    if (typeof spec === "string") return document.createTextNode(spec);
    if (spec.demo) return demoTrigger(spec);
    const n = el(spec.tag, spec.attrs || {});
    if (spec.props) Object.assign(n, spec.props);
    if (spec.html != null) n.innerHTML = spec.html;
    else if (spec.text != null) n.textContent = spec.text;
    (spec.children || []).forEach((c) => { const b = build(c); if (b) n.append(b); });
    return n;
  }

  function boot() {
    const nav = document.querySelector(".ref-nav");
    const main = document.querySelector(".ref-main");
    if (!nav || !main || !el) return;
    const content = el("div", { class: "ref-content" });
    main.appendChild(content);

    // 左导航 = 分页器：每类一行（图标 + 名 + 原语数）
    const navRows = CAT.map((cat, i) => {
      const row = el("an-row", { icon: cat.icon || "blocks", label: cat.cat, meta: String((cat.items || []).length) });
      row.addEventListener("an-select", () => show(i));
      nav.appendChild(row);
      return row;
    });

    // 显一页：页头 + 该类各原语展位
    function show(i) {
      const cat = CAT[i];
      if (!cat) return;
      content.innerHTML = "";
      const nSpec = (cat.items || []).reduce((m, it) => m + (it.specimens || []).length, 0);
      content.append(el("an-ocean-header", { crumb: "能力参考 · Reference", title: cat.cat },
        el("span", { slot: "meta" }, (cat.items || []).length + " 个原语 · " + nSpec + " 个态")));
      (cat.items || []).forEach((item) => {
        const spec = el("an-spec", { name: item.name, tag: item.tag, blurb: item.blurb });
        (item.specimens || []).forEach((sp) => {
          const cell = el("an-specimen", { label: sp.label, span: !!sp.span });
          const live = build(sp);
          if (live) cell.append(live);
          spec.append(cell);
        });
        content.append(spec);
      });
      main.scrollTop = 0;
      navRows.forEach((r) => r.removeAttribute("selected"));
      if (navRows[i]) navRows[i].setAttribute("selected", "");
    }

    show(0);
  }

  if (document.readyState !== "loading") boot();
  else document.addEventListener("DOMContentLoaded", boot);
})();
