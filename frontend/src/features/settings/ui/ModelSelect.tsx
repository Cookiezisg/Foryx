import { useTranslation } from "react-i18next";
import { Select } from "@shared/ui/Select";

export function ModelSelect({ models, value, onChange, disabled }: { models: any; value: any; onChange: any; disabled?: any }) {
  const { t } = useTranslation("settings");
  return (
    <Select
      options={models}
      value={value}
      onChange={onChange}
      disabled={disabled}
      mono
      placeholder={t("model.placeholder")}
      ariaLabel={t("model.ariaLabel")}
    />
  );
}
