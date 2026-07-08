package server

import (
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/goal"
	"github.com/wins/jaz/backend/internal/storage"
)

type sessionResponse struct {
	storage.Session
	Goal    *goal.PublicState `json:"goal,omitempty"`
	Actions *sessionActions   `json:"actions,omitempty"`
}

func canonicalSessionResponses(sessions []storage.Session) []sessionResponse {
	out := make([]sessionResponse, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, canonicalSessionResponse(session))
	}
	return out
}

func canonicalSessionResponse(session storage.Session) sessionResponse {
	session = canonicalSession(session)
	publicGoal := goal.PublicStateFrom(session.Goal)
	session.Goal = nil
	resp := sessionResponse{Session: session, Goal: publicGoal}
	if actions := sessionActionsForSession(session); actions != (sessionActions{}) {
		resp.Actions = &actions
	}
	return resp
}

func canonicalSession(session storage.Session) storage.Session {
	session.QueuedMessages = storage.PublicQueuedMessages(session.QueuedMessages)
	session.ManualTitle = false
	if session.PendingSteer != nil && session.PendingSteer.IsInternal() {
		session.PendingSteer = nil
	}
	if session.Runtime != storage.RuntimeACP || session.RuntimeRef == nil {
		return session
	}
	ref := *session.RuntimeRef
	canonical := acp.CanonicalAgentName(ref.Agent)
	if canonical == "" {
		session.RuntimeRef = &ref
		return session
	}
	if session.ModelProvider != "" && acp.CanonicalAgentName(session.ModelProvider) == canonical {
		session.ModelProvider = canonical
	}
	ref.Agent = canonical
	session.RuntimeRef = &ref
	return session
}
