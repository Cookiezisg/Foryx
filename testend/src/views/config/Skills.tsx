import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { getJSON } from "@/api/devClient";
import { qk } from "@/hooks/queryKeys";
import { EmptyView } from "@/ui";
import type { Skill } from "@frontend/entities/skill/model/types";

export function Skills() {
  const [selected, setSelected] = useState<string | null>(null);

  const list = useQuery<Skill[]>({
    queryKey: qk.skills(),
    queryFn: () => getJSON<Skill[]>("/api/v1/skills"),
  });

  const detail = useQuery<Skill>({
    queryKey: qk.skill(selected ?? ""),
    queryFn: () => getJSON<Skill>(`/api/v1/skills/${encodeURIComponent(selected!)}`),
    enabled: !!selected,
  });

  if (list.isLoading) return <EmptyView>loading…</EmptyView>;
  if (list.isError) return <EmptyView>error loading skills</EmptyView>;
  if (!list.data || list.data.length === 0) return <EmptyView>no skills loaded</EmptyView>;

  const skill = detail.data;
  const fm = skill?.frontmatter;

  return (
    <div style={{ display: "flex", height: "100%", overflow: "hidden" }}>
      {/* Left list */}
      <div style={{ width: 280, borderRight: "1px solid var(--border)", display: "flex", flexDirection: "column", overflow: "hidden" }}>
        <div style={{ padding: "8px 12px", borderBottom: "1px solid var(--border)" }}>
          <strong style={{ fontSize: 13 }}>Skills</strong>
          <span className="muted" style={{ marginLeft: 8, fontSize: 11 }}>{list.data.length}</span>
        </div>
        <div style={{ flex: 1, overflow: "auto" }}>
          {list.data.map((s) => (
            <div
              key={s.name}
              onClick={() => setSelected(s.name)}
              style={{
                padding: "8px 12px", cursor: "pointer", borderBottom: "1px solid var(--border-soft)",
                background: selected === s.name ? "var(--bg-elev)" : undefined,
              }}
            >
              <div style={{ fontSize: 13, fontWeight: 500 }}>{s.name}</div>
              <div className="muted" style={{ fontSize: 11, marginTop: 2, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                {s.description}
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Right detail */}
      <div style={{ flex: 1, overflow: "auto", padding: 16 }}>
        {!selected && <EmptyView>select a skill</EmptyView>}
        {selected && detail.isLoading && <EmptyView>loading…</EmptyView>}
        {selected && detail.isError && <EmptyView>error loading skill detail</EmptyView>}
        {skill && fm && (
          <>
            <h3 style={{ margin: "0 0 12px", fontSize: 15 }}>{skill.name}</h3>
            <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 12 }}>
              <tbody>
                <FmRow label="source" value={skill.source} />
                <FmRow label="description" value={fm.description} />
                <FmRow label="whenToUse" value={fm.whenToUse} />
                <FmRow label="allowedTools" value={fm.allowedTools?.join(", ")} />
                <FmRow label="disableModelInvocation" value={String(fm.disableModelInvocation ?? false)} />
                <FmRow label="userInvocable" value={String(fm.userInvocable ?? false)} />
                <FmRow label="paths" value={fm.paths?.join(", ")} />
                <FmRow label="agent" value={fm.agent} />
                <FmRow label="model" value={fm.model} />
                <FmRow label="effort" value={fm.effort} />
                <FmRow label="context" value={fm.context} />
                <FmRow label="arguments" value={fm.arguments?.join(", ")} />
                <FmRow label="argumentHint" value={fm.argumentHint} />
                <FmRow label="loadedAt" value={skill.loadedAt} />
              </tbody>
            </table>
            <div style={{ marginTop: 16 }}>
              <div style={{ fontSize: 11, fontWeight: 600, marginBottom: 4, color: "var(--fg-muted)", textTransform: "uppercase", letterSpacing: 1 }}>
                Body Path
              </div>
              <code style={{ fontFamily: "var(--mono)", fontSize: 11, wordBreak: "break-all", color: "var(--fg-muted)" }}>
                {skill.bodyPath}
              </code>
            </div>
          </>
        )}
      </div>
    </div>
  );
}

function FmRow({ label, value }: { label: string; value: string | undefined }) {
  if (value == null || value === "" || value === "undefined") return null;
  return (
    <tr>
      <td style={{ padding: "4px 8px 4px 0", fontWeight: 600, whiteSpace: "nowrap", width: 200, verticalAlign: "top", color: "var(--fg-muted)" }}>{label}</td>
      <td style={{ padding: "4px 0", wordBreak: "break-word" }}>{value}</td>
    </tr>
  );
}
