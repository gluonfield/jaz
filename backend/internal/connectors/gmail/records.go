package gmail

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/wins/jaz/backend/pkg/integrations"
)

const (
	RecordKindMessage integrations.RecordKind = "gmail.message"
	CursorKindHistory string                  = "gmail.history"
)

type Address struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

type Attachment struct {
	ID       string `json:"id,omitempty"`
	FileName string `json:"file_name"`
	MIMEType string `json:"mime_type,omitempty"`
	Size     int64  `json:"size,omitempty"`
	Inline   bool   `json:"inline,omitempty"`
}

type Message struct {
	ID           string       `json:"id"`
	ThreadID     string       `json:"thread_id,omitempty"`
	HistoryID    string       `json:"history_id,omitempty"`
	Subject      string       `json:"subject,omitempty"`
	Snippet      string       `json:"snippet,omitempty"`
	From         []Address    `json:"from,omitempty"`
	To           []Address    `json:"to,omitempty"`
	Cc           []Address    `json:"cc,omitempty"`
	Bcc          []Address    `json:"bcc,omitempty"`
	LabelIDs     []string     `json:"label_ids,omitempty"`
	InternalDate time.Time    `json:"internal_date,omitempty"`
	Attachments  []Attachment `json:"attachments,omitempty"`
}

type HistoryCursor struct {
	HistoryID string `json:"history_id"`
}

func MessageRecord(connection integrations.Connection, message Message, receivedAt time.Time) (integrations.Record, error) {
	if message.ID == "" {
		return integrations.Record{}, errors.New("gmail message id is required")
	}
	raw, err := json.Marshal(message)
	if err != nil {
		return integrations.Record{}, err
	}
	occurred := message.InternalDate
	if occurred.IsZero() {
		occurred = receivedAt
	}
	return integrations.Record{
		Provider:     ProviderID,
		ConnectionID: connection.ID,
		AccountID:    connection.AccountID,
		Kind:         RecordKindMessage,
		ExternalID:   message.ID,
		OccurredAt:   occurred,
		ReceivedAt:   receivedAt,
		Raw:          raw,
	}, nil
}

func CursorFromHistoryID(historyID string) (integrations.Cursor, error) {
	value, err := json.Marshal(HistoryCursor{HistoryID: historyID})
	if err != nil {
		return integrations.Cursor{}, err
	}
	return integrations.Cursor{Kind: CursorKindHistory, Value: value}, nil
}
