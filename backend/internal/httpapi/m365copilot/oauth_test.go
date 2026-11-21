package m365copilot

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	core "github.com/wins/jaz/backend/internal/m365copilot"
	"github.com/wins/jaz/backend/internal/oauthbroker"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestCallbackURLPrefersConfiguredBaseAndFallsBackToRequestHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:5299/v1/acp/agents/microsoft_copilot/auth/login", nil)
	if got := CallbackURL(req, "https://jaz.example.com/"); got != "https://jaz.example.com"+core.OAuthCallbackPath {
		t.Fatalf("CallbackURL(configured) = %q", got)
	}
	if got := CallbackURL(req, ""); got != "http://127.0.0.1:5299"+core.OAuthCallbackPath {
		t.Fatalf("CallbackURL(request host) = %q", got)
	}
}

func TestCallbackHandlerRendersSuccessAndFailureHTML(t *testing.T) {
	entra := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
		payload := base64.RawURLEncoding.EncodeToString([]byte(`{"tid":"11111111-2222-3333-4444-555555555555","iss":"https://login.microsoftonline.com/11111111-2222-3333-4444-555555555555/v2.0"}`))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token_type":    "Bearer",
			"access_token":  "work-access-token",
			"refresh_token": "work-refresh-token",
			"expires_in":    3600,
			"id_token":      header + "." + payload + ".",
		})
	}))
	defer entra.Close()

	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	auth := core.NewAuthService(store, core.OAuthConfig{
		ClientID:         "entra-client",
		AuthorityBaseURL: entra.URL,
	}, oauthbroker.Broker{})
	handler := NewHandler(auth)

	badRes := httptest.NewRecorder()
	handler.Callback(badRes, httptest.NewRequest(http.MethodGet, core.OAuthCallbackPath+"?state=unknown&code=abc", nil))
	if badRes.Code != http.StatusBadRequest || !strings.Contains(badRes.Body.String(), "Connection failed") {
		t.Fatalf("unknown state response = %d %q", badRes.Code, badRes.Body.String())
	}

	session, err := auth.StartLogin(t.Context(), "http://127.0.0.1:5299"+core.OAuthCallbackPath)
	if err != nil {
		t.Fatal(err)
	}
	okReq := httptest.NewRequest(http.MethodGet, core.OAuthCallbackPath+"?state="+url.QueryEscape(session.State)+"&code=good-code", nil)
	okRes := httptest.NewRecorder()
	handler.Callback(okRes, okReq)
	if okRes.Code != http.StatusOK || !strings.Contains(okRes.Body.String(), "Connected") || !strings.Contains(okRes.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("valid callback response = %d %q %q", okRes.Code, okRes.Header().Get("Content-Type"), okRes.Body.String())
	}
	if err := <-session.Done; err != nil {
		t.Fatalf("session.Done err = %v", err)
	}
}
