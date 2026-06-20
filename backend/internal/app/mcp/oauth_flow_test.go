package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"go.uber.org/zap"

	mcpdomain "github.com/sunweilin/anselm/backend/internal/domain/mcp"
	reqctxpkg "github.com/sunweilin/anselm/backend/internal/pkg/reqctx"
)

// fakeAuthServer plays a full OAuth 2.1 + DCR authorization server AND the protected MCP resource:
// 401 discovery → PRM (RFC 9728) → AS metadata (RFC 8414) → DCR → token (auth-code + refresh). The
// /token endpoint issues AT-1/RT-1 for the code exchange and AT-2/RT-2 for a refresh, so tests can
// tell the two apart.
//
// fakeAuthServer 扮演完整 OAuth 2.1 + DCR 授权服务器兼受保护 MCP 资源。
func fakeAuthServer(t *testing.T) *httptest.Server { return fakeAuthServerDCR(t, true) }

// fakeAuthServerDCR plays the AS; dcr controls whether it advertises a registration_endpoint and
// accepts DCR (dcr=false models Box/Entra: no DCR, the user brings their own client).
//
// fakeAuthServerDCR 扮演 AS；dcr 控制是否通告注册端点 + 是否接受 DCR（dcr=false 模拟 Box/Entra：无 DCR、用户自带客户端）。
func fakeAuthServerDCR(t *testing.T, dcr bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	base := srv.URL

	writeJSON := func(w http.ResponseWriter, code int, body string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}
	// the protected MCP resource: unauthenticated → 401 advertising its resource metadata.
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+base+`/.well-known/oauth-protected-resource/mcp"`)
		w.WriteHeader(http.StatusUnauthorized)
	})
	mux.HandleFunc("/.well-known/oauth-protected-resource/mcp", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, 200, `{"resource":"`+base+`/mcp","authorization_servers":["`+base+`"]}`)
	})
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, _ *http.Request) {
		reg := ""
		if dcr {
			reg = `"registration_endpoint":"` + base + `/register",`
		}
		writeJSON(w, 200, `{"issuer":"`+base+`","authorization_endpoint":"`+base+`/authorize","token_endpoint":"`+base+`/token",`+reg+`"code_challenge_methods_supported":["S256"]}`)
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, _ *http.Request) {
		if !dcr {
			writeJSON(w, 403, `{"error":"registration not allowed"}`)
			return
		}
		writeJSON(w, 201, `{"client_id":"test-client"}`)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.FormValue("grant_type") {
		case "authorization_code":
			if r.FormValue("code") == "testcode" && r.FormValue("code_verifier") != "" && r.FormValue("resource") != "" {
				writeJSON(w, 200, `{"access_token":"AT-1","refresh_token":"RT-1","token_type":"Bearer","expires_in":3600}`)
				return
			}
		case "refresh_token":
			if r.FormValue("refresh_token") != "" {
				writeJSON(w, 200, `{"access_token":"AT-2","refresh_token":"RT-2","token_type":"Bearer","expires_in":3600}`)
				return
			}
		}
		writeJSON(w, 400, `{"error":"invalid_grant"}`)
	})
	t.Cleanup(srv.Close)
	return srv
}

// fakeOpener simulates the user consenting in the browser: it reads the redirect_uri + state from
// the authorize URL and drives the loopback callback with a canned code.
//
// fakeOpener 模拟用户在浏览器同意：从 authorize URL 读 redirect_uri + state，用预置 code 驱动 loopback 回调。
type fakeOpener struct {
	code string
	hits chan string
}

func (f *fakeOpener) Open(authURL string) error {
	u, err := url.Parse(authURL)
	if err != nil {
		return err
	}
	q := u.Query()
	redirect := q.Get("redirect_uri")
	state := q.Get("state")
	if f.hits != nil {
		f.hits <- authURL
	}
	go func() {
		resp, err := http.Get(redirect + "?code=" + url.QueryEscape(f.code) + "&state=" + url.QueryEscape(state))
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	return nil
}

// TestAuthorizeOAuth_FullFlow drives the whole interactive flow against the fake server with a fake
// browser, asserting it returns a complete grant — discovery, DCR, PKCE authorize, and exchange all
// wired correctly.
//
// TestAuthorizeOAuth_FullFlow 用假服务器 + 假浏览器跑完整交互流程，断言返回完整授权。
func TestAuthorizeOAuth_FullFlow(t *testing.T) {
	as := fakeAuthServer(t)
	hits := make(chan string, 1)
	svc := NewService(newFakeRepo(), nil, &fakeSandbox{}, zap.NewNop())
	svc.SetBrowserOpener(&fakeOpener{code: "testcode", hits: hits})

	creds, err := svc.authorizeOAuth(context.Background(), as.URL+"/mcp", "", "", nil)
	if err != nil {
		t.Fatalf("authorizeOAuth: %v", err)
	}
	if creds.AccessToken != "AT-1" || creds.RefreshToken != "RT-1" {
		t.Errorf("tokens wrong: %+v", creds)
	}
	if creds.ClientID != "test-client" {
		t.Errorf("client_id = %q, want test-client (DCR)", creds.ClientID)
	}
	if creds.Resource != as.URL+"/mcp" {
		t.Errorf("resource = %q, want %q", creds.Resource, as.URL+"/mcp")
	}
	if creds.TokenEndpoint != as.URL+"/token" {
		t.Errorf("token endpoint = %q", creds.TokenEndpoint)
	}
	if creds.Expiry.IsZero() {
		t.Error("expiry must be set from expires_in")
	}
	// the authorize URL the browser got must carry PKCE S256 + the resource indicator.
	authURL := <-hits
	if u, _ := url.Parse(authURL); u.Query().Get("code_challenge_method") != "S256" || u.Query().Get("resource") == "" {
		t.Errorf("authorize URL missing PKCE/resource: %s", authURL)
	}
}

// TestAuthorizeOAuth_BYOClient drives the bring-your-own-client path against a NO-DCR server (Box /
// Entra shape): the user supplies their own client_id/secret, the flow must skip DCR (the /register
// endpoint 403s) and complete using the supplied client.
//
// TestAuthorizeOAuth_BYOClient 对无 DCR 服务器（Box/Entra 形态）跑自带客户端路径：用户给出自己的
// client_id/secret，流程须跳过 DCR（/register 返 403）、用给定客户端跑完。
func TestAuthorizeOAuth_BYOClient(t *testing.T) {
	as := fakeAuthServerDCR(t, false) // no DCR: /register 403s, AS advertises no registration_endpoint
	svc := NewService(newFakeRepo(), nil, &fakeSandbox{}, zap.NewNop())
	svc.SetBrowserOpener(&fakeOpener{code: "testcode"})

	creds, err := svc.authorizeOAuth(context.Background(), as.URL+"/mcp", "my-own-client-id", "my-own-secret", nil)
	if err != nil {
		t.Fatalf("BYO authorizeOAuth: %v", err)
	}
	if creds.ClientID != "my-own-client-id" || creds.ClientSecret != "my-own-secret" {
		t.Errorf("must use the user-supplied client, got id=%q secret=%q", creds.ClientID, creds.ClientSecret)
	}
	if creds.AccessToken != "AT-1" {
		t.Errorf("token exchange failed: %+v", creds)
	}
}

// TestAuthorizeOAuth_NoDCRNoClient confirms that without a supplied client AND without DCR, the flow
// refuses (ErrOAuthNotSupported) rather than silently failing.
//
// TestAuthorizeOAuth_NoDCRNoClient 确认既无自带客户端又无 DCR 时，流程拒绝（ErrOAuthNotSupported）而非静默失败。
func TestAuthorizeOAuth_NoDCRNoClient(t *testing.T) {
	as := fakeAuthServerDCR(t, false)
	svc := NewService(newFakeRepo(), nil, &fakeSandbox{}, zap.NewNop())
	svc.SetBrowserOpener(&fakeOpener{code: "testcode"})
	if _, err := svc.authorizeOAuth(context.Background(), as.URL+"/mcp", "", "", nil); !errors.Is(err, mcpdomain.ErrOAuthNotSupported) {
		t.Errorf("err = %v, want ErrOAuthNotSupported", err)
	}
}

// TestTokenSource_RefreshesAndPersists verifies the runtime token path: an expired access token is
// refreshed via the refresh token and the rotated bundle is written back to the store.
//
// TestTokenSource_RefreshesAndPersists 验证运行时 token 路径：过期 access token 经 refresh token 刷新、
// 轮换后的束写回 store。
func TestTokenSource_RefreshesAndPersists(t *testing.T) {
	as := fakeAuthServer(t)
	repo := newFakeRepo()
	ctx := reqctxpkg.SetWorkspaceID(context.Background(), "ws_1")
	srv := &mcpdomain.Server{
		ID: "mcp_1", WorkspaceID: "ws_1", Name: "x", URL: as.URL + "/mcp",
		OAuth: &mcpdomain.OAuthCredentials{
			TokenEndpoint: as.URL + "/token", ClientID: "test-client", Resource: as.URL + "/mcp",
			AccessToken: "AT-old", RefreshToken: "RT-0", Expiry: time.Now().Add(-time.Minute), // expired
		},
	}
	if err := repo.Save(ctx, srv); err != nil {
		t.Fatalf("seed: %v", err)
	}

	svc := NewService(repo, nil, &fakeSandbox{}, zap.NewNop())
	ts := svc.newTokenSource(srv)
	tok, err := ts.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok != "AT-2" {
		t.Errorf("token = %q, want refreshed AT-2", tok)
	}
	got, err := repo.GetByID(ctx, "mcp_1")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.OAuth.AccessToken != "AT-2" || got.OAuth.RefreshToken != "RT-2" {
		t.Errorf("refreshed grant not persisted: %+v", got.OAuth)
	}
}

// TestTokenSource_ReauthWhenNoRefresh asserts an expired token with no refresh token surfaces
// ErrOAuthReauthRequired rather than sending a dead token.
//
// TestTokenSource_ReauthWhenNoRefresh 断言过期且无 refresh token 时透出 ErrOAuthReauthRequired 而非发死 token。
func TestTokenSource_ReauthWhenNoRefresh(t *testing.T) {
	svc := NewService(newFakeRepo(), nil, &fakeSandbox{}, zap.NewNop())
	srv := &mcpdomain.Server{
		ID: "mcp_2", WorkspaceID: "ws_1",
		OAuth: &mcpdomain.OAuthCredentials{AccessToken: "AT", Expiry: time.Now().Add(-time.Minute)},
	}
	ts := svc.newTokenSource(srv)
	if _, err := ts.Token(context.Background()); err != mcpdomain.ErrOAuthReauthRequired {
		t.Errorf("err = %v, want ErrOAuthReauthRequired", err)
	}
}
