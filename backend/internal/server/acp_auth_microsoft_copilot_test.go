package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	m365copilotapi "github.com/wins/jaz/backend/internal/httpapi/m365copilot"
	"github.com/wins/jaz/backend/internal/m365copilot"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/oauthbroker"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestMicrosoftCopilotOAuthSettingsCallbackAndDisconnect(t *testing.T) {
	entra := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/organizations/oauth2/v2.0/token" {
			http.NotFound(w, r)
			return
		}
		header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
		payload := base64.RawURLEncoding.EncodeToString([]byte(`{"tid":"11111111-2222-3333-4444-555555555555","iss":"https://login.microsoftonline.com/11111111-2222-3333-4444-555555555555/v2.0"}`))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token_type":    "Bearer",
			"access_token":  "server-work-token",
			"refresh_token": "server-refresh-token",
			"expires_in":    3600,
			"id_token":      header + "." + payload + ".",
		})
	}))
	defer entra.Close()

	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Missing client ID is validated through the real AuthService, not a nil fallback.
	unconfigured := m365copilot.NewAuthService(store, m365copilot.OAuthConfig{}, oauthbroker.Broker{})
	unconfiguredSrv := &Server{
		Store:            store,
		Root:             root,
		AuthKey:          "secret-key",
		ModelCatalog:     modelcatalog.NewService(nil),
		MicrosoftCopilot: unconfigured,
	}
	missingClientReq := httptest.NewRequest(http.MethodPost, "/v1/acp/agents/microsoft_copilot/auth/login", strings.NewReader(`{"auth":{"mode":"jaz_profile"}}`))
	missingClientReq.Host = "127.0.0.1:5299"
	missingClientReq.Header.Set("Authorization", "Bearer secret-key")
	missingClientReq.Header.Set("Content-Type", "application/json")
	missingClientRes := httptest.NewRecorder()
	unconfiguredSrv.Handler().ServeHTTP(missingClientRes, missingClientReq)
	if missingClientRes.Code != http.StatusBadRequest || !strings.Contains(missingClientRes.Body.String(), "JAZ_MICROSOFT_COPILOT_OAUTH_CLIENT_ID") {
		t.Fatalf("unconfigured login status = %d, body = %s", missingClientRes.Code, missingClientRes.Body.String())
	}

	msAuth := m365copilot.NewAuthService(store, m365copilot.OAuthConfig{
		ClientID:         "entra-server-client",
		AuthorityBaseURL: entra.URL,
	}, oauthbroker.Broker{})
	msCallback := m365copilotapi.NewHandler(msAuth)
	srv := &Server{
		Store:            store,
		Root:             root,
		AuthKey:          "secret-key",
		ModelCatalog:     modelcatalog.NewService(nil),
		MicrosoftCopilot: msAuth,
		Routes: Routes{
			{Pattern: "GET " + m365copilot.OAuthCallbackPath, Handler: http.HandlerFunc(msCallback.Callback)},
		},
	}
	handler := srv.Handler()

	// Unauthenticated PUT enabling microsoft_copilot must fail cleanly.
	putFail := httptest.NewRequest(http.MethodPut, "/v1/settings/agents", strings.NewReader(`{"acp":{"microsoft_copilot":{"enabled":true}}}`))
	putFail.Header.Set("Authorization", "Bearer secret-key")
	putFail.Header.Set("Content-Type", "application/json")
	putFailRes := httptest.NewRecorder()
	handler.ServeHTTP(putFailRes, putFail)
	if putFailRes.Code != http.StatusBadRequest {
		t.Fatalf("enabling unauthenticated microsoft_copilot status = %d, body = %s", putFailRes.Code, putFailRes.Body.String())
	}

	// Start sign-in via standard ACP auth login route.
	startReq := httptest.NewRequest(http.MethodPost, "/v1/acp/agents/microsoft_copilot/auth/login", strings.NewReader(`{"auth":{"mode":"jaz_profile"}}`))
	startReq.Host = "127.0.0.1:5299"
	startReq.Header.Set("Authorization", "Bearer secret-key")
	startReq.Header.Set("Content-Type", "application/json")
	startRes := httptest.NewRecorder()
	handler.ServeHTTP(startRes, startReq)
	if startRes.Code != http.StatusOK {
		t.Fatalf("start login status = %d, body = %s", startRes.Code, startRes.Body.String())
	}
	var started acpAuthLoginResponse
	if err := json.Unmarshal(startRes.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.Status != "running" || !strings.HasPrefix(started.AuthURL, entra.URL+"/organizations/oauth2/v2.0/authorize?") {
		t.Fatalf("started login = %#v", started)
	}
	parsedAuthURL, err := url.Parse(started.AuthURL)
	if err != nil {
		t.Fatal(err)
	}
	state := parsedAuthURL.Query().Get("state")
	if state == "" {
		t.Fatalf("missing state in auth_url: %s", started.AuthURL)
	}

	// Public OAuth callback works WITHOUT Authorization header (browser redirect).
	cbReq := httptest.NewRequest(http.MethodGet, m365copilot.OAuthCallbackPath+"?code=auth-code-1&state="+url.QueryEscape(state), nil)
	cbRes := httptest.NewRecorder()
	handler.ServeHTTP(cbRes, cbReq)
	if cbRes.Code != http.StatusOK || !strings.Contains(cbRes.Body.String(), "Connected") {
		t.Fatalf("callback status = %d, body = %s", cbRes.Code, cbRes.Body.String())
	}

	// Poll login job with auth header until succeeded.
	var done acpAuthLoginResponse
	for range 100 {
		pollReq := httptest.NewRequest(http.MethodGet, "/v1/acp/auth-logins/"+started.ID, nil)
		pollReq.Header.Set("Authorization", "Bearer secret-key")
		pollRes := httptest.NewRecorder()
		handler.ServeHTTP(pollRes, pollReq)
		if pollRes.Code != http.StatusOK {
			t.Fatalf("poll status = %d, body = %s", pollRes.Code, pollRes.Body.String())
		}
		if err := json.Unmarshal(pollRes.Body.Bytes(), &done); err != nil {
			t.Fatal(err)
		}
		if done.Status != "running" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if done.Status != "succeeded" {
		t.Fatalf("login job = %#v, want succeeded", done)
	}

	// Enabling microsoft_copilot in settings now succeeds.
	putOK := httptest.NewRequest(http.MethodPut, "/v1/settings/agents", strings.NewReader(`{"acp":{"microsoft_copilot":{"enabled":true}}}`))
	putOK.Header.Set("Authorization", "Bearer secret-key")
	putOK.Header.Set("Content-Type", "application/json")
	putOKRes := httptest.NewRecorder()
	handler.ServeHTTP(putOKRes, putOK)
	if putOKRes.Code != http.StatusOK {
		t.Fatalf("enable microsoft_copilot status = %d, body = %s", putOKRes.Code, putOKRes.Body.String())
	}

	// Disconnect removes encrypted OAuth token and disables microsoft_copilot.
	discReq := httptest.NewRequest(http.MethodPost, "/v1/acp/agents/microsoft_copilot/auth/disconnect", nil)
	discReq.RemoteAddr = "127.0.0.1:12345"
	discReq.Host = "127.0.0.1:5299"
	discReq.Header.Set("Authorization", "Bearer secret-key")
	discRes := httptest.NewRecorder()
	handler.ServeHTTP(discRes, discReq)
	if discRes.Code != http.StatusOK {
		t.Fatalf("disconnect status = %d, body = %s", discRes.Code, discRes.Body.String())
	}
	var disconnected acpDisconnectResponse
	if err := json.Unmarshal(discRes.Body.Bytes(), &disconnected); err != nil {
		t.Fatal(err)
	}
	if disconnected.Settings.Enabled || disconnected.Auth.Authenticated || disconnected.Auth.Ready {
		t.Fatalf("unexpected disconnect response: %#v", disconnected)
	}
	if acp.ProbeAgentAuth(acp.AgentMicrosoftCopilot, acp.AgentConfig{BuiltinBridge: true, MicrosoftCopilot: msAuth}, root, nil).Authenticated {
		t.Fatal("credentials remained authenticated after disconnect")
	}
}
