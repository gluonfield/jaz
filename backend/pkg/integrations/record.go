package integrations

import (
	"encoding/json"
	"time"
)

type RecordKind string

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
