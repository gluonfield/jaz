package integrations

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSourceTargetOmitsEmptyStructuredMetadata(t *testing.T) {
	data, err := json.Marshal(SourceTarget{Provider: "gmail", Kind: "email_message", PathHint: "sources/gmail/a.md"})
	if err != nil {
		t.Fatal(err)
	}
	for _, notWant := range []string{`"key":{}`, `"replay":{}`} {
		if strings.Contains(string(data), notWant) {
			t.Fatalf("source target contains %s: %s", notWant, data)
		}
	}
}

func TestSourceTargetIncludesStructuredMetadata(t *testing.T) {
	data, err := json.Marshal(SourceTarget{
		Provider: "telegram",
		Kind:     "chat_day",
		Key:      SourceKey{Entity: "user:1", Day: "2026-06-27"},
		Replay:   Replay{Account: "personal", Scopes: []ReplayScope{{Domain: RecordDomainMessages, Day: "2026-06-27"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"key":{"entity":"user:1","day":"2026-06-27"}`, `"replay":{"account":"personal","scopes":[{"domain":"messages","day":"2026-06-27"}]}`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("source target missing %s: %s", want, data)
		}
	}
}
