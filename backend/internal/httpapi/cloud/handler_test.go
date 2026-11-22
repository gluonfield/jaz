package cloud_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/app"
	"github.com/wins/jaz/backend/internal/cloudsync"
	cloudapi "github.com/wins/jaz/backend/internal/httpapi/cloud"
	sessionsapi "github.com/wins/jaz/backend/internal/httpapi/sessions"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/transcript"
)

func TestCloudHTTPRoutesAndUnifiedSessions(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	var pinnedState bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/.well-known/jaz-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mode": "cloud", "auth": "oidc", "oidc_login_url": "/v1/auth/login"})
		case r.URL.Path == "/v1/auth/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"user_id": "u1", "org_id": "o1", "org_name": "CAS", "email": "a@cas.dev", "display_name": "Alice", "role": "admin"})
		case r.URL.Path == "/v1/spaces":
			_ = json.NewEncoder(w).Encode(map[string]any{"spaces": []map[string]any{{"id": "s1", "org_id": "o1", "name": "Ops"}}})
		case r.URL.Path == "/v1/threads" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"threads": []map[string]any{
				{"id": "ct_1", "org_id": "o1", "owner_user_id": "u1", "run_id": "ct_1", "visibility": "private", "title": "Mirrored Cloud Thread", "status": "idle", "model_profile_id": "vertex", "model": "google/gemini-4-neon", "updated_at": now},
			}})
		case r.URL.Path == "/v1/threads" && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ct_new", "org_id": "o1", "owner_user_id": "u1", "run_id": "ct_new", "visibility": "private",
				"title": "Fresh Cloud", "status": "idle", "model_profile_id": "vertex", "model": "google/gemini-4-neon", "updated_at": now,
			})
		case r.URL.Path == "/v1/threads/ct_1" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ct_1", "org_id": "o1", "owner_user_id": "u1", "run_id": "ct_1", "visibility": "private",
				"title": "Mirrored Cloud Thread", "status": "idle", "pinned": pinnedState, "updated_at": now,
			})
		case r.URL.Path == "/v1/threads/ct_1/events":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "id: 1\nevent: user_message\ndata: %s\n\n", `{"thread_id":"ct_1","seq":1,"type":"user_message","payload":{"text":"ping"}}`)
			fmt.Fprintf(w, "id: 2\nevent: assistant_message\ndata: %s\n\n", `{"thread_id":"ct_1","seq":2,"type":"assistant_message","payload":{"text":"pong"}}`)
		case r.URL.Path == "/v1/threads/ct_1/user-state" && r.Method == http.MethodPut:
			var body struct {
				Pinned *bool `json:"pinned"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Pinned != nil {
				pinnedState = *body.Pinned
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"thread_id": "ct_1", "pinned": pinnedState})
		case r.URL.Path == "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"profile_id": "vertex", "provider": "vertex"}}})
		case r.URL.Path == "/v1/machines":
			_ = json.NewEncoder(w).Encode(map[string]any{"machines": []any{}})
		case r.URL.Path == "/v1/usage/summary":
			_ = json.NewEncoder(w).Encode(map[string]any{"month_start": "2026-11-01T00:00:00Z", "org_spent_usd": 2.0, "user_spent_usd": 1.0})
		case r.URL.Path == "/v1/usage/events":
			_ = json.NewEncoder(w).Encode(map[string]any{"events": []any{}})
		case strings.HasPrefix(r.URL.Path, "/v1/instructions/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"body_md": "", "version": ""})
		case r.URL.Path == "/v1/auth/logout":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.SaveSession(storage.Session{
		ID: "local_1", Slug: "local_1", Title: "Local Thread", Runtime: storage.RuntimeACP, Status: storage.StatusIdle,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	cloudSvc := cloudsync.New(store, nil)
	cloudHandler := cloudapi.NewHandler(cloudSvc)
	listHandler := sessionsapi.NewListHandler(store).WithCloud(cloudSvc)
	msgHandler := sessionsapi.NewMessagesHandler(transcript.NewService(store, nil)).WithCloud(cloudSvc)

	srv := &server.Server{
		Store: store,
		Cloud: cloudSvc,
		Routes: server.Routes{
			{Pattern: "GET /v1/sessions", Handler: listHandler},
			{Pattern: "GET /v1/sessions/{session}/messages", Handler: msgHandler},
			{Pattern: "GET /v1/cloud/status", Handler: http.HandlerFunc(cloudHandler.Status)},
			{Pattern: "POST /v1/cloud/connect", Handler: http.HandlerFunc(cloudHandler.Connect)},
			{Pattern: "POST /v1/cloud/disconnect", Handler: http.HandlerFunc(cloudHandler.Disconnect)},
			{Pattern: "GET /v1/spaces", Handler: http.HandlerFunc(cloudHandler.ListSpaces)},
			{Pattern: "GET /v1/models", Handler: http.HandlerFunc(cloudHandler.ListModels)},
		},
	}
	_ = app.Workspace("")
	h := srv.Handler()

	// Connect to Cloud via /v1/cloud/connect.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/cloud/connect", bytes.NewBufferString(fmt.Sprintf(`{"url":%q,"token":"tok1"}`, upstream.URL)))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("connect status=%d body=%s", rec.Code, rec.Body.String())
	}

	// GET /v1/sessions must return BOTH local_1 and mirrored ct_1.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/sessions?include_children=true", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list sessions status=%d body=%s", rec.Code, rec.Body.String())
	}
	var listed struct {
		Sessions []storage.Session `json:"sessions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Sessions) != 2 {
		t.Fatalf("expected 2 merged sessions (1 local + 1 cloud), got %d: %+v", len(listed.Sessions), listed.Sessions)
	}

	// GET /v1/sessions/ct_1/messages projects cloud thread events transparently.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/sessions/ct_1/messages", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("messages status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"content":"pong"`) {
		t.Fatalf("expected projected assistant reply in messages response: %s", rec.Body.String())
	}

	// POST /v1/sessions with model_profile_id creates a Cloud thread & mirrors it.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(`{"title":"Fresh Cloud","model_profile_id":"vertex","model":"google/gemini-4-neon"}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":"ct_new"`) || !strings.Contains(rec.Body.String(), `"backend":"cloud"`) {
		t.Fatalf("create cloud session status=%d body=%s", rec.Code, rec.Body.String())
	}

	// POST /v1/sessions/ct_1/pin forwards to cloud user-state and updates mirror.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/sessions/ct_1/pin", nil))
	if rec.Code != http.StatusOK || !pinnedState || !strings.Contains(rec.Body.String(), `"pinned":true`) {
		t.Fatalf("pin cloud session status=%d pinned=%v body=%s", rec.Code, pinnedState, rec.Body.String())
	}
}
