// call.go — Service.Call dispatches to a HandlerInstance per the user-clarified
// caller-owns lifetime model (forge_redesign 2026-05-12):
//
//   - chat scope (TriggeredByChat / Owner.Kind="chat"):
//         per-call lifetime — spawn → call → destroy in one Service.Call.
//         No registry interaction. Useful for one-off LLM tool invocations
//         where the cost of spawning a fresh subprocess (~100ms typical) is
//         acceptable for the simplicity gain.
//
//   - workflow / test / session scope:
//         persistent — registry.Acquire spawns the first time and reuses on
//         subsequent Calls within the same owner. Owner-end hooks
//         (workflow.run.End / test.End / session.Release) call DestroyOwner
//         to tear down everything for that scope.
//
// spawn flow (shared by both paths):
//   1. Resolve active version + decrypt config
//   2. Validate config matches InitArgsSchema (required keys present)
//   3. AssembleClass → WriteCodeFile (user_handler.py + driver.py)
//   4. Sync env if not ready
//   5. SpawnLongLived → wrap pipes in handlerinfra.Client → Init
//   6. Capture stderr to a 256KB ring (logged at crash time)
//
// call.go —— Service.Call 按 caller-owns lifetime 派发(2026-05-12 用户细化):
// chat 单调用,workflow/test/session 经 registry 持久。spawn 流程见上。

package handler

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"go.uber.org/zap"

	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// CallInput is the request shape for Service.Call.
//
// CallInput Service.Call 的请求形状。
type CallInput struct {
	HandlerName string         // by name (preferred; LLM uses name)
	HandlerID   string         // alternative — direct id lookup
	Method      string
	Args        map[string]any
	Owner       Owner          // caller-context scope (chat=per-call; others=persistent)
	OnProgress  func(any)      // optional — invoked on each progress yield from streaming methods
}

// Call dispatches a method call on a handler instance, honoring caller-owns
// lifetime per the user's clarified model. Always writes one terminal Call
// row (D22) to handler_calls.
//
// Call 派发 handler instance 的 method 调用,按 caller-owns lifetime 处理;
// 终态写 1 行 Call 到 handler_calls(D22)。
func (s *Service) Call(ctx context.Context, in CallInput) (any, error) {
	uid, _ := reqctxpkg.RequireUserID(ctx) // ok if empty — recordCall handles

	// 1. Resolve handler by ID or name.
	var h *handlerdomain.Handler
	if in.HandlerID != "" {
		got, err := s.repo.GetHandler(ctx, in.HandlerID)
		if err != nil {
			return nil, fmt.Errorf("handlerapp.Call: lookup by id: %w", err)
		}
		h = got
	} else if in.HandlerName != "" {
		got, err := s.repo.GetHandlerByName(ctx, in.HandlerName)
		if err != nil {
			return nil, fmt.Errorf("handlerapp.Call: lookup by name: %w", err)
		}
		h = got
	} else {
		return nil, fmt.Errorf("handlerapp.Call: handlerName or handlerID required")
	}

	if h.ActiveVersionID == "" {
		return nil, fmt.Errorf("handlerapp.Call: %w", handlerdomain.ErrNoActiveVersion)
	}

	startedAt := time.Now().UTC()
	var (
		result     any
		instanceID string
		callErr    error
	)
	if in.Owner.Kind == "chat" || in.Owner.Kind == "" {
		result, instanceID, callErr = s.callPerCallTracked(ctx, h, in)
	} else {
		result, instanceID, callErr = s.callViaRegistryTracked(ctx, h, in)
	}
	endedAt := time.Now().UTC()

	s.recordCall(ctx, uid, h, in, instanceID, startedAt, endedAt, result, callErr, ctx.Err())

	return result, callErr
}

// callPerCallTracked spawns + calls + destroys + returns the instanceID used
// (for call_log row).
//
// callPerCallTracked spawn+call+destroy 返 instanceID(供 call_log 行)。
func (s *Service) callPerCallTracked(ctx context.Context, h *handlerdomain.Handler, in CallInput) (any, string, error) {
	inst, err := s.spawnInstance(ctx, h, Owner{Kind: "chat", ID: "ephemeral"})
	if err != nil {
		return nil, "", fmt.Errorf("handlerapp.Call: spawn: %w", err)
	}
	defer func() {
		_ = inst.Client.Shutdown(ctx)
		_ = inst.Kill()
	}()
	res, err := s.invokeMethod(ctx, inst, in)
	return res, inst.ID, err
}

// callViaRegistryTracked uses registry.Acquire and returns the instanceID used.
//
// callViaRegistryTracked 走 registry.Acquire 返 instanceID。
func (s *Service) callViaRegistryTracked(ctx context.Context, h *handlerdomain.Handler, in CallInput) (any, string, error) {
	inst, err := s.registry.Acquire(ctx, in.Owner, h.Name, func(ctx context.Context) (*Instance, error) {
		return s.spawnInstance(ctx, h, in.Owner)
	})
	if err != nil {
		return nil, "", fmt.Errorf("handlerapp.Call: acquire: %w", err)
	}
	res, err := s.invokeMethod(ctx, inst, in)
	return res, inst.ID, err
}

// recordCall writes a terminal Call row capturing the outcome. Best-effort
// (a failed log write doesn't surface as a call failure). Uses detached ctx
// (§S9) so caller cancel doesn't lose the log.
//
// recordCall 写一行 Call(详 D22)。best-effort + detached ctx。
func (s *Service) recordCall(
	ctx context.Context,
	uid string,
	h *handlerdomain.Handler,
	in CallInput,
	instanceID string,
	startedAt, endedAt time.Time,
	result any,
	callErr error,
	runCtxErr error,
) {
	status := handlerdomain.CallStatusOK
	errorMessage := ""
	if callErr != nil {
		status = handlerdomain.CallStatusFailed
		errorMessage = callErr.Error()
		if errors.Is(runCtxErr, context.DeadlineExceeded) {
			status = handlerdomain.CallStatusTimeout
		} else if errors.Is(runCtxErr, context.Canceled) {
			status = handlerdomain.CallStatusCancelled
		}
	}

	triggeredBy := "http"
	if in.Owner.Kind == "chat" {
		triggeredBy = "chat"
	} else if in.Owner.Kind == "workflow" || in.Owner.Kind == "flowrun" {
		triggeredBy = "workflow"
	} else if in.Owner.Kind == "test" {
		triggeredBy = "test"
	}

	convID, _ := reqctxpkg.GetConversationID(ctx)
	msgID, _ := reqctxpkg.GetMessageID(ctx)
	toolCallID, _ := reqctxpkg.GetToolCallID(ctx)

	call := &handlerdomain.Call{
		ID:             idgenpkg.New("hcl"),
		UserID:         uid,
		Status:         status,
		TriggeredBy:    triggeredBy,
		Input:          in.Args,
		Output:         result,
		ErrorMessage:   errorMessage,
		ElapsedMs:      endedAt.Sub(startedAt).Milliseconds(),
		StartedAt:      startedAt,
		EndedAt:        endedAt,
		ConversationID: convID,
		MessageID:      msgID,
		ToolCallID:     toolCallID,
		HandlerID:      h.ID,
		VersionID:      h.ActiveVersionID,
		Method:         in.Method,
		InstanceID:     instanceID,
		OwnerKind:      in.Owner.Kind,
		OwnerID:        in.Owner.ID,
	}

	detached := reqctxpkg.SetUserID(context.Background(), uid)
	if err := s.repo.SaveCall(detached, call); err != nil {
		s.log.Warn("handlerapp.recordCall: SaveCall failed (best-effort)",
			zap.String("handlerId", h.ID),
			zap.String("method", in.Method),
			zap.Error(err))
	}
}

// spawnInstance is the shared spawn flow used by both paths. Spawns a fresh
// subprocess, sends Init, returns ready-to-use Instance.
//
// spawnInstance 是双路共用的 spawn 流程;起子进程 + 发 Init,返就绪 Instance。
func (s *Service) spawnInstance(ctx context.Context, h *handlerdomain.Handler, owner Owner) (*Instance, error) {
	active, err := s.repo.GetVersion(ctx, h.ActiveVersionID)
	if err != nil {
		return nil, fmt.Errorf("get active version: %w", err)
	}

	// Validate config has all required init_args.
	config, err := s.LoadConfig(ctx, h.ID)
	if err != nil && !errors.Is(err, handlerdomain.ErrConfigDecryptFailed) {
		return nil, fmt.Errorf("load config: %w", err)
	}
	for _, arg := range active.InitArgsSchema {
		if !arg.Required {
			continue
		}
		if config == nil || config[arg.Name] == nil {
			return nil, fmt.Errorf("%w: missing required init_args[%s]",
				handlerdomain.ErrConfigIncomplete, arg.Name)
		}
	}

	// Ensure env is ready. We sync here synchronously (call-path is interactive
	// and the LLM/user is waiting). For first-call latency optimization, future
	// versions could pre-warm on accept.
	//
	// 同步 sync env(call-path 是交互式,LLM/用户在等)。未来可在 accept 时预热。
	if active.EnvStatus != handlerdomain.EnvStatusReady {
		if err := s.syncEnv(ctx, active); err != nil {
			return nil, fmt.Errorf("sync env: %w", err)
		}
	}

	// Compose class + write files.
	classCode := AssembleClass(activeToVersionDraft(active))
	if err := s.sandbox.WriteCodeFile(ctx, h.ID, active.ID, classCode); err != nil {
		return nil, fmt.Errorf("write code: %w", err)
	}

	// Spawn subprocess.
	handle, err := s.sandbox.SpawnLongLived(ctx, SpawnRequest{
		HandlerID: h.ID,
		VersionID: active.ID,
		EnvID:     active.EnvID,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", handlerdomain.ErrInstanceSpawnFailed, err)
	}

	// Stderr → 256KB ring + zap log for crash diagnostics.
	stderrRing := newStderrRing(256 * 1024)
	go captureStderr(handle.Stderr(), stderrRing, s.log.With(zap.String("handlerId", h.ID), zap.Int("pid", handle.PID())))

	// Wrap pipes in client and Init.
	client := s.clientFact(handle.Stdin(), handle.Stdout(), s.log)
	if err := client.Init(ctx, config); err != nil {
		// Init failed; kill the subprocess.
		_ = handle.Kill()
		return nil, fmt.Errorf("init: %w", err)
	}

	inst := &Instance{
		ID:        NewInstanceID(),
		HandlerID: h.ID,
		Owner:     owner,
		Client:    client,
		Kill:      handle.Kill,
	}
	return inst, nil
}

// invokeMethod dispatches the method call (StreamCall if onProgress, Call otherwise).
//
// invokeMethod 派发 method;有 onProgress 走 StreamCall,否则 Call。
func (s *Service) invokeMethod(ctx context.Context, inst *Instance, in CallInput) (any, error) {
	if in.OnProgress != nil {
		return inst.Client.StreamCall(ctx, in.Method, in.Args, in.OnProgress)
	}
	return inst.Client.Call(ctx, in.Method, in.Args)
}

// activeToVersionDraft adapts a persisted Version row to a VersionDraft for
// AssembleClass.
//
// activeToVersionDraft 把 Version 行转 VersionDraft 给 AssembleClass。
func activeToVersionDraft(v *handlerdomain.Version) *VersionDraft {
	return &VersionDraft{
		Imports:        v.Imports,
		InitBody:       v.InitBody,
		ShutdownBody:   v.ShutdownBody,
		Methods:        v.Methods,
		InitArgsSchema: v.InitArgsSchema,
		Dependencies:   v.Dependencies,
		PythonVersion:  v.PythonVersion,
	}
}

// syncEnv runs Sync synchronously, writes terminal env_status to DB, mutates
// v in place so the caller sees the terminal state without re-reading.
// Publishes env_synced / env_failed notifications (slim-payload migration is
// C3; for now the wire actions stay to keep consumers green).
//
// syncEnv 同步跑 Sync + 写 DB 终态 + 镜像到 v(调用方不必 re-read);
// 完成推 env_synced / env_failed 通知(瘦身留 C3 一次性改)。
func (s *Service) syncEnv(ctx context.Context, v *handlerdomain.Version) error {
	now := time.Now().UTC()
	_ = s.repo.UpdateVersionEnv(ctx, v.ID,
		handlerdomain.EnvStatusSyncing, "", "starting", "", nil)
	v.EnvStatus = handlerdomain.EnvStatusSyncing

	onProgress := func(stage, detail string) {
		_ = s.repo.UpdateVersionEnv(ctx, v.ID,
			handlerdomain.EnvStatusSyncing, "", stage, detail, nil)
	}
	req := SyncRequest{
		HandlerID:     v.HandlerID,
		VersionID:     v.ID,
		EnvID:         v.EnvID,
		Dependencies:  v.Dependencies,
		PythonVersion: v.PythonVersion,
		OnProgress:    onProgress,
	}
	if err := s.sandbox.Sync(ctx, req); err != nil {
		stderr := err.Error()
		var syncErr *SyncError
		if errors.As(err, &syncErr) {
			stderr = syncErr.Stderr
		}
		_ = s.repo.UpdateVersionEnv(ctx, v.ID,
			handlerdomain.EnvStatusFailed, stderr, "failed", "", &now)
		v.EnvStatus = handlerdomain.EnvStatusFailed
		v.EnvError = stderr
		v.EnvSyncStage = "failed"
		v.EnvSyncDetail = ""
		v.EnvSyncedAt = &now
		// env_failed / env_synced notifications removed in C3 (D-redo-7);
		// env terminal state carried by tool_result + UI GET (D-redo-6).
		return fmt.Errorf("%w: %v", handlerdomain.ErrEnvFailed, err)
	}
	syncedAt := time.Now().UTC()
	_ = s.repo.UpdateVersionEnv(ctx, v.ID,
		handlerdomain.EnvStatusReady, "", "ready", "", &syncedAt)
	v.EnvStatus = handlerdomain.EnvStatusReady
	v.EnvError = ""
	v.EnvSyncStage = "ready"
	v.EnvSyncDetail = ""
	v.EnvSyncedAt = &syncedAt
	return nil
}

// ── stderr capture ───────────────────────────────────────────────────────────

// stderrRing is a tiny 256KB ring buffer used to capture subprocess stderr.
// On crash the ring's tail is what gets logged.
//
// stderrRing 一个简易环形缓冲区抓子进程 stderr;crash 时尾部进 log。
type stderrRing struct {
	mu  sync.Mutex
	buf []byte
	w   int
	cap int
}

func newStderrRing(cap int) *stderrRing {
	return &stderrRing{buf: make([]byte, 0, cap), cap: cap}
}

func (r *stderrRing) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(p) >= r.cap {
		// truncate to last cap bytes
		p = p[len(p)-r.cap:]
		r.buf = append(r.buf[:0], p...)
		return len(p), nil
	}
	if len(r.buf)+len(p) <= r.cap {
		r.buf = append(r.buf, p...)
		return len(p), nil
	}
	// shift left to make room
	overflow := len(r.buf) + len(p) - r.cap
	copy(r.buf, r.buf[overflow:])
	r.buf = r.buf[:len(r.buf)-overflow]
	r.buf = append(r.buf, p...)
	return len(p), nil
}

func (r *stderrRing) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.buf)
}

// captureStderr scans stderr line-by-line, writing each line to the ring
// AND emitting at zap Info level. Runs in a goroutine for the subprocess
// lifetime — exits on EOF (handle.Kill or natural process exit).
//
// captureStderr 行扫 stderr,每行写 ring + zap Info log。子进程结束(EOF)时
// goroutine 退。
func captureStderr(r io.Reader, ring *stderrRing, log *zap.Logger) {
	if r == nil {
		return
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 64*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		_, _ = ring.Write(append([]byte{}, line...))
		_, _ = ring.Write([]byte{'\n'})
		log.Info("handler.stderr", zap.ByteString("line", line))
	}
}

// _ guard against unused field warnings; stderr ring used only on crash
// reporting which is wired in Plan 02 Phase 5 (LLM tool friendly error msg).
//
// _ 占位防 ring 暂时未读警告(crash 路径在 Phase 5 LLM 工具接驳)。
var _ = (&stderrRing{}).String
