// testend/src/router.tsx — hash router with 44 placeholder routes.
// Real view components land in P3 (per-section).
//
// 44 路由占位;P3 逐 section 替换为真 view。
import { createHashRouter, Navigate } from "react-router-dom";
import { App } from "./App";
import { SQL } from "@/views/dev/SQL";
import { Info } from "@/views/dev/Info";
import { Routes as DevRoutes } from "@/views/dev/Routes";
import { BackendLogs } from "@/views/dev/BackendLogs";
import { Processes } from "@/views/dev/Processes";
import { Metrics } from "@/views/dev/Metrics";
import { Errors } from "@/views/dev/Errors";
import { Prompts } from "@/views/dev/Prompts";
import { EventlogRaw } from "@/views/current/EventlogRaw";
import { WireTrace } from "@/views/current/WireTrace";
import { Notifications as CurrentNotifications } from "@/views/current/Notifications";
import { SubAgents } from "@/views/current/SubAgents";
import { ToolCalls } from "@/views/current/ToolCalls";
import { Todos } from "@/views/current/Todos";
import { AsksPending } from "@/views/current/AsksPending";
import { Attachments } from "@/views/current/Attachments";
import { Compaction } from "@/views/current/Compaction";
import { ApiKeys } from "@/views/config/ApiKeys";
import { ModelConfigs } from "@/views/config/ModelConfigs";
import { Skills } from "@/views/config/Skills";
import { MCPServers } from "@/views/config/MCPServers";
import { Sandbox } from "@/views/config/Sandbox";
import { Memory } from "@/views/config/Memory";
import { Documents } from "@/views/config/Documents";
import { Permissions } from "@/views/config/Permissions";
import { LLMHealth } from "@/views/config/LLMHealth";
import { Profile } from "@/views/config/Profile";

function Placeholder({ name }: { name: string }) {
  return <div className="empty">TODO: {name}</div>;
}

export const router = createHashRouter([
  {
    path: "/",
    element: <App />,
    children: [
      { index: true, element: <Navigate to="/forge/functions" replace /> },

      // current/ (9)
      { path: "current/wire",          element: <WireTrace /> },
      { path: "current/eventlog",      element: <EventlogRaw /> },
      { path: "current/notifications", element: <CurrentNotifications /> },
      { path: "current/subagents",     element: <SubAgents /> },
      { path: "current/tools",         element: <ToolCalls /> },
      { path: "current/todos",         element: <Todos /> },
      { path: "current/asks",          element: <AsksPending /> },
      { path: "current/attachments",   element: <Attachments /> },
      { path: "current/compaction",    element: <Compaction /> },

      // forge/ (7 — TestCollections deleted)
      { path: "forge/functions",       element: <Placeholder name="forge/Functions" /> },
      { path: "forge/functions/:id",   element: <Placeholder name="forge/FunctionDetail" /> },
      { path: "forge/handlers",        element: <Placeholder name="forge/Handlers" /> },
      { path: "forge/handlers/:id",    element: <Placeholder name="forge/HandlerDetail" /> },
      { path: "forge/workflows",       element: <Placeholder name="forge/Workflows" /> },
      { path: "forge/workflows/:id",   element: <Placeholder name="forge/WorkflowDetail" /> },
      { path: "forge/tools",           element: <Placeholder name="forge/ToolsRegistry" /> },

      // execute/ (5)
      { path: "execute/triggers",      element: <Placeholder name="execute/Triggers" /> },
      { path: "execute/flowruns",      element: <Placeholder name="execute/FlowRuns" /> },
      { path: "execute/flowruns/:id",  element: <Placeholder name="execute/FlowRunDetail" /> },
      { path: "execute/approvals",     element: <Placeholder name="execute/ApprovalsQueue" /> },
      { path: "execute/executions",    element: <Placeholder name="execute/Executions" /> },

      // observe/ (5)
      { path: "observe/live",          element: <Placeholder name="observe/LiveSSE" /> },
      { path: "observe/notifications", element: <Placeholder name="observe/NotificationHistory" /> },
      { path: "observe/catalog",       element: <Placeholder name="observe/Catalog" /> },
      { path: "observe/usage",         element: <Placeholder name="observe/Usage" /> },
      { path: "observe/mock-llm",      element: <Placeholder name="observe/MockLLM" /> },

      // config/ (10)
      { path: "config/apikeys",        element: <ApiKeys /> },
      { path: "config/models",         element: <ModelConfigs /> },
      { path: "config/skills",         element: <Skills /> },
      { path: "config/mcp",            element: <MCPServers /> },
      { path: "config/sandbox",        element: <Sandbox /> },
      { path: "config/memory",         element: <Memory /> },
      { path: "config/documents",      element: <Documents /> },
      { path: "config/permissions",    element: <Permissions /> },
      { path: "config/llm-health",     element: <LLMHealth /> },
      { path: "config/profile",        element: <Profile /> },

      // dev/ (8)
      { path: "dev/sql",               element: <SQL /> },
      { path: "dev/info",              element: <Info /> },
      { path: "dev/routes",            element: <DevRoutes /> },
      { path: "dev/logs",              element: <BackendLogs /> },
      { path: "dev/processes",         element: <Processes /> },
      { path: "dev/metrics",           element: <Metrics /> },
      { path: "dev/errors",            element: <Errors /> },
      { path: "dev/prompts",           element: <Prompts /> },

      // catch-all
      { path: "*", element: <Navigate to="/forge/functions" replace /> },
    ],
  },
]);
