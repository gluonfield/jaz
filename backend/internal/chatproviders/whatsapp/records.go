package whatsapp

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	whatsappconnector "github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"github.com/wins/jaz/backend/pkg/integrations"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	waWeb "go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/store"
	waTypes "go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	whatsappHistoricalWindow   = 365 * 24 * time.Hour
	whatsappGroupServerAddress = "@g.us"
)

func connectionFromDevice(device *store.Device) (integrations.Connection, bool) {
	jid := device.GetJID()
	if jid.IsEmpty() {
		return integrations.Connection{}, false
	}
	accountID := jid.User
	if accountID == "" {
		accountID = jid.String()
	}
	accountName := firstNonEmpty(device.PushName, device.BusinessName, redactedPhone(accountID))
	return integrations.Connection{
		ID:          whatsappconnector.ProviderID + ":" + integrations.NormalizeAlias(jid.String()),
		Provider:    whatsappconnector.ProviderID,
		AccountID:   accountID,
		AccountName: accountName,
		Alias:       integrations.DefaultAlias(accountName, accountID),
		Scopes:      []string{"contacts", "messages", "send"},
	}, true
}

// account_name renders everywhere as the account's display identity
// (settings rows, confirm dialogs, agent prompts), so a device without a push
// or business name must not leak its raw phone number there.
func redactedPhone(phone string) string {
	if len(phone) <= 4 {
		return "…" + phone
	}
	return "…" + phone[len(phone)-4:]
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

func whatsappMessageRecord(connection integrations.Connection, event *events.Message) integrations.Record {
	message := event.Message
	raw := rawJSON(whatsappconnector.MessageRecord{
		ID:         string(event.Info.ID),
		Chat:       event.Info.Chat.String(),
		Sender:     event.Info.Sender.String(),
		FromMe:     event.Info.IsFromMe,
		IsGroup:    event.Info.IsGroup,
		Timestamp:  rawJSON(event.Info.Timestamp),
		PushName:   event.Info.PushName,
		Type:       event.Info.Type,
		MediaType:  event.Info.MediaType,
		Text:       whatsappconnector.MessageText(message),
		QuotedText: whatsappconnector.MessageQuotedText(message),
		Message:    protoMessageJSON(message),
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
	records, _, _ := whatsappHistoryRecordsLimited(connection, sync, cutoff, nil, 0)
	return records
}

func whatsappHistoryRecordsLimited(connection integrations.Connection, sync *waHistorySync.HistorySync, cutoff time.Time, groupCounts map[string]int, groupLimit int) ([]integrations.Record, []integrations.Record, map[string]int) {
	if sync == nil {
		return nil, nil, groupCounts
	}
	groupCounts = copyGroupCounts(groupCounts)
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
	var contacts []integrations.Record
	contactSeen := map[string]bool{}
	for _, conversation := range sync.GetConversations() {
		conversationID := conversation.GetID()
		for _, msg := range conversation.GetMessages() {
			record, ok := whatsappWebMessageRecord(connection, conversationID, msg.GetMessage())
			if !ok || !whatsappRecordInWindow(record, cutoff) {
				continue
			}
			if whatsappConversationIsGroup(conversationID) {
				if groupLimit > 0 && groupCounts[conversationID] >= groupLimit {
					continue
				}
				groupCounts[conversationID]++
			}
			records = append(records, record)
			if contact, ok := whatsappHistoryConversationContactRecord(connection, conversation); ok && !contactSeen[contact.ExternalID] {
				contactSeen[contact.ExternalID] = true
				contacts = append(contacts, contact)
			}
		}
	}
	for _, msg := range sync.GetStatusV3Messages() {
		if record, ok := whatsappWebMessageRecord(connection, "status", msg); ok && whatsappRecordInWindow(record, cutoff) {
			records = append(records, record)
		}
	}
	return records, contacts, groupCounts
}

func whatsappHistoryConversationContactRecord(connection integrations.Connection, conversation *waHistorySync.Conversation) (integrations.Record, bool) {
	if conversation == nil {
		return integrations.Record{}, false
	}
	jid, err := waTypes.ParseJID(conversation.GetID())
	if err != nil || jid.IsEmpty() {
		return integrations.Record{}, false
	}
	name := firstNonEmpty(conversation.GetName(), conversation.GetDisplayName())
	if name == "" {
		return integrations.Record{}, false
	}
	if jid.Server == waTypes.GroupServer {
		return whatsappGroupRecord(connection, jid, name), true
	}
	switch jid.Server {
	case waTypes.DefaultUserServer, waTypes.HiddenUserServer, waTypes.LegacyUserServer:
		return whatsappContactRecord(connection, jid, waTypes.ContactInfo{FullName: name}), true
	default:
		return integrations.Record{}, false
	}
}

func copyGroupCounts(counts map[string]int) map[string]int {
	out := make(map[string]int, len(counts))
	for key, count := range counts {
		out[key] = count
	}
	return out
}

func whatsappConversationIsGroup(conversationID string) bool {
	return strings.HasSuffix(conversationID, whatsappGroupServerAddress)
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
	message := info.GetMessage()
	raw := rawJSON(whatsappconnector.MessageRecord{
		ID:             key.GetID(),
		Conversation:   conversationID,
		RemoteJID:      key.GetRemoteJID(),
		Participant:    firstNonEmpty(key.GetParticipant(), info.GetParticipant()),
		FromMe:         key.GetFromMe(),
		Timestamp:      rawJSON(info.GetMessageTimestamp()),
		PushName:       info.GetPushName(),
		Text:           whatsappconnector.MessageText(message),
		QuotedText:     whatsappconnector.MessageQuotedText(message),
		WebMessage:     protoMessageJSON(info),
		MessagePayload: protoMessageJSON(message),
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
