package storage

import (
	"github.com/wins/jaz/backend/internal/codexcompat"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

func DisplayEvents(events []sessionevents.Event) []sessionevents.Event {
	out := make([]sessionevents.Event, 0, len(events))
	for _, event := range events {
		if display, ok := DisplayEvent(event); ok {
			out = append(out, display)
		}
	}
	return out
}

func DisplayEvent(event sessionevents.Event) (sessionevents.Event, bool) {
	if codexcompat.IsHiddenWarningEvent(event) {
		return sessionevents.Event{}, false
	}
	projection, ok, err := GoalProjectionFromEvent(event)
	if !ok {
		return event, true
	}
	event.Content = ""
	if err != nil || !projection.Seen || projection.State == nil {
		event.Type = sessionevents.TypeGoalClear
		event.Goal = nil
		return event, true
	}
	event.Type = sessionevents.TypeGoalUpdate
	event.Goal = projection.State
	return event, true
}
