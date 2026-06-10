package bootstrap

import (
	"net/http"

	"go.uber.org/zap"

	agentapp "github.com/sunweilin/forgify/backend/internal/app/agent"
	aispawnapp "github.com/sunweilin/forgify/backend/internal/app/aispawn"
	apikeyapp "github.com/sunweilin/forgify/backend/internal/app/apikey"
	approvalapp "github.com/sunweilin/forgify/backend/internal/app/approval"
	attachmentapp "github.com/sunweilin/forgify/backend/internal/app/attachment"
	catalogapp "github.com/sunweilin/forgify/backend/internal/app/catalog"
	chatapp "github.com/sunweilin/forgify/backend/internal/app/chat"
	contextmgrapp "github.com/sunweilin/forgify/backend/internal/app/contextmgr"
	controlapp "github.com/sunweilin/forgify/backend/internal/app/control"
	conversationapp "github.com/sunweilin/forgify/backend/internal/app/conversation"
	documentapp "github.com/sunweilin/forgify/backend/internal/app/document"
	envfixapp "github.com/sunweilin/forgify/backend/internal/app/envfix"
	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	mcpapp "github.com/sunweilin/forgify/backend/internal/app/mcp"
	memoryapp "github.com/sunweilin/forgify/backend/internal/app/memory"
	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	notificationapp "github.com/sunweilin/forgify/backend/internal/app/notification"
	relationapp "github.com/sunweilin/forgify/backend/internal/app/relation"
	sandboxapp "github.com/sunweilin/forgify/backend/internal/app/sandbox"
	schedulerapp "github.com/sunweilin/forgify/backend/internal/app/scheduler"
	skillapp "github.com/sunweilin/forgify/backend/internal/app/skill"
	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	todoapp "github.com/sunweilin/forgify/backend/internal/app/todo"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	agenttool "github.com/sunweilin/forgify/backend/internal/app/tool/agent"
	approvaltool "github.com/sunweilin/forgify/backend/internal/app/tool/approval"
	asktool "github.com/sunweilin/forgify/backend/internal/app/tool/ask"
	controltool "github.com/sunweilin/forgify/backend/internal/app/tool/control"
	documenttool "github.com/sunweilin/forgify/backend/internal/app/tool/document"
	filesystemtool "github.com/sunweilin/forgify/backend/internal/app/tool/filesystem"
	functiontool "github.com/sunweilin/forgify/backend/internal/app/tool/function"
	handlertool "github.com/sunweilin/forgify/backend/internal/app/tool/handler"
	mcptool "github.com/sunweilin/forgify/backend/internal/app/tool/mcp"
	memorytool "github.com/sunweilin/forgify/backend/internal/app/tool/memory"
	searchtool "github.com/sunweilin/forgify/backend/internal/app/tool/search"
	shelltool "github.com/sunweilin/forgify/backend/internal/app/tool/shell"
	skilltool "github.com/sunweilin/forgify/backend/internal/app/tool/skill"
	subagenttool "github.com/sunweilin/forgify/backend/internal/app/tool/subagent"
	triggertool "github.com/sunweilin/forgify/backend/internal/app/tool/trigger"
	webtool "github.com/sunweilin/forgify/backend/internal/app/tool/web"
	workflowtool "github.com/sunweilin/forgify/backend/internal/app/tool/workflow"
	triggerapp "github.com/sunweilin/forgify/backend/internal/app/trigger"
	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	workspaceapp "github.com/sunweilin/forgify/backend/internal/app/workspace"
	relationdomain "github.com/sunweilin/forgify/backend/internal/domain/relation"
	mcpinfra "github.com/sunweilin/forgify/backend/internal/infra/mcp"
	sandboxinfra "github.com/sunweilin/forgify/backend/internal/infra/sandbox"
	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
)

// services holds every constructed app Service — the handlers read these, and the boot/shutdown
// sequence calls lifecycle methods on the few that own background work (sandbox/handler/mcp/
// trigger/scheduler/chat).
//
// services 持有所有构造好的 app Service——handler 读它们，boot/shutdown 序列对少数持后台工作的
// （sandbox/handler/mcp/trigger/scheduler/chat）调生命周期方法。
type services struct {
	workspace    *workspaceapp.Service
	apikey       *apikeyapp.Service
	modelCaps    *modelapp.CapabilityService
	relation     *relationapp.Service
	catalog      *catalogapp.Service
	notification *notificationapp.Service
	memory       *memoryapp.Service
	sandbox      *sandboxapp.Service
	document     *documentapp.Service
	todo         *todoapp.Service
	attachment   *attachmentapp.Service
	function     *functionapp.Service
	handler      *handlerapp.Service
	agent        *agentapp.Service
	trigger      *triggerapp.Service
	mcp          *mcpapp.Service
	skill        *skillapp.Service
	control      *controlapp.Service
	approval     *approvalapp.Service
	workflow     *workflowapp.Service
	scheduler    *schedulerapp.Service
	conversation *conversationapp.Service
	chat         *chatapp.Service
	subagent     *subagentapp.Service
	contextmgr   *contextmgrapp.Service
	aispawn      *aispawnapp.Service
}

// toolsetHolder is a mutable ToolsProvider: the subagent Service and agent invoke-deps read the
// toolset lazily (at spawn / invoke time), but the toolset isn't final until the Subagent tool is
// appended — which itself needs the subagent Service. The holder breaks that cycle.
//
// toolsetHolder 是可变 ToolsProvider：subagent Service 与 agent invoke-deps 懒读工具集（spawn / invoke
// 时），但工具集要等 Subagent 工具追加后才定型——而那又需 subagent Service。holder 破此环。
type toolsetHolder struct{ tools []toolapp.Tool }

func (h *toolsetHolder) Tools() []toolapp.Tool { return h.tools }
func (h *toolsetHolder) all() []toolapp.Tool   { return h.tools }

// buildServices constructs all 21 app Services in dependency order, wires every cross-Service
// adapter (R0060), the toolset, and all post-construction injection (relation syncers / catalog
// sources / mention resolvers / invoke deps / ref resolver). mux is the shared ServeMux trigger
// registers webhook routes on; dataDir roots the file-backed stores + sandbox.
//
// buildServices 按依赖序构造全部 21 个 app Service，接好每个跨 Service 适配器（R0060）、工具集，
// 以及所有装配后注入。mux 是 trigger 注册 webhook 路由的共享 mux；dataDir 是文件式 store + sandbox 的根。
func buildServices(st *stores, inf infra, bus buses, mux *http.ServeMux, dataDir string, log *zap.Logger) *services {
	// --- Tier 0: leaves (no app-Service dependencies) ---
	notif := notificationapp.NewService(st.notification, bus.notifications, log)
	ws := workspaceapp.NewService(st.workspace, log)
	keys := apikeyapp.NewService(st.apikey, inf.encryptor, apikeyapp.NewHTTPTester(http.DefaultClient), log)
	modelCaps := modelapp.NewCapabilityService(keys, log)
	cat := catalogapp.New(log)
	mem := memoryapp.NewService(st.memory, notif, log)
	sbx := sandboxapp.New(st.sandbox, dataDir, notif, log)

	// R0060 model-resolution chain (one core, four scenario wrappers) + caps/window lookup.
	lookup := NewModelInfoLookup(modelCaps)
	resolvers := NewModelResolvers(ws, keys, inf.factory, lookup)

	// envfix provisions a function/handler's sandbox env on demand (LLM-driven repair loop).
	prov := envfixapp.NewProvisioner(sbx, ws, keys, inf.factory, log)

	// --- Tier 1: entities (relation injected post-construction; nil-tolerant at build) ---
	doc := documentapp.New(st.document, notif, log)
	todo := todoapp.New(st.todo, bus.messages, log)
	att := attachmentapp.New(st.attachment, st.blob, attachmentapp.NewSandboxExtractor(sbx), log)
	fn := functionapp.NewService(st.function, prov, functionapp.NewSandboxAdapter(sbx, dataDir, bus.entities), notif, log)
	hd := handlerapp.NewService(st.handler, prov, handlerapp.NewSandboxAdapter(sbx, dataDir), inf.encryptor, handlerapp.DefaultClientFactory, notif, log)
	hd.SetEntitiesBridge(bus.entities) // SSE-C: Call tees method yields to the handler's run terminal
	ag := agentapp.NewService(st.agent, notif, log)
	ctl := controlapp.NewService(st.control, notif, log)
	apf := approvalapp.NewService(st.approval, notif, log)
	mcp := mcpapp.New(st.mcp, mcpinfra.NewGitHubRegistrySource(dataDir, log), sbx, log)
	mcp.SetEntitiesBridge(bus.entities) // SSE-C: CallTool tees progress to the server's run terminal
	conv := conversationapp.New(st.conversation, notif, log)
	trg := triggerapp.NewService(st.trigger, mux, NewSensorInvoker(fn, hd, mcp), log)
	trg.SetEntitiesBridge(bus.entities)                        // SSE-C: every fan-out emits a fire signal to the trigger panel
	wf := workflowapp.NewService(st.workflow, nil, notif, log) // resolver set below

	// --- subagent + skill: subagent reads the toolset lazily via the holder ---
	holder := &toolsetHolder{}
	subagentSvc := subagentapp.New(subagentapp.Deps{
		Messages: st.messages,
		Resolver: resolvers.Subagent(),
		Tools:    holder,
		Bridge:   bus.messages,
	}, log)
	skill := skillapp.NewService(st.skill, subagentSvc, notif, log)

	// --- toolset: Resident (filesystem/search/shell) + Lazy (entity tools + web) ---
	guard := pathguardpkg.NewDefault()
	toolset := toolapp.Toolset{
		Resident: concat(
			filesystemtool.FilesystemTools(guard),
			searchtool.SearchTools(guard, log),
			shelltool.NewShellTools().Tools,
			[]toolapp.Tool{asktool.New()}, // R0064: ask_user — agent asks the human (blocks on the humanloop broker)
		),
		Lazy: concat(
			functiontool.FunctionTools(fn),
			handlertool.HandlerTools(hd),
			agenttool.AgentTools(ag),
			controltool.ControlTools(ctl),
			approvaltool.ApprovalTools(apf),
			workflowtool.WorkflowTools(wf),
			triggertool.TriggerTools(trg),
			documenttool.DocumentTools(doc),
			memorytool.MemoryTools(mem),
			mcptool.MCPTools(mcp),
			skilltool.SkillTools(skill),
			webtool.WebTools(ws, keys, inf.factory, ws, log),
		),
	}
	// Append the Subagent tool (depth-1 guard: the subagent registry always filters it out, so a
	// subagent can never spawn another). Then publish the final set to the lazy holder.
	toolset.Lazy = append(toolset.Lazy, subagenttool.New(subagentSvc, subagentapp.NewRegistry().Names()))
	holder.tools = toolset.All()

	// --- context compaction + chat (the dialogue surface) ---
	ctxmgr := contextmgrapp.New(contextmgrapp.Deps{
		Messages:      st.messages,
		Conversations: NewConversationSummary(conv),
		Resolver:      resolvers.ContextmgrUtility(),
		Windows:       lookup.WindowResolver(),
	}, log)
	chat := chatapp.New(st.messages, chatapp.Deps{
		Conversations:  conv,
		Resolver:       resolvers.Chat(),
		Attachments:    NewAttachmentRenderer(att),
		Toolset:        toolset,
		Memory:         mem,
		Catalog:        cat,
		Documents:      NewDocumentRenderer(doc),
		Todo:           todo,
		Bridge:         bus.messages,
		EntitiesBridge: bus.entities,
		Titler:         conv,
		Notifier:       notif,
		Compactor:      ctxmgr,
	}, log)

	// --- durable workflow interpreter ---
	sched := schedulerapp.NewService(st.flowrun, wf, ctl, apf, NewDispatcher(fn, hd, mcp, ag), st.trigger, log)
	sched.SetEntitiesBridge(bus.entities) // SSE-C: Advance streams node progress to the workflow panel

	// D1 execution lifecycle: workflow drives the trigger binder (activate/stage/deactivate/kill engage
	// or release the listener) + the scheduler runner (trigger/kill drive runs); the scheduler drives
	// workflow's drain reconciler (a draining workflow goes inactive when its last run settles).
	// runnerAdapter bridges the primitive Runner port onto scheduler.StartInput so workflow never
	// imports the scheduler. Re-attach of active workflows on boot is App.Boot's job.
	//
	// D1 执行生命周期：workflow 驱动 trigger binder（激活/试运行/关激活/杀 挂或摘监听）+ 调度器 runner
	// （触发/杀 驱动 run）；调度器驱动 workflow 的排空 reconciler（draining workflow 最后一个 run 结算→inactive）。
	// runnerAdapter 把原生 Runner 端口桥到 scheduler.StartInput，使 workflow 绝不 import 调度器。boot 时重挂
	// active workflow 是 App.Boot 的事。
	wf.SetExecutionPorts(trg, runnerAdapter{sched: sched})
	sched.SetLifecycleReconciler(wf)

	// === post-construction injection ===
	// agent's ReAct deps: LLM resolver + the (lazy) tool universe + knowledge-doc prefix builder.
	ag.SetInvokeDeps(agentapp.InvokeDeps{
		Resolver:       resolvers.Agent(),
		Tools:          holder.all,
		Knowledge:      NewKnowledgeProvider(doc),
		EntitiesBridge: bus.entities, // SSE-C: agent run mirrors its ReAct trace to the agent panel
	})
	// workflow ref resolution (CapabilityCheck + pin closure determinism).
	wf.SetResolver(NewRefResolver(fn, hd, ag, ctl, apf, trg, mcp))

	// relation: built with every entity's name resolver (read-time hydration), then injected back
	// into each entity as its RelationSyncer (edge sync on create/edit/delete).
	rel := relationapp.NewService(relationapp.Config{
		Repo: st.relation,
		Namers: map[string]relationapp.Namer{
			relationdomain.EntityKindFunction:     fn,
			relationdomain.EntityKindHandler:      hd,
			relationdomain.EntityKindControl:      ctl,
			relationdomain.EntityKindApproval:     apf,
			relationdomain.EntityKindWorkflow:     wf,
			relationdomain.EntityKindTrigger:      trg,
			relationdomain.EntityKindMCP:          mcp,
			relationdomain.EntityKindSkill:        skill,
			relationdomain.EntityKindDocument:     doc,
			relationdomain.EntityKindConversation: conv,
		},
		Log: log,
	})
	fn.SetRelationSyncer(rel)
	hd.SetRelationSyncer(rel)
	ag.SetRelationSyncer(rel)
	ctl.SetRelationSyncer(rel)
	apf.SetRelationSyncer(rel)
	wf.SetRelationSyncer(rel)
	trg.SetRelationSyncer(rel)
	mcp.SetRelationSyncer(rel)
	skill.SetRelationSyncer(rel)
	doc.SetRelationSyncer(rel)
	conv.SetRelationSyncer(rel)

	// catalog: the LLM-facing "what entities exist" menu, aggregated from each forge source.
	cat.RegisterSource(fn.AsCatalogSource())
	cat.RegisterSource(hd.AsCatalogSource())
	cat.RegisterSource(ag.AsCatalogSource())
	cat.RegisterSource(ctl.AsCatalogSource())
	cat.RegisterSource(apf.AsCatalogSource())
	cat.RegisterSource(wf.AsCatalogSource())
	cat.RegisterSource(trg.AsCatalogSource())
	cat.RegisterSource(mcp.AsCatalogSource())
	cat.RegisterSource(skill.AsCatalogSource())
	cat.RegisterSource(doc.AsCatalogSource())

	// chat @mention resolvers (freeze-on-send snapshot, eight mentionable forge kinds).
	chat.RegisterMentionResolver(doc.AsMentionResolver())
	chat.RegisterMentionResolver(fn.AsMentionResolver())
	chat.RegisterMentionResolver(hd.AsMentionResolver())
	chat.RegisterMentionResolver(wf.AsMentionResolver())
	chat.RegisterMentionResolver(ag.AsMentionResolver())
	chat.RegisterMentionResolver(trg.AsMentionResolver())
	chat.RegisterMentionResolver(ctl.AsMentionResolver())
	chat.RegisterMentionResolver(apf.AsMentionResolver())

	s := &services{
		workspace: ws, apikey: keys, modelCaps: modelCaps, relation: rel, catalog: cat,
		notification: notif, memory: mem, sandbox: sbx, document: doc, todo: todo,
		attachment: att, function: fn, handler: hd, agent: ag, trigger: trg, mcp: mcp,
		skill: skill, control: ctl, approval: apf, workflow: wf, scheduler: sched,
		conversation: conv, chat: chat, subagent: subagentSvc, contextmgr: ctxmgr,
	}
	// aispawn (R0065) composes conversation + chat + a prefix-dispatched execution renderer; built
	// last since it reads the assembled services.
	//
	// aispawn（R0065）组合 conversation + chat + 前缀分发执行渲染器；最后建（它读已装配的 services）。
	s.aispawn = newAispawn(s, log)
	return s
}

// registerSandboxStack registers the mise-managed runtimes (python/node/uv/dotnet) + the docker
// installer + the four env managers, after sandbox.Bootstrap extracts the mise binary. A blank
// MiseBin means bootstrap failed → degraded mode, no runtimes (sandbox-dependent features error
// at call time, the server still runs). PythonEnvManager takes the Service as its ToolRegistry.
//
// registerSandboxStack 在 sandbox.Bootstrap 抽出 mise 二进制后，注册 mise 管的 runtime（python/node/
// uv/dotnet）+ docker installer + 4 个 env manager。MiseBin 空 = bootstrap 失败进 degraded（无 runtime，
// sandbox 相关功能调用时报错，server 照跑）。PythonEnvManager 以 Service 作 ToolRegistry。
func registerSandboxStack(svc *sandboxapp.Service) {
	miseBin := svc.MiseBin()
	if miseBin == "" {
		return
	}
	for kind, ver := range map[string]string{"python": "3.12", "node": "22", "uv": "0.11.4", "dotnet": "10.0.300"} {
		svc.RegisterInstaller(sandboxinfra.NewMiseInstaller(miseBin, kind, ver))
	}
	svc.RegisterInstaller(sandboxinfra.NewDockerInstaller())
	svc.RegisterEnvManager(sandboxinfra.NewPythonEnvManager(svc))
	svc.RegisterEnvManager(sandboxinfra.NewNodeEnvManager())
	svc.RegisterEnvManager(sandboxinfra.NewDockerEnvManager())
	svc.RegisterEnvManager(sandboxinfra.NewDotnetEnvManager())
}

// concat flattens tool groups into one slice.
//
// concat 把多个工具组展平成一个切片。
func concat(groups ...[]toolapp.Tool) []toolapp.Tool {
	var out []toolapp.Tool
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}
