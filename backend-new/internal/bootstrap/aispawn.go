package bootstrap

import (
	"context"
	"encoding/json"
	"strings"

	"go.uber.org/zap"

	agentapp "github.com/sunweilin/forgify/backend/internal/app/agent"
	aispawnapp "github.com/sunweilin/forgify/backend/internal/app/aispawn"
	chatapp "github.com/sunweilin/forgify/backend/internal/app/chat"
	conversationapp "github.com/sunweilin/forgify/backend/internal/app/conversation"
	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	mcpapp "github.com/sunweilin/forgify/backend/internal/app/mcp"
	schedulerapp "github.com/sunweilin/forgify/backend/internal/app/scheduler"
	triggerapp "github.com/sunweilin/forgify/backend/internal/app/trigger"
	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
)

// aispawn adapters (R0065): glue the iterate/triage engine onto the concrete conversation + chat
// services and a prefix-dispatched execution renderer. Kept here (bootstrap) so aispawn stays a
// pure app service depending only on its small ports.
//
// aispawn 适配器（R0065）：把 iterate/triage 引擎黏到具体 conversation + chat 服务 + 前缀分发的执行渲染器。放在
// bootstrap，使 aispawn 保持只依赖其小端口的纯 app 服务。

// aispawnConvStarter wraps conversation.CreateWithSystemPrompt, returning just the new id.
//
// aispawnConvStarter 包 conversation.CreateWithSystemPrompt，只返回新 id。
type aispawnConvStarter struct{ conv *conversationapp.Service }

func (a aispawnConvStarter) StartSeeded(ctx context.Context, systemPrompt string) (string, error) {
	c, err := a.conv.CreateWithSystemPrompt(ctx, "", systemPrompt)
	if err != nil {
		return "", err
	}
	return c.ID, nil
}

// aispawnSender wraps chat.Send to deliver the seeded first turn (carrying @-mentions).
//
// aispawnSender 包 chat.Send 投递预置首条回合（携 @-mention）。
type aispawnSender struct{ chat *chatapp.Service }

func (a aispawnSender) SendSeed(ctx context.Context, conversationID, content string, mentions []mentiondomain.MentionInput) (string, error) {
	return a.chat.Send(ctx, conversationID, chatapp.SendInput{Content: content, Mentions: mentions})
}

// maxTriageRenderBytes caps the serialized execution record fed into the triage system prompt — a
// huge agent transcript or a node-heavy flowrun shouldn't blow the context. The steer already tells
// the LLM to use read/search tools for the full detail beyond this snapshot.
//
// maxTriageRenderBytes 限定喂进 triage system prompt 的序列化执行记录大小——巨大的 agent transcript 或节点很多的
// flowrun 不该撑爆上下文。steer 已告诉 LLM 超出此快照就用读取/搜索工具拿全。
const maxTriageRenderBytes = 6000

// executionRenderer resolves any execution record by its id prefix (S15 ID constitution) and
// serializes it for triage — function / handler / agent / flowrun / mcp call / trigger activation.
// Adding a type is one more case + that service's single-record read.
//
// executionRenderer 按 id 前缀（S15 ID 宪法）解析任意执行记录并为 triage 序列化——function/handler/agent/flowrun/
// mcp 调用/trigger activation。加类型 = 多一个 case + 那个 service 的单条读取。
type executionRenderer struct {
	fn  *functionapp.Service
	hd  *handlerapp.Service
	ag  *agentapp.Service
	sch *schedulerapp.Service
	mcp *mcpapp.Service
	trg *triggerapp.Service
}

func (r executionRenderer) Render(ctx context.Context, executionID string) (string, error) {
	prefix, _, _ := strings.Cut(executionID, "_")
	switch prefix {
	case "fne": // function execution
		rec, err := r.fn.GetExecution(ctx, executionID)
		if err != nil {
			return "", err
		}
		return renderExecution("function execution", rec), nil
	case "hcl": // handler call
		rec, err := r.hd.GetCall(ctx, executionID)
		if err != nil {
			return "", err
		}
		return renderExecution("handler call", rec), nil
	case "agx": // agent execution (carries the full ReAct transcript)
		rec, err := r.ag.GetExecutionDetail(ctx, executionID)
		if err != nil {
			return "", err
		}
		return renderExecution("agent execution", rec), nil
	case "fr": // workflow run (run + node records)
		run, nodes, err := r.sch.GetRunWithNodes(ctx, executionID)
		if err != nil {
			return "", err
		}
		return renderExecution("workflow run", map[string]any{"run": run, "nodes": nodes}), nil
	case "mcl": // mcp tool call
		rec, err := r.mcp.GetCall(ctx, executionID)
		if err != nil {
			return "", err
		}
		return renderExecution("mcp tool call", rec), nil
	case "tra": // trigger activation (the "did it fire, why/why not" action log)
		rec, err := r.trg.GetActivation(ctx, executionID)
		if err != nil {
			return "", err
		}
		return renderExecution("trigger activation", rec), nil
	default:
		return "", errorsdomain.New(errorsdomain.KindInvalid, "UNTRIAGEABLE_EXECUTION",
			"id prefix "+prefix+"_ is not a triageable execution type")
	}
}

func renderExecution(label string, v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		b = []byte("<unrenderable: " + err.Error() + ">")
	}
	body := string(b)
	if len(body) > maxTriageRenderBytes {
		body = body[:maxTriageRenderBytes] + "\n…(truncated — use the read/search tools for the full record)"
	}
	return label + ":\n" + body
}

// newAispawn assembles the iterate/triage engine from the conversation + chat services and a
// renderer over the four executable kinds.
//
// newAispawn 从 conversation + chat 服务 + 覆盖四类可执行体的渲染器组装 iterate/triage 引擎。
func newAispawn(s *services, log *zap.Logger) *aispawnapp.Service {
	return aispawnapp.New(
		aispawnConvStarter{conv: s.conversation},
		aispawnSender{chat: s.chat},
		executionRenderer{fn: s.function, hd: s.handler, ag: s.agent, sch: s.scheduler, mcp: s.mcp, trg: s.trigger},
		log,
	)
}
