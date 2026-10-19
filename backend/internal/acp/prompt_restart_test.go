package acp_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

// A separate process stops at a durable boundary without letting the sender
// finish its next step, as a process killed during startup does.
type restartBoundaryStore struct {
	acp.Store
	stage string
	root  string
}

func (s restartBoundaryStore) StartSessionTurn(id string, turn storage.Turn, messages ...storage.Message) error {
	err := s.Store.StartSessionTurn(id, turn, messages...)
	if err == nil && s.stage == "turn" {
		if err := os.WriteFile(filepath.Join(s.root, "thread"), []byte(id), 0o600); err != nil {
			return err
		}
		os.Exit(42)
	}
	return err
}

func (s restartBoundaryStore) ClaimQueuedTurn(session storage.Session, prompt storage.QueuedMessage) error {
	err := s.Store.ClaimQueuedTurn(session, prompt)
	if err == nil && s.stage == "claim" {
		if err := os.WriteFile(filepath.Join(s.root, "thread"), []byte(session.ID), 0o600); err != nil {
			return err
		}
		os.Exit(42)
	}
	return err
}

func TestPromptRestartProcess(t *testing.T) {
	root := os.Getenv("JAZ_PROMPT_RESTART_ROOT")
	if root == "" {
		return
	}
	store, err := sqlitestore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	agent := os.Getenv("JAZ_PROMPT_RESTART_AGENT")
	stage := os.Getenv("JAZ_PROMPT_RESTART_STAGE")
	boundary := restartBoundaryStore{Store: store, stage: stage, root: root}
	env := map[string]string{
		"JAZ_FAKE_ACP_REQUEST_LOG":  filepath.Join(root, "requests.jsonl"),
		"JAZ_FAKE_ACP_START_LOG":    filepath.Join(root, "pids"),
		"JAZ_FAKE_ACP_BLOCK_PROMPT": "1",
	}
	if stage == "start" || stage == "new" {
		hold := filepath.Join(root, "hold")
		if err := os.WriteFile(hold, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if stage == "start" {
			env["JAZ_FAKE_ACP_START_HOLD"] = hold
		} else {
			env["JAZ_FAKE_ACP_NEW_HOLD"] = hold
		}
	}
	manager := newFakeNamedAgentManagerWithOptions(t, boundary, root, agent, env, "", "")
	kind := os.Getenv("JAZ_PROMPT_RESTART_KIND")
	var id string
	if kind == "worker" {
		parent, err := store.CreateSession(storage.CreateSession{Slug: "parent", SourceType: storage.SourceBot})
		if err != nil {
			t.Fatal(err)
		}
		job, err := manager.Start(t.Context(), acp.SpawnRequest{ACPAgent: agent, Slug: "worker", ParentID: parent.ID, SourceType: storage.SourceBotWorker, SourceID: parent.ID, Directory: root, Commitment: owe(t, store, parent.ID)}, storage.QueuedMessage{Text: "Original task to deliver", From: parent.ID})
		if err != nil {
			t.Fatal(err)
		}
		id = job.ID
	} else {
		session, err := manager.CreateSession(t.Context(), acp.SpawnRequest{ACPAgent: agent, Slug: "restart", Directory: root})
		if err != nil {
			t.Fatal(err)
		}
		id = session.ID
		prompt := storage.QueuedMessage{Text: "Original task to deliver", SentAt: time.Now(), Contexts: []storage.MessageContext{{Type: storage.ContextTypeSelection, Text: "quoted specification"}}}
		if kind == "result" {
			prompt = storage.QueuedMessage{Text: "Original result to deliver", Kind: storage.QueuedMessageKindInternal, Output: &storage.TurnOutput{ReplyTo: "asker", Hops: 2}}
		}
		if err := boundary.ClaimQueuedTurn(session, prompt); err != nil {
			t.Fatal(err)
		}
		handler := &server.Server{Store: store, ACP: manager, Locks: sessionlock.New(), Events: sessionevents.New()}
		go func() {
			if err := handler.ResumeInterruptedTurn(t.Context(), id); err != nil {
				t.Error(err)
			}
		}()
	}
	if err := os.WriteFile(filepath.Join(root, "thread"), []byte(id), 0o600); err != nil {
		t.Fatal(err)
	}
	if stage == "start" || stage == "new" {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			pids, _ := os.ReadFile(filepath.Join(root, "pids"))
			requests, _ := os.ReadFile(filepath.Join(root, "requests.jsonl"))
			if stage == "start" && len(pids) > 0 || stage == "new" && strings.Contains(string(requests), `"method":"session/new"`) {
				os.Exit(42)
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("agent never reached the held startup boundary")
	}
	waitForBlockingTool(t, testContext(t), manager, id)
	manager.Close()
	os.Exit(42)
}

func TestRestartDeliversAnUnsentPromptAndContinuesADeliveredOne(t *testing.T) {
	for _, agent := range []string{acp.AgentCodex, acp.AgentClaude} {
		for _, kind := range []string{"worker", "user", "result"} {
			for _, stage := range []string{"claim", "start", "new", "turn", "delivered"} {
				t.Run(agent+"/"+kind+"/"+stage, func(t *testing.T) {
					root := t.TempDir()
					executable, err := os.Executable()
					if err != nil {
						t.Fatal(err)
					}
					cmd := exec.CommandContext(t.Context(), executable, "-test.run=^TestPromptRestartProcess$")
					cmd.Env = append(os.Environ(), "TMPDIR="+root, "JAZ_PROMPT_RESTART_ROOT="+root, "JAZ_PROMPT_RESTART_AGENT="+agent, "JAZ_PROMPT_RESTART_KIND="+kind, "JAZ_PROMPT_RESTART_STAGE="+stage)
					output, err := cmd.CombinedOutput()
					// A crashed host can leave a fake waiting in startup. Reap only
					// the PIDs this helper recorded, before removing its files.
					pids, _ := os.ReadFile(filepath.Join(root, "pids"))
					for _, value := range strings.Fields(string(pids)) {
						pid, err := strconv.Atoi(value)
						if err != nil || pid <= 0 {
							t.Fatalf("invalid helper pid %q", value)
						}
						process, _ := os.FindProcess(pid)
						_ = process.Kill()
					}
					var exited *exec.ExitError
					if !errors.As(err, &exited) || exited.ExitCode() != 42 {
						t.Fatalf("boundary process = %v: %s", err, output)
					}
					id, err := os.ReadFile(filepath.Join(root, "thread"))
					if err != nil {
						t.Fatal(err)
					}
					store, err := sqlitestore.New(root)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = store.Close() })
					manager := newFakeNamedAgentManagerWithOptions(t, store, root, agent, map[string]string{
						"JAZ_FAKE_ACP_RESUME": "1", "JAZ_FAKE_ACP_REQUEST_LOG": filepath.Join(root, "resumed.jsonl"),
					}, "", "")
					t.Cleanup(manager.Close)
					locks, events := sessionlock.New(), sessionevents.New()
					handler := &server.Server{Store: store, ACP: manager, Locks: locks, Events: events}
					completed := make(chan storage.Session, 1)
					manager.TurnFinished = func(_ context.Context, job acp.Job) {
						thread, err := store.LoadSession(job.ID)
						if err != nil {
							t.Error(err)
						}
						completed <- thread
						if err := store.FinishSessionTurn(job.ID, storage.StatusIdle, "", time.Now(), nil); err != nil {
							t.Error(err)
						}
					}
					ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
					defer cancel()
					if err := sessionrecovery.Resume(ctx, store, handler, locks, events, log.New(os.Stderr)); err != nil {
						t.Fatal(err)
					}
					// A second recovery sweep must not send the same claimed input again.
					if err := sessionrecovery.Resume(ctx, store, handler, locks, events, log.New(os.Stderr)); err != nil {
						t.Fatal(err)
					}
					job, err := manager.Wait(ctx, acp.WaitRequest{Session: string(id)})
					if err != nil || job.State != acp.StateIdle {
						stored, _ := store.LoadSession(string(id))
						t.Fatalf("recovered job = %+v, %v; stored = %+v", job, err, stored)
					}
					thread := <-completed
					if thread.Turn == nil || thread.Turn.Prompt != nil {
						t.Fatalf("sent prompt remained pending: %+v", thread.Turn)
					}
					switch kind {
					case "worker":
						if thread.ParentID == "" || !thread.Turn.NotifyParent || !thread.Turn.ParentVisible {
							t.Fatalf("worker lost its result destination: %+v", thread)
						}
						commitments, err := store.ListOpenCommitments(thread.ParentID)
						if err != nil || len(commitments) != 1 || len(commitments[0].Threads) != 1 || commitments[0].Threads[0].ID != thread.ID {
							t.Fatalf("worker lost its commitment: %+v, %v", commitments, err)
						}
					case "user":
						if thread.Turn.Output == nil || !thread.Turn.Output.User {
							t.Fatalf("user turn lost its output: %+v", thread.Turn)
						}
					case "result":
						if !thread.Turn.AllowSilence || thread.Turn.Output == nil || thread.Turn.Output.ReplyTo != "asker" || thread.Turn.Output.Hops != 2 {
							t.Fatalf("result turn lost its routing: %+v", thread.Turn)
						}
					}
					requests, err := os.ReadFile(filepath.Join(root, "resumed.jsonl"))
					if err != nil {
						t.Fatal(err)
					}
					want := "Original task to deliver"
					if kind == "result" {
						want = "Original result to deliver"
					}
					if stage == "delivered" {
						want = "Continue from where you left off."
					}
					messages, err := store.LoadMessageRecords(string(id))
					if err != nil {
						t.Fatal(err)
					}
					users := 0
					for _, message := range messages {
						if message.Role == "user" {
							users++
							if message.Content != "Original task to deliver" || message.FromUser != (kind == "user") {
								t.Fatalf("altered user message: %+v", message)
							}
						}
					}
					wantUsers := 1
					if kind == "result" {
						wantUsers = 0
					}
					if users != wantUsers {
						t.Fatalf("user messages = %d, want %d", users, wantUsers)
					}
					if strings.Count(string(requests), `"method":"session/prompt"`) != 1 || strings.Count(string(requests), want) != 1 {
						t.Fatalf("restart lost or duplicated %q: %s", want, requests)
					}
					before, _ := os.ReadFile(filepath.Join(root, "requests.jsonl"))
					all := string(before) + string(requests)
					if strings.Count(all, "Original task to deliver")+strings.Count(all, "Original result to deliver") != 1 {
						t.Fatalf("original prompt was lost or repeated across the restart: %s", all)
					}
					if kind == "user" && strings.Count(all, "quoted specification") != 1 {
						t.Fatalf("user context was lost or repeated: %s", all)
					}
				})
			}
		}
	}
}
