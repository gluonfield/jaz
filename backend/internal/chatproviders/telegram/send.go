package telegram

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	telegramconnector "github.com/wins/jaz/backend/internal/connectors/telegram"
)

func (p *Provider) SendMessage(ctx context.Context, req telegramconnector.SendMessageRequest) (telegramconnector.SendMessageResult, error) {
	client := p.client(req.Connection.ID)
	randomID := int64(rand.Uint64() & (1<<63 - 1))
	var peerID string
	send := func(runCtx context.Context, client *telegram.Client) error {
		peer, resolved, err := p.resolvePeer(runCtx, client.API(), req.Recipient)
		if err != nil {
			return err
		}
		peerID = resolved
		if err := p.foregroundCall(runCtx, func(ctx context.Context) error {
			return client.SendMessage(ctx, &tg.MessagesSendMessageRequest{
				Peer:     peer,
				Message:  req.Message,
				RandomID: randomID,
			})
		}); err != nil {
			return err
		}
		return nil
	}
	if client != nil {
		if err := send(ctx, client); err != nil {
			return telegramconnector.SendMessageResult{}, err
		}
		return telegramconnector.SendMessageResult{MessageID: strconv.FormatInt(randomID, 10), PeerID: peerID, SentAt: time.Now().UTC()}, nil
	}
	err := p.withClient(ctx, req.Connection.ID, true, func(runCtx context.Context, client *telegram.Client) error {
		peer, resolved, err := p.resolvePeer(runCtx, client.API(), req.Recipient)
		if err != nil {
			return err
		}
		peerID = resolved
		return p.foregroundCall(runCtx, func(ctx context.Context) error {
			return client.SendMessage(ctx, &tg.MessagesSendMessageRequest{
				Peer:     peer,
				Message:  req.Message,
				RandomID: randomID,
			})
		})
	})
	if err != nil {
		return telegramconnector.SendMessageResult{}, err
	}
	return telegramconnector.SendMessageResult{MessageID: strconv.FormatInt(randomID, 10), PeerID: peerID, SentAt: time.Now().UTC()}, nil
}

func (p *Provider) resolvePeer(ctx context.Context, api *tg.Client, recipient string) (tg.InputPeerClass, string, error) {
	recipient = strings.TrimSpace(recipient)
	if recipient == "" {
		return nil, "", fmt.Errorf("telegram recipient is required")
	}
	parts := strings.Split(recipient, ":")
	switch parts[0] {
	case "user":
		if len(parts) != 3 {
			return nil, "", fmt.Errorf("telegram user recipient must be user:<id>:<access_hash>")
		}
		id, hash, err := parseIDHash(parts[1], parts[2])
		if err != nil {
			return nil, "", err
		}
		return &tg.InputPeerUser{UserID: id, AccessHash: hash}, "user:" + parts[1], nil
	case "channel":
		if len(parts) != 3 {
			return nil, "", fmt.Errorf("telegram channel recipient must be channel:<id>:<access_hash>")
		}
		id, hash, err := parseIDHash(parts[1], parts[2])
		if err != nil {
			return nil, "", err
		}
		return &tg.InputPeerChannel{ChannelID: id, AccessHash: hash}, "channel:" + parts[1], nil
	case "chat":
		if len(parts) != 2 {
			return nil, "", fmt.Errorf("telegram chat recipient must be chat:<id>")
		}
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return nil, "", err
		}
		return &tg.InputPeerChat{ChatID: id}, "chat:" + parts[1], nil
	}
	if strings.Contains(recipient, " ") {
		return nil, "", fmt.Errorf("telegram recipient must be a username or explicit peer id")
	}
	username := strings.TrimPrefix(recipient, "@")
	if username == "" {
		return nil, "", fmt.Errorf("telegram username is required")
	}
	var resolved *tg.ContactsResolvedPeer
	err := p.foregroundCall(ctx, func(ctx context.Context) error {
		var err error
		resolved, err = api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: username})
		return err
	})
	if err != nil {
		return nil, "", err
	}
	return inputPeerFromResolved(resolved)
}

func inputPeerFromResolved(resolved *tg.ContactsResolvedPeer) (tg.InputPeerClass, string, error) {
	switch peer := resolved.Peer.(type) {
	case *tg.PeerUser:
		for _, item := range resolved.Users {
			user, ok := item.(*tg.User)
			if !ok || user.ID != peer.UserID {
				continue
			}
			hash, ok := user.GetAccessHash()
			if !ok {
				return nil, "", fmt.Errorf("telegram user access hash missing")
			}
			return &tg.InputPeerUser{UserID: user.ID, AccessHash: hash}, "user:" + strconv.FormatInt(user.ID, 10), nil
		}
	case *tg.PeerChat:
		return &tg.InputPeerChat{ChatID: peer.ChatID}, "chat:" + strconv.FormatInt(peer.ChatID, 10), nil
	case *tg.PeerChannel:
		for _, item := range resolved.Chats {
			channel, ok := item.(*tg.Channel)
			if !ok || channel.ID != peer.ChannelID {
				continue
			}
			hash, ok := channel.GetAccessHash()
			if !ok {
				return nil, "", fmt.Errorf("telegram channel access hash missing")
			}
			return &tg.InputPeerChannel{ChannelID: channel.ID, AccessHash: hash}, "channel:" + strconv.FormatInt(channel.ID, 10), nil
		}
	}
	return nil, "", fmt.Errorf("telegram username did not resolve to a sendable peer")
}

func parseIDHash(idValue, hashValue string) (int64, int64, error) {
	id, err := strconv.ParseInt(idValue, 10, 64)
	if err != nil {
		return 0, 0, err
	}
	hash, err := strconv.ParseInt(hashValue, 10, 64)
	if err != nil {
		return 0, 0, err
	}
	return id, hash, nil
}
