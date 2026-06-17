/* Anselm demo — schema 渲染器（L3 强制层）。
   renderEntity(kind, data) → 由 KIND_SCHEMA[kind] 声明驱动，拼出 <an-page>（Section + 原语），全程零手搓布局。
   段 layout:'grid' → 块进响应式 2 列网格（auto-fit minmax(--w-block,1fr)，窄了自动塌 1 列）；字段 type:'card' = InfoCard 块，span:'full' 跨行。
   字段型 → 原语：text→<an-field> · kv→<an-kv> · code→<an-code-editor> · json→<an-json-tree> · rows→<an-row> · card→<an-info-card>。 */
(function () {
  const LEAF = {
    text(f, v) {
      const el = document.createElement("an-field");
      if (f.label) el.setAttribute("label", f.label);
      el.setAttribute("value", v == null || v === "" ? "—" : String(v));
      if (f.editable) el.setAttribute("editable", "");   // 就地编辑（hover 铅笔 → input → 派 an-field-change）
      if (f.wrap || f.editable) el.setAttribute("wrap", "");   // 说明类长文本：多行换行 + label 顶对齐
      return el;
    },
    kv(f, v) {
      const el = document.createElement("an-kv");
      el.setAttribute("rows", JSON.stringify(Array.isArray(v) ? v : []));
      if (f.mono) el.setAttribute("mono", "");
      if (f.wrap) el.setAttribute("wrap", "");   // 长 value 多行自适应（列轨反转 + value 左对齐换行）
      return el;
    },
    code(f, v) {
      const el = document.createElement("an-code-editor");
      if (f.lang) el.setAttribute("lang", f.lang);
      if (f.editable !== false) el.setAttribute("editable", "");   // 代码块默认带编辑钮（编辑→保存/取消）；f.editable:false 退化只读
      el.textContent = v == null ? "" : String(v);
      return el;
    },
    json(f, v) {
      const el = document.createElement("an-json-tree");
      el.setAttribute("root", "false");
      if (typeof v === "string") el.setAttribute("json", v);
      else el.data = v == null ? {} : v;
      return el;
    },
    rows(f, v) {
      const wrap = document.createElement("div");   // an-row / an-row-detail 皆 display:block，自然块流堆叠（无需 flex 列）
      (Array.isArray(v) ? v : []).forEach((r) => {
        const row = document.createElement("an-row");
        ["icon", "dot", "label", "meta", "hint"].forEach((k) => { if (r[k] != null && r[k] !== "") row.setAttribute(k, r[k]); });
        if (!r.detail) {
          if (r.passive) row.setAttribute("passive", "");
          wrap.appendChild(row);
          return;
        }
        // 可展开详情（如调用记录）：an-row-detail 内化「行 + 详情面板 + 点行切显隐」，此处只声明行与详情内容
        const rd = document.createElement("an-row-detail");
        row.setAttribute("slot", "row");
        const kv = document.createElement("an-kv"); kv.setAttribute("wrap", ""); kv.rows = r.detail;
        rd.append(row, kv);
        wrap.appendChild(rd);
      });
      return wrap;
    },
    // graph：编排图框 = an-graph-canvas[framed toolbar enterable]（定高外框 + 悬浮缩放 + 进入编辑器，全内化进画布原语）。
    // 实体页展示【编排图定义】（edit 态，framed 自动 fit 全显不裁）；运行态叠加（最近一次 run）是 scheduler 的事，不放这。v = {nodes, edges}
    graph(f, v) {
      v = v || {};
      const cv = document.createElement("an-graph-canvas");
      cv.setAttribute("framed", ""); cv.setAttribute("toolbar", ""); cv.setAttribute("enterable", "");
      cv.setAttribute("mode", "edit");
      cv.graph = { nodes: v.nodes || [], edges: v.edges || [] };
      return cv;
    },
  };

  function renderField(f, data) {
    if (f.type === "card") {
      const card = document.createElement("an-info-card");
      if (f.title) card.setAttribute("title", f.title);
      if (f.icon) card.setAttribute("icon", f.icon);
      (f.fields || []).forEach((sf) => card.appendChild(renderField(sf, data)));
      if (f.span === "full") card.style.gridColumn = "1 / -1";
      return card;
    }
    const v = data ? data[f.key] : undefined;
    return (LEAF[f.type] || LEAF.text)(f, v);
  }

  // 仅渲染 schema 段（不含 an-page 滚动壳）→ DocumentFragment。供实体页 tab pane（概览）复用，避免嵌套 an-page 双滚动。
  window.renderEntitySections = function (kind, data) {
    const frag = document.createDocumentFragment();
    const schema = (window.KIND_SCHEMA || {})[kind];
    if (!schema) {
      const f = document.createElement("an-field");
      f.setAttribute("label", "未登记的实体类型"); f.setAttribute("value", String(kind));
      frag.appendChild(f); return frag;
    }
    schema.sections.forEach((sec) => {
      const s = document.createElement("an-section");
      if (sec.label) s.setAttribute("label", sec.label);
      if (sec.variant) s.setAttribute("variant", sec.variant);
      if (sec.layout === "grid") s.setAttribute("grid", "");   // 网格布局内化进 an-section[grid]，不再手搓容器
      sec.fields.forEach((f) => s.appendChild(renderField(f, data)));
      frag.appendChild(s);
    });
    return frag;
  };

  window.renderEntity = function (kind, data) {
    const page = document.createElement("an-page");
    page.appendChild(window.renderEntitySections(kind, data));
    return page;
  };
})();
