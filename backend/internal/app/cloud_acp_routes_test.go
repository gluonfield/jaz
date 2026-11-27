package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/cloudsync"
	cloudapi "github.com/wins/jaz/backend/internal/httpapi/cloud"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestCloudACPOAuthRoutesForwardSelectedAccount(t *testing.T) {
	const base = "/v1/acp/profiles/"
	type route struct {
		method string
		path   string
		body   string
		status int
	}
	steps := []route{
		{"POST", base + "claude/login", "", 202},
		{"POST", base + "claude/login/session/complete", `{"code":"fixture-code#fixture-state"}`, 200},
		{"POST", base + "claude/refresh", `{"force":true}`, 200},
		{"POST", base + "antigravity/login", `{"gcp_project":"fixture-project","gcp_location":"global"}`, 202},
		{"GET", base + "antigravity/login/session", "", 200},
		{"POST", base + "antigravity/refresh", `{"force":true}`, 200},
	}
	wanted := make(map[string]route, len(steps))
	for _, step := range steps {
		wanted[step.method+" "+step.path] = step
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/jaz-server":
			_, _ = io.WriteString(w, `{"mode":"cloud","auth":"oidc"}`)
		case "/v1/auth/me":
			_, _ = io.WriteString(w, `{"org_id":"org","user_id":"member","role":"admin"}`)
		default:
			want, ok := wanted[r.Method+" "+r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			body, err := io.ReadAll(r.Body)
			if err != nil || string(body) != want.body || r.Header.Get("Authorization") != "Bearer fixture-member-session" {
				t.Errorf("wrong selected account or body on %s %s", r.Method, r.URL.Path)
			}
			w.WriteHeader(want.status)
			_, _ = io.WriteString(w, `{"source":"cloud"}`)
		}
	}))
	defer upstream.Close()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cloud := cloudsync.New(store, nil)
	selected, err := cloud.Connect(t.Context(), cloudsync.ConnectRequest{URL: upstream.URL, Token: "fixture-member-session"})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	for _, registered := range appendCloudRoutes(nil, cloud, nil) {
		mux.Handle(registered.Pattern, registered.Handler)
	}
	for _, step := range steps {
		req := httptest.NewRequest(step.method, step.path, strings.NewReader(step.body))
		req.Header.Set(cloudapi.WorkspaceHeader, selected.ActiveWorkspaceID)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != step.status || rec.Header().Get("Cache-Control") != "no-store" || rec.Body.String() != `{"source":"cloud"}` {
			t.Fatalf("%s %s returned %d %s", step.method, step.path, rec.Code, rec.Body)
		}
	}
}
