package acp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/m365copilot"
	"github.com/wins/jaz/backend/internal/oauthbroker"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
	integrationoauth "github.com/wins/jaz/backend/pkg/integrations/oauth"
)

func TestMicrosoftCopilotManagerSpawnStreamResumeAndModels(t *testing.T) {
	var createCount atomic.Int32
	var turnCount atomic.Int32
	graph := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer valid-work-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/beta/copilot/conversations":
			createCount.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"m365-conv-1","status":"active","turnCount":0}`))
		case "/beta/copilot/conversations/m365-conv-1/chatOverStream":
			n := turnCount.Add(1)
			w.Header().Set("Content-Type", "text/event-stream")
			reply := fmt.Sprintf("Copilot reply %d", n)
			payload, _ := json.Marshal(m365copilot.Conversation{
				ID:        "m365-conv-1",
				State:     "active",
				TurnCount: int(n),
				Messages: []m365copilot.ConversationMessage{
					{
						ODataType: "#microsoft.graph.copilotConversationResponseMessage",
						ID:        fmt.Sprintf("msg-%d", n),
						Text:      reply,
					},
				},
			})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer graph.Close()

	root := t.TempDir()
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	auth := m365copilot.NewAuthService(store, m365copilot.OAuthConfig{
		ClientID:     "entra-client",
		GraphBaseURL: graph.URL + "/beta",
	}, oauthbroker.Broker{})

	cfg := BuiltinAgents()[AgentMicrosoftCopilot]
	cfg.MicrosoftCopilot = auth

	unauthStatus, unauthReadiness := ProbeAuthAndReadinessWithProviders(AgentMicrosoftCopilot, cfg, root, nil, nil)
	if unauthStatus.Authenticated || !unauthStatus.LoginCommandAvailable || unauthReadiness.Available {
		t.Fatalf("unauthenticated probe = %#v, %#v", unauthStatus, unauthReadiness)
	}

	if err := store.SaveToken(t.Context(), m365copilot.TokenConnectionID, integrationoauth.Token{
		AccessToken: "valid-work-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(time.Hour),
		ClientID:    "entra-client",
	}); err != nil {
		t.Fatal(err)
	}

	authStatus, readiness := ProbeAuthAndReadinessWithProviders(AgentMicrosoftCopilot, cfg, root, nil, nil)
	if !authStatus.Authenticated || authStatus.AuthKind != AuthKindOAuth || authStatus.RefreshOwner != RefreshOwnerJazOAuth || !readiness.Available {
		t.Fatalf("authenticated probe = %#v, %#v", authStatus, readiness)
	}

	manager := NewManager(store, Config{
		Root:             root,
		Workspace:        t.TempDir(),
		Agents:           map[string]AgentConfig{AgentMicrosoftCopilot: cfg},
		MicrosoftCopilot: auth,
	}, nil)
	defer manager.Close()

	models, err := manager.DiscoverModels(t.Context(), AgentMicrosoftCopilot)
	if err != nil {
		t.Fatalf("DiscoverModels error: %v", err)
	}
	if len(models.Models) != 0 || models.DefaultModel != "" {
		t.Fatalf("Microsoft Copilot must not invent model metadata: %#v", models)
	}

	spawned, err := manager.Spawn(t.Context(), SpawnRequest{ACPAgent: AgentMicrosoftCopilot})
	if err != nil {
		t.Fatalf("Spawn failed: %v", err)
	}
	if spawned.Session.RuntimeRef == nil || spawned.Session.RuntimeRef.SessionID != "m365-conv-1" {
		t.Fatalf("stored runtime session id = %#v, want m365-conv-1", spawned.Session.RuntimeRef)
	}

	if _, err := manager.Send(t.Context(), SendRequest{Session: spawned.SessionID, Message: "Hello Copilot", FromUser: true}); err != nil {
		t.Fatalf("Send turn 1 failed: %v", err)
	}
	done1, err := manager.Wait(t.Context(), WaitRequest{Session: spawned.SessionID, Timeout: 5 * time.Second})
	if err != nil || done1.State != StateIdle || done1.Assistant != "Copilot reply 1" {
		t.Fatalf("turn 1 result = %#v, err = %v", done1, err)
	}

	// Teardown simulates idle eviction or restart; the next Send must resume m365-conv-1 without creating a new conversation.
	manager.teardown(spawned.SessionID)
	if _, err := manager.Send(t.Context(), SendRequest{Session: spawned.SessionID, Message: "Follow up", FromUser: true}); err != nil {
		t.Fatalf("Send turn 2 after teardown failed: %v", err)
	}
	done2, err := manager.Wait(t.Context(), WaitRequest{Session: spawned.SessionID, Timeout: 5 * time.Second})
	if err != nil || done2.State != StateIdle || done2.Assistant != "Copilot reply 2" {
		t.Fatalf("turn 2 result = %#v, err = %v", done2, err)
	}
	if createCount.Load() != 1 || turnCount.Load() != 2 {
		t.Fatalf("createCount=%d turnCount=%d, want 1 and 2", createCount.Load(), turnCount.Load())
	}

	if err := DisconnectAgentAuth(t.Context(), AgentMicrosoftCopilot, cfg, root, nil); err != nil {
		t.Fatalf("DisconnectAgentAuth failed: %v", err)
	}
	if ProbeAgentAuth(AgentMicrosoftCopilot, cfg, root, nil).Authenticated {
		t.Fatal("expected unauthenticated after DisconnectAgentAuth")
	}
}
