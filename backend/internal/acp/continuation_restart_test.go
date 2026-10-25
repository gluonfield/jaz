package acp_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	"github.com/wins/jaz/backend/internal/sessionrecovery"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

const continuationUnconfirmed = "Jaz restarted before the agent confirmed the continuation. Earlier task progress is saved."

func TestRepeatedRestartPreservesOriginAndNeverReplaysUnconfirmedContinuation(t *testing.T) {
	for _, agent := range []string{acp.AgentCodex, acp.AgentClaude} {
		for _, stage := range []string{"turn", "sending", "written", "acted", "delivered"} {
			t.Run(agent+"/"+stage, func(t *testing.T) {
				root, rawID := crashedPrompt(t, agent, "worker", "confirmed-chunk")
				id := string(rawID)
				read := func() storage.Session {
					t.Helper()
					store, err := sqlitestore.New(root)
					if err != nil {
						t.Fatal(err)
					}
					defer store.Close()
					session, err := store.LoadSession(id)
					if err != nil {
						t.Fatal(err)
					}
					return session
				}
				original := read()
				previous := original.Turn.StartedAt
				for _, boundary := range []string{"turn", stage} {
					crashStoredPrompt(t, root, agent, "worker", boundary, id)
					next := read()
					if next.Turn == nil || !next.Turn.ContinuationOf.Equal(original.Turn.StartedAt) || !next.Turn.StartedAt.After(previous) {
						t.Fatalf("recovery lost root or attempt generation: %+v", next.Turn)
					}
					previous = next.Turn.StartedAt
				}
				continued := stage == "turn" || stage == "delivered"
				store, err := sqlitestore.New(root)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = store.Close() })
				requestLog := filepath.Join(root, "recovered.jsonl")
				env := map[string]string{
					"JAZ_FAKE_ACP_RESUME":         "1",
					"JAZ_FAKE_ACP_UNIQUE_SESSION": "cwd",
					"JAZ_FAKE_ACP_REQUEST_LOG":    requestLog,
				}
				manager := newFakeNamedAgentManagerWithOptions(t, store, root, agent, env, "", "")
				t.Cleanup(manager.Close)
				locks, events := sessionlock.New(), sessionevents.New()
				handler := &server.Server{Store: store, ACP: manager, Locks: locks, Events: events}
				completed := make(chan *storage.Turn, 1)
				manager.TurnFinished = func(ctx context.Context, job acp.Job) {
					if job.ID == id {
						session, err := store.LoadSession(id)
						if err != nil {
							t.Error(err)
						}
						completed <- session.Turn
					}
					handler.HandleACPTurnFinished(ctx, job)
				}
				ctx := testContext(t)
				for range 2 {
					if err := sessionrecovery.Resume(ctx, store, handler, locks, events, log.New(io.Discard)); err != nil {
						t.Fatal(err)
					}
				}
				wantStatus, wantError := storage.StatusError, continuationUnconfirmed
				if continued {
					if job, err := manager.Wait(ctx, acp.WaitRequest{Session: id}); err != nil || job.State != acp.StateIdle {
						t.Fatalf("eligible recovery failed: %+v %v", job, err)
					}
					turn := <-completed
					if turn == nil || !turn.ContinuationOf.Equal(original.Turn.StartedAt) || !turn.StartedAt.After(previous) {
						t.Fatalf("last recovery lost its origin: %+v", turn)
					}
					wantStatus, wantError = storage.StatusIdle, ""
				}
				after, err := store.LoadSession(id)
				if err != nil || after.Status != wantStatus || after.Error != wantError || after.Turn != nil {
					t.Fatalf("finalizer outcome: %+v %v", after, err)
				}
				deadline := time.Now().Add(10 * time.Second)
				for {
					parent, err := store.LoadSession(original.ParentID)
					if err != nil {
						t.Fatal(err)
					}
					if parent.Status == storage.StatusIdle && len(parent.QueuedMessages) == 0 {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("parent result did not settle")
					}
					time.Sleep(time.Millisecond)
				}
				if !continued {
					for range 2 {
						if err := handler.FailInterruptedTurn(ctx, id, storage.ErrPromptUnconfirmed); err != nil {
							t.Fatal(err)
						}
					}
					if err := sessionrecovery.Resume(ctx, store, handler, locks, events, log.New(io.Discard)); err != nil {
						t.Fatal(err)
					}
					// Duplicate finalization must preserve the accurate stored reason too.
					final, err := store.LoadSession(id)
					if err != nil || final.Error != continuationUnconfirmed {
						t.Fatalf("duplicate finalizer changed reason: %+v %v", final, err)
					}
				}
				manager.Close()
				childCalls, reasons := 0, 0
				for _, request := range readFakeACPRequests(t, requestLog) {
					if request.Method != "session/prompt" {
						continue
					}
					var params struct {
						SessionID string
						Prompt    []struct{ Text string }
					}
					if err := json.Unmarshal(request.Params, &params); err != nil {
						t.Fatal(err)
					}
					if params.SessionID == original.RuntimeRef.SessionID {
						childCalls++
						if len(params.Prompt) != 1 || params.Prompt[0].Text != "Continue from where you left off." {
							t.Fatalf("recovery resent original input: %s", request.Params)
						}
					} else if strings.Contains(string(request.Params), continuationUnconfirmed) {
						reasons++
						if !strings.Contains(string(request.Params), "state interrupted") {
							t.Fatalf("wrong parent result category: %s", request.Params)
						}
					}
				}
				wantCalls, wantReasons := 0, 1
				if continued {
					wantCalls, wantReasons = 1, 0
				}
				if childCalls != wantCalls || reasons != wantReasons {
					t.Fatalf("child prompts=%d parent reasons=%d; want %d %d", childCalls, reasons, wantCalls, wantReasons)
				}
				if stage == "acted" {
					effects, err := os.ReadFile(filepath.Join(root, "effects"))
					if err != nil || string(effects) != "effect\n" {
						t.Fatalf("effect lost or replayed: %q %v", effects, err)
					}
					commitments, err := store.ListOpenCommitments(original.ParentID)
					if err != nil || len(commitments) != 1 {
						t.Fatalf("interruption closed the task: %+v %v", commitments, err)
					}
				}
			})
		}
	}
}

func TestLegacyRecoveryKeepsUnknownOriginUntilItsOwnConfirmation(t *testing.T) {
	root, rawID := crashedPrompt(t, acp.AgentCodex, "user", "legacy")
	id := string(rawID)
	crashStoredPrompt(t, root, acp.AgentCodex, "user", "delivered", id)
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.LoadSession(id)
	if err != nil || first.Turn == nil || !first.Turn.ContinuationOf.IsZero() || first.Turn.Prompt.State != storage.PromptConfirmed {
		t.Fatalf("legacy evidence invented an origin: %+v %v", first.Turn, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	crashStoredPrompt(t, root, acp.AgentCodex, "user", "turn", id)
	store, err = sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	second, err := store.LoadSession(id)
	if err != nil || second.Turn == nil || !second.Turn.ContinuationOf.Equal(first.Turn.StartedAt) {
		t.Fatalf("confirmed legacy attempt did not establish its own origin: %+v %v", second.Turn, err)
	}
	manager := newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentCodex, map[string]string{"JAZ_FAKE_ACP_RESUME": "1", "JAZ_FAKE_ACP_UNIQUE_SESSION": "cwd", "JAZ_FAKE_ACP_BLOCK_PROMPT": "1"}, "", "")
	t.Cleanup(manager.Close)
	if _, err := manager.Send(t.Context(), acp.SendRequest{Session: id, Message: "Fresh user instruction", FromUser: true}); err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, testContext(t), manager, id)
	fresh, err := store.LoadSession(id)
	if err != nil || fresh.Turn == nil || !fresh.Turn.ContinuationOf.IsZero() {
		t.Fatalf("new user instruction inherited recovery lineage: %+v %v", fresh.Turn, err)
	}
}
