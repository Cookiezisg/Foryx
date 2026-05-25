// EntityLink — clickable entity-ID chip. Detects prefix → routes to the
// right pane via ui store. Used by TextBlock renderInline + anywhere we
// inline-reference an entity.
//
// EntityLink —— 实体 ID 可点击 chip；按前缀路由到对应 pane。

import { useTranslation } from "react-i18next";
import { Icon } from "../primitives/Icon.jsx";
import { useUIStore } from "../../store/ui.js";
import { useEntityName } from "../../hooks/useEntityName.js";

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

export function EntityLink({ id }) {
  const { t } = useTranslation("misc");
  const openEntity = useUIStore((s) => s.openEntity);
  const setActiveConv = useUIStore((s) => s.setActiveConv);
  const openPane = useUIStore((s) => s.openPane);
  const name = useEntityName(id);

  const prefix = id.split("_")[0];
  const meta = PREFIX_META[prefix] || { pane: "forge", icon: "Hammer" };
  const Ic = Icon[meta.icon] || Icon.Hammer;
  const kindLabel = t(`entityKinds.${prefix}`, { defaultValue: prefix });

  const onClick = (e) => {
    e.stopPropagation();
    if (prefix === "cv") {
      setActiveConv(id);
      openPane("chat");
    } else {
      openEntity(meta.pane, id);
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
