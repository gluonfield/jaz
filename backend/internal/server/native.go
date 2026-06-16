package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/jazagent"
	"github.com/wins/jaz/backend/internal/media"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
)

// voiceModeNote steers spoken turns; it is injected per-request and stripped
// before messages are persisted, so transcripts stay clean.
const voiceModeNote = jazagent.VoiceModeNote
const nativePlanModeNote = jazagent.PlanModeNote
const nativePlanUserInstruction = jazagent.PlanUserInstruction

// stripTransientSystem removes injected system/developer messages — the
// per-turn system prompt and mode notes — before persisting (the agent echoes the full
// request message list back, and SaveMessages replaces the stored list
// wholesale), remapping reasoning indexes onto the stripped list.
func stripTransientSystem(messages []provider.Message, reasoning map[int]string, injected []string) ([]provider.Message, map[int]string) {
	return jazagent.StripTransientSystem(messages, reasoning, injected)
}

func (s *Server) streamNativeSession(w http.ResponseWriter, flusher http.Flusher, r *http.Request, session storage.Session, message string, attachments []storage.Attachment, voiceMode bool, planRequested bool) {
	clientCtx := r.Context()
	send := func(event agent.StreamEvent) {
		if clientCtx.Err() == nil {
			writeSSE(w, flusher, event)
		}
	}
	if s.runNativeSessionWithAttachments(context.WithoutCancel(clientCtx), session, message, attachments, voiceMode, planRequested, send) == storage.StatusIdle {
		s.drainQueueSoon(session.ID)
	}
}

func (s *Server) runNativeSession(ctx context.Context, session storage.Session, message string, voiceMode bool, send func(agent.StreamEvent)) string {
	return s.runNativeSessionWithClaim(ctx, session, message, nil, voiceMode, false, send, false)
}

func (s *Server) runNativeSessionWithAttachments(ctx context.Context, session storage.Session, message string, attachments []storage.Attachment, voiceMode bool, planRequested bool, send func(agent.StreamEvent)) string {
	return s.runNativeSessionWithClaim(ctx, session, message, attachments, voiceMode, planRequested, send, false)
}

func (s *Server) runClaimedNativeSession(ctx context.Context, session storage.Session, message string) string {
	return s.runNativeSessionWithClaim(ctx, session, message, nil, false, false, nil, true)
}

func (s *Server) runClaimedNativeSessionWithAttachments(ctx context.Context, session storage.Session, message string, attachments []storage.Attachment, planRequested bool) string {
	return s.runNativeSessionWithClaim(ctx, session, message, attachments, false, planRequested, nil, true)
}

func (s *Server) runNativeSessionWithClaim(ctx context.Context, session storage.Session, message string, attachments []storage.Attachment, voiceMode bool, planRequested bool, send func(agent.StreamEvent), claimed bool) string {
	if send == nil {
		send = func(agent.StreamEvent) {}
	}
	session, startStatus, generateTitle, err := s.beginNativeTurn(ctx, session, message, claimed)
	if err != nil {
		send(agent.StreamEvent{Type: agent.StreamError, Error: err.Error()})
		send(agent.StreamEvent{Type: agent.StreamDone})
		if startStatus == storage.StatusError {
			s.setSessionError(session, err.Error())
		}
		return startStatus
	}
	logger := s.logger().With("session", session.ID)
	logger.Info("native turn started")
	turnCtx, cancelTurn := context.WithCancel(ctx)
	defer cancelTurn()
	s.turnCancels.Store(session.ID, cancelTurn)
	defer s.turnCancels.Delete(session.ID)

	turn, err := s.nativeRequestMessages(session, message, attachments, voiceMode, planRequested)
	if err != nil {
		send(agent.StreamEvent{Type: agent.StreamError, Error: err.Error()})
		send(agent.StreamEvent{Type: agent.StreamDone})
		s.setSessionError(session, err.Error())
		return storage.StatusError
	}
	s.publishMessagesChanged(session.ID)
	if generateTitle {
		go s.generateAndSaveSessionTitle(context.WithoutCancel(ctx), session, message)
	}

	runCtx := sessioncontext.WithSessionID(turnCtx, session.ID)
	if session.RuntimeRef != nil && strings.TrimSpace(session.RuntimeRef.Cwd) != "" {
		runCtx = sessioncontext.WithCWD(runCtx, session.RuntimeRef.Cwd)
	}
	if planRequested {
		runCtx = sessioncontext.WithCollaborationMode(runCtx, sessioncontext.CollaborationModePlan)
	}
	finalStatus := storage.StatusError
	finalError := "Agent stream ended without a completion event."
	for event := range s.Agent.Run(runCtx, provider.Request{
		Provider:        session.ModelProvider,
		Model:           session.Model,
		ReasoningEffort: session.ReasoningEffort,
		Messages:        turn.Messages,
		MediaRefs:       turn.MediaRefs,
	}) {
		if len(event.Messages) > 0 {
			toSave, reasoning := stripTransientSystem(turn.StorageSnapshot(event.Messages), event.ReasoningByMessage, turn.Transient)
			var err error
			if store, ok := s.Store.(mediaReasoningMessageStore); ok && (len(reasoning) > 0 || len(event.MediaRefs) > 0) {
				err = store.SaveMessagesWithReasoningAndMedia(session.ID, toSave, reasoning, event.MediaRefs)
			} else if store, ok := s.Store.(reasoningMessageStore); ok && len(reasoning) > 0 {
				err = store.SaveMessagesWithReasoning(session.ID, toSave, reasoning)
			} else {
				err = s.Store.SaveMessages(session.ID, toSave)
			}
			if err != nil {
				logger.Error("saving turn messages failed", "error", err)
				send(agent.StreamEvent{Type: agent.StreamError, Error: err.Error()})
				send(agent.StreamEvent{Type: agent.StreamDone})
				s.setSessionError(session, err.Error())
				return storage.StatusError
			}
			s.publishMessagesChanged(session.ID)
		}
		if event.Type == agent.StreamDone {
			finalStatus = storage.StatusIdle
			s.addUsage(session.ID, event.Usage)
		}
		if event.Type == agent.StreamError {
			finalStatus = storage.StatusError
			finalError = event.Error
		}
		send(event)
	}
	if turnCtx.Err() != nil && finalStatus == storage.StatusError {
		finalStatus = storage.StatusIdle
		finalError = ""
	}
	if finalStatus == storage.StatusError {
		logger.Error("native turn failed", "error", finalError)
	} else {
		logger.Info("native turn finished", "status", finalStatus, "cancelled", turnCtx.Err() != nil)
	}
	s.setSessionStatusWithError(session, finalStatus, finalError)
	return finalStatus
}

func (s *Server) beginNativeTurn(ctx context.Context, session storage.Session, message string, claimed bool) (storage.Session, string, bool, error) {
	unlock := s.lockSession(session.ID)
	defer unlock()

	current, err := s.Store.LoadSession(session.ID)
	if err != nil {
		return session, storage.StatusError, false, err
	}
	session = current
	if session.Status == storage.StatusRunning && !claimed {
		return session, storage.StatusRunning, false, fmt.Errorf("session %s is already running", session.Slug)
	}
	if err := s.ensureManagedWorktree(ctx, session); err != nil {
		return session, storage.StatusError, false, err
	}
	existingMessages, err := s.Store.LoadMessages(session.ID)
	if err != nil {
		return session, storage.StatusError, false, err
	}
	session.Status = storage.StatusRunning
	session.Error = ""
	if session.Runtime == "" {
		session.Runtime = storage.RuntimeNative
	}
	if err := s.applyNativeSessionDefaults(&session); err != nil {
		return session, storage.StatusError, false, err
	}
	storage.MarkSessionAttention(&session, time.Now().UTC())
	generateTitle := shouldGenerateTitleFromMessage(session.Title, message, existingMessages)
	if session.Title == "" {
		session.Title = titleFromMessage(message)
	}
	if err := s.Store.SaveSession(session); err != nil {
		return session, storage.StatusError, false, err
	}
	s.publishSessionChanged(session.ID)
	return session, storage.StatusRunning, generateTitle, nil
}

func (s *Server) nativeSessionDefaults() (storage.CreateSession, error) {
	defaults, err := s.agentDefaults()
	if err != nil {
		return storage.CreateSession{}, err
	}
	return storage.CreateSession{
		Runtime:         storage.RuntimeNative,
		ModelProvider:   strings.TrimSpace(defaults.Native.ModelProvider),
		Model:           strings.TrimSpace(defaults.Native.Model),
		ReasoningEffort: strings.TrimSpace(defaults.Native.ReasoningEffort),
	}, nil
}

func (s *Server) applyNativeSessionDefaults(session *storage.Session) error {
	defaults, err := s.nativeSessionDefaults()
	if err != nil {
		return err
	}
	if defaults.ModelProvider != "" && session.ModelProvider == "" {
		session.ModelProvider = defaults.ModelProvider
	}
	if defaults.Model != "" && session.Model == "" {
		session.Model = defaults.Model
	}
	if defaults.ReasoningEffort != "" && session.ReasoningEffort == "" {
		session.ReasoningEffort = defaults.ReasoningEffort
	}
	return s.validateNativeProviderRunnable(session.ModelProvider)
}

func (s *Server) agentDefaults() (agentsettings.AgentDefaults, error) {
	store, ok := s.Store.(storage.SettingsStorage)
	if !ok {
		return agentsettings.AgentDefaults{}, fmt.Errorf("settings store is not configured")
	}
	defaults, err := agentsettings.LoadAgentDefaults(store)
	if err != nil {
		if !errors.Is(err, storage.ErrSettingNotFound) {
			return agentsettings.AgentDefaults{}, err
		}
		seed := s.agentSettingsSeed()
		if _, err := agentsettings.SaveAgentDefaults(store, seed); err != nil {
			return agentsettings.AgentDefaults{}, err
		}
		defaults = seed
	}
	native, err := agentsettings.NormalizeNativeDefaults(defaults.Native)
	if err != nil {
		return agentsettings.AgentDefaults{}, err
	}
	defaults.Native = native
	return defaults, nil
}

type nativeTurnRequest = jazagent.TurnRequest

func (s *Server) nativeRequestMessages(session storage.Session, message string, attachments []storage.Attachment, voiceMode bool, planRequested bool) (nativeTurnRequest, error) {
	var prompts jazagent.PromptSource
	if s.Prompts != nil {
		prompts = s.Prompts
	}
	return jazagent.BuildRequest(s.Store, prompts, jazagent.Request{
		Session:       session,
		Message:       message,
		Attachments:   attachments,
		VoiceMode:     voiceMode,
		PlanRequested: planRequested,
		AppendUser:    true,
	})
}

func (s *Server) nativeMediaRefs(sessionID string) (map[string][]media.Ref, error) {
	return jazagent.MediaRefs(s.Store, sessionID)
}
