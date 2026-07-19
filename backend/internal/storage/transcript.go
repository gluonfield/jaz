package storage

import (
	"context"
	"errors"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

var ErrTranscriptChanged = errors.New("session history changed; reload from the latest page")

const (
	MaxTranscriptPageBytes    = 24 << 20
	MaxTranscriptMessageBytes = 8 << 20
	MaxTranscriptEventBytes   = MaxTranscriptPageBytes - MaxTranscriptMessageBytes
	MaxSessionEventBytes      = 16 << 20
	MaxTextEventBytes         = 256 << 10
)

type TranscriptPage struct {
	Messages         []Message
	Events           []sessionevents.Event
	HasEarlier       bool
	BeforeMessageSeq int64
	BeforeEventSeq   int64
	HistoryRevision  int64
	LatestEventSeq   int64
}

type TranscriptPageRequest struct {
	BeforeMessageSeq int64
	BeforeEventSeq   int64
	HistoryRevision  int64
	Turns            int
}

type TranscriptPageReader interface {
	LoadTranscriptSession(context.Context, string) (Session, error)
	LoadTranscriptPage(context.Context, string, TranscriptPageRequest) (TranscriptPage, error)
	LoadTranscriptSessions(context.Context, string, []string) (TranscriptSessions, error)
}

type TranscriptSessions struct {
	Children   []TranscriptSession
	References []TranscriptSession
}

type TranscriptSession struct {
	ID              string
	Slug            string
	Title           string
	ParentID        string
	Status          string
	ACPAgent        string
	ACPSession      string
	Cwd             string
	Error           string
	ModelProvider   string
	Model           string
	ReasoningEffort string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
