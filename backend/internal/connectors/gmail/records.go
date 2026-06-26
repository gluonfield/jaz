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
	MessageID    string       `json:"message_id,omitempty"`
	References   string       `json:"references,omitempty"`
	InReplyTo    string       `json:"in_reply_to,omitempty"`
	Subject      string       `json:"subject,omitempty"`
	Snippet      string       `json:"snippet,omitempty"`
	From         []Address    `json:"from,omitempty"`
	ReplyTo      []Address    `json:"reply_to,omitempty"`
	To           []Address    `json:"to,omitempty"`
	Cc           []Address    `json:"cc,omitempty"`
	Bcc          []Address    `json:"bcc,omitempty"`
	LabelIDs     []string     `json:"label_ids,omitempty"`
	InternalDate time.Time    `json:"internal_date,omitempty"`
	Attachments  []Attachment `json:"attachments,omitempty"`
}

type Thread struct {
	ID        string    `json:"id"`
	HistoryID string    `json:"history_id,omitempty"`
	Snippet   string    `json:"snippet,omitempty"`
	Messages  []Message `json:"messages,omitempty"`
}

type Draft struct {
	ID      string  `json:"id"`
	Message Message `json:"message"`
}

type HistoryCursor struct {
	HistoryID string `json:"history_id"`
}

func MessageRecord(connection integrations.Connection, message Message, receivedAt time.Time) (integrations.Record, error) {
	return messageRawRecord(connection, message.ID, message.InternalDate, receivedAt, message)
}

func MessageContentRecord(connection integrations.Connection, content MessageContent, receivedAt time.Time) (integrations.Record, error) {
	return messageRawRecord(connection, content.Message.ID, content.Message.InternalDate, receivedAt, content)
}

func messageRawRecord(connection integrations.Connection, id string, occurredAt, receivedAt time.Time, rawValue any) (integrations.Record, error) {
	if id == "" {
		return integrations.Record{}, errors.New("gmail message id is required")
	}
	raw, err := json.Marshal(rawValue)
	if err != nil {
		return integrations.Record{}, err
	}
	occurred := occurredAt
	if occurred.IsZero() {
		occurred = receivedAt
	}
	return integrations.Record{
		Provider:     ProviderID,
		ConnectionID: connection.ID,
		AccountID:    connection.AccountID,
		Kind:         RecordKindMessage,
		ExternalID:   id,
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
