package telegram

import (
	"context"
	"time"

	"github.com/wins/jaz/backend/pkg/integrations"
)

const (
	DefaultSearchLimit = 10
	MaxSearchLimit     = 25
)

type Sender interface {
	SendMessage(context.Context, SendMessageRequest) (SendMessageResult, error)
}

type Searcher interface {
	Search(context.Context, SearchRequest) (SearchResult, error)
}

type SendMessageRequest struct {
	Connection integrations.Connection
	Recipient  string
	Message    string
}

type SendMessageResult struct {
	MessageID string    `json:"message_id,omitempty"`
	PeerID    string    `json:"peer_id,omitempty"`
	SentAt    time.Time `json:"sent_at,omitempty"`
}

type SearchRequest struct {
	Connection integrations.Connection
	Query      string
	Limit      int
}

type SearchResult struct {
	Items []SearchItem `json:"items"`
}

type SearchItemKind string

const (
	SearchItemPerson  SearchItemKind = "person"
	SearchItemBot     SearchItemKind = "bot"
	SearchItemGroup   SearchItemKind = "group"
	SearchItemChannel SearchItemKind = "channel"
)

type SearchItem struct {
	Kind      SearchItemKind `json:"kind"`
	Name      string         `json:"name,omitempty"`
	Username  string         `json:"username,omitempty"`
	Phone     string         `json:"phone,omitempty"`
	Recipient string         `json:"recipient"`
	PeerID    string         `json:"peer_id,omitempty"`
}

func SearchLimit(limit int) int {
	if limit <= 0 {
		return DefaultSearchLimit
	}
	if limit > MaxSearchLimit {
		return MaxSearchLimit
	}
	return limit
}
