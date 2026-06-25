package materialize

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type GmailMaterializer struct{}

func (GmailMaterializer) Materialize(_ context.Context, req integrations.MaterializeRequest) ([]integrations.Artifact, error) {
	if req.Record.Kind != gmailconnector.RecordKindMessage {
		return nil, nil
	}
	var message gmailconnector.Message
	if err := json.Unmarshal(req.Record.Raw, &message); err != nil {
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
		Body:      []byte(gmailMessageMarkdown(req.Connection, req.Record, message, occurred)),
	}}, nil
}

func gmailMessageMarkdown(connection integrations.Connection, record integrations.Record, message gmailconnector.Message, occurred time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s - %s\n\n", occurred.UTC().Format("2006-01-02 15:04"), oneLine(message.Subject))
	fmt.Fprintf(&b, "- Account: `%s`\n", oneLine(connection.AccountRef()))
	fmt.Fprintf(&b, "- Message ID: `%s`\n", oneLine(message.ID))
	if message.ThreadID != "" {
		fmt.Fprintf(&b, "- Thread ID: `%s`\n", oneLine(message.ThreadID))
	}
	if message.HistoryID != "" {
		fmt.Fprintf(&b, "- History ID: `%s`\n", oneLine(message.HistoryID))
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
	if message.Snippet != "" {
		fmt.Fprintf(&b, "\n%s\n", oneLine(message.Snippet))
	}
	if len(message.Attachments) > 0 {
		b.WriteString("\nAttachments:\n")
		for _, attachment := range message.Attachments {
			fmt.Fprintf(&b, "- %s", oneLine(attachment.FileName))
			if attachment.MIMEType != "" {
				fmt.Fprintf(&b, " (%s)", oneLine(attachment.MIMEType))
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
