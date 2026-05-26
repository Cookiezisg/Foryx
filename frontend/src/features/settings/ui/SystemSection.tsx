import { useTranslation } from "react-i18next";
import { Icon } from "@shared/ui/Icon";

export function SystemSection({ open, onToggle }) {
  const { t } = useTranslation("settings");
  return (
    <div className="set-sec">
      <button className="set-sec-h" onClick={onToggle}>
        <Icon.Server className="set-sec-ic icon" />
        <div className="set-sec-tt">
          <div className="set-sec-t1">{t("system.title")}</div>
          <div className="set-sec-t2">{t("system.subtitle")}</div>
        </div>
        <Icon.ChevronRight
          className={"set-sec-chev icon" + (open ? " is-open" : "")}
        />
      </button>
      {open && (
        <div className="set-sec-p">
          <div className="set-sys-row">
            <div className="set-sys-k">{t("system.dataDir")}</div>
            <div>
              <span className="set-sys-mono">~/.forgify/</span>
              <span className="set-sys-hint">{t("system.dataDirHint")}</span>
            </div>
          </div>
          <div className="set-sys-row">
            <div className="set-sys-k">{t("system.sandbox")}</div>
            <div>
              <span className="set-sys-mono">mise</span>
              {" "}
              <span className="badge success" style={{ verticalAlign: "middle" }}>{t("system.sandboxBadge")}</span>
              <span className="set-sys-hint">{t("system.sandboxHint")}</span>
            </div>
          </div>
          <div className="set-sys-row">
            <div className="set-sys-k">{t("system.version")}</div>
            <div>
              <span className="set-sys-mono">Forgify v1.2</span>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
