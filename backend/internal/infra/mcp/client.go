// Package mcp wraps modelcontextprotocol/go-sdk stdio Client with project concerns.
//
// Package mcp 在 modelcontextprotocol/go-sdk stdio Client 之上添加项目层关切。
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
)

const stderrBufferMax = 256 * 1024

// Client is the per-server stdio MCP client interface used by app/mcp Service.
//
// Client 是 app/mcp Service 用的 per-server stdio MCP client 接口。
type Client interface {
	Initialize(ctx context.Context) error
	ListTools(ctx context.Context) ([]mcpdomain.ToolDef, error)
	CallTool(ctx context.Context, name string, args json.RawMessage) (string, error)
	Close() error
	StderrTail() string
}

type stdioClient struct {
	cfg     mcpdomain.ServerConfig
	log     *zap.Logger
	cmd     *exec.Cmd
	session *mcpsdk.ClientSession
	stderr  *ringBuffer
}

// NewStdioClient constructs an unstarted Client; call Initialize to spawn.
//
// NewStdioClient 构造未启动的 Client；调 Initialize 才真起子进程。
func NewStdioClient(cfg mcpdomain.ServerConfig, log *zap.Logger) Client {
	if log == nil {
		log = zap.NewNop()
	}
	return &stdioClient{
		cfg:    cfg,
		log:    log.Named("mcp." + cfg.Name),
		stderr: newRingBuffer(stderrBufferMax),
	}
}

// Initialize spawns the subprocess and runs the MCP handshake; stderr tees to zap + ring.
//
// Initialize 起子进程并走 MCP 握手；stderr 同时进 zap 与环形缓冲。
func (c *stdioClient) Initialize(ctx context.Context) error {
	cmd := exec.Command(c.cfg.Command, c.cfg.Args...)
	cmd.Env = composeEnv(c.cfg.Env)

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("mcp.Client.Initialize: stderr pipe: %w", err)
	}
	go c.drainStderr(stderrPipe)

	c.cmd = cmd

	transport := &mcpsdk.CommandTransport{Command: cmd}
	sdkClient := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    "forgify",
		Version: "1.2.0",
	}, nil)

	session, err := sdkClient.Connect(ctx, transport, nil)
	if err != nil {
		return fmt.Errorf("mcp.Client.Initialize: connect %s: %w: %v",
			c.cfg.Name, mcpdomain.ErrServerNotConnected, err)
	}
	c.session = session
	return nil
}

// ListTools fetches tools/list and converts SDK Tools to domain ToolDefs (stamps ServerName).
//
// ListTools 调 tools/list 并把 SDK Tool 转 domain ToolDef（带 ServerName）。
func (c *stdioClient) ListTools(ctx context.Context) ([]mcpdomain.ToolDef, error) {
	if c.session == nil {
		return nil, fmt.Errorf("mcp.Client.ListTools: %w", mcpdomain.ErrServerNotConnected)
	}
	res, err := c.session.ListTools(ctx, &mcpsdk.ListToolsParams{})
	if err != nil {
		return nil, fmt.Errorf("mcp.Client.ListTools %s: %w: %v",
			c.cfg.Name, mcpdomain.ErrToolCallFailed, err)
	}
	out := make([]mcpdomain.ToolDef, 0, len(res.Tools))
	for _, t := range res.Tools {
		schemaJSON, _ := json.Marshal(t.InputSchema)
		out = append(out, mcpdomain.ToolDef{
			ServerName:  c.cfg.Name,
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
func (c *stdioClient) CallTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	if c.session == nil {
		return "", fmt.Errorf("mcp.Client.CallTool %s/%s: %w",
			c.cfg.Name, name, mcpdomain.ErrServerNotConnected)
	}

	var argsMap any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &argsMap); err != nil {
			return "", fmt.Errorf("mcp.Client.CallTool %s/%s: parse args: %w",
				c.cfg.Name, name, err)
		}
	}

	res, err := c.session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      name,
		Arguments: argsMap,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return "", fmt.Errorf("mcp.Client.CallTool %s/%s: %w: %v",
				c.cfg.Name, name, mcpdomain.ErrToolCallTimeout, err)
		}
		return "", fmt.Errorf("mcp.Client.CallTool %s/%s: %w: %v",
			c.cfg.Name, name, mcpdomain.ErrToolCallFailed, err)
	}
	if res.IsError {
		return "", fmt.Errorf("mcp.Client.CallTool %s/%s: %w: %s",
			c.cfg.Name, name, mcpdomain.ErrToolCallFailed, joinContent(res.Content))
	}
	return joinContent(res.Content), nil
}

// Close shuts down the session; idempotent (SDK handles SIGTERM → 5s → SIGKILL).
//
// Close 关停 session；幂等（SDK 内部 SIGTERM → 5s → SIGKILL）。
func (c *stdioClient) Close() error {
	if c.session == nil {
		return nil
	}
	err := c.session.Close()
	c.session = nil
	return err
}

// StderrTail returns the current ring-buffer contents.
//
// StderrTail 返当前环形缓冲内容。
func (c *stdioClient) StderrTail() string {
	return c.stderr.String()
}

// drainStderr reads stderr lines, levels them by content, and keeps the tail in the ring.
//
// drainStderr 按行读 stderr，按内容选 log 级别，并把尾部留在环形缓冲。
func (c *stdioClient) drainStderr(r io.Reader) {
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

// stderrIndicatesWarnOrError reports whether a stderr line self-identifies as warn/error.
//
// stderrIndicatesWarnOrError 判一行 stderr 是否自报 warn 或 error。
func stderrIndicatesWarnOrError(line string) bool {
	upper := strings.ToUpper(line)
	return strings.Contains(upper, "WARNING") ||
		strings.Contains(upper, "ERROR") ||
		strings.Contains(upper, "FATAL") ||
		strings.Contains(upper, "EXCEPTION") ||
		strings.Contains(upper, "TRACEBACK")
}

// composeEnv layers per-server env atop os.Environ(); nil means SDK inherits everything.
//
// composeEnv 把 per-server env 叠加在 os.Environ() 上；nil 表示 SDK 默认继承全部。
func composeEnv(extras map[string]string) []string {
	if len(extras) == 0 {
		return nil
	}
	out := append([]string(nil), osEnviron()...)
	for k, v := range extras {
		out = append(out, k+"="+v)
	}
	return out
}

var osEnviron = func() []string {
	return defaultOSEnviron()
}

// joinContent flattens an MCP content array; text is verbatim, non-text → placeholder.
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

func newRingBuffer(capacity int) *ringBuffer {
	return &ringBuffer{cap: capacity}
}

// WriteLine appends a line + newline; trims from head when capacity exceeded.
//
// WriteLine 追加 line + \n；超出容量时从头部裁剪。
func (r *ringBuffer) WriteLine(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, line...)
	r.buf = append(r.buf, '\n')
	if len(r.buf) > r.cap {
		r.buf = r.buf[len(r.buf)-r.cap:]
	}
}

// String returns a copy of the current contents.
//
// String 返当前内容的拷贝。
func (r *ringBuffer) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return string(r.buf)
}
