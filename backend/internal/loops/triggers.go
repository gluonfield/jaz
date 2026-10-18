package loops

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/wins/jaz/backend/internal/integrationingest"
)

// HandleIncoming hands each active routine the observed messages its trigger
// matches, in one run per batch.
func (s *Service) HandleIncoming(ctx context.Context, messages []integrationingest.Incoming) {
	routines, err := s.Repo.ListLoops()
	if err != nil {
		s.Log.Warn("list routines for trigger failed", "error", err)
		return
	}
	for _, routine := range routines {
		if routine.Status != StatusActive || !slices.ContainsFunc(messages, routine.Trigger.matches) {
			continue
		}
		loop, run, start, err := s.take(routine, messages)
		if err != nil {
			s.Log.Warn("trigger did not reach a run", "routine", routine.ID, "error", err)
		} else if start {
			s.start(ctx, loop, run)
		}
	}
}

// take gives routine the matching messages it has not taken yet. A message
// older than missedAfter was missed while Jaz was not running: a routine that
// catches up still takes it if it is newer than the last message it took,
// while history from before then, or from before the routine, stays quiet. A
// message counts as taken once a run holds it.
func (s *Service) take(routine Loop, messages []integrationingest.Incoming) (Loop, Run, bool, error) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	since := now.Add(-missedAfter)
	if routine.WhenMissed == MissedCatchUp {
		latest, err := s.Repo.LatestEventAt(routine.ID)
		if err != nil {
			return Loop{}, Run{}, false, err
		}
		if latest.Before(since) {
			since = latest
		}
	}
	if since.Before(routine.CreatedAt) {
		since = routine.CreatedAt
	}
	var events []string
	var taken []SeenEvent
	ids := map[string]bool{}
	for _, message := range messages {
		if !routine.Trigger.matches(message) || message.At.Before(since) || ids[message.ID] {
			continue
		}
		seen, err := s.Repo.EventSeen(routine.ID, message.ID)
		if err != nil {
			return Loop{}, Run{}, false, err
		}
		if seen {
			continue
		}
		ids[message.ID] = true
		events = append(events, describe(message))
		taken = append(taken, SeenEvent{LoopID: routine.ID, ID: message.ID, At: message.At})
	}
	if len(taken) == 0 {
		return Loop{}, Run{}, false, nil
	}
	return s.fireLocked(routine, events, taken, now)
}

func (t *Trigger) matches(in integrationingest.Incoming) bool {
	return t != nil &&
		t.Kind == in.Provider &&
		containsFold(in.From, t.From) &&
		containsFold(in.Subject, t.Subject) &&
		containsFold(in.Subject+"\n"+in.Text, t.Contains)
}

func describe(in integrationingest.Incoming) string {
	parts := []string{fmt.Sprintf("new %s message from %s", in.Provider, in.From)}
	if in.Subject != "" {
		parts = append(parts, "subject: "+in.Subject)
	}
	if text := strings.TrimSpace(in.Text); text != "" {
		if runes := []rune(text); len(runes) > 2000 {
			text = string(runes[:2000]) + "…"
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n")
}

func containsFold(value, filter string) bool {
	return filter == "" || strings.Contains(strings.ToLower(value), strings.ToLower(filter))
}

// Watching reports whether an active routine listens for kind.
func (s *Service) Watching(kind string) bool {
	routines, err := s.Repo.ListLoops()
	if err != nil {
		return false
	}
	return slices.ContainsFunc(routines, func(routine Loop) bool {
		return routine.Status == StatusActive && routine.Trigger != nil && routine.Trigger.Kind == kind
	})
}
