package integrations

import (
	"encoding/json"
	"strings"
	"time"
)

type RecordKind string

type RecordDomain string

const (
	RecordDomainEvents   RecordDomain = "events"
	RecordDomainMessages RecordDomain = "messages"
	RecordDomainContacts RecordDomain = "contacts"
)

func (k RecordKind) Domain() RecordDomain {
	value := strings.ToLower(string(k))
	if recordKindHasSegment(value, "contact") || recordKindHasSegment(value, "contacts") {
		return RecordDomainContacts
	}
	if recordKindHasSegment(value, "message") || recordKindHasSegment(value, "messages") {
		return RecordDomainMessages
	}
	return RecordDomainEvents
}

func recordKindHasSegment(value, segment string) bool {
	for part := range strings.SplitSeq(value, ".") {
		if part == segment {
			return true
		}
	}
	return false
}

type Record struct {
	ID           string          `json:"id"`
	Provider     string          `json:"provider"`
	ConnectionID string          `json:"connection_id"`
	AccountID    string          `json:"account_id"`
	Kind         RecordKind      `json:"kind"`
	ExternalID   string          `json:"external_id"`
	OccurredAt   time.Time       `json:"occurred_at"`
	ReceivedAt   time.Time       `json:"received_at"`
	Raw          json.RawMessage `json:"raw"`
}
