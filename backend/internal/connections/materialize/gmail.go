package materialize

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/internal/emailclean"
	"github.com/wins/jaz/backend/internal/sourcepaths"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type GmailMaterializer struct{}

func (GmailMaterializer) SourceTargets(_ context.Context, req integrations.MaterializeRequest) ([]integrations.SourceTarget, error) {
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
	if occurred.IsZero() {
		return nil, nil
	}
	account := recordAccountSlug(req.Record.AccountID)
	messageID := firstText(content.Message.ID, req.Record.ExternalID)
	utc := occurred.UTC()
	day := utc.Format("2006-01-02")
	return []integrations.SourceTarget{{
		Provider:  gmailconnector.ProviderID,
		Kind:      "email_message",
		PathHint:  sourcepaths.EmailMessagePath(gmailconnector.ProviderID, account, utc, messageID),
		MediaType: "text/markdown",
		Key:       sourceKey(messageID, day),
		Replay:    sourceReplay(account, integrations.ReplayScope{Domain: integrations.RecordDomainMessages, Day: day}),
	}}, nil
}

func (GmailMaterializer) ProjectSource(_ context.Context, req integrations.SourceProjectionRequest) (integrations.Artifact, error) {
	record, ok, err := gmailTargetRecord(req.Target, req.Records)
	if err != nil || !ok {
		return integrations.Artifact{}, err
	}
	content, err := gmailRecordContent(record.Raw)
	if err != nil {
		return integrations.Artifact{}, err
	}
	occurred := record.OccurredAt
	if occurred.IsZero() {
		occurred = record.ReceivedAt
	}
	return integrations.Artifact{
		Provider:  req.Target.Provider,
		Kind:      req.Target.Kind,
		PathHint:  req.Target.PathHint,
		MediaType: sourceMediaType(req.Target.MediaType),
		Body:      []byte(gmailMessageMarkdown(record, content.Message, emailclean.Body(content.BodyText, content.BodyHTML), occurred)),
	}, nil
}

func gmailTargetRecord(target integrations.SourceTarget, records []integrations.Record) (integrations.Record, bool, error) {
	var best integrations.Record
	var ok bool
	for _, record := range records {
		if record.Kind != gmailconnector.RecordKindMessage {
			continue
		}
		content, err := gmailRecordContent(record.Raw)
		if err != nil {
			return integrations.Record{}, false, err
		}
		messageID := firstText(content.Message.ID, record.ExternalID)
		if messageID != target.Key.Entity {
			continue
		}
		if !ok || record.ReceivedAt.After(best.ReceivedAt) {
			best = record
			ok = true
		}
	}
	return best, ok, nil
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

func gmailMessageMarkdown(record integrations.Record, message gmailconnector.Message, body string, occurred time.Time) string {
	var b strings.Builder
	utc := occurred.UTC()
	fmt.Fprintf(&b, "## %s UTC - %s\n\n", utc.Format("2006-01-02"), oneLine(message.Subject))
	fmt.Fprintf(&b, "- Account: `%s`\n", oneLine(record.AccountID))
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
	writeParticipants(&b, record.AccountID, message)
	if record.ID != "" {
		fmt.Fprintf(&b, "- Raw record: `%s`\n", oneLine(record.ID))
	}
	fmt.Fprintf(&b, "\n%s %s: %s\n", utc.Format("15:04:05"), emailSpeaker(record.AccountID, message.From), oneLine(message.Subject))
	if snippet := emailclean.Text(message.Snippet); snippet != "" {
		fmt.Fprintf(&b, "\n%s\n", oneLine(snippet))
	}
	if body != "" {
		fmt.Fprintf(&b, "\n%s\n", strings.TrimSpace(body))
	}
	if len(message.Attachments) > 0 {
		b.WriteString("\nAttachments:\n")
		for i, attachment := range message.Attachments {
			fmt.Fprintf(&b, "- %s", attachmentLabel(attachment))
			if attachment.MIMEType != "" {
				fmt.Fprintf(&b, " (%s)", oneLine(attachment.MIMEType))
			}
			if attachment.Size > 0 {
				fmt.Fprintf(&b, ", %d bytes", attachment.Size)
			}
			if ref := gmailconnector.FormatAttachmentSourceRef(record.AccountID, message.ID, i+1); ref != "" {
				fmt.Fprintf(&b, ", ref `%s`", ref)
			}
			b.WriteByte('\n')
		}
	}
	b.WriteByte('\n')
	return b.String()
}

func writeParticipants(b *strings.Builder, accountID string, message gmailconnector.Message) {
	participants := participantAddresses(accountID, message)
	if len(participants) == 0 {
		return
	}
	b.WriteString("- Participants:\n")
	for _, participant := range participants {
		fmt.Fprintf(b, "  - %s\n", participant)
	}
}

func participantAddresses(accountID string, message gmailconnector.Message) []string {
	own := strings.ToLower(strings.TrimSpace(accountID))
	seen := map[string]bool{}
	var out []string
	for _, address := range append(append(append([]gmailconnector.Address{}, message.From...), message.To...), message.Cc...) {
		key := strings.ToLower(strings.TrimSpace(address.Email))
		if key == "" {
			key = strings.ToLower(oneLine(address.Name))
		}
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		label := formatAddress(address)
		if own != "" && key == own {
			label = "Me: " + label
		}
		out = append(out, label)
	}
	return out
}

func emailSpeaker(accountID string, from []gmailconnector.Address) string {
	if len(from) == 0 {
		return "Unknown"
	}
	label := formatAddress(from[0])
	if strings.EqualFold(strings.TrimSpace(from[0].Email), strings.TrimSpace(accountID)) {
		return "Me <" + oneLine(from[0].Email) + ">"
	}
	return label
}

func attachmentLabel(attachment gmailconnector.Attachment) string {
	if label := oneLine(attachment.FileName); label != "" {
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

var _ integrations.SourceProjector = GmailMaterializer{}

func (GmailMaterializer) SourceProvider() string { return gmailconnector.ProviderID }
