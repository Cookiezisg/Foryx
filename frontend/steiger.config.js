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
];
