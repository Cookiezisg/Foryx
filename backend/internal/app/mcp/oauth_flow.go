package mcp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	mcpdomain "github.com/sunweilin/anselm/backend/internal/domain/mcp"
	oauth "github.com/sunweilin/anselm/backend/internal/infra/mcp/oauth"
	reqctxpkg "github.com/sunweilin/anselm/backend/internal/pkg/reqctx"
)

const (
	// oauthAuthorizeTimeout bounds how long install waits for the user to consent in the browser.
	// oauthAuthorizeTimeout 限定安装等用户在浏览器同意的时长。
	oauthAuthorizeTimeout = 5 * time.Minute
	// oauthRefreshSkew refreshes the access token this long before it actually expires.
	// oauthRefreshSkew 在 access token 真过期前这么久就刷新。
	oauthRefreshSkew = 60 * time.Second
	oauthClientName  = "Anselm"
)

// BrowserOpener opens a URL in the user's default browser. Injectable: production opens the OS
// browser (the sidecar runs on the user's machine); tests inject a fake that drives the loopback
// callback directly with no real browser.
//
// BrowserOpener 在用户默认浏览器打开 URL。可注入：生产开 OS 浏览器（sidecar 跑在用户机上）；测试注入直接
// 驱动 loopback 回调、无真浏览器的假实现。
type BrowserOpener interface {
	Open(target string) error
}

// osBrowserOpener launches the platform's URL handler and reaps it (the launcher exits immediately
// after handing off to the browser — we Wait in a goroutine so it never lingers as a zombie).
//
// osBrowserOpener 起平台 URL 处理器并回收它（处理器把 URL 交给浏览器后立即退出——在 goroutine 里 Wait，
// 绝不留僵尸）。
type osBrowserOpener struct{}

func (osBrowserOpener) Open(target string) error {
	var name string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		name, args = "open", []string{target}
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", target}
	default:
		name, args = "xdg-open", []string{target}
	}
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// authorizeOAuth runs the full interactive OAuth 2.1 flow for a remote MCP server: probe → discover
// (RFC 9728/8414) → DCR (RFC 7591) → PKCE authorize (open browser + loopback callback) → exchange,
// returning the grant to persist. Blocks until the user consents or oauthAuthorizeTimeout.
//
// authorizeOAuth 为 remote MCP server 跑完整交互 OAuth 2.1 流程：探测 → 发现 → 动态注册 → PKCE 授权
// （开浏览器 + loopback 回调）→ 换 token，返回待持久化的授权。阻塞到用户同意或超时。
func (s *Service) authorizeOAuth(ctx context.Context, serverURL, providedClientID, providedClientSecret string) (*mcpdomain.OAuthCredentials, error) {
	hc := http.DefaultClient
	resourceMeta := probeResourceMetadata(ctx, hc, serverURL)
	meta, err := oauth.Discover(ctx, hc, serverURL, resourceMeta)
	if err != nil {
		return nil, err
	}

	cb, err := startCallbackServer()
	if err != nil {
		return nil, fmt.Errorf("mcpapp.authorizeOAuth: %w: callback server: %v", mcpdomain.ErrOAuthAuthorize, err)
	}
	defer cb.close()

	// Two client sources: the user's OWN pre-registered OAuth app (Box / Entra, whose AS has no DCR),
	// else runtime self-registration via DCR. Either way no vendor step.
	// 两种客户端来源：用户自己预注册的 OAuth app（Box/Entra，其 AS 无 DCR），否则经 DCR 运行时自注册。两者都无厂商步骤。
	var clientID, clientSecret string
	if providedClientID != "" {
		clientID, clientSecret = providedClientID, providedClientSecret
	} else {
		if !meta.SupportsDCR() {
			return nil, fmt.Errorf("mcpapp.authorizeOAuth %s: %w", serverURL, mcpdomain.ErrOAuthNotSupported)
		}
		reg, rerr := oauth.Register(ctx, hc, meta.RegistrationEndpoint, oauthClientName, cb.redirectURI, meta.ScopesSupported)
		if rerr != nil {
			return nil, rerr
		}
		clientID, clientSecret = reg.ClientID, reg.ClientSecret
	}
	pkce, err := oauth.NewPKCE()
	if err != nil {
		return nil, fmt.Errorf("mcpapp.authorizeOAuth: %w: %v", mcpdomain.ErrOAuthAuthorize, err)
	}
	state, err := oauth.NewState()
	if err != nil {
		return nil, fmt.Errorf("mcpapp.authorizeOAuth: %w: %v", mcpdomain.ErrOAuthAuthorize, err)
	}

	authURL := oauth.AuthorizeURL(meta, clientID, cb.redirectURI, state, pkce, meta.ScopesSupported)
	if err := s.opener.Open(authURL); err != nil {
		return nil, fmt.Errorf("mcpapp.authorizeOAuth: %w: open browser: %v", mcpdomain.ErrOAuthAuthorize, err)
	}

	wctx, cancel := context.WithTimeout(ctx, oauthAuthorizeTimeout)
	defer cancel()
	select {
	case res := <-cb.result:
		if res.err != nil {
			return nil, fmt.Errorf("mcpapp.authorizeOAuth: %w: %v", mcpdomain.ErrOAuthAuthorize, res.err)
		}
		if res.state != state {
			return nil, fmt.Errorf("mcpapp.authorizeOAuth: %w: state mismatch (possible CSRF)", mcpdomain.ErrOAuthAuthorize)
		}
		if res.code == "" {
			return nil, fmt.Errorf("mcpapp.authorizeOAuth: %w: no authorization code returned", mcpdomain.ErrOAuthAuthorize)
		}
		tok, err := oauth.Exchange(ctx, hc, meta, clientID, clientSecret, res.code, cb.redirectURI, pkce.Verifier, time.Now().UTC())
		if err != nil {
			return nil, err
		}
		return &mcpdomain.OAuthCredentials{
			Resource:            meta.Resource,
			AuthorizationServer: meta.Issuer,
			TokenEndpoint:       meta.TokenEndpoint,
			ClientID:            clientID,
			ClientSecret:        clientSecret,
			Scopes:              meta.ScopesSupported,
			AccessToken:         tok.AccessToken,
			RefreshToken:        tok.RefreshToken,
			Expiry:              tok.Expiry,
		}, nil
	case <-wctx.Done():
		return nil, fmt.Errorf("mcpapp.authorizeOAuth: %w: timed out waiting for authorization", mcpdomain.ErrOAuthAuthorize)
	}
}

// probeResourceMetadata sends an unauthenticated MCP initialize and reads the protected-resource-
// metadata URL from the 401 WWW-Authenticate header (RFC 9728). Best-effort — "" if the server
// doesn't advertise it (Discover then falls back to the well-known path).
//
// probeResourceMetadata 发未认证的 MCP initialize，从 401 WWW-Authenticate 头（RFC 9728）读受保护资源
// 元数据 URL。尽力——server 不通告则返 ""（Discover 退回 well-known 路径）。
func probeResourceMetadata(ctx context.Context, hc *http.Client, serverURL string) string {
	const initBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"anselm","version":"1"}}}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL, strings.NewReader(initBody))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	resp, err := hc.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	return oauth.ResourceMetadataURL(resp.Header.Get("WWW-Authenticate"))
}

// callbackServer is the loopback HTTP listener that catches the OAuth redirect (RFC 8252 native-app
// loopback). It binds 127.0.0.1 on a random port and delivers the (code,state) of the first hit.
//
// callbackServer 是接 OAuth 重定向的 loopback HTTP 监听器（RFC 8252 原生应用 loopback）。绑 127.0.0.1
// 随机端口，投递首次命中的 (code,state)。
type callbackServer struct {
	redirectURI string
	result      chan callbackHit
	srv         *http.Server
}

type callbackHit struct {
	code  string
	state string
	err   error
}

// oauthCallbackPort is the preferred loopback port so a bring-your-own-client user can register one
// exact redirect URI (http://127.0.0.1:47100/callback). DCR clients send the redirect dynamically so
// the port is irrelevant to them; if it's busy we fall back to a random port.
//
// oauthCallbackPort 是首选 loopback 端口，使自带客户端的用户能注册一个确定的 redirect URI。DCR 客户端动态
// 发 redirect、端口无所谓；端口被占则退回随机端口。
const oauthCallbackPort = 47100

func startCallbackServer() (*callbackServer, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", oauthCallbackPort))
	if err != nil {
		if ln, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
			return nil, err
		}
	}
	port := ln.Addr().(*net.TCPAddr).Port
	cb := &callbackServer{
		redirectURI: fmt.Sprintf("http://127.0.0.1:%d/callback", port),
		result:      make(chan callbackHit, 1),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if e := q.Get("error"); e != "" {
			cb.deliver(callbackHit{err: fmt.Errorf("authorization denied: %s %s", e, q.Get("error_description"))})
			_, _ = w.Write([]byte(callbackPage("Authorization failed — you can close this window.")))
			return
		}
		cb.deliver(callbackHit{code: q.Get("code"), state: q.Get("state")})
		_, _ = w.Write([]byte(callbackPage("Authorized — you can close this window and return to Anselm.")))
	})
	cb.srv = &http.Server{Handler: mux}
	go func() { _ = cb.srv.Serve(ln) }()
	return cb, nil
}

func (cb *callbackServer) deliver(h callbackHit) {
	select {
	case cb.result <- h:
	default:
	}
}

func (cb *callbackServer) close() { _ = cb.srv.Close() }

func callbackPage(msg string) string {
	return "<!doctype html><html><head><meta charset=utf-8><title>Anselm</title></head>" +
		"<body style=\"font-family:system-ui;text-align:center;padding-top:4rem;color:#333\"><p>" + msg + "</p></body></html>"
}

// newTokenSource builds the per-connection token provider for an OAuth server: it serves the access
// token, refreshing (and re-persisting) it just before expiry.
//
// newTokenSource 为一个 OAuth server 构造 per-connection token 提供方：供给 access token、临过期前刷新
// （并重存）。
func (s *Service) newTokenSource(srv *mcpdomain.Server) *tokenSource {
	creds := *srv.OAuth // copy: the source mutates its own bundle, never the caller's
	return &tokenSource{svc: s, srvID: srv.ID, wsID: srv.WorkspaceID, creds: &creds}
}

// tokenSource implements mcpinfra.TokenSource. It owns a private copy of the grant; Token refreshes
// under a mutex when the access token nears expiry and persists the rotated bundle back to the store
// on a detached, workspace-seeded context (S9). A blank refresh token, or a failed refresh, surfaces
// ErrOAuthReauthRequired so the call fails loudly rather than hitting the server unauthenticated.
//
// tokenSource 实现 mcpinfra.TokenSource。持授权的私有副本；Token 在 access token 临过期时上锁刷新、把
// 轮换后的束在 detached + workspace 种子 ctx（S9）上重存回 store。refresh token 空或刷新失败即透出
// ErrOAuthReauthRequired，使调用大声失败而非无认证打 server。
type tokenSource struct {
	svc   *Service
	srvID string
	wsID  string
	mu    sync.Mutex
	creds *mcpdomain.OAuthCredentials
}

func (ts *tokenSource) Token(ctx context.Context) (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if !ts.creds.Expired(time.Now().UTC(), oauthRefreshSkew) {
		return ts.creds.AccessToken, nil
	}
	if ts.creds.RefreshToken == "" {
		return "", mcpdomain.ErrOAuthReauthRequired
	}
	tok, err := oauth.Refresh(ctx, http.DefaultClient, ts.creds.TokenEndpoint, ts.creds.ClientID,
		ts.creds.ClientSecret, ts.creds.RefreshToken, ts.creds.Resource, time.Now().UTC())
	if err != nil {
		return "", fmt.Errorf("mcpapp: oauth refresh %s: %w", ts.srvID, mcpdomain.ErrOAuthReauthRequired)
	}
	ts.creds.AccessToken = tok.AccessToken
	ts.creds.RefreshToken = tok.RefreshToken
	ts.creds.Expiry = tok.Expiry
	ts.persist()
	return ts.creds.AccessToken, nil
}

// persist writes the refreshed grant back so a restart reuses it instead of re-authorizing. Best-
// effort: a persist failure only costs a re-refresh next boot, never the live connection.
//
// persist 把刷新后的授权写回，使重启复用而非重新授权。尽力：写失败只是下次 boot 再刷一次，绝不损在用连接。
func (ts *tokenSource) persist() {
	ctx := reqctxpkg.Detached(ts.wsID)
	srv, err := ts.svc.repo.GetByID(ctx, ts.srvID)
	if err != nil {
		ts.svc.log.Warn("mcpapp: oauth token persist: load", zap.String("server", ts.srvID), zap.Error(err))
		return
	}
	creds := *ts.creds
	srv.OAuth = &creds
	if err := ts.svc.repo.Save(ctx, srv); err != nil {
		ts.svc.log.Warn("mcpapp: oauth token persist: save", zap.String("server", ts.srvID), zap.Error(err))
	}
}
