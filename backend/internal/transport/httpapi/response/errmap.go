package response

import (
	"context"
	stderrors "errors"
	"net/http"

	"go.uber.org/zap"

	askapp "github.com/sunweilin/forgify/backend/internal/app/ask"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	catalogdomain "github.com/sunweilin/forgify/backend/internal/domain/catalog"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
	subagentdomain "github.com/sunweilin/forgify/backend/internal/domain/subagent"
	tododomain "github.com/sunweilin/forgify/backend/internal/domain/todo"
	cryptoinfra "github.com/sunweilin/forgify/backend/internal/infra/crypto"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// errMapping pairs a sentinel with its HTTP status and stable wire code.
//
// errMapping 把 sentinel 错误与 HTTP 状态码和对外错误码配对。
type errMapping struct {
	Status int
	Code   string
}

// errTable is the single source of truth for domain → HTTP translation.
// Adding a new domain error: declare sentinel in domain/<name>/errors.go,
// add a row here, done.
//
// errTable 是 domain → HTTP 翻译的唯一事实源。新增 domain 错误：
// 在 domain/<name>/errors.go 声明 sentinel，在本表加一行即可。
var errTable = map[error]errMapping{
	errorsdomain.ErrInvalidRequest: {http.StatusBadRequest, "INVALID_REQUEST"},

	// apikey domain / apikey domain 层
	apikeydomain.ErrNotFound:            {http.StatusNotFound, "API_KEY_NOT_FOUND"},
	apikeydomain.ErrNotFoundForProvider: {http.StatusNotFound, "API_KEY_PROVIDER_NOT_FOUND"},
	apikeydomain.ErrInvalidProvider:     {http.StatusBadRequest, "INVALID_PROVIDER"},
	apikeydomain.ErrBaseURLRequired:     {http.StatusBadRequest, "BASE_URL_REQUIRED"},
	apikeydomain.ErrAPIFormatRequired:   {http.StatusBadRequest, "API_FORMAT_REQUIRED"},
	apikeydomain.ErrKeyRequired:         {http.StatusBadRequest, "KEY_REQUIRED"},

	// conversation domain / conversation domain 层
	convdomain.ErrNotFound: {http.StatusNotFound, "CONVERSATION_NOT_FOUND"},

	// chat domain / chat domain 层
	chatdomain.ErrMessageNotFound:           {http.StatusNotFound, "MESSAGE_NOT_FOUND"},
	chatdomain.ErrStreamNotFound:            {http.StatusNotFound, "STREAM_NOT_FOUND"},
	chatdomain.ErrStreamInProgress:          {http.StatusConflict, "STREAM_IN_PROGRESS"},
	chatdomain.ErrAttachmentTooLarge:        {http.StatusRequestEntityTooLarge, "ATTACHMENT_TOO_LARGE"},
	chatdomain.ErrAttachmentTypeUnsupported: {http.StatusUnsupportedMediaType, "ATTACHMENT_TYPE_UNSUPPORTED"},
	chatdomain.ErrAttachmentParseFailed:     {http.StatusUnprocessableEntity, "ATTACHMENT_PARSE_FAILED"},

	// model domain / model domain 层
	modeldomain.ErrNotConfigured:    {http.StatusUnprocessableEntity, "MODEL_NOT_CONFIGURED"},
	modeldomain.ErrInvalidScenario:  {http.StatusBadRequest, "INVALID_SCENARIO"},
	modeldomain.ErrProviderRequired: {http.StatusBadRequest, "PROVIDER_REQUIRED"},
	modeldomain.ErrModelIDRequired:  {http.StatusBadRequest, "MODEL_ID_REQUIRED"},

	// function domain (forge_redesign trinity — primary Python tool surface) /
	// function domain
	functiondomain.ErrNotFound:             {http.StatusNotFound, "FUNCTION_NOT_FOUND"},
	functiondomain.ErrDuplicateName:        {http.StatusConflict, "FUNCTION_NAME_DUPLICATE"},
	functiondomain.ErrVersionNotFound:      {http.StatusNotFound, "FUNCTION_VERSION_NOT_FOUND"},
	functiondomain.ErrPendingNotFound:      {http.StatusNotFound, "FUNCTION_PENDING_NOT_FOUND"},
	functiondomain.ErrRunFailed:            {http.StatusUnprocessableEntity, "FUNCTION_RUN_FAILED"},
	functiondomain.ErrASTParseError:        {http.StatusUnprocessableEntity, "FUNCTION_AST_PARSE_FAILED"},
	functiondomain.ErrNoActiveVersion:      {http.StatusUnprocessableEntity, "FUNCTION_NO_ACTIVE_VERSION"},
	functiondomain.ErrEnvNotReady:          {http.StatusUnprocessableEntity, "FUNCTION_ENV_NOT_READY"},
	functiondomain.ErrEnvFailed:            {http.StatusUnprocessableEntity, "FUNCTION_ENV_FAILED"},
	functiondomain.ErrDependencyResolution: {http.StatusUnprocessableEntity, "FUNCTION_DEPENDENCY_RESOLUTION"},
	functiondomain.ErrSandboxUnavailable:   {http.StatusServiceUnavailable, "FUNCTION_SANDBOX_UNAVAILABLE"},
	functiondomain.ErrOpInvalid:            {http.StatusBadRequest, "FUNCTION_OP_INVALID"},
	functiondomain.ErrExecutionNotFound:    {http.StatusNotFound, "FUNCTION_EXECUTION_NOT_FOUND"},

	// handler domain (forge_redesign Plan 02 trinity second leg) /
	// handler domain (Plan 02 trinity 第二条腿)
	handlerdomain.ErrNotFound:            {http.StatusNotFound, "HANDLER_NOT_FOUND"},
	handlerdomain.ErrDuplicateName:       {http.StatusConflict, "HANDLER_NAME_DUPLICATE"},
	handlerdomain.ErrMethodNotFound:      {http.StatusNotFound, "HANDLER_METHOD_NOT_FOUND"},
	handlerdomain.ErrVersionNotFound:     {http.StatusNotFound, "HANDLER_VERSION_NOT_FOUND"},
	handlerdomain.ErrPendingNotFound:     {http.StatusNotFound, "HANDLER_PENDING_NOT_FOUND"},
	handlerdomain.ErrInstanceSpawnFailed: {http.StatusUnprocessableEntity, "HANDLER_INSTANCE_SPAWN_FAILED"},
	handlerdomain.ErrInstanceCrashed:     {http.StatusUnprocessableEntity, "HANDLER_INSTANCE_CRASHED"},
	handlerdomain.ErrInstanceRPCTimeout:  {http.StatusGatewayTimeout, "HANDLER_INSTANCE_RPC_TIMEOUT"},
	handlerdomain.ErrInstanceNotFound:    {http.StatusNotFound, "HANDLER_INSTANCE_NOT_FOUND"},
	handlerdomain.ErrNoActiveVersion:     {http.StatusUnprocessableEntity, "HANDLER_NO_ACTIVE_VERSION"},
	handlerdomain.ErrEnvNotReady:         {http.StatusUnprocessableEntity, "HANDLER_ENV_NOT_READY"},
	handlerdomain.ErrEnvFailed:           {http.StatusUnprocessableEntity, "HANDLER_ENV_FAILED"},
	handlerdomain.ErrSandboxUnavailable:  {http.StatusServiceUnavailable, "HANDLER_SANDBOX_UNAVAILABLE"},
	handlerdomain.ErrOpInvalid:           {http.StatusBadRequest, "HANDLER_OP_INVALID"},
	handlerdomain.ErrASTParseError:       {http.StatusUnprocessableEntity, "HANDLER_AST_PARSE_FAILED"},
	handlerdomain.ErrConfigIncomplete:    {http.StatusUnprocessableEntity, "HANDLER_CONFIG_INCOMPLETE"},
	handlerdomain.ErrConfigInvalid:       {http.StatusBadRequest, "HANDLER_CONFIG_INVALID"},
	handlerdomain.ErrConfigDecryptFailed: {http.StatusInternalServerError, "HANDLER_CONFIG_DECRYPT_FAILED"},
	handlerdomain.ErrCallNotFound:        {http.StatusNotFound, "HANDLER_CALL_NOT_FOUND"},

	// todo domain / todo domain 层
	tododomain.ErrNotFound:        {http.StatusNotFound, "TODO_NOT_FOUND"},
	tododomain.ErrSubjectRequired: {http.StatusBadRequest, "TODO_SUBJECT_REQUIRED"},
	tododomain.ErrInvalidStatus:   {http.StatusBadRequest, "TODO_INVALID_STATUS"},

	// sandbox domain / sandbox domain 层
	// 8 sentinels per sandbox.md §5; status mapping follows error-codes.md table.
	sandboxdomain.ErrRuntimeNotSupported:  {http.StatusUnprocessableEntity, "SANDBOX_RUNTIME_NOT_SUPPORTED"},
	sandboxdomain.ErrRuntimeInstallFailed: {http.StatusBadGateway, "SANDBOX_RUNTIME_INSTALL_FAILED"},
	sandboxdomain.ErrEnvNotFound:          {http.StatusNotFound, "SANDBOX_ENV_NOT_FOUND"},
	sandboxdomain.ErrEnvCreateFailed:      {http.StatusBadGateway, "SANDBOX_ENV_CREATE_FAILED"},
	sandboxdomain.ErrDepInstallFailed:     {http.StatusBadGateway, "SANDBOX_DEP_INSTALL_FAILED"},
	sandboxdomain.ErrSpawnFailed:          {http.StatusBadGateway, "SANDBOX_SPAWN_FAILED"},
	sandboxdomain.ErrSpawnTimeout:         {http.StatusGatewayTimeout, "SANDBOX_SPAWN_TIMEOUT"},
	sandboxdomain.ErrEnvInUse:             {http.StatusConflict, "SANDBOX_ENV_IN_USE"},
	sandboxdomain.ErrInvalidOwnerID:       {http.StatusBadRequest, "SANDBOX_INVALID_OWNER_ID"},
	sandboxdomain.ErrCmdRequired:          {http.StatusBadRequest, "SANDBOX_CMD_REQUIRED"},
	// Phase 5 docker sentinels — pre-registered so future docker-runtime
	// integration won't trigger "unmapped domain error" warnings on first
	// touch. 0 current consumers; sentinels live in domain/sandbox/sandbox.go.
	//
	// Phase 5 docker sentinel——预登记防未来 docker-runtime 接入时触发
	// "unmapped domain error"。当前 0 消费者；sentinel 在
	// domain/sandbox/sandbox.go。
	sandboxdomain.ErrDockerNotInstalled: {http.StatusUnprocessableEntity, "SANDBOX_DOCKER_NOT_INSTALLED"},
	sandboxdomain.ErrDockerDaemonDown:   {http.StatusServiceUnavailable, "SANDBOX_DOCKER_DAEMON_DOWN"},

	// subagent domain / subagent domain 层
	// Only these two are real Go sentinels reaching handlers. Max-turns
	// + cancellation surface as `subagentapp.StatusMaxTurns` /
	// `StatusCancelled` string constants on `SpawnResult.Status`, not
	// errors — SubagentTool.Execute renders them as friendly tool_result
	// text, so they never enter the error path / errmap.
	//
	// 只有这两个是真 Go sentinel 会到 handler。Max-turns + 取消是
	// `subagentapp.StatusMaxTurns` / `StatusCancelled` 字符串常量挂
	// `SpawnResult.Status`，不是 error；SubagentTool.Execute 渲染为
	// 友好 tool_result 文本，不进 error 路径 / errmap。
	subagentdomain.ErrTypeNotFound:     {http.StatusNotFound, "SUBAGENT_TYPE_NOT_FOUND"},
	subagentdomain.ErrRecursionAttempt: {http.StatusUnprocessableEntity, "SUBAGENT_RECURSION"},

	// catalog domain / catalog domain 层
	// Reachable via POST /api/v1/catalog:refresh. ErrCoverageIncomplete
	// + ErrGenerationFailed are absorbed inside Service.Refresh
	// (mechanical fallback) — they never reach handler. Only
	// ErrAllSourcesFailed surfaces when every configured source errors
	// simultaneously.
	//
	// catalog domain 层。POST /api/v1/catalog:refresh 触达。
	// ErrCoverageIncomplete + ErrGenerationFailed 在 Service.Refresh 内
	// 被 mechanical fallback 吞掉，不到 handler。只有 ErrAllSourcesFailed
	// 在所有 source 同时挂时上抛。
	catalogdomain.ErrAllSourcesFailed: {http.StatusServiceUnavailable, "CATALOG_ALL_SOURCES_FAILED"},

	// mcp domain / mcp domain 层
	// 5 runtime sentinels (Server* / Tool*) + 5 Registry-flow sentinels.
	// `ErrToolCallFailed` / `ErrInstallFailed` use 502 because the
	// failure originates outside our process (server subprocess /
	// package manager). Per mcp.md §11.
	//
	// 5 个 runtime sentinel + 5 个 Registry-flow sentinel。ErrToolCallFailed
	// / ErrInstallFailed 用 502——失败来源在进程外（server 子进程 / 包管理器）。
	mcpdomain.ErrServerNotFound:        {http.StatusNotFound, "MCP_SERVER_NOT_FOUND"},
	mcpdomain.ErrServerNotConnected:    {http.StatusConflict, "MCP_SERVER_NOT_CONNECTED"},
	mcpdomain.ErrToolNotFound:          {http.StatusNotFound, "MCP_TOOL_NOT_FOUND"},
	mcpdomain.ErrToolCallFailed:        {http.StatusBadGateway, "MCP_TOOL_CALL_FAILED"},
	mcpdomain.ErrToolCallTimeout:       {http.StatusGatewayTimeout, "MCP_TOOL_CALL_TIMEOUT"},
	mcpdomain.ErrRegistryEntryNotFound: {http.StatusNotFound, "MCP_REGISTRY_ENTRY_NOT_FOUND"},
	mcpdomain.ErrRequiredEnvMissing:    {http.StatusUnprocessableEntity, "MCP_REQUIRED_ENV_MISSING"},
	mcpdomain.ErrRequiredArgsMissing:   {http.StatusUnprocessableEntity, "MCP_REQUIRED_ARGS_MISSING"},
	mcpdomain.ErrInstallFailed:         {http.StatusBadGateway, "MCP_INSTALL_FAILED"},
	// Marketplace V2 (2026-05-08): added when official MCP Registry was wired in.
	// Marketplace V2（2026-05-08）：接入官方 MCP Registry 时加。
	mcpdomain.ErrAlreadyInstalled: {http.StatusConflict, "MCP_ALREADY_INSTALLED"},

	// skill domain (V1.2 D7) / skill domain
	skilldomain.ErrSkillNotFound:      {http.StatusNotFound, "SKILL_NOT_FOUND"},
	skilldomain.ErrInvalidFrontmatter: {http.StatusUnprocessableEntity, "SKILL_INVALID_FRONTMATTER"},
	skilldomain.ErrBodyTooLarge:       {http.StatusUnprocessableEntity, "SKILL_BODY_TOO_LARGE"},
	skilldomain.ErrNameConflict:       {http.StatusConflict, "SKILL_NAME_CONFLICT"},
	skilldomain.ErrInvalidName:        {http.StatusUnprocessableEntity, "SKILL_INVALID_NAME"},

	// ask service (AskUserQuestion answer-delivery handler) /
	// ask service（AskUserQuestion 答案投递 handler）
	askapp.ErrNoPendingQuestion: {http.StatusNotFound, "ASK_NO_PENDING_QUESTION"},
	askapp.ErrTimeout:           {http.StatusGatewayTimeout, "ASK_TIMEOUT"},

	// Cross-cutting: explicitly registered to suppress the "unmapped domain
	// error" warning while still returning 500. Both represent server-side
	// state that the user can't recover from.
	//
	// 跨层 sentinel：显式登记以抑制"unmapped domain error"警告，
	// 同时仍返回 500。两者都代表用户无法自行恢复的服务端状态。
	reqctxpkg.ErrMissingUserID:         {http.StatusInternalServerError, "INTERNAL_ERROR"},
	reqctxpkg.ErrMissingConversationID: {http.StatusInternalServerError, "INTERNAL_ERROR"},
	cryptoinfra.ErrUnsupportedVersion:  {http.StatusInternalServerError, "INTERNAL_ERROR"},

	// LLM provider HTTP-status sentinels — wrapped by classifyHTTPError +
	// in-stream chunk.Error path in infra/llm. Letting callers
	// errors.Is() these enables paths like apikey.MarkInvalid on 401/403
	// (key auto-flips to "error" in UI) and provider-specific UX.
	//
	// LLM provider HTTP 状态分类 sentinel——infra/llm 的 classifyHTTPError +
	// 流内 chunk.Error 包装。调用方 errors.Is() 能力让 401/403 触发
	// apikey.MarkInvalid（UI 自动翻"error"）等路径成立。
	llminfra.ErrAuthFailed:    {http.StatusUnauthorized, "LLM_AUTH_FAILED"},
	llminfra.ErrRateLimited:   {http.StatusTooManyRequests, "LLM_RATE_LIMITED"},
	llminfra.ErrBadRequest:    {http.StatusBadRequest, "LLM_BAD_REQUEST"},
	llminfra.ErrModelNotFound: {http.StatusNotFound, "LLM_MODEL_NOT_FOUND"},
	llminfra.ErrProviderError: {http.StatusBadGateway, "LLM_PROVIDER_ERROR"},

	// BYOK web-search providers (Brave / Serper / Tavily / Bocha) sentinels
	// (webtool.ErrAuthFailed / ErrRateLimited / ErrUpstreamHTTP) are
	// control-flow only — `WebSearch.Execute::tryBYOKProvider` catches
	// them internally to fall through to the next provider / MCP tier.
	// They never reach FromDomainError, so no errmap registration is
	// needed (D-redo fix 2026-05-11 removed prior entries).
	//
	// BYOK web 搜索 provider sentinel 是控制流 sentinel——`WebSearch.Execute`
	// 内部 catch 后落到下一 provider / MCP。永不到 FromDomainError，故
	// 不需 errmap 注册（D-redo fix 2026-05-11 删除原条目）。

	// Standard library context errors. Browser hard-refresh / tab close
	// cancels r.Context(), which propagates up through every store call
	// and reaches handlers as ctx-canceled. These are NOT bugs — the
	// client just left — so they must be mapped (suppress the "unmapped
	// domain error" alarm) but the response goes nowhere either way.
	// 499 (nginx convention) = client closed request; 504 = upstream
	// timeout we couldn't beat. Both surfaced here as no-op responses
	// that at least avoid log noise pretending we have an internal bug.
	//
	// 标准库 context 错误。浏览器 hard refresh / 关 tab 取消 r.Context()，
	// 一路冒泡到 handler。不是 bug——客户端走了——但需要登记免触发
	// "unmapped domain error" 警报。响应反正没人收。
	context.Canceled:         {499, "CLIENT_CLOSED"},
	context.DeadlineExceeded: {http.StatusGatewayTimeout, "REQUEST_TIMEOUT"},
}

// FromDomainError translates a domain error to an HTTP envelope via errTable.
// Unmapped errors → 500 INTERNAL_ERROR; raw message is suppressed to
// prevent leaking implementation details.
//
// FromDomainError 通过 errTable 把 domain 错误翻译为 HTTP envelope。
// 未映射的错误 → 500 INTERNAL_ERROR；原始消息被隐藏，防止泄漏实现细节。
func FromDomainError(w http.ResponseWriter, log *zap.Logger, err error) {
	m, matched := lookup(err)
	msg := err.Error()
	if !matched {
		log.Error("unmapped domain error",
			zap.Error(err),
			zap.String("fallback_code", m.Code),
		)
		msg = "internal error"
	}
	Error(w, m.Status, m.Code, msg, nil)
}

// lookup walks errTable with errors.Is so wrapped errors still match.
//
// lookup 用 errors.Is 遍历 errTable，包裹过的错误也能匹配。
func lookup(err error) (errMapping, bool) {
	for sentinel, m := range errTable {
		if stderrors.Is(err, sentinel) {
			return m, true
		}
	}
	return errMapping{http.StatusInternalServerError, "INTERNAL_ERROR"}, false
}
