package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	whatsappconnector "github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"github.com/wins/jaz/backend/pkg/integrations"
	"go.mau.fi/whatsmeow"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	waTypes "go.mau.fi/whatsmeow/types"
)

const whatsappReadRecentTimeout = 5 * time.Second
const whatsappReadRecentCacheLimit = 500

func (p *Provider) ReadRecent(ctx context.Context, req whatsappconnector.ReadRecentRequest) (whatsappconnector.ReadRecentResult, error) {
	client, err := p.clientForConnection(ctx, req.Connection)
	if err != nil {
		return whatsappconnector.ReadRecentResult{}, err
	}
	if !client.IsConnected() {
		if err := client.Connect(); err != nil {
			return whatsappconnector.ReadRecentResult{}, err
		}
	}
	chat, err := recipientJID(req.Chat)
	if err != nil {
		return whatsappconnector.ReadRecentResult{}, err
	}
	limit := whatsappconnector.ReadRecentLimit(req.Limit)
	messages := p.cachedRecentMessages(chat.String(), limit)
	if len(messages) >= limit {
		return whatsappconnector.ReadRecentResult{Chat: chat.String(), Messages: messages}, nil
	}
	if len(messages) > 0 {
		_ = p.requestOlderHistory(ctx, client, chat, messages[0], limit-len(messages))
		messages = p.cachedRecentMessages(chat.String(), limit)
	}
	if len(messages) == 0 {
		return whatsappconnector.ReadRecentResult{}, errors.New("no live WhatsApp messages are cached for this chat yet; keep the WhatsApp connection running until messages or history sync arrive")
	}
	return whatsappconnector.ReadRecentResult{Chat: chat.String(), Messages: messages}, nil
}

func (p *Provider) requestOlderHistory(ctx context.Context, client *whatsmeow.Client, chat waTypes.JID, oldest whatsappconnector.ReadRecentMessage, count int) error {
	if oldest.MessageID == "" || oldest.SentAt.IsZero() || count <= 0 {
		return nil
	}
	waiter := p.addHistoryWaiter(chat.String())
	defer p.removeHistoryWaiter(chat.String(), waiter)
	anchor := &waTypes.MessageInfo{
		MessageSource: waTypes.MessageSource{Chat: chat, IsFromMe: oldest.FromMe},
		ID:            waTypes.MessageID(oldest.MessageID),
		Timestamp:     oldest.SentAt,
	}
	if _, err := client.SendPeerMessage(ctx, client.BuildHistorySyncRequest(anchor, count)); err != nil {
		return err
	}
	waitCtx, cancel := context.WithTimeout(ctx, whatsappReadRecentTimeout)
	defer cancel()
	select {
	case <-waiter:
		return nil
	case <-waitCtx.Done():
		return waitCtx.Err()
	}
}

func (p *Provider) addHistoryWaiter(chat string) chan []whatsappconnector.ReadRecentMessage {
	waiter := make(chan []whatsappconnector.ReadRecentMessage, 1)
	p.historyMu.Lock()
	if p.historyWaiters == nil {
		p.historyWaiters = map[string][]chan []whatsappconnector.ReadRecentMessage{}
	}
	p.historyWaiters[chat] = append(p.historyWaiters[chat], waiter)
	p.historyMu.Unlock()
	return waiter
}

func (p *Provider) removeHistoryWaiter(chat string, waiter chan []whatsappconnector.ReadRecentMessage) {
	p.historyMu.Lock()
	defer p.historyMu.Unlock()
	waiters := p.historyWaiters[chat]
	for i, candidate := range waiters {
		if candidate == waiter {
			waiters = append(waiters[:i], waiters[i+1:]...)
			break
		}
	}
	if len(waiters) == 0 {
		delete(p.historyWaiters, chat)
		return
	}
	p.historyWaiters[chat] = waiters
}

func (p *Provider) publishHistorySync(sync *waHistorySync.HistorySync) {
	if sync == nil {
		return
	}
	messagesByChat := whatsappHistoryMessagesByChat(sync)
	if len(messagesByChat) == 0 {
		return
	}
	for chat, messages := range messagesByChat {
		p.storeLiveMessages(chat, messages)
	}
	p.historyMu.Lock()
	defer p.historyMu.Unlock()
	for chat, messages := range messagesByChat {
		waiters := p.historyWaiters[chat]
		for _, waiter := range waiters {
			select {
			case waiter <- messages:
			default:
			}
		}
	}
}

func (p *Provider) storeLiveMessage(record integrations.Record) {
	message, ok, err := decodeWhatsAppRecentMessage(record)
	if err != nil || !ok {
		return
	}
	var raw whatsappconnector.MessageRecord
	if err := json.Unmarshal(record.Raw, &raw); err != nil {
		return
	}
	chat := raw.ConversationID(record.ExternalID)
	if chat == "" {
		return
	}
	p.storeLiveMessages(chat, []whatsappconnector.ReadRecentMessage{message})
}

func (p *Provider) storeLiveMessages(chat string, messages []whatsappconnector.ReadRecentMessage) {
	if chat == "" || len(messages) == 0 {
		return
	}
	p.recentMu.Lock()
	defer p.recentMu.Unlock()
	if p.recentMessages == nil {
		p.recentMessages = map[string][]whatsappconnector.ReadRecentMessage{}
	}
	merged := append([]whatsappconnector.ReadRecentMessage{}, p.recentMessages[chat]...)
	seen := make(map[string]bool, len(merged)+len(messages))
	for _, message := range merged {
		if message.MessageID != "" {
			seen[message.MessageID] = true
		}
	}
	for _, message := range messages {
		if message.MessageID != "" && seen[message.MessageID] {
			continue
		}
		merged = append(merged, message)
		if message.MessageID != "" {
			seen[message.MessageID] = true
		}
	}
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].SentAt.Before(merged[j].SentAt)
	})
	if len(merged) > whatsappReadRecentCacheLimit {
		merged = merged[len(merged)-whatsappReadRecentCacheLimit:]
	}
	p.recentMessages[chat] = merged
}

func (p *Provider) cachedRecentMessages(chat string, limit int) []whatsappconnector.ReadRecentMessage {
	p.recentMu.Lock()
	defer p.recentMu.Unlock()
	messages := p.recentMessages[chat]
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	return append([]whatsappconnector.ReadRecentMessage{}, messages...)
}

func whatsappHistoryMessagesByChat(sync *waHistorySync.HistorySync) map[string][]whatsappconnector.ReadRecentMessage {
	out := map[string][]whatsappconnector.ReadRecentMessage{}
	for _, conversation := range sync.GetConversations() {
		chat := conversation.GetID()
		if chat == "" {
			continue
		}
		for _, msg := range conversation.GetMessages() {
			record, ok := whatsappWebMessageRecord(integrations.Connection{}, chat, msg.GetMessage())
			if !ok {
				continue
			}
			message, ok, err := decodeWhatsAppRecentMessage(record)
			if err != nil || !ok {
				continue
			}
			out[chat] = append(out[chat], message)
		}
	}
	for chat, messages := range out {
		sort.Slice(messages, func(i, j int) bool {
			return messages[i].SentAt.Before(messages[j].SentAt)
		})
		out[chat] = messages
	}
	return out
}

func decodeWhatsAppRecentMessage(record integrations.Record) (whatsappconnector.ReadRecentMessage, bool, error) {
	if record.Kind != "whatsapp.message" {
		return whatsappconnector.ReadRecentMessage{}, false, nil
	}
	var raw whatsappconnector.MessageRecord
	if err := json.Unmarshal(record.Raw, &raw); err != nil {
		return whatsappconnector.ReadRecentMessage{}, false, err
	}
	return whatsappconnector.ReadRecentMessage{
		MessageID: firstNonEmpty(raw.ID, record.ExternalID),
		SentAt:    recordTime(record),
		FromMe:    raw.FromMe,
		Sender:    firstNonEmpty(raw.Participant, raw.Sender, raw.RemoteJID, raw.Chat, raw.Conversation),
		Text:      raw.DisplayText(),
		MediaType: raw.MediaType,
	}, true, nil
}

func recordTime(record integrations.Record) time.Time {
	if !record.OccurredAt.IsZero() {
		return record.OccurredAt.UTC()
	}
	return record.ReceivedAt.UTC()
}
