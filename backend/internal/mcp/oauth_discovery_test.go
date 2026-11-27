package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/internal/tools"
)

func TestOAuthDiscoveryForAnonymousCatalog(t *testing.T) {
	const metadataRoot = "/.well-known/oauth-protected-resource"
	for _, test := range []struct {
		name, path, resource, bearer string
		headers                      []mcpconfig.Header
		needsAuth                    bool
		publishLate                  bool
		denyFirst                    bool
	}{
		{name: "path metadata", path: metadataRoot + "/catalog", resource: "/catalog", needsAuth: true},
		{name: "root metadata and declined consent", path: metadataRoot, resource: "/catalog", needsAuth: true, denyFirst: true},
		{name: "cached catalog gains OAuth metadata", path: metadataRoot, resource: "/catalog", needsAuth: true, publishLate: true},
		{name: "mismatched resource", path: metadataRoot, resource: "/other"},
		{name: "manual bearer", path: metadataRoot, resource: "/catalog", bearer: "manual-key"},
		{name: "manual header", path: metadataRoot, resource: "/catalog", headers: []mcpconfig.Header{{Name: "Authorization", Value: "Basic manual-key"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			provider := mockAuthServer(t, nil)
			mux := provider.Config.Handler.(*http.ServeMux)
			mux.Handle("/catalog", newEchoHandler())
			var published atomic.Bool
			published.Store(!test.publishLate)
			mux.HandleFunc(test.path, func(w http.ResponseWriter, r *http.Request) {
				if !published.Load() {
					http.NotFound(w, r)
					return
				}
				if r.Header.Get("Authorization") != "" {
					t.Error("discovery sent a configured credential")
				}
				writeJSONResp(w, map[string]any{
					"resource":              provider.URL + test.resource,
					"authorization_servers": []string{provider.URL},
					"scopes_supported":      []string{"catalog:read"},
				})
			})
			server := mcpconfig.Server{ID: "catalog", Name: "catalog", URL: provider.URL + "/catalog", Enabled: true, BearerToken: test.bearer, Headers: test.headers}
			registry := tools.NewRegistry()
			manager := NewManager(&testStore{servers: []mcpconfig.Server{server}}, newMemTokenStore(), registry, log.New(io.Discard))
			defer manager.Close()

			if test.publishLate {
				if status := manager.RefreshServer(ctx, server.ID); status.Status != "connected" {
					t.Fatalf("initial public catalog = %+v", status)
				}
				published.Store(true)
			}
			status := manager.RefreshServer(ctx, server.ID)
			if !test.needsAuth {
				if status.Status != "connected" || len(registry.Definitions()) != 1 {
					t.Fatalf("public or manually authenticated catalog was gated: %+v", status)
				}
				return
			}
			if status.Status != "needs_auth" || len(registry.Definitions()) != 0 {
				t.Fatalf("anonymous discovery exposed tools before sign-in: %+v", status)
			}
			if test.denyFirst {
				state := startCatalogAuthorization(t, ctx, manager, server, "catalog:read")
				err := manager.CompleteAuthorization(ctx, mcpconfig.AuthorizationResponse{State: state, Error: "access_denied"})
				if status := manager.Status(server.ID); err == nil || status.Status != "auth_error" || status.Error == "" {
					t.Fatalf("declined consent did not retain sign-in: %+v, %v", status, err)
				}
			}
			completeCatalogAuthorization(t, ctx, manager, server, "catalog:read")
			if status := manager.Status(server.ID); status.Status != "connected" || len(registry.Definitions()) != 1 {
				t.Fatalf("authorized catalog was not published: %+v", status)
			}
		})
	}
}

func TestToolAuthChallengeStartsSignInWithoutReplayingTool(t *testing.T) {
	for _, test := range []struct {
		name         string
		status       int
		challenge    bool
		repeatBare   bool
		repeatStable bool
	}{
		{name: "unauthorized then bare challenge", status: http.StatusUnauthorized, challenge: true, repeatBare: true},
		{name: "insufficient scope", status: http.StatusForbidden, challenge: true},
		{name: "forbidden without OAuth", status: http.StatusForbidden},
		{name: "tool result metadata and reissued grant", status: http.StatusOK, challenge: true, repeatStable: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			provider := mockAuthServer(t, nil)
			provider.tokenExpiresIn = 0
			mux := provider.Config.Handler.(*http.ServeMux)
			endpoint := provider.URL + "/tools"
			challenge := fmt.Sprintf(`Bearer error="insufficient_scope", resource_metadata="%s/auth-discovery", scope="account:read"`, provider.URL)
			mux.HandleFunc("/auth-discovery", func(w http.ResponseWriter, _ *http.Request) {
				writeJSONResp(w, map[string]any{"resource": endpoint, "authorization_servers": []string{provider.URL}})
			})
			var calls atomic.Int32
			var requireConsent atomic.Bool
			remote := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "catalog"}, nil)
			remote.AddTool(&mcpsdk.Tool{Name: "account", InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
				calls.Add(1)
				if requireConsent.Load() || req.Extra.Header.Get("Authorization") != "Bearer access-token-123" {
					return &mcpsdk.CallToolResult{IsError: true, Meta: mcpsdk.Meta{"mcp/www_authenticate": []string{challenge}}}, nil
				}
				return &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "signed in"}}}, nil
			})
			handle := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return remote }, &mcpsdk.StreamableHTTPOptions{JSONResponse: true})
			mux.HandleFunc("/tools", func(w http.ResponseWriter, r *http.Request) {
				if test.status != http.StatusOK && r.Method == http.MethodPost && r.Header.Get("Authorization") == "" {
					var request struct{ Method string }
					body, err := io.ReadAll(r.Body)
					if err != nil || json.Unmarshal(body, &request) != nil {
						http.Error(w, "invalid request", http.StatusBadRequest)
						return
					}
					if request.Method == "tools/call" {
						if count := calls.Add(1); test.repeatBare && count > 1 {
							w.Header().Set("WWW-Authenticate", "Bearer")
						} else if test.challenge {
							w.Header().Set("WWW-Authenticate", challenge)
						}
						http.Error(w, "tool request refused", test.status)
						return
					}
					r.Body = io.NopCloser(bytes.NewReader(body))
				}
				handle.ServeHTTP(w, r)
			})
			server := mcpconfig.Server{ID: "catalog", Name: "catalog", URL: endpoint, Enabled: true}
			registry := tools.NewRegistry()
			manager := NewManager(&testStore{servers: []mcpconfig.Server{server}}, newMemTokenStore(), registry, log.New(io.Discard))
			defer manager.Close()
			if status := manager.RefreshServer(ctx, server.ID); status.Status != "connected" {
				t.Fatalf("public catalog = %+v", status)
			}
			tool, ok := registry.Get("mcp_catalog_account")
			if !ok {
				t.Fatal("account tool was not exposed")
			}
			_, err := tool.Execute(ctx, nil)
			if !test.challenge {
				if status := manager.Status(server.ID); err == nil || status.Status != "connected" {
					t.Fatalf("non-OAuth denial changed connection state: %+v, %v", status, err)
				}
				return
			}
			if !errors.Is(err, errNeedsAuthorization) {
				t.Fatalf("tool call error = %v", err)
			}
			if status := manager.Status(server.ID); status.Status != "needs_auth" {
				t.Fatalf("tool challenge did not change status: %+v", status)
			}
			if test.repeatBare {
				if _, err := tool.Execute(ctx, nil); !errors.Is(err, errNeedsAuthorization) {
					t.Fatalf("bare challenge = %v", err)
				}
			}
			if status := manager.RefreshServer(ctx, server.ID); status.Status != "needs_auth" {
				t.Fatalf("anonymous catalog masked the tool challenge: %+v", status)
			}
			beforeAuth := calls.Load()
			completeCatalogAuthorization(t, ctx, manager, server, "account:read")
			if calls.Load() != beforeAuth {
				t.Fatalf("sign-in replayed a tool call, total calls = %d", calls.Load())
			}
			if status := manager.Status(server.ID); status.Status != "connected" {
				t.Fatalf("granted connection = %+v", status)
			}
			tool, ok = registry.Get("mcp_catalog_account")
			if !ok {
				t.Fatal("authorized tool was not exposed")
			}
			result, err := tool.Execute(ctx, nil)
			if err != nil || !bytes.Contains([]byte(result.Content), []byte("signed in")) {
				t.Fatalf("authorized tool result = %+v, %v", result, err)
			}
			if test.repeatStable {
				requireConsent.Store(true)
				if _, err := tool.Execute(ctx, nil); !errors.Is(err, errNeedsAuthorization) {
					t.Fatalf("scope challenge for current grant = %v", err)
				}
				if status := manager.Status(server.ID); status.Status != "needs_auth" {
					t.Fatalf("previous grant still considered sufficient: %+v", status)
				}
				requireConsent.Store(false)
				completeCatalogAuthorization(t, ctx, manager, server, "account:read")
				if status := manager.Status(server.ID); status.Status != "connected" {
					t.Fatalf("reissued grant still considered insufficient: %+v", status)
				}
			}
		})
	}
}

func completeCatalogAuthorization(t *testing.T, ctx context.Context, manager *Manager, server mcpconfig.Server, scope string) {
	t.Helper()
	state := startCatalogAuthorization(t, ctx, manager, server, scope)
	if err := manager.CompleteAuthorization(ctx, mcpconfig.AuthorizationResponse{State: state, Code: "approved-code"}); err != nil {
		t.Fatal(err)
	}
}

func startCatalogAuthorization(t *testing.T, ctx context.Context, manager *Manager, server mcpconfig.Server, scope string) string {
	t.Helper()
	start := manager.Authorize(ctx, server, mcpconfig.AuthorizeOptions{RedirectURL: "http://127.0.0.1:5599/callback"})
	if start.Status != "needs_auth" || start.AuthURL == "" {
		t.Fatalf("did not get a sign-in URL: %+v", start)
	}
	authURL, err := url.Parse(start.AuthURL)
	if err != nil {
		t.Fatal(err)
	}
	query := authURL.Query()
	if query.Get("scope") != scope || query.Get("resource") != server.URL || query.Get("code_challenge_method") != "S256" {
		t.Fatalf("unexpected authorization parameters: %v", query)
	}
	return query.Get("state")
}
