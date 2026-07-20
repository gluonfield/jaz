package storage

import (
	"context"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

type SessionOverview struct {
	Threads        []OverviewThread
	SubagentEvents []sessionevents.Event
}

type OverviewThread struct {
	ID              string
	Slug            string
	Title           string
	Status          string
	ACPAgent        string
	Model           string
	ReasoningEffort string
	Archived        bool
	UpdatedAt       time.Time
}

type SessionOverviewReader interface {
	LoadSessionOverview(context.Context, string) (SessionOverview, error)
}
