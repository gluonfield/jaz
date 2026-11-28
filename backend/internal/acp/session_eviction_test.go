package acp_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestSendRecoversEvictedACPSessionDuringSetMode(t *testing.T) {
	for _, tc := range []struct {
		name      string
		history   bool
		missing   bool
		reconnect bool
	}{
		{name: "reload", history: true},
		{name: "reconnect", history: true, reconnect: true},
		{name: "recreate", missing: true},
		{name: "preserve_history", history: true, missing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, root := newTestStore(t)
			ctx := testContext(t)
			requestLog := filepath.Join(root, "requests.jsonl")
			eviction := filepath.Join(root, "evict")
			hold := filepath.Join(root, "hold-replacement")
			env := map[string]string{
				"JAZ_FAKE_ACP_SESSION_EVICTION": eviction,
				"JAZ_FAKE_ACP_REQUEST_LOG":      requestLog,
				"JAZ_FAKE_ACP_UNIQUE_SESSION":   "1",
				"JAZ_FAKE_ACP_NEW_HOLD":         hold,
			}
			manager := newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentAntigravity, env, "", "")
			t.Cleanup(manager.Close)
			thread, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: acp.AgentAntigravity, Slug: "evicted-bot", SourceType: storage.SourceBot})
			if err != nil {
				t.Fatal(err)
			}
			originalID := thread.Session.RuntimeRef.SessionID

			if tc.history {
				sendAndWait(t, ctx, manager, thread.SessionID, "first turn")
			}
			if tc.reconnect {
				manager.Close()
				env["JAZ_FAKE_ACP_REPORTED_MODE"] = "plan"
				manager = newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentAntigravity, env, "", "")
				t.Cleanup(manager.Close)
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

			rejected := tc.history && tc.missing
			replaced := tc.missing && !tc.history
			req := acp.SendRequest{Session: thread.SessionID, Message: "after eviction", FromUser: true}
			if replaced {
				err = sendWhileQueueingDuringReplacement(t, ctx, store, manager, req, hold, requestLog, before)
			} else {
				_, err = manager.Send(ctx, req)
			}
			stored, loadErr := store.LoadSession(thread.SessionID)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
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
			if replaced && (len(stored.QueuedMessages) != 1 || stored.QueuedMessages[0].Text != "queued during replacement") {
				t.Fatalf("queued input lost during replacement: %+v", stored.QueuedMessages)
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
			if providerID == "" || (providerID != originalID) != replaced {
				t.Fatalf("stored provider ID = %q, original = %q, expected replacement = %v", providerID, originalID, replaced)
			}
			want := []string{"session/set_mode", "session/load"}
			if tc.reconnect {
				want = []string{"session/load", "session/set_mode", "session/load", "session/set_mode"}
			}
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

func sendWhileQueueingDuringReplacement(t *testing.T, ctx context.Context, store *sqlitestore.Store, manager *acp.Manager, req acp.SendRequest, hold, requestLog string, before int) error {
	t.Helper()
	if err := os.WriteFile(hold, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(hold)
	sent := make(chan error, 1)
	go func() {
		_, err := manager.Send(ctx, req)
		sent <- err
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for !slices.ContainsFunc(readFakeACPRequests(t, requestLog)[before:], func(req fakeACPRequest) bool { return req.Method == "session/new" }) {
		select {
		case err := <-sent:
			t.Fatalf("send ended before session replacement: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
	if err := store.QueueTurn(storage.TurnDelivery{SessionID: req.Session, Message: storage.NewQueuedMessage("queued during replacement", nil)}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(hold); err != nil {
		t.Fatal(err)
	}
	return <-sent
}
