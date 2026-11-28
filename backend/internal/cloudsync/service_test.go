package cloudsync_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/cloudsync"
	"github.com/wins/jaz/backend/internal/coordinator"
	"github.com/wins/jaz/backend/internal/instructions"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestCloudSyncConnectMirrorStreamAndInstructions(t *testing.T) {
	ctx := context.Background()
	var (
		mu        sync.Mutex
		loggedOut bool
		soulRead  bool
		instBody  = map[string]string{
			"org/org_1/agents":      "Org standing rule: prefer Go.",
			"user/usr_1/agents":     "User rule: be concise.",
			"user/usr_1/long_term":  "Long term context from DB.",
			"user/usr_1/short_term": "Short term sprint from DB.",
			"space/sp_1/agents":     "Space rule: review PRs.",
		}
		events = []map[string]any{
			{"thread_id": "ct_1", "seq": 1, "org_id": "org_1", "actor_user_id": "usr_1", "type": "user_message", "payload": map[string]any{"text": "hello cloud"}, "created_at": time.Now().UTC().Format(time.RFC3339)},
			{"thread_id": "ct_1", "seq": 2, "org_id": "org_1", "type": "assistant_delta", "payload": map[string]any{"text": "hi "}, "created_at": time.Now().UTC().Format(time.RFC3339)},
			{"thread_id": "ct_1", "seq": 3, "org_id": "org_1", "type": "assistant_delta", "payload": map[string]any{"text": "there"}, "created_at": time.Now().UTC().Format(time.RFC3339)},
			{"thread_id": "ct_1", "seq": 4, "org_id": "org_1", "type": "turn_completed", "payload": map[string]any{}, "created_at": time.Now().UTC().Format(time.RFC3339)},
		}
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/.well-known/jaz-server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true, "mode": "cloud", "auth": "oidc", "oidc_login_url": "/v1/auth/login",
				"capabilities": map[string]bool{"spaces": true, "cloud_instructions": true},
			})
		case r.URL.Path == "/v1/auth/exchange":
			var body struct {
				Code         string `json:"code"`
				CodeVerifier string `json:"code_verifier"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Code != "oidc-code-1" || body.CodeVerifier != "verifier-1" {
				http.Error(w, `{"error":"bad code"}`, http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token": "cloud-jwt-1",
				"user":  map[string]any{"user_id": "usr_1", "org_id": "org_1", "email": "u@example.com", "display_name": "Dev", "role": "admin"},
			})
		case r.URL.Path == "/v1/auth/me":
			if r.Header.Get("Authorization") != "Bearer cloud-jwt-1" {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user_id": "usr_1", "org_id": "org_1", "org_name": "Example Systems", "email": "u@example.com", "display_name": "Dev", "role": "admin",
			})
		case r.URL.Path == "/v1/auth/logout":
			mu.Lock()
			loggedOut = true
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case r.URL.Path == "/v1/spaces" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"spaces": []map[string]any{{"id": "sp_1", "org_id": "org_1", "name": "Core"}},
			})
		case r.URL.Path == "/v1/threads" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"threads": []map[string]any{
					{
						"id": "ct_1", "org_id": "org_1", "owner_user_id": "usr_1", "space_id": "sp_1",
						"run_id": "ct_1", "visibility": "space", "title": "Cloud Review", "status": "idle",
						"model_profile_id": "vertex", "model": "google/gemini-4-neon",
						"updated_at": time.Now().UTC().Format(time.RFC3339),
					},
				},
			})
		case r.URL.Path == "/v1/threads/ct_1" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "ct_1", "org_id": "org_1", "owner_user_id": "usr_1", "space_id": "sp_1",
				"run_id": "ct_1", "visibility": "space", "title": "Cloud Review", "status": "idle",
				"model_profile_id": "vertex", "model": "google/gemini-4-neon",
				"updated_at": time.Now().UTC().Format(time.RFC3339),
			})
		case r.URL.Path == "/v1/threads/ct_1/events":
			w.Header().Set("Content-Type", "text/event-stream")
			for _, ev := range events {
				raw, _ := json.Marshal(ev)
				fmt.Fprintf(w, "id: %v\nevent: %v\ndata: %s\n\n", ev["seq"], ev["type"], raw)
			}
		case strings.HasPrefix(r.URL.Path, "/v1/instructions/"):
			key := strings.TrimPrefix(r.URL.Path, "/v1/instructions/")
			parts := strings.Split(key, "/")
			mu.Lock()
			if len(parts) == 3 && parts[2] == "soul" {
				soulRead = true
			}
			body := instBody[key]
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"org_id": "org_1", "scope": parts[0], "scope_id": parts[1], "kind": parts[2],
				"body_md": body, "version": instructions.Digest([]byte(body)),
			})
		case r.URL.Path == "/v1/usage/events":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"events": []map[string]any{
					{
						"id": 7, "org_id": "org_1", "user_id": "usr_1", "thread_id": "ct_1",
						"model_profile_id": "vertex", "model": "google/gemini-4-neon",
						"input_tokens": 200, "cached_input_tokens": 50, "output_tokens": 40,
						"cost_usd": 0.005, "created_at": time.Now().UTC().Format(time.RFC3339),
					},
				},
			})
		case r.URL.Path == "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"profile_id": "vertex", "provider": "vertex"}}})
		case r.URL.Path == "/v1/machines":
			_ = json.NewEncoder(w).Encode(map[string]any{"machines": []any{}})
		case r.URL.Path == "/v1/usage/summary":
			_ = json.NewEncoder(w).Encode(map[string]any{"month_start": "2026-11-01T00:00:00Z", "org_spent_usd": 1.25, "user_spent_usd": 0.50})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	defer store.Close()

	svc := cloudsync.New(store, nil)
	initial := svc.Status(ctx)
	if initial.ActiveWorkspaceID != cloudsync.LocalWorkspaceID || initial.Connected || initial.Workspaces == nil || len(initial.Workspaces) != 0 {
		t.Fatalf("unexpected initial status: %+v", initial)
	}

	st, err := svc.Connect(ctx, cloudsync.ConnectRequest{
		URL:          srv.URL,
		Code:         "oidc-code-1",
		CodeVerifier: "verifier-1",
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !st.Connected || st.ActiveWorkspaceID == cloudsync.LocalWorkspaceID || st.User == nil || st.User.OrgName != "Example Systems" || len(st.Workspaces) != 1 {
		t.Fatalf("unexpected connect status: %+v", st)
	}
	mu.Lock()
	if soulRead {
		mu.Unlock()
		t.Fatalf("syncInstructions must not fetch deleted soul kind")
	}
	mu.Unlock()

	// Verify mirrored sessions & workspace-scoped usage in SQLite.
	sessions, err := svc.ListSessions(ctx, false)
	if err != nil || len(sessions) != 1 || sessions[0].ID != "ct_1" || sessions[0].Backend != "cloud" || sessions[0].WorkspaceID != st.ActiveWorkspaceID {
		t.Fatalf("ListSessions: %+v err=%v", sessions, err)
	}
	usageEvents, err := store.UsageEvents(time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil || len(usageEvents) != 1 || usageEvents[0].Model != "google/gemini-4-neon" {
		t.Fatalf("workspace UsageEvents: %+v err=%v", usageEvents, err)
	}

	// Verify transcript projection pulls events and stores them in SQLite.
	transcript, found, err := svc.LoadTranscript(ctx, "ct_1")
	if err != nil || !found {
		t.Fatalf("LoadTranscript: found=%v err=%v", found, err)
	}
	if len(transcript.Messages) != 2 || transcript.Messages[0].Content != "hello cloud" || transcript.Messages[1].Content != "hi there" {
		t.Fatalf("unexpected projected messages: %+v", transcript.Messages)
	}

	// Verify DB-stored instructions inject into local coordinator prompts when signed into Cloud (no SOUL!).
	instStore := cloudsync.NewInstructionsStore(svc, instructions.NewDBStore(store, root, instructions.NewLocalFSStore(root)))
	builder := coordinator.NewBuilder(root, root, root, func() bool { return true }).
		WithInstructions(instStore).
		WithCloudScopeProvider(svc.CloudScope)
	prompt, err := builder.ACPPrompt(root)
	if err != nil {
		t.Fatalf("ACPPrompt: %v", err)
	}
	for _, want := range []string{
		"Org standing rule: prefer Go.",
		"User rule: be concise.",
		"Long term context from DB.",
		"Short term sprint from DB.",
		"Standing instructions are stored in Jaz Cloud",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q; prompt:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "SOUL") {
		t.Fatalf("prompt must not mention deleted SOUL:\n%s", prompt)
	}

	// Verify offline read-only cache still serves transcript after upstream shuts down.
	srv.Close()
	offlineTranscript, found, err := svc.LoadTranscript(ctx, "ct_1")
	if err != nil || !found || len(offlineTranscript.Messages) != 2 {
		t.Fatalf("offline LoadTranscript failed: found=%v msgs=%d err=%v", found, len(offlineTranscript.Messages), err)
	}

	// Verify StreamProjector accumulates deltas properly.
	proj := cloudsync.NewStreamProjector("ct_1")
	ev1 := proj.Convert(storage.CloudThreadEvent{ThreadID: "ct_1", Seq: 10, Type: "assistant_delta", PayloadJSON: json.RawMessage(`{"text":"hel"}`)})
	ev2 := proj.Convert(storage.CloudThreadEvent{ThreadID: "ct_1", Seq: 11, Type: "assistant_delta", PayloadJSON: json.RawMessage(`{"text":"lo"}`)})
	if ev1.ACP == nil || ev1.ACP.Assistant != "hel" || ev2.ACP == nil || ev2.ACP.Assistant != "hello" {
		t.Fatalf("StreamProjector delta accumulation failed: ev1=%+v ev2=%+v", ev1, ev2)
	}
	evDone := proj.Convert(storage.CloudThreadEvent{ThreadID: "ct_1", Seq: 12, Type: "turn_completed", PayloadJSON: json.RawMessage(`{}`)})
	if evDone.ACP == nil || evDone.ACP.State != "idle" || evDone.Type != "acp" {
		t.Fatalf("StreamProjector terminal failed: %+v", evDone)
	}
	_ = sessionevents.Event{}

	// Verify Disconnect clears mirror and falls back to local prompt mode.
	if err := svc.Disconnect(ctx, ""); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if after := svc.Status(ctx); after.Connected || after.ActiveWorkspaceID != cloudsync.LocalWorkspaceID || len(after.Workspaces) != 0 {
		t.Fatalf("expected disconnected local status after Disconnect, got %+v", after)
	}
	if svc.CloudScope(ctx) != nil {
		t.Fatalf("expected CloudScope nil after Disconnect")
	}
	_ = loggedOut
}

func TestCloudSyncMultiWorkspaceSwitchAndIsolation(t *testing.T) {
	ctx := context.Background()
	newMock := func(orgID, orgName, userID, threadID, threadTitle, orgRule, model string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/.well-known/jaz-server":
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mode": "cloud", "auth": "oidc"})
			case r.URL.Path == "/v1/auth/me":
				_ = json.NewEncoder(w).Encode(map[string]any{"user_id": userID, "org_id": orgID, "org_name": orgName, "email": userID + "@example.com", "display_name": orgName + " User", "role": "admin"})
			case r.URL.Path == "/v1/spaces":
				_ = json.NewEncoder(w).Encode(map[string]any{"spaces": []map[string]any{{"id": "sp_" + orgID, "org_id": orgID, "name": orgName + " Space"}}})
			case r.URL.Path == "/v1/threads":
				_ = json.NewEncoder(w).Encode(map[string]any{"threads": []map[string]any{{
					"id": threadID, "org_id": orgID, "owner_user_id": userID, "run_id": threadID,
					"visibility": "private", "title": threadTitle, "status": "idle",
					"model_profile_id": "vertex", "model": model,
					"updated_at": time.Now().UTC().Format(time.RFC3339),
				}}})
			case strings.HasPrefix(r.URL.Path, "/v1/instructions/"):
				parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/instructions/"), "/")
				body := ""
				if parts[0] == "org" && parts[2] == "agents" {
					body = orgRule
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"org_id": orgID, "scope": parts[0], "scope_id": parts[1], "kind": parts[2], "body_md": body, "version": instructions.Digest([]byte(body))})
			case r.URL.Path == "/v1/usage/events":
				_ = json.NewEncoder(w).Encode(map[string]any{"events": []map[string]any{{
					"id": 1, "org_id": orgID, "user_id": userID, "thread_id": threadID,
					"model_profile_id": "vertex", "model": model,
					"input_tokens": 100, "output_tokens": 25, "created_at": time.Now().UTC().Format(time.RFC3339),
				}}})
			case r.URL.Path == "/v1/auth/logout":
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
			default:
				_ = json.NewEncoder(w).Encode(map[string]any{})
			}
		}))
	}

	srvA := newMock("org-a", "Acme", "u-a", "th-a", "Acme Thread", "Acme Rule Only", "model-a")
	defer srvA.Close()
	srvB := newMock("org-b", "Beta", "u-b", "th-b", "Beta Thread", "Beta Rule Only", "model-b")
	defer srvB.Close()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("Legacy Local AGENTS Rule"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	svc := cloudsync.New(store, nil)
	instStore := cloudsync.NewInstructionsStore(svc, instructions.NewDBStore(store, root, instructions.NewLocalFSStore(root)))

	// Connect Cloud A.
	stA, err := svc.Connect(ctx, cloudsync.ConnectRequest{URL: srvA.URL, Token: "tok-a"})
	if err != nil {
		t.Fatalf("Connect A: %v", err)
	}
	idA := stA.ActiveWorkspaceID
	if !stA.Connected || len(stA.Workspaces) != 1 || stA.Workspaces[0].ID != idA {
		t.Fatalf("unexpected status A: %+v", stA)
	}

	// Switch to local: Connected=false, Workspaces still lists Cloud A, instructions fall back to DB-only local AGENTS (one-time import from AGENTS.md).
	stLocal, err := svc.SwitchWorkspace(ctx, cloudsync.LocalWorkspaceID)
	if err != nil {
		t.Fatalf("SwitchWorkspace local: %v", err)
	}
	if stLocal.ActiveWorkspaceID != cloudsync.LocalWorkspaceID || stLocal.Connected || stLocal.Status != "disconnected" || len(stLocal.Workspaces) != 1 {
		t.Fatalf("unexpected local status: %+v", stLocal)
	}
	if sessions, err := svc.ListSessions(ctx, false); err != nil || len(sessions) != 0 {
		t.Fatalf("local workspace must not return Cloud threads: %+v err=%v", sessions, err)
	}
	if _, err := svc.SyncNow(ctx); !errors.Is(err, cloudsync.ErrNotConnected) {
		t.Fatalf("SyncNow in local workspace = %v, want ErrNotConnected", err)
	}
	localDoc, err := instStore.Get(ctx, instructions.Key{Scope: instructions.ScopeUser, Kind: instructions.KindAgents})
	if err != nil || localDoc.Content != "Legacy Local AGENTS Rule" {
		t.Fatalf("expected one-time DB import of local AGENTS, got %+v err=%v", localDoc, err)
	}

	// Connect Cloud B: both A and B appear in Workspaces, active is B.
	stB, err := svc.Connect(ctx, cloudsync.ConnectRequest{URL: srvB.URL, Token: "tok-b"})
	if err != nil {
		t.Fatalf("Connect B: %v", err)
	}
	idB := stB.ActiveWorkspaceID
	if !stB.Connected || idB == idA || len(stB.Workspaces) != 2 {
		t.Fatalf("unexpected status B: %+v", stB)
	}
	sessionsB, err := svc.ListSessions(ctx, false)
	if err != nil || len(sessionsB) != 1 || sessionsB[0].ID != "th-b" {
		t.Fatalf("Cloud B sessions = %+v err=%v", sessionsB, err)
	}
	docB, err := instStore.Get(ctx, instructions.Key{Scope: instructions.ScopeOrg, Kind: instructions.KindAgents})
	if err != nil || docB.Content != "Beta Rule Only" {
		t.Fatalf("Cloud B org AGENTS = %+v err=%v", docB, err)
	}

	// Switch unknown workspace returns ErrNotFound.
	if _, err := svc.SwitchWorkspace(ctx, "does-not-exist"); !errors.Is(err, cloudsync.ErrNotFound) {
		t.Fatalf("SwitchWorkspace unknown = %v, want ErrNotFound", err)
	}

	// Switch back to Cloud A (cloudA -> local -> cloudB -> cloudA).
	stBackA, err := svc.SwitchWorkspace(ctx, idA)
	if err != nil {
		t.Fatalf("SwitchWorkspace back to A: %v", err)
	}
	if !stBackA.Connected || stBackA.ActiveWorkspaceID != idA || len(stBackA.Workspaces) != 2 {
		t.Fatalf("unexpected status back to A: %+v", stBackA)
	}
	sessionsA, err := svc.ListSessions(ctx, false)
	if err != nil || len(sessionsA) != 1 || sessionsA[0].ID != "th-a" {
		t.Fatalf("Cloud A sessions = %+v err=%v", sessionsA, err)
	}
	docA, err := instStore.Get(ctx, instructions.Key{Scope: instructions.ScopeOrg, Kind: instructions.KindAgents})
	if err != nil || docA.Content != "Acme Rule Only" {
		t.Fatalf("Cloud A org AGENTS = %+v err=%v", docA, err)
	}

	// Disconnect Cloud A deletes ONLY Cloud A, switches active to local, and preserves Cloud B intact!
	if err := svc.Disconnect(ctx, idA); err != nil {
		t.Fatalf("Disconnect A: %v", err)
	}
	stAfterDisc := svc.Status(ctx)
	if stAfterDisc.ActiveWorkspaceID != cloudsync.LocalWorkspaceID || stAfterDisc.Connected || len(stAfterDisc.Workspaces) != 1 || stAfterDisc.Workspaces[0].ID != idB {
		t.Fatalf("unexpected status after disconnecting A: %+v", stAfterDisc)
	}
	if _, err := svc.SwitchWorkspace(ctx, idB); err != nil {
		t.Fatalf("SwitchWorkspace to preserved B: %v", err)
	}
	preservedB, err := svc.ListSessions(ctx, false)
	if err != nil || len(preservedB) != 1 || preservedB[0].ID != "th-b" {
		t.Fatalf("Cloud B sessions must survive Cloud A disconnect: %+v err=%v", preservedB, err)
	}
}

func TestCloudSyncDisconnectCancelsInFlightSyncWithoutResurrection(t *testing.T) {
	ctx := context.Background()
	started := make(chan struct{})
	release := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/jaz-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mode": "cloud", "auth": "oidc"})
		case "/v1/auth/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"user_id": "u1", "org_id": "o1", "org_name": "Example Systems", "email": "u@example.com", "display_name": "Dev", "role": "admin"})
		case "/v1/spaces":
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"spaces": []any{}})
		case "/v1/threads":
			select {
			case started <- struct{}{}:
			default:
			}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"threads": []any{}})
		case "/v1/auth/logout":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}
	}))
	defer srv.Close()

	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	accID := "o1:u1@" + srv.URL
	if err := store.SaveCloudAccount(ctx, storage.CloudAccount{
		ID: accID, BaseURL: srv.URL, Token: "tok", UserID: "u1", OrgID: "o1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyCloudWorkspaceChange(ctx, storage.CloudWorkspaceChange{ActiveID: accID}); err != nil {
		t.Fatal(err)
	}
	svc := cloudsync.New(store, nil)

	syncDone := make(chan struct{})
	go func() {
		_, _ = svc.SyncNow(ctx)
		close(syncDone)
	}()

	<-started
	if err := svc.Disconnect(ctx, ""); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	close(release)
	<-syncDone

	if svc.Status(ctx).Connected {
		t.Fatalf("in-flight SyncNow resurrected disconnected cloud account in memory")
	}
	if _, ok, err := store.LoadCloudAccount(ctx, accID); err != nil || ok {
		t.Fatalf("in-flight SyncNow resurrected disconnected cloud account in SQLite: ok=%v err=%v", ok, err)
	}
}

func TestCloudSyncHandlesNullSpacesAndThreads(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/jaz-server":
			_, _ = w.Write([]byte(`{"ok":true,"mode":"cloud","auth":"oidc"}`))
		case "/v1/auth/me":
			_, _ = w.Write([]byte(`{"user_id":"u1","org_id":"o1","org_name":"Example Systems","email":"u@example.com","display_name":"Dev","role":"member"}`))
		case "/v1/spaces":
			_, _ = w.Write([]byte(`{"spaces":null}`))
		case "/v1/threads":
			_, _ = w.Write([]byte(`{"threads":null}`))
		case "/v1/usage/events":
			_, _ = w.Write([]byte(`{"events":null}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()

	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	svc := cloudsync.New(store, nil)
	st, err := svc.Connect(ctx, cloudsync.ConnectRequest{URL: srv.URL, Token: "tok"})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if st.LastError != "" || st.LastSyncAt.IsZero() {
		t.Fatalf("expected clean sync with null slices, got last_error=%q last_sync_at=%v", st.LastError, st.LastSyncAt)
	}
}

func TestCloudSyncRejectsPathTraversalAndScopesContextBySessionWorkspace(t *testing.T) {
	ctx := context.Background()
	upstreamHits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/jaz-server":
			_, _ = w.Write([]byte(`{"ok":true,"mode":"cloud","auth":"oidc"}`))
		case "/v1/auth/me":
			_, _ = w.Write([]byte(`{"user_id":"u1","org_id":"o1","org_name":"Example Systems","email":"u@example.com","display_name":"Dev","role":"member"}`))
		case "/v1/spaces":
			_, _ = w.Write([]byte(`{"spaces":[]}`))
		case "/v1/threads":
			_, _ = w.Write([]byte(`{"threads":[]}`))
		case "/v1/usage/events":
			_, _ = w.Write([]byte(`{"events":[]}`))
		default:
			if strings.HasPrefix(r.URL.Path, "/v1/runs/") || strings.HasPrefix(r.URL.Path, "/v1/auth/secret") {
				upstreamHits++
			}
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()

	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	svc := cloudsync.New(store, nil)
	st, err := svc.Connect(ctx, cloudsync.ConnectRequest{URL: srv.URL, Token: "tok"})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Path traversal segments must fail with ErrInvalidPath before contacting upstream.
	if _, err := svc.OpenRunFile(ctx, "run-1", "../../auth/secret"); !errors.Is(err, cloudsync.ErrInvalidPath) {
		t.Fatalf("OpenRunFile traversal err = %v, want ErrInvalidPath", err)
	}
	if _, err := svc.OpenRunFile(ctx, "..", "report.pdf"); !errors.Is(err, cloudsync.ErrInvalidPath) {
		t.Fatalf("OpenRunFile runID traversal err = %v, want ErrInvalidPath", err)
	}
	if _, _, err := svc.CreateTurn(ctx, "../other", json.RawMessage(`{"text":"hi"}`)); !errors.Is(err, cloudsync.ErrInvalidPath) {
		t.Fatalf("CreateTurn traversal err = %v, want ErrInvalidPath", err)
	}
	if upstreamHits != 0 {
		t.Fatalf("expected 0 upstream hits for traversal requests, got %d", upstreamHits)
	}

	// Session workspace context overrides the sidebar's active workspace for CloudScope.
	localCtx := sessioncontext.WithWorkspaceID(ctx, storage.LocalWorkspaceID)
	if scope := svc.CloudScope(localCtx); scope != nil {
		t.Fatalf("local session must not inherit active Cloud workspace scope, got %+v", scope)
	}
	if _, err := svc.SwitchWorkspace(ctx, cloudsync.LocalWorkspaceID); err != nil {
		t.Fatalf("SwitchWorkspace local: %v", err)
	}
	cloudSessionCtx := sessioncontext.WithWorkspaceID(ctx, st.ActiveWorkspaceID)
	scope := svc.CloudScope(cloudSessionCtx)
	if scope == nil || scope.OrgID != "o1" || scope.UserID != "u1" {
		t.Fatalf("cloud session must keep its own CloudScope while sidebar is local, got %+v", scope)
	}
}

func TestCloudSyncSecurityRedirectsAndDiscoverySanitization(t *testing.T) {
	ctx := context.Background()
	ssrfHits := 0
	victim := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ssrfHits++
		_, _ = w.Write([]byte("secret-local-service"))
	}))
	defer victim.Close()

	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/jaz-server":
			w.Header().Set("Content-Type", "application/json")
			// Malicious cross-origin oidc_login_url must be rejected and reset to /v1/auth/login.
			_, _ = w.Write([]byte(`{"ok":true,"mode":"cloud","auth":"oidc","oidc_login_url":"https://phish.evil.test/login"}`))
		case "/v1/auth/me":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"user_id":"u1","org_id":"o1","org_name":"Example Systems","email":"u@example.com","display_name":"Dev","role":"member"}`))
		case "/v1/spaces":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"spaces":[]}`))
		case "/v1/threads":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"threads":[]}`))
		case "/v1/usage/events":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"events":[]}`))
		case "/v1/models":
			// API endpoints must never follow redirects.
			http.Redirect(w, r, victim.URL+"/steal", http.StatusFound)
		case "/v1/runs/r1/files/evil.txt":
			// Redirecting to a different local loopback service must be blocked.
			http.Redirect(w, r, victim.URL+"/secret", http.StatusFound)
		case "/v1/runs/r1/files/meta.txt":
			// Redirecting to cloud metadata link-local IP must be blocked.
			http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
		case "/v1/runs/r1/files/ok.txt":
			// Same-origin loopback blob preview redirect stays permitted in local dev.
			http.Redirect(w, r, srvURL+"/_blobs/ok.txt", http.StatusFound)
		case "/_blobs/ok.txt":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("safe-content"))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	svc := cloudsync.New(store, nil)
	st, err := svc.Connect(ctx, cloudsync.ConnectRequest{URL: srv.URL, Token: "tok"})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if st.OIDCLoginURL != "/v1/auth/login" {
		t.Fatalf("malicious discovery oidc_login_url was not sanitized: %q", st.OIDCLoginURL)
	}
	if _, err := svc.ListModels(ctx); err == nil {
		t.Fatalf("expected API 302 redirect to fail instead of following")
	}
	if resp, err := svc.OpenRunFile(ctx, "r1", "evil.txt"); err == nil {
		resp.Body.Close()
		t.Fatalf("expected cross-origin loopback redirect to be blocked")
	}
	if resp, err := svc.OpenRunFile(ctx, "r1", "meta.txt"); err == nil {
		resp.Body.Close()
		t.Fatalf("expected metadata IP redirect to be blocked")
	}
	if ssrfHits != 0 {
		t.Fatalf("redirect reached victim local service %d times", ssrfHits)
	}
	resp, err := svc.OpenRunFile(ctx, "r1", "ok.txt")
	if err != nil {
		t.Fatalf("same-origin loopback preview redirect failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("same-origin preview status = %d, want 200", resp.StatusCode)
	}
}
