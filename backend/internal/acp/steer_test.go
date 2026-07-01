package acp_test

import (
	"context"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessionevents"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestManagerSteerCancelsPendingQuestion(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeAgentManager(t, store, t.TempDir(), map[string]string{
		"JAZ_FAKE_ACP_PROMPT_QUEUEING":   "1",
		"JAZ_FAKE_ACP_STRICT_ELICIT_HOL": "1",
	})
	manager.Events = sessionevents.New()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "fake-question"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()

	sub := manager.Events.Subscribe(ctx, spawned.SessionID)
	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "ask then block", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	waitForSteerEventType(t, sub, "permission_request")

	if _, err := manager.Steer(ctx, acp.SteerRequest{Session: spawned.SessionID, Message: "say hello"}); err != nil {
		t.Fatal(err)
	}
	job, err := manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if job.State != acp.StateIdle || job.Assistant != "hello from fake agent" {
		t.Fatalf("steered job state=%s stop=%q assistant=%q error=%q", job.State, job.StopReason, job.Assistant, job.Error)
	}
	messages, err := store.LoadMessages(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 ||
		provider.MessageContent(messages[0]) != "ask then block" ||
		provider.MessageContent(messages[1]) != "say hello" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestManagerSteerHandsOffWhenQuestionCancelStopsPrompt(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeAgentManager(t, store, t.TempDir(), map[string]string{
		"JAZ_FAKE_ACP_PROMPT_QUEUEING":    "1",
		"JAZ_FAKE_ACP_ELICIT_CANCEL_STOP": "1",
		"JAZ_FAKE_ACP_STRICT_ELICIT_HOL":  "1",
	})
	manager.Events = sessionevents.New()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "fake-question-cancel"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()

	sub := manager.Events.Subscribe(ctx, spawned.SessionID)
	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "ask then block", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	waitForSteerEventType(t, sub, "permission_request")

	if _, err := manager.Steer(ctx, acp.SteerRequest{Session: spawned.SessionID, Message: "say hello"}); err != nil {
		t.Fatal(err)
	}
	job, err := manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if job.State != acp.StateIdle || job.Assistant != "hello from fake agent" {
		t.Fatalf("steered job state=%s stop=%q assistant=%q error=%q", job.State, job.StopReason, job.Assistant, job.Error)
	}
}

func TestManagerSteerReusesPendingQuestionHandoff(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manager := newFakeAgentManager(t, store, t.TempDir(), map[string]string{
		"JAZ_FAKE_ACP_PROMPT_QUEUEING":   "1",
		"JAZ_FAKE_ACP_STRICT_ELICIT_HOL": "1",
	})
	manager.Events = sessionevents.New()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	spawned, err := manager.Spawn(ctx, acp.SpawnRequest{ACPAgent: "fake", Slug: "fake-question-two-steers"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = manager.Cancel(context.Background(), spawned.SessionID) }()

	sub := manager.Events.Subscribe(ctx, spawned.SessionID)
	if _, err := manager.Send(ctx, acp.SendRequest{Session: spawned.SessionID, Message: "ask then block", Completion: acp.CompletionInline}); err != nil {
		t.Fatal(err)
	}
	waitForSteerEventType(t, sub, "permission_request")

	if _, err := manager.Steer(ctx, acp.SteerRequest{Session: spawned.SessionID, Message: "say hello"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Steer(ctx, acp.SteerRequest{Session: spawned.SessionID, Message: "say hello again"}); err != nil {
		t.Fatal(err)
	}
	job, err := manager.Wait(ctx, acp.WaitRequest{Session: spawned.SessionID, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if job.State != acp.StateIdle || job.Error != "" {
		t.Fatalf("steered job state=%s stop=%q assistant=%q error=%q", job.State, job.StopReason, job.Assistant, job.Error)
	}
	messages, err := store.LoadMessages(spawned.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 ||
		provider.MessageContent(messages[0]) != "ask then block" ||
		provider.MessageContent(messages[1]) != "say hello" ||
		provider.MessageContent(messages[2]) != "say hello again" {
		t.Fatalf("messages = %#v", messages)
	}
}

func waitForSteerEventType(t *testing.T, ch <-chan sessionevents.Event, eventType string) {
	t.Helper()
	for {
		select {
		case event := <-ch:
			if event.Type == eventType {
				return
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for %q event", eventType)
		}
	}
}
