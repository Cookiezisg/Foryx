// ObservePane — pane shell around the full RelGraph. Force-directed view
// of all entities in the workspace + relations between them.
//
// ObservePane —— 全图谱 pane（RelGraph 力导向 + filter + node detail）。

import { Icon } from "../../components/primitives/Icon.jsx";
import { RelGraph } from "../../components/shared/RelGraph.jsx";

export function ObservePane() {
  return (
    <div className="page" style={{ display: "flex", flexDirection: "column", overflow: "hidden" }}>
      <div className="page-header">
        <div className="page-header-text">
          <div className="page-title"><Icon.GitBranch /> 洞察</div>
          <div className="page-subtitle">实体之间的引用关系。</div>
        </div>
      </div>
      <div style={{ flex: 1, minHeight: 0 }}>
        <RelGraph />
      </div>
    </div>
  );
}
