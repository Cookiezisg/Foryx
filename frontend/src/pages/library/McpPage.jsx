// McpPage — MCP server list with status + health history strip.
//
// McpPage —— MCP server 列表 + 健康历史。

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Icon } from "../../components/primitives/Icon.jsx";
import { Button } from "../../components/primitives/Button.jsx";
import { Badge } from "../../components/primitives/Badge.jsx";
import { RelTime } from "../../shared/ui/RelTime.jsx";
import { useMcpServers, useReconnectMcp, useRemoveMcp } from "../../api/library.js";
import { useToastStore } from "@shared/ui/toastStore";

const STATUS_KIND = { ready: "success", connecting: "info", degraded: "warn", failed: "error", stopped: "muted" };

export function McpPage() {
  const { t } = useTranslation(["library", "common"]);
  const { data: servers = [], isLoading } = useMcpServers();
  const reconnect = useReconnectMcp();
  const remove = useRemoveMcp();
  const pushToast = useToastStore((s) => s.pushToast);

  return (
    <div className="page">
      <div className="page-header">
        <div className="page-header-text">
          <div className="page-title"><Icon.Server /> MCP</div>
          <div className="page-subtitle">{t("mcp.subtitle")}</div>
        </div>
        <div className="page-actions">
          <Button size="sm" variant="accent"><Icon.Plus /> {t("mcp.addServer")}</Button>
        </div>
      </div>

      <div className="page-body" style={{ padding: 24 }}>
        {isLoading
          ? <div className="empty"><div className="sub">{t("common:loading")}</div></div>
          : servers.length === 0
            ? <div className="empty">
                <Icon.Server className="icon" />
                <div className="title">{t("mcp.emptyTitle")}</div>
                <div className="sub">{t("mcp.emptySub")}</div>
              </div>
            : <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
                {servers.map((s) => (
                  <div key={s.name} className="card" style={{ cursor: "default" }}>
                    <div className="card-head">
                      <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                        <div style={{
                          width: 28, height: 28, borderRadius: 6, background: "var(--bg-elev-2)",
                          border: "1px solid var(--border-soft)",
                          display: "grid", placeItems: "center", color: "var(--fg-muted)",
                        }}>
                          <Icon.Server style={{ width: 14, height: 14 }} />
                        </div>
                        <div>
                          <div className="card-title">{s.name}</div>
                          <div style={{ fontSize: 11, color: "var(--fg-faint)" }}>
                            {t("mcp.toolStats", { tools: s.tools?.length ?? 0, calls: s.totalCalls ?? 0, failures: s.totalFailures ?? 0 })}
                          </div>
                        </div>
                      </div>
                      <Badge kind={STATUS_KIND[s.status] || "muted"}>{s.status}</Badge>
                    </div>
                    {s.connectedAt && (
                      <div style={{ fontSize: 11, color: "var(--fg-muted)", marginTop: 6 }}>
                        {t("mcp.connected")} <RelTime ts={s.connectedAt} />
                      </div>
                    )}
                    <div className="card-foot">
                      <span style={{ fontSize: 11, color: "var(--fg-faint)" }}>
                        {t("mcp.consecutiveFailures", { count: s.consecutiveFailures ?? 0 })}
                      </span>
                      <div style={{ display: "flex", gap: 6 }}>
                        <Button
                          size="xs"
                          onClick={() => reconnect.mutate(s.name, {
                            onSuccess: () => pushToast({ kind: "success", title: t("mcp.reconnectSuccess") }),
                          })}
                        >
                          <Icon.Refresh /> {t("mcp.reconnect")}
                        </Button>
                        <Button
                          size="xs"
                          variant="danger"
                          onClick={() => {
                            if (confirm(t("mcp.removeConfirm", { name: s.name }))) {
                              remove.mutate(s.name, {
                                onSuccess: () => pushToast({ kind: "success", title: t("mcp.removeSuccess") }),
                              });
                            }
                          }}
                        >
                          <Icon.Trash /> {t("mcp.remove")}
                        </Button>
                      </div>
                    </div>
                  </div>
                ))}
              </div>}
      </div>
    </div>
  );
}
