import js from "@eslint/js";
import globals from "globals";
import tseslint from "typescript-eslint";
import reactHooks from "eslint-plugin-react-hooks";
import boundaries from "eslint-plugin-boundaries";

export default tseslint.config(
  { ignores: ["dist", "coverage", "node_modules", "**/*.test.{js,jsx,ts,tsx}"] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ["src/**/*.{js,jsx,ts,tsx}"],
    languageOptions: { globals: { ...globals.browser } },
    plugins: { "react-hooks": reactHooks, boundaries },
    settings: {
      "boundaries/elements": [
        // 阶段1已定型的 FSD shared 层(正式 element)
        { type: "shared", pattern: "src/shared/**" },
        // 迁移期临时 element:现有扁平目录;bridge 已迁至 shared,从此排除
        { type: "shared-tmp", pattern: "src/{api,sse,store,hooks,motion,i18n,components/primitives}/**" },
        { type: "app-tmp",    pattern: "src/{App.jsx,main.jsx}" },
        { type: "feature-tmp", pattern: "src/{panes,components/{overlays,config,shared,layout}}/**" }
      ]
    },
    rules: {
      // Downgrade all react-hooks recommended rules to "warn" for migration baseline.
      // Phase 0 goal: quantity the violations, not block the build.
      ...Object.fromEntries(
        Object.entries(reactHooks.configs.recommended.rules).map(([k, v]) => [
          k,
          Array.isArray(v) ? ["warn", ...v.slice(1)] : "warn",
        ])
      ),
      "boundaries/dependencies": ["error", {
        default: "allow",
        rules: [
          // shared 层强制:不得依赖任何上层应用代码(error 级)
          { from: { type: "shared" }, disallow: { to: { type: ["shared-tmp", "feature-tmp", "app-tmp"] } }, message: "shared 不能依赖上层或未迁移应用代码" },
          // 迁移期:旧扁平层 shared-tmp 不得依赖上层(warn)
          { from: { type: "shared-tmp" }, disallow: { to: { type: ["feature-tmp", "app-tmp"] } }, message: "shared 不能依赖上层" }
        ]
      }],
      "no-unused-vars": "off",
      "@typescript-eslint/no-unused-vars": "off",
      "@typescript-eslint/no-explicit-any": "off",
      // Downgrade js/ts recommended rules that would cause exit 1 during migration baseline.
      "no-undef": "warn",
      "no-empty": "warn",
      "@typescript-eslint/no-require-imports": "warn"
    }
  }
);
