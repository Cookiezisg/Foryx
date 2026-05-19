// Root component — Phase 0 stub. Real shell lands in Phase 2.
//
// 根组件 —— Phase 0 占位；真正的 shell 在 Phase 2 落地。

import { useEffect, useState } from "react";
import { getBaseUrl } from "./bridge/wails";

export default function App() {
  const [health, setHealth] = useState("checking…");

  useEffect(() => {
    const base = getBaseUrl();
    fetch(base + "/api/v1/health")
      .then((r) => r.json())
      .then((j) => setHealth(`ok · ${JSON.stringify(j.data ?? j)}`))
      .catch((e) => setHealth(`failed · ${e.message}`));
  }, []);

  return (
    <div
      style={{
        height: "100vh",
        display: "grid",
        placeItems: "center",
        fontFamily: "var(--font-sans, -apple-system, sans-serif)",
        color: "var(--fg-strong, #37352f)",
        background: "var(--bg-window, #ffffff)",
      }}
    >
      <div style={{ textAlign: "center" }}>
        <div style={{ fontSize: 22, fontWeight: 600, marginBottom: 12 }}>Forgify</div>
        <div style={{ fontSize: 12, color: "var(--fg-muted, #9b9a97)" }}>
          frontend bootstrap · backend {health}
        </div>
      </div>
    </div>
  );
}
