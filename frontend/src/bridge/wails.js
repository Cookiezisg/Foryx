// Re-export shim — 实现已迁至 shared/bridge/wails.ts（阶段1 FSD 定型）。
// 现有 import 路径 "../bridge/wails" / "../../bridge/wails" 无需修改。
export { initBaseUrl, getBaseUrl, apiUrl } from "@shared/bridge/wails";
