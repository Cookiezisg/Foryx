package bootstrap

import (
	"context"
	"net/http"
	"os"
	"path/filepath"

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
	searchapp "github.com/sunweilin/forgify/backend/internal/app/search"
	settingsapp "github.com/sunweilin/forgify/backend/internal/app/settings"
	skillapp "github.com/sunweilin/forgify/backend/internal/app/skill"
	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	todoapp "github.com/sunweilin/forgify/backend/internal/app/todo"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	agenttool "github.com/sunweilin/forgify/backend/internal/app/tool/agent"
	approvaltool "github.com/sunweilin/forgify/backend/internal/app/tool/approval"
	asktool "github.com/sunweilin/forgify/backend/internal/app/tool/ask"
	blockstool "github.com/sunweilin/forgify/backend/internal/app/tool/blocks"
	controltool "github.com/sunweilin/forgify/backend/internal/app/tool/control"
	conversationtool "github.com/sunweilin/forgify/backend/internal/app/tool/conversation"
	documenttool "github.com/sunweilin/forgify/backend/internal/app/tool/document"
	filesystemtool "github.com/sunweilin/forgify/backend/internal/app/tool/filesystem"
	functiontool "github.com/sunweilin/forgify/backend/internal/app/tool/function"
	handlertool "github.com/sunweilin/forgify/backend/internal/app/tool/handler"
	mcptool "github.com/sunweilin/forgify/backend/internal/app/tool/mcp"
	memorytool "github.com/sunweilin/forgify/backend/internal/app/tool/memory"
	mounttool "github.com/sunweilin/forgify/backend/internal/app/tool/mount"
	relationtool "github.com/sunweilin/forgify/backend/internal/app/tool/relation"
	searchtool "github.com/sunweilin/forgify/backend/internal/app/tool/search"
	shelltool "github.com/sunweilin/forgify/backend/internal/app/tool/shell"
	skilltool "github.com/sunweilin/forgify/backend/internal/app/tool/skill"
	subagenttool "github.com/sunweilin/forgify/backend/internal/app/tool/subagent"
	todotool "github.com/sunweilin/forgify/backend/internal/app/tool/todo"
	triggertool "github.com/sunweilin/forgify/backend/internal/app/tool/trigger"
	webtool "github.com/sunweilin/forgify/backend/internal/app/tool/web"
	workflowtool "github.com/sunweilin/forgify/backend/internal/app/tool/workflow"
	triggerapp "github.com/sunweilin/forgify/backend/internal/app/trigger"
	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	workspaceapp "github.com/sunweilin/forgify/backend/internal/app/workspace"
	relationdomain "github.com/sunweilin/forgify/backend/internal/domain/relation"
	searchdomain "github.com/sunweilin/forgify/backend/internal/domain/search"
	mcpinfra "github.com/sunweilin/forgify/backend/internal/infra/mcp"
	sandboxinfra "github.com/sunweilin/forgify/backend/internal/infra/sandbox"
	searchengine "github.com/sunweilin/forgify/backend/internal/infra/search/engine"
	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
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
	settings     *settingsapp.Service
	conversation *conversationapp.Service
	chat         *chatapp.Service
	subagent     *subagentapp.Service
	contextmgr   *contextmgrapp.Service
	aispawn      *aispawnapp.Service
	search       *searchapp.Service
}

// toolsetHolder is a mutable ToolsProvider: the subagent Service and agent invoke-deps read the
// toolset lazily (at spawn / invoke time), but the toolset isn't final until the Subagent tool is
// appended — which itself needs the subagent Service. The holder breaks that cycle.
//
// toolsetHolder 是可变 ToolsProvider：subagent Service 与 agent invoke-deps 懒读工具集（spawn / invoke
// 时），但工具集要等 Subagent 工具追加后才定型——而那又需 subagent Service。holder 破此环。
type toolsetHolder struct{ tools []toolapp.Tool }

func (h *toolsetHolder) Tools() []toolapp.Tool { return h.tools }

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
	// search: one engine behind every surface (omni/vertical/blocks/RAG); sources and
	// notifiers wire post-construction, the worker starts at App.Boot.
	// search：所有出口背后的同一个引擎（综搜/垂搜/积木/RAG）；source 与 notifier 在装配后
	// 接线，worker 于 App.Boot 启动。
	searchSvc := searchapp.New(st.search, log)
	searchSvc.SetEmbeddingProviders(searchengine.NewBuiltin(sbx, log), func(baseURL, model string) searchdomain.EmbeddingProvider {
		return searchengine.NewOllama(baseURL, model)
	})
	searchSvc.SetSifter(&llmSifter{picker: ws, keys: keys, factory: inf.factory})

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
	fn.SetEntitiesBridge(bus.entities) // SSE-C: env 物化尝试行 tee 到 function 锻造终端（不分入口）
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

	// --- durable workflow interpreter (before the toolset: the flowrun-observability tools read it) ---
	// --- durable workflow 解释器（先于 toolset：flowrun 可观测工具要读它）---
	sched := schedulerapp.NewService(st.flowrun, wf, ctl, apf, NewDispatcher(fn, hd, mcp, ag), st.trigger, log)
	sched.SetEntitiesBridge(bus.entities) // SSE-C: Advance streams node progress to the workflow panel

	// --- subagent + skill: subagent reads the toolset lazily via the holder ---
	holder := &toolsetHolder{}
	subagentSvc := subagentapp.New(subagentapp.Deps{
		Messages: st.messages,
		Resolver: resolvers.Subagent(),
		Tools:    holder,
		Bridge:   bus.messages,
	}, log)
	skill := skillapp.NewService(st.skill, subagentSvc, notif, log)

	// relation: built with every entity's name resolver (read-time hydration), then injected back
	// into each entity as its RelationSyncer (edge sync on create/edit/delete).
	rel := relationapp.NewService(relationapp.Config{
		Repo: st.relation,
		Namers: map[string]relationapp.Namer{
			relationdomain.EntityKindFunction:     fn,
			relationdomain.EntityKindHandler:      hd,
			relationdomain.EntityKindAgent:        ag, // workflow→agent equip / conversation→agent forged 边的目标端 hydrate
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

	// --- toolset: Resident (filesystem/search/shell) + Lazy (entity tools + web) ---
	guard := pathguardpkg.NewDefault()
	toolset := toolapp.Toolset{
		Resident: concat(
			filesystemtool.FilesystemTools(guard),
			searchtool.SearchTools(guard, log),
			shelltool.NewShellTools().Tools,
			[]toolapp.Tool{asktool.New()}, // R0064: ask_user — agent asks the human (blocks on the humanloop broker)
			todotool.TodoTools(todo),      // todo_write — the checklist's only write path (the HTTP board is read-only)
		),
		Lazy: concat(
			functiontool.FunctionTools(fn, searchSvc),
			handlertool.HandlerTools(hd, searchSvc),
			agenttool.AgentTools(ag, searchSvc),
			controltool.ControlTools(ctl, searchSvc),
			approvaltool.ApprovalTools(apf, searchSvc),
			workflowtool.WorkflowTools(wf, searchSvc, sched),
			triggertool.TriggerTools(trg, searchSvc),
			documenttool.DocumentTools(doc, searchSvc),
			memorytool.MemoryTools(mem),
			mcptool.MCPTools(mcp),
			skilltool.SkillTools(skill),
			blockstool.BlocksTools(searchSvc),
			conversationtool.ConversationTools(searchSvc),
			relationtool.RelationTools(rel),
			webtool.WebTools(ws, keys, inf.factory, ws, ws, log),
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
	sched.SetNotifier(notif) // run_failed / approval_pending 唤回用户；completed 熄 attention
	// Deleting a conversation cancels its in-flight generation (chat satisfies the port;
	// post-build injection breaks the chat→conversation→chat cycle).
	// 删对话连带取消在途生成（chat 满足该端口；后注入破 chat→conversation→chat 环）。
	conv.SetGenerationCanceler(chat)

	// apikey delete-guard (RefScanner): refuse to delete a key still referenced, so the
	// reference never dangles. Two real sources — a workspace's scenario default models /
	// search key, and an agent's pinned modelOverride; both implement RefScanner structurally.
	// Without these the guard consults an empty scanner list and API_KEY_IN_USE can never fire.
	//
	// apikey 删除守卫（RefScanner）：仍被引用的 key 拒删，引用绝不悬空。两个真实来源——workspace
	// 的 scenario 默认模型 / 搜索 key，与 agent 钉死的 modelOverride；二者结构上满足 RefScanner。
	// 缺这两行则守卫询问空 scanner 列、API_KEY_IN_USE 永不触发。
	keys.AddRefScanner(ws)
	keys.AddRefScanner(ag)

	// Workspace delete cascades (PD-1 plan A): kill every workflow's automation (detach
	// listeners + cancel in-flight runs + inactive — idempotent on already-inactive ones, and
	// it also reaps manually-triggered runs), stop the workspace's resident handler/mcp
	// processes, then remove its on-disk tree (skills / memories). All on a Detached(target)
	// ctx — the DELETE request may arrive from a DIFFERENT workspace. Best-effort: the row
	// delete that follows is what makes the data unreachable.
	//
	// workspace 删除级联（PD-1 A 案）：杀每个 workflow 的自动化（摘监听 + 取消在途 run +
	// inactive——对已 inactive 幂等，且连手动触发的 run 一并收割）、停本 workspace 常驻
	// handler/mcp 进程、删盘上文件树（skills / memories）。全程用 Detached(目标) ctx——
	// DELETE 请求可能来自**另一个** workspace。best-effort：随后的删行才是数据不可达的根因。
	ws.SetReaper(func(_ context.Context, wsID string) {
		wsCtx := reqctxpkg.Detached(wsID)
		if wfs, err := wf.ListAll(wsCtx); err == nil {
			for _, w := range wfs {
				if _, kerr := wf.Kill(wsCtx, w.ID); kerr != nil {
					log.Warn("workspace reaper: kill workflow failed",
						zap.String("workspaceId", wsID), zap.String("workflowId", w.ID), zap.Error(kerr))
				}
			}
		} else {
			log.Warn("workspace reaper: list workflows failed", zap.String("workspaceId", wsID), zap.Error(err))
		}
		hd.StopWorkspaceInstances(wsCtx)
		mcp.DisconnectWorkspace(wsCtx)
		if perr := searchSvc.PurgeWorkspace(wsCtx, wsID); perr != nil {
			log.Warn("workspace reaper: purge search index failed", zap.String("workspaceId", wsID), zap.Error(perr))
		}
		if dataDir != "" {
			if rerr := os.RemoveAll(filepath.Join(dataDir, "workspaces", wsID)); rerr != nil {
				log.Warn("workspace reaper: remove file tree failed", zap.String("workspaceId", wsID), zap.Error(rerr))
			}
		}
	})

	// === post-construction injection ===
	// agent's ReAct deps: LLM resolver + mount synthesis (the agent's tool universe is exactly its
	// fn_/hd_/mcp mounts — never the system-tool registry) + skill guide + knowledge prefix.
	// agent 的 ReAct 依赖：LLM resolver + 挂载合成（agent 的工具宇宙恰是其 fn_/hd_/mcp 挂载——绝非系统
	// 工具表）+ skill 指南 + knowledge 前缀。
	ag.SetInvokeDeps(agentapp.InvokeDeps{
		Resolver:       resolvers.Agent(),
		Mounts:         mounttool.NewResolver(fn, hd, mcp),
		Skill:          skill,
		Knowledge:      NewKnowledgeProvider(doc),
		EntitiesBridge: bus.entities, // SSE-C: agent run mirrors its ReAct trace to the agent panel
	})
	// workflow ref resolution (CapabilityCheck + pin closure determinism).
	wf.SetResolver(NewRefResolver(fn, hd, ag, ctl, apf, trg, mcp))

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

	// search wiring: 12 entity projections in, one notifier out to every writer
	// (incl. chat/subagent message completion — anchor routes the incremental path).
	// search 接线：12 个实体投影接入，一个 notifier 发给所有写者（含 chat/subagent 的
	// message 完成——anchor 路由增量路径）。
	searchSvc.RegisterSource(fn.SearchSource())
	searchSvc.RegisterSource(hd.SearchSource())
	searchSvc.RegisterSource(ag.SearchSource())
	searchSvc.RegisterSource(wf.SearchSource())
	searchSvc.RegisterSource(trg.SearchSource())
	searchSvc.RegisterSource(ctl.SearchSource())
	searchSvc.RegisterSource(apf.SearchSource())
	searchSvc.RegisterSource(doc.SearchSource())
	searchSvc.RegisterSource(conv.SearchSource(st.messages))
	searchSvc.RegisterSource(mem.SearchSource())
	searchSvc.RegisterSource(skill.SearchSource())
	searchSvc.RegisterSource(mcp.SearchSource())
	sn := searchSvc.Notifier()
	fn.SetSearchNotifier(sn)
	hd.SetSearchNotifier(sn)
	ag.SetSearchNotifier(sn)
	wf.SetSearchNotifier(sn)
	trg.SetSearchNotifier(sn)
	ctl.SetSearchNotifier(sn)
	apf.SetSearchNotifier(sn)
	doc.SetSearchNotifier(sn)
	conv.SetSearchNotifier(sn)
	mem.SetSearchNotifier(sn)
	skill.SetSearchNotifier(sn)
	mcp.SetSearchNotifier(sn)
	chat.SetSearchNotifier(sn)
	subagentSvc.SetSearchNotifier(sn)
	// events.md 的 mcp.{installed,updated,removed,reconnected} 族——缺此线整族哑火（AC-29）。
	mcp.SetNotifier(notif)

	s := &services{
		workspace: ws, apikey: keys, modelCaps: modelCaps, relation: rel, catalog: cat,
		notification: notif, memory: mem, sandbox: sbx, document: doc, todo: todo,
		attachment: att, function: fn, handler: hd, agent: ag, trigger: trg, mcp: mcp,
		skill: skill, control: ctl, approval: apf, workflow: wf, scheduler: sched,
		conversation: conv, chat: chat, subagent: subagentSvc, contextmgr: ctxmgr,
		search: searchSvc,
	}
	// aispawn (R0065) composes conversation + chat + a prefix-dispatched execution renderer; built
	// last since it reads the assembled services.
	//
	// aispawn（R0065）组合 conversation + chat + 前缀分发执行渲染器；最后建（它读已装配的 services）。
	s.aispawn = newAispawn(s, log)
	return s
}

// registerSandboxStack registers the four self-built runtime installers (python/node/uv/dotnet, each
// fetching its pinned tarball straight from upstream on first use) + the docker installer + the four
// env managers. No mise, no embed, no bootstrap gate — the installers carry no host state.
// PythonEnvManager takes the Service as its ToolRegistry.
//
// registerSandboxStack 注册四个自研运行时 installer（python/node/uv/dotnet，各自首次使用时直接从上游拉
// 钉死的 tarball）+ docker installer + 4 个 env manager。无 mise、无内嵌、无 bootstrap 门控——installer
// 不持有宿主状态。PythonEnvManager 以 Service 作 ToolRegistry。
func registerSandboxStack(svc *sandboxapp.Service) {
	for _, inst := range sandboxinfra.DirectInstallers() {
		svc.RegisterInstaller(inst)
	}
	// Search embedder artifacts (llama-server + GGUF model) ride the same
	// installer registry — one download discipline for everything (§decisions/0001).
	// 搜索 embedder 产物（llama-server + GGUF 模型）走同一 installer 注册表——
	// 全部下载共用一套纪律（§decisions/0001）。
	for _, inst := range sandboxinfra.EngineInstallers() {
		svc.RegisterInstaller(inst)
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
