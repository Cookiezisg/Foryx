// Steiger FSD linter config — 阶段1:只有 src/shared 已完成 FSD 化。
// 其余目录(components/store/api/sse/hooks/panes 等)是迁移期扁平结构,
// 阶段2-5 逐步搬迁;在此之前临时 ignore 以避免无意义噪音。
// steiger 全绿是阶段5的终态目标,不是阶段1的要求。

import fsd from "@feature-sliced/steiger-plugin";

export default [
  ...fsd.configs.recommended,
  {
    // 忽略尚未迁移到 FSD 的扁平目录
    ignores: [
      "src/api/**",
      "src/sse/**",
      "src/store/**",
      "src/hooks/**",
      "src/motion/**",
      "src/i18n/**",
      "src/bridge/**",
      "src/components/**",
      "src/panes/**",
      "src/App.jsx",
      "src/main.jsx",
    ],
  },
  {
    // insignificant-slice は阶段2迁移期间暂时无引用;
    // 调用点仍在 shared-tmp(api/config.js re-export),steiger 无法跨层追踪。
    // 阶段5调用点全量迁移后移除此豁免。
    files: ["src/entities/**"],
    rules: { "fsd/insignificant-slice": "off" },
  },
  {
    // features 阶段3:slice 只含 model 段,steiger insignificant-slice 会触发;
    // 调用点仍在 panes(feature-tmp),阶段4迁入 pages/widgets 后移除此豁免。
    files: ["src/features/**"],
    rules: { "fsd/insignificant-slice": "off" },
  },
];
