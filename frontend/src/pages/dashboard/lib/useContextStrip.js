// useContextStrip — adaptive single-line status hint for the welcome page.
// Priority: waiting_approval > failed > running > recent conv (<24h).
//
// useContextStrip —— 欢迎页底下自适应一行;按 P1>P2>P3>P4 优先级取最重要的;
// 都没就返 null,整行隐藏。

import { useTranslation } from "react-i18next";
import { useFlowRuns } from "../../../api/flowruns.js";
import { useConversations } from "../../../api/conversations.js";

const DAY_MS = 24 * 60 * 60 * 1000;

export function useContextStrip() {
  const { t } = useTranslation("dashboard");
  const { data: flowruns = [] } = useFlowRuns();
  const { data: convs = [] } = useConversations();

  const waiting = flowruns.filter((f) => f.status === "waiting_approval");
  if (waiting.length > 0) {
    return {
      kind: "waiting",
      payload: { count: waiting.length, flowName: waiting[0].workflow || waiting[0].workflowId, flowRunId: waiting[0].id },
    };
  }

  const failed = flowruns.filter((f) => f.status === "failed");
  if (failed.length > 0) {
    return { kind: "failed", payload: { count: failed.length } };
  }

  const running = flowruns.filter((f) => f.status === "running");
  if (running.length > 0) {
    const latest = running.reduce((a, b) =>
      new Date(a.startedAt) > new Date(b.startedAt) ? a : b
    );
    return {
      kind: "running",
      payload: { count: running.length, latestStartedAt: latest.startedAt },
    };
  }

  const now = Date.now();
  const recent = convs
    .filter((c) => c.updatedAt && now - new Date(c.updatedAt).getTime() < DAY_MS)
    .sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt));
  if (recent.length > 0) {
    return {
      kind: "recent",
      payload: { convId: recent[0].id, convTitle: recent[0].title || t("contextStrip.untitled"), updatedAt: recent[0].updatedAt },
    };
  }

  return null;
}
