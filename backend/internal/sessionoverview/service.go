package sessionoverview

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

type LiveReader interface {
	HydrationJobs([]string) map[string]acp.HydrationView
}

type Service struct {
	store storage.SessionOverviewReader
	live  LiveReader
}

type View struct {
	Threads   []Thread
	Subagents []Subagent
}

type Thread struct {
	ID              string
	Slug            string
	Title           string
	Agent           string
	Model           string
	ReasoningEffort string
	State           string
	Archived        bool
	UpdatedAt       time.Time
	LastEventAt     time.Time
}

type Subagent struct {
	sessionevents.ProviderSubagentEvent
	Seq       int64
	Key       string
	UpdatedAt time.Time
}

func NewService(store storage.SessionOverviewReader, live LiveReader) *Service {
	return &Service{store: store, live: live}
}

func (s *Service) Load(ctx context.Context, ref string) (View, error) {
	records, err := s.store.LoadSessionOverview(ctx, ref)
	if err != nil {
		return View{}, err
	}
	ids := make([]string, len(records.Threads))
	for i := range records.Threads {
		ids[i] = records.Threads[i].ID
	}
	active := s.live.HydrationJobs(ids)
	view := View{Threads: make([]Thread, 0, len(records.Threads))}
	for _, record := range records.Threads {
		view.Threads = append(view.Threads, threadView(record, active[record.ID]))
	}
	view.Subagents = subagentViews(records.SubagentEvents)
	return view, nil
}

func threadView(session storage.OverviewThread, active acp.HydrationView) Thread {
	view := Thread{
		ID: session.ID, Slug: session.Slug, Title: session.Title, Model: session.Model,
		ReasoningEffort: session.ReasoningEffort, State: acp.StateIdle,
		Archived: session.Archived, UpdatedAt: session.UpdatedAt,
	}
	view.Agent = acp.CanonicalAgentName(session.ACPAgent)
	if session.Status == storage.StatusError {
		view.State = acp.StateFailed
		return view
	}
	if active.ID == "" {
		return view
	}
	view.Agent = acp.CanonicalAgentName(first(active.ACPAgent, view.Agent))
	view.Model = first(session.Model, active.Model)
	view.ReasoningEffort = first(session.ReasoningEffort, active.ReasoningEffort)
	view.State = active.State
	view.LastEventAt = active.LastEventAt
	if active.UpdatedAt.After(view.UpdatedAt) {
		view.UpdatedAt = active.UpdatedAt
	}
	return view
}

func subagentViews(events []sessionevents.Event) []Subagent {
	byKey := make(map[string]Subagent)
	for _, event := range events {
		if event.ProviderSubagent == nil || event.ProviderSubagent.ID == "" {
			continue
		}
		key := event.ProjectionKey
		if key == "" {
			key = sessionevents.ProviderSubagentProjectionKey("", *event.ProviderSubagent)
		}
		if key == "" {
			continue
		}
		next := *event.ProviderSubagent
		updatedAt := event.At
		if previous, ok := byKey[key]; ok {
			next = sessionevents.MergeProviderSubagentEvent(previous.ProviderSubagentEvent, next)
			if previous.UpdatedAt.After(updatedAt) {
				updatedAt = previous.UpdatedAt
			}
		}
		byKey[key] = Subagent{
			ProviderSubagentEvent: next, Seq: event.Seq, Key: key, UpdatedAt: updatedAt,
		}
	}
	out := make([]Subagent, 0, len(byKey))
	for _, subagent := range byKey {
		out = append(out, subagent)
	}
	sort.Slice(out, func(i, j int) bool {
		if active := subagentActive(out[i]) != subagentActive(out[j]); active {
			return subagentActive(out[i])
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func subagentActive(subagent Subagent) bool {
	switch strings.ToLower(subagent.Status) {
	case "completed", "failed", "cancelled", "canceled", "shutdown", "closed":
		return false
	default:
		return true
	}
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
