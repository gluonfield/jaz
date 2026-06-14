package storage

import (
	"encoding/json"
	"testing"
)

func TestQueuedMessageJSONReadsLegacyStrings(t *testing.T) {
	var messages []QueuedMessage
	if err := json.Unmarshal([]byte(`[" first ",{"text":"second","attachment_ids":["a"," b "]}]`), &messages); err != nil {
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
