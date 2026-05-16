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
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// CallInput is the request shape for Service.Call.
//
// CallInput 是 Service.Call 的请求形状。
type CallInput struct {
	HandlerName string
	HandlerID   string
	Method      string
	Args        map[string]any
	Owner       Owner
	OnProgress  func(any)
}

// Call dispatches a method on a handler instance honoring caller-owns lifetime; always writes one Call row.
//
// Call 按 caller-owns lifetime 派发 handler 方法调用，并写一行 Call。
func (s *Service) Call(ctx context.Context, in CallInput) (any, error) {
	uid, _ := reqctxpkg.RequireUserID(ctx)

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

// spawnInstance is the shared spawn flow: fresh subprocess, send Init, return ready Instance.
//
// spawnInstance 是共用 spawn 流程：起子进程、发 Init、返就绪 Instance。
func (s *Service) spawnInstance(ctx context.Context, h *handlerdomain.Handler, owner Owner) (*Instance, error) {
	active, err := s.repo.GetVersion(ctx, h.ActiveVersionID)
	if err != nil {
		return nil, fmt.Errorf("get active version: %w", err)
	}

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

	if active.EnvStatus != handlerdomain.EnvStatusReady {
		if err := s.syncEnv(ctx, active); err != nil {
			return nil, fmt.Errorf("sync env: %w", err)
		}
	}

	classCode := AssembleClass(activeToVersionDraft(active))
	if err := s.sandbox.WriteCodeFile(ctx, h.ID, active.ID, classCode); err != nil {
		return nil, fmt.Errorf("write code: %w", err)
	}

	// Lazy rebuild if env was destroyed externally; retry once.
	// 外部销毁导致 env 行丢失时按存档重建并重试一次。
	spawnReq := SpawnRequest{
		HandlerID: h.ID,
		VersionID: active.ID,
		EnvID:     active.EnvID,
	}
	handle, err := s.sandbox.SpawnLongLived(ctx, spawnReq)
	if err != nil && errors.Is(err, sandboxdomain.ErrEnvNotFound) {
		s.log.Info("handler env evicted externally; rebuilding then retrying call",
			zap.String("handlerId", h.ID),
			zap.String("versionId", active.ID),
			zap.String("envId", active.EnvID))
		if syncErr := s.syncEnv(ctx, active); syncErr != nil {
			return nil, fmt.Errorf("rebuild after evict: %w", syncErr)
		}
		spawnReq.EnvID = active.EnvID
		handle, err = s.sandbox.SpawnLongLived(ctx, spawnReq)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", handlerdomain.ErrInstanceSpawnFailed, err)
	}

	stderrRing := newStderrRing(256 * 1024)
	go captureStderr(handle.Stderr(), stderrRing, s.log.With(zap.String("handlerId", h.ID), zap.Int("pid", handle.PID())))

	client := s.clientFact(handle.Stdin(), handle.Stdout(), s.log)
	if err := client.Init(ctx, config); err != nil {
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

func (s *Service) invokeMethod(ctx context.Context, inst *Instance, in CallInput) (any, error) {
	if in.OnProgress != nil {
		return inst.Client.StreamCall(ctx, in.Method, in.Args, in.OnProgress)
	}
	return inst.Client.Call(ctx, in.Method, in.Args)
}

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

// syncEnv materializes the venv synchronously, writes terminal EnvStatus to DB + v in place.
//
// syncEnv 同步物化 venv，终态写 DB 并镜像到 v。
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

// stderrRing is a small ring buffer capturing subprocess stderr; the tail is logged on crash.
//
// stderrRing 小型环形缓冲，捕获子进程 stderr，crash 时尾部入 log。
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
		p = p[len(p)-r.cap:]
		r.buf = append(r.buf[:0], p...)
		return len(p), nil
	}
	if len(r.buf)+len(p) <= r.cap {
		r.buf = append(r.buf, p...)
		return len(p), nil
	}
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

// captureStderr scans stderr line-by-line into the ring and emits at zap Info.
//
// captureStderr 行扫 stderr，写入 ring 并以 zap Info 输出。
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

var _ = (&stderrRing{}).String
