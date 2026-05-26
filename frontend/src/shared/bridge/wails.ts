// Wails bridge — exposes a single GetBackendPort call. Falls back to the
// Vite dev proxy (/api) when Wails runtime is absent (standalone `npm run
// dev` in a browser).
//
// Wails 桥接 —— 只暴露 GetBackendPort。Wails runtime 不在时（独立 `npm run
// dev` 浏览器调试）回退到 Vite dev proxy（/api 自动反代后端）。

declare global {
  interface Window {
    go?: {
      main?: {
        App?: {
          GetBackendPort: () => Promise<number>;
        };
      };
    };
  }
}

const VITE_DEV_FALLBACK = "/api";

let _baseUrl: string | null = null;

export async function initBaseUrl(): Promise<string> {
  if (typeof window !== "undefined" && window.go?.main?.App?.GetBackendPort) {
    const port = await window.go.main.App.GetBackendPort();
    _baseUrl = `http://localhost:${port}`;
    return _baseUrl;
  }
  _baseUrl = ""; // empty → relative path → Vite proxy → backend
  return _baseUrl;
}

export function getBaseUrl(): string {
  if (_baseUrl === null) {
    throw new Error("baseUrl not initialized; call initBaseUrl() first");
  }
  return _baseUrl;
}

// apiUrl(path) — `path` starts with `/api/v1/...`. Returns absolute URL in
// Wails mode, relative URL in browser dev mode (so Vite proxy intercepts).
export function apiUrl(path: string): string {
  const base = getBaseUrl();
  if (base === "") return path; // browser dev → relative → Vite proxy
  return base + path;
}
