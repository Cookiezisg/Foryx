package chat

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	documentapp "github.com/sunweilin/forgify/backend/internal/app/document"
	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	agentstatepkg "github.com/sunweilin/forgify/backend/internal/pkg/agentstate"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	limitspkg "github.com/sunweilin/forgify/backend/internal/pkg/limits"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

func (s *Service) getOrCreateQueue(conversationID string) *convQueue {
	q := &convQueue{
		ch:         make(chan queuedTask, queueCapacity),
		agentState: &agentstatepkg.AgentState{},
	}
	actual, loaded := s.queues.LoadOrStore(conversationID, q)
	if loaded {
		return actual.(*convQueue)
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runQueue(conversationID, q)
	}()
	return q
}

func (s *Service) runQueue(conversationID string, q *convQueue) {
	const idleTimeout = 5 * time.Minute
	timer := time.NewTimer(idleTimeout)
	defer func() {
		timer.Stop()
		s.queues.Delete(conversationID)
	}()
	for {
		select {
		case task := <-q.ch:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			s.processTask(conversationID, q, task)
			timer.Reset(idleTimeout)
		case <-timer.C:
			return
		case <-s.shutdown:
			return
		}
	}
}

// processTask runs one queued chat turn. Its wall-clock backstop is
// limits.Agent.MaxTurnDurationSec (0 = unbounded) — a high safety net against a
// genuinely runaway turn, NOT a cap on healthy long work; the LLM idle-timeout +
// user stop are the primary controls (#1 ctx-over-timeout). Raised from a flat
// 10min so legitimately long turns aren't killed.
//
// processTask 跑一个排队 chat turn。墙钟兜底取自 limits.Agent.MaxTurnDurationSec
// （0 = 无限）——防真正失控回合的高安全网，非限健康长活；LLM idle 超时 + 用户
// stop 才是主控。从固定 10min 抬高，避免杀健康长回合。
func (s *Service) processTask(conversationID string, q *convQueue, task queuedTask) {
	ctx := task.ctx

	var (
		agentCtx context.Context
		cancel   context.CancelFunc
	)
	if turnDur := time.Duration(limitspkg.Current().Agent.MaxTurnDurationSec) * time.Second; turnDur > 0 {
		agentCtx, cancel = context.WithTimeout(ctx, turnDur)
	} else {
		agentCtx, cancel = context.WithCancel(ctx)
	}
	q.mu.Lock()
	q.cancel = cancel
	q.mu.Unlock()
	defer func() {
		cancel()
		q.mu.Lock()
		q.cancel = nil
		q.mu.Unlock()
	}()
	agentCtx = reqctxpkg.WithConversationID(agentCtx, conversationID)
	agentCtx = reqctxpkg.WithAgentState(agentCtx, q.agentState)
	agentCtx = eventlogpkg.With(agentCtx, s.emitter)

	// Pre-allocate msgID so pre-LLM errors can attach message_stop to a stable ID.
	msgID := newMsgID()
	agentCtx = reqctxpkg.WithMessageID(agentCtx, msgID)

	s.emitter.EmitMessageStart(agentCtx, msgID, chatdomain.RoleAssistant, "", nil)

	// §12.3 per-conv override: conv.ModelOverride beats user's dialogue-scenario default.
	// Also stash on ctx so nested subagent spawns inherit the same effective override.
	//
	// §12.3 对话级 override：conv.ModelOverride 优先于 dialogue scenario 默认;
	// 顺便塞进 ctx 让嵌套 subagent 承袭同一 override。
	agentCtx = reqctxpkg.WithModelOverride(agentCtx, task.conv.ModelOverride)
	bc, err := llmclientpkg.ResolveDialogueWithOverride(agentCtx, task.conv.ModelOverride, s.modelPicker, s.keyProvider, s.llmFactory)
	if err != nil {
		code := "LLM_PROVIDER_ERROR"
		switch {
		case errors.Is(err, llmclientpkg.ErrPickModel):
			code = "MODEL_NOT_CONFIGURED"
		case errors.Is(err, llmclientpkg.ErrResolveCreds):
			code = "API_KEY_PROVIDER_NOT_FOUND"
		case errors.Is(err, llmclientpkg.ErrBuildClient):
			code = "LLM_BUILD_FAILED"
		}
		s.emitFatalError(agentCtx, task.conv, task.uid, msgID, code, err.Error())
		return
	}

	baseReq := llminfra.Request{
		ModelID:  bc.ModelID,
		Key:      bc.Key,
		BaseURL:  bc.BaseURL,
		System:   s.buildSystemPrompt(agentCtx, task.conv),
		Thinking: bc.Thinking,
		Options:  bc.Options,
	}

	host := &chatHost{
		svc:       s,
		convID:    task.conv.ID,
		uid:       task.uid,
		msgID:     msgID,
		userMsgID: task.userMsgID,
		provider:  bc.Provider,
		modelID:   bc.ModelID,
	}
	// Install V1.2 §3 interceptor (permissions gate + Pre/PostToolUse).
	// Nil when SetPermissionsAndHooks wasn't called → loop sees noop.
	// 装 V1.2 §3 interceptor。未 SetPermissionsAndHooks 时为 nil，loop 走 noop。
	if s.interceptor != nil {
		agentCtx = loopapp.WithInterceptor(agentCtx, s.interceptor)
	}
	ms := limitspkg.Current().Agent.MaxSteps
	if ms <= 0 {
		ms = 1_000_000 // 0 = unbounded; a runaway backstop the user can always interrupt
	}
	result := loopapp.Run(agentCtx, host, bc.Client, baseReq, ms, s.log)

	s.log.Info("agent run complete",
		zap.String("conversation_id", task.conv.ID),
		zap.String("stop_reason", result.StopReason),
		zap.Int("input_tokens", result.TokensIn),
		zap.Int("output_tokens", result.TokensOut))

	// Compaction runs synchronously before autoTitle so the fake LLM FIFO is deterministic.
	if s.compactor != nil {
		compactCtx := reqctxpkg.SetUserID(context.Background(), task.uid)
		compactCtx = reqctxpkg.WithConversationID(compactCtx, task.conv.ID)
		if err := s.compactor.MaybeCompact(compactCtx, task.conv.ID, bc.Provider, bc.ModelID); err != nil {
			s.log.Warn("contextmgr.MaybeCompact failed (non-fatal)",
				zap.String("conv", task.conv.ID), zap.Error(err))
		}
	}

	if task.conv.Title == "" && !task.conv.AutoTitled {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.autoTitle(context.Background(), task.conv, task.uid, result.LastMessage)
		}()
	}
}

// emitFatalError persists an error stub Message and emits message_stop to close the SSE bubble.
//
// emitFatalError 落库 error 占位 Message 并推 message_stop 关闭 SSE bubble。
func (s *Service) emitFatalError(
	ctx context.Context,
	conv *convdomain.Conversation,
	uid, msgID, code, message string,
) {
	s.log.Error("chat fatal error",
		zap.String("conversation_id", conv.ID),
		zap.String("code", code), zap.String("message", message))

	// Detached saveCtx mirrors host.WriteFinalize: upstream cancel must not block terminal write.
	saveCtx := reqctxpkg.SetUserID(context.Background(), uid)
	saveCtx = reqctxpkg.WithConversationID(saveCtx, conv.ID)
	// emitFatalError fires before bundle.Provider/ModelID are known
	// (resolve failed), so leave them empty — usage aggregation drops
	// zero-token rows anyway.
	// emitFatalError 在 bundle.Provider/ModelID 已知前触发（resolve 已
	// 失败），留空——usage 聚合本就丢 0 token 行。
	msg := buildMessage(msgID, conv.ID, uid,
		chatdomain.StatusError, chatdomain.StopReasonError,
		code, message, 0, 0, "", "")
	if err := s.repo.SaveMessage(saveCtx, msg); err != nil {
		s.log.Error("CRITICAL: fatal-error stub message persist failed — message lost",
			zap.String("msg_id", msgID), zap.Error(err))
	}

	s.emitter.StopMessage(saveCtx, msgID, eventlogdomain.StatusError,
		chatdomain.StopReasonError, code, message, 0, 0)
}

// PromptSection is one named segment in the chat system prompt; sections concatenate via separator into the wire prompt.
//
// PromptSection 是 chat system prompt 的一段；按顺序拼接为最终 wire prompt。
type PromptSection struct {
	Name    string `json:"name"` // "identity" / "how_to_work" / "tools" / "capabilities" / "memory" / "documents" / "user_system_prompt" / "environment"
	Content string `json:"content"`
}

// SystemPromptSections returns the per-conv assembled prompt as ordered named sections (cache-friendly order: static-first, dynamic-last).
//
// SystemPromptSections 返按 cache-friendly 顺序（静态前 / 动态后）排好的命名段；外部预览端点直接消费。
func (s *Service) SystemPromptSections(ctx context.Context, conv *convdomain.Conversation) []PromptSection {
	out := make([]PromptSection, 0, 9)
	out = append(out, PromptSection{Name: "identity", Content: identitySection})
	out = append(out, PromptSection{Name: "how_to_work", Content: howToWorkSection})
	out = append(out, PromptSection{Name: "tools", Content: toolsSection})

	if capContent := s.buildCapabilitiesSection(ctx); capContent != "" {
		out = append(out, PromptSection{Name: "capabilities", Content: capContent})
	}
	if s.memory != nil {
		if memoryText := s.memory.ForSystemPrompt(ctx); memoryText != "" {
			out = append(out, PromptSection{Name: "memory", Content: memoryText})
		}
	}
	if s.documents != nil && len(conv.AttachedDocuments) > 0 {
		docs, err := s.documents.ResolveAttached(ctx, conv.AttachedDocuments)
		if err != nil {
			s.log.Warn("chat.SystemPromptSections: ResolveAttached failed",
				zap.String("conv_id", conv.ID), zap.Error(err))
		} else if len(docs) > 0 {
			out = append(out, PromptSection{Name: "documents", Content: documentapp.RenderAttachedAsXML(docs)})
		}
	}
	if conv.SystemPrompt != "" {
		out = append(out, PromptSection{Name: "user_system_prompt", Content: conv.SystemPrompt})
	}
	lang := "English"
	if reqctxpkg.GetLocale(ctx) == reqctxpkg.LocaleZhCN {
		lang = "Chinese"
	}
	out = append(out, PromptSection{Name: "environment",
		Content: fmt.Sprintf("%s · reply language: %s", time.Now().Format("2006-01-02"), lang)})
	// Architecture rules + critical rules go LAST (殿后): deepseek respects end-of-prompt most.
	// Validated by LLM experiments (doc 13 §4.5, doc 15 §D):
	//   architecture rules: +10pt on complex workflow creation
	//   critical rules:     impossible capability ban +78pt (17→95); commit-after-recon revert +65pt (35→100)
	out = append(out, PromptSection{Name: "architecture_rules", Content: architectureRulesSection})
	out = append(out, PromptSection{Name: "critical_rules", Content: criticalRulesSection})
	return out
}

// identitySection + howToWorkSection open every chat system prompt: who you are, then how to work.
//
// identitySection + howToWorkSection 是每轮 chat system prompt 的身份 + 工作原则开头。
const identitySection = "You are Forgify, an Agentic Workflow Platform. You turn user needs into reusable Quadrinity capabilities: Functions (stateless logic), Handlers (stateful resource managers), Agents (configured expert workers), and Workflows (durable orchestration programs)."

const howToWorkSection = `- Reuse first: Always search_* the user's library and extend existing entities before forging new ones. Build the smallest fit: Function for stateless logic, Handler for stateful resource management, Agent for configured expert intelligence, and Workflow to orchestrate them into durable programs.
- Evidence over claim: Test everything you forge (run_function / call_handler / invoke_agent / trigger_workflow dryRun). Report the raw physical result and any stdout/traceback from the sandbox — never claim untested success or guess outcomes.
- Grounding: Inspect before changing (get_* / read_document); if the physical reality of an entity contradicts the user's description, surface the mismatch immediately instead of plowing ahead.
- Destructive Guard: Before irreversible or high-stakes actions — deleting a forge, force-reverting, external state writes, or external MCP writes — you MUST set destructive=true to trigger a user confirmation bubble.
- Informed Asking: Use AskUserQuestion when a request is logically impossible to satisfy or essential config is missing. Build with sensible defaults when possible, but do not interrogate the user over standard/safe configurations.
- Be concise: Lead with the actual result or the change made, skip the "play-by-play" of your tool calls, and always match the user's language.
- Parallel Execution: Group independent subtasks into the same execution_group to run them in parallel; use Subagent to fan out complex independent research; keep coupled or side-effecting work sequential.`

// architectureRulesSection teaches semantic architecture decisions that LLMs get wrong on first draft.
// Each rule is validated by LLM experiments (doc 13 §4.5, doc 15 §D). These go before criticalRulesSection.
//
// architectureRulesSection 教 LLM 首草图常犯的语义架构决策（实测验证，+10pt）。
const architectureRulesSection = `Architecture decision rules — apply these before building:
- Durable execution mental model: A Workflow is a formal program, NOT a chat. Every step result and control-flow decision is Journaled for deterministic replay.
- Core 5-Node limit: Construct graphs using ONLY trigger (entry), agent (intelligent node), tool (capability dispatch), case (branching), and approval (human sign-off).
- Tool Node Versatility: Use a 'tool' node for any discrete action or computation. It can call a stateless Function (fn_), a stateful Handler method (hd_), or an external MCP tool (mcp:).
- Intelligence vs Action: Does the step require reasoning, classification, or intent detection? Use an 'agent' node. Is it a deterministic computation or API call? Use a 'tool' node.
- State Persistence: Do not store state in local files during a Workflow; use 'payload' to pass data between nodes. The Journal is the only source of truth.
- Polling Trigger: Use a polling function (poll(last_cursor) → {events, next_cursor}); NEVER use cron + full database pull for event streams.
- Logic Guards: Case nodes must use boolean CEL expressions on the 'payload' (e.g. when: "payload.status == 200"). Case is a router, NOT an analyst — do not put LLM calls or complex analysis inside case guards.
- Iteration & Retry: Implement loops and retries via back-edges. Use an explicit counter: when: "(has(payload.attempt)?payload.attempt:0) < 3" and emit: {attempt: "(has(payload.attempt)?payload.attempt:0) + 1"}.
- Atomic Authoring: Build the complete graph in a single create_workflow call. Always run capability_check_workflow after editing to verify refs.`

// criticalRulesSection goes LAST in the prompt (殿后): deepseek respects end-of-prompt most.
// Every rule here maps to an observed failure mode with measured recovery (+11pt to +95%).
//
// criticalRulesSection 殿后（deepseek 对 prompt 末尾遵守度最高）。每条对应实测失败模式。
const criticalRulesSection = `CRITICAL RULES — highest priority, follow exactly:

1. WORKFLOW NODE RESTRICTION: You can only build Workflows using the 5 Core Nodes: trigger, agent, tool, case, and approval. Workers (agent/tool nodes) may ONLY use fn_/hd_/mcp callables. An agent NEVER calls another agent. Platform tools (fs/shell/web/memory/ask) are for the Chat Assistant only.

2. FORGE-TEST-ACCEPT LOOP: After creating or editing any entity (Quadrinity), you MUST immediately run/invoke/trigger it to verify logic. Report the RAW physical result and stdout from the sandbox. Do NOT ask "does this look okay?" — show the physical evidence of success or failure.

3. IMPOSSIBLE CAPABILITY BAN: NEVER write a prompt for an Agent (entity or node) that claims it can do things it lacks tools for. If an agent needs web access, you must explicitly attach a web tool. Verify tool availability before writing the prompt.

4. COMMIT AFTER RECON: Once you have searched/read an entity's metadata/code, proceed directly to the requested action (edit/run/invoke/delete). Do not re-search or re-read the same entity in the same turn. "Reconnaissance loops" are a critical bug.

5. SATISFIABILITY CHECK: Only flag a conflict when requirements are logically impossible (e.g. "fully automated" vs "manual approval required"). If information is simply incomplete (missing an API endpoint or email), use a placeholder/default and move on. Do NOT interrupt the user over incomplete details.

6. GRAPH RULES: Triggers (cron/manual) carry NO business data; the first subsequent node must fetch its own data. Use strict boolean CEL guards for 'case' branches. Build the complete graph in one create_workflow call. Always call capability_check_workflow before accepting.`

// toolsSection states the tool model + the three standard fields once, instead of repeating across every tool schema.
//
// toolsSection 统一讲工具模型 + 三个标准字段,避免在每个 tool schema 里重复。
const toolsSection = `Common tools are always loaded; pull the rest on demand with activate_tools(category):
function/handler/agent/workflow — create · edit · delete · revert · run/call/invoke/trigger · inspect.
Three standard fields on every call: summary (one line: what + why), destructive (true if irreversible), execution_group (int; same group runs in parallel, groups run in order).
Search before you act; call by a real id, never a guess.`

// IdentityText / HowToWorkText / ToolsText expose static chat prompt segments to the §18 inventory endpoint.
//
// IdentityText / HowToWorkText / ToolsText 把静态段暴露给 §18 prompt 总览端点。
func IdentityText() string  { return identitySection }
func HowToWorkText() string { return howToWorkSection }
func ToolsText() string     { return toolsSection }

// categoryLabels maps the known lazy category names to their human-readable descriptions for the capabilities index.
//
// categoryLabels 把已知的 lazy 分类名映射成给 LLM 看的人类可读说明。
var categoryLabels = map[string]string{
	"function": "create/edit/delete/revert/run/get/search functions",
	"handler":  "create/edit/delete/revert/call/get/search handlers",
	"agent":    "create/edit/delete/revert/invoke/get/search agent",
	"workflow": "create/edit/delete/revert/trigger/get/search workflows",
	"mcp":      "install/uninstall/call/list external MCP",
	"document": "create/edit/delete/move/read/search documents",
	"skill":    "skill execution logs",
}

// buildCapabilitiesSection assembles (a) sorted tool-group index from Lazy + (b) catalog asset menu.
// Returns "" when both are empty so the caller can skip the section entirely.
//
// buildCapabilitiesSection 拼 (a) Lazy 的有序 tool-group 索引 + (b) catalog 资产菜单。
// 两者均空时返 ""，调用方跳过该段。
func (s *Service) buildCapabilitiesSection(ctx context.Context) string {
	var sb strings.Builder

	// (a) Tool-group index — only when Lazy is non-empty.
	if len(s.toolset.Lazy) > 0 {
		categories := make([]string, 0, len(s.toolset.Lazy))
		for cat := range s.toolset.Lazy {
			categories = append(categories, cat)
		}
		sort.Strings(categories)

		sb.WriteString("Common tools are loaded. To act on a category, call activate_tools(category):\n")
		for _, cat := range categories {
			label, ok := categoryLabels[cat]
			if !ok {
				label = cat
			}
			n := len(s.toolset.Lazy[cat])
			sb.WriteString(fmt.Sprintf("- %s — %s (%d)\n", cat, label, n))
		}
		sb.WriteString("Prefer loaded tools; activate a category only when you need it.")
	}

	// (b) Asset menu — only when catalog returns non-empty text.
	catalogText := ""
	if s.catalog != nil {
		catalogText = s.catalog.GetForSystemPrompt(ctx)
	}
	if catalogText != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("## Your library\n")
		sb.WriteString(catalogText)
	}

	return sb.String()
}

func (s *Service) buildSystemPrompt(ctx context.Context, conv *convdomain.Conversation) string {
	return AssemblePromptSections(s.SystemPromptSections(ctx, conv))
}

// AssemblePromptSections wraps each section in <section name="..."> markers so the LLM (and the preview UI) can see boundaries.
//
// AssemblePromptSections 把每段用 <section name="..."> 包起来，LLM 与预览 UI 都能看到边界。
func AssemblePromptSections(sections []PromptSection) string {
	var sb strings.Builder
	for i, sec := range sections {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("<section name=\"")
		sb.WriteString(sec.Name)
		sb.WriteString("\">\n")
		sb.WriteString(sec.Content)
		sb.WriteString("\n</section>")
	}
	return sb.String()
}

// autoTitle generates a short title via LLM, persists, and publishes a notification (best-effort).
//
// autoTitle 经 LLM 生成短标题、持久化、发 conversation 通知（失败静默）。
func (s *Service) autoTitle(ctx context.Context, conv *convdomain.Conversation, uid, assistantContent string) {
	titleCtx := reqctxpkg.SetUserID(ctx, uid)
	bc, err := llmclientpkg.ResolveUtility(titleCtx, s.modelPicker, s.keyProvider, s.llmFactory)
	if err != nil {
		return
	}

	tCtx, cancel := context.WithTimeout(titleCtx, 10*time.Second)
	defer cancel()

	req := llminfra.Request{
		ModelID:  bc.ModelID,
		Key:      bc.Key,
		BaseURL:  bc.BaseURL,
		Thinking: bc.Thinking,
		Options:  bc.Options,
		System:   "Generate a short conversation title (5 words or fewer). Reply with ONLY the title, no punctuation.\n只返回标题本身，不超过 10 个字，不加标点。",
		Messages: []llminfra.LLMMessage{
			{Role: llminfra.RoleUser, Content: "Assistant said: " + truncate(assistantContent, 300)},
		},
	}
	title, err := llminfra.Generate(tCtx, bc.Client, req)
	if err != nil || title == "" {
		return
	}
	conv.Title = strings.TrimSpace(title)
	conv.AutoTitled = true
	if err := s.convRepo.Save(titleCtx, conv); err != nil {
		s.log.Warn("auto-title save failed", zap.Error(err))
		return
	}
	s.notifications.Publish(titleCtx, "conversation", conv.ID,
		map[string]any{"action": "auto_titled", "title": conv.Title, "autoTitled": true},
		conv.ID)
	s.log.Info("auto-title generated",
		zap.String("conversation_id", conv.ID), zap.String("title", conv.Title))
}
