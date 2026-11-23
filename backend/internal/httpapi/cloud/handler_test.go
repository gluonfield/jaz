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
	usageapi "github.com/wins/jaz/backend/internal/httpapi/usage"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/transcript"
	usagecore "github.com/wins/jaz/backend/internal/usage"
)

func TestCloudHTTPRoutesAndWorkspaceIsolation(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	var pinnedState bool
	newCloudUpstream := func(orgID, orgName, threadID, threadTitle, model string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/.well-known/jaz-server":
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mode": "cloud", "auth": "oidc", "oidc_login_url": "/v1/auth/login"})
			case r.URL.Path == "/v1/auth/me":
				_ = json.NewEncoder(w).Encode(map[string]any{"user_id": "u1", "org_id": orgID, "org_name": orgName, "email": "a@" + orgID + ".dev", "display_name": "Alice", "role": "admin"})
			case r.URL.Path == "/v1/spaces":
				_ = json.NewEncoder(w).Encode(map[string]any{"spaces": []map[string]any{{"id": "s_" + orgID, "org_id": orgID, "name": orgName + " Ops"}}})
			case r.URL.Path == "/v1/threads" && r.Method == http.MethodGet:
				_ = json.NewEncoder(w).Encode(map[string]any{"threads": []map[string]any{
					{"id": threadID, "org_id": orgID, "owner_user_id": "u1", "run_id": threadID, "visibility": "private", "title": threadTitle, "status": "idle", "model_profile_id": "vertex", "model": model, "updated_at": now},
				}})
			case r.URL.Path == "/v1/threads" && r.Method == http.MethodPost:
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": threadID + "_new", "org_id": orgID, "owner_user_id": "u1", "run_id": threadID + "_new", "visibility": "private",
					"title": "Fresh Cloud", "status": "idle", "model_profile_id": "vertex", "model": model, "updated_at": now,
				})
			case r.URL.Path == "/v1/threads/"+threadID && r.Method == http.MethodGet:
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": threadID, "org_id": orgID, "owner_user_id": "u1", "run_id": threadID, "visibility": "private",
					"title": threadTitle, "status": "idle", "pinned": pinnedState, "updated_at": now,
				})
			case r.URL.Path == "/v1/threads/"+threadID+"/events":
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "id: 1\nevent: user_message\ndata: %s\n\n", fmt.Sprintf(`{"thread_id":%q,"seq":1,"type":"user_message","payload":{"text":"ping"}}`, threadID))
				fmt.Fprintf(w, "id: 2\nevent: assistant_message\ndata: %s\n\n", fmt.Sprintf(`{"thread_id":%q,"seq":2,"type":"assistant_message","payload":{"text":"pong"}}`, threadID))
			case r.URL.Path == "/v1/threads/"+threadID+"/user-state" && r.Method == http.MethodPut:
				var body struct {
					Pinned *bool `json:"pinned"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body.Pinned != nil {
					pinnedState = *body.Pinned
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"thread_id": threadID, "pinned": pinnedState})
			case r.URL.Path == "/v1/models":
				_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"profile_id": "vertex", "provider": "vertex"}}})
			case r.URL.Path == "/v1/machines":
				_ = json.NewEncoder(w).Encode(map[string]any{"machines": []any{}})
			case r.URL.Path == "/v1/usage/summary":
				_ = json.NewEncoder(w).Encode(map[string]any{"month_start": "2026-11-01T00:00:00Z", "org_spent_usd": 2.0, "user_spent_usd": 1.0})
			case r.URL.Path == "/v1/usage/events":
				_ = json.NewEncoder(w).Encode(map[string]any{"events": []map[string]any{
					{"id": 1, "org_id": orgID, "user_id": "u1", "thread_id": threadID, "model_profile_id": "vertex", "model": model, "input_tokens": 300, "output_tokens": 50, "created_at": now},
				}})
			case strings.HasPrefix(r.URL.Path, "/v1/instructions/"):
				_ = json.NewEncoder(w).Encode(map[string]any{"body_md": "", "version": ""})
			case r.URL.Path == "/v1/auth/logout":
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
			default:
				http.NotFound(w, r)
			}
		}))
	}

	upstreamA := newCloudUpstream("org_a", "Acme", "ct_a", "Acme Cloud Thread", "gemini-acme")
	defer upstreamA.Close()
	upstreamB := newCloudUpstream("org_b", "Beta", "ct_b", "Beta Cloud Thread", "gemini-beta")
	defer upstreamB.Close()

	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Seed a local-only thread and local usage before connecting any Cloud workspace.
	if err := store.SaveSession(storage.Session{
		ID: "local_1", Slug: "local_1", Title: "Local Only Thread", WorkspaceID: storage.LocalWorkspaceID,
		Runtime: storage.RuntimeACP, Status: storage.StatusIdle, ModelProvider: "openai", Model: "gpt-local",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddUsage("local_1", storage.Usage{InputTokens: 111, OutputTokens: 22, TotalTokens: 133}); err != nil {
		t.Fatal(err)
	}

	cloudSvc := cloudsync.New(store, nil)
	cloudHandler := cloudapi.NewHandler(cloudSvc)
	listHandler := sessionsapi.NewListHandler(store).WithCloud(cloudSvc)
	msgHandler := sessionsapi.NewMessagesHandler(transcript.NewService(store, nil)).WithCloud(cloudSvc)
	usageSvc := usagecore.NewService(store).WithWorkspace(cloudSvc.ActiveWorkspaceID)

	srv := &server.Server{
		Store: store,
		Cloud: cloudSvc,
		Routes: server.Routes{
			{Pattern: "GET /v1/sessions", Handler: listHandler},
			{Pattern: "GET /v1/sessions/{session}/messages", Handler: msgHandler},
			{Pattern: "GET /v1/cloud/status", Handler: http.HandlerFunc(cloudHandler.Status)},
			{Pattern: "POST /v1/cloud/connect", Handler: http.HandlerFunc(cloudHandler.Connect)},
			{Pattern: "POST /v1/cloud/switch", Handler: http.HandlerFunc(cloudHandler.Switch)},
			{Pattern: "POST /v1/cloud/disconnect", Handler: http.HandlerFunc(cloudHandler.Disconnect)},
			{Pattern: "GET /v1/spaces", Handler: http.HandlerFunc(cloudHandler.ListSpaces)},
			{Pattern: "GET /v1/models", Handler: http.HandlerFunc(cloudHandler.ListModels)},
			{Pattern: "GET /v1/usage/models", Handler: usageapi.NewModelsHandler(usageSvc)},
		},
	}
	_ = app.Workspace("")
	h := srv.Handler()

	listIDs := func() []string {
		t.Helper()
		rec := httptest.NewRecorder()
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
		ids := make([]string, 0, len(listed.Sessions))
		for _, s := range listed.Sessions {
			ids = append(ids, s.ID)
		}
		return ids
	}

	usageModels := func() []string {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/usage/models?days=1&timezone=UTC", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("usage models status=%d body=%s", rec.Code, rec.Body.String())
		}
		var body struct {
			Models []struct {
				Model string `json:"model"`
			} `json:"models"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		out := make([]string, 0, len(body.Models))
		for _, m := range body.Models {
			out = append(out, m.Model)
		}
		return out
	}

	// 1. Connect Cloud A via /v1/cloud/connect.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/cloud/connect", bytes.NewBufferString(fmt.Sprintf(`{"url":%q,"token":"tokA"}`, upstreamA.URL))))
	if rec.Code != http.StatusOK {
		t.Fatalf("connect A status=%d body=%s", rec.Code, rec.Body.String())
	}
	var stA cloudsync.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &stA); err != nil {
		t.Fatal(err)
	}
	idA := stA.ActiveWorkspaceID

	// Add a Mac-local ACP thread inside Cloud A.
	if err := store.SaveSession(storage.Session{
		ID: "mac_in_a", Slug: "mac_in_a", Title: "Mac ACP inside A", WorkspaceID: idA,
		Runtime: storage.RuntimeACP, Status: storage.StatusIdle, ModelProvider: "openai", Model: "gpt-mac-a",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddUsage("mac_in_a", storage.Usage{InputTokens: 200, OutputTokens: 30, TotalTokens: 230}); err != nil {
		t.Fatal(err)
	}

	// In Cloud A: ONLY ct_a + mac_in_a appear (never local_1!), and usage shows gemini-acme + gpt-mac-a (never gpt-local!).
	idsA := listIDs()
	if len(idsA) != 2 || strings.Join(idsA, ",") == "local_1" {
		t.Fatalf("Cloud A sessions = %v, want ct_a and mac_in_a only", idsA)
	}
	for _, id := range idsA {
		if id == "local_1" {
			t.Fatalf("local_1 leaked into Cloud A: %v", idsA)
		}
	}
	modelsA := usageModels()
	if len(modelsA) != 2 {
		t.Fatalf("Cloud A usage models = %v, want [gemini-acme, gpt-mac-a]", modelsA)
	}
	for _, m := range modelsA {
		if m == "gpt-local" {
			t.Fatalf("local usage leaked into Cloud A: %v", modelsA)
		}
	}

	// GET /v1/sessions/ct_a/messages projects Cloud A thread events transparently.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/sessions/ct_a/messages", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"content":"pong"`) {
		t.Fatalf("messages status=%d body=%s", rec.Code, rec.Body.String())
	}

	// POST /v1/sessions with model_profile_id creates a Cloud thread in Cloud A.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/sessions", bytes.NewBufferString(`{"title":"Fresh Cloud","model_profile_id":"vertex","model":"gemini-acme"}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":"ct_a_new"`) || !strings.Contains(rec.Body.String(), `"backend":"cloud"`) {
		t.Fatalf("create cloud session status=%d body=%s", rec.Code, rec.Body.String())
	}

	// POST /v1/sessions/ct_a/pin forwards to cloud user-state and updates mirror.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/sessions/ct_a/pin", nil))
	if rec.Code != http.StatusOK || !pinnedState || !strings.Contains(rec.Body.String(), `"pinned":true`) {
		t.Fatalf("pin cloud session status=%d pinned=%v body=%s", rec.Code, pinnedState, rec.Body.String())
	}

	// 2. Connect Cloud B -> strict isolation from both local_1 and Cloud A!
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/cloud/connect", bytes.NewBufferString(fmt.Sprintf(`{"url":%q,"token":"tokB"}`, upstreamB.URL))))
	if rec.Code != http.StatusOK {
		t.Fatalf("connect B status=%d body=%s", rec.Code, rec.Body.String())
	}
	var stB cloudsync.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &stB); err != nil {
		t.Fatal(err)
	}
	idB := stB.ActiveWorkspaceID
	if len(stB.Workspaces) != 2 {
		t.Fatalf("expected 2 workspaces after connecting B, got %+v", stB.Workspaces)
	}
	if got := listIDs(); len(got) != 1 || got[0] != "ct_b" {
		t.Fatalf("Cloud B sessions = %v, want [ct_b]", got)
	}
	if got := usageModels(); len(got) != 1 || got[0] != "gemini-beta" {
		t.Fatalf("Cloud B usage models = %v, want [gemini-beta]", got)
	}

	// 3. POST /v1/cloud/switch -> "local": ONLY local_1 and gpt-local appear!
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/cloud/switch", bytes.NewBufferString(`{"workspace_id":"local"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("switch local status=%d body=%s", rec.Code, rec.Body.String())
	}
	var stLocal cloudsync.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &stLocal); err != nil {
		t.Fatal(err)
	}
	if stLocal.ActiveWorkspaceID != "local" || stLocal.Connected || len(stLocal.Workspaces) != 2 {
		t.Fatalf("unexpected local status: %+v", stLocal)
	}
	if got := listIDs(); len(got) != 1 || got[0] != "local_1" {
		t.Fatalf("local sessions = %v, want [local_1]", got)
	}
	if got := usageModels(); len(got) != 1 || got[0] != "gpt-local" {
		t.Fatalf("local usage models = %v, want [gpt-local]", got)
	}

	// 4. POST /v1/cloud/switch with unknown workspace_id returns 404.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/cloud/switch", bytes.NewBufferString(`{"workspace_id":"missing"}`)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("switch unknown status=%d want 404 body=%s", rec.Code, rec.Body.String())
	}

	// 5. POST /v1/cloud/disconnect with explicit workspace_id disconnects ONLY A, keeping B!
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/cloud/disconnect", bytes.NewBufferString(fmt.Sprintf(`{"workspace_id":%q}`, idA))))
	if rec.Code != http.StatusOK {
		t.Fatalf("disconnect A status=%d body=%s", rec.Code, rec.Body.String())
	}
	var stAfter cloudsync.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &stAfter); err != nil {
		t.Fatal(err)
	}
	if len(stAfter.Workspaces) != 1 || stAfter.Workspaces[0].ID != idB {
		t.Fatalf("expected only Cloud B remaining after disconnecting A, got %+v", stAfter.Workspaces)
	}

	// 6. Switch to B, then disconnect with empty body disconnects active Cloud B -> returns to local.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/cloud/switch", bytes.NewBufferString(fmt.Sprintf(`{"workspace_id":%q}`, idB))))
	if rec.Code != http.StatusOK {
		t.Fatalf("switch B status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/cloud/disconnect", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("disconnect active B status=%d body=%s", rec.Code, rec.Body.String())
	}
	var stEmpty cloudsync.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &stEmpty); err != nil {
		t.Fatal(err)
	}
	if stEmpty.ActiveWorkspaceID != "local" || stEmpty.Connected || len(stEmpty.Workspaces) != 0 {
		t.Fatalf("expected local + 0 workspaces after disconnecting B, got %+v", stEmpty)
	}
}
