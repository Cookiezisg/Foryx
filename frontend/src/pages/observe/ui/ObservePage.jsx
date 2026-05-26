// ObservePage — pane shell around the full RelGraph. Force-directed view
// of all entities in the workspace + relations between them.
//
// ObservePage —— 全图谱 page（RelGraph 力导向 + filter + node detail）。

import { useTranslation } from "react-i18next";
import { Icon } from "../../../components/primitives/Icon.jsx";
import { RelGraph } from "../../../widgets/rel-graph/RelGraph.jsx";

export function ObservePage() {
  const { t } = useTranslation("misc");
  return (
    <div className="page" style={{ display: "flex", flexDirection: "column", overflow: "hidden" }}>
      <div className="page-header">
        <div className="page-header-text">
          <div className="page-title"><Icon.GitBranch /> {t("observePane.title")}</div>
          <div className="page-subtitle">{t("observePane.subtitle")}</div>
        </div>
      </div>
      <div style={{ flex: 1, minHeight: 0 }}>
        <RelGraph />
      </div>
    </div>
  );
}
