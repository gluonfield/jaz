package materialize

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/internal/emailclean"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type GmailMaterializer struct{}

func (GmailMaterializer) Materialize(_ context.Context, req integrations.MaterializeRequest) ([]integrations.Artifact, error) {
	if req.Record.Kind != gmailconnector.RecordKindMessage {
		return nil, nil
	}
	content, err := gmailRecordContent(req.Record.Raw)
	if err != nil {
		return nil, err
	}
	occurred := req.Record.OccurredAt
	if occurred.IsZero() {
		occurred = req.Record.ReceivedAt
	}
	account := integrations.NormalizeAlias(req.Connection.AccountRef())
	if account == "" {
		account = "unknown"
	}
	return []integrations.Artifact{{
		Kind:      "memory_source",
		PathHint:  path.Join("sources/email/gmail", account, occurred.UTC().Format("2006-01")+".md"),
		MediaType: "text/markdown",
		Body:      []byte(gmailMessageMarkdown(req.Connection, req.Record, content.Message, emailclean.Body(content.BodyText, content.BodyHTML), occurred)),
	}}, nil
}

func gmailRecordContent(raw json.RawMessage) (gmailconnector.MessageContent, error) {
	var message gmailconnector.Message
	if err := json.Unmarshal(raw, &message); err != nil {
		return gmailconnector.MessageContent{}, err
	}
	if message.ID != "" {
		return gmailconnector.MessageContent{Message: message}, nil
	}
	var content gmailconnector.MessageContent
	if err := json.Unmarshal(raw, &content); err != nil {
		return gmailconnector.MessageContent{}, err
	}
	return content, nil
}

func gmailMessageMarkdown(connection integrations.Connection, record integrations.Record, message gmailconnector.Message, body string, occurred time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s - %s\n\n", occurred.UTC().Format("2006-01-02 15:04"), oneLine(message.Subject))
	fmt.Fprintf(&b, "- Account: `%s`\n", oneLine(connection.AccountRef()))
	fmt.Fprintf(&b, "- Message ID: `%s`\n", oneLine(message.ID))
	if message.ThreadID != "" {
		fmt.Fprintf(&b, "- Thread ID: `%s`\n", oneLine(message.ThreadID))
	}
	if len(message.LabelIDs) > 0 {
		fmt.Fprintf(&b, "- Labels: %s\n", strings.Join(message.LabelIDs, ", "))
	}
	writeAddresses(&b, "From", message.From)
	writeAddresses(&b, "To", message.To)
	writeAddresses(&b, "Cc", message.Cc)
	if record.ID != "" {
		fmt.Fprintf(&b, "- Raw record: `%s`\n", oneLine(record.ID))
	}
	if snippet := emailclean.Text(message.Snippet); snippet != "" {
		fmt.Fprintf(&b, "\n%s\n", oneLine(snippet))
	}
	if body != "" {
		fmt.Fprintf(&b, "\n%s\n", strings.TrimSpace(body))
	}
	if len(message.Attachments) > 0 {
		b.WriteString("\nAttachments:\n")
		for _, attachment := range message.Attachments {
			fmt.Fprintf(&b, "- %s", attachmentLabel(attachment))
			if attachment.MIMEType != "" {
				fmt.Fprintf(&b, " (%s)", oneLine(attachment.MIMEType))
			}
			if attachment.ID != "" {
				fmt.Fprintf(&b, ", id `%s`", oneLine(attachment.ID))
			}
			if attachment.Size > 0 {
				fmt.Fprintf(&b, ", %d bytes", attachment.Size)
			}
			b.WriteByte('\n')
		}
	}
	b.WriteByte('\n')
	return b.String()
}

func attachmentLabel(attachment gmailconnector.Attachment) string {
	if label := oneLine(attachment.FileName); label != "" {
		return label
	}
	if label := oneLine(attachment.ID); label != "" {
		return label
	}
	return "attachment"
}

func writeAddresses(b *strings.Builder, label string, addresses []gmailconnector.Address) {
	if len(addresses) == 0 {
		return
	}
	values := make([]string, 0, len(addresses))
	for _, address := range addresses {
		values = append(values, formatAddress(address))
	}
	fmt.Fprintf(b, "- %s: %s\n", label, strings.Join(values, ", "))
}

func formatAddress(address gmailconnector.Address) string {
	email := oneLine(address.Email)
	if address.Name == "" {
		return email
	}
	return oneLine(address.Name) + " <" + email + ">"
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
