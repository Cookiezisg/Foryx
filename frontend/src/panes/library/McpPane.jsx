// McpPane — MCP server list with status + health history strip.
//
// McpPane —— MCP server 列表 + 健康历史。

import { useState } from "react";
import { Icon } from "../../components/primitives/Icon.jsx";
import { Button } from "../../components/primitives/Button.jsx";
import { Badge } from "../../components/primitives/Badge.jsx";
import { RelTime } from "../../components/shared/RelTime.jsx";
import { useMcpServers, useReconnectMcp, useRemoveMcp } from "../../api/library.js";
import { useUIStore } from "../../store/ui.js";

const STATUS_KIND = { ready: "success", connecting: "info", degraded: "warn", failed: "error", stopped: "muted" };

export function McpPane() {
  const { data: servers = [], isLoading } = useMcpServers();
  const reconnect = useReconnectMcp();
  const remove = useRemoveMcp();
  const pushToast = useUIStore((s) => s.pushToast);

  return (
    <div className="page">
      <div className="page-header">
        <div className="page-header-text">
          <div className="page-title"><Icon.Server /> MCP</div>
          <div className="page-subtitle">Model Context Protocol servers</div>
        </div>
        <div className="page-actions">
          <Button size="sm" variant="accent"><Icon.Plus /> 添加服务器</Button>
        </div>
      </div>

      <div className="page-body" style={{ padding: 24 }}>
        {isLoading
          ? <div className="empty"><div className="sub">加载中…</div></div>
          : servers.length === 0
            ? <div className="empty">
                <Icon.Server className="icon" />
                <div className="title">还没有 MCP server</div>
                <div className="sub">从 marketplace 安装或手工 ~/.forgify/mcp.json 配置</div>
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
                            {(s.tools?.length ?? 0)} 个 tool · {(s.totalCalls ?? 0)} calls · {(s.totalFailures ?? 0)} fail
                          </div>
                        </div>
                      </div>
                      <Badge kind={STATUS_KIND[s.status] || "muted"}>{s.status}</Badge>
                    </div>
                    {s.connectedAt && (
                      <div style={{ fontSize: 11, color: "var(--fg-muted)", marginTop: 6 }}>
                        connected <RelTime ts={s.connectedAt} />
                      </div>
                    )}
                    <div className="card-foot">
                      <span style={{ fontSize: 11, color: "var(--fg-faint)" }}>
                        连续失败 {s.consecutiveFailures ?? 0}
                      </span>
                      <div style={{ display: "flex", gap: 6 }}>
                        <Button
                          size="xs"
                          onClick={() => reconnect.mutate(s.name, {
                            onSuccess: () => pushToast({ kind: "success", title: "重连请求已发出" }),
                          })}
                        >
                          <Icon.Refresh /> 重连
                        </Button>
                        <Button
                          size="xs"
                          variant="danger"
                          onClick={() => {
                            if (confirm(`确认移除 ${s.name}?`)) {
                              remove.mutate(s.name, {
                                onSuccess: () => pushToast({ kind: "success", title: "已移除" }),
                              });
                            }
                          }}
                        >
                          <Icon.Trash /> 移除
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
