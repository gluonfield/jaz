package acp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestManagerReleasesManagedProcessAfterEachTurn(t *testing.T) {
	for _, agent := range []string{acp.AgentClaude, acp.AgentCodex, acp.AgentGrok} {
		t.Run(agent, func(t *testing.T) {
			stateDir := t.TempDir()
			startLog := filepath.Join(stateDir, "starts")
			env := map[string]string{
				"JAZ_FAKE_ACP_LOAD":         "1",
				"JAZ_FAKE_ACP_PROMPT_DELAY": "1",
				"JAZ_FAKE_ACP_START_LOG":    startLog,
			}
			if agent == acp.AgentCodex {
				env["JAZ_FAKE_ACP_MATERIALIZED_SESSION"] = filepath.Join(stateDir, "session")
			}
			manager, spawned := newNamedProcessTestManager(t, t.TempDir(), agent, env)

			ctx := context.Background()
			sent, err := manager.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "first", Completion: acp.CompletionInline})
			if err != nil {
				t.Fatal(err)
			}
			ref := sent.ACPSession
			if job, err := manager.Wait(ctx, acp.WaitRequest{Session: ref, Timeout: 10 * time.Second}); err != nil || job.State != acp.StateIdle {
				t.Fatalf("first turn = %#v, %v", job, err)
			}
			if _, err := manager.Send(ctx, acp.SendRequest{Session: ref}); err == nil {
				t.Fatal("empty turn unexpectedly started")
			}

			start := make(chan struct{})
			errs := make(chan error, 2)
			for _, message := range []string{"second-a", "second-b"} {
				go func() {
					<-start
					_, err := manager.Send(ctx, acp.SendRequest{Session: ref, Message: message, Completion: acp.CompletionInline})
					errs <- err
				}()
			}
			close(start)
			succeeded := 0
			for range 2 {
				if err := <-errs; err == nil {
					succeeded++
				} else if !strings.Contains(err.Error(), "already running") {
					t.Fatal(err)
				}
			}
			if succeeded != 1 {
				t.Fatalf("successful concurrent sends = %d, want 1", succeeded)
			}
			if job, err := manager.Wait(ctx, acp.WaitRequest{Session: ref, Timeout: 10 * time.Second}); err != nil || job.State != acp.StateIdle {
				t.Fatalf("second turn = %#v, %v", job, err)
			}
			data, err := os.ReadFile(startLog)
			if err != nil {
				t.Fatal(err)
			}
			if starts := len(strings.Fields(string(data))); starts != 3 {
				t.Fatalf("process starts = %d, want one spawn validation plus one per turn; log=%q", starts, data)
			}
		})
	}
}

func TestManagerRejectsManagedAgentWithoutSessionLoad(t *testing.T) {
	startLog := filepath.Join(t.TempDir(), "starts")
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeNamedAgentManagerWithOptions(t, store, t.TempDir(), acp.AgentGrok, map[string]string{
		"JAZ_FAKE_ACP_START_LOG": startLog,
		"JAZ_FAKE_ACP_LOAD":      "0",
	}, "", "")
	t.Cleanup(manager.Close)
	_, err = manager.Spawn(context.Background(), acp.SpawnRequest{ACPAgent: acp.AgentGrok, Slug: "grok-process-test"})
	if err == nil || !strings.Contains(err.Error(), "requires session/load") {
		t.Fatalf("spawn error = %v", err)
	}
	data, err := os.ReadFile(startLog)
	if err != nil {
		t.Fatal(err)
	}
	if starts := len(strings.Fields(string(data))); starts != 1 {
		t.Fatalf("process starts = %d, want capability probe only; log=%q", starts, data)
	}
}

func TestManagerReleasesManagedProcessAfterIdleSideChat(t *testing.T) {
	stateDir := t.TempDir()
	startLog := filepath.Join(stateDir, "starts")
	manager, spawned := newNamedProcessTestManager(t, t.TempDir(), acp.AgentCodex, map[string]string{
		"JAZ_FAKE_ACP_MATERIALIZED_SESSION": filepath.Join(stateDir, "session"),
		"JAZ_FAKE_ACP_START_LOG":            startLog,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, id := range []string{"side-1", "side-2"} {
		if err := manager.SendSideChat(ctx, acp.SideChatRequest{Session: spawned.SessionID, ID: id, Message: "check"}); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(startLog)
	if err != nil {
		t.Fatal(err)
	}
	if starts := len(strings.Fields(string(data))); starts != 3 {
		t.Fatalf("process starts = %d, want one spawn validation plus one per side chat; log=%q", starts, data)
	}
}

func TestManagerDoesNotReplaceEstablishedCodexSession(t *testing.T) {
	for _, test := range []struct {
		name  string
		event bool
	}{{name: "message"}, {name: "event", event: true}} {
		t.Run(test.name, func(t *testing.T) {
			manager, store, spawned := newUnmaterializedCodexTestSession(t)
			var err error
			if test.event {
				err = store.AppendSessionEvents(spawned.SessionID, sessionevents.Event{Type: sessionevents.TypeACPMessage, Content: "prior turn"})
			} else {
				err = storage.AppendUserMessage(store, spawned.SessionID, "prior turn", nil, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			_, err = manager.Send(context.Background(), acp.SendRequest{
				Session: spawned.SessionID, Message: "first", Completion: acp.CompletionInline,
			})
			if err == nil || !strings.Contains(err.Error(), "-32002") {
				t.Fatalf("established session error = %v", err)
			}
		})
	}
}

func TestManagerDoesNotForkEstablishedCodexSessionWithoutProviderID(t *testing.T) {
	for _, test := range []struct {
		name  string
		event bool
	}{{name: "message"}, {name: "event", event: true}} {
		t.Run(test.name, func(t *testing.T) {
			manager, store, spawned := newFreshCodexTestSession(t)
			var err error
			if test.event {
				err = store.AppendSessionEvents(spawned.SessionID, sessionevents.Event{Type: sessionevents.TypeACPMessage, Content: "prior turn"})
			} else {
				err = storage.AppendUserMessage(store, spawned.SessionID, "prior turn", nil, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			_, err = manager.Send(context.Background(), acp.SendRequest{
				Session: spawned.SessionID, Message: "continue", Completion: acp.CompletionInline,
			})
			if err == nil || !strings.Contains(err.Error(), "provider session id is missing") {
				t.Fatalf("established session error = %v", err)
			}
		})
	}
}

func TestManagerDoesNotPersistUnsentCodexSession(t *testing.T) {
	manager, store, spawned := newUnmaterializedCodexTestSession(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := manager.Send(ctx, acp.SendRequest{
		Session:     spawned.SessionID,
		Message:     "first",
		Attachments: []storage.Attachment{{Name: "missing-uri"}},
		Completion:  acp.CompletionInline,
	}); err == nil {
		t.Fatal("invalid attachment unexpectedly started a turn")
	}
	session, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.RuntimeRef.SessionID != "" {
		t.Fatalf("unsent ACP session persisted as %q", session.RuntimeRef.SessionID)
	}
	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "retry", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	if job, err := manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second}); err != nil || job.State != acp.StateIdle {
		t.Fatalf("retried turn = %#v, %v", job, err)
	}
	session, err = store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.RuntimeRef.SessionID == "" {
		t.Fatal("sent ACP session was not persisted")
	}
}

func newUnmaterializedCodexTestSession(t *testing.T) (*acp.Manager, *jsonstore.Store, acp.SpawnResult) {
	t.Helper()
	manager, store, spawned := newFreshCodexTestSession(t)
	session, err := store.LoadSession(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	session.RuntimeRef.SessionID = "fake-session"
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	return manager, store, spawned
}

func newFreshCodexTestSession(t *testing.T) (*acp.Manager, *jsonstore.Store, acp.SpawnResult) {
	t.Helper()
	stateDir := t.TempDir()
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeNamedAgentManagerWithOptions(t, store, t.TempDir(), acp.AgentCodex, map[string]string{
		"JAZ_FAKE_ACP_MATERIALIZED_SESSION": filepath.Join(stateDir, "session"),
	}, "", "")
	t.Cleanup(manager.Close)
	spawned, err := manager.Spawn(context.Background(), acp.SpawnRequest{ACPAgent: acp.AgentCodex, Slug: "unmaterialized-codex"})
	if err != nil {
		t.Fatal(err)
	}
	return manager, store, spawned
}

func TestManagerRecordsClaudeRuntimeAuthFailure(t *testing.T) {
	for _, key := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_APIKEY", "JAZ_ACP_CLAUDE_API_KEY"} {
		t.Setenv(key, "")
	}
	root := t.TempDir()
	configDir := filepath.Join(root, "acp", "claude")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, ".claude.json"), []byte(`{"oauthAccount":{"accountUuid":"account-id"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, spawned := newClaudeTestManager(t, root, map[string]string{
		"JAZ_FAKE_ACP_AUTH_REQUIRED": "1",
	})

	ctx := context.Background()
	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "fail", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	job, err := manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if job.State != acp.StateFailed || job.Error != "Authentication required" {
		t.Fatalf("failed job = %#v", job)
	}
	status := acp.ProbeAgentAuth(acp.AgentClaude, acp.AgentConfig{Auth: acp.AgentAuthConfig{Mode: acp.AuthModeJazProfile}}, root, nil)
	if status.Authenticated || !strings.Contains(status.Reason, "reconnect Claude") {
		t.Fatalf("auth status after runtime rejection = %#v", status)
	}
}

func newClaudeTestManager(t *testing.T, root string, env map[string]string) (*acp.Manager, acp.SpawnResult) {
	return newNamedProcessTestManager(t, root, acp.AgentClaude, env)
}

func newNamedProcessTestManager(t *testing.T, root, agent string, env map[string]string) (*acp.Manager, acp.SpawnResult) {
	t.Helper()
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeNamedAgentManagerWithOptions(t, store, root, agent, env, "", "")
	t.Cleanup(manager.Close)
	spawned, err := manager.Spawn(context.Background(), acp.SpawnRequest{ACPAgent: agent, Slug: agent + "-process-test"})
	if err != nil {
		t.Fatal(err)
	}
	return manager, spawned
}
