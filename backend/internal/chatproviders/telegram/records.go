package telegram

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gotd/td/tg"
	telegramconnector "github.com/wins/jaz/backend/internal/connectors/telegram"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func telegramConnection(connectionID string, self *tg.User) integrations.Connection {
	accountID := strconv.FormatInt(self.ID, 10)
	accountName := telegramUserName(self)
	return integrations.Connection{
		ID:          connectionID,
		Provider:    telegramconnector.ProviderID,
		AccountID:   accountID,
		AccountName: accountName,
		Alias:       integrations.DefaultAlias(accountName, accountID),
		Scopes:      []string{"contacts", "messages", "send"},
	}
}

func telegramContactRecord(connection integrations.Connection, user *tg.User) integrations.Record {
	raw := rawJSON(map[string]any{
		"id":             user.ID,
		"access_hash":    user.AccessHash,
		"first_name":     user.FirstName,
		"last_name":      user.LastName,
		"username":       user.Username,
		"phone":          user.Phone,
		"bot":            user.Bot,
		"contact":        user.Contact,
		"mutual_contact": user.MutualContact,
	})
	return integrations.Record{
		Provider:     telegramconnector.ProviderID,
		ConnectionID: connection.ID,
		AccountID:    connection.AccountID,
		Kind:         "telegram.contact",
		ExternalID:   "user:" + strconv.FormatInt(user.ID, 10),
		Raw:          raw,
	}
}

func telegramChatRecord(connection integrations.Connection, chat *tg.Chat) integrations.Record {
	raw := rawJSON(map[string]any{
		"id":    chat.ID,
		"title": chat.Title,
		"type":  "chat",
	})
	return integrations.Record{
		Provider:     telegramconnector.ProviderID,
		ConnectionID: connection.ID,
		AccountID:    connection.AccountID,
		Kind:         "telegram.contact",
		ExternalID:   "chat:" + strconv.FormatInt(chat.ID, 10),
		Raw:          raw,
	}
}

func telegramChannelRecord(connection integrations.Connection, channel *tg.Channel) integrations.Record {
	raw := rawJSON(map[string]any{
		"id":          channel.ID,
		"access_hash": channel.AccessHash,
		"title":       channel.Title,
		"username":    channel.Username,
		"broadcast":   channel.Broadcast,
		"megagroup":   channel.Megagroup,
		"type":        "channel",
	})
	return integrations.Record{
		Provider:     telegramconnector.ProviderID,
		ConnectionID: connection.ID,
		AccountID:    connection.AccountID,
		Kind:         "telegram.contact",
		ExternalID:   "channel:" + strconv.FormatInt(channel.ID, 10),
		Raw:          raw,
	}
}

func telegramPeerRecords(connection integrations.Connection, users map[int64]*tg.User, chats map[int64]*tg.Chat, channels map[int64]*tg.Channel) []integrations.Record {
	records := make([]integrations.Record, 0, len(users)+len(chats)+len(channels))
	for _, user := range users {
		records = append(records, telegramContactRecord(connection, user))
	}
	for _, chat := range chats {
		records = append(records, telegramChatRecord(connection, chat))
	}
	for _, channel := range channels {
		records = append(records, telegramChannelRecord(connection, channel))
	}
	return records
}

func telegramMessageRecords(connection integrations.Connection, messages []tg.MessageClass) []integrations.Record {
	records := make([]integrations.Record, 0, len(messages))
	for _, message := range messages {
		switch msg := message.(type) {
		case *tg.Message:
			records = append(records, telegramMessageRecord(connection, msg))
		case *tg.MessageService:
			records = append(records, telegramServiceMessageRecord(connection, msg))
		}
	}
	return records
}

func telegramMessageRecord(connection integrations.Connection, msg *tg.Message) integrations.Record {
	raw := rawJSON(map[string]any{
		"id":      msg.ID,
		"out":     msg.Out,
		"date":    msg.Date,
		"message": msg.Message,
		"from_id": peerID(msg.FromID),
		"peer_id": peerID(msg.PeerID),
		"raw":     msg,
	})
	return integrations.Record{
		Provider:     telegramconnector.ProviderID,
		ConnectionID: connection.ID,
		AccountID:    connection.AccountID,
		Kind:         "telegram.message",
		ExternalID:   telegramMessageExternalID(msg.PeerID, msg.ID),
		OccurredAt:   time.Unix(int64(msg.Date), 0).UTC(),
		Raw:          raw,
	}
}

func telegramServiceMessageRecord(connection integrations.Connection, msg *tg.MessageService) integrations.Record {
	raw := rawJSON(map[string]any{
		"id":      msg.ID,
		"out":     msg.Out,
		"date":    msg.Date,
		"from_id": peerID(msg.FromID),
		"peer_id": peerID(msg.PeerID),
		"kind":    "service",
		"raw":     msg,
	})
	return integrations.Record{
		Provider:     telegramconnector.ProviderID,
		ConnectionID: connection.ID,
		AccountID:    connection.AccountID,
		Kind:         "telegram.message",
		ExternalID:   telegramMessageExternalID(msg.PeerID, msg.ID),
		OccurredAt:   time.Unix(int64(msg.Date), 0).UTC(),
		Raw:          raw,
	}
}

func telegramMessageExternalID(peer tg.PeerClass, id int) string {
	if peerID := peerID(peer); peerID != "" {
		return peerID + ":" + strconv.Itoa(id)
	}
	return "message:" + strconv.Itoa(id)
}

func peerID(peer tg.PeerClass) string {
	switch p := peer.(type) {
	case *tg.PeerUser:
		return "user:" + strconv.FormatInt(p.UserID, 10)
	case *tg.PeerChat:
		return "chat:" + strconv.FormatInt(p.ChatID, 10)
	case *tg.PeerChannel:
		return "channel:" + strconv.FormatInt(p.ChannelID, 10)
	default:
		return ""
	}
}

func peerMaps(users []tg.UserClass, chats []tg.ChatClass) (map[int64]*tg.User, map[int64]*tg.Chat, map[int64]*tg.Channel) {
	userMap := map[int64]*tg.User{}
	chatMap := map[int64]*tg.Chat{}
	channelMap := map[int64]*tg.Channel{}
	for _, item := range users {
		if user, ok := item.(*tg.User); ok {
			userMap[user.ID] = user
		}
	}
	for _, item := range chats {
		switch chat := item.(type) {
		case *tg.Chat:
			chatMap[chat.ID] = chat
		case *tg.Channel:
			channelMap[chat.ID] = chat
		}
	}
	return userMap, chatMap, channelMap
}

func lowestMessageID(messages []tg.MessageClass) int {
	var out int
	for _, message := range messages {
		id := telegramMessageID(message)
		if id <= 0 {
			continue
		}
		if out == 0 || id < out {
			out = id
		}
	}
	return out
}

func messageDate(messages []tg.MessageClass, id int) int {
	for _, message := range messages {
		if telegramMessageID(message) != id {
			continue
		}
		return telegramMessageDate(message)
	}
	return 0
}

func telegramMessageID(message tg.MessageClass) int {
	switch msg := message.(type) {
	case *tg.Message:
		return msg.ID
	case *tg.MessageService:
		return msg.ID
	default:
		return 0
	}
}

func telegramMessageDate(message tg.MessageClass) int {
	switch msg := message.(type) {
	case *tg.Message:
		return msg.Date
	case *tg.MessageService:
		return msg.Date
	default:
		return 0
	}
}

func telegramUserName(user *tg.User) string {
	name := strings.TrimSpace(strings.Join([]string{user.FirstName, user.LastName}, " "))
	if name != "" {
		return name
	}
	if user.Username != "" {
		return "@" + user.Username
	}
	return strconv.FormatInt(user.ID, 10)
}

func rawJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return data
}
