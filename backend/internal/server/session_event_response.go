package server

import (
	"github.com/wins/jaz/backend/internal/goal"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

type sessionEventResponse struct {
	sessionevents.Event
	Goal *goal.PublicState `json:"goal,omitempty"`
}

func sessionEventResponses(events []sessionevents.Event) []sessionEventResponse {
	out := make([]sessionEventResponse, 0, len(events))
	for _, event := range events {
		out = append(out, sessionEventResponseFrom(event))
	}
	return out
}

func sessionEventResponseFrom(event sessionevents.Event) sessionEventResponse {
	publicGoal := goal.PublicStateFrom(event.Goal)
	event.Goal = nil
	return sessionEventResponse{
		Event: event,
		Goal:  publicGoal,
	}
}
