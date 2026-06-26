package whatsapp

import (
	"encoding/json"
	"fmt"
	"time"

	whatsappconnector "github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"github.com/wins/jaz/backend/pkg/integrations"
	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	waWeb "go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/store"
	waTypes "go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const whatsappHistoricalWindow = 365 * 24 * time.Hour

func connectionFromDevice(device *store.Device) (integrations.Connection, bool) {
	jid := device.GetJID()
	if jid.IsEmpty() {
		return integrations.Connection{}, false
	}
	accountID := jid.User
	if accountID == "" {
		accountID = jid.String()
	}
	accountName := firstNonEmpty(device.PushName, device.BusinessName, accountID)
	return integrations.Connection{
		ID:          whatsappconnector.ProviderID + ":" + integrations.NormalizeAlias(jid.String()),
		Provider:    whatsappconnector.ProviderID,
		AccountID:   accountID,
		AccountName: accountName,
		Alias:       integrations.DefaultAlias(accountName, accountID),
		Scopes:      []string{"contacts", "messages", "send"},
	}, true
}

func whatsappContactRecord(connection integrations.Connection, jid waTypes.JID, contact waTypes.ContactInfo) integrations.Record {
	names := contactNames(contact)
	raw := rawJSON(map[string]any{
		"whatsapp_id":    jid.String(),
		"jid":            jid.String(),
		"phone_number":   jid.User,
		"phone":          jid.User,
		"display_name":   whatsappContactDisplayName(jid, contact, names),
		"contact_names":  names,
		"first_name":     contact.FirstName,
		"full_name":      contact.FullName,
		"push_name":      contact.PushName,
		"business_name":  contact.BusinessName,
		"redacted_phone": contact.RedactedPhone,
	})
	return integrations.Record{
		Provider:     whatsappconnector.ProviderID,
		ConnectionID: connection.ID,
		AccountID:    connection.AccountID,
		Kind:         "whatsapp.contact",
		ExternalID:   jid.String(),
		Raw:          raw,
	}
}

func whatsappContactDisplayName(jid waTypes.JID, contact waTypes.ContactInfo, names []string) string {
	if len(names) > 0 {
		return names[0]
	}
	return firstNonEmpty(contact.RedactedPhone, jid.User, jid.String())
}

func contactNames(contact waTypes.ContactInfo) []string {
	var out []string
	seen := map[string]bool{}
	for _, value := range []string{contact.FullName, contact.PushName, contact.BusinessName, contact.FirstName} {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func whatsappContactActionRecord(connection integrations.Connection, event *events.Contact) integrations.Record {
	rawAction, _ := protojson.Marshal(event.Action)
	raw := rawJSON(map[string]any{
		"whatsapp_id":    event.JID.String(),
		"jid":            event.JID.String(),
		"phone_number":   event.JID.User,
		"phone":          event.JID.User,
		"timestamp":      event.Timestamp,
		"from_full_sync": event.FromFullSync,
		"action":         json.RawMessage(rawAction),
	})
	return integrations.Record{
		Provider:     whatsappconnector.ProviderID,
		ConnectionID: connection.ID,
		AccountID:    connection.AccountID,
		Kind:         "whatsapp.contact",
		ExternalID:   event.JID.String(),
		OccurredAt:   event.Timestamp,
		Raw:          raw,
	}
}

func whatsappMessageRecord(connection integrations.Connection, event *events.Message) integrations.Record {
	message := event.Message
	rawMessage := protoMessageJSON(message)
	raw := rawJSON(map[string]any{
		"id":         string(event.Info.ID),
		"chat":       event.Info.Chat.String(),
		"sender":     event.Info.Sender.String(),
		"from_me":    event.Info.IsFromMe,
		"is_group":   event.Info.IsGroup,
		"timestamp":  event.Info.Timestamp,
		"push_name":  event.Info.PushName,
		"type":       event.Info.Type,
		"media_type": event.Info.MediaType,
		"text":       whatsappText(message),
		"message":    rawMessage,
	})
	return integrations.Record{
		Provider:     whatsappconnector.ProviderID,
		ConnectionID: connection.ID,
		AccountID:    connection.AccountID,
		Kind:         "whatsapp.message",
		ExternalID:   string(event.Info.ID),
		OccurredAt:   event.Info.Timestamp,
		Raw:          raw,
	}
}

func whatsappHistoryRecords(connection integrations.Connection, sync *waHistorySync.HistorySync, cutoff time.Time) []integrations.Record {
	if sync == nil {
		return nil
	}
	records := []integrations.Record{{
		Provider:     whatsappconnector.ProviderID,
		ConnectionID: connection.ID,
		AccountID:    connection.AccountID,
		Kind:         "whatsapp.history",
		ExternalID:   fmt.Sprintf("%s-%d", sync.GetSyncType().String(), sync.GetChunkOrder()),
		Raw: rawJSON(map[string]any{
			"sync_type":            sync.GetSyncType().String(),
			"chunk_order":          sync.GetChunkOrder(),
			"progress":             sync.GetProgress(),
			"conversation_count":   len(sync.GetConversations()),
			"status_message_count": len(sync.GetStatusV3Messages()),
		}),
	}}
	for _, conversation := range sync.GetConversations() {
		for _, msg := range conversation.GetMessages() {
			if record, ok := whatsappWebMessageRecord(connection, conversation.GetID(), msg.GetMessage()); ok && whatsappRecordInWindow(record, cutoff) {
				records = append(records, record)
			}
		}
	}
	for _, msg := range sync.GetStatusV3Messages() {
		if record, ok := whatsappWebMessageRecord(connection, "status", msg); ok && whatsappRecordInWindow(record, cutoff) {
			records = append(records, record)
		}
	}
	return records
}

func whatsappWebMessageRecord(connection integrations.Connection, conversationID string, info *waWeb.WebMessageInfo) (integrations.Record, bool) {
	if info == nil || info.GetKey() == nil {
		return integrations.Record{}, false
	}
	key := info.GetKey()
	occurred := time.Unix(int64(info.GetMessageTimestamp()), 0).UTC()
	if info.GetMessageTimestamp() == 0 {
		occurred = time.Time{}
	}
	externalID := key.GetID()
	if externalID == "" {
		externalID = fmt.Sprintf("%s:%d", conversationID, info.GetMessageTimestamp())
	}
	raw := rawJSON(map[string]any{
		"id":              key.GetID(),
		"conversation":    conversationID,
		"remote_jid":      key.GetRemoteJID(),
		"participant":     firstNonEmpty(key.GetParticipant(), info.GetParticipant()),
		"from_me":         key.GetFromMe(),
		"timestamp":       info.GetMessageTimestamp(),
		"push_name":       info.GetPushName(),
		"text":            whatsappText(info.GetMessage()),
		"web_message":     protoMessageJSON(info),
		"message_payload": protoMessageJSON(info.GetMessage()),
	})
	return integrations.Record{
		Provider:     whatsappconnector.ProviderID,
		ConnectionID: connection.ID,
		AccountID:    connection.AccountID,
		Kind:         "whatsapp.message",
		ExternalID:   externalID,
		OccurredAt:   occurred,
		Raw:          raw,
	}, true
}

func whatsappHistoryCutoff(now time.Time) time.Time {
	return now.UTC().Add(-whatsappHistoricalWindow)
}

func whatsappRecordInWindow(record integrations.Record, cutoff time.Time) bool {
	return cutoff.IsZero() || record.OccurredAt.IsZero() || !record.OccurredAt.Before(cutoff)
}

func whatsappText(message *waE2E.Message) string {
	if message == nil {
		return ""
	}
	if text := message.GetConversation(); text != "" {
		return text
	}
	if extended := message.GetExtendedTextMessage(); extended != nil {
		return extended.GetText()
	}
	return ""
}

func protoMessageJSON(message proto.Message) json.RawMessage {
	if message == nil {
		return nil
	}
	data, err := protojson.Marshal(message)
	if err != nil {
		return nil
	}
	return data
}

func rawJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return data
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
