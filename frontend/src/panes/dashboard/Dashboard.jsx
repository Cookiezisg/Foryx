// Dashboard — landing screen. Real flowruns + conversations data via
// TanStack Query; KPI cells animate into focus when relevant runs exist.
//
// Dashboard —— 着陆页；真实 flowruns + conversations；KPI 有运行时高亮。

import { Icon } from "../../components/primitives/Icon.jsx";
import { RelTime } from "../../components/shared/RelTime.jsx";
import { useUIStore } from "../../store/ui.js";
import { useFlowRuns } from "../../api/flowruns.js";
import { useConversations } from "../../api/conversations.js";

function greeting() {
  const h = new Date().getHours();
  if (h < 6) return "凌晨好";
  if (h < 11) return "早上好";
  if (h < 14) return "中午好";
  if (h < 18) return "下午好";
  return "晚上好";
}

export function Dashboard() {
  const openPane = useUIStore((s) => s.openPane);
  const setActiveConv = useUIStore((s) => s.setActiveConv);
  const togglePane = useUIStore((s) => s.togglePane);

  const { data: flowruns = [] } = useFlowRuns();
  const { data: conversations = [] } = useConversations();

  const running   = flowruns.filter((f) => f.status === "running");
  const waiting   = flowruns.filter((f) => f.status === "waiting_approval");
  const failed    = flowruns.filter((f) => f.status === "failed");
  const completed = flowruns.filter((f) => f.status === "completed");
  const todayCount = completed.length + running.length + failed.length + waiting.length;
  const successRate = todayCount === 0 ? 0 : Math.round((completed.length / todayCount) * 100);

  const recentConvs = conversations.slice(0, 4);

  return (
    <div className="dash">
      <div className="dash-inner">
        <div className="dash-greeting">
          <div className="dash-greet-text">{greeting()}</div>
          <div className="dash-greet-sub">
            {new Date().toLocaleDateString("zh-CN", {
              weekday: "long", month: "long", day: "numeric",
            })}
          </div>
        </div>

        <div className="dash-kpis">
          <div className="dash-kpi" onClick={() => openPane("execute")}>
            <div className="dash-kpi-num">{todayCount}</div>
            <div className="dash-kpi-label">运行总数</div>
            <div className="dash-kpi-sub">{successRate}% 成功率</div>
          </div>
          <div className={"dash-kpi" + (running.length ? " is-active" : "")} onClick={() => openPane("execute")}>
            <div className="dash-kpi-num">{running.length}</div>
            <div className="dash-kpi-label">运行中</div>
            <div className="dash-kpi-sub">
              {running.length ? (running[0].workflow || running[0].workflowId) : "没有正在跑的"}
            </div>
          </div>
          <div className={"dash-kpi" + (waiting.length ? " is-warn" : "")} onClick={() => openPane("execute")}>
            <div className="dash-kpi-num">{waiting.length}</div>
            <div className="dash-kpi-label">待批准</div>
            <div className="dash-kpi-sub">
              {waiting.length
                ? <>{waiting[0].workflow} · <RelTime ts={waiting[0].startedAt} /></>
                : "无"}
            </div>
          </div>
          <div className={"dash-kpi" + (failed.length ? " is-error" : "")} onClick={() => openPane("execute")}>
            <div className="dash-kpi-num">{failed.length}</div>
            <div className="dash-kpi-label">需关注</div>
            <div className="dash-kpi-sub">
              {failed.length
                ? <>{failed[0].workflow} · <RelTime ts={failed[0].startedAt} /></>
                : "无"}
            </div>
          </div>
        </div>

        <div className="dash-grid-2">
          <div className="dash-section">
            <div className="dash-section-head">
              <Icon.MessageSquare style={{ width: 14, height: 14 }} />
              <span>继续对话</span>
            </div>
            <div className="dash-conv-list">
              {recentConvs.length === 0 && (
                <div style={{ padding: 16, fontSize: 12, color: "var(--fg-faint)" }}>
                  还没有对话历史
                </div>
              )}
              {recentConvs.map((c) => (
                <button
                  key={c.id}
                  className="dash-conv"
                  onClick={() => { setActiveConv(c.id); togglePane("chat"); }}
                >
                  <div className="dash-conv-title">{c.title || "(无标题)"}</div>
                  <div className="dash-conv-sub">
                    <RelTime ts={c.updatedAt} /> · {c.model || ""}
                  </div>
                </button>
              ))}
            </div>
          </div>

          <div className="dash-section">
            <div className="dash-section-head">
              <Icon.Sparkles style={{ width: 14, height: 14, color: "var(--accent)" }} />
              <span>开始新的</span>
            </div>
            <div className="dash-quick-list">
              <button className="dash-quick" onClick={() => openPane("chat")}>
                <Icon.Plus /> <span>新对话</span>
              </button>
              <button className="dash-quick" onClick={() => openPane("forge")}>
                <Icon.Hammer /> <span>造个 function / handler / workflow</span>
              </button>
              <button className="dash-quick" onClick={() => openPane("documents")}>
                <Icon.FileText /> <span>新文档</span>
              </button>
              <button className="dash-quick" onClick={() => openPane("skills")}>
                <Icon.Sparkles /> <span>导入 skill</span>
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
