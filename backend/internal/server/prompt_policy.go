package server

import (
	"fmt"

	"github.com/wins/jaz/backend/internal/storage"
)

func (s *Server) validateGoalRequest(session storage.Session, requested bool) error {
	if !requested {
		return nil
	}
	runtime := session.Runtime
	if runtime == "" {
		runtime = storage.RuntimeACP
	}
	if runtime == storage.RuntimeACP {
		return nil
	}
	return fmt.Errorf("goal mode is not supported by this runtime")
}

func (s *Server) validateQueuedPrompt(session storage.Session, prompt storage.QueuedMessage) error {
	if err := s.validateGoalRequest(session, prompt.GoalRequested); err != nil {
		return queueInputError{err.Error()}
	}
	return nil
}

func (s *Server) validateQueuedMessage(session storage.Session, message storage.QueuedMessage) error {
	if !message.IsAction() {
		return s.validateQueuedPrompt(session, message)
	}
	if s.canStartQueuedAction(session, message.Action) {
		return nil
	}
	return queueInputError{fmt.Sprintf("queued action %q cannot be queued for this session", message.Action)}
}
