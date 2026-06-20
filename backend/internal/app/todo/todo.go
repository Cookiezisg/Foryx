// Package todo (app layer) owns the Service for the agent's working-memory checklist:
// whole-list writes scoped to (conversation, subagent?), a render for keeping the list in
// front of the model, and a live push to the messages stream for the user's task board.
// Workspace isolation is automatic at the orm layer; the execution scope (conversation id
// + optional subagent id) is read from ctx via reqctx — so this stays a leaf module,
// importing neither the conversation nor the subagent business package.
//
// Package todo（app 层）持有 agent 工作记忆清单的 Service：作用域 (conversation, subagent?)
// 的整列写入、把清单顶在模型眼前的渲染、以及推 messages 流给用户任务看板的 live 推送。
// workspace 隔离在 orm 层自动；执行作用域（对话 id + 可选 subagent id）经 reqctx 从 ctx 读——
// 故本模块仍是叶子，既不 import conversation 也不 import subagent 业务包。
package todo

import (
	"context"
	"encoding/json"

	"go.uber.org/zap"

	streamdomain "github.com/sunweilin/anselm/backend/internal/domain/stream"
	tododomain "github.com/sunweilin/anselm/backend/internal/domain/todo"
	reqctxpkg "github.com/sunweilin/anselm/backend/internal/pkg/reqctx"
)

// signalNodeType is the messages-stream node.type carried by a todo live push. The "todo"
// identity lives here (the event type), NOT in the scope kind — the scope anchors to the
// conversation (where the board renders); the protocol keeps anchor and event-type apart.
//
// signalNodeType 是 todo live 推送在 messages 流上携带的 node.type。"todo" 身份在此（事件
// 类型），**不在** scope kind——scope 锚定对话（看板渲染处）；协议把锚点与事件类型分开。
const signalNodeType = "todo"

// Service is the checklist application façade.
//
// Service 是清单应用 façade。
type Service struct {
	repo   tododomain.Repository
	bridge streamdomain.Bridge // messages SSE stream; nil → no live push (still persisted)
	log    *zap.Logger
}

// New constructs a Service. bridge is the messages stream (nil → persist only, no live
// board update — wired at boot); nil repo / logger panics.
//
// New 构造 Service。bridge 是 messages 流（nil → 只持久化、不更新看板——boot 装配）；nil repo / logger panic。
func NewService(repo tododomain.Repository, bridge streamdomain.Bridge, log *zap.Logger) *Service {
	if repo == nil {
		panic("todoapp.New: repo is nil")
	}
	if log == nil {
		panic("todoapp.New: logger is nil")
	}
	return &Service{repo: repo, bridge: bridge, log: log}
}

// Write replaces the current scope's checklist wholesale (TodoWrite semantics) and
// returns it rendered for the tool result, so the model sees its just-written plan echoed
// back. The scope is taken from ctx: a subagent run writes its own list, the main
// conversation writes the conversation's. Items are validated + trimmed; an empty write
// clears the list.
//
// Write 整体替换当前作用域清单（TodoWrite 语义），返回渲染好的文本给 tool 结果，使模型看到刚
// 写的计划被回显。作用域取自 ctx：subagent run 写自己的清单，主对话写对话的。items 校验 + trim；
// 空写清空清单。
func (s *Service) Write(ctx context.Context, items []tododomain.Item) (string, error) {
	conv, sub, err := scopeFromCtx(ctx)
	if err != nil {
		return "", err
	}
	clean, err := normalize(items)
	if err != nil {
		return "", err
	}
	l := &tododomain.List{
		ScopeID:        scopeID(conv, sub),
		ConversationID: conv,
		SubagentID:     sub,
		Items:          clean,
	}
	if err := s.repo.Upsert(ctx, l); err != nil {
		return "", err
	}
	s.broadcast(ctx, conv, sub, clean)
	return render(clean), nil
}

// Get returns the current scope's checklist (empty when none). Used by the per-turn
// reminder and by read-only tool paths.
//
// Get 返回当前作用域清单（无则空）。供每轮 reminder 与只读工具路径用。
func (s *Service) Get(ctx context.Context) ([]tododomain.Item, error) {
	conv, sub, err := scopeFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	return s.itemsFor(ctx, scopeID(conv, sub))
}

// GetForScope returns a checklist by explicit (conversation, subagent?) — the REST read
// path, where the conversation comes from the URL, not ctx. subagentID nil = main list.
//
// GetForScope 按显式 (conversation, subagent?) 返清单——REST 读路径，对话来自 URL 而非 ctx。
// subagentID nil = 主清单。
func (s *Service) GetForScope(ctx context.Context, conversationID string, subagentID *string) ([]tododomain.Item, error) {
	return s.itemsFor(ctx, scopeID(conversationID, subagentID))
}

// SystemReminder renders the current scope's open (non-completed) checklist as a reminder
// block for injection into the next model turn, plus whether to inject (false when empty
// or fully completed). This is the mechanism that keeps the plan in front of the model —
// the loop calls it each iteration. A read error degrades to "do not inject".
//
// SystemReminder 把当前作用域的未完成清单渲染成 reminder 块、供注入下一轮模型回合，外加是否
// 注入（空或全完成时 false）。这是把计划顶在模型眼前的机制——loop 每轮迭代调它。读出错
// 降级为"不注入"。
func (s *Service) SystemReminder(ctx context.Context) (string, bool) {
	items, err := s.Get(ctx)
	if err != nil {
		return "", false
	}
	return reminder(items)
}

// ReadRendered returns the current scope's full checklist rendered (INCLUDING completed items)
// for the todo_read tool — the read-back path so the agent lists its todos from the saved truth,
// not from memory. Without it a fully-completed list is invisible (the per-turn reminder suppresses
// a 0-open list and there was no read tool), so the agent confabulated when asked to list todos.
// Empty list renders the "(todo list cleared — no tasks)" string (soft, not an error).
//
// ReadRendered 返回当前作用域整张清单的渲染（含已完成项）给 todo_read 工具——读回路径，使 agent
// 从已存真相而非记忆列出 todo。没它，全完成清单不可见（每轮 reminder 抑制 0-open 清单、又无读
// 工具），故被问列 todo 时 agent 编造。空清单渲染 "(todo list cleared — no tasks)" 串（软、非错）。
func (s *Service) ReadRendered(ctx context.Context) (string, error) {
	items, err := s.Get(ctx)
	if err != nil {
		return "", err
	}
	return render(items), nil
}

func (s *Service) itemsFor(ctx context.Context, scope string) ([]tododomain.Item, error) {
	l, err := s.repo.GetByScope(ctx, scope)
	if err != nil {
		return nil, err
	}
	if l == nil {
		return nil, nil
	}
	return l.Items, nil
}

// broadcast pushes the full checklist as a durable signal on the messages stream,
// anchored to the conversation (so a frontend viewing that conversation receives it); the
// subagent id rides in the payload so a subagent's board nests under the right subtree.
// Best-effort: a missed push is recovered by the REST read on next open.
//
// broadcast 把整张清单作为 durable signal 推到 messages 流、锚定对话（使查看该对话的前端收到）；
// subagent id 随 payload，使 subagent 看板嵌到正确子树下。best-effort：漏推由下次打开的 REST 读兜回。
func (s *Service) broadcast(ctx context.Context, conv string, sub *string, items []tododomain.Item) {
	if s.bridge == nil {
		return
	}
	payload := map[string]any{
		"conversationId": conv,
		"todos":          items,
	}
	if sub != nil {
		payload["subagentId"] = *sub
	}
	content, err := json.Marshal(payload)
	if err != nil {
		s.log.Warn("todo signal marshal failed", zap.String("conversation_id", conv), zap.Error(err))
		return
	}
	if _, err := s.bridge.Publish(ctx, streamdomain.Event{
		Scope: streamdomain.Scope{Kind: streamdomain.KindConversation, ID: conv},
		ID:    scopeID(conv, sub),
		Frame: streamdomain.Signal{Node: streamdomain.Node{Type: signalNodeType, Content: content}},
	}); err != nil {
		s.log.Warn("todo SSE push failed", zap.String("conversation_id", conv), zap.Error(err))
	}
}

// scopeFromCtx reads the execution scope from ctx: conversation id (required) + optional
// subagent id (present only inside a subagent run).
//
// scopeFromCtx 从 ctx 读执行作用域：对话 id（必填）+ 可选 subagent id（仅 subagent run 内有）。
func scopeFromCtx(ctx context.Context) (conv string, sub *string, err error) {
	conv, err = reqctxpkg.RequireConversationID(ctx)
	if err != nil {
		return "", nil, err
	}
	if sid, ok := reqctxpkg.GetSubagentID(ctx); ok {
		sub = &sid
	}
	return conv, sub, nil
}

// scopeID is the polymorphic owner id: the subagent id inside a subagent run, else the
// conversation id. It is the todos PK and the SSE event id.
//
// scopeID 是多态 owner id：subagent run 内为 subagent id、否则为对话 id。它是 todos 主键与 SSE 事件 id。
func scopeID(conv string, sub *string) string {
	if sub != nil {
		return *sub
	}
	return conv
}
