package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestACPCompactRoutesToManager(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-compact",
		Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     "codex",
			SessionID: "acp-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{
		ID:              session.ID,
		Slug:            session.Slug,
		ACPAgent:        "codex",
		State:           acp.StateIdle,
		ActiveOperation: acp.ActiveOperationCompact,
	}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/compact", nil).WithContext(ctx)
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	manager.mu.Lock()
	compacted := manager.compacted
	compactCtxErr := manager.compactCtxErr
	sent := manager.sent
	manager.mu.Unlock()
	if compactCtxErr != nil {
		t.Fatalf("compact used cancelled request context: %v", compactCtxErr)
	}
	if compacted.Session != session.ID {
		t.Fatalf("compact request = %#v, want session %q", compacted, session.ID)
	}
	if sent.Session != "" || sent.Message != "" {
		t.Fatalf("normal send was used for compact: %#v", sent)
	}
}

func TestACPStreamCompactCommandRoutesToManagerCompact(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "codex-typed-compact",
		Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     "codex",
			SessionID: "acp-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{
		ID:              session.ID,
		Slug:            session.Slug,
		State:           acp.StateIdle,
		ActiveOperation: acp.ActiveOperationCompact,
	}}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/messages:stream", strings.NewReader(`{"message":"/compact","attachment_ids":["missing"],"contexts":[{"type":"selection","text":"ignored"}]}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	manager.mu.Lock()
	compacted := manager.compacted
	sent := manager.sent
	manager.mu.Unlock()
	if compacted.Session != session.ID {
		t.Fatalf("compact request = %#v, want session %q", compacted, session.ID)
	}
	if sent.Session != "" || sent.Message != "" {
		t.Fatalf("normal send was used for slash compact: %#v", sent)
	}
}

func TestACPCompactRejectsUnsupportedAgentBeforeTurn(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "grok-compact",
		Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     acp.AgentGrok,
			SessionID: "acp-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{ID: session.ID, Slug: session.Slug, State: acp.StateIdle}}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/compact", nil)
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	manager.mu.Lock()
	compacted := manager.compacted
	manager.mu.Unlock()
	if compacted.Session != "" {
		t.Fatalf("compact request = %#v, want none", compacted)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != storage.StatusIdle {
		t.Fatalf("session status = %q, want idle", loaded.Status)
	}
}

func TestACPStreamCompactRejectsUnsupportedAgentBeforeTurn(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{
		Slug:    "grok-typed-compact",
		Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:      storage.RuntimeACP,
			Agent:     acp.AgentGrok,
			SessionID: "acp-session",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{job: acp.Job{ID: session.ID, Slug: session.Slug, State: acp.StateIdle}}

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/"+session.ID+"/messages:stream", strings.NewReader(`{"message":"/compact","attachment_ids":["missing"]}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()

	(&Server{Store: store, ACP: manager}).Handler().ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", res.Code, res.Body.String())
	}
	manager.mu.Lock()
	compacted := manager.compacted
	sent := manager.sent
	manager.mu.Unlock()
	if compacted.Session != "" || sent.Session != "" {
		t.Fatalf("manager called: compact=%#v send=%#v", compacted, sent)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != storage.StatusIdle {
		t.Fatalf("session status = %q, want idle", loaded.Status)
	}
}

func TestSessionResponseAdvertisesCompactActionForSupportedACPAgents(t *testing.T) {
	for _, tc := range []struct {
		agent string
		want  bool
	}{
		{agent: acp.AgentCodex, want: true},
		{agent: acp.AgentClaude, want: true},
		{agent: acp.AgentGrok, want: false},
	} {
		t.Run(tc.agent, func(t *testing.T) {
			resp := canonicalSessionResponse(storage.Session{
				Runtime: storage.RuntimeACP,
				RuntimeRef: &storage.RuntimeRef{
					Type:  storage.RuntimeACP,
					Agent: tc.agent,
				},
			})
			got := resp.Actions != nil && resp.Actions.Compact
			if got != tc.want {
				t.Fatalf("compact action = %v, want %v", got, tc.want)
			}
		})
	}
}
