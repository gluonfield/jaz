package acp_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/acpcredentials"
	jsonstore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestCloudClaudeRestoresNativeSessionWithRefreshedCredentials(t *testing.T) {
	root := t.TempDir()
	tokens := filepath.Join(root, "token-log")
	rejected := filepath.Join(root, "rejected")
	store, err := jsonstore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	var versions []int64
	const workspace, profileID = "fixture-workspace", "acp:claude"
	manager := newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentClaude, map[string]string{
		"JAZ_FAKE_ACP_CLAUDE_AUTH_LOG":      tokens,
		"JAZ_FAKE_ACP_SIGN_IN_EXPIRED_FILE": rejected,
	}, "", "", func(cfg *acp.Config) {
		cfg.ResolveCloudProfile = func(_ context.Context, workspaceID, id string, rejectedVersion int64) (acpcredentials.Profile, error) {
			if workspaceID != workspace || id != profileID {
				t.Errorf("unexpected OAuth resolution scope: %q %q", workspaceID, id)
			}
			versions = append(versions, rejectedVersion)
			version, token := int64(1), "fixture-initial"
			if rejectedVersion == version {
				version, token = 2, "fixture-refreshed"
			}
			return acpcredentials.Profile{ID: id, Agent: acp.AgentClaude, Kind: acpcredentials.OAuthBundle, Version: version,
				OAuth: &acpcredentials.OAuthTokens{AccessToken: token, ExpiresAt: time.Now().Add(time.Hour)}}, nil
		}
	})
	t.Cleanup(manager.Close)
	ctx := t.Context()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: acp.AgentClaude, WorkspaceID: workspace, CloudCredentialProfileID: profileID})
	if err != nil {
		t.Fatal(err)
	}
	sendAndWait(t, ctx, manager, spawned.SessionID, "first turn")
	if err := os.WriteFile(rejected, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, sendErr := manager.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "rejected turn", Completion: acp.CompletionInline})
	job, err := manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil || job.State != acp.StateFailed {
		t.Fatalf("expected OAuth rejection, got %s (%v, %v)", job.State, sendErr, err)
	}
	if err := os.Remove(rejected); err != nil {
		t.Fatal(err)
	}
	sendAndWait(t, ctx, manager, spawned.SessionID, "next turn")
	raw, err := os.ReadFile(tokens)
	if err != nil || !slices.Equal(strings.Fields(string(raw)), []string{"fixture-initial", "fixture-refreshed"}) || !slices.Equal(versions, []int64{0, 1}) {
		t.Fatalf("native Claude did not receive the refreshed credential after rejection: versions %v, read error %v", versions, err)
	}
}
