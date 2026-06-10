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

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
)

const stderrBufferMax = 256 * 1024

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
	sdkClient := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "forgify", Version: "1.2.0"},
		&mcpsdk.ClientOptions{ProgressNotificationHandler: c.onProgress})

	var transport mcpsdk.Transport
	if c.spec.isRemote() {
		t, err := c.remoteTransport()
		if err != nil {
			return err
		}
		transport = t
	} else {
		if c.spec.Stderr != nil {
			go c.drainStderr(c.spec.Stderr)
		}
		transport = &mcpsdk.IOTransport{Reader: c.spec.Stdout, Writer: c.spec.Stdin}
	}

	session, err := sdkClient.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("mcp.Client.Initialize: connect %s: %w: %v",
			c.spec.Name, mcpdomain.ErrServerNotConnected, err)
	}
	c.session = session
	return nil
}

// remoteTransport builds an SSE / streamable-HTTP transport whose HTTP client injects the
// configured headers (Authorization: Bearer ...) on every request.
//
// remoteTransport 构造 SSE / streamable-HTTP transport，其 HTTP client 在每个请求注入配置的
// header（Authorization: Bearer ...）。
func (c *client) remoteTransport() (mcpsdk.Transport, error) {
	httpClient := &http.Client{Transport: &headerRoundTripper{base: http.DefaultTransport, headers: c.spec.Headers}}
	switch c.spec.Transport {
	case mcpdomain.TransportSSE:
		return &mcpsdk.SSEClientTransport{Endpoint: c.spec.URL, HTTPClient: httpClient}, nil
	case mcpdomain.TransportStreamableHTTP, "":
		return &mcpsdk.StreamableClientTransport{Endpoint: c.spec.URL, HTTPClient: httpClient}, nil
	default:
		return nil, fmt.Errorf("mcp.Client.Initialize: %w: unknown transport %q",
			mcpdomain.ErrServerNotConnected, c.spec.Transport)
	}
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
	if sink := progressFrom(ctx); sink != nil {
		token := strconv.FormatInt(c.progSeq.Add(1), 10)
		c.progress.Store(token, sink)
		defer c.progress.Delete(token)
		params.SetProgressToken(token)
	}

	res, err := c.session.CallTool(ctx, params)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return "", fmt.Errorf("mcp.Client.CallTool %s/%s: %w: %v",
				c.spec.Name, name, mcpdomain.ErrToolCallTimeout, err)
		}
		return "", fmt.Errorf("mcp.Client.CallTool %s/%s: %w: %v",
			c.spec.Name, name, mcpdomain.ErrToolCallFailed, err)
	}
	if res.IsError {
		return "", fmt.Errorf("mcp.Client.CallTool %s/%s: %w: %s",
			c.spec.Name, name, mcpdomain.ErrToolCallFailed, joinContent(res.Content))
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
