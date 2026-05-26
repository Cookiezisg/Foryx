import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useFunctions } from "@entities/function";
import { useHandlers } from "@entities/handler";
import { useWorkflows } from "@entities/workflow";
import { useDocuments } from "@entities/document";
import { useSkills } from "@entities/skill";
import { useMcpServers } from "@entities/mcp";
import { useConversations } from "@entities/conversation";
import { useAllRelations } from "@entities/relation";

export interface EntityNode {
  id: string;
  kind: string;
  label: string;
  sub: string;
}

export interface EntityEdge {
  from: string;
  to: string;
  kind: string;
}

export interface EntityDirectory {
  nodes: EntityNode[];
  edges: EntityEdge[];
}

// Raw backend relation → normalised edge (fromId/from, toId/to, kind/type).
// Filters out malformed entries without from/to.
export function normEdges(relations: unknown[]): EntityEdge[] {
  return (relations || []).map((r: any) => ({
    from: r.fromId || r.from,
    to: r.toId || r.to,
    kind: r.kind || r.type,
  })).filter((e) => e.from && e.to) as EntityEdge[];
}

// Aggregates all 7 entity list queries into a flat node list plus normalised
// edges from useAllRelations — mirrors RelGraph's local useEntityDirectory +
// normEdges for consumption by the force-directed graph.
export function useEntityDirectory(): EntityDirectory {
  const fnQ = useFunctions();
  const hdQ = useHandlers();
  const wfQ = useWorkflows();
  const dcQ = useDocuments();
  const skQ = useSkills();
  const mcQ = useMcpServers();
  const cvQ = useConversations();
  const { data: rawRel = [] } = useAllRelations();

  const { t } = useTranslation("misc");

  const nodes = useMemo<EntityNode[]>(() => {
    const out: EntityNode[] = [];
    for (const x of (fnQ.data as any[] || [])) out.push({ id: x.id, kind: "function",  label: x.name || x.id, sub: x.description || x.desc || "" });
    for (const x of (hdQ.data as any[] || [])) out.push({ id: x.id, kind: "handler",   label: x.name || x.id, sub: x.description || x.desc || "" });
    for (const x of (wfQ.data as any[] || [])) out.push({ id: x.id, kind: "workflow",  label: x.name || x.id, sub: x.description || x.desc || "" });
    for (const x of (dcQ.data as any[] || [])) out.push({ id: x.id, kind: "document",  label: x.name || x.title || x.id, sub: t("relGraph.subDocument") });
    for (const x of (skQ.data as any[] || [])) out.push({ id: x.id, kind: "skill",     label: x.name || x.id, sub: x.description || "" });
    for (const x of (mcQ.data as any[] || [])) out.push({ id: x.id, kind: "mcp",       label: x.name || x.id, sub: t("relGraph.subTools", { count: x.tools?.length || x.tools || 0 }) });
    for (const x of (cvQ.data as any[] || [])) out.push({ id: x.id, kind: "conversation", label: x.title || x.id, sub: x.model || "" });
    return out;
  }, [fnQ.data, hdQ.data, wfQ.data, dcQ.data, skQ.data, mcQ.data, cvQ.data, t]);

  const edges = useMemo<EntityEdge[]>(() => normEdges(rawRel as unknown[]), [rawRel]);

  return { nodes, edges };
}
