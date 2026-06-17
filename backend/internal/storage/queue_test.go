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
}

func TestUnmarshalQueuedMessagesRejectsInvalidEntries(t *testing.T) {
	if _, err := UnmarshalQueuedMessages(`[{"text":"ok"},1]`); err == nil {
		t.Fatal("expected invalid queued message entry error")
	}
}
