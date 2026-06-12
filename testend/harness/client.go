// client.go is the typed black-box HTTP view: N1 envelope decode, workspace header,
// wire-code assertions. Deliberately thin — if a flow needs gymnastics here, that is a
// product finding about the API, not something to hide in the harness.
//
// client.go 是带类型的黑盒 HTTP 视图：N1 envelope 解包、workspace 头、wire code 断言。
// 刻意薄——一个流程若在这里需要体操，那是 API 的产品 finding，不是 harness 该藏的。
package harness

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

// Client binds a Server to one workspace identity.
//
// Client 把一个 Server 绑到一个 workspace 身份。
type Client struct {
	t     *testing.T
	base  string
	ws    string
	httpc *http.Client
}

// Client returns a client without workspace identity (for /workspaces itself + health).
//
// Client 返回不带 workspace 身份的客户端（用于 /workspaces 本身与 health）。
func (s *Server) Client(t *testing.T) *Client {
	return &Client{t: t, base: s.BaseURL, httpc: http.DefaultClient}
}

// WS returns a copy bound to the given workspace id.
//
// WS 返回绑定到给定 workspace id 的副本。
func (c *Client) WS(wsID string) *Client {
	cp := *c
	cp.ws = wsID
	return &cp
}

// Resp is one decoded N1 envelope.
//
// Resp 是一个解包后的 N1 envelope。
type Resp struct {
	Status int
	Data   json.RawMessage
	Code   string // error.code when failed. 失败时的 error.code。
	Msg    string
	Raw    []byte
}

// Do performs one request and decodes the envelope. body nil → no payload.
//
// Do 执行一次请求并解包 envelope。body nil → 无载荷。
func (c *Client) Do(method, path string, body any) *Resp {
	c.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("client: marshal body: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rdr)
	if err != nil {
		c.t.Fatalf("client: new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.ws != "" {
		req.Header.Set(HeaderWorkspace, c.ws)
	}
	resp, err := c.httpc.Do(req)
	if err != nil {
		c.t.Fatalf("client: %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	out := &Resp{Status: resp.StatusCode, Raw: raw}
	var env struct {
		Data  json.RawMessage `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &env); err != nil {
			c.t.Fatalf("client: %s %s: non-envelope body (%d): %s", method, path, resp.StatusCode, raw)
		}
	}
	out.Data = env.Data
	if env.Error != nil {
		out.Code, out.Msg = env.Error.Code, env.Error.Message
	}
	return out
}

// OK asserts a 2xx and unmarshals data into v (v nil → discard).
//
// OK 断言 2xx 并把 data 反序列化进 v（v nil → 丢弃）。
func (r *Resp) OK(t *testing.T, v any) *Resp {
	t.Helper()
	if r.Status < 200 || r.Status > 299 {
		t.Fatalf("want 2xx, got %d code=%s msg=%s body=%s", r.Status, r.Code, r.Msg, r.Raw)
	}
	if v != nil {
		if err := json.Unmarshal(r.Data, v); err != nil {
			t.Fatalf("decode data: %v\n%s", err, r.Data)
		}
	}
	return r
}

// Fail asserts an error response with the exact wire code.
//
// Fail 断言错误响应且 wire code 完全一致。
func (r *Resp) Fail(t *testing.T, wantStatus int, wantCode string) *Resp {
	t.Helper()
	if r.Status != wantStatus || r.Code != wantCode {
		t.Fatalf("want %d/%s, got %d/%s body=%s", wantStatus, wantCode, r.Status, r.Code, r.Raw)
	}
	return r
}

// Field digs a top-level string field out of data (quick id extraction).
//
// Field 从 data 顶层取一个 string 字段（快捷取 id）。
func (r *Resp) Field(t *testing.T, name string) string {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(r.Data, &m); err != nil {
		t.Fatalf("field %s: data not object: %s", name, r.Data)
	}
	var s string
	if err := json.Unmarshal(m[name], &s); err != nil {
		t.Fatalf("field %s: not string in %s", name, r.Data)
	}
	return s
}

// GET/POST/PATCH/DELETE sugar.
func (c *Client) GET(path string) *Resp             { return c.Do(http.MethodGet, path, nil) }
func (c *Client) POST(path string, body any) *Resp  { return c.Do(http.MethodPost, path, body) }
func (c *Client) PATCH(path string, body any) *Resp { return c.Do(http.MethodPatch, path, body) }
func (c *Client) PUT(path string, body any) *Resp   { return c.Do(http.MethodPut, path, body) }
func (c *Client) DELETE(path string) *Resp          { return c.Do(http.MethodDelete, path, nil) }

// Eventually polls fn until true or timeout — write-path ripples (search indexing,
// notifications) are asynchronous by design.
//
// Eventually 轮询 fn 直到为真或超时——写路径涟漪（搜索索引、通知）按设计是异步的。
func Eventually(t *testing.T, timeoutMS int, what string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("Eventually: %s — not within %dms", what, timeoutMS)
}
