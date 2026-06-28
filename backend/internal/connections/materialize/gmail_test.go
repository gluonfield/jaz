package materialize

import (
	"context"
	"strings"
	"testing"
	"time"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func TestGmailMaterializerCreatesMessageSourceArtifact(t *testing.T) {
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
	artifact := projectOne(t, GmailMaterializer{}, record)
	if !strings.HasPrefix(artifact.PathHint, "sources/gmail/augustinas-example-com/messages/2026/06/25/msg-1-") || !strings.HasSuffix(artifact.PathHint, ".md") || artifact.Kind != "email_message" || artifact.MediaType != "text/markdown" {
		t.Fatalf("artifact = %#v", artifact)
	}
	body := string(artifact.Body)
	for _, want := range []string{
		"## 2026-06-25 UTC - Hello from Gmail",
		"- Message ID: `msg_1`",
		"- Thread ID: `thread_1`",
		"- Labels: INBOX, UNREAD",
		"- From: Friend <friend@example.com>",
		"- Participants:",
		"  - Friend <friend@example.com>",
		"  - Me: augustinas@example.com",
		"09:00:00 Friend <friend@example.com>: Hello from Gmail",
		"This is the visible Gmail snippet.",
		"- plan.pdf (application/pdf), id `att_1`, 1234 bytes",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
}

func projectOne(t *testing.T, projector integrations.SourceProjector, record integrations.Record) integrations.Artifact {
	t.Helper()
	targets, err := projector.SourceTargets(context.Background(), integrations.MaterializeRequest{
		Record: record,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Fatalf("targets = %#v", targets)
	}
	artifact, err := projector.ProjectSource(context.Background(), integrations.SourceProjectionRequest{Target: targets[0], Records: []integrations.Record{record}})
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

func TestGmailMaterializerIgnoresOtherRecordKinds(t *testing.T) {
	targets, err := (GmailMaterializer{}).SourceTargets(context.Background(), integrations.MaterializeRequest{
		Record: integrations.Record{Kind: "calendar.event"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if targets != nil {
		t.Fatalf("targets = %#v", targets)
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
			Snippet:      "Open https://example.com/open-source?utm=1 https://tracker.example.com/open/pixel.png",
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

	artifact := projectOne(t, GmailMaterializer{}, record)
	body := string(artifact.Body)
	for _, want := range []string{
		"Quarterly update",
		"Read report (https://example.com/report)",
		"Open https://example.com/open-source",
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
