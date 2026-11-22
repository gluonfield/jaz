package cloudsync_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/cloudsync"
	"github.com/wins/jaz/backend/internal/coordinator"
	"github.com/wins/jaz/backend/internal/instructions"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestCloudSyncConnectMirrorStreamAndInstructions(t *testing.T) {
	ctx := context.Background()
	var (
		mu        sync.Mutex
		loggedOut bool
		instBody  = map[string]string{
			"org/org_1/agents":      "Org standing rule: prefer Go.",
			"user/usr_1/agents":     "User rule: be concise.",
			"user/usr_1/soul":       "Helpful and direct.",
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
				"user":  map[string]any{"user_id": "usr_1", "org_id": "org_1", "email": "u@cas.dev", "display_name": "Dev", "role": "admin"},
			})
		case r.URL.Path == "/v1/auth/me":
			if r.Header.Get("Authorization") != "Bearer cloud-jwt-1" {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"user_id": "usr_1", "org_id": "org_1", "org_name": "CAS", "email": "u@cas.dev", "display_name": "Dev", "role": "admin",
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
	st, err := svc.Connect(ctx, cloudsync.ConnectRequest{
		URL:          srv.URL,
		Code:         "oidc-code-1",
		CodeVerifier: "verifier-1",
	})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !st.Connected || st.User == nil || st.User.OrgName != "CAS" {
		t.Fatalf("unexpected connect status: %+v", st)
	}

	// Verify mirrored sessions & unified usage in SQLite.
	sessions, err := svc.ListSessions(ctx, false)
	if err != nil || len(sessions) != 1 || sessions[0].ID != "ct_1" || sessions[0].Backend != "cloud" {
		t.Fatalf("ListSessions: %+v err=%v", sessions, err)
	}
	usageEvents, err := store.UsageEvents(time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil || len(usageEvents) != 1 || usageEvents[0].Model != "google/gemini-4-neon" {
		t.Fatalf("unified UsageEvents: %+v err=%v", usageEvents, err)
	}

	// Verify transcript projection pulls events and stores them in SQLite.
	transcript, found, err := svc.LoadTranscript(ctx, "ct_1")
	if err != nil || !found {
		t.Fatalf("LoadTranscript: found=%v err=%v", found, err)
	}
	if len(transcript.Messages) != 2 || transcript.Messages[0].Content != "hello cloud" || transcript.Messages[1].Content != "hi there" {
		t.Fatalf("unexpected projected messages: %+v", transcript.Messages)
	}

	// Verify DB-stored instructions & horizons inject into local coordinator prompts when signed into Cloud!
	instStore := cloudsync.NewInstructionsStore(svc, instructions.NewLocalFSStore(root))
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
		"Helpful and direct.",
		"Long term context from DB.",
		"Short term sprint from DB.",
		"Standing instructions are stored in Jaz Cloud",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q; prompt:\n%s", want, prompt)
		}
	}

	// Verify Cloud MCP server config is exposed while connected.
	mcpSrv, ok := svc.MCPServer(ctx)
	if !ok || mcpSrv.URL != srv.URL+"/v1/mcp" || len(mcpSrv.Headers) != 1 {
		t.Fatalf("unexpected MCPServer: %+v ok=%v", mcpSrv, ok)
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
	if err := svc.Disconnect(ctx); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if svc.Status(ctx).Connected {
		t.Fatalf("expected disconnected after Disconnect")
	}
	if _, ok := svc.MCPServer(ctx); ok {
		t.Fatalf("expected MCPServer absent after Disconnect")
	}
	if svc.CloudScope(ctx) != nil {
		t.Fatalf("expected CloudScope nil after Disconnect")
	}
	_ = loggedOut
}

func TestCloudSyncDisconnectCancelsInFlightSyncWithoutResurrection(t *testing.T) {
	ctx := context.Background()
	started := make(chan struct{})
	release := make(chan struct{})
	var firstMe sync.Once

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/jaz-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mode": "cloud", "auth": "oidc"})
		case "/v1/auth/me":
			_ = json.NewEncoder(w).Encode(map[string]any{"user_id": "u1", "org_id": "o1", "org_name": "CAS", "email": "u@cas.dev", "display_name": "Dev", "role": "admin"})
		case "/v1/spaces":
			firstMe.Do(func() {
				// Let initial Connect sync finish cleanly; block only on second sync below.
			})
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

	svc := cloudsync.New(store, nil)
	// Save an already-connected account directly so Connect does not block on release.
	if err := store.SaveCloudAccount(ctx, storage.CloudAccount{
		BaseURL: srv.URL, Token: "tok", UserID: "u1", OrgID: "o1",
	}); err != nil {
		t.Fatal(err)
	}
	svc = cloudsync.New(store, nil)

	syncDone := make(chan struct{})
	go func() {
		_, _ = svc.SyncNow(ctx)
		close(syncDone)
	}()

	<-started
	if err := svc.Disconnect(ctx); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	close(release)
	<-syncDone

	if svc.Status(ctx).Connected {
		t.Fatalf("in-flight SyncNow resurrected disconnected cloud account in memory")
	}
	if _, ok, err := store.LoadCloudAccount(ctx); err != nil || ok {
		t.Fatalf("in-flight SyncNow resurrected disconnected cloud account in SQLite: ok=%v err=%v", ok, err)
	}
}
