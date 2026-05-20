// Backend test helpers — REST seeding for tests that need data.
//
// 后端测试辅助：测试需要数据时通过 REST 种子。

const BACKEND_URL = process.env.BACKEND_URL || "http://localhost:8742";
const DEEPSEEK_KEY = process.env.DEEPSEEK_KEY || "";

async function api(path, opts = {}) {
  const res = await fetch(BACKEND_URL + "/api/v1" + path, {
    method: opts.method || "GET",
    headers: { "Content-Type": "application/json", ...(opts.headers || {}) },
    body: opts.body ? JSON.stringify(opts.body) : undefined,
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`${opts.method || "GET"} ${path} → ${res.status}: ${text.slice(0, 200)}`);
  }
  if (res.status === 204) return null;
  const j = await res.json();
  return j.data ?? j;
}

export const backend = {
  health:      () => api("/health"),
  users:       () => api("/users"),
  conversations: () => api("/conversations"),
  createConv:  (title) => api("/conversations", { method: "POST", body: { title } }),
  sendMsg:     (id, content) => api(`/conversations/${id}/messages`, { method: "POST", body: { content } }),
  apiKeys:     () => api("/api-keys"),
  addKey:      (provider, key, displayName) =>
    api("/api-keys", { method: "POST", body: { provider, key, displayName: displayName || provider } }),
  testKey:     (id) => api(`/api-keys/${id}:test`, { method: "POST" }),
  setModel:    (scenario, provider, modelId) =>
    api(`/model-configs/${scenario}`, { method: "PUT", body: { provider, modelId } }),
  hasDeepseekKey: () => !!DEEPSEEK_KEY,
  deepseekKey: () => DEEPSEEK_KEY,
};
