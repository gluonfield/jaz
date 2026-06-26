package integrationingest

import (
	"encoding/json"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/internal/emailclean"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func cleanGmailSyncRecords(records []integrations.Record) ([]integrations.Record, error) {
	out := make([]integrations.Record, len(records))
	copy(out, records)
	for i := range out {
		if out[i].Kind != gmailconnector.RecordKindMessage {
			continue
		}
		raw, err := cleanGmailSyncRecordRaw(out[i].Raw)
		if err != nil {
			return nil, err
		}
		out[i].Raw = raw
	}
	return out, nil
}

func cleanGmailSyncRecordRaw(raw json.RawMessage) (json.RawMessage, error) {
	var content gmailconnector.MessageContent
	if err := json.Unmarshal(raw, &content); err != nil {
		return nil, err
	}
	content.Message = cleanGmailSyncMessage(content.Message)
	content.BodyText = emailclean.Body(content.BodyText, content.BodyHTML)
	content.BodyHTML = ""
	return json.Marshal(content)
}

func cleanGmailSyncMessage(message gmailconnector.Message) gmailconnector.Message {
	message.HistoryID = ""
	message.MessageID = ""
	message.References = ""
	message.InReplyTo = ""
	message.Snippet = emailclean.Text(message.Snippet)
	return message
}
