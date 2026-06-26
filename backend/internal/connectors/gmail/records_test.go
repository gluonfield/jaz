package gmail

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestMessageRecordPreservesGmailMessage(t *testing.T) {
	occurred := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)
	received := occurred.Add(time.Minute)
	record, err := MessageRecord(integrations.Connection{
		ID:        "conn_1",
		AccountID: "augustinas@example.com",
	}, Message{
		ID:           "msg_1",
		ThreadID:     "thread_1",
		HistoryID:    "history_2",
		Subject:      "Hello",
		InternalDate: occurred,
		From:         []Address{{Email: "friend@example.com"}},
		Attachments:  []Attachment{{ID: "att_1", FileName: "note.pdf", MIMEType: "application/pdf"}},
	}, received)
	if err != nil {
		t.Fatal(err)
	}
	if record.Provider != ProviderID ||
		record.ConnectionID != "conn_1" ||
		record.AccountID != "augustinas@example.com" ||
		record.Kind != RecordKindMessage ||
		record.ExternalID != "msg_1" ||
		!record.OccurredAt.Equal(occurred) ||
		!record.ReceivedAt.Equal(received) {
		t.Fatalf("record = %#v", record)
	}
	var raw Message
	if err := json.Unmarshal(record.Raw, &raw); err != nil {
		t.Fatal(err)
	}
	if raw.ThreadID != "thread_1" || raw.Attachments[0].FileName != "note.pdf" {
		t.Fatalf("raw = %#v", raw)
	}
}

func TestMessageContentRecordPreservesBody(t *testing.T) {
	occurred := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)
	record, err := MessageContentRecord(integrations.Connection{
		ID:        "conn_1",
		AccountID: "augustinas@example.com",
	}, MessageContent{
		Message: Message{
			ID:           "msg_1",
			ThreadID:     "thread_1",
			Subject:      "Hello",
			InternalDate: occurred,
		},
		BodyText: "full body",
	}, occurred.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	var raw MessageContent
	if err := json.Unmarshal(record.Raw, &raw); err != nil {
		t.Fatal(err)
	}
	if raw.Message.ID != "msg_1" || raw.BodyText != "full body" {
		t.Fatalf("raw = %#v", raw)
	}
}

func TestMessageRecordRejectsMissingID(t *testing.T) {
	_, err := MessageRecord(integrations.Connection{}, Message{}, time.Now())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCursorFromHistoryID(t *testing.T) {
	cursor, err := CursorFromHistoryID("123")
	if err != nil {
		t.Fatal(err)
	}
	if cursor.Kind != CursorKindHistory {
		t.Fatalf("cursor kind = %q", cursor.Kind)
	}
	var value HistoryCursor
	if err := json.Unmarshal(cursor.Value, &value); err != nil {
		t.Fatal(err)
	}
	if value.HistoryID != "123" {
		t.Fatalf("cursor = %#v", value)
	}
}
