package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultHTTPTimeout is the per-request timeout used when node.Config doesn't specify one.
//
// DefaultHTTPTimeout 节点未指定时的默认 per-request timeout。
const DefaultHTTPTimeout = 30 * time.Second

// HTTPDispatcher bridges workflow http nodes to net/http with an SSRF guard.
//
// HTTPDispatcher 把 workflow http 节点桥接到 net/http，并带 SSRF 守卫。
type HTTPDispatcher struct {
	client *http.Client
}

// NewHTTPDispatcher constructs HTTPDispatcher; pass a custom client for test wiring.
//
// NewHTTPDispatcher 构造 HTTPDispatcher，测试可传自定义 client。
func NewHTTPDispatcher(client *http.Client) *HTTPDispatcher {
	if client == nil {
		client = &http.Client{Timeout: DefaultHTTPTimeout}
	}
	return &HTTPDispatcher{client: client}
}

// Dispatch reads method/url/headers/body from node.Config.
//
// Dispatch 读 method/url/headers/body。
func (d *HTTPDispatcher) Dispatch(ctx context.Context, in DispatchInput) DispatchOutput {
	method, _ := in.Node.Config["method"].(string)
	rawURL, _ := in.Node.Config["url"].(string)
	if rawURL == "" {
		return DispatchOutput{Error: fmt.Errorf("http node %q: url required", in.Node.ID)}
	}
	if method == "" {
		method = http.MethodGet
	}
	method = strings.ToUpper(method)

	if err := ssrfGuard(rawURL); err != nil {
		return DispatchOutput{Error: fmt.Errorf("http node %q: %w", in.Node.ID, err)}
	}

	var body io.Reader
	if bodyVal, ok := in.Node.Config["body"]; ok && bodyVal != nil {
		buf, err := json.Marshal(bodyVal)
		if err != nil {
			return DispatchOutput{Error: fmt.Errorf("http node %q: marshal body: %w", in.Node.ID, err)}
		}
		body = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return DispatchOutput{Error: fmt.Errorf("http node %q: new request: %w", in.Node.ID, err)}
	}
	if headers, ok := in.Node.Config["headers"].(map[string]any); ok {
		for k, v := range headers {
			if s, ok := v.(string); ok {
				req.Header.Set(k, s)
			}
		}
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return DispatchOutput{Error: fmt.Errorf("http node %q: do: %w", in.Node.ID, err)}
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return DispatchOutput{Error: fmt.Errorf("http node %q: read response: %w", in.Node.ID, err)}
	}

	var parsed any
	if json.Valid(respBody) {
		_ = json.Unmarshal(respBody, &parsed)
	} else {
		parsed = string(respBody)
	}
	return DispatchOutput{Outputs: map[string]any{
		"out":    parsed,
		"status": resp.StatusCode,
	}}
}

func ssrfGuard(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("only http/https schemes allowed")
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("empty host")
	}
	if isBlockedHost(host) {
		return fmt.Errorf("ssrf guard: host %q blocked", host)
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("ssrf guard: host %q resolves to blocked %s", host, ip)
		}
	}
	return nil
}

func isBlockedHost(h string) bool {
	h = strings.ToLower(h)
	return h == "localhost" || strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".internal")
}

func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() || ip.IsUnspecified() {
		return true
	}
	return false
}
