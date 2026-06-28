package server

import (
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func mobileSessionEvents(events []sessionevents.Event) []sessionevents.Event {
	if len(events) == 0 {
		return events
	}
	out := make([]sessionevents.Event, len(events))
	for i, event := range events {
		out[i] = mobileSessionEvent(event)
	}
	return out
}

func mobileSessionEvent(event sessionevents.Event) sessionevents.Event {
	if event.ACP != nil {
		acp := *event.ACP
		acp.ToolCalls = mobileACPToolCalls(acp.ToolCalls)
		event.ACP = &acp
	}
	return event
}

func mobileACPStates(states []storage.ACPState) []storage.ACPState {
	if len(states) == 0 {
		return states
	}
	out := make([]storage.ACPState, len(states))
	for i, state := range states {
		out[i] = mobileACPState(state)
	}
	return out
}

func mobileACPState(state storage.ACPState) storage.ACPState {
	state.ToolCalls = mobileACPToolCalls(state.ToolCalls)
	return state
}

func mobileACPToolCalls(calls []sessionevents.ACPToolCall) []sessionevents.ACPToolCall {
	if len(calls) == 0 {
		return calls
	}
	out := make([]sessionevents.ACPToolCall, len(calls))
	for i, call := range calls {
		out[i] = sessionevents.ACPToolCall{
			ID:     call.ID,
			Title:  call.Title,
			Status: call.Status,
		}
	}
	return out
}
