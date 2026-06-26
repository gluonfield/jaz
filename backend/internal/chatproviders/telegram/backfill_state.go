package telegram

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/gotd/td/tg"
)

type backfillState struct {
	Completed           bool             `json:"completed,omitempty"`
	PausedUntil         time.Time        `json:"paused_until,omitempty"`
	DialogOffset        dialogOffset     `json:"dialog_offset,omitempty"`
	CurrentPeer         *telegramPeerRef `json:"current_peer,omitempty"`
	CurrentPeerOffsetID int              `json:"current_peer_offset_id,omitempty"`
	CompletedPeers      map[string]bool  `json:"completed_peers,omitempty"`
	UpdatedAt           time.Time        `json:"updated_at,omitempty"`
}

type dialogOffset struct {
	Peer *telegramPeerRef `json:"peer,omitempty"`
	ID   int              `json:"id,omitempty"`
	Date int              `json:"date,omitempty"`
}

type telegramPeerRef struct {
	Kind       string `json:"kind"`
	ID         int64  `json:"id"`
	AccessHash int64  `json:"access_hash,omitempty"`
}

func (p *Provider) loadBackfillState(connectionID string) (backfillState, error) {
	data, err := os.ReadFile(p.backfillStatePath(connectionID))
	if errors.Is(err, os.ErrNotExist) {
		return backfillState{CompletedPeers: map[string]bool{}}, nil
	}
	if err != nil {
		return backfillState{}, err
	}
	var state backfillState
	if err := json.Unmarshal(data, &state); err != nil {
		return backfillState{}, err
	}
	if state.CompletedPeers == nil {
		state.CompletedPeers = map[string]bool{}
	}
	return state, nil
}

func (p *Provider) saveBackfillState(connectionID string, state backfillState) error {
	state.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p.backfillStatePath(connectionID), data, 0o600)
}

func (p *Provider) finishBackfill(connectionID string, state backfillState) error {
	state.Completed = true
	if err := p.saveBackfillState(connectionID, state); err != nil {
		return err
	}
	return os.WriteFile(p.backfillMarkerPath(connectionID), []byte(time.Now().UTC().Format(time.RFC3339Nano)), 0o600)
}

func (p *Provider) backfillStatePath(connectionID string) string {
	return p.backfillMarkerPath(connectionID) + ".json"
}

func (o dialogOffset) inputPeer() tg.InputPeerClass {
	if o.Peer == nil {
		return &tg.InputPeerEmpty{}
	}
	return o.Peer.inputPeer()
}

func (r telegramPeerRef) key() string {
	return r.Kind + ":" + strconv.FormatInt(r.ID, 10)
}

func (r telegramPeerRef) inputPeer() tg.InputPeerClass {
	switch r.Kind {
	case "user":
		return &tg.InputPeerUser{UserID: r.ID, AccessHash: r.AccessHash}
	case "channel":
		return &tg.InputPeerChannel{ChannelID: r.ID, AccessHash: r.AccessHash}
	case "chat":
		return &tg.InputPeerChat{ChatID: r.ID}
	default:
		return &tg.InputPeerEmpty{}
	}
}

func peerRefFromDialog(dialog tg.DialogClass, users map[int64]*tg.User, chats map[int64]*tg.Chat, channels map[int64]*tg.Channel) (telegramPeerRef, bool) {
	switch peer := dialog.GetPeer().(type) {
	case *tg.PeerUser:
		user := users[peer.UserID]
		if user == nil {
			return telegramPeerRef{}, false
		}
		hash, ok := user.GetAccessHash()
		if !ok {
			return telegramPeerRef{}, false
		}
		return telegramPeerRef{Kind: "user", ID: user.ID, AccessHash: hash}, true
	case *tg.PeerChat:
		if chats[peer.ChatID] == nil {
			return telegramPeerRef{}, false
		}
		return telegramPeerRef{Kind: "chat", ID: peer.ChatID}, true
	case *tg.PeerChannel:
		channel := channels[peer.ChannelID]
		if channel == nil {
			return telegramPeerRef{}, false
		}
		hash, ok := channel.GetAccessHash()
		if !ok {
			return telegramPeerRef{}, false
		}
		return telegramPeerRef{Kind: "channel", ID: channel.ID, AccessHash: hash}, true
	default:
		return telegramPeerRef{}, false
	}
}
