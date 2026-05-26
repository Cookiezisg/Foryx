// EntityLink — clickable entity-ID chip. Detects prefix → routes to the
// right pane via ui store. Used by TextBlock renderInline + anywhere we
// inline-reference an entity.
//
// EntityLink —— 实体 ID 可点击 chip；按前缀路由到对应 pane。

import React from "react";
import { useTranslation } from "react-i18next";
import { Icon } from "@shared/ui/Icon";
import { navigate } from "@shared/lib/navigation";
import { useEntityName } from "./useEntityName";

const PREFIX_META = {
  f:   { pane: "forge",     icon: "Code"          },
  fn:  { pane: "forge",     icon: "Code"          },
  h:   { pane: "forge",     icon: "Server"        },
  hd:  { pane: "forge",     icon: "Server"        },
  w:   { pane: "forge",     icon: "Workflow"      },
  wf:  { pane: "forge",     icon: "Workflow"      },
  s:   { pane: "skills",    icon: "Sparkles"      },
  sk:  { pane: "skills",    icon: "Sparkles"      },
  mcp: { pane: "mcp",       icon: "Server"        },
  m:   { pane: "memory",    icon: "Brain"         },
  mem: { pane: "memory",    icon: "Brain"         },
  cv:  { pane: "chat",      icon: "MessageSquare" },
  fr:  { pane: "execute",   icon: "Play"          },
  d:   { pane: "documents", icon: "FileText"      },
  doc: { pane: "documents", icon: "FileText"      },
};

interface EntityLinkProps {
  id: string;
}

export function EntityLink({ id }: EntityLinkProps) {
  const { t } = useTranslation("misc");
  const name = useEntityName(id);

  const prefix = id.split("_")[0];
  const meta = (PREFIX_META as Record<string, { pane: string; icon: string }>)[prefix] || { pane: "forge", icon: "Hammer" };
  const Ic = (Icon as Record<string, React.ComponentType<any>>)[meta.icon] || Icon.Hammer;
  const kindLabel = t(`entityKinds.${prefix}`, { defaultValue: prefix });

  const onClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (prefix === "cv") {
      navigate.openConv(id);
    } else {
      navigate.openEntity(meta.pane, id);
    }
  };

  const display = name || id;
  const tip = name ? `${kindLabel} · ${name} · ${id}` : `${kindLabel} · ${id}`;

  return (
    <button className="entity-link" title={tip} onClick={onClick}>
      <Ic className="icon" />
      <span>{display}</span>
    </button>
  );
}
