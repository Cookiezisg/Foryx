// Vitest config — co-located `*.test.js` / `*.test.jsx` next to
// source. Coverage is opt-in via `npm run test:coverage`. setupFiles
// stubs the browser APIs we touch but jsdom doesn't ship.

import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  esbuild: { jsx: "automatic" },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test-setup.js"],
    include: ["src/**/*.{test,spec}.{js,jsx}"],
    exclude: ["tests/**", "node_modules/**", "dist/**"],
    coverage: {
      provider: "v8",
      reporter: ["text", "html", "lcov"],
      include: ["src/**/*.{js,jsx}"],
      // Skip list — each excluded file has a documented reason:
      //   constants / tokens — no logic to test
      //   icon barrels — lucide-react re-exports
      //   composition shells — App, AppShell, main: covered by Playwright e2e
      //   placeholder — Dashboard / PlaceholderPane: no logic
      //   heavy editors — DocEditor (Tiptap), WorkflowEditor (canvas DAG),
      //     RelGraph (force layout): full integration via Playwright instead
      //   ConfigPane — large form aggregate; per-field already tested via
      //     SettingsPopover/Onboarding; full e2e covers integration
      //   trivial helpers — PaneCollapseToggle, useCollapsible: 1-line wrappers
      //   DataViewerInspector — debug-only modal, internal
      //   _testHarness — vitest helpers
      exclude: [
        "src/test-setup.js",
        "src/test-setup.test.js",
        "src/**/*.{test,spec}.{js,jsx}",
        "src/api/_testHarness.js",
        "src/main.jsx",
        "src/App.jsx",
        "src/motion/tokens.js",
        "src/components/primitives/Icon.jsx",
        "src/components/primitives/Spinner.jsx",
        "src/components/primitives/Kbd.jsx",
        "src/components/shared/lowlightInstance.js",
        "src/components/shared/RelGraph.jsx",
        "src/components/shared/PaneCollapseToggle.jsx",
        "src/components/shared/BottomSheet.jsx",
        "src/components/shared/DataViewerInspector.jsx",
        "src/components/layout/AppShell.jsx",
        "src/panes/PlaceholderPane.jsx",
        "src/panes/dashboard/Dashboard.jsx",
        "src/panes/library/DocEditor.jsx",
        "src/panes/library/CodeBlockNode.jsx",
        "src/panes/forge/WorkflowEditor.jsx",
        "src/hooks/useCollapsible.js",
      ],
      // Thresholds: v8 counts every arrow inside JSX as a separate
      // function, so even comprehensively-tested components like
      // BlockRenderer often show ~40% function coverage despite >80%
      // branch + line coverage. Functions threshold accordingly held at
      // 75 — the more meaningful gates are branches + lines.
      //
      // Threshold —— v8 把 JSX 里每个箭头都算独立函数；BlockRenderer 这种
      // 即便实测全跑过函数覆盖率也只有 ~40%。分支/行覆盖才是真信号。
      thresholds: {
        statements: 80,
        branches: 75,
        functions: 75,
        lines: 80,
      },
    },
  },
});
