// Package bootstrap is the composition root: it constructs every Service, wires the cross-Service
// adapters that satisfy each consumer's DIP port, assembles the tool registry + HTTP router, and
// runs the boot/shutdown lifecycle. It is the one place allowed to import across the whole app —
// nothing imports bootstrap, so this central coupling forms no cycle. Adapters live here (not in
// the provider packages) precisely so a provider like model never imports a consumer like chat.
//
// Package bootstrap 是 composition root：构造每个 Service、焊接满足各消费者 DIP 端口的跨 Service
// 适配器、装配工具表 + HTTP router、跑 boot/shutdown 生命周期。它是唯一允许横跨整个 app import 的
// 地方——无人 import bootstrap，故此处中央耦合不成环。适配器放这里（而非 provider 包），正是为了让
// model 这样的 provider 永不 import chat 这样的 consumer。
package bootstrap

import (
	"context"

	agentapp "github.com/sunweilin/forgify/backend/internal/app/agent"
	chatapp "github.com/sunweilin/forgify/backend/internal/app/chat"
	contextmgrapp "github.com/sunweilin/forgify/backend/internal/app/contextmgr"
	modelclientapp "github.com/sunweilin/forgify/backend/internal/app/modelclient"
	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// CredsResolver is the slice of apikey.Service the model resolvers need: an api-key id → decrypted
// credentials (provider + plaintext key + base url + wire format). *apikeyapp.Service satisfies it.
//
// CredsResolver 是 model resolver 需要的 apikey.Service 切片：api-key id → 解密凭证（provider +
// 明文 key + base url + wire 方言）。*apikeyapp.Service 满足它。
type CredsResolver interface {
	ResolveCredentialsByID(ctx context.Context, apiKeyID string) (apikeydomain.Credentials, error)
}

// modelResolver is the shared model→client core every per-consumer resolver wraps: it runs the one
// resolution chain (model.Resolve picks the ref → apikey decrypts creds → llm.Factory builds the
// client + base url → a pre-filled Request). This is the chain that was only ever a port + test
// fake before M7 — bootstrap is where it's finally implemented, once.
//
// modelResolver 是每个 per-consumer resolver 包的共享 model→client 核：跑唯一的解析链（model.Resolve
// 选 ref → apikey 解密 creds → llm.Factory 造 client + base url → 预填 Request）。这条链在 M7 前只是
// 端口 + 测试 fake——bootstrap 是它终于被实现的地方，仅一次。
type modelResolver struct {
	picker  modeldomain.ModelPicker
	keys    CredsResolver
	factory *llminfra.Factory
}

// resolve runs the chain for a scenario (+ optional override) and returns a ready client, a base
// Request (System/Messages filled later by the caller), and the resolved provider (turn provenance).
//
// resolve 为某 scenario（+ 可选 override）跑链，返回即用 client、base Request（System/Messages 由
// caller 后填）、解析出的 provider（回合溯源）。
func (r *modelResolver) resolve(ctx context.Context, scenario string, override *modeldomain.ModelRef) (llminfra.Client, llminfra.Request, string, error) {
	return modelclientapp.Resolve(ctx, scenario, override, r.picker, r.keys, r.factory)
}

// ModelResolvers exposes the one resolution core as the four differently-shaped Bundles the LLM
// consumers expect. One core, four typed accessors — no resolution logic is duplicated.
//
// ModelResolvers 把同一个解析核暴露成 LLM 消费者各自期望的四种 Bundle 形状。一核四口——解析逻辑不重复。
type ModelResolvers struct {
	core   *modelResolver
	lookup ModelInfoLookup // for chat's content capabilities (vision/native-docs per resolved model)
}

// NewModelResolvers builds the shared core from the workspace picker, the apikey credential
// resolver, and the llm factory; lookup supplies chat's per-model content capabilities.
//
// NewModelResolvers 由 workspace picker、apikey 凭证解析、llm factory 构造共享核；lookup 供 chat 的
// per-model 内容能力。
func NewModelResolvers(picker modeldomain.ModelPicker, keys CredsResolver, factory *llminfra.Factory, lookup ModelInfoLookup) ModelResolvers {
	return ModelResolvers{core: &modelResolver{picker: picker, keys: keys, factory: factory}, lookup: lookup}
}

// Chat / ContextmgrUtility / Subagent / Agent return the resolver each Service's port wants.
//
// Chat / ContextmgrUtility / Subagent / Agent 返回各 Service 端口要的 resolver。
func (m ModelResolvers) Chat() chatapp.ModelResolver {
	return chatResolver{core: m.core, lookup: m.lookup}
}
func (m ModelResolvers) ContextmgrUtility() contextmgrapp.UtilityResolver {
	return contextmgrResolver{m.core}
}
func (m ModelResolvers) Subagent() subagentapp.ModelResolver { return subagentResolver{m.core} }
func (m ModelResolvers) Agent() agentapp.LLMResolver         { return agentResolver{m.core} }

// --- chat (dialogue + utility; Caps carries the model's content abilities) ---

type chatResolver struct {
	core   *modelResolver
	lookup ModelInfoLookup
}

var _ chatapp.ModelResolver = chatResolver{}

func (r chatResolver) ResolveChat(ctx context.Context, override *modeldomain.ModelRef) (chatapp.Bundle, error) {
	return r.bundle(ctx, modeldomain.ScenarioDialogue, override)
}

func (r chatResolver) ResolveUtility(ctx context.Context) (chatapp.Bundle, error) {
	return r.bundle(ctx, modeldomain.ScenarioUtility, nil)
}

func (r chatResolver) bundle(ctx context.Context, scenario string, override *modeldomain.ModelRef) (chatapp.Bundle, error) {
	client, req, provider, err := r.core.resolve(ctx, scenario, override)
	if err != nil {
		return chatapp.Bundle{}, err
	}
	// Caps comes from the resolved model's catalog entry (vision / native-docs); an unknown model
	// yields zero caps, so chat renders attachments conservatively rather than over-claiming.
	//
	// Caps 取自解析出的模型目录项（vision / native-docs）；未知模型得零 caps，chat 保守渲染附件而非
	// 过度声明。
	caps := r.lookup.contentCaps(ctx, provider, req.ModelID)
	return chatapp.Bundle{Client: client, Request: req, Caps: caps, Provider: provider}, nil
}

// --- contextmgr (utility model for the compaction summary) ---

type contextmgrResolver struct{ core *modelResolver }

var _ contextmgrapp.UtilityResolver = contextmgrResolver{}

func (r contextmgrResolver) ResolveUtility(ctx context.Context) (contextmgrapp.Bundle, error) {
	client, req, _, err := r.core.resolve(ctx, modeldomain.ScenarioUtility, nil)
	if err != nil {
		return contextmgrapp.Bundle{}, err
	}
	return contextmgrapp.Bundle{Client: client, Request: req}, nil
}

// --- subagent (dialogue model; no per-conversation override inheritance, M5.2+) ---

type subagentResolver struct{ core *modelResolver }

var _ subagentapp.ModelResolver = subagentResolver{}

func (r subagentResolver) Resolve(ctx context.Context) (subagentapp.Bundle, error) {
	client, req, provider, err := r.core.resolve(ctx, modeldomain.ScenarioDialogue, nil)
	if err != nil {
		return subagentapp.Bundle{}, err
	}
	return subagentapp.Bundle{Client: client, Request: req, Provider: provider}, nil
}

// --- agent (agent scenario; override = the invoked agent's pinned model) ---

type agentResolver struct{ core *modelResolver }

var _ agentapp.LLMResolver = agentResolver{}

func (r agentResolver) ResolveAgent(ctx context.Context, override *modeldomain.ModelRef) (agentapp.LLMBundle, error) {
	client, req, _, err := r.core.resolve(ctx, modeldomain.ScenarioAgent, override)
	if err != nil {
		return agentapp.LLMBundle{}, err
	}
	return agentapp.LLMBundle{Client: client, Request: req}, nil
}
