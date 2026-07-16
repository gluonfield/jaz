package acp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestManagerReleasesClaudeProcessAfterEachTurn(t *testing.T) {
	startLog := filepath.Join(t.TempDir(), "starts")
	manager, spawned := newClaudeTestManager(t, t.TempDir(), map[string]string{
		"JAZ_FAKE_ACP_LOAD":         "1",
		"JAZ_FAKE_ACP_PROMPT_DELAY": "1",
		"JAZ_FAKE_ACP_START_LOG":    startLog,
	})

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
	if starts := len(strings.Fields(string(data))); starts != 2 {
		t.Fatalf("Claude process starts = %d, want one per turn; log=%q", starts, data)
	}
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
	t.Helper()
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentClaude, env, "", "")
	t.Cleanup(manager.Close)
	spawned, err := manager.Spawn(context.Background(), acp.SpawnRequest{ACPAgent: acp.AgentClaude, Slug: "claude-test"})
	if err != nil {
		t.Fatal(err)
	}
	return manager, spawned
}
