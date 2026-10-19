package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/connectors/drive"
	connectionsapi "github.com/wins/jaz/backend/internal/httpapi/connections"
	mcpruntime "github.com/wins/jaz/backend/internal/mcp"
	"github.com/wins/jaz/backend/internal/mcpconfig"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/tools"
)

func TestDriveConnectionSignInRefreshAndDisconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var refreshes atomic.Int32
	var searches atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if r.FormValue("client_id") != "drive-client" || r.FormValue("client_secret") != "drive-secret" {
				t.Error("token request used the wrong client")
			}
			switch r.FormValue("grant_type") {
			case "authorization_code":
				if r.FormValue("code") != "approved" || r.FormValue("code_verifier") == "" {
					t.Error("missing authorization code or PKCE verifier")
				}
			case "refresh_token":
				if r.FormValue("refresh_token") != "refresh-drive" {
					t.Error("wrong refresh token")
				}
				refreshes.Add(1)
			default:
				t.Errorf("unexpected grant: %s", r.FormValue("grant_type"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"drive-access","refresh_token":"refresh-drive","token_type":"Bearer","expires_in":1}`)
		case "/drive/v3/about":
			if r.Header.Get("Authorization") != "Bearer drive-access" || r.URL.Query().Get("fields") != "user(emailAddress,displayName)" {
				t.Error("Drive account request was not authenticated or projected")
			}
			_, _ = io.WriteString(w, `{"user":{"emailAddress":"work@example.com","displayName":"Work"}}`)
		case "/drive/v3/files":
			if r.Header.Get("Authorization") != "Bearer drive-access" {
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}
			q := r.URL.Query()
			if q.Get("q") != "trashed = false and ('shared-folder' in parents)" || q.Get("supportsAllDrives") != "true" || q.Get("includeItemsFromAllDrives") != "true" {
				t.Errorf("Drive search query = %v", q)
			}
			searches.Add(1)
			_, _ = io.WriteString(w, `{"files":[{"id":"shared-file","name":"Shared-folder document","mimeType":"application/vnd.google-apps.document"}]}`)
		default:
			t.Errorf("unexpected upstream path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	baseTransport := http.DefaultTransport
	upstreamURL, _ := url.Parse(upstream.URL)
	http.DefaultTransport = driveTestTransport{base: baseTransport, upstream: upstreamURL}
	t.Cleanup(func() {
		http.DefaultTransport = baseTransport
	})

	catalog := connections.NewCatalog()
	oauth := NewConnectionOAuthService(store, Config{Connections: ConnectionsConfig{
		Drive: GoogleConnectionConfig{OAuthClientID: "drive-client", OAuthClientSecret: "drive-secret"},
	}})
	connect := connections.NewConnectService(catalog, oauth, nil, nil, nil)
	registry := tools.NewRegistry()
	manager := mcpruntime.NewManager(connectionMCPServerReader{store: store, catalog: catalog}, store, registry, log.New(io.Discard))
	defer manager.Close()
	handler := connectionsapi.NewConnectHandler(connect, oauth, nil, manager, "http://127.0.0.1:5299")
	request := httptest.NewRequest(http.MethodPost, "/v1/connections/plugins/google_drive/connect", nil)
	request.SetPathValue("id", drive.ProviderID)
	response := httptest.NewRecorder()
	handler.Start(response, request)
	var start connections.ConnectStart
	if err := json.Unmarshal(response.Body.Bytes(), &start); err != nil || response.Code != http.StatusOK {
		t.Fatalf("start: %d %s, error = %v", response.Code, response.Body.String(), err)
	}
	authURL, err := url.Parse(start.AuthURL)
	if err != nil {
		t.Fatal(err)
	}
	query := authURL.Query()
	if start.Type != "oauth" || authURL.Host != "accounts.google.com" || query.Get("client_id") != "drive-client" || query.Get("access_type") != "offline" || query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") == "" {
		t.Fatalf("sign-in = %+v", start)
	}
	for _, scope := range []string{"https://www.googleapis.com/auth/drive.readonly", "https://www.googleapis.com/auth/drive.file"} {
		if !slices.Contains(strings.Fields(query.Get("scope")), scope) {
			t.Fatalf("missing scope %s", scope)
		}
	}
	response = httptest.NewRecorder()
	handler.Callback(response, httptest.NewRequest(http.MethodGet, "/v1/connections/oauth/callback?code=approved&state="+url.QueryEscape(query.Get("state")), nil))
	if response.Code != http.StatusOK {
		t.Fatalf("callback: %d %s", response.Code, response.Body.String())
	}
	accounts, err := store.ListConnections(ctx, drive.ProviderID)
	if err != nil || len(accounts) != 1 || accounts[0].AccountID != "work@example.com" {
		t.Fatalf("accounts = %+v, error = %v", accounts, err)
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for manager.Status(accounts[0].ID).Status != "connected" {
		select {
		case <-ctx.Done():
			t.Fatalf("MCP status after sign-in = %+v", manager.Status(accounts[0].ID))
		case <-ticker.C:
		}
	}
	definitions := registry.Definitions()
	if len(definitions) != 8 {
		t.Fatalf("tools = %+v", definitions)
	}
	proxy := httptest.NewServer(manager.Handler())
	defer proxy.Close()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "agent", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcpsdk.StreamableClientTransport{Endpoint: proxy.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	listed, err := session.ListTools(ctx, nil)
	if err != nil || len(listed.Tools) != 8 {
		t.Fatalf("agent tool discovery = %+v, error = %v", listed, err)
	}
	name := "mcp_" + accounts[0].AccountRef() + "_search_files"
	before := refreshes.Load()
	result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: map[string]any{"query": "parentId = 'shared-folder'"}})
	encoded, _ := json.Marshal(result)
	if err != nil || result.IsError || !strings.Contains(string(encoded), "Shared-folder document") || searches.Load() != 1 || refreshes.Load() <= before {
		t.Fatalf("search after expiry: %+v, error = %v, refreshes = %d", result, err, refreshes.Load())
	}
	manager.Refresh(ctx)
	result, err = session.CallTool(ctx, &mcpsdk.CallToolParams{Name: name, Arguments: map[string]any{"query": "parentId = 'shared-folder'"}})
	if err != nil || result.IsError || searches.Load() != 2 {
		t.Fatalf("search after catalog refresh = %+v, %v", result, err)
	}
	if _, ok, err := store.LoadToken(ctx, mcpconfig.OAuthConnectionID(accounts[0].ID)); err != nil || ok {
		t.Fatalf("grant was duplicated under MCP: exists = %v, error = %v", ok, err)
	}
	service := connections.NewService(catalog, store, nil)
	if result, err := service.DisconnectAccount(ctx, accounts[0].ID); err != nil || !result.MCPServersChanged {
		t.Fatalf("disconnect = %+v, error = %v", result, err)
	}
	manager.Refresh(ctx)
	if len(registry.Definitions()) != 0 {
		t.Fatal("Drive tools remain available after disconnect")
	}
	listed, err = session.ListTools(ctx, nil)
	if err != nil || len(listed.Tools) != 0 {
		t.Fatalf("agent tools after disconnect = %+v, %v", listed, err)
	}
	if _, ok, err := store.LoadToken(ctx, accounts[0].ID); err != nil || ok {
		t.Fatalf("account grant remains: exists = %v, error = %v", ok, err)
	}
}

type driveTestTransport struct {
	base     http.RoundTripper
	upstream *url.URL
}

func (t driveTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.Host {
	case "oauth2.googleapis.com", "www.googleapis.com", "drivemcp.googleapis.com":
		copy := req.Clone(req.Context())
		copy.URL.Scheme = t.upstream.Scheme
		copy.URL.Host = t.upstream.Host
		copy.Host = t.upstream.Host
		return t.base.RoundTrip(copy)
	default:
		return t.base.RoundTrip(req)
	}
}
