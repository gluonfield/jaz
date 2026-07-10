package storage

import (
	"encoding/json"
	"testing"
)

func TestNormalizeQueuedMessagesTrims(t *testing.T) {
	var messages []QueuedMessage
	if err := json.Unmarshal([]byte(`[{"text":" first "},{"text":"second","attachment_ids":["a"," b "]}]`), &messages); err != nil {
		t.Fatal(err)
	}
	messages = NormalizeQueuedMessages(messages)
	if len(messages) != 2 || messages[0].Text != "first" || messages[1].Text != "second" {
		t.Fatalf("messages = %#v", messages)
	}
	if got := messages[1].AttachmentIDs; len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("attachment ids = %#v", got)
	}
}

func TestQueuedMessageContextsRoundTripAndTrim(t *testing.T) {
	raw, err := MarshalQueuedMessages([]QueuedMessage{{Text: "ask", Quotes: []string{" keep ", "  "}}})
	if err != nil {
		t.Fatal(err)
	}
	messages, err := UnmarshalQueuedMessages(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages = %#v", messages)
	}
	if got := messages[0].Contexts; len(got) != 1 || got[0].Type != ContextTypeSelection || got[0].Text != "keep" {
		t.Fatalf("contexts = %#v", got)
	}
}

func TestQueuedMessageAllowsContextWithoutText(t *testing.T) {
	raw, err := MarshalQueuedMessages([]QueuedMessage{{Quotes: []string{" keep "}}})
	if err != nil {
		t.Fatal(err)
	}
	messages, err := UnmarshalQueuedMessages(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Text != "" || len(messages[0].Contexts) != 1 {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestQueuedMessageAllowsAttachmentWithoutText(t *testing.T) {
	raw, err := MarshalQueuedMessages([]QueuedMessage{{AttachmentIDs: []string{" file-1 "}}})
	if err != nil {
		t.Fatal(err)
	}
	messages, err := UnmarshalQueuedMessages(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 ||
		messages[0].Text != "" ||
		len(messages[0].AttachmentIDs) != 1 ||
		messages[0].AttachmentIDs[0] != "file-1" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestQueuedActionRoundTrip(t *testing.T) {
	raw, err := MarshalQueuedMessages([]QueuedMessage{{
		Text:          " Archive thread ",
		Action:        QueuedActionArchive,
		Contexts:      []MessageContext{{Type: ContextTypeSelection, Text: "ignored"}},
		AttachmentIDs: []string{"ignored"},
		PlanRequested: true,
		GoalRequested: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	messages, err := UnmarshalQueuedMessages(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages = %#v", messages)
	}
	got := messages[0]
	if got.Kind != QueuedMessageKindPublic || got.Action != QueuedActionArchive || got.Text != "Archive thread" {
		t.Fatalf("action = %#v", got)
	}
	if len(got.Contexts) != 0 || len(got.AttachmentIDs) != 0 || got.PlanRequested || got.GoalRequested {
		t.Fatalf("queued action kept prompt fields: %#v", got)
	}
}

func TestQueuedActionWithoutTextDoesNotInventDisplayText(t *testing.T) {
	raw, err := MarshalQueuedMessages([]QueuedMessage{{
		Action: QueuedActionRepoMergeFromMain,
	}})
	if err != nil {
		t.Fatal(err)
	}
	messages, err := UnmarshalQueuedMessages(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages = %#v", messages)
	}
	if messages[0].Action != QueuedActionRepoMergeFromMain || messages[0].Text != "" {
		t.Fatalf("action = %#v, want action with empty display text", messages[0])
	}
}

func TestQueuedMessageBrowserAnnotationsRoundTripAndTrim(t *testing.T) {
	raw, err := MarshalQueuedMessages([]QueuedMessage{{
		Text: "ask",
		Contexts: []MessageContext{{
			Type: ContextTypeBrowserAnnotation,
			BrowserAnnotation: &BrowserAnnotation{
				URL:              " http://127.0.0.1:3000 ",
				Target:           " headline ",
				Selector:         " main > h1 ",
				RequestedChanges: " tighter ",
				Comment:          " please ",
			},
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	messages, err := UnmarshalQueuedMessages(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || len(messages[0].Contexts) != 1 {
		t.Fatalf("messages = %#v", messages)
	}
	got := messages[0].Contexts[0].BrowserAnnotation
	if got == nil || got.URL != "http://127.0.0.1:3000" || got.Selector != "main > h1" || got.Comment != "please" {
		t.Fatalf("annotation = %#v", got)
	}
}

func TestUnmarshalQueuedMessagesAcceptsLegacyStrings(t *testing.T) {
	messages, err := UnmarshalQueuedMessages(`[" first ",{"text":"second","attachment_ids":["a"," b "]},""]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Text != "first" || messages[1].Text != "second" {
		t.Fatalf("messages = %#v", messages)
	}
	if got := messages[1].AttachmentIDs; len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("attachment ids = %#v", got)
	}
	if messages[0].ID != "legacy-0" || messages[1].ID != "legacy-1" {
		t.Fatalf("legacy ids = %#v", messages)
	}
}

func TestCanonicalSessionQueueAssignsStableLegacyIDs(t *testing.T) {
	session := Session{
		QueuedMessages: []QueuedMessage{
			{Text: "first"},
			{ID: "same", Text: "second"},
			{ID: "same", Text: "third"},
		},
		PendingSteer: &QueuedMessage{Text: "pending"},
	}

	got := CanonicalSessionQueue(session)
	if len(got.QueuedMessages) != 3 {
		t.Fatalf("queued messages = %#v", got.QueuedMessages)
	}
	if got.QueuedMessages[0].ID != "legacy-0" || got.QueuedMessages[1].ID != "same" || got.QueuedMessages[2].ID != "legacy-2" {
		t.Fatalf("queued ids = %#v", got.QueuedMessages)
	}
	if got.PendingSteer == nil || got.PendingSteer.ID != "legacy-0" || got.PendingSteer.Text != "pending" {
		t.Fatalf("pending steer = %#v", got.PendingSteer)
	}
}

func TestUnmarshalQueuedMessagesRejectsInvalidEntries(t *testing.T) {
	if _, err := UnmarshalQueuedMessages(`[{"text":"ok"},1]`); err == nil {
		t.Fatal("expected invalid queued message entry error")
	}
}
