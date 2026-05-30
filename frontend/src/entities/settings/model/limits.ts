// entities/settings — operational limits (backend behavior config: the
// settings.json "limits" block). Distinct from settingsStore (frontend prefs);
// edited via the "advanced capabilities" section → PUT /settings/limits.
//
// 运行上限（后端行为配置：settings.json "limits" 块）。区别于 settingsStore
// （前端偏好）；经「高级能力」区编辑 → PUT /settings/limits。

export interface Limits {
  agent: {
    maxSteps: number;
    maxTurnDurationSec: number;
    subagentTimeoutSec: number;
    subagentMaxTurns: number;
  };
  output: {
    unknownModelMaxTokens: number;
    perScenarioOverride?: Record<string, number>;
  };
  context: { softRatio: number; hardRatio: number };
  timeout: { llmIdleSec: number; mcpCallSec: number; bashDefaultTimeoutSec: number };
  tools: { searchTopN: number; readDefaultLines: number; bashOutputCapKB: number };
  workflow: { agentNodeMaxTurns: number; agentNodeMaxTurnsHard: number };
  guards: { attachmentMaxMB: number; httpNodeRespMaxMB: number; webhookBodyMaxMB: number };
}

// DEFAULT_LIMITS mirrors backend limits.Default() — used by "restore defaults".
// Keep in sync with internal/pkg/limits.Default.
//
// DEFAULT_LIMITS 镜像后端 limits.Default()，供「恢复默认」用，与之保持同步。
export const DEFAULT_LIMITS: Limits = {
  agent: { maxSteps: 150, maxTurnDurationSec: 1800, subagentTimeoutSec: 600, subagentMaxTurns: 30 },
  output: { unknownModelMaxTokens: 64000 },
  context: { softRatio: 0.7, hardRatio: 0.85 },
  timeout: { llmIdleSec: 150, mcpCallSec: 180, bashDefaultTimeoutSec: 120 },
  tools: { searchTopN: 10, readDefaultLines: 2000, bashOutputCapKB: 256 },
  workflow: { agentNodeMaxTurns: 10, agentNodeMaxTurnsHard: 50 },
  guards: { attachmentMaxMB: 50, httpNodeRespMaxMB: 10, webhookBodyMaxMB: 10 },
};
