import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Wails dev mode reads frontend:dev:serverUrl from wails.json and proxies
// requests through itself. For standalone `npm run dev` (browser testing),
// /api proxies to the backend HTTP server on a fixed port.
//
// Wails dev 模式经 frontend:dev:serverUrl 取页面；独立 `npm run dev`（浏览器
// 调试）时 /api 反代到后端 HTTP server。
const BACKEND_DEV_PORT = Number(process.env.FORGIFY_BACKEND_PORT) || 7788;

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      "/api": {
        target: `http://localhost:${BACKEND_DEV_PORT}`,
        changeOrigin: false,
      },
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    target: "es2020",
    sourcemap: false,
  },
  esbuild: {
    jsx: "automatic",
  },
});
