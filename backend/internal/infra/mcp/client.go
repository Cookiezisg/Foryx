// Package mcp wraps modelcontextprotocol/go-sdk's client with project concerns: stdio
// (the subprocess is spawned & OWNED by sandbox via SpawnLongLived — we only wire its
// pipes through go-sdk's IOTransport) AND remote (SSE / streamable-HTTP with header auth).
// This package never touches sandbox, the host PATH, or process lifecycle directly.
//
// Package mcp 在 go-sdk client 之上加项目层关切：stdio（子进程由 sandbox 经 SpawnLongLived 起且
// 归其管——我们只把它的管道经 go-sdk IOTransport 接上）与 remote（SSE / streamable-HTTP，header
// 鉴权）。本包不碰 sandbox、宿主 PATH 或进程生命周期。
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	mcpdomain "github.com/sunweilin/anselm/backend/internal/domain/mcp"
)

const stderrBufferMax = 256 * 1024

// remoteConnectAttempt bounds ONE transport attempt so a hanging wrong-transport try (SSE GET to a
// streamable server) fails fast and the fallback gets its own budget instead of eating the whole
// install timeout.
//
// remoteConnectAttempt 限单次 transport 尝试，使挂住的错 transport（对 streamable server 发 SSE GET）快速失败、
// fallback 有自己的预算，而非吃掉整个安装超时。
const remoteConnectAttempt = 25 * time.Second

// ClientSpec is the resolved connection recipe. URL != "" → remote (Transport + Headers);
// else stdio — Stdin/Stdout/Stderr are the pipes of a sandbox-owned subprocess.
//
// ClientSpec 是解析后的连接配方。URL != "" → remote（Transport + Headers）；否则 stdio——
// Stdin/Stdout/Stderr 是 sandbox 所属子进程的管道。
type ClientSpec struct {
	Name string

	// stdio: pipes of a sandbox-owned subprocess (SpawnLongLived handle)
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Stderr io.ReadCloser

	// remote
	URL       string
	Transport string // sse | streamable-http
	Headers   map[string]string

	// remote OAuth: when set, every request gets a freshly-valid Bearer from the source (refreshed
	// + re-persisted by app/mcp) instead of a static Authorization header.
	// remote OAuth：设了则每个请求从 source 取实时有效 Bearer（由 app/mcp 刷新+重存）而非静态 Authorization。
	TokenSource TokenSource
}

func (s ClientSpec) isRemote() bool { return s.URL != "" }

// Client is the per-server interface used by app/mcp Service.
//
// Client 是 app/mcp Service 用的 per-server 接口。
type Client interface {
	Initialize(ctx context.Context) error
	ListTools(ctx context.Context) ([]mcpdomain.ToolDef, error)
	CallTool(ctx context.Context, name string, args json.RawMessage) (string, error)
	Close() error
	StderrTail() string
}

type client struct {
	spec    ClientSpec
	log     *zap.Logger
	session *mcpsdk.ClientSession
	stderr  *ringBuffer

	// progress correlates an MCP server's progress notifications back to the in-flight CallTool
	// that opted in (via a per-call token) so they can be forwarded to that call's sink. The go-sdk
	// progress handler is session-global, hence this token→sink map. progSeq mints the tokens.
	//
	// progress 把 MCP server 的进度通知按 per-call token 关联回那次 opt-in 的 CallTool，转发到该调用的
	// sink。go-sdk 的进度 handler 是 session 级全局，故用此 token→sink 表。progSeq 发 token。
	progress sync.Map // token string → func(string)
	progSeq  atomic.Int64
}

// NewClient constructs an unstarted Client; Initialize runs the MCP handshake over the spec's
// transport. For stdio the caller must have already spawned the subprocess (sandbox).
//
// NewClient 构造未启动的 Client；Initialize 在 spec 的 transport 上走 MCP 握手。stdio 情形调用方
// 须已起好子进程（sandbox）。
func NewClient(spec ClientSpec, log *zap.Logger) Client {
	if log == nil {
		log = zap.NewNop()
	}
	return &client{spec: spec, log: log.Named("mcp." + spec.Name), stderr: newRingBuffer(stderrBufferMax)}
}

// Initialize builds the right transport (stdio IOTransport vs remote) and runs the handshake.
//
// Initialize 构造对应 transport（stdio IOTransport vs remote）并走握手。
func (c *client) Initialize(ctx context.Context) error {
	sdkClient := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "anselm", Version: "1.2.0"},
		&mcpsdk.ClientOptions{ProgressNotificationHandler: c.onProgress})

	if c.spec.isRemote() {
		return c.connectRemote(ctx, sdkClient)
	}
	if c.spec.Stderr != nil {
		go c.drainStderr(c.spec.Stderr)
	}
	session, err := sdkClient.Connect(ctx, &mcpsdk.IOTransport{Reader: c.spec.Stdout, Writer: c.spec.Stdin}, nil)
	if err != nil {
		return fmt.Errorf("mcp.Client.Initialize: connect %s: %w: %v",
			c.spec.Name, mcpdomain.ErrServerNotConnected, err)
	}
	c.session = session
	return nil
}

// connectRemote dials a remote MCP server, trying the declared transport first then falling back to
// the other. Registry transport_type labels are widely stale — after the MCP SSE deprecation many
// servers labeled "sse" actually serve streamable-http (and vice versa), so a single fixed transport
// silently fails to connect (F85). The HTTP client (static header or refreshing OAuth bearer) is
// shared across attempts.
//
// connectRemote 连 remote MCP server，先试声明的 transport、失败再退另一个。registry 的 transport_type
// 标签普遍陈旧——MCP SSE 弃用后大量标「sse」的 server 实际服 streamable-http（反之亦然），单一固定 transport
// 会静默连不上（F85）。HTTP client（静态 header 或会刷新的 OAuth bearer）跨尝试共享。
func (c *client) connectRemote(ctx context.Context, sdkClient *mcpsdk.Client) error {
	var rt http.RoundTripper
	if c.spec.TokenSource != nil {
		rt = &oauthRoundTripper{base: http.DefaultTransport, src: c.spec.TokenSource, extra: c.spec.Headers}
	} else {
		rt = &headerRoundTripper{base: http.DefaultTransport, headers: c.spec.Headers}
	}
	httpClient := &http.Client{Transport: rt}
	var lastErr error
	for _, tt := range remoteTransportOrder(c.spec.Transport) {
		var transport mcpsdk.Transport
		if tt == mcpdomain.TransportSSE {
			transport = &mcpsdk.SSEClientTransport{Endpoint: c.spec.URL, HTTPClient: httpClient}
		} else {
			transport = &mcpsdk.StreamableClientTransport{Endpoint: c.spec.URL, HTTPClient: httpClient}
		}
		// Bound each attempt: a wrong transport can HANG (an SSE GET to a streamable server holds the
		// connection open) instead of erroring, so without this the fallback waits out the whole install
		// timeout. The session outlives this ctx (Connect's ctx is handshake-only — the caller already
		// cancels its connect ctx after Initialize and sessions persist), so cancelling here is safe.
		// 给每次尝试设界：错的 transport 会卡住（对 streamable server 发 SSE GET 会挂住连接）而非报错，否则 fallback
		// 会耗尽整个安装超时。session 比此 ctx 活得久（Connect 的 ctx 只管握手——调用方 Initialize 后即取消其连接 ctx、
		// session 仍存），故在此取消安全。
		attemptCtx, cancel := context.WithTimeout(ctx, remoteConnectAttempt)
		session, err := sdkClient.Connect(attemptCtx, transport, nil)
		cancel()
		if err == nil {
			c.session = session
			return nil
		}
		lastErr = err
	}
	return fmt.Errorf("mcp.Client.Initialize: connect %s: %w: %v",
		c.spec.Name, mcpdomain.ErrServerNotConnected, lastErr)
}

// remoteTransportOrder lists the transports to try, declared first then the other. streamable-http
// is the modern default, so a blank/unknown label tries it before SSE.
//
// remoteTransportOrder 返回要试的 transport，先声明的再另一个。streamable-http 是现代默认，故空/未知标签先试它再 SSE。
func remoteTransportOrder(declared string) []string {
	if declared == mcpdomain.TransportSSE {
		return []string{mcpdomain.TransportSSE, mcpdomain.TransportStreamableHTTP}
	}
	return []string{mcpdomain.TransportStreamableHTTP, mcpdomain.TransportSSE}
}

// ListTools fetches tools/list and converts SDK Tools to domain ToolDefs (stamps ServerName).
//
// ListTools 调 tools/list 并把 SDK Tool 转 domain ToolDef（带 ServerName）。
func (c *client) ListTools(ctx context.Context) ([]mcpdomain.ToolDef, error) {
	if c.session == nil {
		return nil, fmt.Errorf("mcp.Client.ListTools: %w", mcpdomain.ErrServerNotConnected)
	}
	res, err := c.session.ListTools(ctx, &mcpsdk.ListToolsParams{})
	if err != nil {
		return nil, fmt.Errorf("mcp.Client.ListTools %s: %w: %v",
			c.spec.Name, mcpdomain.ErrToolCallFailed, err)
	}
	out := make([]mcpdomain.ToolDef, 0, len(res.Tools))
	for _, t := range res.Tools {
		schemaJSON, _ := json.Marshal(t.InputSchema)
		out = append(out, mcpdomain.ToolDef{
			ServerName:  c.spec.Name,
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schemaJSON,
		})
	}
	return out, nil
}

// CallTool invokes one tool; ctx carries the per-call timeout; ctx.Done → ErrToolCallTimeout.
//
// CallTool 调一个 tool；ctx 携超时；ctx.Done 时返 ErrToolCallTimeout。
func (c *client) CallTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	if c.session == nil {
		return "", fmt.Errorf("mcp.Client.CallTool %s/%s: %w",
			c.spec.Name, name, mcpdomain.ErrServerNotConnected)
	}

	var argsMap any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &argsMap); err != nil {
			return "", fmt.Errorf("mcp.Client.CallTool %s/%s: parse args: %w", c.spec.Name, name, err)
		}
	}

	params := &mcpsdk.CallToolParams{Name: name, Arguments: argsMap}
	// If the caller opted into progress (the chat tool layer put a sink in ctx), register a per-call
	// token so this server's progress notifications stream to that sink for the call's duration.
	//
	// 若调用方 opt-in 进度（chat 工具层在 ctx 放了 sink），登记 per-call token，使本 server 的进度通知在
	// 调用期间流到该 sink。
	var progressSeen chan struct{}
	if sink := ProgressFrom(ctx); sink != nil {
		token := strconv.FormatInt(c.progSeq.Add(1), 10)
		// Wrap the sink to also signal delivery, so we can drain in-flight progress before the
		// token is unregistered (below). Buffered+non-blocking: never stalls the handler.
		// 包裹 sink 使其同时发投递信号，以便在 token 注销前排空在途进度。缓冲+非阻塞：绝不卡处理器。
		progressSeen = make(chan struct{}, 1)
		c.progress.Store(token, func(line string) {
			sink(line)
			select {
			case progressSeen <- struct{}{}:
			default:
			}
		})
		defer c.progress.Delete(token)
		params.SetProgressToken(token)
	}

	res, err := c.session.CallTool(ctx, params)
	// The server emits progress notifications before the result on the ordered stream, but the SDK
	// dispatches them on a separate handler goroutine — so when CallTool returns they may be enqueued
	// yet unrun, and the deferred Delete above would then drop them (the sink never fires, the durable
	// call log races empty). Yield so the handler flushes them to the sink while the token is still
	// registered. Repro of the loss: GOMAXPROCS=1.
	//
	// server 在有序流上把进度通知发在 result 之前，但 SDK 在独立 handler goroutine 上分发——故 CallTool
	// 返回时它们可能已入队却未跑，上面 deferred Delete 随即丢弃它们（sink 不触发、durable 调用日志竞争为空）。
	// 让出调度使 handler 在 token 仍登记时把它们刷进 sink。丢失复现：GOMAXPROCS=1。
	if progressSeen != nil {
		drainProgress(progressSeen)
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return "", fmt.Errorf("mcp.Client.CallTool %s/%s: %w: %v",
				c.spec.Name, name, mcpdomain.ErrToolCallTimeout, err)
		}
		return "", fmt.Errorf("mcp.Client.CallTool %s/%s: %w: %v",
			c.spec.Name, name, mcpdomain.ErrToolCallFailed.WithDetails(map[string]any{"reason": err.Error()}), err)
	}
	if res.IsError {
		// Carry the tool's error content (which names the bad field) into Details so the HTTP :invoke
		// envelope surfaces it instead of a bare MCP_RPC_ERROR — the LLM/workflow path already reads it
		// from the message, this gives the direct-invoke face parity (cf F8/F69).
		// 把工具错误内容（点名坏字段）带进 Details，使 HTTP :invoke envelope 透出而非裸 MCP_RPC_ERROR。
		detail := joinContent(res.Content)
		return "", fmt.Errorf("mcp.Client.CallTool %s/%s: %w: %s",
			c.spec.Name, name, mcpdomain.ErrToolCallFailed.WithDetails(map[string]any{"reason": detail}), detail)
	}
	return joinContent(res.Content), nil
}

// Close shuts down the session (closes the writer → the sandbox subprocess sees EOF). The
// subprocess itself is killed by the Service via its sandbox handle.
//
// Close 关停 session（关写端 → sandbox 子进程收到 EOF）。子进程本身由 Service 经其 sandbox handle 杀。
func (c *client) Close() error {
	if c.session == nil {
		return nil
	}
	err := c.session.Close()
	c.session = nil
	return err
}

// StderrTail returns the stdio subprocess's stderr ring-buffer tail (empty for remote).
//
// StderrTail 返 stdio 子进程的 stderr 环形缓冲尾部（remote 为空）。
func (c *client) StderrTail() string { return c.stderr.String() }

// drainStderr reads stderr lines, levels them by content, and keeps the tail in the ring.
//
// drainStderr 按行读 stderr，按内容选 log 级别，把尾部留在环形缓冲。
func (c *client) drainStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if stderrIndicatesWarnOrError(line) {
			c.log.Warn("stderr", zap.String("line", line))
		} else {
			c.log.Info("stderr", zap.String("line", line))
		}
		c.stderr.WriteLine(line)
	}
}

func stderrIndicatesWarnOrError(line string) bool {
	upper := strings.ToUpper(line)
	return strings.Contains(upper, "WARNING") || strings.Contains(upper, "ERROR") ||
		strings.Contains(upper, "FATAL") || strings.Contains(upper, "EXCEPTION") ||
		strings.Contains(upper, "TRACEBACK")
}

// headerRoundTripper injects fixed headers (auth tokens) on every remote request.
//
// headerRoundTripper 在每个 remote 请求注入固定 header（鉴权 token）。
type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range h.headers {
		req.Header.Set(k, v)
	}
	return h.base.RoundTrip(req)
}

// TokenSource yields a currently-valid OAuth access token for a remote server, refreshing +
// re-persisting it as needed. Implemented in app/mcp (which owns the store + the oauth flow); infra
// only consumes it, so the HTTP layer stays oblivious to storage/encryption (DIP).
//
// TokenSource 为 remote server 产出当前有效的 OAuth access token，按需刷新+重存。由 app/mcp 实现（它持
// store + oauth 流程）；infra 只消费，故 HTTP 层对存储/加密无感（DIP）。
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// oauthRoundTripper injects a freshly-valid OAuth bearer on every remote request (plus any non-auth
// static headers). The token comes from the source per-request so a refresh between calls is picked
// up transparently.
//
// oauthRoundTripper 在每个 remote 请求注入实时有效的 OAuth bearer（外加任何非认证静态 header）。token 每次
// 从 source 取，故两次调用间的刷新被透明拾取。
type oauthRoundTripper struct {
	base  http.RoundTripper
	src   TokenSource
	extra map[string]string
}

func (o *oauthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	tok, err := o.src.Token(req.Context())
	if err != nil {
		return nil, err
	}
	for k, v := range o.extra {
		req.Header.Set(k, v)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return o.base.RoundTrip(req)
}

// joinContent flattens an MCP content array; text verbatim, non-text → placeholder.
//
// joinContent 把 MCP content 数组拍平；text 原样，非 text 渲染为占位符。
func joinContent(content []mcpsdk.Content) string {
	var b strings.Builder
	for _, c := range content {
		switch v := c.(type) {
		case *mcpsdk.TextContent:
			b.WriteString(v.Text)
		case *mcpsdk.ImageContent:
			fmt.Fprintf(&b, "[image: %s]", v.MIMEType)
		case *mcpsdk.AudioContent:
			fmt.Fprintf(&b, "[audio: %s]", v.MIMEType)
		case *mcpsdk.ResourceLink:
			fmt.Fprintf(&b, "[resource: %s]", v.URI)
		case *mcpsdk.EmbeddedResource:
			if v.Resource != nil {
				fmt.Fprintf(&b, "[resource: %s]", v.Resource.URI)
			} else {
				b.WriteString("[resource]")
			}
		default:
			fmt.Fprintf(&b, "[%T]", c)
		}
	}
	return b.String()
}

// ringBuffer is a concurrency-safe fixed-capacity byte buffer (drops oldest when full).
//
// ringBuffer 是并发安全的固定容量字节缓冲；满时丢最早数据。
type ringBuffer struct {
	mu  sync.Mutex
	buf []byte
	cap int
}

func newRingBuffer(capacity int) *ringBuffer { return &ringBuffer{cap: capacity} }

func (r *ringBuffer) WriteLine(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, line...)
	r.buf = append(r.buf, '\n')
	if len(r.buf) > r.cap {
		r.buf = r.buf[len(r.buf)-r.cap:]
	}
}

func (r *ringBuffer) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.buf)
}
