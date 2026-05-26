// Config-related hooks — api-keys / providers / scenarios / model-configs.
//
// 设置相关 hooks。

// apikey hooks 已迁移至 entities/apikey (FSD 阶段2);此处转 re-export 保持调用点零改。
export { useApiKeys, useCreateApiKey, useUpdateApiKey, useDeleteApiKey, useTestApiKey } from "@entities/apikey";

// model-config / providers / scenarios hooks 已迁移至 entities/model-config (FSD 阶段2);此处转 re-export 保持调用点零改。
export { useModelConfigs, useUpsertModelConfig, useProviders, useScenarios } from "@entities/model-config";
