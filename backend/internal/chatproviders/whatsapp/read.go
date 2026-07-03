package whatsapp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
	chat, err := recipientJID(req.Chat)
	if err != nil {
		return whatsappconnector.ReadRecentResult{}, err
	}
	limit := whatsappconnector.ReadRecentLimit(req.Limit)
	stored, err := p.storedRecentMessages(ctx, req.Connection, chat.String(), limit)
	live := p.cachedRecentMessages(chat.String(), limit)
	if err != nil && len(live) == 0 {
		return whatsappconnector.ReadRecentResult{}, err
	}
	messages := mergeRecentMessages(stored, live, limit)
	if len(messages) < limit && len(live) > 0 && p.container != nil {
		if client, err := p.clientForConnection(ctx, req.Connection); err == nil {
			if !client.IsConnected() {
				_ = client.Connect()
			}
			if client.IsConnected() {
				_ = p.requestOlderHistory(ctx, client, chat, live[0], limit-len(messages))
				live = p.cachedRecentMessages(chat.String(), limit)
				messages = mergeRecentMessages(stored, live, limit)
			}
		}
	}
	if len(messages) == 0 {
		return whatsappconnector.ReadRecentResult{}, errors.New("no live or stored WhatsApp messages are available for this chat")
	}
	return whatsappconnector.ReadRecentResult{Chat: chat.String(), Messages: messages}, nil
}

func (p *Provider) storedRecentMessages(ctx context.Context, connection integrations.Connection, chat string, limit int) ([]whatsappconnector.ReadRecentMessage, error) {
	root := filepath.Join(p.cfg.RawRoot, whatsappconnector.ProviderID, integrations.NormalizeAlias(connection.AccountID), string(integrations.RecordDomainMessages))
	if p.cfg.RawRoot == "" || connection.AccountID == "" {
		return nil, nil
	}
	var messages []whatsappconnector.ReadRecentMessage
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "messages.jsonl" {
			return nil
		}
		records, err := readWhatsAppRawRecords(path)
		if err != nil {
			return err
		}
		for _, record := range records {
			message, ok, err := decodeStoredWhatsAppMessage(record, chat)
			if err != nil {
				return err
			}
			if ok {
				messages = append(messages, message)
			}
		}
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return mergeRecentMessages(nil, messages, limit), nil
}

func readWhatsAppRawRecords(path string) ([]integrations.Record, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var records []integrations.Record
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var record integrations.Record
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, err
		}
		if record.Kind == "whatsapp.message" {
			records = append(records, record)
		}
	}
	return records, scanner.Err()
}

func decodeStoredWhatsAppMessage(record integrations.Record, chat string) (whatsappconnector.ReadRecentMessage, bool, error) {
	var raw whatsappconnector.MessageRecord
	if err := json.Unmarshal(record.Raw, &raw); err != nil {
		return whatsappconnector.ReadRecentMessage{}, false, err
	}
	if raw.ConversationID(record.ExternalID) != chat {
		return whatsappconnector.ReadRecentMessage{}, false, nil
	}
	return decodeWhatsAppRecentMessage(record)
}

func mergeRecentMessages(left, right []whatsappconnector.ReadRecentMessage, limit int) []whatsappconnector.ReadRecentMessage {
	merged := make([]whatsappconnector.ReadRecentMessage, 0, len(left)+len(right))
	seen := map[string]bool{}
	add := func(message whatsappconnector.ReadRecentMessage) {
		key := recentMessageKey(message)
		if key != "" && seen[key] {
			return
		}
		if key != "" {
			seen[key] = true
		}
		merged = append(merged, message)
	}
	for _, message := range left {
		add(message)
	}
	for _, message := range right {
		add(message)
	}
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].SentAt.Equal(merged[j].SentAt) {
			return merged[i].MessageID < merged[j].MessageID
		}
		return merged[i].SentAt.Before(merged[j].SentAt)
	})
	if len(merged) > limit {
		merged = merged[len(merged)-limit:]
	}
	return merged
}

func recentMessageKey(message whatsappconnector.ReadRecentMessage) string {
	if message.MessageID != "" {
		return "id:" + message.MessageID
	}
	if !message.SentAt.IsZero() || message.Sender != "" || message.Text != "" {
		return "fallback:" + message.SentAt.UTC().Format(time.RFC3339Nano) + "\x00" + message.Sender + "\x00" + message.Text
	}
	return ""
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
	p.recentMessages[chat] = mergeRecentMessages(p.recentMessages[chat], messages, whatsappReadRecentCacheLimit)
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
