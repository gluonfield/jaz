package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/media"
	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	agentsettings "github.com/wins/jaz/backend/internal/settings"
	"github.com/wins/jaz/backend/internal/storage"
)

// voiceModeNote steers spoken turns; it is injected per-request and stripped
// before messages are persisted, so transcripts stay clean.
const voiceModeNote = "Voice mode: the user spoke this message aloud and your final reply will be read out by text-to-speech. Keep the final response to a few short conversational sentences of plain prose — no markdown, lists, headings, or code blocks. Using tools is fine."

// stripTransientSystem removes injected system messages — the per-turn system
// prompt, the voice note — before persisting (the agent echoes the full
// request message list back, and SaveMessages replaces the stored list
// wholesale), remapping reasoning indexes onto the stripped list.
func stripTransientSystem(messages []provider.Message, reasoning map[int]string, injected []string) ([]provider.Message, map[int]string) {
	if len(injected) == 0 {
		return messages, reasoning
	}
	drop := make(map[int]bool)
	for i, msg := range messages {
		if msg.OfSystem == nil {
			continue
		}
		if slices.Contains(injected, msg.OfSystem.Content.OfString.Value) {
			drop[i] = true
		}
	}
	if len(drop) == 0 {
		return messages, reasoning
	}
	out := make([]provider.Message, 0, len(messages)-len(drop))
	remapped := make(map[int]string, len(reasoning))
	for i, msg := range messages {
		if drop[i] {
			continue
		}
		if text, ok := reasoning[i]; ok {
			remapped[len(out)] = text
		}
		out = append(out, msg)
	}
	if len(reasoning) == 0 {
		return out, reasoning
	}
	return out, remapped
}

func (s *Server) streamNativeSession(w http.ResponseWriter, flusher http.Flusher, r *http.Request, session storage.Session, message string, attachments []storage.Attachment, voiceMode bool) {
	clientCtx := r.Context()
	send := func(event agent.StreamEvent) {
		if clientCtx.Err() == nil {
			writeSSE(w, flusher, event)
		}
	}
	if s.runNativeSessionWithAttachments(context.WithoutCancel(clientCtx), session, message, attachments, voiceMode, send) == storage.StatusIdle {
		s.drainQueueSoon(session.ID)
	}
}

func (s *Server) runNativeSession(ctx context.Context, session storage.Session, message string, voiceMode bool, send func(agent.StreamEvent)) string {
	return s.runNativeSessionWithClaim(ctx, session, message, nil, voiceMode, send, false)
}

func (s *Server) runNativeSessionWithAttachments(ctx context.Context, session storage.Session, message string, attachments []storage.Attachment, voiceMode bool, send func(agent.StreamEvent)) string {
	return s.runNativeSessionWithClaim(ctx, session, message, attachments, voiceMode, send, false)
}

func (s *Server) runClaimedNativeSession(ctx context.Context, session storage.Session, message string) string {
	return s.runNativeSessionWithClaim(ctx, session, message, nil, false, nil, true)
}

func (s *Server) runNativeSessionWithClaim(ctx context.Context, session storage.Session, message string, attachments []storage.Attachment, voiceMode bool, send func(agent.StreamEvent), claimed bool) string {
	if send == nil {
		send = func(agent.StreamEvent) {}
	}
	session, startStatus, err := s.beginNativeTurn(session, message, claimed)
	if err != nil {
		send(agent.StreamEvent{Type: agent.StreamError, Error: err.Error()})
		send(agent.StreamEvent{Type: agent.StreamDone})
		return startStatus
	}

	logger := s.logger().With("session", session.ID)
	logger.Info("native turn started")
	turnCtx, cancelTurn := context.WithCancel(ctx)
	defer cancelTurn()
	s.turnCancels.Store(session.ID, cancelTurn)
	defer s.turnCancels.Delete(session.ID)

	turn, err := s.nativeRequestMessages(session.ID, message, attachments, voiceMode)
	if err != nil {
		send(agent.StreamEvent{Type: agent.StreamError, Error: err.Error()})
		send(agent.StreamEvent{Type: agent.StreamDone})
		s.setSessionError(session, err.Error())
		return storage.StatusError
	}
	s.publishMessagesChanged(session.ID)

	runCtx := sessioncontext.WithSessionID(turnCtx, session.ID)
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
			toSave, reasoning := stripTransientSystem(turn.storageSnapshot(event.Messages), event.ReasoningByMessage, turn.Transient)
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

func (s *Server) beginNativeTurn(session storage.Session, message string, claimed bool) (storage.Session, string, error) {
	unlock := s.lockSession(session.ID)
	defer unlock()

	current, err := s.Store.LoadSession(session.ID)
	if err != nil {
		return session, storage.StatusError, err
	}
	session = current
	if session.Status == storage.StatusRunning && !claimed {
		return session, storage.StatusRunning, fmt.Errorf("session %s is already running", session.Slug)
	}
	session.Status = storage.StatusRunning
	if session.Runtime == "" {
		session.Runtime = storage.RuntimeNative
	}
	if err := s.applyNativeSessionDefaults(&session); err != nil {
		return session, storage.StatusError, err
	}
	if session.Title == "" {
		session.Title = titleFromMessage(message)
	}
	if err := s.Store.SaveSession(session); err != nil {
		return session, storage.StatusError, err
	}
	return session, storage.StatusRunning, nil
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
	return nil
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

type nativeTurnRequest struct {
	Messages         []provider.Message
	MediaRefs        map[string][]media.Ref
	Transient        []string
	userMessageIndex int
	displayUser      provider.Message
}

func (r nativeTurnRequest) storageSnapshot(messages []provider.Message) []provider.Message {
	if r.userMessageIndex < 0 || r.userMessageIndex >= len(messages) {
		return messages
	}
	out := append([]provider.Message(nil), messages...)
	out[r.userMessageIndex] = r.displayUser
	return out
}

func (s *Server) nativeRequestMessages(sessionID, message string, attachments []storage.Attachment, voiceMode bool) (nativeTurnRequest, error) {
	messages, err := s.Store.LoadMessages(sessionID)
	if err != nil {
		return nativeTurnRequest{}, err
	}
	mediaRefs, err := s.nativeMediaRefs(sessionID)
	if err != nil {
		return nativeTurnRequest{}, err
	}
	// The system prompt is derived fresh for every turn (skills, SOUL.md and
	// friends are re-read from disk) and never persisted. Older sessions stored
	// it as their first message; drop that copy so the fresh one replaces it.
	for len(messages) > 0 && messages[0].OfSystem != nil {
		messages = messages[1:]
	}
	transient := make([]string, 0, 2)
	if s.Prompts != nil {
		if prompt := strings.TrimSpace(s.Prompts.SystemPrompt()); prompt != "" {
			messages = append([]provider.Message{provider.SystemMessage(prompt)}, messages...)
			transient = append(transient, prompt)
		}
	}
	if voiceMode {
		messages = append(messages, provider.SystemMessage(voiceModeNote))
		transient = append(transient, voiceModeNote)
	}
	userMessageIndex := len(messages)
	messages = append(messages, provider.UserMessage(nativeMessageWithAttachmentLinks(message, attachments)))
	if err := storage.AppendUserMessage(s.Store, sessionID, message, attachments); err != nil {
		return nativeTurnRequest{}, err
	}
	return nativeTurnRequest{
		Messages:         messages,
		MediaRefs:        mediaRefs,
		Transient:        transient,
		userMessageIndex: userMessageIndex,
		displayUser:      provider.UserMessage(message),
	}, nil
}

func (s *Server) nativeMediaRefs(sessionID string) (map[string][]media.Ref, error) {
	recordStore, ok := s.Store.(messageRecordStore)
	if !ok {
		return nil, nil
	}
	records, err := recordStore.LoadMessageRecords(sessionID)
	if err != nil {
		return nil, err
	}
	return storage.MediaRefsByToolCall(records), nil
}
