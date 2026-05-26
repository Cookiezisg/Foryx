import React from "react";
import ReactDOM from "react-dom/client";
import { QueryClient, QueryClientProvider, MutationCache, QueryCache } from "@tanstack/react-query";

import { initBaseUrl } from "@shared/bridge/wails";
import i18n from "@shared/lib/i18n";
import "@shared/lib/i18n";
import { ApiError } from "@shared/api/httpClient";
import { errorKey, kindForCode } from "@shared/api/errorMap";
import { useToastStore } from "@shared/ui/toastStore";
import App from "./App.tsx";
import "@/styles/tokens.css";
import "@/styles/base.css";
import "@/styles/components.css";
import "@/styles/panes.css";

function handleQueryError(error: unknown, mutation?: { options?: { meta?: { suppressGlobal?: boolean } } }) {
  // suppressGlobal — mutation opted out (e.g. cancel stream uses warn via feature).
  //
  // 跳过已声明 suppressGlobal 的 mutation（如取消流用 feature 的 warn 处理）。
  if (mutation?.options?.meta?.suppressGlobal) return;

  const code = error instanceof ApiError ? error.code : "UNKNOWN";
  const key = errorKey(code);
  const kind = kindForCode(code);
  const text = i18n.t(key);
  useToastStore.getState().pushToast({ kind, title: text });
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      refetchOnWindowFocus: false,
      retry: 2,
    },
  },
  mutationCache: new MutationCache({
    onError: (error, _variables, _context, mutation) => handleQueryError(error, mutation),
  }),
  queryCache: new QueryCache({
    onError: (error) => handleQueryError(error, undefined),
  }),
});

async function bootstrap() {
  try {
    await initBaseUrl();
  } catch (err) {
    console.error("baseUrl init failed", err);
  }

  ReactDOM.createRoot(document.getElementById("root")).render(
    <React.StrictMode>
      <QueryClientProvider client={queryClient}>
        <App />
      </QueryClientProvider>
    </React.StrictMode>
  );
}

bootstrap();
