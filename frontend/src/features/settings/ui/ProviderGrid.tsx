import { Icon } from "@shared/ui/Icon";

export function ProviderGrid({ providers, hints, selected, onPick, configured = [], tall = false }) {
  return (
    <div className="onb-gridwrap">
      <div className={"onb-grid" + (tall ? " is-tall" : "")}>
        {providers.map((p) => {
          const h = hints[p.name] || { abbr: p.name.slice(0, 2).toUpperCase(), color: "#6b6459" };
          return (
            <button key={p.name} type="button"
              className={"onb-prov" + (selected === p.name ? " is-active" : "")}
              onClick={() => onPick(p.name)}>
              <span className="onb-pchip" style={{ background: h.color }}>{h.abbr}</span>
              <span style={{ minWidth: 0 }}>
                <span className="onb-pname">{p.displayName || p.name}</span>
                <span className="onb-pdesc" style={{ display: "block" }}>{p.defaultBaseUrl?.replace(/^https?:\/\//, "") || ""}</span>
              </span>
              {configured.includes(p.name) && <span className="onb-prov-ck"><Icon.Check /></span>}
            </button>
          );
        })}
      </div>
      {!tall && <div className="onb-grid-fade" />}
    </div>
  );
}
