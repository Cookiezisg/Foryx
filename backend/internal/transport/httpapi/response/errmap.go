package response

import (
	"context"
	stderrors "errors"
	"net/http"

	"go.uber.org/zap"

	askapp "github.com/sunweilin/forgify/backend/internal/app/ask"
	schedulerapp "github.com/sunweilin/forgify/backend/internal/app/scheduler"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	catalogdomain "github.com/sunweilin/forgify/backend/internal/domain/catalog"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	documentdomain "github.com/sunweilin/forgify/backend/internal/domain/document"
	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
	subagentdomain "github.com/sunweilin/forgify/backend/internal/domain/subagent"
	tododomain "github.com/sunweilin/forgify/backend/internal/domain/todo"
	userdomain "github.com/sunweilin/forgify/backend/internal/domain/user"
	cryptoinfra "github.com/sunweilin/forgify/backend/internal/infra/crypto"
	handlerinfra "github.com/sunweilin/forgify/backend/internal/infra/handler"
	memorydomain "github.com/sunweilin/forgify/backend/internal/domain/memory"
	permdomain "github.com/sunweilin/forgify/backend/internal/domain/permissions"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// errMapping pairs a sentinel with HTTP status + stable wire code.
//
// errMapping 把 sentinel 与 HTTP 状态码和对外错误码配对。
type errMapping struct {
	Status int
	Code   string
}

// errTable is the single source of truth for domain → HTTP translation.
//
// errTable 是 domain → HTTP 翻译的唯一事实源。
var errTable = map[error]errMapping{
	errorsdomain.ErrInvalidRequest: {http.StatusBadRequest, "INVALID_REQUEST"},

	// apikey
	apikeydomain.ErrNotFound:            {http.StatusNotFound, "API_KEY_NOT_FOUND"},
	apikeydomain.ErrNotFoundForProvider: {http.StatusNotFound, "API_KEY_PROVIDER_NOT_FOUND"},
	apikeydomain.ErrInvalidProvider:     {http.StatusBadRequest, "INVALID_PROVIDER"},
	apikeydomain.ErrBaseURLRequired:     {http.StatusBadRequest, "BASE_URL_REQUIRED"},
	apikeydomain.ErrAPIFormatRequired:   {http.StatusBadRequest, "API_FORMAT_REQUIRED"},
	apikeydomain.ErrKeyRequired:         {http.StatusBadRequest, "KEY_REQUIRED"},
	apikeydomain.ErrDisplayNameConflict: {http.StatusConflict, "API_KEY_NAME_CONFLICT"},

	// conversation
	convdomain.ErrNotFound: {http.StatusNotFound, "CONVERSATION_NOT_FOUND"},

	// chat
	chatdomain.ErrMessageNotFound:           {http.StatusNotFound, "MESSAGE_NOT_FOUND"},
	chatdomain.ErrStreamNotFound:            {http.StatusNotFound, "STREAM_NOT_FOUND"},
	chatdomain.ErrStreamInProgress:          {http.StatusConflict, "STREAM_IN_PROGRESS"},
	chatdomain.ErrAttachmentTooLarge:        {http.StatusRequestEntityTooLarge, "ATTACHMENT_TOO_LARGE"},
	chatdomain.ErrAttachmentTypeUnsupported: {http.StatusUnsupportedMediaType, "ATTACHMENT_TYPE_UNSUPPORTED"},
	chatdomain.ErrAttachmentParseFailed:     {http.StatusUnprocessableEntity, "ATTACHMENT_PARSE_FAILED"},
	chatdomain.ErrAttachmentNotFound:        {http.StatusNotFound, "ATTACHMENT_NOT_FOUND"},
	chatdomain.ErrEmptyContent:              {http.StatusBadRequest, "EMPTY_CONTENT"},

	// model
	modeldomain.ErrNotConfigured:    {http.StatusUnprocessableEntity, "MODEL_NOT_CONFIGURED"},
	modeldomain.ErrInvalidScenario:  {http.StatusBadRequest, "INVALID_SCENARIO"},
	modeldomain.ErrProviderRequired: {http.StatusBadRequest, "PROVIDER_REQUIRED"},
	modeldomain.ErrModelIDRequired:  {http.StatusBadRequest, "MODEL_ID_REQUIRED"},
	modeldomain.ErrProviderHasNoKey: {http.StatusUnprocessableEntity, "PROVIDER_HAS_NO_KEY"},

	// function
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

	// handler
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

	// handler infra subprocess errors
	handlerinfra.ErrCallFailed:     {http.StatusUnprocessableEntity, "HANDLER_CALL_FAILED"},
	handlerinfra.ErrInitFailed:     {http.StatusUnprocessableEntity, "HANDLER_INIT_FAILED"},
	handlerinfra.ErrCrashed:        {http.StatusUnprocessableEntity, "HANDLER_INSTANCE_CRASHED_INFRA"},
	handlerinfra.ErrProtocol:       {http.StatusInternalServerError, "HANDLER_PROTOCOL_ERROR"},
	handlerinfra.ErrShutdownAlready: {http.StatusUnprocessableEntity, "HANDLER_SHUTDOWN_ALREADY"},

	// flowrun + trigger + scheduler
	flowrundomain.ErrNotFound:                {http.StatusNotFound, "FLOWRUN_NOT_FOUND"},
	flowrundomain.ErrNotCancellable:          {http.StatusUnprocessableEntity, "FLOWRUN_NOT_CANCELLABLE"},
	flowrundomain.ErrNotPaused:               {http.StatusUnprocessableEntity, "FLOWRUN_NOT_PAUSED"},
	flowrundomain.ErrApprovalNodeNotFound:    {http.StatusNotFound, "FLOWRUN_APPROVAL_NODE_NOT_FOUND"},
	flowrundomain.ErrApprovalDecisionInvalid: {http.StatusBadRequest, "FLOWRUN_APPROVAL_DECISION_INVALID"},
	flowrundomain.ErrNodeNotFound:            {http.StatusNotFound, "FLOWRUN_NODE_NOT_FOUND"},

	triggerdomain.ErrPathNotExist:          {http.StatusUnprocessableEntity, "TRIGGER_PATH_NOT_EXIST"},
	triggerdomain.ErrPathConflict:          {http.StatusConflict, "TRIGGER_PATH_CONFLICT"},
	triggerdomain.ErrWebhookSecretMismatch: {http.StatusUnauthorized, "TRIGGER_WEBHOOK_SECRET_MISMATCH"},
	triggerdomain.ErrInvalidCronExpression: {http.StatusBadRequest, "TRIGGER_INVALID_CRON_EXPRESSION"},

	schedulerapp.ErrWorkflowDisabled:       {http.StatusUnprocessableEntity, "WORKFLOW_DISABLED"},
	schedulerapp.ErrWorkflowNeedsAttention: {http.StatusUnprocessableEntity, "WORKFLOW_NEEDS_ATTENTION"},
	schedulerapp.ErrConcurrencyLimit:       {http.StatusConflict, "FLOWRUN_CONCURRENCY_LIMIT"},
	schedulerapp.ErrWorkflowNotFound:       {http.StatusNotFound, "WORKFLOW_NOT_FOUND_FOR_TRIGGER"},

	// workflow
	workflowdomain.ErrNotFound:              {http.StatusNotFound, "WORKFLOW_NOT_FOUND"},
	workflowdomain.ErrDuplicateName:         {http.StatusConflict, "WORKFLOW_NAME_DUPLICATE"},
	workflowdomain.ErrVersionNotFound:       {http.StatusNotFound, "WORKFLOW_VERSION_NOT_FOUND"},
	workflowdomain.ErrPendingNotFound:       {http.StatusNotFound, "WORKFLOW_PENDING_NOT_FOUND"},
	workflowdomain.ErrNoActiveVersion:       {http.StatusUnprocessableEntity, "WORKFLOW_NO_ACTIVE_VERSION"},
	workflowdomain.ErrDAGCycle:              {http.StatusUnprocessableEntity, "WORKFLOW_DAG_CYCLE"},
	workflowdomain.ErrInvalidReference:      {http.StatusUnprocessableEntity, "WORKFLOW_INVALID_REFERENCE"},
	workflowdomain.ErrNoTrigger:             {http.StatusUnprocessableEntity, "WORKFLOW_NO_TRIGGER"},
	workflowdomain.ErrOpInvalid:             {http.StatusBadRequest, "WORKFLOW_OP_INVALID"},
	workflowdomain.ErrCapabilityNotFound:    {http.StatusUnprocessableEntity, "WORKFLOW_CAPABILITY_NOT_FOUND"},
	workflowdomain.ErrMCPServerNotInstalled: {http.StatusUnprocessableEntity, "WORKFLOW_MCP_SERVER_NOT_INSTALLED"},

	// todo
	tododomain.ErrNotFound:        {http.StatusNotFound, "TODO_NOT_FOUND"},
	tododomain.ErrSubjectRequired: {http.StatusBadRequest, "TODO_SUBJECT_REQUIRED"},
	tododomain.ErrInvalidStatus:   {http.StatusBadRequest, "TODO_INVALID_STATUS"},

	userdomain.ErrNotFound:          {http.StatusNotFound, "USER_NOT_FOUND"},
	userdomain.ErrUsernameRequired:  {http.StatusBadRequest, "USERNAME_REQUIRED"},
	userdomain.ErrUsernameConflict:  {http.StatusConflict, "USERNAME_CONFLICT"},
	userdomain.ErrUsernameInvalid:   {http.StatusBadRequest, "USERNAME_INVALID"},
	userdomain.ErrCannotDeleteLast:  {http.StatusUnprocessableEntity, "CANNOT_DELETE_LAST_USER"},
	userdomain.ErrLanguageInvalid:   {http.StatusBadRequest, "LANGUAGE_INVALID"},

	// sandbox
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
	sandboxdomain.ErrDockerNotInstalled: {http.StatusUnprocessableEntity, "SANDBOX_DOCKER_NOT_INSTALLED"},
	sandboxdomain.ErrDockerDaemonDown:   {http.StatusServiceUnavailable, "SANDBOX_DOCKER_DAEMON_DOWN"},

	// subagent
	subagentdomain.ErrTypeNotFound:     {http.StatusNotFound, "SUBAGENT_TYPE_NOT_FOUND"},
	subagentdomain.ErrRecursionAttempt: {http.StatusUnprocessableEntity, "SUBAGENT_RECURSION"},

	// catalog
	catalogdomain.ErrAllSourcesFailed: {http.StatusServiceUnavailable, "CATALOG_ALL_SOURCES_FAILED"},

	// mcp
	mcpdomain.ErrServerNotFound:        {http.StatusNotFound, "MCP_SERVER_NOT_FOUND"},
	mcpdomain.ErrServerNotConnected:    {http.StatusConflict, "MCP_SERVER_NOT_CONNECTED"},
	mcpdomain.ErrToolNotFound:          {http.StatusNotFound, "MCP_TOOL_NOT_FOUND"},
	mcpdomain.ErrToolCallFailed:        {http.StatusBadGateway, "MCP_TOOL_CALL_FAILED"},
	mcpdomain.ErrToolCallTimeout:       {http.StatusGatewayTimeout, "MCP_TOOL_CALL_TIMEOUT"},
	mcpdomain.ErrRegistryEntryNotFound: {http.StatusNotFound, "MCP_REGISTRY_ENTRY_NOT_FOUND"},
	mcpdomain.ErrRequiredEnvMissing:    {http.StatusUnprocessableEntity, "MCP_REQUIRED_ENV_MISSING"},
	mcpdomain.ErrRequiredArgsMissing:   {http.StatusUnprocessableEntity, "MCP_REQUIRED_ARGS_MISSING"},
	mcpdomain.ErrInstallFailed:         {http.StatusBadGateway, "MCP_INSTALL_FAILED"},
	mcpdomain.ErrAlreadyInstalled: {http.StatusConflict, "MCP_ALREADY_INSTALLED"},

	// skill
	skilldomain.ErrSkillNotFound:      {http.StatusNotFound, "SKILL_NOT_FOUND"},
	skilldomain.ErrInvalidFrontmatter: {http.StatusUnprocessableEntity, "SKILL_INVALID_FRONTMATTER"},
	skilldomain.ErrBodyTooLarge:       {http.StatusUnprocessableEntity, "SKILL_BODY_TOO_LARGE"},
	skilldomain.ErrNameConflict:       {http.StatusConflict, "SKILL_NAME_CONFLICT"},
	skilldomain.ErrInvalidName:        {http.StatusUnprocessableEntity, "SKILL_INVALID_NAME"},

	// memory
	memorydomain.ErrNotFound:     {http.StatusNotFound, "MEMORY_NOT_FOUND"},
	memorydomain.ErrNameConflict: {http.StatusConflict, "MEMORY_NAME_CONFLICT"},
	memorydomain.ErrInvalidName:  {http.StatusBadRequest, "MEMORY_INVALID_NAME"},

	// document (Phase 5 §14)
	documentdomain.ErrNotFound:        {http.StatusNotFound, "DOCUMENT_NOT_FOUND"},
	documentdomain.ErrInvalidParent:   {http.StatusUnprocessableEntity, "DOCUMENT_INVALID_PARENT"},
	documentdomain.ErrNameConflict:    {http.StatusConflict, "DOCUMENT_NAME_CONFLICT"},
	documentdomain.ErrContentTooLarge: {http.StatusRequestEntityTooLarge, "DOCUMENT_CONTENT_TOO_LARGE"},
	documentdomain.ErrInvalidName:     {http.StatusBadRequest, "DOCUMENT_INVALID_NAME"},
	documentdomain.ErrParentNotFound:  {http.StatusUnprocessableEntity, "DOCUMENT_PARENT_NOT_FOUND"},

	// permissions (V1.2 §3 final-sweep)
	permdomain.ErrInvalidSettings: {http.StatusBadRequest, "INVALID_SETTINGS"},
	permdomain.ErrBlockedByRule:   {http.StatusUnprocessableEntity, "BLOCKED_BY_RULE"},

	// ask
	askapp.ErrNoPendingQuestion: {http.StatusNotFound, "ASK_NO_PENDING_QUESTION"},
	askapp.ErrTimeout:           {http.StatusGatewayTimeout, "ASK_TIMEOUT"},

	// cross-cutting infra sentinels (always 500)
	reqctxpkg.ErrMissingUserID:         {http.StatusInternalServerError, "INTERNAL_ERROR"},
	reqctxpkg.ErrMissingConversationID: {http.StatusInternalServerError, "INTERNAL_ERROR"},
	cryptoinfra.ErrUnsupportedVersion:  {http.StatusInternalServerError, "INTERNAL_ERROR"},

	// LLM provider classification sentinels
	llminfra.ErrAuthFailed:    {http.StatusUnauthorized, "LLM_AUTH_FAILED"},
	llminfra.ErrRateLimited:   {http.StatusTooManyRequests, "LLM_RATE_LIMITED"},
	llminfra.ErrBadRequest:    {http.StatusBadRequest, "LLM_BAD_REQUEST"},
	llminfra.ErrModelNotFound: {http.StatusNotFound, "LLM_MODEL_NOT_FOUND"},
	llminfra.ErrProviderError: {http.StatusBadGateway, "LLM_PROVIDER_ERROR"},

	// stdlib context (client closed / upstream timeout)
	context.Canceled:         {499, "CLIENT_CLOSED"},
	context.DeadlineExceeded: {http.StatusGatewayTimeout, "REQUEST_TIMEOUT"},
}

// FromDomainError maps via errTable; unmapped → 500 with suppressed message.
//
// FromDomainError 通过 errTable 映射;未登记 → 500 并隐藏原文。
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

func lookup(err error) (errMapping, bool) {
	for sentinel, m := range errTable {
		if stderrors.Is(err, sentinel) {
			return m, true
		}
	}
	return errMapping{http.StatusInternalServerError, "INTERNAL_ERROR"}, false
}
