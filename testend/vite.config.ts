import { defineConfig } from 'vite';
import vue from '@vitejs/plugin-vue';
import { fileURLToPath, URL } from 'node:url';

/**
 * Vite config for the Forgify testend SPA.
 *
 * Output:
 *   - `dist/` is served by the backend's DevHandler at /dev/static/* (and / dev/ catch-all).
 *   - `Makefile`'s `test-console` target passes `--integration-dir ../testend/dist`.
 *
 * Base path:
 *   Set to `/dev/static/` so all asset URLs in the built HTML are prefixed with
 *   `/dev/static/`. The catch-all `/dev/` route serves `dist/index.html`, which
 *   then loads `/dev/static/assets/<hashed>.js` etc.
 *
 * Dev mode proxy:
 *   `npm run dev` runs the Vite dev server on :5173 and proxies API calls back
 *   to the backend (assumed on :5174 — override with `BACKEND_PORT` env var).
 *   Useful for fast UI iteration without rebuilding the SPA every save.
 */
const BACKEND_PORT = process.env.BACKEND_PORT ? Number(process.env.BACKEND_PORT) : 5174;

export default defineConfig({
  plugins: [vue()],
  base: '/dev/static/',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    sourcemap: true,
    target: 'es2022',
    rollupOptions: {
      output: {
        // Hash JS/CSS chunks for cache busting; HTML stays at fixed path.
        // Hash 化 JS/CSS,HTML 走固定路径。
        entryFileNames: 'assets/[name]-[hash].js',
        chunkFileNames: 'assets/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash].[ext]',
      },
    },
  },
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    proxy: {
      // Backend HTTP API.
      '/api': { target: `http://localhost:${BACKEND_PORT}`, changeOrigin: false },
      // Dev endpoints (sql, info, routes, logs SSE, ...).
      '/dev': { target: `http://localhost:${BACKEND_PORT}`, changeOrigin: false },
    },
  },
});
