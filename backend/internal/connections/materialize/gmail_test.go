package materialize

import (
	"context"
	"strings"
	"testing"
	"time"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestGmailMaterializerCreatesMonthlyEmailSourceArtifact(t *testing.T) {
	occurred := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)
	record, err := gmailconnector.MessageRecord(integrations.Connection{
		ID:          "conn_1",
		AccountID:   "augustinas@example.com",
		AccountName: "augustinas@example.com",
		Alias:       "Personal Gmail",
	}, gmailconnector.Message{
		ID:        "msg_1",
		ThreadID:  "thread_1",
		HistoryID: "history_2",
		Subject:   "Hello from Gmail",
		Snippet:   "This is the visible Gmail snippet.",
		From:      []gmailconnector.Address{{Name: "Friend", Email: "friend@example.com"}},
		To:        []gmailconnector.Address{{Email: "augustinas@example.com"}},
		LabelIDs:  []string{"INBOX", "UNREAD"},
		Attachments: []gmailconnector.Attachment{{
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
	artifacts, err := (GmailMaterializer{}).Materialize(context.Background(), integrations.MaterializeRequest{
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

func TestGmailMaterializerIgnoresOtherRecordKinds(t *testing.T) {
	artifacts, err := (GmailMaterializer{}).Materialize(context.Background(), integrations.MaterializeRequest{
		Record: integrations.Record{Kind: "calendar.event"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifacts != nil {
		t.Fatalf("artifacts = %#v", artifacts)
	}
}

func TestGmailMaterializerCleansHTMLBodiesForMarkdown(t *testing.T) {
	occurred := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)
	longTrackingURL := "https://tracker.example.com/open/" + strings.Repeat("a", 260) + ".png?utm_source=newsletter"
	record, err := gmailconnector.MessageContentRecord(integrations.Connection{
		ID:        "conn_1",
		AccountID: "augustinas@example.com",
		Alias:     "Personal Gmail",
	}, gmailconnector.MessageContent{
		Message: gmailconnector.Message{
			ID:           "msg_1",
			Subject:      "HTML mail",
			InternalDate: occurred,
		},
		BodyHTML: `<html><body>
			<style>.x{display:none}</style>
			<p>Quarterly update</p>
			<a href="https://example.com/report?utm_source=newsletter&really=` + strings.Repeat("b", 220) + `">Read report</a>
			<img src="` + longTrackingURL + `">
			<script>alert("x")</script>
		</body></html>`,
	}, occurred)
	if err != nil {
		t.Fatal(err)
	}

	artifacts, err := (GmailMaterializer{}).Materialize(context.Background(), integrations.MaterializeRequest{
		Connection: integrations.Connection{Alias: "Personal Gmail", AccountID: "augustinas@example.com"},
		Record:     record,
	})
	if err != nil {
		t.Fatal(err)
	}

	body := string(artifacts[0].Body)
	for _, want := range []string{
		"Quarterly update",
		"Read report (https://example.com/report)",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	for _, unwanted := range []string{"tracker.example.com", "<img", "utm_source", strings.Repeat("a", 80), "alert"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("body contains %q:\n%s", unwanted, body)
		}
	}
}
