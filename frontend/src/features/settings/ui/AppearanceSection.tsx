import { useTranslation } from "react-i18next";
import { Icon } from "@shared/ui/Icon";
import { useSettingsStore } from "@entities/settings";
import { ACCENTS } from "@shared/lib/onboarding-strings";

export function AppearanceSection({ open, onToggle }) {
  const { t } = useTranslation("settings");
  const settings = useSettingsStore();
  return (
    <div className="set-sec">
      <button className="set-sec-h" onClick={onToggle}>
        <Icon.Brush className="set-sec-ic icon" />
        <div className="set-sec-tt">
          <div className="set-sec-t1">{t("appearance.title")}</div>
          <div className="set-sec-t2">{t("appearance.subtitle")}</div>
        </div>
        <Icon.ChevronRight
          className={"set-sec-chev icon" + (open ? " is-open" : "")}
        />
      </button>
      {open && (
        <div className="set-sec-p">
          <div className="set-look-row">
            <div className="set-look-k">{t("appearance.theme")}</div>
            <div className="onb-seg">
              {["light", "dark", "system"].map((v) => (
                <button
                  key={v}
                  className={"onb-seg-opt" + (settings.theme === v ? " is-active" : "")}
                  onClick={() => (settings as any).set({ theme: v })}
                >
                  {t(`appearance.themeOpts.${v}`)}
                </button>
              ))}
            </div>
          </div>
          <div className="set-look-row">
            <div className="set-look-k">{t("appearance.accent")}</div>
            <div className="onb-swatches">
              {ACCENTS.map(([k, c]) => (
                <button
                  key={k}
                  className={"onb-swatch" + (settings.accent === k ? " is-active" : "")}
                  style={{ background: c }}
                  onClick={() => (settings as any).set({ accent: k })}
                />
              ))}
            </div>
          </div>
          <div className="set-look-row">
            <div className="set-look-k">{t("appearance.density")}</div>
            <div className="onb-seg">
              {["compact", "cozy", "comfortable"].map((v) => (
                <button
                  key={v}
                  className={"onb-seg-opt" + (settings.density === v ? " is-active" : "")}
                  onClick={() => (settings as any).set({ density: v })}
                >
                  {t(`appearance.densityOpts.${v}`)}
                </button>
              ))}
            </div>
          </div>
          <div className="set-look-row">
            <div className="set-look-k">{t("appearance.language")}</div>
            <div className="onb-seg">
              {[["zh", "中文"], ["en", "English"]].map(([v, label]) => (
                <button
                  key={v}
                  className={"onb-seg-opt" + (settings.lang === v ? " is-active" : "")}
                  onClick={() => (settings as any).set({ lang: v })}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>
          <div className="set-look-row">
            <div className="set-look-k">{t("appearance.reasoning")}</div>
            <div className="onb-seg">
              {["collapsed", "expanded"].map((v) => (
                <button
                  key={v}
                  className={"onb-seg-opt" + (settings.reasoningDefault === v ? " is-active" : "")}
                  onClick={() => (settings as any).set({ reasoningDefault: v })}
                >
                  {t(`appearance.reasoningOpts.${v}`)}
                </button>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
