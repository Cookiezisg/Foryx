// ModelConfig entity types — mirrors backend domain/model/model.go fields,
// camelCase per API response contract (json tags on the Go struct).
//
// 对齐后端 domain/model ModelConfig struct 的 json tag 字段名(camelCase)。

export interface ModelConfig {
  id: string;
  scenario: string;
  provider: string;
  modelId: string;
  createdAt: string;
  updatedAt: string;
}

// Provider entry from GET /api/v1/providers — static whitelist from the
// apikey registry; used by model-config UI to populate provider dropdowns.
//
// GET /api/v1/providers 返回的 provider 白名单条目;用于 model-config UI 的下拉。
export interface Provider {
  name: string;
  displayName: string;
  category: string;
  defaultBaseUrl?: string;
  baseUrlRequired: boolean;
}

// Scenario entry from GET /api/v1/scenarios — backend authoritative whitelist.
//
// GET /api/v1/scenarios 返回的后端权威 scenario 白名单条目。
export interface Scenario {
  name: string;
}

export interface UpsertModelConfigBody {
  provider: string;
  modelId: string;
}
