package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/agent"
	"github.com/wins/jaz/backend/internal/storage"
)

type acpTurnKind int

const (
	acpTurnPrompt acpTurnKind = iota
	acpTurnCompact
)

var errACPTurnRunning = errors.New("session is already running")

type acpStreamTurn struct {
	Kind          acpTurnKind
	Message       string
	Contexts      []storage.MessageContext
	Attachments   []storage.Attachment
	PlanRequested bool
	GoalRequested bool
}

func acpStreamTurnFromRequest(req streamRequest) acpStreamTurn {
	turn := acpStreamTurn{
		Kind:          acpTurnPrompt,
		Message:       req.Message,
		PlanRequested: req.PlanRequested,
		GoalRequested: req.GoalRequested,
	}
	if isACPCompactCommand(req.Message) {
		turn.Kind = acpTurnCompact
		turn.GoalRequested = false
	}
	return turn
}

func (t acpStreamTurn) compact() bool {
	return t.Kind == acpTurnCompact
}

func (s *Server) streamACPSession(w http.ResponseWriter, flusher http.Flusher, clientCtx context.Context, session storage.Session, turn acpStreamTurn) {
	if s.ACP == nil {
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamError, Error: "acp manager is not configured"})
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDone})
		return
	}

	var err error
	turnTitle := turn.Message
	if turn.compact() {
		turnTitle = "Compact"
	}
	session, err = s.beginACPTurn(clientCtx, session, turnTitle)
	if err != nil {
		if !turn.compact() && errors.Is(err, errACPTurnRunning) {
			err = s.queueACPStreamTurn(session.ID, turn)
			if err == nil {
				writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDone})
				return
			}
		}
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamError, Error: err.Error()})
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDone})
		return
	}
	releaseStream := s.ACP.RetainStream(session.ID)
	defer releaseStream()
	startCtx, cancelStart := serverActionContextFrom(clientCtx)
	var job acp.Job
	if turn.compact() {
		job, err = s.ACP.Compact(startCtx, acp.CompactRequest{Session: session.ID})
	} else {
		job, err = s.ACP.Send(startCtx, acp.SendRequest{
			Session:       session.ID,
			Message:       turn.Message,
			Contexts:      turn.Contexts,
			Attachments:   turn.Attachments,
			Completion:    acp.CompletionInline,
			PlanRequested: turn.PlanRequested,
			GoalRequested: turn.GoalRequested,
		})
	}
	cancelStart()
	if err != nil {
		sendErr := acpSendError(session, err)
		s.logger().Error("acp send failed", "session", session.ID, "error", sendErr)
		s.setSessionError(session, sendErr.Error())
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamError, Error: sendErr.Error()})
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDone})
		return
	}
	s.publishMessagesChanged(session.ID)

	stream := acp.StreamViewFromJob(job)
	emittedAssistant := 0
	emittedThought := 0
	seenTools := map[string]struct{}{}
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()

	for {
		emitACPStream(w, flusher, stream, &emittedAssistant, &emittedThought, seenTools)
		if stream.State == acp.StateFailed {
			s.setSessionError(session, stream.Error)
			writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamError, Error: stream.Error})
			writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDone})
			return
		}
		if isACPTerminal(stream.State) {
			s.setSessionStatus(session, storage.StatusIdle)
			writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDone})
			return
		}
		select {
		case <-clientCtx.Done():
			return
		case <-ticker.C:
			stream, err = s.ACP.StreamStatus(session.ID)
			if err != nil {
				s.setSessionError(session, err.Error())
				writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamError, Error: err.Error()})
				writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDone})
				return
			}
		}
	}
}

func (s *Server) beginACPTurn(ctx context.Context, session storage.Session, message string) (storage.Session, error) {
	unlock := s.lockSession(session.ID)
	defer unlock()

	current, err := s.Store.LoadSession(session.ID)
	if err != nil {
		return session, err
	}
	session = current
	if session.Status == storage.StatusRunning || s.sessionRuntimeRunning(session) {
		return session, errACPTurnRunning
	}
	if err := s.ensureManagedWorktree(ctx, session); err != nil {
		return session, err
	}
	markSessionRunning(&session)
	if session.Title == "" {
		session.Title = titleFromMessage(message)
	}
	if err := s.Store.SaveSession(session); err != nil {
		return session, err
	}
	s.maybeGenerateSessionTitle(session, message)
	s.publishSessionChanged(session.ID)
	return session, nil
}

func (s *Server) queueACPStreamTurn(sessionID string, turn acpStreamTurn) error {
	attachmentIDs := make([]string, 0, len(turn.Attachments))
	for _, attachment := range turn.Attachments {
		attachmentIDs = append(attachmentIDs, attachment.ID)
	}
	_, err := s.updateSessionQueue(sessionID, queueRequest{
		Op: "append",
		Message: storage.QueuedMessage{
			Text:          turn.Message,
			Contexts:      turn.Contexts,
			AttachmentIDs: attachmentIDs,
			PlanRequested: turn.PlanRequested,
			GoalRequested: turn.GoalRequested,
		},
	})
	return err
}

func emitACPStream(w http.ResponseWriter, flusher http.Flusher, stream acp.StreamView, emittedAssistant, emittedThought *int, seenTools map[string]struct{}) {
	for _, call := range stream.Tools {
		key := firstNonEmpty(call.ID, call.Title)
		if key == "" {
			continue
		}
		if _, ok := seenTools[key]; ok {
			continue
		}
		seenTools[key] = struct{}{}
		writeSSE(w, flusher, agent.StreamEvent{
			Type:     agent.StreamToolCall,
			ToolName: firstNonEmpty(call.Title, call.ID),
		})
	}
	if *emittedAssistant < len(stream.Assistant) {
		delta := stream.Assistant[*emittedAssistant:]
		*emittedAssistant = len(stream.Assistant)
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamDelta, Delta: delta})
	}
	if *emittedThought < len(stream.Thought) {
		delta := stream.Thought[*emittedThought:]
		*emittedThought = len(stream.Thought)
		writeSSE(w, flusher, agent.StreamEvent{Type: agent.StreamReasoning, Reasoning: delta})
	}
}

func isACPTerminal(state string) bool {
	return state == acp.StateIdle || state == acp.StateCancelled
}

func acpSendError(session storage.Session, err error) error {
	if strings.Contains(err.Error(), "active acp session not found") {
		return fmt.Errorf("acp session %q (%s) could not be resumed: %v", session.Slug, session.ID, err)
	}
	return err
}
