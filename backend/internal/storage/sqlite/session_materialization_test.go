package sqlite

import (
	"testing"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestRuntimeSessionMaterializationState(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{
		Slug: "materialization",
		RuntimeRef: &storage.RuntimeRef{
			Type:  storage.RuntimeACP,
			Agent: "codex",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if hasTranscript, err := store.HasAgentTranscript(session.ID); err != nil || hasTranscript {
		t.Fatalf("empty transcript = %t, %v", hasTranscript, err)
	}
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{
		SessionID: session.ID, Type: sessionevents.TypeAgentSession,
		AgentSession: &sessionevents.AgentSession{Auth: &sessionevents.AgentAuthIdentity{Kind: "account", Label: "Signed in"}},
	}); err != nil {
		t.Fatal(err)
	}
	if hasTranscript, err := store.HasAgentTranscript(session.ID); err != nil || hasTranscript {
		t.Fatalf("metadata-only transcript = %t, %v", hasTranscript, err)
	}
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{
		SessionID: session.ID, Type: sessionevents.TypeVoiceMessage,
		Voice: &sessionevents.VoiceMessage{ID: "speech", CallID: "call", Role: "user", Text: "Count the files"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{
		SessionID: session.ID, Type: "voice_diagnostic", Content: `{"id":"connected","call_id":"call"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if hasTranscript, err := store.HasAgentTranscript(session.ID); err != nil || hasTranscript {
		t.Fatalf("voice-only agent transcript = %t, %v", hasTranscript, err)
	}
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{
		SessionID: session.ID, Type: sessionevents.TypeBotActivity,
		BotActivity: &sessionevents.BotActivityEvent{Kind: "message_received", Label: "Research"},
	}); err != nil {
		t.Fatal(err)
	}
	if hasTranscript, err := store.HasAgentTranscript(session.ID); err != nil || hasTranscript {
		t.Fatalf("a thread whose bot was only written to has an agent transcript = %t, %v", hasTranscript, err)
	}
	if updated, err := store.ReplaceRuntimeSessionID(session.ID, "wrong", "new"); err != nil || updated {
		t.Fatalf("mismatched replacement = %t, %v", updated, err)
	}
	if updated, err := store.ReplaceRuntimeSessionID(session.ID, "", "new"); err != nil || !updated {
		t.Fatalf("matched replacement = %t, %v", updated, err)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RuntimeRef.SessionID != "new" {
		t.Fatalf("runtime session = %q", loaded.RuntimeRef.SessionID)
	}
	if err := storage.AppendUserMessage(store, session.ID, "started", nil, nil, false); err != nil {
		t.Fatal(err)
	}
	if hasTranscript, err := store.HasAgentTranscript(session.ID); err != nil || !hasTranscript {
		t.Fatalf("started transcript = %t, %v", hasTranscript, err)
	}
}

func TestAgentSwitchStartsTheTranscriptOver(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "switching", RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Agent: "codex"}})
	if err != nil {
		t.Fatal(err)
	}
	has := func() bool {
		t.Helper()
		hasTranscript, err := store.HasAgentTranscript(session.ID)
		if err != nil {
			t.Fatal(err)
		}
		return hasTranscript
	}
	if err := storage.AppendUserMessage(store, session.ID, "prior turn", nil, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{Type: sessionevents.TypeACPMessage, Content: "prior reply"}); err != nil {
		t.Fatal(err)
	}
	if !has() {
		t.Fatal("history before a switch was not counted")
	}
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{Type: sessionevents.TypeAgentSwitch, Content: "claude"}); err != nil {
		t.Fatal(err)
	}
	if has() {
		t.Fatal("the old agent's history still counts after the switch")
	}
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{Type: sessionevents.TypeACPMessage, Content: "new agent reply"}); err != nil {
		t.Fatal(err)
	}
	if !has() {
		t.Fatal("the new agent's history is not counted")
	}
}
