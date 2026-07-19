package sessionview

import (
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/goal"
	"github.com/wins/jaz/backend/internal/storage"
)

type Response struct {
	storage.Session
	Goal    *goal.PublicState `json:"goal,omitempty"`
	Actions *Actions          `json:"actions,omitempty"`
}

type Actions struct {
	Compact bool `json:"compact,omitempty"`
}

func Responses(sessions []storage.Session) []Response {
	out := make([]Response, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, Public(session))
	}
	return out
}

func Public(session storage.Session) Response {
	session = Canonical(session)
	publicGoal := goal.PublicStateFrom(session.Goal)
	session.Goal = nil
	response := Response{Session: session, Goal: publicGoal}
	if SupportsCompact(session) {
		response.Actions = &Actions{Compact: true}
	}
	return response
}

func Canonical(session storage.Session) storage.Session {
	session.QueuedMessages = storage.PublicQueuedMessages(session.QueuedMessages)
	session.ManualTitle = false
	session.TitleLocked = false
	if session.PendingSteer != nil && session.PendingSteer.IsInternal() {
		session.PendingSteer = nil
	}
	if session.Runtime != storage.RuntimeACP || session.RuntimeRef == nil {
		return session
	}
	ref := *session.RuntimeRef
	canonical := acp.CanonicalAgentName(ref.Agent)
	if canonical != "" {
		if session.ModelProvider != "" && acp.CanonicalAgentName(session.ModelProvider) == canonical {
			session.ModelProvider = canonical
		}
		ref.Agent = canonical
	}
	session.RuntimeRef = &ref
	return session
}

func SupportsCompact(session storage.Session) bool {
	if session.Runtime != storage.RuntimeACP {
		return false
	}
	agent := session.ModelProvider
	if session.RuntimeRef != nil && session.RuntimeRef.Agent != "" {
		agent = session.RuntimeRef.Agent
	}
	return acp.AgentSupportsCompact(agent)
}
