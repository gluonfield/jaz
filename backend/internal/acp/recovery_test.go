package acp_test

import (
	"context"
	"errors"
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

type unavailableEventStore struct{ acp.Store }

func (s unavailableEventStore) AppendSessionEvents(string, ...sessionevents.Event) error {
	return errors.New("transcript unavailable")
}

func TestResumeInterruptedTurnPreservesProviderSessionAndIntent(t *testing.T) {
	for _, agent := range []string{acp.AgentCodex, acp.AgentClaude} {
		for _, kind := range []string{"chat", "plan", "goal", "steered_goal", "compact"} {
			t.Run(agent+"/"+kind, func(t *testing.T) {
				store, err := sqlitestore.New(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = store.Close() })
				root := t.TempDir()
				planConfig := "0"
				if agent == acp.AgentCodex {
					planConfig = "1"
				}
				first := newFakeNamedAgentManagerWithOptions(t, unavailableEventStore{Store: store}, root, agent, map[string]string{
					"JAZ_FAKE_ACP_BLOCK_PROMPT":    "1",
					"JAZ_FAKE_ACP_PROMPT_QUEUEING": "1",
					"JAZ_FAKE_ACP_PLAN_CONFIG":     planConfig,
					"JAZ_FAKE_ACP_SET_CONFIG":      "1",
				}, "", "")
				t.Cleanup(first.Close)
				ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
				defer cancel()
				spawned, err := first.Spawn(ctx, acp.SpawnRequest{ACPAgent: agent, Slug: "restart-" + kind})
				if err != nil {
					t.Fatal(err)
				}
				if kind == "compact" {
					_, err = first.Compact(ctx, acp.CompactRequest{Session: spawned.SessionID})
				} else {
					_, err = first.Send(ctx, acp.SendRequest{
						Session: spawned.SessionID, Message: "work to resume", PlanRequested: kind == "plan",
						GoalRequested: kind == "goal",
					})
				}
				if err != nil {
					t.Fatal(err)
				}
				for {
					job, err := first.Status(spawned.SessionID)
					if err != nil {
						t.Fatal(err)
					}
					if len(job.ToolCalls) > 0 {
						break
					}
					select {
					case <-ctx.Done():
						t.Fatal("provider did not start its blocking tool")
					case <-time.After(10 * time.Millisecond):
					}
				}
				if kind == "steered_goal" {
					if _, err := first.Steer(ctx, acp.SteerRequest{Session: spawned.SessionID, Message: "work to resume", GoalRequested: true}); err != nil {
						t.Fatal(err)
					}
				}
				first.Close()
				stored, err := store.LoadSession(spawned.SessionID)
				if err != nil || stored.Status != storage.StatusInterrupted || stored.RuntimeRef.SessionID != "fake-session" {
					t.Fatalf("shutdown session = %+v, %v", stored, err)
				}
				goalRequested := kind == "goal" || kind == "steered_goal"
				if stored.Turn == nil || stored.Turn.PlanRequested != (kind == "plan") || stored.Turn.GoalRequested != goalRequested {
					t.Fatalf("interrupted turn settings = %+v", stored.Turn)
				}
				events, err := store.LoadSessionEvents(spawned.SessionID)
				if err != nil || len(events) != 0 {
					t.Fatalf("expected unavailable transcript, events = %d, err = %v", len(events), err)
				}
				requestLog := filepath.Join(root, "resumed-requests.jsonl")
				second := newFakeNamedAgentManagerWithOptions(t, store, root, agent, map[string]string{
					"JAZ_FAKE_ACP_REQUEST_LOG": requestLog,
					"JAZ_FAKE_ACP_PLAN_CONFIG": planConfig,
					"JAZ_FAKE_ACP_SET_CONFIG":  "1",
					"JAZ_FAKE_ACP_RESUME":      "1",
				}, "", "")
				t.Cleanup(second.Close)
				locks := sessionlock.New()
				bus := sessionevents.New()
				handler := &server.Server{Store: store, ACP: second, Locks: locks, Events: bus}
				second.TurnFinished = handler.HandleACPTurnFinished
				if err := sessionrecovery.Resume(ctx, store, second, locks, bus, log.New(io.Discard)); err != nil {
					t.Fatal(err)
				}
				job, err := second.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID})
				if err != nil || job.State != acp.StateIdle || job.ACPSession != "fake-session" {
					t.Fatalf("resumed job = %+v, %v", job, err)
				}
				stored, err = store.LoadSession(spawned.SessionID)
				if err != nil || stored.Status != storage.StatusIdle || stored.Error != "" || stored.Turn != nil {
					t.Fatalf("completed recovery session = %+v, %v", stored, err)
				}
				if kind == "plan" && len(job.Plan) == 0 {
					t.Fatal("interrupted planning resumed in execution mode")
				}
				requests, err := os.ReadFile(requestLog)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Count(string(requests), `"method":"session/resume"`) != 1 || strings.Contains(string(requests), `"method":"session/new"`) {
					t.Fatalf("provider session was not restored exactly once: %s", requests)
				}
				if strings.Count(string(requests), `"method":"session/prompt"`) != 1 {
					t.Fatalf("expected one continuation prompt: %s", requests)
				}
				if kind == "compact" && !strings.Contains(string(requests), `"text":"/compact"`) {
					t.Fatalf("compaction was not resumed: %s", requests)
				}
				if goalRequested && !strings.Contains(string(requests), `create_goal`) {
					t.Fatalf("goal mode was not resumed: %s", requests)
				}
				messages, err := store.LoadMessageRecords(spawned.SessionID)
				if err != nil {
					t.Fatal(err)
				}
				userCount := 0
				for _, message := range messages {
					if message.Role == "user" {
						userCount++
						if message.Content != "work to resume" {
							t.Fatalf("synthetic transcript message: %+v", message)
						}
					}
				}
				wantUsers := 1
				if kind == "compact" {
					wantUsers = 0
				} else if kind == "steered_goal" {
					wantUsers = 2
				}
				if userCount != wantUsers {
					t.Fatalf("user message count = %d, want %d", userCount, wantUsers)
				}
			})
		}
	}
}

func TestRecoveryDoesNotReplaceMissingProviderSession(t *testing.T) {
	for _, providerID := range []string{"", "missing-session"} {
		t.Run("id="+providerID, func(t *testing.T) {
			store, err := sqlitestore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			root := t.TempDir()
			requestLog := filepath.Join(root, "requests.jsonl")
			manager := newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentCodex, map[string]string{
				"JAZ_FAKE_ACP_REQUEST_LOG": requestLog,
			}, "", "")
			t.Cleanup(manager.Close)
			session, err := store.CreateSession(storage.CreateSession{
				Slug: "missing-provider", RuntimeRef: &storage.RuntimeRef{Agent: acp.AgentCodex, SessionID: providerID, Cwd: root},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := manager.ResumeInterruptedTurn(t.Context(), session.ID); err == nil {
				t.Fatal("recovery succeeded without the original provider session")
			}
			requests, err := os.ReadFile(requestLog)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if strings.Contains(string(requests), `"method":"session/new"`) || strings.Contains(string(requests), `"method":"session/prompt"`) {
				t.Fatalf("recovery created or prompted a replacement: %s", requests)
			}
		})
	}
}
