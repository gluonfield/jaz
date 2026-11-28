package cloud_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wins/jaz/backend/internal/cloudsync"
	cloudapi "github.com/wins/jaz/backend/internal/httpapi/cloud"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestACPProfilesFollowsSelectedAccountAndDiscardsStaleResponses(t *testing.T) {
	var connected atomic.Bool
	started, release := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/jaz-server":
			_, _ = w.Write([]byte(`{"mode":"cloud","auth":"oidc"}`))
		case "/v1/auth/me":
			_, _ = w.Write([]byte(`{"org_id":"org","user_id":"member","role":"admin"}`))
		case "/v1/models":
			if connected.Load() {
				_, _ = w.Write([]byte(`{"models":[{"profile_id":"acp:grok","scope":"user"}]}`))
			} else {
				_, _ = w.Write([]byte(`{"models":[]}`))
			}
		case "/v1/acp/profiles/grok":
			if r.Header.Get("Authorization") != "Bearer member-session" {
				t.Error("selected account session token was not forwarded")
			}
			if r.Method == http.MethodPut {
				var body struct {
					Key string `json:"api_key"`
				}
				if json.NewDecoder(r.Body).Decode(&body) != nil || body.Key != "test-key" {
					t.Error("API key did not reach the Cloud profile endpoint")
				}
				connected.Store(true)
			} else {
				connected.Store(false)
			}
			_, _ = w.Write([]byte(`{"agent":"grok"}`))
		case "/v1/acp/profiles":
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
			}
			_, _ = w.Write([]byte(`{"profiles":[{"account_label":"old account"}]}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer upstream.Close()

	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := cloudsync.New(store, nil)
	account, err := svc.Connect(t.Context(), cloudsync.ConnectRequest{URL: upstream.URL, Token: "member-session"})
	if err != nil {
		t.Fatal(err)
	}
	handler := cloudapi.NewHandler(svc)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/acp/profiles", handler.ACPProfiles)
	mux.HandleFunc("/v1/acp/profiles/{agent}", handler.ACPProfiles)
	mux.HandleFunc("/v1/models", handler.ListModels)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set(cloudapi.WorkspaceHeader, account.ActiveWorkspaceID)
		mux.ServeHTTP(rec, req)
		return rec
	}

	const profile = "/v1/acp/profiles/grok"
	for _, step := range []struct{ method, path, body, want string }{
		{"GET", "/v1/models", "", `"models":[]`},
		{"PUT", profile, `{"api_key":"test-key"}`, `"agent":"grok"`},
		{"GET", "/v1/models", "", "acp:grok"},
		{"DELETE", profile, "", `"agent":"grok"`},
		{"GET", "/v1/models", "", `"models":[]`},
	} {
		rec := call(step.method, step.path, step.body)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), step.want) || strings.Contains(rec.Body.String(), "test-key") {
			t.Fatalf("%s %s returned %d %s", step.method, step.path, rec.Code, rec.Body)
		}
	}

	response := make(chan *httptest.ResponseRecorder, 1)
	go func() { response <- call("GET", "/v1/acp/profiles", "") }()
	<-started
	if _, err := svc.SwitchWorkspace(t.Context(), cloudsync.LocalWorkspaceID); err != nil {
		close(release)
		t.Fatal(err)
	}
	if _, err := svc.SwitchWorkspace(t.Context(), account.ActiveWorkspaceID); err != nil {
		close(release)
		t.Fatal(err)
	}
	close(release)
	if rec := <-response; rec.Code != http.StatusConflict || strings.Contains(rec.Body.String(), "old account") {
		t.Fatalf("stale ACP response escaped after switching away and back: %d %s", rec.Code, rec.Body)
	}
}
