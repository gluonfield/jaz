package server

import (
	"fmt"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

type promptOptions struct {
	GoalRequested bool
}

func promptOptionsFromStream(req streamRequest) promptOptions {
	return promptOptions{GoalRequested: req.GoalRequested}
}

func promptOptionsFromQueued(prompt storage.QueuedMessage) promptOptions {
	return promptOptions{GoalRequested: prompt.GoalRequested}
}

func (s *Server) validatePromptOptions(session storage.Session, options promptOptions) error {
	if !options.GoalRequested {
		return nil
	}
	if sessionSupportsNativeGoal(session) {
		return nil
	}
	if sessionMayNegotiateNativeGoal(session) {
		return nil
	}
	return fmt.Errorf("goal mode is not supported by this ACP agent")
}

func (s *Server) validateQueuedPrompt(session storage.Session, prompt storage.QueuedMessage) error {
	if err := s.validatePromptOptions(session, promptOptionsFromQueued(prompt)); err != nil {
		return queueInputError{err.Error()}
	}
	return nil
}

func sessionSupportsNativeGoal(session storage.Session) bool {
	if session.Runtime == "" {
		session.Runtime = storage.RuntimeACP
	}
	session = canonicalSessionResponse(session)
	return session.Runtime == storage.RuntimeACP &&
		session.RuntimeRef != nil &&
		session.RuntimeRef.Capabilities != nil &&
		session.RuntimeRef.Capabilities.NativeGoal
}

func sessionMayNegotiateNativeGoal(session storage.Session) bool {
	if session.Runtime != "" && session.Runtime != storage.RuntimeACP {
		return false
	}
	if session.RuntimeRef == nil || session.RuntimeRef.SessionID != "" || session.RuntimeRef.Capabilities != nil {
		return false
	}
	return acp.CatalogAgentCapabilitiesFor(session.RuntimeRef.Agent).NativeGoal
}
