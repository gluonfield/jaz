package telegram

import (
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	telegramconnector "github.com/wins/jaz/backend/internal/connectors/telegram"
)

func (p *Provider) ReadRecent(ctx context.Context, req telegramconnector.ReadRecentRequest) (telegramconnector.ReadRecentResult, error) {
	limit := telegramconnector.ReadRecentLimit(req.Limit)
	var peerID string
	var messages []telegramconnector.ReadRecentMessage
	read := func(runCtx context.Context, client *telegram.Client) error {
		peer, resolved, err := p.resolvePeer(runCtx, client.API(), req.Peer)
		if err != nil {
			return err
		}
		peerID = resolved
		var history tg.MessagesMessagesClass
		if err := p.foregroundCall(runCtx, func(ctx context.Context) error {
			var err error
			history, err = client.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
				Peer:  peer,
				Limit: limit,
			})
			return err
		}); err != nil {
			return err
		}
		messages = telegramRecentMessages(telegramHistoryMessages(history), limit)
		return nil
	}
	if client := p.client(req.Connection.ID); client != nil {
		if err := read(ctx, client); err != nil {
			return telegramconnector.ReadRecentResult{}, err
		}
		return telegramconnector.ReadRecentResult{PeerID: peerID, Messages: messages}, nil
	}
	if err := p.withClient(ctx, req.Connection.ID, true, read); err != nil {
		return telegramconnector.ReadRecentResult{}, err
	}
	return telegramconnector.ReadRecentResult{PeerID: peerID, Messages: messages}, nil
}

func telegramHistoryMessages(history tg.MessagesMessagesClass) []tg.MessageClass {
	switch h := history.(type) {
	case *tg.MessagesMessages:
		return h.Messages
	case *tg.MessagesMessagesSlice:
		return h.Messages
	case *tg.MessagesChannelMessages:
		return h.Messages
	default:
		return nil
	}
}

func telegramRecentMessages(raw []tg.MessageClass, limit int) []telegramconnector.ReadRecentMessage {
	out := make([]telegramconnector.ReadRecentMessage, 0, len(raw))
	for _, item := range raw {
		switch msg := item.(type) {
		case *tg.Message:
			out = append(out, telegramRecentMessage(msg))
		case *tg.MessageService:
			out = append(out, telegramServiceRecentMessage(msg))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].SentAt.Before(out[j].SentAt)
	})
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func telegramRecentMessage(msg *tg.Message) telegramconnector.ReadRecentMessage {
	return telegramconnector.ReadRecentMessage{
		MessageID: strconv.Itoa(msg.ID),
		SentAt:    time.Unix(int64(msg.Date), 0).UTC(),
		FromMe:    msg.Out,
		Sender:    peerID(msg.FromID),
		Text:      msg.Message,
	}
}

func telegramServiceRecentMessage(msg *tg.MessageService) telegramconnector.ReadRecentMessage {
	return telegramconnector.ReadRecentMessage{
		MessageID: strconv.Itoa(msg.ID),
		SentAt:    time.Unix(int64(msg.Date), 0).UTC(),
		FromMe:    msg.Out,
		Sender:    peerID(msg.FromID),
		Text:      "[service]",
	}
}
