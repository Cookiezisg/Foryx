// i18n — react-i18next 配置。import 即 init(side-effect)。lng 初值取已
// hydrate 的 settings.lang;切换由 App 的 effect 调 changeLanguage 驱动。

import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import { resources } from "./resources.js";
// TODO(阶段4): settings 下沉 shared 或由 app 注入 lang 后移除此豁免
// eslint-disable-next-line boundaries/dependencies
import { useSettings } from "../../../store/settings.js";

i18n.use(initReactI18next).init({
  resources,
  lng: useSettings.getState().lang,
  fallbackLng: "zh",
  defaultNS: "common",
  interpolation: { escapeValue: false },
  returnNull: false,
});

export default i18n;
