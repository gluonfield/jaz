package acp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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

func TestRestartInterruptsUnconfirmedPromptsAndContinuesConfirmedOnes(t *testing.T) {
	for _, agent := range []string{acp.AgentCodex, acp.AgentClaude} {
		for _, kind := range []string{"worker", "user", "result"} {
			for _, stage := range []string{"sending", "written", "legacy", "acted", "metadata", "confirming", "confirmed-chunk", "confirmed-thought", "confirmed-tool", "confirmed-plan"} {
				t.Run(agent+"/"+kind+"/"+stage, func(t *testing.T) {
					root, id := crashedPrompt(t, agent, kind, stage)
					store, err := sqlitestore.New(root)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = store.Close() })
					before, err := store.LoadSession(string(id))
					if err != nil {
						t.Fatal(err)
					}
					confirmed := strings.HasPrefix(stage, "confirmed-")
					wantState := storage.PromptSending
					if confirmed {
						wantState = storage.PromptConfirmed
					}
					if before.Turn == nil || stage == "legacy" && before.Turn.Prompt != nil || stage != "legacy" && (before.Turn.Prompt == nil || before.Turn.Prompt.State != wantState) {
						t.Fatalf("boundary lost durable prompt state: %+v", before.Turn)
					}
					if kind == "result" && (before.Turn.Output == nil || before.Turn.Output.ReplyTo != "asker" || before.Turn.Output.Hops != 2) {
						t.Fatalf("result routing lost: %+v", before.Turn)
					}
					requestLog := filepath.Join(root, "recovered.jsonl")
					manager := newFakeNamedAgentManagerWithOptions(t, store, root, agent, map[string]string{
						"JAZ_FAKE_ACP_RESUME": "1", "JAZ_FAKE_ACP_UNIQUE_SESSION": "cwd", "JAZ_FAKE_ACP_REQUEST_LOG": requestLog,
					}, "", "")
					t.Cleanup(manager.Close)
					locks, events := sessionlock.New(), sessionevents.New()
					handler := &server.Server{Store: store, ACP: manager, Locks: locks, Events: events}
					manager.TurnFinished = handler.HandleACPTurnFinished
					ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
					defer cancel()
					for range 2 {
						if err := sessionrecovery.Resume(ctx, store, handler, locks, events, log.New(io.Discard)); err != nil {
							t.Fatal(err)
						}
					}
					if confirmed {
						job, err := manager.Wait(ctx, acp.WaitRequest{Session: string(id)})
						if err != nil || job.State != acp.StateIdle {
							t.Fatalf("confirmed turn did not continue: %+v, %v", job, err)
						}
					}
					thread, err := store.LoadSession(string(id))
					if err != nil {
						t.Fatal(err)
					}
					wantStatus, wantError := storage.StatusError, storage.ErrPromptUnconfirmed.Error()
					if confirmed {
						wantStatus, wantError = storage.StatusIdle, ""
					}
					if thread.Status != wantStatus || thread.Error != wantError || thread.Turn != nil {
						t.Fatalf("recovery outcome = %s %q, turn %+v; want %s %q with no active turn", thread.Status, thread.Error, thread.Turn, wantStatus, wantError)
					}
					if kind == "worker" {
						deadline := time.Now().Add(10 * time.Second)
						for {
							parent, err := store.LoadSession(thread.ParentID)
							if err != nil {
								t.Fatal(err)
							}
							if parent.Status == storage.StatusIdle && len(parent.QueuedMessages) == 0 {
								break
							}
							if time.Now().After(deadline) {
								t.Fatal("parent never settled after receiving the result")
							}
							time.Sleep(time.Millisecond)
						}
						commitments, err := store.ListOpenCommitments(thread.ParentID)
						if err != nil || len(commitments) != 1 || len(commitments[0].Threads) != 1 || commitments[0].Threads[0].ID != thread.ID || commitments[0].Threads[0].State == "working" {
							t.Fatalf("commitment closed, lost or still moving: %+v, %v", commitments, err)
						}
					}
					manager.Close()
					requests, _ := os.ReadFile(requestLog)
					calls := 0
					for _, line := range strings.Split(string(requests), "\n") {
						var request struct {
							Method string
							Params struct {
								SessionID string
								Prompt    []struct{ Text string }
							}
						}
						if line == "" {
							continue
						}
						if err := json.Unmarshal([]byte(line), &request); err != nil {
							t.Fatal(err)
						}
						if request.Method != "session/prompt" || request.Params.SessionID != before.RuntimeRef.SessionID {
							continue
						}
						calls++
						if !confirmed || len(request.Params.Prompt) != 1 || request.Params.Prompt[0].Text != "Continue from where you left off." {
							t.Fatalf("unconfirmed task was sent again, or confirmation changed: %s", line)
						}
					}
					wantCalls := 0
					if confirmed {
						wantCalls = 1
					}
					if calls != wantCalls {
						t.Fatalf("recovery sent %d task prompts, want %d", calls, wantCalls)
					}
					if kind == "worker" && !confirmed && (strings.Count(string(requests), storage.ErrPromptUnconfirmed.Error()) != 1 || !strings.Contains(string(requests), "state interrupted")) {
						t.Fatalf("parent missed interrupted result or received it twice: %s", requests)
					}
					messages, err := store.LoadMessageRecords(thread.ID)
					if err != nil {
						t.Fatal(err)
					}
					users := 0
					for _, message := range messages {
						if message.Role == "user" {
							users++
							if message.Content != "Original task to deliver" || message.FromUser != (kind == "user") {
								t.Fatalf("original visible prompt changed: %+v", message)
							}
						}
					}
					wantUsers := 1
					if kind == "result" {
						wantUsers = 0
					}
					if users != wantUsers {
						t.Fatalf("original user prompt count = %d, want %d", users, wantUsers)
					}
					if stage == "acted" {
						effects, err := os.ReadFile(filepath.Join(root, "effects"))
						if err != nil || string(effects) != "effect\n" {
							t.Fatalf("action lost or repeated: %q, %v", effects, err)
						}
					}
				})
			}
		}
	}
}

type confirmationStore struct {
	acp.Store
	attempts      atomic.Int32
	reject        bool
	rejectSending bool
}

func (s *confirmationStore) SetTurnPromptState(id, agentSessionID string, startedAt time.Time, state storage.PromptState) error {
	if state == storage.PromptSending && s.rejectSending {
		return errors.New("sending write unavailable")
	}
	if state == storage.PromptConfirmed {
		s.attempts.Add(1)
		if s.reject {
			return errors.New("confirmation write unavailable")
		}
	}
	return s.Store.SetTurnPromptState(id, agentSessionID, startedAt, state)
}

func TestPromptConfirmationIsRecordedOncePerTurn(t *testing.T) {
	for _, agent := range []string{acp.AgentCodex, acp.AgentClaude} {
		for _, reject := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reject=%t", agent, reject), func(t *testing.T) {
				store, root := newTestStore(t)
				counted := &confirmationStore{Store: store, reject: reject}
				manager := newFakeNamedAgentManagerWithOptions(t, counted, root, agent, nil, "", "")
				t.Cleanup(manager.Close)
				spawned, err := manager.Spawn(t.Context(), acp.SpawnRequest{ACPAgent: agent, Slug: "confirmation"})
				if err != nil {
					t.Fatal(err)
				}
				for turn := range 2 {
					sendAndWait(t, t.Context(), manager, spawned.SessionID, "answer in the final reply")
					thread, err := store.LoadSession(spawned.SessionID)
					if err != nil {
						t.Fatal(err)
					}
					want := storage.PromptConfirmed
					if reject {
						want = storage.PromptSending
					}
					if counted.attempts.Load() != int32(turn+1) || thread.Turn.Prompt.State != want {
						t.Fatalf("confirmation attempts=%d, state=%s; want %d, %s", counted.attempts.Load(), thread.Turn.Prompt.State, turn+1, want)
					}
				}
			})
		}
	}
}

func TestPromptIsNotWrittenWhenSendingCannotBeRecorded(t *testing.T) {
	for _, agent := range []string{acp.AgentCodex, acp.AgentClaude} {
		t.Run(agent, func(t *testing.T) {
			store, root := newTestStore(t)
			counted := &confirmationStore{Store: store, rejectSending: true}
			requestLog := filepath.Join(root, "requests.jsonl")
			manager := newFakeNamedAgentManagerWithOptions(t, counted, root, agent, map[string]string{"JAZ_FAKE_ACP_REQUEST_LOG": requestLog}, "", "")
			t.Cleanup(manager.Close)
			spawned, err := manager.Spawn(t.Context(), acp.SpawnRequest{ACPAgent: agent, Slug: "unsent"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := manager.Send(t.Context(), acp.SendRequest{Session: spawned.SessionID, Message: "Original task to deliver", FromUser: true}); err != nil {
				t.Fatal(err)
			}
			job, err := manager.Wait(t.Context(), acp.WaitRequest{Session: spawned.SessionID})
			if err != nil || job.State != acp.StateFailed || !strings.Contains(job.Error, "sending write unavailable") {
				t.Fatalf("unrecordable send = %+v, %v", job, err)
			}
			requests, err := os.ReadFile(requestLog)
			if err != nil || strings.Contains(string(requests), `"method":"session/prompt"`) {
				t.Fatalf("agent received an unrecorded prompt: %s, %v", requests, err)
			}
		})
	}
}
