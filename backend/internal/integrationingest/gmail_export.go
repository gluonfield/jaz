package integrationingest

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/internal/emailclean"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type GmailMessageExport struct {
	ID          string                  `json:"id"`
	ThreadID    string                  `json:"thread,omitempty"`
	At          string                  `json:"at,omitempty"`
	Subject     string                  `json:"subject,omitempty"`
	From        []string                `json:"from,omitempty"`
	To          []string                `json:"to,omitempty"`
	Cc          []string                `json:"cc,omitempty"`
	Labels      []string                `json:"labels,omitempty"`
	Snippet     string                  `json:"snippet,omitempty"`
	Body        string                  `json:"body,omitempty"`
	Attachments []GmailAttachmentExport `json:"attachments,omitempty"`
}

type GmailAttachmentExport struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
	Size int64  `json:"size,omitempty"`
}

func (w RawWriter) WriteGmailMessages(ctx context.Context, records []integrations.Record) (int, error) {
	root, err := w.root()
	if err != nil {
		return 0, err
	}
	written := 0
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		record = w.prepare(record)
		path, err := RawRecordPath(root, record)
		if err != nil {
			return written, err
		}
		message, err := gmailMessageExport(record)
		if err != nil {
			return written, err
		}
		if err := appendJSONLine(root, path, message); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}

func gmailMessageExport(record integrations.Record) (GmailMessageExport, error) {
	var content gmailconnector.MessageContent
	if err := json.Unmarshal(record.Raw, &content); err != nil {
		return GmailMessageExport{}, err
	}
	message := content.Message
	body := emailclean.Body(content.BodyText, content.BodyHTML)
	snippet := emailclean.Text(message.Snippet)
	if snippet != "" && strings.Contains(body, snippet) {
		snippet = ""
	}
	at := record.OccurredAt
	if at.IsZero() {
		at = message.InternalDate
	}
	if at.IsZero() {
		at = record.ReceivedAt
	}
	return GmailMessageExport{
		ID:          firstNonEmpty(message.ID, record.ExternalID),
		ThreadID:    message.ThreadID,
		At:          formatExportTime(at),
		Subject:     oneLine(message.Subject),
		From:        exportAddresses(message.From),
		To:          exportAddresses(message.To),
		Cc:          exportAddresses(message.Cc),
		Labels:      message.LabelIDs,
		Snippet:     snippet,
		Body:        body,
		Attachments: exportAttachments(message.Attachments),
	}, nil
}

func exportAddresses(addresses []gmailconnector.Address) []string {
	if len(addresses) == 0 {
		return nil
	}
	out := make([]string, 0, len(addresses))
	for _, address := range addresses {
		formatted := formatExportAddress(address)
		if formatted != "" {
			out = append(out, formatted)
		}
	}
	return out
}

func formatExportAddress(address gmailconnector.Address) string {
	email := oneLine(address.Email)
	name := oneLine(address.Name)
	if name == "" {
		return email
	}
	if email == "" {
		return name
	}
	return name + " <" + email + ">"
}

func exportAttachments(attachments []gmailconnector.Attachment) []GmailAttachmentExport {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]GmailAttachmentExport, 0, len(attachments))
	for _, attachment := range attachments {
		out = append(out, GmailAttachmentExport{
			ID:   attachment.ID,
			Name: oneLine(attachment.FileName),
			Type: oneLine(attachment.MIMEType),
			Size: attachment.Size,
		})
	}
	return out
}

func formatExportTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
