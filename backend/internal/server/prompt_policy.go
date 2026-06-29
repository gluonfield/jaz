package server

import (
	"fmt"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

type promptOptions struct {
	GoalRequested bool
}

type promptFeatureSupport uint8

const (
	promptFeatureUnsupported promptFeatureSupport = iota
	promptFeatureSupported
	promptFeatureNegotiable
)

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
	switch nativeGoalSupport(session) {
	case promptFeatureSupported, promptFeatureNegotiable:
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

func nativeGoalSupport(session storage.Session) promptFeatureSupport {
	runtime := session.Runtime
	if runtime == "" {
		runtime = storage.RuntimeACP
	}
	if runtime != storage.RuntimeACP || session.RuntimeRef == nil {
		return promptFeatureUnsupported
	}
	caps := acp.EffectiveRuntimeCapabilities(session.RuntimeRef.Agent, session.RuntimeRef.Capabilities)
	if caps != nil && caps.NativeGoal {
		return promptFeatureSupported
	}
	if caps != nil && caps.NativeGoalNegotiable && session.RuntimeRef.SessionID == "" {
		return promptFeatureNegotiable
	}
	return promptFeatureUnsupported
}
