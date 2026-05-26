import { useNeighborhood } from "@entities/relation";

// Best-effort id prefix → entity kind mapping (closed backend enum).
export function guessKind(id: string | undefined): string {
  if (!id) return "function";
  const p = id.split("_")[0];
  return ({
    f: "function", fn: "function",
    h: "handler",  hd: "handler",
    w: "workflow", wf: "workflow",
    cv: "conversation",
    d: "document", doc: "document",
    s: "skill", sk: "skill",
    mcp: "mcp",
    m: "memory", mem: "memory",
    fr: "flowrun",
  } as Record<string, string>)[p] || "function";
}

export interface NeighborhoodResult {
  neighbours: string[];
  guessedKind: string;
}

// Wraps useNeighborhood with guessKind + dedupe + limit for the EntityRelMeta
// strip — picks the other side of each edge, dedupes, caps at `limit`.
export function useEntityNeighborhood(
  entityId: string | undefined,
  kind?: string,
  limit = 3,
): NeighborhoodResult {
  const guessedKind = kind || guessKind(entityId);
  const { data: rels = [] } = useNeighborhood({ kind: guessedKind, id: entityId!, depth: 1 });

  const neighbours: string[] = [];
  const seen = new Set([entityId]);
  for (const r of (rels as any[]) || []) {
    const otherId = r.fromId === entityId ? r.toId : r.fromId;
    if (!otherId || seen.has(otherId)) continue;
    seen.add(otherId);
    neighbours.push(otherId);
    if (neighbours.length >= limit) break;
  }

  return { neighbours, guessedKind };
}
