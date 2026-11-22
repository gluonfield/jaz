package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/serverconfig"
	sqlite "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestAuthenticatedMCPProtocol(t *testing.T) {
	protocol := mcp.NewServer(&mcp.Implementation{Name: "security-fixture", Version: "1"}, nil)
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return protocol
	}, &mcp.StreamableHTTPOptions{JSONResponse: true})
	srv := httptest.NewServer((&Server{AuthKey: "internal-credential"}).withAuth(handler))
	t.Cleanup(srv.Close)
	for _, credential := range []string{"", "internal-credential"} {
		body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"security-probe","version":"1"}}}`
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/mcp/jaztools", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if credential != "" {
			req.Header.Set("Authorization", "Bearer "+credential)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if credential == "" {
			if res.StatusCode != http.StatusUnauthorized {
				t.Fatal("MCP accepted an unauthenticated protocol client")
			}
			continue
		}
		if res.StatusCode != http.StatusOK || res.Header.Get("Mcp-Session-Id") == "" {
			t.Fatalf("authenticated MCP initialization status=%d", res.StatusCode)
		}
		req, err = http.NewRequest(http.MethodDelete, srv.URL+"/mcp/jaztools", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+credential)
		req.Header.Set("Mcp-Session-Id", res.Header.Get("Mcp-Session-Id"))
		res, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
	}
}

func TestInternalEndpointsRequireCredentialsBehindTunnel(t *testing.T) {
	for _, path := range []string{"/mcp/jaztools", "/mcp/jazmem", "/mcp/proxy", "/v1/sessions/thread/browser"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://jaz.example.com"+path, nil)
			req.RemoteAddr = "127.0.0.1:12345"
			req.Header.Set("Origin", "https://web.jaz.chat")
			req.Header.Set("X-Forwarded-Proto", "https")
			res := httptest.NewRecorder()
			(&Server{AuthKey: "root"}).Handler().ServeHTTP(res, req)
			if res.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated endpoint status=%d", res.Code)
			}
		})
	}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:5299/mcp/jaztools", strings.NewReader(`[]`))
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Origin", "https://untrusted.example")
	res := httptest.NewRecorder()
	(&Server{AuthKey: "root"}).Handler().ServeHTTP(res, req)
	if res.Code != http.StatusForbidden || res.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("website-origin request status=%d headers=%v", res.Code, res.Header())
	}
}

func TestResourceTicketsAreStatelessScopedAndInvalidateOnKeyRotation(t *testing.T) {
	srv := &Server{AuthKey: "root"}
	handler := srv.Handler()
	issueViaHTTP := func() ResourceTicket {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/v1/auth/check", nil)
		req.Header.Set("Authorization", "Bearer "+srv.AuthKey)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("auth check status=%d body=%s", res.Code, res.Body.String())
		}
		var body struct {
			ResourceTicket ResourceTicket `json:"resource_ticket"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.ResourceTicket
	}

	first := issueViaHTTP()
	authed := srv.withAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, test := range []struct {
		path   string
		status int
	}{
		{"/v1/bots/events?key=root", http.StatusUnauthorized},
		{"/v1/bots/events?ticket=" + first.Token, http.StatusNoContent},
		{"/v1/bots/events?ticket=" + first.Token + "tamper", http.StatusUnauthorized},
		{"/v1/bots/events?ticket=invalid", http.StatusUnauthorized},
		{"/v1/settings/agents?ticket=" + first.Token, http.StatusUnauthorized},
		{"/v1/sessions/test/terminal?ticket=" + first.Token, http.StatusUnauthorized},
	} {
		res := httptest.NewRecorder()
		authed.ServeHTTP(res, httptest.NewRequest(http.MethodGet, test.path, nil))
		if res.Code != test.status {
			t.Fatalf("%s status=%d want=%d", test.path, res.Code, test.status)
		}
	}
	if validResourceTicket(srv.AuthKey, first.Token, first.ExpiresAt.Add(time.Second)) {
		t.Fatal("expired resource ticket remained valid")
	}

	// 2,000 /v1/auth/check polls must never evict or invalidate an earlier active ticket.
	for range 2000 {
		_ = issueViaHTTP()
	}
	res := httptest.NewRecorder()
	authed.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/bots/events?ticket="+first.Token, nil))
	if res.Code != http.StatusNoContent {
		t.Fatalf("earlier active ticket failed after 2000 polls: status=%d", res.Code)
	}

	// Rotating AuthKey invalidates all previously issued tickets immediately.
	srv.AuthKey = "rotated-root"
	res = httptest.NewRecorder()
	authed.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/bots/events?ticket="+first.Token, nil))
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("rotated auth key accepted old ticket: status=%d", res.Code)
	}
}

func TestPublicClientOriginUsesNormalizedConnectionURL(t *testing.T) {
	srv := &Server{ServerConfig: serverconfig.Config{PublicURL: "https://self-hosted.example/app"}}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://self-hosted.example")
	res := httptest.NewRecorder()
	srv.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusOK || res.Header().Get("Access-Control-Allow-Origin") != "https://self-hosted.example" {
		t.Fatalf("configured client origin rejected: status=%d", res.Code)
	}
}

func TestMCPSettingsRedactAndPreserveSavedSecrets(t *testing.T) {
	store, err := sqlite.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	handler := (&Server{Store: store}).Handler()
	request := httptest.NewRequest(http.MethodPost, "/v1/mcp/servers", strings.NewReader(`{"name":"Private tools","url":"https://example.invalid/mcp","bearer_token":"sensitive-bearer","headers":[{"name":"X-Key","value":"sensitive-header"}]}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("create MCP server: %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "sensitive-") {
		t.Fatal("create response exposed credentials")
	}
	request = httptest.NewRequest(http.MethodPut, "/v1/mcp/servers/"+created.ID, strings.NewReader(`{"name":"Renamed tools","url":"https://example.invalid/mcp","headers":[{"name":"X-Key"}]}`))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	saved, err := store.LoadMCPServer(created.ID)
	if err != nil || response.Code != http.StatusOK || saved.BearerToken != "sensitive-bearer" || len(saved.Headers) != 1 || saved.Headers[0].Value != "sensitive-header" {
		t.Fatalf("editing redacted settings lost credentials: %v, %s", err, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodPut, "/v1/mcp/servers/"+created.ID, strings.NewReader(`{"name":"Renamed tools","url":"https://example.invalid/mcp","bearer_token":"","headers":[]}`))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	saved, err = store.LoadMCPServer(created.ID)
	if err != nil || response.Code != http.StatusOK || saved.BearerToken != "" || len(saved.Headers) != 0 {
		t.Fatalf("explicit credential removal failed: %v, %s", err, response.Body.String())
	}
}
