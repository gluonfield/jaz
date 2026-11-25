package cloud_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/app"
	"github.com/wins/jaz/backend/internal/cloudsync"
	botsapi "github.com/wins/jaz/backend/internal/httpapi/bots"
	cloudapi "github.com/wins/jaz/backend/internal/httpapi/cloud"
	pluginsapi "github.com/wins/jaz/backend/internal/httpapi/plugins"
	sessionsapi "github.com/wins/jaz/backend/internal/httpapi/sessions"
	usageapi "github.com/wins/jaz/backend/internal/httpapi/usage"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/machinebridge"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	"github.com/wins/jaz/backend/internal/transcript"
	usagecore "github.com/wins/jaz/backend/internal/usage"
	"github.com/wins/jaz/backend/internal/widgets"
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

func TestCloudMembersAndBoardsProxyAndLocalFallback(t *testing.T) {
	var seenQuery string
	var createdVisibility string
	var patchedMembers []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/.well-known/jaz-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mode": "cloud", "auth": "oidc", "capabilities": map[string]bool{"boards": true, "members": true}})
		case r.URL.Path == "/v1/auth/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"user_id": "u_admin", "org_id": "org_1", "org_name": "Acme", "email": "admin@acme.test", "display_name": "Admin", "role": "admin"})
		case r.URL.Path == "/v1/spaces" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"spaces": []map[string]any{
				{"id": "sp_team", "org_id": "org_1", "name": "All Hands", "visibility": "team", "can_edit": true},
			}})
		case r.URL.Path == "/v1/spaces" && r.Method == http.MethodPost:
			var body cloudsync.CreateSpaceRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "sp_2", "org_id": "org_1", "name": body.Name, "visibility": body.Visibility, "can_edit": true})
		case r.URL.Path == "/v1/spaces/sp_2" && r.Method == http.MethodPatch:
			var body struct {
				Visibility    string   `json:"visibility"`
				MemberUserIDs []string `json:"member_user_ids"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "sp_2", "org_id": "org_1", "name": "Core", "visibility": body.Visibility, "can_edit": true})
		case r.URL.Path == "/v1/threads":
			_ = json.NewEncoder(w).Encode(map[string]any{"threads": []any{}})
		case r.URL.Path == "/v1/usage/events":
			_ = json.NewEncoder(w).Encode(map[string]any{"events": []any{}})
		case strings.HasPrefix(r.URL.Path, "/v1/instructions/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"body_md": "", "version": ""})
		case r.URL.Path == "/v1/members" && r.Method == http.MethodGet:
			seenQuery = r.URL.RawQuery
			_ = json.NewEncoder(w).Encode(map[string]any{"members": []map[string]any{
				{"id": "u_bob", "org_id": "org_1", "email": "bob@acme.test", "display_name": "Bob Builder", "role": "member", "status": "invited"},
			}})
		case r.URL.Path == "/v1/members" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "u_carol", "org_id": "org_1", "email": "carol@acme.test", "display_name": "Carol", "role": "member", "status": "invited"})
		case r.URL.Path == "/v1/members/u_carol" && r.Method == http.MethodPatch:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "u_carol", "org_id": "org_1", "email": "carol@acme.test", "display_name": "Carol", "role": "admin", "status": "invited"})
		case r.URL.Path == "/v1/members/u_carol" && r.Method == http.MethodDelete:
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case r.URL.Path == "/v1/boards" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"boards": []map[string]any{
				{"id": "cb_1", "name": "Cloud Team Board", "visibility": "team", "is_owner": true, "can_edit": true},
			}})
		case r.URL.Path == "/v1/boards" && r.Method == http.MethodPost:
			var body struct {
				Name       string `json:"name"`
				Visibility string `json:"visibility"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			createdVisibility = body.Visibility
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "cb_2", "name": body.Name, "visibility": body.Visibility, "is_owner": true, "can_edit": true})
		case r.URL.Path == "/v1/boards/cb_2" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"board": map[string]any{"id": "cb_2", "name": "Ops", "visibility": "selected", "can_edit": true}, "items": []any{}})
		case r.URL.Path == "/v1/boards/cb_2" && r.Method == http.MethodPatch:
			var body struct {
				Visibility    string   `json:"visibility"`
				MemberUserIDs []string `json:"member_user_ids"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			patchedMembers = body.MemberUserIDs
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "cb_2", "name": "Ops", "visibility": body.Visibility, "can_edit": true})
		case r.URL.Path == "/v1/boards/cb_2/widgets/w_1" && r.Method == http.MethodDelete:
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case r.URL.Path == "/v1/boards/cb_2" && r.Method == http.MethodDelete:
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

	cloudSvc := cloudsync.New(store, nil)
	cloudHandler := cloudapi.NewHandler(cloudSvc)
	widgetSvc := widgets.NewService(store, nil).WithWorkspace(cloudSvc.ActiveWorkspaceID)
	srv := &server.Server{
		Store:   store,
		Cloud:   cloudSvc,
		Widgets: widgetSvc,
		Routes: server.Routes{
			{Pattern: "POST /v1/cloud/connect", Handler: http.HandlerFunc(cloudHandler.Connect)},
			{Pattern: "POST /v1/cloud/switch", Handler: http.HandlerFunc(cloudHandler.Switch)},
			{Pattern: "GET /v1/members", Handler: http.HandlerFunc(cloudHandler.ListMembers)},
			{Pattern: "POST /v1/members", Handler: http.HandlerFunc(cloudHandler.CreateMember)},
			{Pattern: "PATCH /v1/members/{id}", Handler: http.HandlerFunc(cloudHandler.UpdateMember)},
			{Pattern: "DELETE /v1/members/{id}", Handler: http.HandlerFunc(cloudHandler.DeleteMember)},
			{Pattern: "GET /v1/spaces", Handler: http.HandlerFunc(cloudHandler.ListSpaces)},
			{Pattern: "POST /v1/spaces", Handler: http.HandlerFunc(cloudHandler.CreateSpace)},
			{Pattern: "PATCH /v1/spaces/{id}", Handler: http.HandlerFunc(cloudHandler.PatchSpace)},
		},
	}
	h := srv.Handler()

	// Create a local board before connecting Cloud.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/boards", bytes.NewBufferString(`{"name":"Local Only Board"}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Local Only Board") {
		t.Fatalf("create local board status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Connect Cloud workspace.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/cloud/connect", bytes.NewBufferString(fmt.Sprintf(`{"url":%q,"token":"tok"}`, upstream.URL))))
	if rec.Code != http.StatusOK {
		t.Fatalf("connect status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Members proxy: search with query escaping, create, update, delete, path traversal rejection.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/members?q=bob+b&limit=10", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "bob@acme.test") || !strings.Contains(seenQuery, "q=bob+b") {
		t.Fatalf("list members status=%d query=%q body=%s", rec.Code, seenQuery, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/members", bytes.NewBufferString(`{"email":"carol@acme.test","display_name":"Carol","role":"member"}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create member status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/v1/members/u_carol", bytes.NewBufferString(`{"role":"admin"}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"role":"admin"`) {
		t.Fatalf("update member status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/v1/members/u_carol", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete member status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, _, err := cloudSvc.DeleteMember(t.Context(), "../evil"); err != cloudsync.ErrInvalidPath {
		t.Fatalf("expected ErrInvalidPath for member traversal, got %v", err)
	}

	// Boards proxy in Cloud workspace: never leaks Local Only Board.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/boards", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Cloud Team Board") || strings.Contains(rec.Body.String(), "Local Only Board") {
		t.Fatalf("cloud list boards status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/boards", bytes.NewBufferString(`{"name":"Ops","visibility":"team"}`)))
	if rec.Code != http.StatusCreated || createdVisibility != "team" {
		t.Fatalf("cloud create board status=%d vis=%q body=%s", rec.Code, createdVisibility, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/boards/cb_2", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":"cb_2"`) {
		t.Fatalf("cloud get board status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/v1/boards/cb_2", bytes.NewBufferString(`{"visibility":"selected","member_user_ids":["u_bob"]}`)))
	if rec.Code != http.StatusOK || len(patchedMembers) != 1 || patchedMembers[0] != "u_bob" {
		t.Fatalf("cloud patch board status=%d members=%v body=%s", rec.Code, patchedMembers, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/v1/boards/cb_2/widgets/w_1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("cloud delete board widget status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/v1/boards/cb_2", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("cloud delete board status=%d body=%s", rec.Code, rec.Body.String())
	}
	badBoardReq := httptest.NewRequest(http.MethodDelete, "/v1/boards/cb_2/widgets/escape", nil)
	badBoardReq.URL.Path = "/v1/boards/cb_2/widgets/../escape"
	rec = httptest.NewRecorder()
	if !cloudSvc.ProxyActiveWorkspace(rec, badBoardReq) || rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for board widget traversal, got %d %s", rec.Code, rec.Body.String())
	}

	// Spaces proxy: live visibility list, create with visibility + members, patch visibility, traversal guard.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/spaces", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"visibility":"team"`) {
		t.Fatalf("cloud list spaces status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/spaces", bytes.NewBufferString(`{"name":"Core","visibility":"selected","member_user_ids":["u_bob"]}`)))
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"visibility":"selected"`) {
		t.Fatalf("cloud create space status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/v1/spaces/sp_2", bytes.NewBufferString(`{"visibility":"team","member_user_ids":[]}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"visibility":"team"`) {
		t.Fatalf("cloud patch space status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, _, err := cloudSvc.PatchSpace(t.Context(), "../evil", json.RawMessage(`{}`)); err != cloudsync.ErrInvalidPath {
		t.Fatalf("expected ErrInvalidPath for space traversal, got %v", err)
	}

	// Switch back to Local -> /v1/boards serves local SQLite boards cleanly again.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/cloud/switch", bytes.NewBufferString(`{"workspace_id":"local"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("switch local status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/boards", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Local Only Board") || strings.Contains(rec.Body.String(), "Cloud Team Board") {
		t.Fatalf("local list boards status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCloudHighPriorityProxiesAndLocalFallback(t *testing.T) {
	var lastSSEQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/.well-known/jaz-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mode": "cloud", "auth": "oidc"})
		case r.URL.Path == "/v1/auth/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"user_id": "u1", "org_id": "org_1", "org_name": "Acme", "email": "u@acme.test", "role": "admin"})
		case r.URL.Path == "/v1/spaces":
			_ = json.NewEncoder(w).Encode(map[string]any{"spaces": []any{}})
		case r.URL.Path == "/v1/threads":
			_ = json.NewEncoder(w).Encode(map[string]any{"threads": []any{}})
		case r.URL.Path == "/v1/usage/events":
			_ = json.NewEncoder(w).Encode(map[string]any{"events": []any{}})
		case strings.HasPrefix(r.URL.Path, "/v1/instructions/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"body_md": "", "version": ""})
		case r.URL.Path == "/v1/bots" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"bots": []map[string]any{{"id": "bot_cloud_1", "name": "Cloud Research"}}})
		case r.URL.Path == "/v1/bots/events":
			lastSSEQuery = r.URL.RawQuery
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "event: snapshot\ndata: {\"bots\":[{\"id\":\"bot_cloud_1\"}]}\n\n")
		case r.URL.Path == "/v1/loops" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"loops": []map[string]any{{"id": "loop_c1", "name": "Cloud KPI"}}})
		case r.URL.Path == "/v1/boards/b_1/loops" && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case r.URL.Path == "/v1/widgets/w_c1/content" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<div>Cloud Live Widget</div>"))
		case r.URL.Path == "/v1/plugins" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"plugins": []map[string]any{{"id": "sales-kit", "display_name": "Sales Kit"}}, "sources": []any{}})
		case r.URL.Path == "/v1/skills" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"skills": []map[string]any{{"name": "deal-review", "description": "Cloud skill", "path": "cloud://plugins/sales-kit/SKILL.md"}}})
		case r.URL.Path == "/v1/mcp/servers" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"servers": []map[string]any{{"id": "mcp_c1", "name": "Cloud CRM"}}, "oauth_redirect_url": "https://cloud.example/v1/connectors/oauth/callback"})
		case r.URL.Path == "/v1/connectors" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"connectors": []map[string]any{{"id": "conn_1", "server_id": "mcp_c1", "alias": "default"}}})
		case r.URL.Path == "/v1/user-skills" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"skills": []map[string]any{{"skill_name": "personal-check"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cloudSvc := cloudsync.New(store, nil)
	cloudHandler := cloudapi.NewHandler(cloudSvc)
	botsHandler := botsapi.NewHandler(nil, nil).WithCloud(cloudSvc)
	pluginsHandler := pluginsapi.NewHandler(nil, nil, nil, "").WithCloud(cloudSvc)
	loopSvc := loops.NewService(store, nil, nil, loops.WithWorkspace(cloudSvc.ActiveWorkspaceID))
	widgetSvc := widgets.NewService(store, nil).WithWorkspace(cloudSvc.ActiveWorkspaceID)

	srv := &server.Server{
		Root:    root,
		Store:   store,
		Cloud:   cloudSvc,
		Loops:   loopSvc,
		Widgets: widgetSvc,
		Routes: server.Routes{
			{Pattern: "POST /v1/cloud/connect", Handler: http.HandlerFunc(cloudHandler.Connect)},
			{Pattern: "POST /v1/cloud/switch", Handler: http.HandlerFunc(cloudHandler.Switch)},
			{Pattern: "GET /v1/bots", Handler: http.HandlerFunc(botsHandler.List)},
			{Pattern: "GET /v1/bots/events", Handler: http.HandlerFunc(botsHandler.Events)},
			{Pattern: "GET /v1/plugins", Handler: http.HandlerFunc(pluginsHandler.List)},
			{Pattern: "GET /v1/connectors", Handler: http.HandlerFunc(cloudHandler.Proxy)},
			{Pattern: "GET /v1/user-skills", Handler: http.HandlerFunc(cloudHandler.Proxy)},
		},
	}
	h := srv.Handler()

	// Connect Cloud workspace.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/cloud/connect", bytes.NewBufferString(fmt.Sprintf(`{"url":%q,"token":"tok"}`, upstream.URL))))
	if rec.Code != http.StatusOK {
		t.Fatalf("connect status=%d body=%s", rec.Code, rec.Body.String())
	}

	// Verify every high-priority Cloud proxy endpoint forwards cleanly.
	for _, tc := range []struct {
		method string
		path   string
		body   string
		want   string
	}{
		{http.MethodGet, "/v1/bots", "", "bot_cloud_1"},
		{http.MethodGet, "/v1/bots/events?once=1&ticket=local-secret", "", "bot_cloud_1"},
		{http.MethodGet, "/v1/loops", "", "loop_c1"},
		{http.MethodPost, "/v1/boards/b_1/loops", `{"loop_ids":["loop_c1"]}`, `"ok":true`},
		{http.MethodGet, "/v1/plugins", "", "Sales Kit"},
		{http.MethodGet, "/v1/skills", "", "deal-review"},
		{http.MethodGet, "/v1/mcp/servers", "", "Cloud CRM"},
		{http.MethodGet, "/v1/connectors", "", "conn_1"},
		{http.MethodGet, "/v1/user-skills", "", "personal-check"},
	} {
		rec := httptest.NewRecorder()
		var bodyReader *bytes.Buffer
		if tc.body != "" {
			bodyReader = bytes.NewBufferString(tc.body)
		} else {
			bodyReader = bytes.NewBuffer(nil)
		}
		h.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, bodyReader))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatalf("%s %s expected 200 containing %q, got %d %s", tc.method, tc.path, tc.want, rec.Code, rec.Body.String())
		}
	}

	if strings.Contains(lastSSEQuery, "ticket") || lastSSEQuery != "once=1" {
		t.Fatalf("expected proxy to strip local ticket query param, upstream got %q", lastSSEQuery)
	}

	// Widget HTML content proxy MUST enforce sandbox CSP & XSS defense headers.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/widgets/w_c1/content", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Cloud Live Widget") ||
		rec.Header().Get("Content-Security-Policy") != "default-src 'none'; sandbox" ||
		rec.Header().Get("X-Content-Type-Options") != "nosniff" ||
		rec.Header().Get("X-Frame-Options") != "DENY" ||
		rec.Header().Get("Referrer-Policy") != "no-referrer" ||
		rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("widget content proxy failed security headers: code=%d hdr=%+v body=%s", rec.Code, rec.Header(), rec.Body.String())
	}

	// Proxy path traversal rejection (400).
	badReq := httptest.NewRequest(http.MethodGet, "/v1/widgets/..%2Fsecret/content", nil)
	badReq.URL.Path = "/v1/widgets/../secret/content"
	rec = httptest.NewRecorder()
	if !cloudSvc.ProxyActiveWorkspace(rec, badReq) || rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 rejecting traversal path, got %d %s", rec.Code, rec.Body.String())
	}

	// Switch back to Local -> Cloud-only endpoints return 401 and local /v1/loops serves SQLite cleanly.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/cloud/switch", bytes.NewBufferString(`{"workspace_id":"local"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("switch local status=%d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/connectors", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on /v1/connectors in local mode, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/loops", nil))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "loop_c1") {
		t.Fatalf("local /v1/loops leaked cloud loop: %d %s", rec.Code, rec.Body.String())
	}
}

func TestExpiredCloudSessionIsNotRelayedAsLocalAuthRejection(t *testing.T) {
	var expired atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case expired.Load():
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprint(w, `{"error":"invalid session token","code":"unauthorized"}`)
		case r.URL.Path == "/v1/auth/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"user_id": "u1", "org_id": "org_1"})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}
	}))
	defer upstream.Close()

	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cloudSvc := cloudsync.New(store, nil)
	cloudHandler := cloudapi.NewHandler(cloudSvc)
	botsHandler := botsapi.NewHandler(nil, nil).WithCloud(cloudSvc)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/cloud/connect", cloudHandler.Connect)
	mux.HandleFunc("GET /v1/cloud/members", cloudHandler.ListMembers)
	mux.HandleFunc("GET /v1/connectors", cloudHandler.Proxy)
	mux.HandleFunc("GET /v1/bots/events", botsHandler.Events)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/cloud/connect", bytes.NewBufferString(fmt.Sprintf(`{"url":%q,"token":"tok"}`, upstream.URL))))
	if rec.Code != http.StatusOK {
		t.Fatalf("connect status=%d body=%s", rec.Code, rec.Body.String())
	}
	expired.Store(true)

	for _, path := range []string{"/v1/cloud/members", "/v1/connectors", "/v1/bots/events"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusUnauthorized || strings.Contains(rec.Body.String(), `"code"`) {
			t.Fatalf("%s: want an uncoded 401, got %d %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestCloudMachineBridgeEndpoints(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cloudSvc := cloudsync.New(store, nil)

	// 1. Nil bridge controller returns 501 Not Implemented.
	disabledHandler := cloudapi.NewHandler(cloudSvc)
	disabledSrv := &server.Server{
		Store: store,
		Cloud: cloudSvc,
		Routes: server.Routes{
			{Pattern: "GET /v1/cloud/machine-bridge", Handler: http.HandlerFunc(disabledHandler.MachineBridgeStatus)},
			{Pattern: "PUT /v1/cloud/machine-bridge", Handler: http.HandlerFunc(disabledHandler.UpdateMachineBridge)},
		},
	}
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		rec := httptest.NewRecorder()
		disabledSrv.Handler().ServeHTTP(rec, httptest.NewRequest(method, "/v1/cloud/machine-bridge", bytes.NewBufferString(`{}`)))
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("%s with nil bridge status=%d want 501 body=%s", method, rec.Code, rec.Body.String())
		}
	}

	// 2. Live bridge controller serves GET/PUT and validates payloads strictly.
	workspace := t.TempDir()
	sharedDir := filepath.Join(t.TempDir(), "Work Repo")
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bridge := app.NewMachineBridgeSupervisor(cloudSvc, store, workspace, nil)
	cloudHandler := cloudapi.NewHandler(cloudSvc).WithMachineBridge(bridge)
	srv := &server.Server{
		Store: store,
		Cloud: cloudSvc,
		Routes: server.Routes{
			{Pattern: "GET /v1/cloud/machine-bridge", Handler: http.HandlerFunc(cloudHandler.MachineBridgeStatus)},
			{Pattern: "PUT /v1/cloud/machine-bridge", Handler: http.HandlerFunc(cloudHandler.UpdateMachineBridge)},
		},
	}
	h := srv.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/cloud/machine-bridge", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET machine-bridge status=%d body=%s", rec.Code, rec.Body.String())
	}
	var initial machinebridge.BridgeStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &initial); err != nil {
		t.Fatal(err)
	}
	if initial.DeviceID == "" || !initial.Config.Enabled || initial.Config.AllowExec || len(initial.Config.Grants) != 1 {
		t.Fatalf("unexpected initial GET status: %+v", initial)
	}

	payload, err := json.Marshal(machinebridge.ShareConfig{
		Enabled:     true,
		DisplayName: "Office Mac",
		AllowExec:   true,
		Grants: []machinebridge.FolderGrant{
			{RootPath: sharedDir, Mode: machinebridge.AccessReadWrite},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/v1/cloud/machine-bridge", bytes.NewReader(payload)))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT machine-bridge status=%d body=%s", rec.Code, rec.Body.String())
	}
	var saved machinebridge.BridgeStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.DeviceID != initial.DeviceID || saved.Config.DisplayName != "Office Mac" || !saved.Config.AllowExec || len(saved.Config.Grants) != 1 || saved.Config.Grants[0].ID != "work-repo" {
		t.Fatalf("unexpected saved PUT status: %+v", saved)
	}

	// Invalid payloads return 400.
	badBodies := []string{
		`{not-json`,
		`{"enabled":true}{"extra":1}`,
		fmt.Sprintf(`{"enabled":true,"grants":[{"id":"x","root_path":%q,"mode":"ro"}]}`, filepath.Join(workspace, "missing")),
		fmt.Sprintf(`{"enabled":true,"grants":[{"id":"x","root_path":%q,"mode":"admin"}]}`, sharedDir),
		fmt.Sprintf(`{"enabled":true,"grants":[{"id":"bad=id","root_path":%q,"mode":"ro"}]}`, sharedDir),
		`{"enabled":true,"display_name":"Bad\nName","grants":[]}`,
	}
	for _, body := range badBodies {
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/v1/cloud/machine-bridge", bytes.NewBufferString(body)))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("PUT invalid body %s status=%d want 400 resp=%s", body, rec.Code, rec.Body.String())
		}
	}
}

func TestProxyRunFileDownloadAndAuthSanitization(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/.well-known/jaz-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mode": "cloud", "auth": "oidc", "oidc_login_url": "/v1/auth/login"})
		case r.URL.Path == "/v1/auth/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"user_id": "u1", "org_id": "org_a", "org_name": "Acme", "email": "dev@acme.test", "display_name": "Dev", "role": "member"})
		case r.URL.Path == "/v1/runs/run_1/files/docs/report.md":
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			w.Header().Set("ETag", `"etag-1"`)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("# Report"))
		case r.URL.Path == "/v1/runs/run_1/files/expired.txt":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"expired jwt","code":"unauthorized"}`))
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

	cloudSvc := cloudsync.New(store, nil)
	if _, err := cloudSvc.Connect(t.Context(), cloudsync.ConnectRequest{URL: upstream.URL, Token: "cloud-jwt"}); err != nil {
		t.Fatal(err)
	}
	h := cloudapi.NewHandler(cloudSvc)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/runs/{run_id}/files/{path...}", h.ProxyRunFile)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/runs/run_1/files/docs/report.md?download=1", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "# Report" {
		t.Fatalf("download run file status=%d body=%q", rec.Code, rec.Body.String())
	}
	if disp := rec.Header().Get("Content-Disposition"); !strings.Contains(disp, "attachment") || !strings.Contains(disp, "report.md") {
		t.Fatalf("Content-Disposition = %q, want attachment filename=report.md", disp)
	}
	if got := rec.Header().Get("ETag"); got != `"etag-1"` {
		t.Fatalf("ETag = %q, want %q", got, `"etag-1"`)
	}

	unauthRec := httptest.NewRecorder()
	mux.ServeHTTP(unauthRec, httptest.NewRequest(http.MethodGet, "/v1/runs/run_1/files/expired.txt", nil))
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expired upstream status=%d want 401", unauthRec.Code)
	}
	if strings.Contains(unauthRec.Body.String(), `"code":"unauthorized"`) {
		t.Fatalf("upstream 401 leaked local unauthorized code: %s", unauthRec.Body.String())
	}

	missingRec := httptest.NewRecorder()
	mux.ServeHTTP(missingRec, httptest.NewRequest(http.MethodGet, "/v1/runs/run_1/files/missing.md?download=1", nil))
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("missing file status=%d want 404", missingRec.Code)
	}
	if disp := missingRec.Header().Get("Content-Disposition"); disp != "" {
		t.Fatalf("404 response should not set Content-Disposition, got %q", disp)
	}
}
