package response

import (
	stderrors "errors"
	"net/http"

	"go.uber.org/zap"

	askapp "github.com/sunweilin/forgify/backend/internal/app/ask"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
	subagentdomain "github.com/sunweilin/forgify/backend/internal/domain/subagent"
	tododomain "github.com/sunweilin/forgify/backend/internal/domain/todo"
	cryptoinfra "github.com/sunweilin/forgify/backend/internal/infra/crypto"
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
	errorsdomain.ErrInternal:       {http.StatusInternalServerError, "INTERNAL_ERROR"},

	// apikey domain / apikey domain 层
	apikeydomain.ErrNotFound:            {http.StatusNotFound, "API_KEY_NOT_FOUND"},
	apikeydomain.ErrNotFoundForProvider: {http.StatusNotFound, "API_KEY_PROVIDER_NOT_FOUND"},
	apikeydomain.ErrInvalidProvider:     {http.StatusBadRequest, "INVALID_PROVIDER"},
	apikeydomain.ErrBaseURLRequired:     {http.StatusBadRequest, "BASE_URL_REQUIRED"},
	apikeydomain.ErrAPIFormatRequired:   {http.StatusBadRequest, "API_FORMAT_REQUIRED"},
	apikeydomain.ErrKeyRequired:         {http.StatusBadRequest, "KEY_REQUIRED"},
	apikeydomain.ErrTestFailed:          {http.StatusUnprocessableEntity, "API_KEY_TEST_FAILED"},
	apikeydomain.ErrInvalid:             {http.StatusUnauthorized, "API_KEY_INVALID"},

	// conversation domain / conversation domain 层
	convdomain.ErrNotFound: {http.StatusNotFound, "CONVERSATION_NOT_FOUND"},

	// chat domain / chat domain 层
	chatdomain.ErrMessageNotFound:           {http.StatusNotFound, "MESSAGE_NOT_FOUND"},
	chatdomain.ErrStreamNotFound:            {http.StatusNotFound, "STREAM_NOT_FOUND"},
	chatdomain.ErrStreamInProgress:          {http.StatusConflict, "STREAM_IN_PROGRESS"},
	chatdomain.ErrProviderUnavailable:       {http.StatusBadGateway, "LLM_PROVIDER_ERROR"},
	chatdomain.ErrAttachmentTooLarge:        {http.StatusRequestEntityTooLarge, "ATTACHMENT_TOO_LARGE"},
	chatdomain.ErrAttachmentTypeUnsupported: {http.StatusUnsupportedMediaType, "ATTACHMENT_TYPE_UNSUPPORTED"},
	chatdomain.ErrAttachmentParseFailed:     {http.StatusUnprocessableEntity, "ATTACHMENT_PARSE_FAILED"},
	chatdomain.ErrVisionNotSupported:        {http.StatusUnprocessableEntity, "VISION_NOT_SUPPORTED"},

	// model domain / model domain 层
	modeldomain.ErrNotConfigured:    {http.StatusUnprocessableEntity, "MODEL_NOT_CONFIGURED"},
	modeldomain.ErrInvalidScenario:  {http.StatusBadRequest, "INVALID_SCENARIO"},
	modeldomain.ErrProviderRequired: {http.StatusBadRequest, "PROVIDER_REQUIRED"},
	modeldomain.ErrModelIDRequired:  {http.StatusBadRequest, "MODEL_ID_REQUIRED"},

	// forge domain / forge domain 层
	// (TOOL_* wire codes preserved from Phase 1 for client compatibility;
	// new env / sandbox sentinels use FORGE_* per sandbox iteration §12.)
	// (TOOL_* wire code 来自 Phase 1，为客户端兼容保留；新 env / sandbox
	// sentinel 按沙箱迭代 §12 用 FORGE_* 前缀。)
	forgedomain.ErrNotFound:             {http.StatusNotFound, "TOOL_NOT_FOUND"},
	forgedomain.ErrDuplicateName:        {http.StatusConflict, "TOOL_NAME_DUPLICATE"},
	forgedomain.ErrVersionNotFound:      {http.StatusNotFound, "TOOL_VERSION_NOT_FOUND"},
	forgedomain.ErrPendingNotFound:      {http.StatusNotFound, "TOOL_PENDING_NOT_FOUND"},
	forgedomain.ErrPendingConflict:      {http.StatusConflict, "TOOL_PENDING_CONFLICT"},
	forgedomain.ErrTestCaseNotFound:     {http.StatusNotFound, "TOOL_TEST_CASE_NOT_FOUND"},
	forgedomain.ErrRunFailed:            {http.StatusUnprocessableEntity, "TOOL_RUN_FAILED"},
	forgedomain.ErrASTParseError:        {http.StatusUnprocessableEntity, "TOOL_AST_PARSE_FAILED"},
	forgedomain.ErrImportInvalid:        {http.StatusBadRequest, "TOOL_IMPORT_INVALID"},
	forgedomain.ErrEnvNotReady:          {http.StatusUnprocessableEntity, "FORGE_ENV_NOT_READY"},
	forgedomain.ErrNoActiveVersion:      {http.StatusUnprocessableEntity, "FORGE_NO_ACTIVE_VERSION"},
	forgedomain.ErrEnvFailed:            {http.StatusUnprocessableEntity, "FORGE_ENV_FAILED"},
	forgedomain.ErrSandboxUnavailable:   {http.StatusServiceUnavailable, "FORGE_SANDBOX_UNAVAILABLE"},
	forgedomain.ErrDependencyResolution: {http.StatusUnprocessableEntity, "FORGE_DEPENDENCY_RESOLUTION"},

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

	// subagent domain / subagent domain 层
	// Only the first two reach handlers; ErrMaxTurnsExceeded / ErrCancelled
	// are converted to friendly tool_result strings by SubagentTool.Execute
	// and never propagate (run.Status reflects them).
	// 只有前两个会到 handler；ErrMaxTurnsExceeded / ErrCancelled 在
	// SubagentTool.Execute 内转友好 tool_result 字符串，不上抛
	// （run.Status 已反映）。
	subagentdomain.ErrTypeNotFound:     {http.StatusNotFound, "SUBAGENT_TYPE_NOT_FOUND"},
	subagentdomain.ErrRecursionAttempt: {http.StatusUnprocessableEntity, "SUBAGENT_RECURSION"},

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
	mcpdomain.ErrRuntimeMissing:        {http.StatusUnprocessableEntity, "MCP_RUNTIME_MISSING"},
	mcpdomain.ErrRequiredEnvMissing:    {http.StatusUnprocessableEntity, "MCP_REQUIRED_ENV_MISSING"},
	mcpdomain.ErrRequiredArgsMissing:   {http.StatusUnprocessableEntity, "MCP_REQUIRED_ARGS_MISSING"},
	mcpdomain.ErrInstallFailed:         {http.StatusBadGateway, "MCP_INSTALL_FAILED"},
	// Marketplace V2 (2026-05-08): added when official MCP Registry was wired in.
	// Marketplace V2（2026-05-08）：接入官方 MCP Registry 时加。
	mcpdomain.ErrMarketplaceUnavailable: {http.StatusBadGateway, "MCP_MARKETPLACE_UNAVAILABLE"},
	mcpdomain.ErrQueryRequired:          {http.StatusBadRequest, "MCP_QUERY_REQUIRED"},
	mcpdomain.ErrAlreadyInstalled:       {http.StatusConflict, "MCP_ALREADY_INSTALLED"},
	mcpdomain.ErrUnsupportedRuntime:     {http.StatusUnprocessableEntity, "MCP_UNSUPPORTED_RUNTIME"},
	mcpdomain.ErrHandshakeFailed:        {http.StatusBadGateway, "MCP_HANDSHAKE_FAILED"},

	// skill domain (V1.2 D7) / skill domain
	skilldomain.ErrSkillNotFound:      {http.StatusNotFound, "SKILL_NOT_FOUND"},
	skilldomain.ErrInvalidFrontmatter: {http.StatusUnprocessableEntity, "SKILL_INVALID_FRONTMATTER"},
	skilldomain.ErrBodyTooLarge:       {http.StatusUnprocessableEntity, "SKILL_BODY_TOO_LARGE"},
	skilldomain.ErrNameConflict:       {http.StatusConflict, "SKILL_NAME_CONFLICT"},
	skilldomain.ErrInvalidName:        {http.StatusUnprocessableEntity, "SKILL_INVALID_NAME"},

	// ask service (AskUserQuestion answer-delivery handler) /
	// ask service（AskUserQuestion 答案投递 handler）
	askapp.ErrNoPendingQuestion: {http.StatusNotFound, "ASK_NO_PENDING_QUESTION"},
	askapp.ErrAlreadyAnswered:   {http.StatusConflict, "ASK_ALREADY_ANSWERED"},
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
