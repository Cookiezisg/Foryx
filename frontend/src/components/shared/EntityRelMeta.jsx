// EntityRelMeta — "· 与 X · Y 相关 [···]" strip for any entity header.
//
// Uses /relations/neighborhood?kind=&id=&depth=1 — this is the actual
// entity-filtered endpoint. /relations?entityId= was silently dropped by
// the backend filter, leaking arbitrary edges into every meta strip.
// Renders nothing when the entity has zero edges (孤岛不显示).
//
// EntityRelMeta —— 实体头部的引用条；neighborhood 端点按实体过滤；零关联
// 时整条不渲染。

import { useTranslation } from "react-i18next";
import { EntityLink } from "./EntityLink.jsx";
import { RelMore } from "./RelGraph.jsx";
import { useEntityNeighborhood } from "@features/entity-link";

export function EntityRelMeta({ entityId, kind, limit = 3 }) {
  const { t } = useTranslation("misc");
  const { neighbours, guessedKind } = useEntityNeighborhood(entityId, kind, limit);

  if (!entityId) return null;
  if (neighbours.length === 0) return null;

  return (
    <span style={{ fontSize: 11, color: "var(--fg-faint)", display: "inline-flex", alignItems: "center", gap: 4, flexWrap: "wrap" }}>
      <span>{t("entityRelMeta.relatedWith")}</span>
      {neighbours.map((id, i) => (
        <span key={id} style={{ display: "inline-flex", alignItems: "center", gap: 4 }}>
          <EntityLink id={id} />
          {i < neighbours.length - 1 && <span>·</span>}
        </span>
      ))}
      {t("entityRelMeta.related") && <span>{t("entityRelMeta.related")}</span>}
      <RelMore entityId={entityId} kind={guessedKind} label={t("entityRelMeta.viewRefs")} />
    </span>
  );
}
