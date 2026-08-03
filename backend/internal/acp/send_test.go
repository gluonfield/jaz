package acp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

type rejectingMessageStore struct{ Store }

func (s rejectingMessageStore) AppendMessages(string, ...provider.Message) error {
	return errors.New("message persistence failed")
}

type rejectingStatusStore struct{ Store }

func (s rejectingStatusStore) UpdateSessionStatus(string, string, string, time.Time) error {
	return errors.New("status persistence failed")
}

type recordingLocalRunner struct{ called chan struct{} }

func (r recordingLocalRunner) Run(context.Context, LocalAgentRequest) <-chan agent.StreamEvent {
	close(r.called)
	events := make(chan agent.StreamEvent)
	close(events)
	return events
}

func TestSendDoesNotStartAgentWhenUserMessageCannotPersist(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "persist-before-send", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}

	const localAgent = "local"
	manager := NewManager(rejectingMessageStore{Store: store}, Config{
		Agents: map[string]AgentConfig{localAgent: {Local: true}},
	}, nil)
	runner := recordingLocalRunner{called: make(chan struct{})}
	manager.RegisterLocalAgent(localAgent, runner)
	manager.addJob(newIdleJob(session, localAgent, "runtime-session", "", ModeState{}), nil)

	_, err = manager.Send(t.Context(), SendRequest{Session: session.ID, Message: "keep me"})
	if err == nil || !strings.Contains(err.Error(), "append user message: message persistence failed") {
		t.Fatalf("send error = %v", err)
	}
	select {
	case <-runner.called:
		t.Fatal("agent started without a durable user message")
	default:
	}
	stored, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != storage.StatusIdle {
		t.Fatalf("session status = %q, want idle", stored.Status)
	}
}

func TestSendDoesNotPersistMessageWhenTurnCannotBeReserved(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "status-before-message", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}

	const localAgent = "local"
	manager := NewManager(rejectingStatusStore{Store: store}, Config{
		Agents: map[string]AgentConfig{localAgent: {Local: true}},
	}, nil)
	runner := recordingLocalRunner{called: make(chan struct{})}
	manager.RegisterLocalAgent(localAgent, runner)
	manager.addJob(newIdleJob(session, localAgent, "runtime-session", "", ModeState{}), nil)

	_, err = manager.Send(t.Context(), SendRequest{Session: session.ID, Message: "keep me"})
	if err == nil || !strings.Contains(err.Error(), "mark session running: status persistence failed") {
		t.Fatalf("send error = %v", err)
	}
	select {
	case <-runner.called:
		t.Fatal("agent started without a reserved turn")
	default:
	}
	messages, err := store.LoadMessages(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestReserveSteerDoesNotQueuePromptWhenUserMessageCannotPersist(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "failed-steer-persistence", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(rejectingMessageStore{Store: store}, Config{}, nil)
	job := newIdleJob(session, "fake", "runtime-session", "", ModeState{})
	job.steerMethod = steerPromptQueueing
	job.startTurn(CompletionInline, false, false)

	_, err = manager.reserveSteer(job, SteerRequest{Session: session.ID, Message: "keep me"}, nil)
	if err == nil || !strings.Contains(err.Error(), "append user message: message persistence failed") {
		t.Fatalf("reserve steer error = %v", err)
	}
	if job.turn.promptCalls != 1 {
		t.Fatalf("prompt calls = %d, want original prompt only", job.turn.promptCalls)
	}
	messages, err := store.LoadMessages(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Fatalf("messages = %#v", messages)
	}
}
