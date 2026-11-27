package acp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestSendRecoversEvictedACPSessionDuringSetMode(t *testing.T) {
	for _, tc := range []struct {
		name    string
		history bool
		missing bool
	}{
		{name: "reload", history: true},
		{name: "recreate", missing: true},
		{name: "preserve_history", history: true, missing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, root := newTestStore(t)
			ctx := testContext(t)
			requestLog := filepath.Join(root, "requests.jsonl")
			eviction := filepath.Join(root, "evict")
			manager := newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentAntigravity, map[string]string{
				"JAZ_FAKE_ACP_SESSION_EVICTION": eviction,
				"JAZ_FAKE_ACP_REQUEST_LOG":      requestLog,
				"JAZ_FAKE_ACP_UNIQUE_SESSION":   "1",
			}, "", "")
			t.Cleanup(manager.Close)
			thread, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: acp.AgentAntigravity, Slug: "evicted-bot", SourceType: storage.SourceBot})
			if err != nil {
				t.Fatal(err)
			}
			originalID := thread.Session.RuntimeRef.SessionID

			if tc.history {
				sendAndWait(t, ctx, manager, thread.SessionID, "first turn")
			}
			before := len(readFakeACPRequests(t, requestLog))
			if tc.missing {
				if err := os.Remove(filepath.Join(root, "fake-provider-sessions", originalID)); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(eviction, nil, 0o600); err != nil {
				t.Fatal(err)
			}

			_, err = manager.Send(ctx, acp.SendRequest{Session: thread.SessionID, Message: "after eviction", FromUser: true})
			stored, loadErr := store.LoadSession(thread.SessionID)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			rejected := tc.history && tc.missing
			if rejected {
				if err == nil || !strings.Contains(err.Error(), "conversation history") || strings.Contains(err.Error(), "-32602") {
					t.Fatalf("send with a missing session and history = %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if job, err := manager.Wait(ctx, acp.WaitRequest{Session: thread.SessionID}); err != nil || job.State != acp.StateIdle {
					t.Fatalf("retried turn = %+v, %v", job, err)
				}
			}
			messages, err := store.LoadMessageRecords(thread.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			var received int
			for _, message := range messages {
				if message.Role == "user" && message.Content == "after eviction" {
					received++
				}
			}
			wantMessages := 1
			if rejected {
				wantMessages = 0
			}
			if received != wantMessages {
				t.Fatalf("retried input written %d times, want %d", received, wantMessages)
			}
			providerID := stored.RuntimeRef.SessionID
			replaced := tc.missing && !rejected
			if providerID == "" || (providerID != originalID) != replaced {
				t.Fatalf("stored provider ID = %q, original = %q, expected replacement = %v", providerID, originalID, replaced)
			}
			want := []string{"session/set_mode", "session/load"}
			if replaced {
				want = append(want, "session/new")
			}
			if !rejected {
				want = append(want, "session/set_mode", "session/prompt")
			}
			var methods []string
			for _, request := range readFakeACPRequests(t, requestLog)[before:] {
				if !strings.HasPrefix(request.Method, "session/") {
					continue
				}
				var params struct {
					SessionID string `json:"sessionId"`
					ModeID    string `json:"modeId"`
				}
				if err := json.Unmarshal(request.Params, &params); err != nil {
					t.Fatal(err)
				}
				id := providerID
				if len(methods) < 2 {
					id = originalID
				}
				if request.Method != "session/new" && params.SessionID != id || request.Method == "session/set_mode" && params.ModeID != "default" {
					t.Fatalf("unexpected %s params: %s", request.Method, request.Params)
				}
				methods = append(methods, request.Method)
			}
			if !slices.Equal(methods, want) {
				t.Fatalf("ACP calls = %v, want %v", methods, want)
			}
		})
	}
}
