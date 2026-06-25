package gmail

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestMaterializerCreatesMonthlyEmailSourceArtifact(t *testing.T) {
	occurred := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)
	record, err := MessageRecord(integrations.Connection{
		ID:          "conn_1",
		AccountID:   "augustinas@example.com",
		AccountName: "augustinas@example.com",
		Alias:       "Personal Gmail",
	}, Message{
		ID:        "msg_1",
		ThreadID:  "thread_1",
		HistoryID: "history_2",
		Subject:   "Hello from Gmail",
		Snippet:   "This is the visible Gmail snippet.",
		From:      []Address{{Name: "Friend", Email: "friend@example.com"}},
		To:        []Address{{Email: "augustinas@example.com"}},
		LabelIDs:  []string{"INBOX", "UNREAD"},
		Attachments: []Attachment{{
			ID:       "att_1",
			FileName: "plan.pdf",
			MIMEType: "application/pdf",
			Size:     1234,
		}},
		InternalDate: occurred,
	}, occurred.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := (Materializer{}).Materialize(context.Background(), integrations.MaterializeRequest{
		Connection: integrations.Connection{Alias: "Personal Gmail", AccountID: "augustinas@example.com"},
		Record:     record,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("artifacts = %#v", artifacts)
	}
	artifact := artifacts[0]
	if artifact.PathHint != "sources/email/gmail/personal-gmail/2026-06.md" || artifact.MediaType != "text/markdown" {
		t.Fatalf("artifact = %#v", artifact)
	}
	body := string(artifact.Body)
	for _, want := range []string{
		"## 2026-06-25 09:00 - Hello from Gmail",
		"- Message ID: `msg_1`",
		"- Thread ID: `thread_1`",
		"- Labels: INBOX, UNREAD",
		"- From: Friend <friend@example.com>",
		"This is the visible Gmail snippet.",
		"- plan.pdf (application/pdf), 1234 bytes",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
}

func TestMaterializerIgnoresOtherRecordKinds(t *testing.T) {
	artifacts, err := (Materializer{}).Materialize(context.Background(), integrations.MaterializeRequest{
		Record: integrations.Record{Kind: "calendar.event"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifacts != nil {
		t.Fatalf("artifacts = %#v", artifacts)
	}
}
